package ctarchiveserve

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestHTTPMethodPolicy_SupportedRoutes_GETAndHEAD(t *testing.T) {
	t.Parallel()

	// Create a minimal server with /metrics endpoint
	cfg := Config{
		ArchivePath:          "/tmp/test",
		ArchiveFolderPattern: "ct_*",
	}
	logger := NewLogger(LoggerOptions{})
	metrics := NewMetrics(prometheus.NewRegistry())
	server := NewServer(cfg, logger, metrics, nil, nil, nil)

	// Test /metrics accepts GET
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /metrics status = %d, want %d", w.Code, http.StatusOK)
	}

	// Test /metrics accepts HEAD
	req = httptest.NewRequest(http.MethodHead, "/metrics", nil)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("HEAD /metrics status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.Len() > 0 {
		t.Errorf("HEAD /metrics body length = %d, want 0 (no body for HEAD)", w.Body.Len())
	}
}

func TestHTTPMethodPolicy_UnsupportedMethods_405(t *testing.T) {
	t.Parallel()

	cfg := Config{
		ArchivePath:          "/tmp/test",
		ArchiveFolderPattern: "ct_*",
	}
	logger := NewLogger(LoggerOptions{})
	metrics := NewMetrics(prometheus.NewRegistry())
	server := NewServer(cfg, logger, metrics, nil, nil, nil)

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodOptions}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/metrics", nil)
			w := httptest.NewRecorder()
			server.ServeHTTP(w, req)
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s /metrics status = %d, want %d", method, w.Code, http.StatusMethodNotAllowed)
			}
			allow := w.Header().Get("Allow")
			if !strings.Contains(allow, "GET") || !strings.Contains(allow, "HEAD") {
				t.Errorf("%s /metrics Allow header = %q, want to contain GET and HEAD", method, allow)
			}
		})
	}
}

func TestHTTPMethodPolicy_UnknownRoutes_404(t *testing.T) {
	t.Parallel()

	cfg := Config{
		ArchivePath:          "/tmp/test",
		ArchiveFolderPattern: "ct_*",
	}
	logger := NewLogger(LoggerOptions{})
	metrics := NewMetrics(prometheus.NewRegistry())
	server := NewServer(cfg, logger, metrics, nil, nil, nil)

	methods := []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/unknown/route", nil)
			w := httptest.NewRecorder()
			server.ServeHTTP(w, req)
			if w.Code != http.StatusNotFound {
				t.Errorf("%s /unknown/route status = %d, want %d", method, w.Code, http.StatusNotFound)
			}
		})
	}
}

