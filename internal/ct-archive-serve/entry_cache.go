package ctarchiveserve

import (
	"container/list"
	"hash/fnv"
	"strings"
	"sync"
)

// defaultEntryContentShards is the number of internal shards used to reduce lock
// contention under high concurrency. With 64 shards, concurrent goroutines almost
// always hit distinct shards.
const defaultEntryContentShards = 64

// EntryContentCache is a sharded, memory-budgeted LRU cache for decompressed zip
// entry content.
//
// It eliminates repeated decompression for frequently accessed tiles by caching the
// raw bytes of zip entries keyed by zipPath + entryName. The total memory budget is
// distributed evenly across internal shards. Each shard has its own RWMutex and LRU
// list, so concurrent requests for entries in different shards do not contend.
//
// All operations are safe for concurrent use.
type EntryContentCache struct {
	metrics   *Metrics
	shards    []entryContentShard
	numShards uint64
}

// entryContentShard is a single shard of the EntryContentCache.
type entryContentShard struct {
	mu       sync.RWMutex
	items    map[string]*list.Element // compositeKey -> *list.Element
	lru      *list.List               // front = most recently used
	curBytes int64
	maxBytes int64
}

// entryCacheItem is stored in the LRU list.
type entryCacheItem struct {
	key  string // composite key: zipPath + "\x00" + entryName
	data []byte
}

// NewEntryContentCache constructs a new sharded EntryContentCache.
//
// maxBytes is the maximum total bytes of decompressed content to cache across all
// shards. If maxBytes <= 0, the cache is effectively disabled (Get always misses,
// Put is a no-op).
func NewEntryContentCache(maxBytes int64, metrics *Metrics) *EntryContentCache {
	numShards := uint64(defaultEntryContentShards)
	perShard := maxBytes / int64(numShards)
	if perShard < 1 && maxBytes > 0 {
		perShard = 1
	}

	shards := make([]entryContentShard, numShards)
	for i := range shards {
		shards[i] = entryContentShard{
			items:    make(map[string]*list.Element),
			lru:      list.New(),
			maxBytes: perShard,
		}
	}

	return &EntryContentCache{
		metrics:   metrics,
		shards:    shards,
		numShards: numShards,
	}
}

// compositeKey builds a cache key from zipPath and entryName.
// Uses null byte separator which cannot appear in file paths.
func compositeKey(zipPath, entryName string) string {
	return zipPath + "\x00" + entryName
}

// shardFor returns the shard for the given composite key using FNV-1a hashing.
func (c *EntryContentCache) shardFor(key string) *entryContentShard {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key)) // fnv hash.Write never returns an error
	return &c.shards[h.Sum64()%c.numShards]
}

// maxBytes returns the total max bytes budget across all shards.
func (c *EntryContentCache) maxBytes() int64 {
	if c == nil || len(c.shards) == 0 {
		return 0
	}
	return c.shards[0].maxBytes * int64(c.numShards) //nolint:gosec // numShards is a small constant (64); overflow is not possible
}

// Get returns the cached decompressed content for the given zip entry.
// Returns (data, true) on cache hit, or (nil, false) on cache miss.
//
// The returned byte slice MUST NOT be modified by the caller.
func (c *EntryContentCache) Get(zipPath, entryName string) ([]byte, bool) {
	if c == nil || c.maxBytes() <= 0 {
		return nil, false
	}

	key := compositeKey(zipPath, entryName)
	shard := c.shardFor(key)

	shard.mu.RLock()
	elem, ok := shard.items[key]
	if !ok {
		shard.mu.RUnlock()
		if c.metrics != nil {
			c.metrics.IncEntryCacheMisses()
		}
		return nil, false
	}
	shard.mu.RUnlock()

	// Promote to front under shard write lock.
	shard.mu.Lock()
	shard.lru.MoveToFront(elem)
	shard.mu.Unlock()

	item, _ := elem.Value.(*entryCacheItem) //nolint:errcheck // internal invariant: LRU list only contains *entryCacheItem

	if c.metrics != nil {
		c.metrics.IncEntryCacheHits()
	}

	return item.data, true
}

