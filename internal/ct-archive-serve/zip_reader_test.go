package ctarchiveserve

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestZipReader_OpenEntry_OK(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	zipPath := filepath.Join(root, "000.zip")
	mustCreateZip(t, zipPath, map[string][]byte{
		"checkpoint": []byte("hello"),
		"tile/0/001": []byte{0x01, 0x02, 0x03},
	})

	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, nil)
	zr := NewZipReader(zic)

	rc, err := zr.OpenEntry(zipPath, "checkpoint")
	if err != nil {
		t.Fatalf("OpenEntry() error = %v", err)
	}
	defer func() { _ = rc.Close() }()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !bytes.Equal(got, []byte("hello")) {
		t.Fatalf("entry bytes = %q, want %q", got, "hello")
	}
}

func TestZipReader_OpenEntry_NotFound(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	zipPath := filepath.Join(root, "000.zip")
	mustCreateZip(t, zipPath, map[string][]byte{
		"checkpoint": []byte("hello"),
	})

	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, nil)
	zr := NewZipReader(zic)

	_, err := zr.OpenEntry(zipPath, "nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("OpenEntry() error = %v, want ErrNotFound", err)
	}
}

func TestZipReader_OpenEntry_TemporarilyUnavailable(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	zipPath := filepath.Join(root, "000.zip")
	//nolint:errcheck // Test helper: intentionally creating invalid zip for testing
	_ = os.WriteFile(zipPath, []byte("not-a-zip"), 0o600)

	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, nil)
	zr := NewZipReader(zic)

	_, err := zr.OpenEntry(zipPath, "checkpoint")
	if !errors.Is(err, ErrZipTemporarilyUnavailable) {
		t.Fatalf("OpenEntry() error = %v, want ErrZipTemporarilyUnavailable", err)
	}
}

func mustCreateZip(t *testing.T, path string, files map[string][]byte) {
	t.Helper()

	//nolint:gosec // G304: path is validated and comes from test helpers, not user input
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create(%q) error = %v", path, err)
	}
	defer func() { _ = f.Close() }()

	w := zip.NewWriter(f)
	for name, contents := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatalf("zip.Create(%q) error = %v", name, err)
		}
		if _, err := fw.Write(contents); err != nil {
			t.Fatalf("zip write %q error = %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zip.Close() error = %v", err)
	}
}

