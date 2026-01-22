package ctarchiveserve

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
)

// ErrNotFound indicates the requested content does not exist (404).
var ErrNotFound = errors.New("not found")

// ZipReader opens and streams entries from zip parts.
type ZipReader struct {
	integrity *ZipIntegrityCache
	cache     *ZipPartCache // Optional: Phase 5 performance optimization
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

// OpenEntry opens a zip entry by name and returns an io.ReadCloser for streaming.
//
// Errors:
// - ErrNotFound for missing zip parts or missing entries (404)
// - ErrZipTemporarilyUnavailable for zip integrity failures (503)
func (zr *ZipReader) OpenEntry(zipPath, entryName string) (io.ReadCloser, error) {
	if zr == nil {
		return nil, fmt.Errorf("zip reader is nil")
	}

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

	// Try cache first (Phase 5 optimization)
	if zr.cache != nil {
		cacheEntry, err := zr.cache.Get(zipPath)
		if err == nil {
			// Found in cache: use prebuilt index
			entry := cacheEntry.index.Lookup(entryName)
			if entry == nil {
				return nil, fmt.Errorf("%w: zip entry missing", ErrNotFound)
			}

			// Open entry stream (note: we don't close the zip reader here;
			// it's managed by the cache)
			rc, err := entry.Open()
			if err != nil {
				// If entry open fails, remove from cache (may be corrupt)
				zr.cache.Remove(zipPath)
				if zr.integrity != nil {
					zr.integrity.InvalidatePassed(zipPath)
				}
				return nil, fmt.Errorf("%w: %w", ErrZipTemporarilyUnavailable, err)
			}

			// Return a read closer that doesn't close the zip reader
			// (it's cached and will be closed on eviction)
			return &cachedZipEntryReadCloser{entry: rc}, nil
		}
		// Cache miss or error: fall through to on-demand open
	}

	// On-demand open (baseline behavior, also used when cache is disabled)
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
	return z.entry.Read(p)
}

func (z *zipEntryReadCloser) Close() error {
	err1 := z.entry.Close()
	err2 := z.zip.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

// cachedZipEntryReadCloser wraps an entry ReadCloser without closing the cached zip reader.
type cachedZipEntryReadCloser struct {
	entry io.ReadCloser
}

func (c *cachedZipEntryReadCloser) Read(p []byte) (int, error) {
	return c.entry.Read(p)
}

func (c *cachedZipEntryReadCloser) Close() error {
	return c.entry.Close()
	// Note: we don't close the zip reader here; it's managed by ZipPartCache
}

var _ io.ReadCloser = (*zipEntryReadCloser)(nil)
var _ io.ReadCloser = (*cachedZipEntryReadCloser)(nil)
