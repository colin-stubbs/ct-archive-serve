package ctarchiveserve

import (
	"archive/zip"
	"container/list"
	"context"
	"errors"
	"fmt"
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

	// Delete expired failure under write lock if needed.
	z.mu.Lock()
	if exp, ok := z.failed[path]; ok && !z.now().Before(exp) {
		delete(z.failed, path)
	}
	z.mu.Unlock()

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
	element  *list.Element // back-pointer to LRU list position
}

// ZipPartCache is a bounded LRU cache for open zip file handles and entry indices.
//
// This cache avoids repeated central-directory parsing for hot zip parts.
// When the cache exceeds maxOpen, LRU entries are evicted.
//
// LRU ordering uses a doubly-linked list for O(1) update/evict/remove operations.
// A semaphore limits concurrent zip.OpenReader calls to prevent I/O storms.
type ZipPartCache struct {
	maxOpen int
	metrics *Metrics
	now     func() time.Time

	mu      sync.Mutex
	entries map[string]*ZipPartCacheEntry // path -> entry
	lru     *list.List                    // front = most recently used, back = least recently used

	group   singleflight.Group     // deduplicates concurrent opens of the same path
	openSem *semaphore.Weighted    // limits concurrent zip.OpenReader calls
}

// NewZipPartCache constructs a new ZipPartCache.
// maxConcurrentOpens controls the maximum number of concurrent zip.OpenReader calls
// (defaults to 8 if <= 0).
func NewZipPartCache(maxOpen int, metrics *Metrics, maxConcurrentOpens int) *ZipPartCache {
	if maxOpen <= 0 {
		maxOpen = 256 // Default
	}
	if maxConcurrentOpens <= 0 {
		maxConcurrentOpens = 8 // Default
	}
	return &ZipPartCache{
		maxOpen: maxOpen,
		metrics: metrics,
		now:     time.Now,
		entries: make(map[string]*ZipPartCacheEntry),
		lru:     list.New(),
		openSem: semaphore.NewWeighted(int64(maxConcurrentOpens)),
	}
}

// Get returns a cached zip part entry, or opens and caches it if not present.
// Returns the entry and nil error if found/cached, or nil and error on failure.
//
// The global mutex is only held for fast in-memory cache lookups and insertions.
// All disk I/O (zip.OpenReader, central directory parsing) is performed outside
// the mutex and deduplicated via singleflight so that concurrent requests for
// the same uncached zip path only perform the I/O once. A semaphore limits
// concurrent zip.OpenReader calls to prevent I/O storms during cold starts.
func (c *ZipPartCache) Get(path string) (*ZipPartCacheEntry, error) {
	if c == nil {
		return nil, errors.New("zip part cache not initialized")
	}

	// Fast path: check cache under lock -- O(1) map lookup + O(1) LRU move.
	c.mu.Lock()
	if entry, ok := c.entries[path]; ok {
		c.lru.MoveToFront(entry.element)
		entry.lastUsed = c.now()
		c.mu.Unlock()
		return entry, nil
	}
	c.mu.Unlock()

	// Slow path: open and index the zip via singleflight (no global lock held).
	val, err, _ := c.group.Do(path, func() (interface{}, error) {
		// Re-check cache inside singleflight (another goroutine may have completed).
		c.mu.Lock()
		if entry, ok := c.entries[path]; ok {
			c.lru.MoveToFront(entry.element)
			entry.lastUsed = c.now()
			c.mu.Unlock()
			return entry, nil
		}
		c.mu.Unlock()

		// Acquire semaphore to limit concurrent zip.OpenReader calls.
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

		// Insert into cache under lock.
		c.mu.Lock()
		// Double-check: another caller may have inserted while we were opening.
		if existing, ok := c.entries[path]; ok {
			c.lru.MoveToFront(existing.element)
			existing.lastUsed = c.now()
			c.mu.Unlock()
			// Close the reader we just opened; the cached one wins.
			_ = reader.Close()
			return existing, nil
		}

		// Evict LRU if at capacity -- O(1).
		if len(c.entries) >= c.maxOpen {
			c.evictLRU()
		}

		entry.element = c.lru.PushFront(path)
		c.entries[path] = entry

		if c.metrics != nil {
			c.metrics.SetZipCacheOpen(len(c.entries))
		}
		c.mu.Unlock()

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

// evictLRU removes the least recently used entry -- O(1).
// Caller must hold c.mu.
func (c *ZipPartCache) evictLRU() {
	elem := c.lru.Back()
	if elem == nil {
		return
	}

	c.lru.Remove(elem)

	oldestPath, _ := elem.Value.(string) //nolint:errcheck // internal invariant: LRU list only contains string path values
	entry, ok := c.entries[oldestPath]
	if !ok {
		return
	}

	// Close resources
	_ = entry.reader.Close()
	delete(c.entries, oldestPath)

	// Update metrics
	if c.metrics != nil {
		c.metrics.IncZipCacheEvictions()
		c.metrics.SetZipCacheOpen(len(c.entries))
	}
}

// Remove removes an entry from the cache and closes its resources -- O(1).
func (c *ZipPartCache) Remove(path string) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[path]
	if !ok {
		return
	}

	// Remove from LRU list -- O(1).
	c.lru.Remove(entry.element)

	// Close resources
	_ = entry.reader.Close()
	delete(c.entries, path)

	// Update metrics
	if c.metrics != nil {
		c.metrics.SetZipCacheOpen(len(c.entries))
	}
}
