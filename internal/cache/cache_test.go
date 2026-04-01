package cache

import (
	"sync"
	"testing"
	"time"
)

func TestNewCache(t *testing.T) {
	c := NewCache(time.Minute)
	defer c.Stop()

	if c == nil {
		t.Fatal("NewCache returned nil")
	}
	if c.entries == nil {
		t.Fatal("cache entries map is nil")
	}
}

func TestSetGetRoundtrip(t *testing.T) {
	c := NewCache(time.Minute)
	defer c.Stop()

	c.Set("key1", "<h1>hello</h1>", "# hello")
	entry, ok := c.Get("key1")
	if !ok {
		t.Fatal("Get returned false for existing key")
	}
	if entry.HTMLContent != "<h1>hello</h1>" {
		t.Errorf("HTMLContent = %q, want %q", entry.HTMLContent, "<h1>hello</h1>")
	}
	if entry.MarkdownContent != "# hello" {
		t.Errorf("MarkdownContent = %q, want %q", entry.MarkdownContent, "# hello")
	}
}

func TestGetMissingKey(t *testing.T) {
	c := NewCache(time.Minute)
	defer c.Stop()

	_, ok := c.Get("nonexistent")
	if ok {
		t.Error("Get returned true for missing key")
	}
}

func TestGetExpiredEntry(t *testing.T) {
	c := NewCache(50 * time.Millisecond)
	defer c.Stop()

	c.Set("key1", "html", "markdown")
	time.Sleep(100 * time.Millisecond)

	_, ok := c.Get("key1")
	if ok {
		t.Error("Get returned true for expired entry")
	}
}

func TestClear(t *testing.T) {
	c := NewCache(time.Minute)
	defer c.Stop()

	c.Set("a", "html-a", "md-a")
	c.Set("b", "html-b", "md-b")
	c.Set("c", "html-c", "md-c")

	c.Clear()

	_, ok := c.Get("a")
	if ok {
		t.Error("Get returned true after Clear")
	}
	_, ok = c.Get("b")
	if ok {
		t.Error("Get returned true after Clear")
	}
	_, ok = c.Get("c")
	if ok {
		t.Error("Get returned true after Clear")
	}
}

func TestStats(t *testing.T) {
	c := NewCache(time.Minute)
	defer c.Stop()

	size, entries := c.Stats()
	if size != 0 {
		t.Errorf("Stats size = %d, want 0", size)
	}
	if len(entries) != 0 {
		t.Errorf("Stats entries = %v, want empty", entries)
	}

	c.Set("key1", "html1", "md1")
	c.Set("key2", "html2", "md2")

	size, entries = c.Stats()
	if size != 2 {
		t.Errorf("Stats size = %d, want 2", size)
	}
	if len(entries) != 2 {
		t.Errorf("Stats entries len = %d, want 2", len(entries))
	}

	found := map[string]bool{"key1": false, "key2": false}
	for _, e := range entries {
		found[e] = true
	}
	for k, v := range found {
		if !v {
			t.Errorf("expected key %q in Stats entries", k)
		}
	}
}

func TestStop(t *testing.T) {
	c := NewCache(time.Minute)
	c.Set("key1", "html", "md")

	done := make(chan struct{})
	go func() {
		c.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop deadlocked")
	}
}

func TestConcurrentSetGet(t *testing.T) {
	c := NewCache(time.Minute)
	defer c.Stop()

	var wg sync.WaitGroup
	numOps := 100

	for i := 0; i < numOps; i++ {
		wg.Add(2)

		go func(idx int) {
			defer wg.Done()
			c.Set(string(rune('a'+idx%26)), "html", "md")
		}(i)

		go func(idx int) {
			defer wg.Done()
			c.Get(string(rune('a' + idx%26)))
		}(i)
	}

	wg.Wait()

	size, _ := c.Stats()
	if size == 0 {
		t.Error("expected non-zero cache size after concurrent Set/Get")
	}
}
