package ctarchiveserve

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ArchiveSnapshot is an immutable view of the currently discovered archive state.
type ArchiveSnapshot struct {
	Logs map[string]ArchiveLog
}

// ArchiveLog describes one discovered log folder under CT_ARCHIVE_PATH.
type ArchiveLog struct {
	Log        string
	FolderName string
	FolderPath string

	// ZipParts are the discovered `NNN.zip` indices for this log, sorted ascending.
	ZipParts []int

	// FirstDiscovered is the timestamp when this log was first discovered (when 000.zip was first found).
	// This is used to set the "retired" state timestamp in logs.v3.json.
	FirstDiscovered time.Time
}

// ArchiveIndex maintains an in-memory view of discovered logs and zip parts.
//
// The request hot path MUST consult this in-memory snapshot and MUST NOT rescan disk.
type ArchiveIndex struct {
	cfg     Config
	readDir func(string) ([]os.DirEntry, error)

	logger  *slog.Logger
	metrics *Metrics

	snap atomic.Value // stores ArchiveSnapshot

	// refreshMu serializes refresh operations to prevent concurrent disk scans
	// (e.g., if a refresh takes longer than the refresh interval)
	refreshMu sync.Mutex
}

func NewArchiveIndex(cfg Config, logger *slog.Logger, metrics *Metrics) (*ArchiveIndex, error) {
	ai := &ArchiveIndex{
		cfg:     cfg,
		readDir: os.ReadDir,
		logger:  logger,
		metrics: metrics,
	}

	if logger != nil {
		logger.Debug("Building initial archive snapshot", "archive_path", cfg.ArchivePath, "folder_pattern", cfg.ArchiveFolderPrefix+"*")
	}
	snap, err := buildArchiveSnapshot(cfg, ai.readDir, logger, nil)
	if err != nil {
		return nil, err
	}
	if logger != nil {
		logger.Debug("Archive snapshot built", "log_count", len(snap.Logs))
	}
	ai.snap.Store(snap)
	ai.updateResourceMetrics(snap)

	return ai, nil
}

func (ai *ArchiveIndex) Start(ctx context.Context) {
	if ai == nil {
		return
	}

	t := time.NewTicker(ai.cfg.ArchiveRefreshInterval)
	go func() {
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				ai.refreshOnce()
			}
		}
	}()
}

func (ai *ArchiveIndex) LookupLog(log string) (ArchiveLog, bool) {
	if ai == nil {
		return ArchiveLog{}, false
	}
	val := ai.snap.Load()
	if val == nil {
		return ArchiveLog{}, false
	}
	snap, ok := val.(ArchiveSnapshot)
	if !ok {
		// This should never happen - atomic.Value only stores ArchiveSnapshot
		panic("archive index: invalid type in atomic.Value")
	}
	l, ok := snap.Logs[log]
	return l, ok
}

// SelectZipPart selects the appropriate zip part index for a tile request per spec.md FR-008.
//
// For hash tiles at level L with index N:
//   - L <= 2: zipIndex = floor(N / 256^(3-L))
//   - L >= 3: prefer 000.zip (shared metadata), else lowest available zip
//
// For data tiles with index N:
//   - zipIndex = N / 65536
//
// Returns the zip part index and true if found, or 0 and false if not available.
func (ai *ArchiveIndex) SelectZipPart(log string, tileLevel uint8, tileIndex uint64, isDataTile bool) (int, bool) {
	if ai == nil {
		return 0, false
	}

	archiveLog, ok := ai.LookupLog(log)
	if !ok {
		return 0, false
	}

	var zipIndex int
	if isDataTile {
		// Data tiles: zipIndex = N / 65536
		// Check if division result would overflow int before conversion
		divResult := tileIndex / 65536
		if divResult > math.MaxInt {
			return 0, false
		}
		zipIndex = int(divResult)
	} else {
		// Hash tiles
		if tileLevel <= 2 {
			// L=0: zipIndex = N / 65536 (256^2)
			// L=1: zipIndex = N / 256 (256^1)
			// L=2: zipIndex = N (256^0)
			switch tileLevel {
			case 0:
				// Check if division result would overflow int before conversion
				divResult := tileIndex / 65536
				if divResult > math.MaxInt {
					return 0, false
				}
				zipIndex = int(divResult) // 256^2
			case 1:
				// Check if division result would overflow int before conversion
				divResult := tileIndex / 256
				if divResult > math.MaxInt {
					return 0, false
				}
				zipIndex = int(divResult) // 256^1
			case 2:
				// Check for overflow before conversion
				if tileIndex > math.MaxInt {
					return 0, false
				}
				zipIndex = int(tileIndex) // 256^0
			}
		} else {
			// L >= 3: prefer 000.zip, else lowest available zip
			if len(archiveLog.ZipParts) == 0 {
				return 0, false
			}
			// Check if 000.zip exists
			for _, zp := range archiveLog.ZipParts {
				if zp == 0 {
					return 0, true
				}
			}
			// Return lowest available zip
			return archiveLog.ZipParts[0], true
		}
	}

	// Check if the calculated zip index exists
	for _, zp := range archiveLog.ZipParts {
		if zp == zipIndex {
			return zipIndex, true
		}
	}

	return 0, false
}

// GetAllLogs returns a copy of all discovered logs (for logs.v3.json building).
func (ai *ArchiveIndex) GetAllLogs() ArchiveSnapshot {
	if ai == nil {
		return ArchiveSnapshot{Logs: make(map[string]ArchiveLog)}
	}
	val := ai.snap.Load()
	if val == nil {
		return ArchiveSnapshot{Logs: make(map[string]ArchiveLog)}
	}
	snap, ok := val.(ArchiveSnapshot)
	if !ok {
		// This should never happen - atomic.Value only stores ArchiveSnapshot
		panic("archive index: invalid type in atomic.Value")
	}
	return snap
}

