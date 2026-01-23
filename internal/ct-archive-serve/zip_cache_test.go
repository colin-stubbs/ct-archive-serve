package ctarchiveserve

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
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

	cache := NewZipPartCache(10, nil)

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
	cache := NewZipPartCache(2, nil) // Small cache for testing

	// Create 3 zip files
	zip1 := filepath.Join(root, "000.zip")
	zip2 := filepath.Join(root, "001.zip")
	zip3 := filepath.Join(root, "002.zip")
	mustCreateZip(t, zip1, map[string][]byte{"file1": []byte("data1")})
	mustCreateZip(t, zip2, map[string][]byte{"file2": []byte("data2")})
	mustCreateZip(t, zip3, map[string][]byte{"file3": []byte("data3")})

	// Add zip1 and zip2 (cache at capacity)
	_, err := cache.Get(zip1)
	if err != nil {
		t.Fatalf("Get(zip1) error = %v", err)
	}
	_, err = cache.Get(zip2)
	if err != nil {
		t.Fatalf("Get(zip2) error = %v", err)
	}

	// Add zip3: should evict zip1 (LRU)
	_, err = cache.Get(zip3)
	if err != nil {
		t.Fatalf("Get(zip3) error = %v", err)
	}

	// zip1 should be evicted (cache miss)
	cache.mu.Lock()
	_, ok := cache.entries[zip1]
	cache.mu.Unlock()
	if ok {
		t.Errorf("zip1 should have been evicted, but it's still in cache")
	}

	// zip2 and zip3 should still be cached
	cache.mu.Lock()
	_, ok2 := cache.entries[zip2]
	_, ok3 := cache.entries[zip3]
	cache.mu.Unlock()
	if !ok2 || !ok3 {
		t.Errorf("zip2 or zip3 should still be cached, but zip2=%v zip3=%v", ok2, ok3)
	}
}

func TestZipPartCache_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cache := NewZipPartCache(100, nil)

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
