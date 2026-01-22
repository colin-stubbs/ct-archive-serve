package ctarchiveserve

import (
	"archive/zip"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
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

	mu     sync.Mutex
	passed map[string]struct{}
	failed map[string]time.Time // path -> expiresAt
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

	// Fast path: cached pass.
	z.mu.Lock()
	if _, ok := z.passed[path]; ok {
		z.mu.Unlock()
		return nil
	}

	// Cached failure (unexpired).
	if exp, ok := z.failed[path]; ok {
		if z.now().Before(exp) {
			z.mu.Unlock()
			return ErrZipTemporarilyUnavailable
		}
		delete(z.failed, path)
	}
	z.mu.Unlock()

	// Slow path: verify.
	if err := z.verify(path); err != nil {
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

func verifyZipStructural(path string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open entry %q: %w", f.Name, err)
		}
		_ = rc.Close()
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

// ZipPartCacheEntry represents a cached zip part with its open file handle and entry index.
type ZipPartCacheEntry struct {
	path      string
	file      *os.File
	reader    *zip.ReadCloser
	index     *ZipEntryIndex
	lastUsed  time.Time
}

// ZipPartCache is a bounded LRU cache for open zip file handles and entry indices.
//
// This cache avoids repeated central-directory parsing for hot zip parts.
// When the cache exceeds maxOpen, LRU entries are evicted.
type ZipPartCache struct {
	maxOpen  int
	metrics  *Metrics
	now      func() time.Time

	mu      sync.Mutex
	entries map[string]*ZipPartCacheEntry // path -> entry
	order   []string                       // LRU order (oldest first)
}

// NewZipPartCache constructs a new ZipPartCache.
func NewZipPartCache(maxOpen int, metrics *Metrics) *ZipPartCache {
	if maxOpen <= 0 {
		maxOpen = 256 // Default
	}
	return &ZipPartCache{
		maxOpen: maxOpen,
		metrics: metrics,
		now:     time.Now,
		entries: make(map[string]*ZipPartCacheEntry),
		order:   make([]string, 0, maxOpen),
	}
}

// Get returns a cached zip part entry, or opens and caches it if not present.
// Returns the entry and nil error if found/cached, or nil and error on failure.
func (c *ZipPartCache) Get(path string) (*ZipPartCacheEntry, error) {
	if c == nil {
		return nil, errors.New("zip part cache not initialized")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check cache
	if entry, ok := c.entries[path]; ok {
		// Update LRU order (move to end)
		c.updateLRUOrder(path)
		entry.lastUsed = c.now()
		return entry, nil
	}

	// Cache miss: open and index the zip
	//nolint:gosec // G304: path is validated internally from archive index, not user input
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open zip file: %w", err)
	}

	reader, err := zip.OpenReader(path)
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("open zip reader: %w", err)
	}

	// Build entry index
	index := &ZipEntryIndex{
		entries: make(map[string]*zip.File, len(reader.File)),
	}
	for _, f := range reader.File {
		index.entries[f.Name] = f
	}

	entry := &ZipPartCacheEntry{
		path:     path,
		file:     file,
			reader:   reader,
		index:    index,
		lastUsed: c.now(),
	}

	// Evict LRU if at capacity
	if len(c.entries) >= c.maxOpen {
		c.evictLRU()
	}

	// Add to cache
	c.entries[path] = entry
	c.order = append(c.order, path)

	// Update metrics
	if c.metrics != nil {
		c.metrics.SetZipCacheOpen(len(c.entries))
	}

	return entry, nil
}

// updateLRUOrder moves path to the end of the LRU order (most recently used).
func (c *ZipPartCache) updateLRUOrder(path string) {
	for i, p := range c.order {
		if p == path {
			// Move to end
			c.order = append(c.order[:i], c.order[i+1:]...)
			c.order = append(c.order, path)
			break
		}
	}
}

// evictLRU removes the least recently used entry.
func (c *ZipPartCache) evictLRU() {
	if len(c.order) == 0 {
		return
	}

	// Remove oldest (first in order)
	oldestPath := c.order[0]
	c.order = c.order[1:]

	entry, ok := c.entries[oldestPath]
	if !ok {
		return
	}

	// Close resources
	_ = entry.reader.Close()
	_ = entry.file.Close()

	delete(c.entries, oldestPath)

	// Update metrics
	if c.metrics != nil {
		c.metrics.IncZipCacheEvictions()
		c.metrics.SetZipCacheOpen(len(c.entries))
	}
}

// Remove removes an entry from the cache and closes its resources.
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

	// Remove from order
	for i, p := range c.order {
		if p == path {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}

	// Close resources
	_ = entry.reader.Close()
	_ = entry.file.Close()

	delete(c.entries, path)

	// Update metrics
	if c.metrics != nil {
		c.metrics.SetZipCacheOpen(len(c.entries))
	}
}