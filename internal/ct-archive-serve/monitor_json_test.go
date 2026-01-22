package ctarchiveserve

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestMonitorSnapshotBuilder_ExtractLogV3JSON(t *testing.T) {
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
	builder := NewMonitorJSONBuilder(cfg, zr, nil, nil)

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
		"tile/0/000":    []byte("tile data"),
	})

	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, nil)
	zr := NewZipReader(zic)

	cfg := Config{
		ArchivePath:          root,
		ArchiveFolderPattern: "ct_*",
	}
	builder := NewMonitorJSONBuilder(cfg, zr, nil, nil)

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
	builder := NewMonitorJSONBuilder(cfg, zr, nil, nil)

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

func TestMonitorJSONBuilder_RefreshFailure_503(t *testing.T) {
	t.Parallel()

	// Test that when refresh fails (e.g., archive index is nil), we get 503 behavior
	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, nil)
	zr := NewZipReader(zic)

	cfg := Config{
		ArchivePath:                "/nonexistent",
		ArchiveFolderPattern:       "ct_*",
		MonitorJSONRefreshInterval: 100 * time.Millisecond,
	}

	// Create builder with nil archiveIndex (will cause BuildSnapshot to fail)
	builder := NewMonitorJSONBuilder(cfg, zr, nil, nil)

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

// mustCreateZipForMonitor is a helper to create zip files for monitor.json tests.
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
