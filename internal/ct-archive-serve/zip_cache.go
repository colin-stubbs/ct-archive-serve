package ctarchiveserve

import (
	"archive/zip"
	"container/list"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"
	"golang.org/x/sync/singleflight"
)

// ErrZipTemporarilyUnavailable indicates a zip part exists but is not currently usable
// (e.g., still downloading / structurally invalid).
var ErrZipTemporarilyUnavailable = errors.New("zip temporarily unavailable")

// ZipIntegrityCache caches zip structural integrity results.
//
// Passed entries are cached for the lifetime of the process and are only removed if
// a later read attempt fails (call InvalidatePassed).
//
// Failed entries are cached with TTL to allow re-testing once the zip part becomes complete.
type ZipIntegrityCache struct {
	failTTL time.Duration
	now     func() time.Time
	verify  func(path string) error
	metrics *Metrics

	mu     sync.RWMutex
	passed map[string]struct{}
	failed map[string]time.Time // path -> expiresAt

	group singleflight.Group // deduplicates concurrent verifications of the same path
}

func NewZipIntegrityCache(
	failTTL time.Duration,
	now func() time.Time,
	verify func(path string) error,
	metrics *Metrics,
) *ZipIntegrityCache {
	if now == nil {
		now = time.Now
	}
	if verify == nil {
		verify = verifyZipStructural
	}

	return &ZipIntegrityCache{
		failTTL: failTTL,
		now:     now,
		verify:  verify,
		metrics: metrics,
		passed:  make(map[string]struct{}),
		failed:  make(map[string]time.Time),
	}
}

// Check verifies that the zip part at path is structurally valid (central directory + local headers)
// or returns ErrZipTemporarilyUnavailable.
func (z *ZipIntegrityCache) Check(path string) error {
	if z == nil {
		return nil
	}

	// Fast path: read-only check under RLock (hot path, no writes needed).
	z.mu.RLock()
	if _, ok := z.passed[path]; ok {
		z.mu.RUnlock()
		return nil
	}

	// Cached failure (unexpired) -- still read-only.
	if exp, ok := z.failed[path]; ok {
		if z.now().Before(exp) {
			z.mu.RUnlock()
			return ErrZipTemporarilyUnavailable
		}
		// Expired failure: need write lock to delete, handled below.
	}
	z.mu.RUnlock()

	// Delete expired failure under write lock -- only if the path is actually in
	// the failed map (avoid taking an exclusive lock on the common hot path where
	// the path is not in the failed map at all).
	z.mu.RLock()
	_, inFailed := z.failed[path]
	z.mu.RUnlock()
	if inFailed {
		z.mu.Lock()
		if exp, ok := z.failed[path]; ok && !z.now().Before(exp) {
			delete(z.failed, path)
		}
		z.mu.Unlock()
	}

	// Slow path: verify via singleflight to prevent thundering herd.
	_, err, _ := z.group.Do(path, func() (interface{}, error) {
		// Re-check cache inside singleflight (another goroutine may have completed).
		z.mu.RLock()
		if _, ok := z.passed[path]; ok {
			z.mu.RUnlock()
			return nil, nil
		}
		z.mu.RUnlock()

		return nil, z.verify(path)
	})

	if err != nil {
		z.mu.Lock()
		z.failed[path] = z.now().Add(z.failTTL)
		z.mu.Unlock()
		if z.metrics != nil {
			z.metrics.IncZipIntegrityFailed()
		}
		return fmt.Errorf("%w: %w", ErrZipTemporarilyUnavailable, err)
	}

	z.mu.Lock()
	z.passed[path] = struct{}{}
	delete(z.failed, path)
	z.mu.Unlock()
	if z.metrics != nil {
		z.metrics.IncZipIntegrityPassed()
	}

	return nil
}

// InvalidatePassed removes a previously-passed zip part from the passed cache.
// Callers should use this when later open/read attempts fail for that zip part.
func (z *ZipIntegrityCache) InvalidatePassed(path string) {
	if z == nil {
		return
	}
	z.mu.Lock()
	delete(z.passed, path)
	z.mu.Unlock()
}

