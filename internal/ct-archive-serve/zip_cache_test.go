package ctarchiveserve

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestZipIntegrityCache_FailedTTLAndRetest(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return now }

	verifyCalls := 0
	verifyErr := errors.New("bad zip")
	verify := func(string) error {
		verifyCalls++
		return verifyErr
	}

	z := NewZipIntegrityCache(5*time.Minute, nowFn, verify, nil)

	path := "/tmp/000.zip"

	// First check: verify called and fails.
	if err := z.Check(path); !errors.Is(err, ErrZipTemporarilyUnavailable) {
		t.Fatalf("Check() error = %v, want ErrZipTemporarilyUnavailable", err)
	}
	if got, want := verifyCalls, 1; got != want {
		t.Fatalf("verifyCalls = %d, want %d", got, want)
	}

	// Second check before expiry: verify not called again.
	if err := z.Check(path); !errors.Is(err, ErrZipTemporarilyUnavailable) {
		t.Fatalf("Check() error = %v, want ErrZipTemporarilyUnavailable", err)
	}
	if got, want := verifyCalls, 1; got != want {
		t.Fatalf("verifyCalls = %d, want %d", got, want)
	}

	// Advance past TTL and make verify succeed.
	now = now.Add(6 * time.Minute)
	verifyErr = nil

	if err := z.Check(path); err != nil {
		t.Fatalf("Check() error = %v, want nil", err)
	}
	if got, want := verifyCalls, 2; got != want {
		t.Fatalf("verifyCalls = %d, want %d", got, want)
	}
}

func TestZipIntegrityCache_PassedCachePersistsUntilInvalidated(t *testing.T) {
	t.Parallel()

	nowFn := func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

	verifyCalls := 0
	verify := func(string) error {
		verifyCalls++
		return nil
	}

	z := NewZipIntegrityCache(5*time.Minute, nowFn, verify, nil)
	path := "/tmp/000.zip"

	if err := z.Check(path); err != nil {
		t.Fatalf("Check() error = %v, want nil", err)
	}
	if err := z.Check(path); err != nil {
		t.Fatalf("Check() error = %v, want nil", err)
	}
	if got, want := verifyCalls, 1; got != want {
		t.Fatalf("verifyCalls = %d, want %d", got, want)
	}

	z.InvalidatePassed(path)
	if err := z.Check(path); err != nil {
		t.Fatalf("Check() error = %v, want nil", err)
	}
	if got, want := verifyCalls, 2; got != want {
		t.Fatalf("verifyCalls = %d, want %d", got, want)
	}
}

func TestZipPartCache_GetAndCache(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	zipPath := filepath.Join(root, "000.zip")
	mustCreateZip(t, zipPath, map[string][]byte{
		"test.txt": []byte("test content"),
	})

	cache := NewZipPartCache(10, nil, 0)

	// First get: cache miss, should open and cache
	entry1, err := cache.Get(zipPath)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if entry1 == nil {
		t.Fatalf("Get() returned nil entry")
	}
	if entry1.index.Lookup("test.txt") == nil {
		t.Errorf("index.Lookup(\"test.txt\") = nil, want non-nil")
	}

	// Second get: cache hit
	entry2, err := cache.Get(zipPath)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if entry2 != entry1 {
		t.Errorf("Get() returned different entry on cache hit")
	}
}

func TestZipPartCache_LRUEviction(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// With 64 shards and maxOpen=64, each shard holds 1 entry (perShard=1).
	// Inserting more than 64 distinct paths guarantees at least some shards
	// receive 2+ entries (pigeonhole principle), triggering per-shard eviction.
	cache := NewZipPartCache(64, nil, 0)

	const numFiles = 80
	for i := 0; i < numFiles; i++ {
		p := filepath.Join(root, fmt.Sprintf("%03d.zip", i))
		mustCreateZip(t, p, map[string][]byte{
			fmt.Sprintf("file%d", i): []byte(fmt.Sprintf("data%d", i)),
		})
		_, err := cache.Get(p)
		if err != nil {
			t.Fatalf("Get(%q) error = %v", p, err)
		}
	}

	// Per-shard capacity is 1, so each shard can hold at most 1 entry.
	// Total capacity across all 64 shards is 64.
	total := cache.totalOpen()
	if total > 64 {
		t.Errorf("totalOpen() = %d, want <= 64 (eviction should have occurred)", total)
	}
	// With 80 entries across 64 shards (pigeonhole), at least 16 evictions
	// must have occurred, so total must be strictly less than 80.
	if total >= numFiles {
		t.Errorf("totalOpen() = %d, want < %d (some eviction expected)", total, numFiles)
	}
}

func TestZipPartCache_ShardedEviction(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// With maxOpen=64 and 64 shards, each shard holds 1 entry.
	cache := NewZipPartCache(64, nil, 0)

	// Create 65 zip files to exceed total capacity.
	zipPaths := make([]string, 65)
	for i := 0; i < 65; i++ {
		p := filepath.Join(root, fmt.Sprintf("%03d.zip", i))
		mustCreateZip(t, p, map[string][]byte{
			fmt.Sprintf("file%d", i): []byte(fmt.Sprintf("data%d", i)),
		})
		zipPaths[i] = p
	}

	// Insert all 65 entries.
	for _, p := range zipPaths {
		_, err := cache.Get(p)
		if err != nil {
			t.Fatalf("Get(%q) error = %v", p, err)
		}
	}

	// Verify total is at most 64 (at least one eviction occurred).
	total := cache.totalOpen()
	if total > 64 {
		t.Errorf("totalOpen() = %d, want <= 64 (eviction should have occurred)", total)
	}
}

