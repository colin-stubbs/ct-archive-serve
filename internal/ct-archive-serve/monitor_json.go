package ctarchiveserve

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// LogV3Entry represents a log entry from log.v3.json.
type LogV3Entry struct {
	Description string                 `json:"description"`
	LogID       string                 `json:"log_id"`
	Key         string                 `json:"key"`
	MMD         int                    `json:"mmd"`
	LogType     string                 `json:"log_type"`
	State       map[string]interface{} `json:"state"`
	URL         string                 `json:"url,omitempty"` // Will be removed per FR-006b
}

// LogListV3JSONSnapshot is an immutable snapshot of the logs.v3.json state.
type LogListV3JSONSnapshot struct {
	Version          string                 `json:"version"`
	LogListTimestamp string                 `json:"log_list_timestamp"`
	Operators        []LogListV3JSONOperator  `json:"operators"`
	LastError        error                  `json:"-"` // Internal: tracks refresh failure state (not in JSON)
}

// LogListV3JSONOperator represents the single operator in loglist v3 JSON.
type LogListV3JSONOperator struct {
	Name      string            `json:"name"`
	Email     []string          `json:"email"`
	Logs      []interface{}     `json:"logs"`
	TiledLogs []LogListV3JSONTiledLog `json:"tiled_logs"`
}

// LogListV3JSONTiledLog represents a tiled log entry in logs.v3.json.
type LogListV3JSONTiledLog struct {
	Description    string                 `json:"description"`
	LogID          string                 `json:"log_id"`
	Key            string                 `json:"key"`
	MMD            int                    `json:"mmd"`
	LogType        string                 `json:"log_type"`
	State          map[string]interface{} `json:"state"`
	SubmissionURL  string                 `json:"submission_url"`
	MonitoringURL  string                 `json:"monitoring_url"`
	HasIssuers     bool                   `json:"has_issuers"`
	LogName        string                 `json:"-"` // Internal: log name for URL construction
}

// zipFileCacheEntry stores cached data for a zip file along with its modification time.
type zipFileCacheEntry struct {
	mtime      time.Time
	logV3Entry *LogV3Entry
	hasIssuers bool
}

// LogListV3JSONBuilder builds logs.v3.json snapshots from discovered archives.
type LogListV3JSONBuilder struct {
	zipReader    *ZipReader
	archiveIndex *ArchiveIndex
	logger       *slog.Logger
	cfg          Config

	snap atomic.Value // stores *LogListV3JSONSnapshot

	// refreshMu serializes refresh operations to prevent concurrent refreshes
	// (e.g., if a refresh takes longer than the refresh interval)
	refreshMu sync.Mutex

	// zipCache stores cached log.v3.json data keyed by zip file path.
	// Protected by refreshMu (only accessed during refresh operations).
	zipCache map[string]zipFileCacheEntry
}

// NewLogListV3JSONBuilder constructs a new LogListV3JSONBuilder.
func NewLogListV3JSONBuilder(
	cfg Config,
	zipReader *ZipReader,
	archiveIndex *ArchiveIndex,
	logger *slog.Logger,
) *LogListV3JSONBuilder {
	return &LogListV3JSONBuilder{
		zipReader:    zipReader,
		archiveIndex: archiveIndex,
		logger:       logger,
		cfg:          cfg,
		zipCache:     make(map[string]zipFileCacheEntry),
	}
}

// GetSnapshot returns the current loglist v3 JSON snapshot.
func (b *LogListV3JSONBuilder) GetSnapshot() *LogListV3JSONSnapshot {
	if b == nil {
		return nil
	}
	val := b.snap.Load()
	if val == nil {
		return nil
	}
	snap, ok := val.(*LogListV3JSONSnapshot)
	if !ok {
		// This should never happen - atomic.Value only stores *LogListV3JSONSnapshot
		panic("loglist v3 JSON builder: invalid type in atomic.Value")
	}
	return snap
}

