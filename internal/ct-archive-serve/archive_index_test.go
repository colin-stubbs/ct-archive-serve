package ctarchiveserve

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildArchiveSnapshot_DiscoversLogsAndZipParts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	mustMkdir(t, filepath.Join(root, "ct_log1"))
	mustMkdir(t, filepath.Join(root, "ct_log2"))
	mustMkdir(t, filepath.Join(root, "not_ct_log"))

	mustWriteFile(t, filepath.Join(root, "ct_log1", "000.zip"), []byte("x"))
	mustWriteFile(t, filepath.Join(root, "ct_log1", "001.zip"), []byte("x"))
	mustWriteFile(t, filepath.Join(root, "ct_log1", "bad.zip"), []byte("x"))
	mustWriteFile(t, filepath.Join(root, "ct_log1", "00.zip"), []byte("x"))

	mustWriteFile(t, filepath.Join(root, "ct_log2", "000.zip"), []byte("x"))

	cfg := Config{
		ArchivePath:         root,
		ArchiveFolderPrefix: "ct_",
	}

	snap, err := buildArchiveSnapshot(cfg, os.ReadDir)
	if err != nil {
		t.Fatalf("buildArchiveSnapshot() error = %v", err)
	}

	if got, want := len(snap.Logs), 2; got != want {
		t.Fatalf("len(Logs) = %d, want %d", got, want)
	}

	l1, ok := snap.Logs["log1"]
	if !ok {
		t.Fatalf("expected log1 to be discovered")
	}
	if got := filepath.Base(l1.FolderPath); got != "ct_log1" {
		t.Fatalf("log1 FolderPath base = %q, want %q", got, "ct_log1")
	}
	if got, want := l1.ZipParts, []int{0, 1}; !intSlicesEqual(got, want) {
		t.Fatalf("log1 ZipParts = %v, want %v", got, want)
	}

	l2, ok := snap.Logs["log2"]
	if !ok {
		t.Fatalf("expected log2 to be discovered")
	}
	if got := filepath.Base(l2.FolderPath); got != "ct_log2" {
		t.Fatalf("log2 FolderPath base = %q, want %q", got, "ct_log2")
	}
	if got, want := l2.ZipParts, []int{0}; !intSlicesEqual(got, want) {
		t.Fatalf("log2 ZipParts = %v, want %v", got, want)
	}
}

func TestBuildArchiveSnapshot_LogCollisionFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "ct_a"))
	mustWriteFile(t, filepath.Join(root, "ct_a", "000.zip"), []byte("x"))

	cfg := Config{
		ArchivePath:         root,
		ArchiveFolderPrefix: "ct_",
	}

	dupReadDir := func(path string) ([]os.DirEntry, error) {
		ents, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}
		// Return the same directory entry twice to force a collision code path.
		return append(ents, ents...), nil
	}

	_, err := buildArchiveSnapshot(cfg, dupReadDir)
	if err == nil {
		t.Fatalf("buildArchiveSnapshot() error = nil, want non-nil")
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func intSlicesEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestArchiveIndex_SelectZipPart_DataTiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	logFolder := filepath.Join(root, "ct_test_log")
	mustMkdir(t, logFolder)
	mustWriteFile(t, filepath.Join(logFolder, "000.zip"), []byte("x"))
	mustWriteFile(t, filepath.Join(logFolder, "001.zip"), []byte("x"))
	mustWriteFile(t, filepath.Join(logFolder, "002.zip"), []byte("x"))

	cfg := Config{
		ArchivePath:         root,
		ArchiveFolderPrefix: "ct_",
	}
	ai, err := NewArchiveIndex(cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewArchiveIndex() error = %v", err)
	}

	tests := []struct {
		name      string
		tileIndex uint64
		wantZip   int
		wantOK    bool
	}{
		{"N=0", 0, 0, true},
		{"N=65535", 65535, 0, true},
		{"N=65536", 65536, 1, true},
		{"N=131071", 131071, 1, true},
		{"N=131072", 131072, 2, true},
		{"N=999999", 999999, 0, false}, // 999999 / 65536 = 15, but zip 15 doesn't exist
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zipIndex, ok := ai.SelectZipPart("test_log", 0, tt.tileIndex, true)
			if ok != tt.wantOK {
				t.Errorf("SelectZipPart() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && zipIndex != tt.wantZip {
				t.Errorf("SelectZipPart() zipIndex = %d, want %d", zipIndex, tt.wantZip)
			}
		})
	}
}

