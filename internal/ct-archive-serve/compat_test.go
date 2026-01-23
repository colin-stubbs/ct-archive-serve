package ctarchiveserve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/certificate-transparency-go/loglist3"
	"github.com/prometheus/client_golang/prometheus"
)

// TestCompatibility_SmokeTest verifies that ct-archive-serve can serve basic Static-CT assets
// per spec.md NFR-012 (independent implementation, no reuse of other repositories).
func TestCompatibility_SmokeTest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	logFolder := filepath.Join(root, "ct_test_log")
	if err := os.MkdirAll(logFolder, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Create a minimal archive with required entries
	// Use valid base64-encoded values for log_id and key (loglist3 expects base64-encoded byte arrays)
	// log_id: base64("test_log_id_32_bytes_long!!") = "dGVzdF9sb2dfaWRfMzJfYnl0ZXNfbG9uZyEh"
	// key: base64("test_key_32_bytes_long_data!!") = "dGVzdF9rZXlfMzJfYnl0ZXNfbG9uZ19kYXRhISE="
	zipPath := filepath.Join(logFolder, "000.zip")
	mustCreateZip(t, zipPath, map[string][]byte{
		"checkpoint":        []byte("test checkpoint"),
		"log.v3.json":       []byte(`{"description":"Test Log","log_id":"dGVzdF9sb2dfaWRfMzJfYnl0ZXNfbG9uZyEh","key":"dGVzdF9rZXlfMzJfYnl0ZXNfbG9uZ19kYXRhISE=","mmd":86400,"log_type":"prod","state":{}}`),
		"tile/0/x000":       []byte("hash tile"),
		"tile/data/x000":    []byte("data tile"),
		"issuer/abc123def": []byte("cert data"),
	})

	cfg := Config{
		ArchivePath:                root,
		ArchiveFolderPattern:       "ct_*",
		ArchiveFolderPrefix:        "ct_",
		LogListV3JSONRefreshInterval: 1 * time.Minute,
	}
	logger := NewLogger(LoggerOptions{})
	metrics := NewMetrics(prometheus.NewRegistry())

	archiveIndex, err := NewArchiveIndex(cfg, logger, metrics)
	if err != nil {
		t.Fatalf("NewArchiveIndex() error = %v", err)
	}

	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, metrics)
	zr := NewZipReader(zic)
	logListV3JSON := NewLogListV3JSONBuilder(cfg, zr, archiveIndex, logger)
	
	// Start refresh loop with a context that will be cancelled when test completes
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logListV3JSON.Start(ctx)
	
	// Wait a bit for initial refresh to complete
	time.Sleep(100 * time.Millisecond)

	server := NewServer(cfg, logger, metrics, archiveIndex, zr, logListV3JSON)

	// Test /logs.v3.json
	t.Run("logs.v3.json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/logs.v3.json", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("GET /logs.v3.json status = %d, want %d", w.Code, http.StatusOK)
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/json")
		}

		// Validate using loglist3 library per spec.md FR-006 validation requirement
		logList, err := loglist3.NewFromJSON(w.Body.Bytes())
		if err != nil {
			t.Fatalf("loglist3.NewFromJSON() error = %v (logs.v3.json does not conform to v3 schema)", err)
		}

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
	})

	// Test /<log>/checkpoint
	t.Run("checkpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test_log/checkpoint", nil)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("GET /test_log/checkpoint status = %d, want %d", w.Code, http.StatusOK)
		}
		if body := w.Body.String(); body != "test checkpoint" {
			t.Errorf("body = %q, want %q", body, "test checkpoint")
		}
	})

	// Test /<log>/log.v3.json
	t.Run("log.v3.json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test_log/log.v3.json", nil)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("GET /test_log/log.v3.json status = %d, want %d", w.Code, http.StatusOK)
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/json")
		}
	})

	// Test /<log>/tile/<L>/<N>
	t.Run("hash_tile", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test_log/tile/0/x000", nil)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("GET /test_log/tile/0/x000 status = %d, want %d", w.Code, http.StatusOK)
		}
		if body := w.Body.String(); body != "hash tile" {
			t.Errorf("body = %q, want %q", body, "hash tile")
		}
	})

	// Test /<log>/tile/data/<N>
	t.Run("data_tile", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test_log/tile/data/x000", nil)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("GET /test_log/tile/data/x000 status = %d, want %d", w.Code, http.StatusOK)
		}
		if body := w.Body.String(); body != "data tile" {
			t.Errorf("body = %q, want %q", body, "data tile")
		}
	})

	// Test /<log>/issuer/<fingerprint>
	t.Run("issuer", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test_log/issuer/abc123def", nil)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("GET /test_log/issuer/abc123def status = %d, want %d", w.Code, http.StatusOK)
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/pkix-cert" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/pkix-cert")
		}
		if body := w.Body.String(); body != "cert data" {
			t.Errorf("body = %q, want %q", body, "cert data")
		}
	})
}