// extractLogV3JSONAndCheckIssuers opens a zip part once and performs both operations:
// extracts/parses log.v3.json and checks for issuer/ entries. This avoids opening
// the same ZIP file twice, which is expensive for large ZIPs with many entries.
// It uses mtime-based caching to avoid re-reading unchanged zip files.
func (b *LogListV3JSONBuilder) extractLogV3JSONAndCheckIssuers(zipPath string) (*LogV3Entry, bool, error) {
	// Check mtime to see if we can use cached data
	stat, err := os.Stat(zipPath)
	if err != nil {
		return nil, false, fmt.Errorf("stat zip: %w", err)
	}

	// Check cache (protected by refreshMu, which is held by caller)
	if cached, ok := b.zipCache[zipPath]; ok {
		if cached.mtime.Equal(stat.ModTime()) {
			// mtime matches, use cached data
			if b.logger != nil {
				b.logger.Debug("Using cached log.v3.json data (mtime unchanged)", "zip_path", zipPath)
			}
			// Return a copy of the cached entry to avoid sharing mutable state
			entryCopy := *cached.logV3Entry
			return &entryCopy, cached.hasIssuers, nil
		}
		// mtime changed, remove from cache and re-read
		if b.logger != nil {
			b.logger.Debug("Zip file mtime changed, re-reading", "zip_path", zipPath, "old_mtime", cached.mtime, "new_mtime", stat.ModTime())
		}
		delete(b.zipCache, zipPath)
	}

	// Read from zip file
	if b.logger != nil {
		b.logger.Debug("Opening zip file for log.v3.json extraction and issuer check", "zip_path", zipPath)
	}
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, false, fmt.Errorf("open zip: %w", err)
	}
	defer func() { _ = r.Close() }()

	if b.logger != nil {
		b.logger.Debug("Scanning zip entries", "zip_path", zipPath, "entry_count", len(r.File))
	}

	var logV3File *zip.File
	hasIssuers := false
	issuerLogged := false

	for _, f := range r.File {
		if f.Name == "log.v3.json" {
			logV3File = f
		} else if strings.HasPrefix(f.Name, "issuer/") {
			hasIssuers = true
			// Only log the first issuer entry found to reduce verbosity
			if b.logger != nil && !issuerLogged {
				b.logger.Debug("Found issuer entry", "zip_path", zipPath, "entry", f.Name)
				issuerLogged = true
			}
		}
	}

	if logV3File == nil {
		return nil, hasIssuers, errors.New("log.v3.json not found in zip")
	}

	if b.logger != nil {
		b.logger.Debug("Reading log.v3.json from zip", "zip_path", zipPath)
	}
	rc, err := logV3File.Open()
	if err != nil {
		return nil, hasIssuers, fmt.Errorf("open log.v3.json: %w", err)
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, hasIssuers, fmt.Errorf("read log.v3.json: %w", err)
	}

	var entry LogV3Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, hasIssuers, fmt.Errorf("parse log.v3.json: %w", err)
	}

	// Cache the result
	b.zipCache[zipPath] = zipFileCacheEntry{
		mtime:      stat.ModTime(),
		logV3Entry: &entry,
		hasIssuers: hasIssuers,
	}

	if b.logger != nil {
		b.logger.Debug("Successfully extracted and parsed log.v3.json", "zip_path", zipPath)
		if !hasIssuers {
			b.logger.Debug("No issuer entries found", "zip_path", zipPath)
		}
	}
	return &entry, hasIssuers, nil
}

// extractLogV3JSON extracts and parses log.v3.json from a zip part.
//
// Deprecated: Use extractLogV3JSONAndCheckIssuers to avoid opening ZIP twice.
func (b *LogListV3JSONBuilder) extractLogV3JSON(zipPath string) (*LogV3Entry, error) {
	entry, _, err := b.extractLogV3JSONAndCheckIssuers(zipPath)
	return entry, err
}

