package ctarchiveserve

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
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

// MonitorJSONSnapshot is an immutable snapshot of the monitor.json state.
type MonitorJSONSnapshot struct {
	Version          string                 `json:"version"`
	LogListTimestamp string                 `json:"log_list_timestamp"`
	Operators        []MonitorJSONOperator  `json:"operators"`
	LastError        error                  `json:"-"` // Internal: tracks refresh failure state (not in JSON)
}

// MonitorJSONOperator represents the single operator in monitor.json.
type MonitorJSONOperator struct {
	Name      string            `json:"name"`
	Email     []string          `json:"email"`
	Logs      []interface{}     `json:"logs"`
	TiledLogs []MonitorJSONTiledLog `json:"tiled_logs"`
}

// MonitorJSONTiledLog represents a tiled log entry in monitor.json.
type MonitorJSONTiledLog struct {
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

// MonitorJSONBuilder builds monitor.json snapshots from discovered archives.
type MonitorJSONBuilder struct {
	zipReader    *ZipReader
	archiveIndex *ArchiveIndex
	logger       *slog.Logger
	cfg          Config

	snap atomic.Value // stores *MonitorJSONSnapshot
}

// NewMonitorJSONBuilder constructs a new MonitorJSONBuilder.
func NewMonitorJSONBuilder(
	cfg Config,
	zipReader *ZipReader,
	archiveIndex *ArchiveIndex,
	logger *slog.Logger,
) *MonitorJSONBuilder {
	return &MonitorJSONBuilder{
		zipReader:    zipReader,
		archiveIndex: archiveIndex,
		logger:       logger,
		cfg:          cfg,
	}
}

// GetSnapshot returns the current monitor.json snapshot.
func (b *MonitorJSONBuilder) GetSnapshot() *MonitorJSONSnapshot {
	if b == nil {
		return nil
	}
	val := b.snap.Load()
	if val == nil {
		return nil
	}
	return val.(*MonitorJSONSnapshot)
}

// extractLogV3JSON extracts and parses log.v3.json from a zip part.
func (b *MonitorJSONBuilder) extractLogV3JSON(zipPath string) (*LogV3Entry, error) {
	rc, err := b.zipReader.OpenEntry(zipPath, "log.v3.json")
	if err != nil {
		return nil, fmt.Errorf("open log.v3.json: %w", err)
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read log.v3.json: %w", err)
	}

	var entry LogV3Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("parse log.v3.json: %w", err)
	}

	return &entry, nil
}

// checkHasIssuers checks if a zip part contains any issuer/ entries (metadata-only check).
func (b *MonitorJSONBuilder) checkHasIssuers(zipPath string) (bool, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return false, fmt.Errorf("open zip: %w", err)
	}
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		if strings.HasPrefix(f.Name, "issuer/") {
			return true, nil
		}
	}
	return false, nil
}

// BuildSnapshot builds a new monitor.json snapshot from the current archive index state.
// The publicBaseURL is used to set submission_url and monitoring_url per spec.md FR-006.
func (b *MonitorJSONBuilder) BuildSnapshot(publicBaseURL string) (*MonitorJSONSnapshot, error) {
	if b.archiveIndex == nil {
		return nil, fmt.Errorf("archive index not initialized")
	}

	snap := b.archiveIndex.GetAllLogs()

	var tiledLogs []MonitorJSONTiledLog
	logNames := make([]string, 0, len(snap.Logs))
	for logName := range snap.Logs {
		logNames = append(logNames, logName)
	}
	sort.Strings(logNames) // Deterministic sort per FR-006

	for _, logName := range logNames {
		log := snap.Logs[logName]
		zipPath := fmt.Sprintf("%s/000.zip", log.FolderPath)

		// Extract log.v3.json
		logV3, err := b.extractLogV3JSON(zipPath)
		if err != nil {
			if b.logger != nil {
				b.logger.Warn("Failed to extract log.v3.json", "log", logName, "error", err)
			}
			continue // Skip this log
		}

		// Check has_issuers
		hasIssuers, err := b.checkHasIssuers(zipPath)
		if err != nil {
			if b.logger != nil {
				b.logger.Warn("Failed to check has_issuers", "log", logName, "error", err)
			}
			hasIssuers = false // Default to false on error
		}

		// Build tiled log entry (remove url, add submission_url/monitoring_url per FR-006b)
		tiledLog := MonitorJSONTiledLog{
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
	}

	return &MonitorJSONSnapshot{
		Version:          "3.0",
		LogListTimestamp: time.Now().UTC().Format(time.RFC3339),
		Operators: []MonitorJSONOperator{
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

// Start begins the periodic refresh loop for monitor.json.
// It performs an initial refresh at startup, then refreshes on CT_MONITOR_JSON_REFRESH_INTERVAL.
// Note: publicBaseURL is a placeholder for the refresh loop; actual URLs are set per-request.
func (b *MonitorJSONBuilder) Start(ctx context.Context) {
	if b == nil {
		return
	}

	// Initial refresh at startup (using placeholder URL; will be overridden per-request)
	b.refreshOnce("http://placeholder")

	// Periodic refresh loop
	t := time.NewTicker(b.cfg.MonitorJSONRefreshInterval)
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
func (b *MonitorJSONBuilder) refreshOnce(publicBaseURL string) {
	snap, err := b.BuildSnapshot(publicBaseURL)
	if err != nil {
		if b.logger != nil {
			b.logger.Error("Monitor.json refresh failed", "error", err)
		}
		// Store snapshot with error state (will cause 503 responses per FR-006)
		// Always store a snapshot, even on error, so GetSnapshot can return error state
		if snap == nil {
			snap = &MonitorJSONSnapshot{
				Version:          "3.0",
				LogListTimestamp: time.Now().UTC().Format(time.RFC3339),
				Operators:        []MonitorJSONOperator{{Name: "ct-archive-serve", Email: []string{}, Logs: []interface{}{}, TiledLogs: []MonitorJSONTiledLog{}}},
				LastError:        err,
			}
		} else {
			snap.LastError = err
		}
	}
	b.snap.Store(snap)
}

// GetSnapshotForRequest returns a snapshot with URLs set from the request's publicBaseURL.
func (b *MonitorJSONBuilder) GetSnapshotForRequest(publicBaseURL string) *MonitorJSONSnapshot {
	if b == nil {
		return nil
	}
	snap := b.GetSnapshot()
	if snap == nil || snap.LastError != nil {
		return snap // Return as-is (will result in 503)
	}

	// Clone snapshot and update URLs per request
	clone := *snap
	if len(clone.Operators) > 0 && len(clone.Operators[0].TiledLogs) > 0 {
		clone.Operators = make([]MonitorJSONOperator, len(snap.Operators))
		for i, op := range snap.Operators {
			clone.Operators[i] = MonitorJSONOperator{
				Name:      op.Name,
				Email:     op.Email,
				Logs:      op.Logs,
				TiledLogs: make([]MonitorJSONTiledLog, len(op.TiledLogs)),
			}
			for j, tlog := range op.TiledLogs {
				// Update URLs using stored log name
				clone.Operators[i].TiledLogs[j] = MonitorJSONTiledLog{
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