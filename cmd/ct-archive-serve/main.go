package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	ctarchiveserve "ct-archive-serve/internal/ct-archive-serve"

	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	var (
		help    = flag.Bool("h", false, "Show help")
		helpLong = flag.Bool("help", false, "Show help")
		verbose = flag.Bool("v", false, "Enable verbose logging (log successful HTTP requests)")
		verboseLong = flag.Bool("verbose", false, "Enable verbose logging (log successful HTTP requests)")
		debug   = flag.Bool("d", false, "Enable debug logging")
		debugLong = flag.Bool("debug", false, "Enable debug logging")
	)
	flag.Parse()

	if *help || *helpLong {
		// Help output to stdout - if this fails, the program is in a bad state anyway
		_, _ = fmt.Fprintf(os.Stdout, "Usage: %s [flags]\n\n", os.Args[0])
		_, _ = fmt.Fprintf(os.Stdout, "Flags:\n")
		flag.PrintDefaults()
		_, _ = fmt.Fprintf(os.Stdout, "\nEnvironment Variables:\n\n")
		_, _ = fmt.Fprintf(os.Stdout, "Archive Configuration:\n")
		_, _ = fmt.Fprintf(os.Stdout, "  CT_ARCHIVE_PATH\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Path to the archive directory containing log folders (default: /var/log/ct/archive)\n\n")
		_, _ = fmt.Fprintf(os.Stdout, "  CT_ARCHIVE_FOLDER_PATTERN\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Glob pattern for matching log folders, must end with '*' (default: ct_*)\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Example: ct_* matches folders like ct_digicert_nessie_2022/\n\n")
		_, _ = fmt.Fprintf(os.Stdout, "Refresh Intervals:\n")
		_, _ = fmt.Fprintf(os.Stdout, "  CT_LOGLISTV3_JSON_REFRESH_INTERVAL\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Interval for refreshing /logs.v3.json (default: 10m)\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Format: Go duration (e.g., 10m, 5m, 30s, 1h)\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Optimized for large archive sets (100+ logs, 10TB+ data)\n\n")
		_, _ = fmt.Fprintf(os.Stdout, "  CT_ARCHIVE_REFRESH_INTERVAL\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Interval for refreshing archive index (default: 5m)\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Format: Go duration (e.g., 5m, 1m, 30s)\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Optimized for large archive sets to reduce disk I/O\n\n")
		_, _ = fmt.Fprintf(os.Stdout, "Zip Cache Configuration:\n")
		_, _ = fmt.Fprintf(os.Stdout, "  CT_ZIP_CACHE_MAX_OPEN\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Maximum number of open zip parts to cache (default: 256)\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Higher values improve performance for hot zip parts but increase memory usage\n\n")
		_, _ = fmt.Fprintf(os.Stdout, "  CT_ZIP_INTEGRITY_FAIL_TTL\n")
		_, _ = fmt.Fprintf(os.Stdout, "    TTL for failed zip integrity checks (default: 5m)\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Failed zip parts are re-tested after this interval\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Format: Go duration (e.g., 5m, 1m, 10m)\n\n")
		_, _ = fmt.Fprintf(os.Stdout, "HTTP Server Configuration:\n")
		_, _ = fmt.Fprintf(os.Stdout, "  CT_HTTP_READ_HEADER_TIMEOUT\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Maximum time to read request headers (default: 5s)\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Format: Go duration (e.g., 5s, 10s, 0 to disable)\n\n")
		_, _ = fmt.Fprintf(os.Stdout, "  CT_HTTP_IDLE_TIMEOUT\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Maximum idle connection timeout (default: 60s)\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Format: Go duration (e.g., 60s, 2m, 0 to disable)\n\n")
		_, _ = fmt.Fprintf(os.Stdout, "  CT_HTTP_MAX_HEADER_BYTES\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Maximum size of request headers in bytes (default: 8192)\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Must be > 0\n\n")
		_, _ = fmt.Fprintf(os.Stdout, "  CT_HTTP_WRITE_TIMEOUT\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Maximum time to write response (default: 0, disabled)\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Format: Go duration (e.g., 30s, 1m, 0 to disable)\n\n")
		_, _ = fmt.Fprintf(os.Stdout, "  CT_HTTP_READ_TIMEOUT\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Maximum time to read request body (default: 0, disabled)\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Format: Go duration (e.g., 30s, 1m, 0 to disable)\n\n")
		_, _ = fmt.Fprintf(os.Stdout, "  CT_HTTP_TRUSTED_SOURCES\n")
		_, _ = fmt.Fprintf(os.Stdout, "    CSV list of trusted IP addresses or CIDR networks for X-Forwarded-* headers\n")
		_, _ = fmt.Fprintf(os.Stdout, "    If set, X-Forwarded-Host and X-Forwarded-Proto are trusted when request\n")
		_, _ = fmt.Fprintf(os.Stdout, "    source IP matches. If unset or empty, X-Forwarded-* headers are ignored.\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Format: comma-separated IPs or CIDRs (e.g., 127.0.0.1/32,10.0.0.0/8)\n")
		_, _ = fmt.Fprintf(os.Stdout, "    Example: 127.0.0.1/32,10.0.0.0/8,172.16.0.0/12\n\n")
		_, _ = fmt.Fprintf(os.Stdout, "For more details, see README.md\n")
		os.Exit(0)
	}

	verboseEnabled := *verbose || *verboseLong
	debugEnabled := *debug || *debugLong

	// Load configuration from environment
	logger := ctarchiveserve.NewLogger(ctarchiveserve.LoggerOptions{
		Verbose: verboseEnabled,
		Debug:   debugEnabled,
	})
	logger.Debug("Loading configuration from environment")
	cfg, err := ctarchiveserve.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	logger.Debug("Configuration loaded", "archive_path", cfg.ArchivePath, "folder_pattern", cfg.ArchiveFolderPattern)

	// Initialize metrics
	logger.Debug("Initializing metrics")
	reg := prometheus.NewRegistry()
	metrics := ctarchiveserve.NewMetrics(reg)

	// Initialize archive index
	logger.Debug("Initializing archive index", "archive_path", cfg.ArchivePath)
	archiveIndex, err := ctarchiveserve.NewArchiveIndex(cfg, logger, metrics)
	if err != nil {
		logger.Error("Failed to initialize archive index", "error", err)
		os.Exit(1)
	}
	logger.Debug("Archive index initialized")

	// Start archive index refresh loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger.Debug("Starting archive index refresh loop", "interval", cfg.ArchiveRefreshInterval)
	archiveIndex.Start(ctx)

	// Initialize zip integrity cache
	logger.Debug("Initializing zip integrity cache", "fail_ttl", cfg.ZipIntegrityFailTTL)
	zipIntegrityCache := ctarchiveserve.NewZipIntegrityCache(
		cfg.ZipIntegrityFailTTL,
		time.Now,
		nil, // use default verify function
		metrics,
	)

	// Initialize zip part cache (Phase 5 performance optimization)
	logger.Debug("Initializing zip part cache", "max_open", cfg.ZipCacheMaxOpen)
	zipPartCache := ctarchiveserve.NewZipPartCache(cfg.ZipCacheMaxOpen, metrics)

	// Initialize zip reader
	logger.Debug("Initializing zip reader")
	zipReader := ctarchiveserve.NewZipReader(zipIntegrityCache)
	zipReader.SetZipPartCache(zipPartCache)

	// Initialize logs.v3.json builder
	logger.Debug("Initializing logs.v3.json builder")
	logListV3JSON := ctarchiveserve.NewLogListV3JSONBuilder(cfg, zipReader, archiveIndex, logger)

	// Start logs.v3.json refresh loop (URLs set per-request)
	logger.Debug("Starting logs.v3.json refresh loop", "interval", cfg.LogListV3JSONRefreshInterval)
	logger.Debug("Performing initial logs.v3.json refresh (this may take time with many archives)")
	logListV3JSON.Start(ctx)
	logger.Debug("Logs.v3.json initial refresh completed")

	// Create HTTP server
	logger.Debug("Creating HTTP server")
	server := ctarchiveserve.NewServer(cfg, logger, metrics, archiveIndex, zipReader, logListV3JSON)
	server.SetVerbose(verboseEnabled)

	// Configure http.Server with timeouts and limits per spec.md FR-012
	httpServer := &http.Server{
		Addr:              ":8080",
		Handler:           server,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
		MaxHeaderBytes:    cfg.HTTPMaxHeaderBytes,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		ReadTimeout:       cfg.HTTPReadTimeout,
	}
	logger.Debug("HTTP server configured", "addr", httpServer.Addr)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logger.Info("Received signal, shutting down", "signal", sig.String())
		cancel() // Stop archive index refresh

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("Error during server shutdown", "error", err)
		}
	}()

	logger.Info("Starting ct-archive-serve", "addr", httpServer.Addr)
	logger.Debug("Attempting to bind HTTP listener", "addr", httpServer.Addr)

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("Server error", "error", err)
		//nolint:gocritic // exitAfterDefer: os.Exit is intentional here for fatal server errors
		// The defer cancel() above is for graceful shutdown, but if ListenAndServe fails
		// during startup, we exit immediately rather than attempting shutdown.
		os.Exit(1)
	}

	logger.Info("Server stopped")
}
