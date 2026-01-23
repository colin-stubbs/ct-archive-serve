package ctarchiveserve

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server is the HTTP server for ct-archive-serve.
type Server struct {
	cfg     Config
	logger  *slog.Logger
	metrics *Metrics
	verbose bool // Enable verbose logging (log 2xx responses)

	// Components (may be nil during initial setup)
	archiveIndex *ArchiveIndex
	zipReader    *ZipReader
	logListV3JSON  *LogListV3JSONBuilder
}

// NewServer constructs a new Server instance.
func NewServer(
	cfg Config,
	logger *slog.Logger,
	metrics *Metrics,
	archiveIndex *ArchiveIndex,
	zipReader *ZipReader,
	logListV3JSON *LogListV3JSONBuilder,
) *Server {
	return &Server{
		cfg:          cfg,
		logger:      logger,
		metrics:     metrics,
		verbose:     false, // Will be set from CLI flags in main.go
		archiveIndex: archiveIndex,
		zipReader:   zipReader,
		logListV3JSON: logListV3JSON,
	}
}

// SetVerbose enables verbose logging (logs 2xx responses).
func (s *Server) SetVerbose(v bool) {
	s.verbose = v
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	route, ok := ParseRoute(r.URL.Path)
	
	// Create a response writer that captures status code
	rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

	if !ok {
		// Unknown/unsupported routes return 404 regardless of method per spec.md FR-002a
		http.NotFound(rw, r)
		s.logRequest(r, route, rw.statusCode, time.Since(start))
		return
	}

	// Enforce HTTP method policy per spec.md FR-002a
	// For supported routes, only GET and HEAD are allowed
	if !s.isMethodAllowed(r.Method) {
		rw.Header().Set("Allow", "GET, HEAD")
		rw.statusCode = http.StatusMethodNotAllowed
		http.Error(rw, "Method Not Allowed", http.StatusMethodNotAllowed)
		s.logRequest(r, route, rw.statusCode, time.Since(start))
		return
	}

	switch route.Kind {
	case RouteMetrics:
		s.handleMetrics(rw, r)
	case RouteLogListV3JSON:
		s.handleLogListV3JSON(rw, r)
	case RouteCheckpoint:
		s.handleCheckpoint(rw, r, route)
	case RouteLogV3JSON:
		s.handleLogV3JSON(rw, r, route)
	case RouteHashTile:
		s.handleHashTile(rw, r, route)
	case RouteDataTile:
		s.handleDataTile(rw, r, route)
	case RouteIssuer:
		s.handleIssuer(rw, r, route)
	default:
		// Other routes will be implemented in later tasks
		http.NotFound(rw, r)
	}
	
	s.logRequest(r, route, rw.statusCode, time.Since(start))
}

// isMethodAllowed returns true if the HTTP method is allowed (GET or HEAD).
func (s *Server) isMethodAllowed(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

// handleMetrics serves GET /metrics via promhttp.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	
	// For HEAD requests, use a response writer that discards the body
	if r.Method == http.MethodHead {
		headWriter := &headResponseWriter{ResponseWriter: w}
		promhttp.Handler().ServeHTTP(headWriter, r)
		return
	}
	
	promhttp.Handler().ServeHTTP(w, r)
}

// headResponseWriter wraps http.ResponseWriter to discard body for HEAD requests.
type headResponseWriter struct {
	http.ResponseWriter
}

func (w *headResponseWriter) Write(b []byte) (int, error) {
	// Discard body for HEAD requests
	return len(b), nil
}

