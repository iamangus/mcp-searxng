package cache

import (
	"sync"
	"time"
)

type CacheEntry struct {
	HTMLContent     string
	MarkdownContent string
	Timestamp       time.Time
}

type Cache struct {
	mu      sync.RWMutex
	entries map[string]CacheEntry
	ttl     time.Duration
	stopCh  chan struct{}
}

func NewCache(ttl time.Duration) *Cache {
	c := &Cache{
		entries: make(map[string]CacheEntry),
		ttl:     ttl,
		stopCh:  make(chan struct{}),
	}
	go c.cleanupLoop()
	return c
}

func (c *Cache) Get(key string) (CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok {
		return CacheEntry{}, false
	}

	if time.Since(entry.Timestamp) > c.ttl {
		return CacheEntry{}, false
	}

	return entry, true
}

func (c *Cache) Set(key, htmlContent, markdownContent string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = CacheEntry{
		HTMLContent:     htmlContent,
		MarkdownContent: markdownContent,
		Timestamp:       time.Now(),
	}
}

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]CacheEntry)
}

func (c *Cache) Stop() {
	close(c.stopCh)
}

func (c *Cache) Stats() (size int, entries []string) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	size = len(c.entries)
	for k := range c.entries {
		entries = append(entries, k)
	}
	return
}

func (c *Cache) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCh:
			return
		}
	}
}

func (c *Cache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.entries {
		if now.Sub(entry.Timestamp) > c.ttl {
			delete(c.entries, key)
		}
	}
}