func (ai *ArchiveIndex) refreshOnce() {
	ai.refreshMu.Lock()
	defer ai.refreshMu.Unlock()

	// Get previous snapshot to preserve FirstDiscovered timestamps
	var prevSnap *ArchiveSnapshot
	if val := ai.snap.Load(); val != nil {
		if snap, ok := val.(ArchiveSnapshot); ok {
			prevSnap = &snap
		}
	}

	snap, err := buildArchiveSnapshot(ai.cfg, ai.readDir, ai.logger, prevSnap)
	if err != nil {
		if ai.logger != nil {
			ai.logger.Error("archive refresh failed", "error", err)
		}
		return
	}
	ai.snap.Store(snap)
	ai.updateResourceMetrics(snap)
}

func (ai *ArchiveIndex) updateResourceMetrics(snap ArchiveSnapshot) {
	if ai.metrics == nil {
		return
	}
	logCount := len(snap.Logs)
	zipPartCount := 0
	for _, l := range snap.Logs {
		zipPartCount += len(l.ZipParts)
	}
	ai.metrics.SetArchiveDiscovered(logCount, zipPartCount)
}

func buildArchiveSnapshot(cfg Config, readDir func(string) ([]os.DirEntry, error), logger *slog.Logger, prevSnap *ArchiveSnapshot) (ArchiveSnapshot, error) {
	if readDir == nil {
		readDir = os.ReadDir
	}

	entries, err := readDir(cfg.ArchivePath)
	if err != nil {
		return ArchiveSnapshot{}, fmt.Errorf("read archive path: %w", err)
	}

	if logger != nil {
		logger.Debug("Scanning archive directory", "path", cfg.ArchivePath, "entry_count", len(entries))
	}

	now := time.Now()
	logs := make(map[string]ArchiveLog)
	discoveredCount := 0
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}

		folderName := ent.Name()
		if cfg.ArchiveFolderPrefix != "" && !strings.HasPrefix(folderName, cfg.ArchiveFolderPrefix) {
			if logger != nil {
				logger.Debug("Skipping directory (doesn't match pattern)", "folder", folderName, "pattern", cfg.ArchiveFolderPrefix+"*")
			}
			continue
		}

		logName := strings.TrimPrefix(folderName, cfg.ArchiveFolderPrefix)
		if logName == "" {
			// Empty <log> is not meaningful; ignore.
			continue
		}

		if prev, ok := logs[logName]; ok {
			return ArchiveSnapshot{}, fmt.Errorf("archive folder collision for log %q: %q and %q", logName, prev.FolderName, folderName)
		}

		folderPath := filepath.Join(cfg.ArchivePath, folderName)
		if logger != nil {
			logger.Debug("Discovering zip parts", "log", logName, "folder", folderPath)
		}
		zipParts, err := discoverZipParts(folderPath, logger)
		if err != nil {
			return ArchiveSnapshot{}, fmt.Errorf("discover zip parts for %q: %w", folderName, err)
		}
		if logger != nil {
			logger.Debug("Discovered zip parts", "log", logName, "zip_parts", zipParts)
		}

		// Determine FirstDiscovered timestamp:
		// - If log existed in previous snapshot, preserve its FirstDiscovered timestamp
		// - If log is new and has 000.zip, set FirstDiscovered to now
		// - If log is new but doesn't have 000.zip yet, set to zero time (will be set when 000.zip appears)
		var firstDiscovered time.Time
		if prevSnap != nil {
			if prevLog, ok := prevSnap.Logs[logName]; ok {
				// Log existed before, preserve its discovery timestamp
				firstDiscovered = prevLog.FirstDiscovered
			}
		}
		// If this is a new log and has 000.zip, set discovery timestamp
		if firstDiscovered.IsZero() {
			has000Zip := false
			for _, zp := range zipParts {
				if zp == 0 {
					has000Zip = true
					break
				}
			}
			if has000Zip {
				firstDiscovered = now
				if logger != nil {
					logger.Debug("New log discovered with 000.zip", "log", logName, "discovered_at", firstDiscovered)
				}
			}
		}

		logs[logName] = ArchiveLog{
			Log:            logName,
			FolderName:     folderName,
			FolderPath:     folderPath,
			ZipParts:       zipParts,
			FirstDiscovered: firstDiscovered,
		}
		discoveredCount++
	}

	if logger != nil {
		logger.Debug("Archive snapshot complete", "discovered_logs", discoveredCount)
	}

	return ArchiveSnapshot{Logs: logs}, nil
}

func discoverZipParts(folderPath string, logger *slog.Logger) ([]int, error) {
	ents, err := os.ReadDir(folderPath)
	if err != nil {
		return nil, fmt.Errorf("read zip parts directory: %w", err)
	}

	var out []int
	for _, ent := range ents {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if len(name) != len("000.zip") || !strings.HasSuffix(name, ".zip") {
			continue
		}
		prefix := name[:3]
		if name[3:] != ".zip" {
			continue
		}
		if prefix[0] < '0' || prefix[0] > '9' || prefix[1] < '0' || prefix[1] > '9' || prefix[2] < '0' || prefix[2] > '9' {
			continue
		}
		n, err := strconv.Atoi(prefix)
		if err != nil {
			continue
		}
		out = append(out, n)
		if logger != nil {
			logger.Debug("Found zip part", "zip_file", name, "index", n, "folder", folderPath)
		}
	}

	sort.Ints(out)
	return out, nil
}