// handleLogListV3JSON serves GET /logs.v3.json per spec.md FR-006.
func (s *Server) handleLogListV3JSON(w http.ResponseWriter, r *http.Request) {
	if s.logListV3JSON == nil {
		http.Error(w, "Logs.v3.json not initialized", http.StatusInternalServerError)
		return
	}

	// Derive public base URL from request
	publicBaseURL := s.derivePublicBaseURL(r)

	// Get snapshot with URLs set from this request's publicBaseURL
	snap := s.logListV3JSON.GetSnapshotForRequest(publicBaseURL)
	if snap == nil || snap.LastError != nil {
		// Refresh failure behavior per FR-006: return 503
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		//nolint:errcheck // If Write fails after WriteHeader, there's nothing we can do
		_, _ = w.Write([]byte(`{"error":"temporarily unavailable"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(snap); err != nil {
		if s.logger != nil {
			s.logger.Error("Failed to encode logs.v3.json", "error", err)
		}
	}
}

// handleCheckpoint serves GET /<log>/checkpoint per spec.md FR-002, FR-009.
func (s *Server) handleCheckpoint(w http.ResponseWriter, r *http.Request, route Route) {
	if s.zipReader == nil || s.archiveIndex == nil {
		http.Error(w, "Server not fully initialized", http.StatusInternalServerError)
		return
	}

	archiveLog, ok := s.archiveIndex.LookupLog(route.Log)
	if !ok {
		http.NotFound(w, r)
		return
	}

	zipPath := archiveLog.FolderPath + "/000.zip"
	rc, err := s.zipReader.OpenEntry(zipPath, "checkpoint")
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, ErrZipTemporarilyUnavailable) {
			http.Error(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = rc.Close() }()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if r.Method == http.MethodHead {
		return // HEAD: no body
	}

	if _, err := io.Copy(w, rc); err != nil {
		if s.logger != nil {
			s.logger.Error("Failed to write checkpoint response", "log", route.Log, "error", err)
		}
	}
}

// handleLogV3JSON serves GET /<log>/log.v3.json per spec.md FR-002, FR-009.
func (s *Server) handleLogV3JSON(w http.ResponseWriter, r *http.Request, route Route) {
	if s.zipReader == nil || s.archiveIndex == nil {
		http.Error(w, "Server not fully initialized", http.StatusInternalServerError)
		return
	}

	archiveLog, ok := s.archiveIndex.LookupLog(route.Log)
	if !ok {
		http.NotFound(w, r)
		return
	}

	zipPath := archiveLog.FolderPath + "/000.zip"
	rc, err := s.zipReader.OpenEntry(zipPath, "log.v3.json")
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, ErrZipTemporarilyUnavailable) {
			http.Error(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = rc.Close() }()

	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodHead {
		return // HEAD: no body
	}

	if _, err := io.Copy(w, rc); err != nil {
		if s.logger != nil {
			s.logger.Error("Failed to write log.v3.json response", "log", route.Log, "error", err)
		}
	}
}

// handleHashTile serves GET /<log>/tile/<L>/<N>[.p/<W>] per spec.md FR-002, FR-008, FR-008a.
func (s *Server) handleHashTile(w http.ResponseWriter, r *http.Request, route Route) {
	if s.zipReader == nil || s.archiveIndex == nil {
		http.Error(w, "Server not fully initialized", http.StatusInternalServerError)
		return
	}

	archiveLog, ok := s.archiveIndex.LookupLog(route.Log)
	if !ok {
		http.NotFound(w, r)
		return
	}

	// Select zip part for this tile
	zipIndex, ok := s.archiveIndex.SelectZipPart(route.Log, route.TileLevel, route.TileIndex, false)
	if !ok {
		http.NotFound(w, r)
		return
	}

	zipPath := fmt.Sprintf("%s/%03d.zip", archiveLog.FolderPath, zipIndex)
	rc, err := s.zipReader.OpenEntry(zipPath, route.EntryPath)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, ErrZipTemporarilyUnavailable) {
			http.Error(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = rc.Close() }()

	w.Header().Set("Content-Type", "application/octet-stream")
	if r.Method == http.MethodHead {
		return // HEAD: no body
	}

	if _, err := io.Copy(w, rc); err != nil {
		if s.logger != nil {
			s.logger.Error("Failed to write hash tile response", "log", route.Log, "level", route.TileLevel, "index", route.TileIndex, "error", err)
		}
	}
}

// handleDataTile serves GET /<log>/tile/data/<N>[.p/<W>] per spec.md FR-002, FR-008, FR-008a.
func (s *Server) handleDataTile(w http.ResponseWriter, r *http.Request, route Route) {
	if s.zipReader == nil || s.archiveIndex == nil {
		http.Error(w, "Server not fully initialized", http.StatusInternalServerError)
		return
	}

	archiveLog, ok := s.archiveIndex.LookupLog(route.Log)
	if !ok {
		http.NotFound(w, r)
		return
	}

	// Select zip part for this data tile
	zipIndex, ok := s.archiveIndex.SelectZipPart(route.Log, 0, route.TileIndex, true)
	if !ok {
		http.NotFound(w, r)
		return
	}

	zipPath := fmt.Sprintf("%s/%03d.zip", archiveLog.FolderPath, zipIndex)
	rc, err := s.zipReader.OpenEntry(zipPath, route.EntryPath)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, ErrZipTemporarilyUnavailable) {
			http.Error(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = rc.Close() }()

	w.Header().Set("Content-Type", "application/octet-stream")
	if r.Method == http.MethodHead {
		return // HEAD: no body
	}

	if _, err := io.Copy(w, rc); err != nil {
		if s.logger != nil {
			s.logger.Error("Failed to write data tile response", "log", route.Log, "index", route.TileIndex, "error", err)
		}
	}
}

// handleIssuer serves GET /<log>/issuer/<fingerprint> per spec.md FR-002, FR-009.
func (s *Server) handleIssuer(w http.ResponseWriter, r *http.Request, route Route) {
	if s.zipReader == nil || s.archiveIndex == nil {
		http.Error(w, "Server not fully initialized", http.StatusInternalServerError)
		return
	}

	archiveLog, ok := s.archiveIndex.LookupLog(route.Log)
	if !ok {
		http.NotFound(w, r)
		return
	}

	// Issuers are in 000.zip
	zipPath := archiveLog.FolderPath + "/000.zip"
	rc, err := s.zipReader.OpenEntry(zipPath, route.EntryPath)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, ErrZipTemporarilyUnavailable) {
			http.Error(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = rc.Close() }()

	w.Header().Set("Content-Type", "application/pkix-cert")
	if r.Method == http.MethodHead {
		return // HEAD: no body
	}

	if _, err := io.Copy(w, rc); err != nil {
		if s.logger != nil {
			s.logger.Error("Failed to write issuer response", "log", route.Log, "fingerprint", route.IssuerFingerprint, "error", err)
		}
	}
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// logRequest logs HTTP requests per spec.md NFR-010.
// Always logs non-2xx responses. Logs 2xx only when verbose mode is enabled.
func (s *Server) logRequest(r *http.Request, route Route, statusCode int, duration time.Duration) {
	if s.logger == nil {
		return
	}

	// Always log non-2xx responses
	shouldLog := statusCode < 200 || statusCode >= 300
	
	// Log 2xx only when verbose mode is enabled
	if statusCode >= 200 && statusCode < 300 {
		shouldLog = s.verbose
	}

	if !shouldLog {
		return
	}

	attrs := []interface{}{
		"method", r.Method,
		"path", r.URL.Path,
		"status", statusCode,
		"duration_ms", duration.Milliseconds(),
	}

	if route.Log != "" {
		attrs = append(attrs, "log", route.Log)
	}

	// Include X-Forwarded-* headers when present (for logging, not for URL formation)
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		attrs = append(attrs, "x_forwarded_host", fwdHost)
	}
	if fwdProto := r.Header.Get("X-Forwarded-Proto"); fwdProto != "" {
		attrs = append(attrs, "x_forwarded_proto", fwdProto)
	}

	switch {
	case statusCode >= 500:
		s.logger.Error("HTTP request", attrs...)
	case statusCode >= 400:
		s.logger.Warn("HTTP request", attrs...)
	default:
		s.logger.Info("HTTP request", attrs...)
	}
}

// derivePublicBaseURL derives the public base URL from the incoming request per spec.md FR-006.
//
// It uses Host header by default. If CT_HTTP_TRUSTED_SOURCES is set and the request source IP
// matches a trusted source, it uses X-Forwarded-Host/X-Forwarded-Proto. Otherwise, it ignores
// X-Forwarded-* headers and uses Host/http.
func (s *Server) derivePublicBaseURL(r *http.Request) string {
	// Extract source IP from RemoteAddr (format: "IP:port")
	sourceIPStr, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// Fallback: treat as untrusted if we can't parse
		sourceIPStr = r.RemoteAddr
	}

	sourceIP, err := netip.ParseAddr(sourceIPStr)
	if err != nil {
		// Fallback: treat as untrusted if we can't parse
		sourceIP = netip.Addr{}
	}

	// Check if source IP is trusted
	isTrusted := false
	for _, prefix := range s.cfg.HTTPTrustedSources {
		if prefix.Contains(sourceIP) {
			isTrusted = true
			break
		}
	}

	// Determine host
	var host string
	if isTrusted {
		if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
			host = firstNonEmptyAfterTrim(strings.Split(fwdHost, ","))
		}
	}
	if host == "" {
		host = r.Host
	}

	// Determine scheme
	var scheme string
	if isTrusted {
		if fwdProto := r.Header.Get("X-Forwarded-Proto"); fwdProto != "" {
			scheme = firstNonEmptyAfterTrim(strings.Split(fwdProto, ","))
		}
	}
	if scheme == "" {
		scheme = "http"
	}
	scheme = strings.ToLower(scheme)

	return scheme + "://" + host
}// firstNonEmptyAfterTrim returns the first non-empty element after trimming ASCII whitespace.
func firstNonEmptyAfterTrim(elems []string) string {
	for _, elem := range elems {
		trimmed := strings.TrimSpace(elem)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}