package ctarchiveserve

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// BenchmarkZipReader_OpenEntry benchmarks zip entry opening performance.
//
// To profile this benchmark:
//   go test -bench=BenchmarkZipReader_OpenEntry -cpuprofile=cpu.prof -memprofile=mem.prof
//   go tool pprof cpu.prof
//   go tool pprof mem.prof
//
// To check for mutex contention:
//   go test -bench=BenchmarkZipReader_OpenEntry -mutexprofile=mutex.prof
//   go tool pprof mutex.prof
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
	cache := NewZipPartCache(100, nil)

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

// createZipForBench is a helper to create zip files for benchmarks.
func createZipForBench(b *testing.B, path string, files map[string][]byte) {
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