func TestZipPartCache_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cache := NewZipPartCache(100, nil, 0)

	// Create multiple zip files
	zipFiles := make([]string, 10)
	for i := 0; i < 10; i++ {
		zipPath := filepath.Join(root, fmt.Sprintf("%03d.zip", i))
		mustCreateZip(t, zipPath, map[string][]byte{
			fmt.Sprintf("file%d", i): []byte(fmt.Sprintf("data%d", i)),
		})
		zipFiles[i] = zipPath
	}

	// Concurrent access from multiple goroutines
	var wg sync.WaitGroup
	const numGoroutines = 20
	const iterationsPerGoroutine = 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterationsPerGoroutine; j++ {
				zipPath := zipFiles[id%len(zipFiles)]
				entry, err := cache.Get(zipPath)
				if err != nil {
					t.Errorf("goroutine %d iteration %d: Get() error = %v", id, j, err)
					return
				}
				if entry == nil {
					t.Errorf("goroutine %d iteration %d: Get() returned nil", id, j)
					return
				}
				// Verify index lookup works
				expectedFile := fmt.Sprintf("file%d", id%len(zipFiles))
				if entry.index.Lookup(expectedFile) == nil {
					t.Errorf("goroutine %d: index.Lookup(%q) = nil", id, expectedFile)
				}
			}
		}(i)
	}

	wg.Wait()
}

func TestZipPartCache_SingleflightDeduplication(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	zipPath := filepath.Join(root, "dedup.zip")
	mustCreateZip(t, zipPath, map[string][]byte{
		"entry.txt": []byte("singleflight test"),
	})

	cache := NewZipPartCache(100, nil, 0)

	// Launch N goroutines that all hit the same uncached path simultaneously.
	const numGoroutines = 50
	var wg sync.WaitGroup
	gate := make(chan struct{}) // start gate to maximise concurrency

	entries := make([]*ZipPartCacheEntry, numGoroutines)
	errs := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-gate // wait for all goroutines to be ready
			entries[idx], errs[idx] = cache.Get(zipPath)
		}(i)
	}

	// Release all goroutines at once.
	close(gate)
	wg.Wait()

	// All should succeed.
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: Get() error = %v", i, err)
		}
	}

	// All should return the same cached entry instance (singleflight deduplication).
	first := entries[0]
	for i := 1; i < numGoroutines; i++ {
		if entries[i] != first {
			t.Errorf("goroutine %d returned different entry pointer than goroutine 0 (singleflight did not deduplicate)", i)
		}
	}

	// Cache should contain exactly one entry (the path resides in one shard).
	total := cache.totalOpen()
	if total != 1 {
		t.Errorf("totalOpen() = %d, want 1", total)
	}
}

func TestZipPartCache_ConcurrentMultiLogStress(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cache := NewZipPartCache(256, nil, 0)

	// Simulate 45 logs each with 3 zip parts (135 unique paths).
	const numLogs = 45
	const partsPerLog = 3
	zipPaths := make([][]string, numLogs)

	for i := 0; i < numLogs; i++ {
		zipPaths[i] = make([]string, partsPerLog)
		for j := 0; j < partsPerLog; j++ {
			p := filepath.Join(root, fmt.Sprintf("log%02d_%03d.zip", i, j))
			mustCreateZip(t, p, map[string][]byte{
				fmt.Sprintf("entry_%d_%d", i, j): []byte("data"),
			})
			zipPaths[i][j] = p
		}
	}

	// Launch 45 goroutines (one per log), each accessing its 3 parts in a loop.
	var wg sync.WaitGroup
	gate := make(chan struct{})
	const iterations = 20

	for i := 0; i < numLogs; i++ {
		wg.Add(1)
		go func(logIdx int) {
			defer wg.Done()
			<-gate
			for iter := 0; iter < iterations; iter++ {
				for _, p := range zipPaths[logIdx] {
					entry, err := cache.Get(p)
					if err != nil {
						t.Errorf("log %d: Get(%q) error = %v", logIdx, p, err)
						return
					}
					if entry == nil {
						t.Errorf("log %d: Get(%q) returned nil", logIdx, p)
						return
					}
				}
			}
		}(i)
	}

	close(gate)
	wg.Wait()

	// Verify cache is not empty and within bounds.
	total := cache.totalOpen()
	if total == 0 {
		t.Error("totalOpen() = 0 after stress test, expected non-zero")
	}
	if total > 256 {
		t.Errorf("totalOpen() = %d, exceeds maxOpen=256", total)
	}
}

func TestZipIntegrityCache_ThunderingHerd(t *testing.T) {
	t.Parallel()

	var verifyCalls atomic.Int64
	slowVerify := func(string) error {
		verifyCalls.Add(1)
		// Simulate expensive verification.
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	nowFn := func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	z := NewZipIntegrityCache(5*time.Minute, nowFn, slowVerify, nil)

	const numGoroutines = 50
	var wg sync.WaitGroup
	gate := make(chan struct{})
	errs := make([]error, numGoroutines)
	path := "/tmp/thundering.zip"

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-gate
			errs[idx] = z.Check(path)
		}(i)
	}

	close(gate)
	wg.Wait()

	// All should succeed.
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: Check() error = %v", i, err)
		}
	}

	// The slow verify function should be called exactly once (singleflight deduplication).
	if got := verifyCalls.Load(); got != 1 {
		t.Errorf("verify called %d times, want 1 (singleflight should deduplicate)", got)
	}

	// The path should be in the passed cache.
	z.mu.Lock()
	_, ok := z.passed[path]
	z.mu.Unlock()
	if !ok {
		t.Error("path should be in passed cache after successful verification")
	}
}