// checkHasIssuers checks if a zip part contains any issuer/ entries (metadata-only check).
//
// Deprecated: Use extractLogV3JSONAndCheckIssuers to avoid opening ZIP twice.
func (b *LogListV3JSONBuilder) checkHasIssuers(zipPath string) (bool, error) {
	_, hasIssuers, err := b.extractLogV3JSONAndCheckIssuers(zipPath)
	return hasIssuers, err
}

// BuildSnapshot builds a new logs.v3.json snapshot from the current archive index state.
// The publicBaseURL is used to set submission_url and monitoring_url per spec.md FR-006.
func (b *LogListV3JSONBuilder) BuildSnapshot(publicBaseURL string) (*LogListV3JSONSnapshot, error) {
	if b.archiveIndex == nil {
		return nil, errors.New("archive index not initialized")
	}

	snap := b.archiveIndex.GetAllLogs()

	if b.logger != nil {
		b.logger.Debug("Building logs.v3.json snapshot", "log_count", len(snap.Logs))
	}

	var tiledLogs []LogListV3JSONTiledLog
	logNames := make([]string, 0, len(snap.Logs))
	for logName := range snap.Logs {
		logNames = append(logNames, logName)
	}
	sort.Strings(logNames) // Deterministic sort per FR-006

	for i, logName := range logNames {
		log := snap.Logs[logName]
		zipPath := log.FolderPath + "/000.zip"

		if b.logger != nil {
			b.logger.Debug("Processing log for logs.v3.json", "log", logName, "progress", fmt.Sprintf("%d/%d", i+1, len(logNames)), "zip_path", zipPath)
		}

		// Extract log.v3.json and check for issuer entries in a single ZIP open
		if b.logger != nil {
			b.logger.Debug("Extracting log.v3.json and checking for issuer entries", "log", logName, "zip_path", zipPath)
		}
		logV3, hasIssuers, err := b.extractLogV3JSONAndCheckIssuers(zipPath)
		if err != nil {
			if b.logger != nil {
				b.logger.Warn("Failed to extract log.v3.json or check issuers", "log", logName, "error", err)
			}
			continue // Skip this log
		}
		if b.logger != nil {
			b.logger.Debug("Extracted log.v3.json and checked issuers", "log", logName, "description", logV3.Description, "has_issuers", hasIssuers)
		}

		// Build tiled log entry (remove url, add submission_url/monitoring_url per FR-006b)
		tiledLog := LogListV3JSONTiledLog{
			Description:   logV3.Description,
			LogID:         logV3.LogID,
			Key:           logV3.Key,
			MMD:           logV3.MMD,
			LogType:       logV3.LogType,
			State:         logV3.State,
			SubmissionURL: publicBaseURL + "/" + logName,
			MonitoringURL: publicBaseURL + "/" + logName,
			HasIssuers:    hasIssuers,
			LogName:       logName, // Store for per-request URL updates
		}

		tiledLogs = append(tiledLogs, tiledLog)
		if b.logger != nil {
			b.logger.Debug("Added log to loglist v3 JSON snapshot", "log", logName, "has_issuers", hasIssuers)
		}
	}

	// Clean up cache entries for logs that are no longer in the archive index
	// Build a set of current zip paths
	currentZipPaths := make(map[string]bool, len(snap.Logs))
	for _, log := range snap.Logs {
		currentZipPaths[log.FolderPath+"/000.zip"] = true
	}

	// Remove cache entries for zip files that no longer exist in the archive
	for zipPath := range b.zipCache {
		if !currentZipPaths[zipPath] {
			if b.logger != nil {
				b.logger.Debug("Removing cache entry for removed log", "zip_path", zipPath)
			}
			delete(b.zipCache, zipPath)
		}
	}

	if b.logger != nil {
		b.logger.Debug("Logs.v3.json snapshot build complete", "tiled_log_count", len(tiledLogs))
	}

	return &LogListV3JSONSnapshot{
		Version:          "3.0",
		LogListTimestamp: time.Now().UTC().Format(time.RFC3339),
		Operators: []LogListV3JSONOperator{
			{
				Name:      "ct-archive-serve",
				Email:     []string{},
				Logs:      []interface{}{},
				TiledLogs: tiledLogs,
			},
		},
		LastError: nil,
	}, nil
}