func TestPublicBaseURL_UntrustedSource_UsesHost(t *testing.T) {
	t.Parallel()

	cfg := Config{
		ArchivePath:          "/tmp/test",
		ArchiveFolderPattern: "ct_*",
		HTTPTrustedSources:   []netip.Prefix{}, // empty = no trusted sources
	}
	logger := NewLogger(LoggerOptions{})
	metrics := NewMetrics(prometheus.NewRegistry())
	server := NewServer(cfg, logger, metrics, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/monitor.json", nil)
	req.Host = "example.com"
	req.RemoteAddr = "192.168.1.100:12345" // untrusted IP
	req.Header.Set("X-Forwarded-Host", "evil.com")
	req.Header.Set("X-Forwarded-Proto", "https")

	baseURL := server.derivePublicBaseURL(req)
	if baseURL != "http://example.com" {
		t.Errorf("derivePublicBaseURL() = %q, want %q (should ignore X-Forwarded-* from untrusted source)", baseURL, "http://example.com")
	}
}

func TestPublicBaseURL_TrustedSource_UsesXForwarded(t *testing.T) {
	t.Parallel()

	//nolint:errcheck // Test helper with known-good value
	trusted, _ := netip.ParsePrefix("127.0.0.1/32")
	cfg := Config{
		ArchivePath:          "/tmp/test",
		ArchiveFolderPattern: "ct_*",
		HTTPTrustedSources:   []netip.Prefix{trusted},
	}
	logger := NewLogger(LoggerOptions{})
	metrics := NewMetrics(prometheus.NewRegistry())
	server := NewServer(cfg, logger, metrics, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/monitor.json", nil)
	req.Host = "example.com"
	req.RemoteAddr = "127.0.0.1:12345" // trusted IP
	req.Header.Set("X-Forwarded-Host", "proxy.example.com")
	req.Header.Set("X-Forwarded-Proto", "https")

	baseURL := server.derivePublicBaseURL(req)
	if baseURL != "https://proxy.example.com" {
		t.Errorf("derivePublicBaseURL() = %q, want %q (should use X-Forwarded-* from trusted source)", baseURL, "https://proxy.example.com")
	}
}

func TestPublicBaseURL_TrustedSource_NoXForwarded_UsesHost(t *testing.T) {
	t.Parallel()

	//nolint:errcheck // Test helper with known-good value
	trusted, _ := netip.ParsePrefix("127.0.0.1/32")
	cfg := Config{
		ArchivePath:          "/tmp/test",
		ArchiveFolderPattern: "ct_*",
		HTTPTrustedSources:   []netip.Prefix{trusted},
	}
	logger := NewLogger(LoggerOptions{})
	metrics := NewMetrics(prometheus.NewRegistry())
	server := NewServer(cfg, logger, metrics, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/monitor.json", nil)
	req.Host = "example.com"
	req.RemoteAddr = "127.0.0.1:12345" // trusted IP, but no X-Forwarded-* headers

	baseURL := server.derivePublicBaseURL(req)
	if baseURL != "http://example.com" {
		t.Errorf("derivePublicBaseURL() = %q, want %q (should fallback to Host when X-Forwarded-* missing)", baseURL, "http://example.com")
	}
}

func TestPublicBaseURL_CommaSeparated_FirstNonEmpty(t *testing.T) {
	t.Parallel()

	//nolint:errcheck // Test helper with known-good value
	trusted, _ := netip.ParsePrefix("127.0.0.1/32")
	cfg := Config{
		ArchivePath:          "/tmp/test",
		ArchiveFolderPattern: "ct_*",
		HTTPTrustedSources:   []netip.Prefix{trusted},
	}
	logger := NewLogger(LoggerOptions{})
	metrics := NewMetrics(prometheus.NewRegistry())
	server := NewServer(cfg, logger, metrics, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/monitor.json", nil)
	req.Host = "example.com"
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-Host", " , proxy.example.com , other.com")
	req.Header.Set("X-Forwarded-Proto", "https,http")

	baseURL := server.derivePublicBaseURL(req)
	if baseURL != "https://proxy.example.com" {
		t.Errorf("derivePublicBaseURL() = %q, want %q (should use first non-empty after trimming)", baseURL, "https://proxy.example.com")
	}
}

func TestPublicBaseURL_SchemeLowercased(t *testing.T) {
	t.Parallel()

	//nolint:errcheck // Test helper with known-good value
	trusted, _ := netip.ParsePrefix("127.0.0.1/32")
	cfg := Config{
		ArchivePath:          "/tmp/test",
		ArchiveFolderPattern: "ct_*",
		HTTPTrustedSources:   []netip.Prefix{trusted},
	}
	logger := NewLogger(LoggerOptions{})
	metrics := NewMetrics(prometheus.NewRegistry())
	server := NewServer(cfg, logger, metrics, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/monitor.json", nil)
	req.Host = "example.com"
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-Proto", "HTTPS")

	baseURL := server.derivePublicBaseURL(req)
	if baseURL != "https://example.com" {
		t.Errorf("derivePublicBaseURL() = %q, want %q (should lowercase scheme)", baseURL, "https://example.com")
	}
}

func TestServer_HandleCheckpoint_200(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	logFolder := filepath.Join(root, "ct_test_log")
	if err := os.MkdirAll(logFolder, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	zipPath := filepath.Join(logFolder, "000.zip")
	mustCreateZip(t, zipPath, map[string][]byte{
		"checkpoint": []byte("test checkpoint data"),
	})

	cfg := Config{
		ArchivePath:          root,
		ArchiveFolderPattern: "ct_*",
		ArchiveFolderPrefix:  "ct_", // Parsed from pattern
	}
	logger := NewLogger(LoggerOptions{})
	metrics := NewMetrics(prometheus.NewRegistry())

	archiveIndex, err := NewArchiveIndex(cfg, logger, metrics)
	if err != nil {
		t.Fatalf("NewArchiveIndex() error = %v", err)
	}

	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, metrics)
	zr := NewZipReader(zic)
	server := NewServer(cfg, logger, metrics, archiveIndex, zr, nil)

	req := httptest.NewRequest(http.MethodGet, "/test_log/checkpoint", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /test_log/checkpoint status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/plain; charset=utf-8")
	}
	if body := w.Body.String(); body != "test checkpoint data" {
		t.Errorf("body = %q, want %q", body, "test checkpoint data")
	}
}

func TestServer_HandleCheckpoint_404(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := Config{
		ArchivePath:          root,
		ArchiveFolderPattern: "ct_*",
		ArchiveFolderPrefix:  "ct_", // Parsed from pattern
	}
	logger := NewLogger(LoggerOptions{})
	metrics := NewMetrics(prometheus.NewRegistry())

	archiveIndex, err := NewArchiveIndex(cfg, logger, metrics)
	if err != nil {
		t.Fatalf("NewArchiveIndex() error = %v", err)
	}

	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, metrics)
	zr := NewZipReader(zic)
	server := NewServer(cfg, logger, metrics, archiveIndex, zr, nil)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent/checkpoint", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("GET /nonexistent/checkpoint status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestServer_HandleCheckpoint_HEAD(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	logFolder := filepath.Join(root, "ct_test_log")
	if err := os.MkdirAll(logFolder, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	zipPath := filepath.Join(logFolder, "000.zip")
	mustCreateZip(t, zipPath, map[string][]byte{
		"checkpoint": []byte("test checkpoint data"),
	})

	cfg := Config{
		ArchivePath:          root,
		ArchiveFolderPattern: "ct_*",
		ArchiveFolderPrefix:  "ct_", // Parsed from pattern
	}
	logger := NewLogger(LoggerOptions{})
	metrics := NewMetrics(prometheus.NewRegistry())

	archiveIndex, err := NewArchiveIndex(cfg, logger, metrics)
	if err != nil {
		t.Fatalf("NewArchiveIndex() error = %v", err)
	}

	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, metrics)
	zr := NewZipReader(zic)
	server := NewServer(cfg, logger, metrics, archiveIndex, zr, nil)

	req := httptest.NewRequest(http.MethodHead, "/test_log/checkpoint", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("HEAD /test_log/checkpoint status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.Len() > 0 {
		t.Errorf("HEAD /test_log/checkpoint body length = %d, want 0 (no body for HEAD)", w.Body.Len())
	}
}

func TestServer_HandleLogV3JSON_200(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	logFolder := filepath.Join(root, "ct_test_log")
	if err := os.MkdirAll(logFolder, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	zipPath := filepath.Join(logFolder, "000.zip")
	mustCreateZip(t, zipPath, map[string][]byte{
		"log.v3.json": []byte(`{"description":"Test Log"}`),
	})

	cfg := Config{
		ArchivePath:          root,
		ArchiveFolderPattern: "ct_*",
		ArchiveFolderPrefix:  "ct_", // Parsed from pattern
	}
	logger := NewLogger(LoggerOptions{})
	metrics := NewMetrics(prometheus.NewRegistry())

	archiveIndex, err := NewArchiveIndex(cfg, logger, metrics)
	if err != nil {
		t.Fatalf("NewArchiveIndex() error = %v", err)
	}

	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, metrics)
	zr := NewZipReader(zic)
	server := NewServer(cfg, logger, metrics, archiveIndex, zr, nil)

	req := httptest.NewRequest(http.MethodGet, "/test_log/log.v3.json", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /test_log/log.v3.json status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

func TestServer_HandleHashTile_200(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	logFolder := filepath.Join(root, "ct_test_log")
	if err := os.MkdirAll(logFolder, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Create zip with hash tile at level 0, index 0 (should be in 000.zip)
	zipPath := filepath.Join(logFolder, "000.zip")
	mustCreateZip(t, zipPath, map[string][]byte{
		"tile/0/x000": []byte("hash tile data"),
	})

	cfg := Config{
		ArchivePath:          root,
		ArchiveFolderPattern: "ct_*",
		ArchiveFolderPrefix:  "ct_",
	}
	logger := NewLogger(LoggerOptions{})
	metrics := NewMetrics(prometheus.NewRegistry())

	archiveIndex, err := NewArchiveIndex(cfg, logger, metrics)
	if err != nil {
		t.Fatalf("NewArchiveIndex() error = %v", err)
	}

	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, metrics)
	zr := NewZipReader(zic)
	server := NewServer(cfg, logger, metrics, archiveIndex, zr, nil)

	req := httptest.NewRequest(http.MethodGet, "/test_log/tile/0/x000", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /test_log/tile/0/x000 status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/octet-stream")
	}
	if body := w.Body.String(); body != "hash tile data" {
		t.Errorf("body = %q, want %q", body, "hash tile data")
	}
}

func TestServer_HandleDataTile_200(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	logFolder := filepath.Join(root, "ct_test_log")
	if err := os.MkdirAll(logFolder, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Create zip with data tile at index 0 (should be in 000.zip)
	zipPath := filepath.Join(logFolder, "000.zip")
	mustCreateZip(t, zipPath, map[string][]byte{
		"tile/data/x000": []byte("data tile data"),
	})

	cfg := Config{
		ArchivePath:          root,
		ArchiveFolderPattern: "ct_*",
		ArchiveFolderPrefix:  "ct_",
	}
	logger := NewLogger(LoggerOptions{})
	metrics := NewMetrics(prometheus.NewRegistry())

	archiveIndex, err := NewArchiveIndex(cfg, logger, metrics)
	if err != nil {
		t.Fatalf("NewArchiveIndex() error = %v", err)
	}

	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, metrics)
	zr := NewZipReader(zic)
	server := NewServer(cfg, logger, metrics, archiveIndex, zr, nil)

	req := httptest.NewRequest(http.MethodGet, "/test_log/tile/data/x000", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /test_log/tile/data/x000 status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/octet-stream")
	}
	if body := w.Body.String(); body != "data tile data" {
		t.Errorf("body = %q, want %q", body, "data tile data")
	}
}

func TestServer_HandleHashTile_Partial_200(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	logFolder := filepath.Join(root, "ct_test_log")
	if err := os.MkdirAll(logFolder, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Create zip with partial hash tile
	zipPath := filepath.Join(logFolder, "000.zip")
	mustCreateZip(t, zipPath, map[string][]byte{
		"tile/0/x000.p/128": []byte("partial tile data"),
	})

	cfg := Config{
		ArchivePath:          root,
		ArchiveFolderPattern: "ct_*",
		ArchiveFolderPrefix:  "ct_",
	}
	logger := NewLogger(LoggerOptions{})
	metrics := NewMetrics(prometheus.NewRegistry())

	archiveIndex, err := NewArchiveIndex(cfg, logger, metrics)
	if err != nil {
		t.Fatalf("NewArchiveIndex() error = %v", err)
	}

	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, metrics)
	zr := NewZipReader(zic)
	server := NewServer(cfg, logger, metrics, archiveIndex, zr, nil)

	req := httptest.NewRequest(http.MethodGet, "/test_log/tile/0/x000.p/128", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /test_log/tile/0/x000.p/128 status = %d, want %d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); body != "partial tile data" {
		t.Errorf("body = %q, want %q", body, "partial tile data")
	}
}

func TestServer_HandleIssuer_200(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	logFolder := filepath.Join(root, "ct_test_log")
	if err := os.MkdirAll(logFolder, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	zipPath := filepath.Join(logFolder, "000.zip")
	mustCreateZip(t, zipPath, map[string][]byte{
		"issuer/abc123def456": []byte("cert data"),
	})

	cfg := Config{
		ArchivePath:          root,
		ArchiveFolderPattern: "ct_*",
		ArchiveFolderPrefix:  "ct_",
	}
	logger := NewLogger(LoggerOptions{})
	metrics := NewMetrics(prometheus.NewRegistry())

	archiveIndex, err := NewArchiveIndex(cfg, logger, metrics)
	if err != nil {
		t.Fatalf("NewArchiveIndex() error = %v", err)
	}

	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, metrics)
	zr := NewZipReader(zic)
	server := NewServer(cfg, logger, metrics, archiveIndex, zr, nil)

	req := httptest.NewRequest(http.MethodGet, "/test_log/issuer/abc123def456", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /test_log/issuer/abc123def456 status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/pkix-cert" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/pkix-cert")
	}
	if body := w.Body.String(); body != "cert data" {
		t.Errorf("body = %q, want %q", body, "cert data")
	}
}

func TestServer_HandleIssuer_404(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := Config{
		ArchivePath:          root,
		ArchiveFolderPattern: "ct_*",
		ArchiveFolderPrefix:  "ct_",
	}
	logger := NewLogger(LoggerOptions{})
	metrics := NewMetrics(prometheus.NewRegistry())

	archiveIndex, err := NewArchiveIndex(cfg, logger, metrics)
	if err != nil {
		t.Fatalf("NewArchiveIndex() error = %v", err)
	}

	zic := NewZipIntegrityCache(5*time.Minute, time.Now, nil, metrics)
	zr := NewZipReader(zic)
	server := NewServer(cfg, logger, metrics, archiveIndex, zr, nil)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent/issuer/abc123", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("GET /nonexistent/issuer/abc123 status = %d, want %d", w.Code, http.StatusNotFound)
	}
}
