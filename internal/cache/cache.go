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
	mu       sync.RWMutex
	entries  map[string]CacheEntry
	ttl      time.Duration
	stopCh   chan struct{}
	stopOnce sync.Once
	bytes    int
}

const MaxBytes = 16 << 20
const MaxEntries = 128

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
	size := len(key) + len(htmlContent) + len(markdownContent)
	if size > MaxBytes {
		return
	}
	if old, ok := c.entries[key]; ok {
		c.bytes -= len(key) + len(old.HTMLContent) + len(old.MarkdownContent)
		delete(c.entries, key)
	}
	for len(c.entries) >= MaxEntries || c.bytes+size > MaxBytes {
		var oldestKey string
		var oldestTime time.Time
		for k, entry := range c.entries {
			if oldestTime.IsZero() || entry.Timestamp.Before(oldestTime) {
				oldestKey, oldestTime = k, entry.Timestamp
			}
		}
		old := c.entries[oldestKey]
		c.bytes -= len(oldestKey) + len(old.HTMLContent) + len(old.MarkdownContent)
		delete(c.entries, oldestKey)
	}

	c.entries[key] = CacheEntry{
		HTMLContent:     htmlContent,
		MarkdownContent: markdownContent,
		Timestamp:       time.Now(),
	}
	c.bytes += size
}

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]CacheEntry)
	c.bytes = 0
}

func (c *Cache) Stop() {
	c.stopOnce.Do(func() { close(c.stopCh) })
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
			c.bytes -= len(key) + len(entry.HTMLContent) + len(entry.MarkdownContent)
			delete(c.entries, key)
		}
	}
}