// verifyZipStructural validates that the zip file's central directory is readable.
//
// This is a lightweight check: it only opens the zip (which parses the central
// directory and end-of-central-directory record) and verifies that at least one
// entry exists. It does NOT open or decompress individual entries, which avoids
// the O(N) I/O cost of the previous implementation (where N is the number of
// entries, typically 65K+).
//
// If an individual entry is corrupt, it will be caught at read time and the
// integrity cache will be invalidated via InvalidatePassed.
func verifyZipStructural(path string) error {
	//nolint:gosec // G304: path is validated internally from archive index, not user input
	r, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer func() { _ = r.Close() }()

	// Central directory parsed successfully by OpenReader.
	// Verify at least one entry exists (empty zip is suspicious).
	if len(r.File) == 0 {
		return errors.New("zip has no entries")
	}

	return nil
}

// ZipEntryIndex provides O(1) lookup of zip entries by name.
type ZipEntryIndex struct {
	entries map[string]*zip.File
}

// Lookup returns the zip.File for the given entry name, or nil if not found.
func (idx *ZipEntryIndex) Lookup(entryName string) *zip.File {
	if idx == nil {
		return nil
	}
	return idx.entries[entryName]
}

// ZipPartCacheEntry represents a cached zip part with its open reader and entry index.
type ZipPartCacheEntry struct {
	path     string
	reader   *zip.ReadCloser
	index    *ZipEntryIndex
	lastUsed time.Time
	element  *list.Element // back-pointer to LRU list position within its shard
}

// defaultZipPartShards is the number of internal shards used to reduce lock contention
// under high concurrency. With 64 shards and typical workloads of 45+ concurrent logs,
// each goroutine almost always hits a distinct shard.
const defaultZipPartShards = 64

// zipPartShard is a single shard of the ZipPartCache. Each shard has its own mutex,
// LRU list, entries map, and singleflight group, eliminating cross-shard lock contention.
type zipPartShard struct {
	mu      sync.Mutex
	entries map[string]*ZipPartCacheEntry
	lru     *list.List
	group   singleflight.Group
	maxOpen int
}

// ZipPartCache is a sharded, bounded LRU cache for open zip file handles and entry indices.
//
// This cache avoids repeated central-directory parsing for hot zip parts.
// The cache is internally sharded (default 64 shards) so that concurrent requests
// for different zip paths do not contend on a single lock. Each shard has its own
// mutex, LRU list, and singleflight group.
//
// A global semaphore limits concurrent zip.OpenReader calls to prevent I/O storms.
type ZipPartCache struct {
	metrics   *Metrics
	now       func() time.Time
	shards    []zipPartShard
	numShards uint64
	openSem   *semaphore.Weighted
}

// NewZipPartCache constructs a new sharded ZipPartCache.
// maxConcurrentOpens controls the maximum number of concurrent zip.OpenReader calls
// (defaults to 64 if <= 0).
func NewZipPartCache(maxOpen int, metrics *Metrics, maxConcurrentOpens int) *ZipPartCache {
	if maxOpen <= 0 {
		maxOpen = 2048 // Default
	}
	if maxConcurrentOpens <= 0 {
		maxConcurrentOpens = 64 // Default
	}

	numShards := uint64(defaultZipPartShards)
	perShard := maxOpen / int(numShards)
	if perShard < 1 {
		perShard = 1
	}

	shards := make([]zipPartShard, numShards)
	for i := range shards {
		shards[i] = zipPartShard{
			entries: make(map[string]*ZipPartCacheEntry),
			lru:     list.New(),
			maxOpen: perShard,
		}
	}

	return &ZipPartCache{
		metrics:   metrics,
		now:       time.Now,
		shards:    shards,
		numShards: numShards,
		openSem:   semaphore.NewWeighted(int64(maxConcurrentOpens)),
	}
}

// shardFor returns the shard index for the given path using FNV-1a hashing.
func (c *ZipPartCache) shardFor(path string) *zipPartShard {
	h := fnv.New64a()
	_, _ = h.Write([]byte(path)) // fnv hash.Write never returns an error
	return &c.shards[h.Sum64()%c.numShards]
}

