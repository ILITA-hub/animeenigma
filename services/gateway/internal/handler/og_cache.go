package handler

import (
	"container/list"
	"sync"
	"time"
)

// ogCacheCapacity bounds the number of rendered Open Graph pages held in
// memory at once. Each entry is a ~2 KB HTML page, so this cap keeps the OG
// cache's footprint at a few MB regardless of how many distinct keys an
// (unauthenticated) caller requests — the invariant that closes the
// unbounded-heap-growth DoS (CWE-770). Well beyond any realistic crawler
// working set for a self-hosted deployment.
const ogCacheCapacity = 4096

// ogCache is a fixed-capacity, concurrency-safe LRU of rendered OG pages. It
// replaces the previous unbounded sync.Map: no matter how many distinct keys
// arrive, the map never holds more than ogCacheCapacity entries — the oldest is
// evicted first. TTL is still enforced by callers on read (and reclaimed in
// bulk by purgeExpired); the LRU cap is the memory-bound backstop on top of it.
type ogCache struct {
	mu    sync.Mutex
	cap   int
	ll    *list.List // front = most recently used, back = eviction candidate
	items map[string]*list.Element
}

// ogCacheNode is the value stored in each list element.
type ogCacheNode struct {
	key   string
	entry *ogCacheEntry
}

func newOGCache(capacity int) *ogCache {
	if capacity < 1 {
		capacity = 1
	}
	return &ogCache{
		cap:   capacity,
		ll:    list.New(),
		items: make(map[string]*list.Element, capacity),
	}
}

// Load returns the entry for key and marks it most-recently-used.
func (c *ogCache) Load(key string) (*ogCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.ll.MoveToFront(el)
	return el.Value.(*ogCacheNode).entry, true
}

// Store inserts or updates key, evicting the least-recently-used entries until
// the cap is respected.
func (c *ogCache) Store(key string, entry *ogCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value.(*ogCacheNode).entry = entry
		c.ll.MoveToFront(el)
		return
	}
	el := c.ll.PushFront(&ogCacheNode{key: key, entry: entry})
	c.items[key] = el
	for c.ll.Len() > c.cap {
		oldest := c.ll.Back()
		if oldest == nil {
			break
		}
		c.ll.Remove(oldest)
		delete(c.items, oldest.Value.(*ogCacheNode).key)
	}
}

// Delete removes key if present.
func (c *ogCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.ll.Remove(el)
		delete(c.items, key)
	}
}

// Len reports the current number of entries (used by tests and metrics).
func (c *ogCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}

// purgeExpired drops every entry whose TTL has elapsed as of now. This preserves
// the previous background sweeper's behaviour (expired entries are reclaimed
// even if never read again), now folded onto the bounded structure.
func (c *ogCache) purgeExpired(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for el := c.ll.Back(); el != nil; {
		prev := el.Prev()
		node := el.Value.(*ogCacheNode)
		if now.After(node.entry.expiresAt) {
			c.ll.Remove(el)
			delete(c.items, node.key)
		}
		el = prev
	}
}
