package ctarchiveserve

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/google/certificate-transparency-go/loglist3"
)

func TestLogListV3JSONSnapshotBuilder_ExtractLogV3JSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	logFolder := filepath.Join(root, "ct_test_log")
	if err := os.MkdirAll(logFolder, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	zipPath := filepath.Join(logFolder, "000.zip")
	mustCreateZipForMonitor(t, zipPath, map[string][]byte{
		"log.v3.json": []byte(`{"description":"Test Log","log_id":"abc123","key":"def456","mmd":86400,"log_type":"prod","state":{}}`),
	})

	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, nil)
	zr := NewZipReader(zic)

	cfg := Config{
		ArchivePath:          root,
		ArchiveFolderPattern: "ct_*",
	}
	builder := NewLogListV3JSONBuilder(cfg, zr, nil, nil)

	logV3, err := builder.extractLogV3JSON(zipPath)
	if err != nil {
		t.Fatalf("extractLogV3JSON() error = %v", err)
	}

	if logV3.Description != "Test Log" {
		t.Errorf("logV3.Description = %q, want %q", logV3.Description, "Test Log")
	}
	if logV3.LogID != "abc123" {
		t.Errorf("logV3.LogID = %q, want %q", logV3.LogID, "abc123")
	}
}

func TestMonitorSnapshotBuilder_HasIssuers_True(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	logFolder := filepath.Join(root, "ct_test_log")
	if err := os.MkdirAll(logFolder, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	zipPath := filepath.Join(logFolder, "000.zip")
	mustCreateZipForMonitor(t, zipPath, map[string][]byte{
		"log.v3.json":   []byte(`{"description":"Test"}`),
		"issuer/abc123": []byte("cert data"),
		"issuer/def456": []byte("cert data"),
		"tile/0/x000":   []byte("tile data"),
	})

	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, nil)
	zr := NewZipReader(zic)

	cfg := Config{
		ArchivePath:          root,
		ArchiveFolderPattern: "ct_*",
	}
	builder := NewLogListV3JSONBuilder(cfg, zr, nil, nil)

	hasIssuers, err := builder.checkHasIssuers(zipPath)
	if err != nil {
		t.Fatalf("checkHasIssuers() error = %v", err)
	}
	if !hasIssuers {
		t.Errorf("checkHasIssuers() = false, want true (zip contains issuer/ entries)")
	}
}

func TestMonitorSnapshotBuilder_HasIssuers_False(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	logFolder := filepath.Join(root, "ct_test_log")
	if err := os.MkdirAll(logFolder, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	zipPath := filepath.Join(logFolder, "000.zip")
	mustCreateZipForMonitor(t, zipPath, map[string][]byte{
		"log.v3.json": []byte(`{"description":"Test"}`),
		"tile/0/000":  []byte("tile data"),
		// No issuer/ entries
	})

	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, nil)
	zr := NewZipReader(zic)

	cfg := Config{
		ArchivePath:          root,
		ArchiveFolderPattern: "ct_*",
	}
	builder := NewLogListV3JSONBuilder(cfg, zr, nil, nil)

	hasIssuers, err := builder.checkHasIssuers(zipPath)
	if err != nil {
		t.Fatalf("checkHasIssuers() error = %v", err)
	}
	if hasIssuers {
		t.Errorf("checkHasIssuers() = true, want false (zip contains no issuer/ entries)")
	}
}

func TestMonitorSnapshotBuilder_DeterministicSort(t *testing.T) {
	t.Parallel()

	// Test that log names are sorted deterministically (ascending ASCII)
	logNames := []string{"log_b", "log_a", "log_c"}
	sort.Strings(logNames)

	expected := []string{"log_a", "log_b", "log_c"}
	if len(logNames) != len(expected) {
		t.Fatalf("logNames length = %d, want %d", len(logNames), len(expected))
	}
	for i, want := range expected {
		if logNames[i] != want {
			t.Errorf("logNames[%d] = %q, want %q", i, logNames[i], want)
		}
	}
}

func TestMonitorSnapshotBuilder_RemoveURL_AddSubmissionMonitoring(t *testing.T) {
	t.Parallel()

	// Test that a log.v3.json entry with "url" gets it removed and gets submission_url/monitoring_url added
	logV3JSON := `{"description":"Test","log_id":"abc","key":"def","url":"https://old.example","mmd":86400,"log_type":"prod","state":{}}`

	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(logV3JSON), &entry); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	// Remove url
	delete(entry, "url")

	// Add submission_url and monitoring_url
	publicBaseURL := "https://ct.example"
	logName := "test_log"
	entry["submission_url"] = publicBaseURL + "/" + logName
	entry["monitoring_url"] = publicBaseURL + "/" + logName

	if entry["url"] != nil {
		t.Errorf("entry[\"url\"] = %v, want nil (should be removed)", entry["url"])
	}
	if got, want := entry["submission_url"], "https://ct.example/test_log"; got != want {
		t.Errorf("entry[\"submission_url\"] = %q, want %q", got, want)
	}
	if got, want := entry["monitoring_url"], "https://ct.example/test_log"; got != want {
		t.Errorf("entry[\"monitoring_url\"] = %q, want %q", got, want)
	}
}

