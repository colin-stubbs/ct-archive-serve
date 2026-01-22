package ctarchiveserve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	zipPath := filepath.Join(logFolder, "000.zip")
	mustCreateZip(t, zipPath, map[string][]byte{
		"checkpoint":        []byte("test checkpoint"),
		"log.v3.json":       []byte(`{"description":"Test Log","log_id":"abc123","key":"def456","mmd":86400,"log_type":"prod","state":{}}`),
		"tile/0/x000":       []byte("hash tile"),
		"tile/data/x000":    []byte("data tile"),
		"issuer/abc123def": []byte("cert data"),
	})

	cfg := Config{
		ArchivePath:                root,
		ArchiveFolderPattern:       "ct_*",
		ArchiveFolderPrefix:        "ct_",
		MonitorJSONRefreshInterval: 1 * time.Minute,
	}
	logger := NewLogger(LoggerOptions{})
	metrics := NewMetrics(prometheus.NewRegistry())

	archiveIndex, err := NewArchiveIndex(cfg, logger, metrics)
	if err != nil {
		t.Fatalf("NewArchiveIndex() error = %v", err)
	}

	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, metrics)
	zr := NewZipReader(zic)
	monitorJSON := NewMonitorJSONBuilder(cfg, zr, archiveIndex, logger)
	
	// Start refresh loop with a context that will be cancelled when test completes
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	monitorJSON.Start(ctx)
	
	// Wait a bit for initial refresh to complete
	time.Sleep(100 * time.Millisecond)

	server := NewServer(cfg, logger, metrics, archiveIndex, zr, monitorJSON)

	// Test /monitor.json
	t.Run("monitor.json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/monitor.json", nil)
		req.Host = "example.com"
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("GET /monitor.json status = %d, want %d", w.Code, http.StatusOK)
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/json")
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse monitor.json: %v", err)
		}
		if v, ok := resp["version"].(string); !ok || v != "3.0" {
			t.Errorf("version = %v, want %q", resp["version"], "3.0")
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
