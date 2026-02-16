package ctarchiveserve

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
)

func TestEntryContentCache_GetPut(t *testing.T) {
	t.Parallel()

	cache := NewEntryContentCache(1024*1024, nil) // 1 MiB

	data := []byte("hello world")
	cache.Put("/archive/000.zip", "entry.txt", data)

	got, ok := cache.Get("/archive/000.zip", "entry.txt")
	if !ok {
		t.Fatal("Get() returned miss, want hit")
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("Get() data = %q, want %q", got, data)
	}

	// Miss for unknown entry.
	_, ok = cache.Get("/archive/000.zip", "nonexistent.txt")
	if ok {
		t.Error("Get() returned hit for nonexistent entry, want miss")
	}
}

func TestEntryContentCache_Disabled(t *testing.T) {
	t.Parallel()

	cache := NewEntryContentCache(0, nil)
	cache.Put("/archive/000.zip", "entry.txt", []byte("data"))

	_, ok := cache.Get("/archive/000.zip", "entry.txt")
	if ok {
		t.Error("Get() hit on disabled cache (maxBytes=0), want miss")
	}
}

func TestEntryContentCache_PerShardBudget(t *testing.T) {
	t.Parallel()

	// 64 shards * 100 bytes per shard = 6400 bytes total budget.
	cache := NewEntryContentCache(6400, nil)

	// Insert an entry that fits in one shard (< 100 bytes).
	cache.Put("/archive/000.zip", "small.txt", make([]byte, 50))
	_, ok := cache.Get("/archive/000.zip", "small.txt")
	if !ok {
		t.Error("Get() miss for small entry, want hit")
	}

	// An entry larger than a single shard budget should be rejected.
	// Per-shard budget = 6400/64 = 100 bytes.
	cache.Put("/archive/001.zip", "big.txt", make([]byte, 101))
	_, ok = cache.Get("/archive/001.zip", "big.txt")
	if ok {
		t.Error("Get() hit for oversized entry, want miss (exceeds per-shard budget)")
	}
}

func TestEntryContentCache_Eviction(t *testing.T) {
	t.Parallel()

	// Create cache with very small per-shard budget.
	// 64 shards * 200 bytes = 12800 bytes total.
	cache := NewEntryContentCache(12800, nil)

	// Put many entries to force eviction within shards.
	// Each entry is 150 bytes. Per-shard budget is 200 bytes, so at most 1 entry per shard.
	for i := 0; i < 100; i++ {
		z := fmt.Sprintf("/archive/%03d.zip", i)
		cache.Put(z, "entry.txt", make([]byte, 150))
	}

	// Verify cache is bounded: total bytes should not exceed the budget.
	totalBytes, totalItems := cache.totals()
	if totalBytes > 12800 {
		t.Errorf("totalBytes = %d, exceeds budget 12800", totalBytes)
	}
	if totalItems > 64 {
		t.Errorf("totalItems = %d, exceeds max possible 64 (one per shard)", totalItems)
	}
}

func TestEntryContentCache_Invalidate(t *testing.T) {
	t.Parallel()

	cache := NewEntryContentCache(1024*1024, nil)

	// Insert multiple entries for the same zip path.
	cache.Put("/archive/000.zip", "entry1.txt", []byte("data1"))
	cache.Put("/archive/000.zip", "entry2.txt", []byte("data2"))
	cache.Put("/archive/001.zip", "entry1.txt", []byte("other"))

	// Invalidate all entries for 000.zip.
	cache.Invalidate("/archive/000.zip")

	_, ok1 := cache.Get("/archive/000.zip", "entry1.txt")
	_, ok2 := cache.Get("/archive/000.zip", "entry2.txt")
	if ok1 || ok2 {
		t.Error("Get() hit after Invalidate(), want miss for all entries of invalidated zip")
	}

	// 001.zip entries should be unaffected.
	_, ok := cache.Get("/archive/001.zip", "entry1.txt")
	if !ok {
		t.Error("Get() miss for unrelated zip after Invalidate(), want hit")
	}
}

func TestEntryContentCache_UpdateInPlace(t *testing.T) {
	t.Parallel()

	cache := NewEntryContentCache(1024*1024, nil)

	cache.Put("/archive/000.zip", "entry.txt", []byte("version1"))
	cache.Put("/archive/000.zip", "entry.txt", []byte("version2"))

	got, ok := cache.Get("/archive/000.zip", "entry.txt")
	if !ok {
		t.Fatal("Get() miss after update, want hit")
	}
	if string(got) != "version2" {
		t.Fatalf("Get() data = %q, want %q", got, "version2")
	}
}

func TestEntryContentCache_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	cache := NewEntryContentCache(64*1024*1024, nil) // 64 MiB

	const numGoroutines = 50
	const iterationsPerGoroutine = 100

	// Pre-populate with entries.
	for i := 0; i < numGoroutines; i++ {
		z := fmt.Sprintf("/archive/%03d.zip", i)
		cache.Put(z, "entry.txt", []byte(fmt.Sprintf("data_%d", i)))
	}

	var wg sync.WaitGroup
	gate := make(chan struct{})

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-gate
			z := fmt.Sprintf("/archive/%03d.zip", id)
			for j := 0; j < iterationsPerGoroutine; j++ {
				// Mix of reads and writes.
				if j%10 == 0 {
					cache.Put(z, "entry.txt", []byte(fmt.Sprintf("data_%d_%d", id, j)))
				}
				data, ok := cache.Get(z, "entry.txt")
				if !ok {
					t.Errorf("goroutine %d: Get() miss on iteration %d", id, j)
					return
				}
				if len(data) == 0 {
					t.Errorf("goroutine %d: Get() returned empty data", id)
					return
				}
			}
		}(i)
	}

	close(gate)
	wg.Wait()
}

func TestEntryContentCache_NilReceiver(t *testing.T) {
	t.Parallel()

	var cache *EntryContentCache

	// All methods should be safe on nil receiver.
	_, ok := cache.Get("/archive/000.zip", "entry.txt")
	if ok {
		t.Error("Get() hit on nil cache, want miss")
	}

	// Put should not panic.
	cache.Put("/archive/000.zip", "entry.txt", []byte("data"))

	// Invalidate should not panic.
	cache.Invalidate("/archive/000.zip")
}