// Start begins the periodic refresh loop for logs.v3.json.
// It performs an initial refresh at startup, then refreshes on CT_LOGLISTV3_JSON_REFRESH_INTERVAL.
// Note: publicBaseURL is a placeholder for the refresh loop; actual URLs are set per-request.
func (b *LogListV3JSONBuilder) Start(ctx context.Context) {
	if b == nil {
		return
	}

	// Initial refresh at startup (using placeholder URL; will be overridden per-request)
	if b.logger != nil {
		b.logger.Debug("Starting initial logs.v3.json refresh")
	}
	b.refreshOnce("http://placeholder")
	if b.logger != nil {
		b.logger.Debug("Initial loglist v3 JSON refresh completed")
	}

	// Periodic refresh loop
	t := time.NewTicker(b.cfg.LogListV3JSONRefreshInterval)
	go func() {
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				// Refresh with placeholder; actual URLs set per-request
				b.refreshOnce("http://placeholder")
			}
		}
	}()
}

// refreshOnce attempts to build a new snapshot and update the atomic value.
// On success, LastError is nil. On failure, LastError is set and the snapshot may be nil.
// This method is protected by refreshMu to prevent concurrent refreshes.
func (b *LogListV3JSONBuilder) refreshOnce(publicBaseURL string) {
	b.refreshMu.Lock()
	defer b.refreshMu.Unlock()

	snap, err := b.BuildSnapshot(publicBaseURL)
	if err != nil {
		if b.logger != nil {
			b.logger.Error("Logs.v3.json refresh failed", "error", err)
		}
		// Store snapshot with error state (will cause 503 responses per FR-006)
		// Always store a snapshot, even on error, so GetSnapshot can return error state
		if snap == nil {
			snap = &LogListV3JSONSnapshot{
				Version:          "3.0",
				LogListTimestamp: time.Now().UTC().Format(time.RFC3339),
				Operators:        []LogListV3JSONOperator{{Name: "ct-archive-serve", Email: []string{}, Logs: []interface{}{}, TiledLogs: []LogListV3JSONTiledLog{}}},
				LastError:        err,
			}
		} else {
			snap.LastError = err
		}
	}
	b.snap.Store(snap)
}

// GetSnapshotForRequest returns a snapshot with URLs set from the request's publicBaseURL.
func (b *LogListV3JSONBuilder) GetSnapshotForRequest(publicBaseURL string) *LogListV3JSONSnapshot {
	if b == nil {
		return nil
	}
	snap := b.GetSnapshot()
	if snap == nil || snap.LastError != nil {
		return snap // Return as-is (will result in 503)
	}	// Clone snapshot and update URLs per request
	clone := *snap
	if len(clone.Operators) > 0 && len(clone.Operators[0].TiledLogs) > 0 {
		clone.Operators = make([]LogListV3JSONOperator, len(snap.Operators))
		for i, op := range snap.Operators {
			clone.Operators[i] = LogListV3JSONOperator{
				Name:      op.Name,
				Email:     op.Email,
				Logs:      op.Logs,
				TiledLogs: make([]LogListV3JSONTiledLog, len(op.TiledLogs)),
			}
			for j, tlog := range op.TiledLogs {
				// Update URLs using stored log name
				clone.Operators[i].TiledLogs[j] = LogListV3JSONTiledLog{
					Description:   tlog.Description,
					LogID:        tlog.LogID,
					Key:          tlog.Key,
					MMD:          tlog.MMD,
					LogType:      tlog.LogType,
					State:        tlog.State,
					SubmissionURL: publicBaseURL + "/" + tlog.LogName,
					MonitoringURL: publicBaseURL + "/" + tlog.LogName,
					HasIssuers:   tlog.HasIssuers,
					LogName:      tlog.LogName,
				}
			}
		}
	}
	return &clone
}