// Get returns a cached zip part entry, or opens and caches it if not present.
// Returns the entry and nil error if found/cached, or nil and error on failure.
//
// Only the per-shard mutex is held for fast in-memory cache lookups and insertions.
// All disk I/O (zip.OpenReader, central directory parsing) is performed outside
// the mutex and deduplicated via per-shard singleflight so that concurrent requests
// for the same uncached zip path only perform the I/O once. A global semaphore
// limits concurrent zip.OpenReader calls to prevent I/O storms during cold starts.
func (c *ZipPartCache) Get(path string) (*ZipPartCacheEntry, error) {
	if c == nil {
		return nil, errors.New("zip part cache not initialized")
	}

	shard := c.shardFor(path)

	// Fast path: check cache under shard lock -- O(1) map lookup + O(1) LRU move.
	shard.mu.Lock()
	if entry, ok := shard.entries[path]; ok {
		shard.lru.MoveToFront(entry.element)
		entry.lastUsed = c.now()
		shard.mu.Unlock()
		return entry, nil
	}
	shard.mu.Unlock()

	// Slow path: open and index the zip via per-shard singleflight (no lock held).
	val, err, _ := shard.group.Do(path, func() (interface{}, error) {
		// Re-check cache inside singleflight (another goroutine may have completed).
		shard.mu.Lock()
		if entry, ok := shard.entries[path]; ok {
			shard.lru.MoveToFront(entry.element)
			entry.lastUsed = c.now()
			shard.mu.Unlock()
			return entry, nil
		}
		shard.mu.Unlock()

		// Acquire global semaphore to limit concurrent zip.OpenReader calls.
		if err := c.openSem.Acquire(context.Background(), 1); err != nil {
			return nil, fmt.Errorf("acquire open semaphore: %w", err)
		}
		defer c.openSem.Release(1)

		// Perform all disk I/O outside the mutex.
		//nolint:gosec // G304: path is validated internally from archive index, not user input
		reader, err := zip.OpenReader(path)
		if err != nil {
			return nil, fmt.Errorf("open zip reader: %w", err)
		}

		// Build entry index.
		index := &ZipEntryIndex{
			entries: make(map[string]*zip.File, len(reader.File)),
		}
		for _, f := range reader.File {
			index.entries[f.Name] = f
		}

		entry := &ZipPartCacheEntry{
			path:     path,
			reader:   reader,
			index:    index,
			lastUsed: c.now(),
		}

		// Insert into cache under shard lock.
		shard.mu.Lock()
		// Double-check: another caller may have inserted while we were opening.
		if existing, ok := shard.entries[path]; ok {
			shard.lru.MoveToFront(existing.element)
			existing.lastUsed = c.now()
			shard.mu.Unlock()
			// Close the reader we just opened; the cached one wins.
			_ = reader.Close()
			return existing, nil
		}

		// Evict LRU if at capacity -- O(1).
		if len(shard.entries) >= shard.maxOpen {
			c.evictLRU(shard)
		}

		entry.element = shard.lru.PushFront(path)
		shard.entries[path] = entry

		if c.metrics != nil {
			c.metrics.SetZipCacheOpen(c.totalOpen())
		}
		shard.mu.Unlock()

		return entry, nil
	})

	if err != nil {
		return nil, err
	}

	entry, ok := val.(*ZipPartCacheEntry)
	if !ok {
		return nil, errors.New("zip part cache: unexpected singleflight result type")
	}

	return entry, nil
}

// evictLRU removes the least recently used entry from the given shard -- O(1).
// Caller must hold shard.mu.
func (c *ZipPartCache) evictLRU(shard *zipPartShard) {
	elem := shard.lru.Back()
	if elem == nil {
		return
	}

	shard.lru.Remove(elem)

	oldestPath, _ := elem.Value.(string) //nolint:errcheck // internal invariant: LRU list only contains string path values
	entry, ok := shard.entries[oldestPath]
	if !ok {
		return
	}

	// Close resources
	_ = entry.reader.Close()
	delete(shard.entries, oldestPath)

	// Update metrics
	if c.metrics != nil {
		c.metrics.IncZipCacheEvictions()
		// Note: totalOpen() is called by the caller after eviction if needed.
	}
}

// Remove removes an entry from the cache and closes its resources -- O(1).
func (c *ZipPartCache) Remove(path string) {
	if c == nil {
		return
	}

	shard := c.shardFor(path)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	entry, ok := shard.entries[path]
	if !ok {
		return
	}

	// Remove from LRU list -- O(1).
	shard.lru.Remove(entry.element)

	// Close resources
	_ = entry.reader.Close()
	delete(shard.entries, path)

	// Update metrics
	if c.metrics != nil {
		c.metrics.SetZipCacheOpen(c.totalOpen())
	}
}

// totalOpen returns the total number of open entries across all shards.
// Callers that need an exact count should hold all shard locks; callers that
// only need a metric approximation (our case) can call this lock-free --
// the slight race is acceptable for Prometheus gauge updates.
func (c *ZipPartCache) totalOpen() int {
	total := 0
	for i := range c.shards {
		total += len(c.shards[i].entries)
	}
	return total
}
