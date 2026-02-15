package ctarchiveserve

import (
	"container/list"
	"sync"
)

// EntryContentCache is a memory-budgeted LRU cache for decompressed zip entry content.
//
// It eliminates repeated decompression for frequently accessed tiles by caching the
// raw bytes of zip entries keyed by zipPath + entryName. The cache enforces a maximum
// memory budget (in bytes) and evicts the least recently used entries when the budget
// is exceeded.
//
// All operations are safe for concurrent use.
type EntryContentCache struct {
	maxBytes int64
	metrics  *Metrics

	mu       sync.RWMutex
	items    map[string]*list.Element // compositeKey -> *list.Element
	lru      *list.List               // front = most recently used
	curBytes int64
}

// entryCacheItem is stored in the LRU list.
type entryCacheItem struct {
	key  string // composite key: zipPath + "\x00" + entryName
	data []byte
}

// NewEntryContentCache constructs a new EntryContentCache.
//
// maxBytes is the maximum total bytes of decompressed content to cache.
// If maxBytes <= 0, the cache is effectively disabled (Get always misses, Put is a no-op).
func NewEntryContentCache(maxBytes int64, metrics *Metrics) *EntryContentCache {
	return &EntryContentCache{
		maxBytes: maxBytes,
		metrics:  metrics,
		items:    make(map[string]*list.Element),
		lru:      list.New(),
	}
}

// compositeKey builds a cache key from zipPath and entryName.
// Uses null byte separator which cannot appear in file paths.
func compositeKey(zipPath, entryName string) string {
	return zipPath + "\x00" + entryName
}

// Get returns the cached decompressed content for the given zip entry.
// Returns (data, true) on cache hit, or (nil, false) on cache miss.
//
// The returned byte slice MUST NOT be modified by the caller.
func (c *EntryContentCache) Get(zipPath, entryName string) ([]byte, bool) {
	if c == nil || c.maxBytes <= 0 {
		return nil, false
	}

	key := compositeKey(zipPath, entryName)

	c.mu.RLock()
	elem, ok := c.items[key]
	if !ok {
		c.mu.RUnlock()
		if c.metrics != nil {
			c.metrics.IncEntryCacheMisses()
		}
		return nil, false
	}
	c.mu.RUnlock()

	// Promote to front under write lock.
	c.mu.Lock()
	c.lru.MoveToFront(elem)
	c.mu.Unlock()

	item, _ := elem.Value.(*entryCacheItem) //nolint:errcheck // internal invariant: LRU list only contains *entryCacheItem

	if c.metrics != nil {
		c.metrics.IncEntryCacheHits()
	}

	return item.data, true
}

// Put stores the decompressed content for the given zip entry.
//
// If the entry is larger than maxBytes, it is not cached.
// If storing the entry would exceed the memory budget, LRU entries are evicted
// until there is room.
func (c *EntryContentCache) Put(zipPath, entryName string, data []byte) {
	if c == nil || c.maxBytes <= 0 {
		return
	}

	size := int64(len(data))
	if size > c.maxBytes {
		// Single entry larger than entire budget; skip.
		return
	}

	key := compositeKey(zipPath, entryName)

	c.mu.Lock()
	defer c.mu.Unlock()

	// If already cached, update in place.
	if elem, ok := c.items[key]; ok {
		old, _ := elem.Value.(*entryCacheItem) //nolint:errcheck // internal invariant: LRU list only contains *entryCacheItem
		c.curBytes -= int64(len(old.data))
		old.data = data
		c.curBytes += size
		c.lru.MoveToFront(elem)
		c.evictUntilBudget()
		return
	}

	// Evict until we have room.
	for c.curBytes+size > c.maxBytes && c.lru.Len() > 0 {
		c.evictBack()
	}

	item := &entryCacheItem{key: key, data: data}
	elem := c.lru.PushFront(item)
	c.items[key] = elem
	c.curBytes += size

	if c.metrics != nil {
		c.metrics.SetEntryCacheBytes(c.curBytes)
		c.metrics.SetEntryCacheItems(c.lru.Len())
	}
}

// Invalidate removes all cached entries for the given zipPath.
// This should be called when a zip file is known to have changed or become invalid.
func (c *EntryContentCache) Invalidate(zipPath string) {
	if c == nil {
		return
	}

	prefix := zipPath + "\x00"

	c.mu.Lock()
	defer c.mu.Unlock()

	// Collect keys to delete (can't modify map during iteration in all cases).
	var toDelete []string
	for key := range c.items {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			toDelete = append(toDelete, key)
		}
	}

	for _, key := range toDelete {
		if elem, ok := c.items[key]; ok {
			item, _ := elem.Value.(*entryCacheItem) //nolint:errcheck // internal invariant: LRU list only contains *entryCacheItem
			c.curBytes -= int64(len(item.data))
			c.lru.Remove(elem)
			delete(c.items, key)
		}
	}

	if c.metrics != nil {
		c.metrics.SetEntryCacheBytes(c.curBytes)
		c.metrics.SetEntryCacheItems(c.lru.Len())
	}
}

// evictBack removes the least recently used entry. Caller must hold c.mu.
func (c *EntryContentCache) evictBack() {
	elem := c.lru.Back()
	if elem == nil {
		return
	}

	c.lru.Remove(elem)
	item, _ := elem.Value.(*entryCacheItem) //nolint:errcheck // internal invariant: LRU list only contains *entryCacheItem
	c.curBytes -= int64(len(item.data))
	delete(c.items, item.key)

	if c.metrics != nil {
		c.metrics.IncEntryCacheEvictions()
		c.metrics.SetEntryCacheBytes(c.curBytes)
		c.metrics.SetEntryCacheItems(c.lru.Len())
	}
}

// evictUntilBudget evicts LRU entries until curBytes <= maxBytes. Caller must hold c.mu.
func (c *EntryContentCache) evictUntilBudget() {
	for c.curBytes > c.maxBytes && c.lru.Len() > 0 {
		c.evictBack()
	}
}