func TestArchiveIndex_SelectZipPart_HashTiles_Level0_1_2(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	logFolder := filepath.Join(root, "ct_test_log")
	mustMkdir(t, logFolder)
	mustWriteFile(t, filepath.Join(logFolder, "000.zip"), []byte("x"))
	mustWriteFile(t, filepath.Join(logFolder, "001.zip"), []byte("x"))
	mustWriteFile(t, filepath.Join(logFolder, "256.zip"), []byte("x"))

	cfg := Config{
		ArchivePath:         root,
		ArchiveFolderPrefix: "ct_",
	}
	ai, err := NewArchiveIndex(cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewArchiveIndex() error = %v", err)
	}

	tests := []struct {
		name      string
		level     uint8
		tileIndex uint64
		wantZip   int
		wantOK    bool
	}{
		// Level 0: zipIndex = N / 65536
		{"L=0, N=0", 0, 0, 0, true},
		{"L=0, N=65535", 0, 65535, 0, true},
		{"L=0, N=65536", 0, 65536, 1, true},
		// Level 1: zipIndex = N / 256
		{"L=1, N=0", 1, 0, 0, true},
		{"L=1, N=255", 1, 255, 0, true},
		{"L=1, N=256", 1, 256, 1, true},
		// Level 2: zipIndex = N
		{"L=2, N=0", 2, 0, 0, true},
		{"L=2, N=1", 2, 1, 1, true}, // zipIndex = N, so N=1 -> zip 1
		{"L=2, N=256", 2, 256, 256, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zipIndex, ok := ai.SelectZipPart("test_log", tt.level, tt.tileIndex, false)
			if ok != tt.wantOK {
				t.Errorf("SelectZipPart() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && zipIndex != tt.wantZip {
				t.Errorf("SelectZipPart() zipIndex = %d, want %d", zipIndex, tt.wantZip)
			}
		})
	}
}

func TestArchiveIndex_SelectZipPart_HashTiles_Level3Plus_Prefer000(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	logFolder := filepath.Join(root, "ct_test_log")
	mustMkdir(t, logFolder)
	mustWriteFile(t, filepath.Join(logFolder, "000.zip"), []byte("x"))
	mustWriteFile(t, filepath.Join(logFolder, "001.zip"), []byte("x"))
	mustWriteFile(t, filepath.Join(logFolder, "010.zip"), []byte("x"))

	cfg := Config{
		ArchivePath:         root,
		ArchiveFolderPrefix: "ct_",
	}
	ai, err := NewArchiveIndex(cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewArchiveIndex() error = %v", err)
	}

	// Level 3+: should prefer 000.zip
	zipIndex, ok := ai.SelectZipPart("test_log", 3, 12345, false)
	if !ok {
		t.Errorf("SelectZipPart() ok = false, want true")
	}
	if zipIndex != 0 {
		t.Errorf("SelectZipPart() zipIndex = %d, want 0 (prefer 000.zip)", zipIndex)
	}
}

func TestArchiveIndex_SelectZipPart_HashTiles_Level3Plus_No000_UseLowest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	logFolder := filepath.Join(root, "ct_test_log")
	mustMkdir(t, logFolder)
	// No 000.zip, only 001.zip and 010.zip
	mustWriteFile(t, filepath.Join(logFolder, "001.zip"), []byte("x"))
	mustWriteFile(t, filepath.Join(logFolder, "010.zip"), []byte("x"))

	cfg := Config{
		ArchivePath:         root,
		ArchiveFolderPrefix: "ct_",
	}
	ai, err := NewArchiveIndex(cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewArchiveIndex() error = %v", err)
	}

	// Level 3+: should use lowest available zip (001)
	zipIndex, ok := ai.SelectZipPart("test_log", 3, 12345, false)
	if !ok {
		t.Errorf("SelectZipPart() ok = false, want true")
	}
	if zipIndex != 1 {
		t.Errorf("SelectZipPart() zipIndex = %d, want 1 (lowest available)", zipIndex)
	}
}