// Put stores the decompressed content for the given zip entry.
//
// If the entry is larger than the per-shard budget, it is not cached.
// If storing the entry would exceed the shard's memory budget, LRU entries are
// evicted until there is room.
func (c *EntryContentCache) Put(zipPath, entryName string, data []byte) {
	if c == nil || c.maxBytes() <= 0 {
		return
	}

	key := compositeKey(zipPath, entryName)
	shard := c.shardFor(key)
	size := int64(len(data))

	if size > shard.maxBytes {
		// Single entry larger than shard budget; skip.
		return
	}

	shard.mu.Lock()
	defer shard.mu.Unlock()

	// If already cached, update in place.
	if elem, ok := shard.items[key]; ok {
		old, _ := elem.Value.(*entryCacheItem) //nolint:errcheck // internal invariant: LRU list only contains *entryCacheItem
		shard.curBytes -= int64(len(old.data))
		old.data = data
		shard.curBytes += size
		shard.lru.MoveToFront(elem)
		evictShardUntilBudget(c, shard)
		return
	}

	// Evict until we have room.
	for shard.curBytes+size > shard.maxBytes && shard.lru.Len() > 0 {
		evictShardBack(c, shard)
	}

	item := &entryCacheItem{key: key, data: data}
	elem := shard.lru.PushFront(item)
	shard.items[key] = elem
	shard.curBytes += size

	if c.metrics != nil {
		totalBytes, totalItems := c.totals()
		c.metrics.SetEntryCacheBytes(totalBytes)
		c.metrics.SetEntryCacheItems(totalItems)
	}
}

// Invalidate removes all cached entries for the given zipPath.
// This should be called when a zip file is known to have changed or become invalid.
//
// Because entries for the same zipPath with different entryNames may hash to different
// shards, this method scans all shards. This is the invalidation path, not the hot
// path, so iterating all shards is acceptable.
func (c *EntryContentCache) Invalidate(zipPath string) {
	if c == nil {
		return
	}

	prefix := zipPath + "\x00"

	for i := range c.shards {
		shard := &c.shards[i]
		shard.mu.Lock()

		// Collect keys to delete.
		var toDelete []string
		for key := range shard.items {
			if strings.HasPrefix(key, prefix) {
				toDelete = append(toDelete, key)
			}
		}

		for _, key := range toDelete {
			if elem, ok := shard.items[key]; ok {
				item, _ := elem.Value.(*entryCacheItem) //nolint:errcheck // internal invariant: LRU list only contains *entryCacheItem
				shard.curBytes -= int64(len(item.data))
				shard.lru.Remove(elem)
				delete(shard.items, key)
			}
		}

		shard.mu.Unlock()
	}

	if c.metrics != nil {
		totalBytes, totalItems := c.totals()
		c.metrics.SetEntryCacheBytes(totalBytes)
		c.metrics.SetEntryCacheItems(totalItems)
	}
}

// evictShardBack removes the least recently used entry from the given shard.
// Caller must hold shard.mu.
func evictShardBack(c *EntryContentCache, shard *entryContentShard) {
	elem := shard.lru.Back()
	if elem == nil {
		return
	}

	shard.lru.Remove(elem)
	item, _ := elem.Value.(*entryCacheItem) //nolint:errcheck // internal invariant: LRU list only contains *entryCacheItem
	shard.curBytes -= int64(len(item.data))
	delete(shard.items, item.key)

	if c.metrics != nil {
		c.metrics.IncEntryCacheEvictions()
	}
}

// evictShardUntilBudget evicts LRU entries from the given shard until
// curBytes <= maxBytes. Caller must hold shard.mu.
func evictShardUntilBudget(c *EntryContentCache, shard *entryContentShard) {
	for shard.curBytes > shard.maxBytes && shard.lru.Len() > 0 {
		evictShardBack(c, shard)
	}
}

// totals returns aggregate byte and item counts across all shards.
// The values are approximate when called without holding all shard locks.
func (c *EntryContentCache) totals() (totalBytes int64, totalItems int) {
	for i := range c.shards {
		totalBytes += c.shards[i].curBytes
		totalItems += c.shards[i].lru.Len()
	}
	return totalBytes, totalItems
}
