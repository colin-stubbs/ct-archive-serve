package ctarchiveserve

import (
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for ct-archive-serve.
//
// Source of truth: specs/001-ct-archive-serve/spec.md.
type Config struct {
	ArchivePath          string
	ArchiveFolderPattern string
	ArchiveFolderPrefix  string

	MonitorJSONRefreshInterval time.Duration
	ArchiveRefreshInterval     time.Duration

	ZipCacheMaxOpen     int
	ZipIntegrityFailTTL time.Duration

	HTTPReadHeaderTimeout time.Duration
	HTTPIdleTimeout       time.Duration
	HTTPMaxHeaderBytes    int
	HTTPWriteTimeout      time.Duration
	HTTPReadTimeout       time.Duration

	HTTPTrustedSources []netip.Prefix
}

type envLookup func(key string) (string, bool)

// LoadConfig loads configuration from environment variables.
//
// This is the production entry point for loading configuration. It reads all
// configuration values from the process environment using os.LookupEnv.
//
// Usage pattern:
//
//	cfg, err := ctarchiveserve.LoadConfig()
//	if err != nil {
//		log.Fatalf("failed to load configuration: %v", err)
//	}
//	// Use cfg to initialize server components
//
// For testing, use parseConfigFromMap instead to provide explicit test values
// without relying on environment variables.
//
// Returns an error if any required configuration value is invalid (e.g., invalid
// duration format, invalid IP/CIDR in CT_HTTP_TRUSTED_SOURCES, or invalid
// numeric values). All configuration values have sensible defaults if not set.
func LoadConfig() (Config, error) {
	return parseConfigFromLookup(os.LookupEnv)
}

func parseConfigFromMap(env map[string]string) (Config, error) {
	return parseConfigFromLookup(func(key string) (string, bool) {
		v, ok := env[key]
		return v, ok
	})
}

func parseConfigFromLookup(lookup envLookup) (Config, error) {
	cfg := Config{
		ArchivePath:          "/var/log/ct/archive",
		ArchiveFolderPattern: "ct_*",
		MonitorJSONRefreshInterval: 5 * time.Minute,
		ArchiveRefreshInterval:     1 * time.Minute,
		ZipCacheMaxOpen:            256,
		ZipIntegrityFailTTL:        5 * time.Minute,
		HTTPReadHeaderTimeout:      5 * time.Second,
		HTTPIdleTimeout:            60 * time.Second,
		HTTPMaxHeaderBytes:         8192,
		HTTPWriteTimeout:           0,
		HTTPReadTimeout:            0,
	}

	if v, ok := lookup("CT_ARCHIVE_PATH"); ok && v != "" {
		cfg.ArchivePath = v
	}

	if v, ok := lookup("CT_ARCHIVE_FOLDER_PATTERN"); ok {
		if v == "" {
			return Config{}, fmt.Errorf("CT_ARCHIVE_FOLDER_PATTERN: empty value is invalid")
		}
		cfg.ArchiveFolderPattern = v
	}

	prefix, err := parseArchiveFolderPrefix(cfg.ArchiveFolderPattern)
	if err != nil {
		return Config{}, fmt.Errorf("CT_ARCHIVE_FOLDER_PATTERN: %w", err)
	}
	cfg.ArchiveFolderPrefix = prefix

	if v, ok := lookup("CT_MONITOR_JSON_REFRESH_INTERVAL"); ok && v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("CT_MONITOR_JSON_REFRESH_INTERVAL: %w", err)
		}
		if d <= 0 {
			return Config{}, fmt.Errorf("CT_MONITOR_JSON_REFRESH_INTERVAL: must be > 0")
		}
		cfg.MonitorJSONRefreshInterval = d
	}

	if v, ok := lookup("CT_ARCHIVE_REFRESH_INTERVAL"); ok && v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("CT_ARCHIVE_REFRESH_INTERVAL: %w", err)
		}
		if d <= 0 {
			return Config{}, fmt.Errorf("CT_ARCHIVE_REFRESH_INTERVAL: must be > 0")
		}
		cfg.ArchiveRefreshInterval = d
	}

	if v, ok := lookup("CT_ZIP_CACHE_MAX_OPEN"); ok && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("CT_ZIP_CACHE_MAX_OPEN: %w", err)
		}
		if n <= 0 {
			return Config{}, fmt.Errorf("CT_ZIP_CACHE_MAX_OPEN: must be > 0")
		}
		cfg.ZipCacheMaxOpen = n
	}

	if v, ok := lookup("CT_ZIP_INTEGRITY_FAIL_TTL"); ok && v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("CT_ZIP_INTEGRITY_FAIL_TTL: %w", err)
		}
		if d <= 0 {
			return Config{}, fmt.Errorf("CT_ZIP_INTEGRITY_FAIL_TTL: must be > 0")
		}
		cfg.ZipIntegrityFailTTL = d
	}

	if v, ok := lookup("CT_HTTP_READ_HEADER_TIMEOUT"); ok && v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("CT_HTTP_READ_HEADER_TIMEOUT: %w", err)
		}
		if d < 0 {
			return Config{}, fmt.Errorf("CT_HTTP_READ_HEADER_TIMEOUT: must be >= 0")
		}
		cfg.HTTPReadHeaderTimeout = d
	}

	if v, ok := lookup("CT_HTTP_IDLE_TIMEOUT"); ok && v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("CT_HTTP_IDLE_TIMEOUT: %w", err)
		}
		if d < 0 {
			return Config{}, fmt.Errorf("CT_HTTP_IDLE_TIMEOUT: must be >= 0")
		}
		cfg.HTTPIdleTimeout = d
	}

	if v, ok := lookup("CT_HTTP_MAX_HEADER_BYTES"); ok && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("CT_HTTP_MAX_HEADER_BYTES: %w", err)
		}
		if n <= 0 {
			return Config{}, fmt.Errorf("CT_HTTP_MAX_HEADER_BYTES: must be > 0")
		}
		cfg.HTTPMaxHeaderBytes = n
	}

	if v, ok := lookup("CT_HTTP_WRITE_TIMEOUT"); ok && v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("CT_HTTP_WRITE_TIMEOUT: %w", err)
		}
		if d < 0 {
			return Config{}, fmt.Errorf("CT_HTTP_WRITE_TIMEOUT: must be >= 0")
		}
		cfg.HTTPWriteTimeout = d
	}

	if v, ok := lookup("CT_HTTP_READ_TIMEOUT"); ok && v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("CT_HTTP_READ_TIMEOUT: %w", err)
		}
		if d < 0 {
			return Config{}, fmt.Errorf("CT_HTTP_READ_TIMEOUT: must be >= 0")
		}
		cfg.HTTPReadTimeout = d
	}

	if v, ok := lookup("CT_HTTP_TRUSTED_SOURCES"); ok {
		ps, err := parseTrustedSourcesCSV(v)
		if err != nil {
			return Config{}, fmt.Errorf("CT_HTTP_TRUSTED_SOURCES: %w", err)
		}
		cfg.HTTPTrustedSources = ps
	}

	return cfg, nil
}

func parseArchiveFolderPrefix(pattern string) (string, error) {
	if !strings.HasSuffix(pattern, "*") {
		return "", fmt.Errorf("pattern must be of the form <prefix>* (missing trailing '*')")
	}
	if strings.Count(pattern, "*") != 1 {
		return "", fmt.Errorf("pattern must contain exactly one '*' and it must be the final character")
	}
	return strings.TrimSuffix(pattern, "*"), nil
}

func parseTrustedSourcesCSV(csv string) ([]netip.Prefix, error) {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return nil, nil
	}

	parts := strings.Split(csv, ",")
	out := make([]netip.Prefix, 0, len(parts))
	for _, raw := range parts {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}

		if strings.Contains(s, "/") {
			p, err := netip.ParsePrefix(s)
			if err != nil {
				return nil, fmt.Errorf("invalid CIDR %q: %w", s, err)
			}
			out = append(out, p)
			continue
		}

		a, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fmt.Errorf("invalid IP %q: %w", s, err)
		}
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}

	return out, nil
}

