package ctarchiveserve

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
)

// ErrNotFound indicates the requested content does not exist (404).
var ErrNotFound = errors.New("not found")

// ZipReader opens and streams entries from zip parts.
type ZipReader struct {
	integrity  *ZipIntegrityCache
	cache      *ZipPartCache        // Optional: zip part handle cache
	entryCache *EntryContentCache   // Optional: decompressed entry content cache
}

// NewZipReader constructs a ZipReader that uses the provided integrity cache.
func NewZipReader(integrity *ZipIntegrityCache) *ZipReader {
	return &ZipReader{
		integrity: integrity,
		cache:     nil, // Cache is optional (Phase 5 optimization)
	}
}

// SetZipPartCache sets the optional zip part cache for performance optimization.
func (zr *ZipReader) SetZipPartCache(cache *ZipPartCache) {
	zr.cache = cache
}

// SetEntryContentCache sets the optional decompressed entry content cache.
func (zr *ZipReader) SetEntryContentCache(cache *EntryContentCache) {
	zr.entryCache = cache
}

// OpenEntry opens a zip entry by name and returns an io.ReadCloser for streaming.
//
// The lookup order is optimized to minimize syscalls and I/O on the hot path:
//  1. Entry content cache (zero I/O: returns cached []byte via bytes.NewReader)
//  2. Zip part cache (no stat/integrity: entry already validated when cached)
//  3. Slow path: os.Stat -> integrity check -> populate cache -> open entry
//  4. Fallback: on-demand zip.OpenReader (when cache is nil or cache.Get failed)
//
// Errors:
// - ErrNotFound for missing zip parts or missing entries (404)
// - ErrZipTemporarilyUnavailable for zip integrity failures (503)
func (zr *ZipReader) OpenEntry(zipPath, entryName string) (io.ReadCloser, error) {
	if zr == nil {
		return nil, errors.New("zip reader is nil")
	}

	// Fast path: try entry content cache first (zero I/O, zero decompression).
	if zr.entryCache != nil {
		data, ok := zr.entryCache.Get(zipPath, entryName)
		if ok {
			return io.NopCloser(bytes.NewReader(data)), nil
		}
	}

	// Fast path: try zip part cache (skip stat + integrity for cached entries).
	if zr.cache != nil {
		cacheEntry, err := zr.cache.Get(zipPath)
		if err == nil {
			return zr.openFromCacheEntry(cacheEntry, zipPath, entryName)
		}
		// Cache miss: fall through to full validation path.
	}

	// Slow path: stat -> integrity -> open -> populate cache.
	if _, err := os.Stat(zipPath); err != nil {
		if zr.integrity != nil {
			zr.integrity.InvalidatePassed(zipPath)
		}
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: zip part missing", ErrNotFound)
		}
		return nil, fmt.Errorf("%w: %w", ErrZipTemporarilyUnavailable, err)
	}

	if zr.integrity != nil {
		if err := zr.integrity.Check(zipPath); err != nil {
			return nil, err
		}
	}

	// After validation, try to populate the cache instead of doing a
	// redundant on-demand open.
	if zr.cache != nil {
		cacheEntry, err := zr.cache.Get(zipPath)
		if err == nil {
			return zr.openFromCacheEntry(cacheEntry, zipPath, entryName)
		}
	}

	// Fallback: on-demand open (when cache is nil or cache.Get failed).
	return zr.openOnDemand(zipPath, entryName)
}

// openFromCacheEntry opens an entry from a cached zip part, optionally populating
// the entry content cache with the decompressed bytes.
func (zr *ZipReader) openFromCacheEntry(cacheEntry *ZipPartCacheEntry, zipPath, entryName string) (io.ReadCloser, error) {
	entry := cacheEntry.index.Lookup(entryName)
	if entry == nil {
		return nil, fmt.Errorf("%w: zip entry missing", ErrNotFound)
	}

	rc, err := entry.Open()
	if err != nil {
		zr.cache.Remove(zipPath)
		if zr.integrity != nil {
			zr.integrity.InvalidatePassed(zipPath)
		}
		return nil, fmt.Errorf("%w: %w", ErrZipTemporarilyUnavailable, err)
	}

	// If entry content cache is available, read fully, cache, and return from cache.
	if zr.entryCache != nil {
		data, readErr := io.ReadAll(rc)
		_ = rc.Close()
		if readErr != nil {
			zr.cache.Remove(zipPath)
			if zr.integrity != nil {
				zr.integrity.InvalidatePassed(zipPath)
			}
			return nil, fmt.Errorf("%w: %w", ErrZipTemporarilyUnavailable, readErr)
		}
		zr.entryCache.Put(zipPath, entryName, data)
		return io.NopCloser(bytes.NewReader(data)), nil
	}

	return &cachedZipEntryReadCloser{entry: rc}, nil
}

// openOnDemand opens a zip entry without using the cache (baseline behavior).
func (zr *ZipReader) openOnDemand(zipPath, entryName string) (io.ReadCloser, error) {
	//nolint:gosec // G304: path is validated internally from archive index, not user input
	zrdr, err := zip.OpenReader(zipPath)
	if err != nil {
		if zr.integrity != nil {
			zr.integrity.InvalidatePassed(zipPath)
		}
		return nil, fmt.Errorf("%w: %w", ErrZipTemporarilyUnavailable, err)
	}

	for _, f := range zrdr.File {
		if f.Name != entryName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			_ = zrdr.Close()
			if zr.integrity != nil {
				zr.integrity.InvalidatePassed(zipPath)
			}
			return nil, fmt.Errorf("%w: %w", ErrZipTemporarilyUnavailable, err)
		}
		return &zipEntryReadCloser{entry: rc, zip: zrdr}, nil
	}

	_ = zrdr.Close()
	return nil, fmt.Errorf("%w: zip entry missing", ErrNotFound)
}

type zipEntryReadCloser struct {
	entry io.ReadCloser
	zip   *zip.ReadCloser
}

func (z *zipEntryReadCloser) Read(p []byte) (int, error) {
	//nolint:wrapcheck // io.Reader.Read is a low-level interface method, pass-through
	return z.entry.Read(p)
}

func (z *zipEntryReadCloser) Close() error {
	err1 := z.entry.Close()
	err2 := z.zip.Close()
	if err1 != nil {
		return fmt.Errorf("close zip entry: %w", err1)
	}
	if err2 != nil {
		return fmt.Errorf("close zip file: %w", err2)
	}
	return nil
}

// cachedZipEntryReadCloser wraps an entry ReadCloser without closing the cached zip reader.
type cachedZipEntryReadCloser struct {
	entry io.ReadCloser
}

func (c *cachedZipEntryReadCloser) Read(p []byte) (int, error) {
	//nolint:wrapcheck // io.Reader.Read is a low-level interface method, pass-through
	return c.entry.Read(p)
}

func (c *cachedZipEntryReadCloser) Close() error {
	//nolint:wrapcheck // io.Closer.Close is a low-level interface method, pass-through
	return c.entry.Close()
	// Note: we don't close the zip reader here; it's managed by ZipPartCache
}

var _ io.ReadCloser = (*zipEntryReadCloser)(nil)
var _ io.ReadCloser = (*cachedZipEntryReadCloser)(nil)