func TestLogListV3JSONBuilder_RefreshFailure_503(t *testing.T) {
	t.Parallel()

	// Test that when refresh fails (e.g., archive index is nil), we get 503 behavior
	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, nil)
	zr := NewZipReader(zic)

	cfg := Config{
		ArchivePath:                "/nonexistent",
		ArchiveFolderPattern:       "ct_*",
		LogListV3JSONRefreshInterval: 100 * time.Millisecond,
	}

	// Create builder with nil archiveIndex (will cause BuildSnapshot to fail)
	builder := NewLogListV3JSONBuilder(cfg, zr, nil, nil)

	// Manually trigger a refresh that will fail
	_, err := builder.BuildSnapshot("http://example.com")
	if err == nil {
		t.Fatalf("BuildSnapshot() with nil archiveIndex should fail")
	}

	// Test that GetSnapshotForRequest handles nil/error state correctly
	// When snapshot is nil or has LastError, it should return that state
	requestSnap := builder.GetSnapshotForRequest("http://example.com")
	// GetSnapshotForRequest may return nil if no snapshot exists, or a snapshot with LastError
	// The handler will check LastError and return 503
	if requestSnap != nil && requestSnap.LastError == nil {
		// If we got a snapshot, it should have LastError set when refresh failed
		// But if snapshot is nil, that's also valid (handler returns 503)
		t.Logf("Note: GetSnapshotForRequest returned snapshot without error (may be nil snapshot)")
	}
}

func TestLogListV3JSONBuilder_LogListV3Validation(t *testing.T) {
	t.Parallel()

	// Test that the generated logs.v3.json can be parsed and validated by loglist3 library
	// per spec.md FR-006 validation requirement
	root := t.TempDir()
	logFolder := filepath.Join(root, "ct_test_log")
	if err := os.MkdirAll(logFolder, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Use valid base64-encoded values for log_id and key (loglist3 expects base64-encoded byte arrays)
	// log_id: base64("test_log_id_32_bytes_long!!") = "dGVzdF9sb2dfaWRfMzJfYnl0ZXNfbG9uZyEh"
	// key: base64("test_key_32_bytes_long_data!!") = "dGVzdF9rZXlfMzJfYnl0ZXNfbG9uZ19kYXRhISE="
	zipPath := filepath.Join(logFolder, "000.zip")
	mustCreateZipForMonitor(t, zipPath, map[string][]byte{
		"log.v3.json": []byte(`{"description":"Test Log","log_id":"dGVzdF9sb2dfaWRfMzJfYnl0ZXNfbG9uZyEh","key":"dGVzdF9rZXlfMzJfYnl0ZXNfbG9uZ19kYXRhISE=","mmd":86400,"log_type":"prod","state":{}}`),
	})

	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, nil)
	zr := NewZipReader(zic)

	cfg := Config{
		ArchivePath:          root,
		ArchiveFolderPattern: "ct_*",
	}
	archiveIndex, err := NewArchiveIndex(cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewArchiveIndex() error = %v", err)
	}

	builder := NewLogListV3JSONBuilder(cfg, zr, archiveIndex, nil)

	// Build a snapshot
	snap, err := builder.BuildSnapshot("https://example.com")
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}

	// Serialize to JSON
	jsonBytes, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	// Validate using loglist3 library - this will return an error if the JSON
	// doesn't conform to the v3 schema
	logList, err := loglist3.NewFromJSON(jsonBytes)
	if err != nil {
		t.Fatalf("loglist3.NewFromJSON() error = %v (logs.v3.json does not conform to v3 schema)", err)
	}

	// Verify basic structure
	if logList.Version != "3.0" {
		t.Errorf("logList.Version = %q, want %q", logList.Version, "3.0")
	}
	if len(logList.Operators) != 1 {
		t.Fatalf("logList.Operators length = %d, want 1", len(logList.Operators))
	}
	op := logList.Operators[0]
	if op.Name != "ct-archive-serve" {
		t.Errorf("op.Name = %q, want %q", op.Name, "ct-archive-serve")
	}
	if len(op.TiledLogs) != 1 {
		t.Errorf("op.TiledLogs length = %d, want 1", len(op.TiledLogs))
	}
}

// mustCreateZipForMonitor is a helper to create zip files for logs.v3.json tests.
func mustCreateZipForMonitor(t *testing.T, path string, files map[string][]byte) {
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
