package ctarchiveserve

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// BenchmarkZipReader_OpenEntry benchmarks zip entry opening performance.
//
// To profile this benchmark:
//
//	go test -bench=BenchmarkZipReader_OpenEntry -cpuprofile=cpu.prof -memprofile=mem.prof
//	go tool pprof cpu.prof
//	go tool pprof mem.prof
//
// To check for mutex contention:
//
//	go test -bench=BenchmarkZipReader_OpenEntry -mutexprofile=mutex.prof
//	go tool pprof mutex.prof
func BenchmarkZipReader_OpenEntry(b *testing.B) {
	root := b.TempDir()
	zipPath := filepath.Join(root, "000.zip")
	createZipForBench(b, zipPath, map[string][]byte{
		"test.txt": make([]byte, 1024*1024), // 1MB entry
	})

	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, nil)
	zr := NewZipReader(zic)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rc, err := zr.OpenEntry(zipPath, "test.txt")
		if err != nil {
			b.Fatalf("OpenEntry() error = %v", err)
		}
		_ = rc.Close()
	}
}

// BenchmarkZipPartCache_Get benchmarks zip part cache performance.
//
// This benchmark simulates a large working set (many distinct zip parts)
// to verify that cache eviction and lookup remain efficient.
func BenchmarkZipPartCache_Get(b *testing.B) {
	root := b.TempDir()
	cache := NewZipPartCache(100, nil, 0)

	// Create many zip files to simulate large working set
	zipFiles := make([]string, 200)
	for i := 0; i < 200; i++ {
		zipPath := filepath.Join(root, fmt.Sprintf("%03d.zip", i))
		createZipForBench(b, zipPath, map[string][]byte{
			"entry.txt": []byte("test data"),
		})
		zipFiles[i] = zipPath
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		zipPath := zipFiles[i%len(zipFiles)]
		entry, err := cache.Get(zipPath)
		if err != nil {
			b.Fatalf("Get() error = %v", err)
		}
		_ = entry
	}
}

// BenchmarkZipPartCache_ConcurrentContention benchmarks cache performance under
// concurrent access from many goroutines (simulating 45+ simultaneous log downloads).
func BenchmarkZipPartCache_ConcurrentContention(b *testing.B) {
	root := b.TempDir()
	cache := NewZipPartCache(2048, nil, 0)

	// Create zip files simulating 45 logs with 3 parts each.
	const numLogs = 45
	const partsPerLog = 3
	zipPaths := make([]string, numLogs*partsPerLog)
	for i := 0; i < numLogs; i++ {
		for j := 0; j < partsPerLog; j++ {
			idx := i*partsPerLog + j
			p := filepath.Join(root, fmt.Sprintf("log%02d_%03d.zip", i, j))
			createZipForBench(b, p, map[string][]byte{
				"entry.txt": []byte("test data"),
			})
			zipPaths[idx] = p
		}
	}

	// Warm cache.
	for _, p := range zipPaths {
		if _, err := cache.Get(p); err != nil {
			b.Fatalf("warm Get(%q) error = %v", p, err)
		}
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			p := zipPaths[i%len(zipPaths)]
			entry, err := cache.Get(p)
			if err != nil {
				b.Fatalf("Get() error = %v", err)
			}
			_ = entry
			i++
		}
	})
}

// BenchmarkEntryContentCache_ConcurrentContention benchmarks entry content cache
// performance under concurrent access.
func BenchmarkEntryContentCache_ConcurrentContention(b *testing.B) {
	cache := NewEntryContentCache(256*1024*1024, nil) // 256 MiB

	// Pre-populate with entries simulating 45 logs.
	const numEntries = 500
	type kv struct{ zip, entry string }
	keys := make([]kv, numEntries)
	for i := 0; i < numEntries; i++ {
		k := kv{
			zip:   fmt.Sprintf("/archive/log%02d/%03d.zip", i%45, i/45),
			entry: "entry.txt",
		}
		keys[i] = k
		cache.Put(k.zip, k.entry, make([]byte, 4096))
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			k := keys[i%len(keys)]
			data, ok := cache.Get(k.zip, k.entry)
			if !ok {
				b.Fatalf("Get() miss for %s/%s", k.zip, k.entry)
			}
			_ = data
			i++
		}
	})
}

// createZipForBench is a helper to create zip files for benchmarks.
func createZipForBench(b *testing.B, path string, files map[string][]byte) {
	b.Helper()

	//nolint:gosec // G304: path is validated and comes from benchmark helpers, not user input
	f, err := os.Create(path)
	if err != nil {
		b.Fatalf("Create(%q) error = %v", path, err)
	}
	defer func() { _ = f.Close() }()

	w := zip.NewWriter(f)
	for name, contents := range files {
		fw, err := w.Create(name)
		if err != nil {
			b.Fatalf("zip.Create(%q) error = %v", name, err)
		}
		if _, err := fw.Write(contents); err != nil {
			b.Fatalf("zip write %q error = %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		b.Fatalf("zip.Close() error = %v", err)
	}
}

// BenchmarkZipPartCache_GetParallel_GOMAXPROCS benchmarks sharded cache scaling
// across increasing goroutine counts to verify contention reduction.
func BenchmarkZipPartCache_GetParallel_GOMAXPROCS(b *testing.B) {
	root := b.TempDir()
	cache := NewZipPartCache(2048, nil, 0)

	const numFiles = 100
	zipPaths := make([]string, numFiles)
	for i := 0; i < numFiles; i++ {
		p := filepath.Join(root, fmt.Sprintf("%03d.zip", i))
		createZipForBench(b, p, map[string][]byte{
			"entry.txt": []byte("data"),
		})
		zipPaths[i] = p
	}

	// Warm cache.
	for _, p := range zipPaths {
		if _, err := cache.Get(p); err != nil {
			b.Fatalf("warm Get(%q) error = %v", p, err)
		}
	}

	for _, numG := range []int{1, 4, 16, 64} {
		b.Run(fmt.Sprintf("goroutines=%d", numG), func(b *testing.B) {
			var wg sync.WaitGroup
			perG := b.N / numG
			if perG < 1 {
				perG = 1
			}
			b.ResetTimer()
			for g := 0; g < numG; g++ {
				wg.Add(1)
				go func(gid int) {
					defer wg.Done()
					for i := 0; i < perG; i++ {
						p := zipPaths[(gid*perG+i)%numFiles]
						entry, err := cache.Get(p)
						if err != nil {
							b.Errorf("Get() error = %v", err)
							return
						}
						_ = entry
					}
				}(g)
			}
			wg.Wait()
		})
	}
}
