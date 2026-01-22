package ctarchiveserve

import (
	"testing"
	"time"
)

func TestParseConfig_Defaults(t *testing.T) {
	t.Parallel()

	cfg, err := parseConfigFromMap(map[string]string{})
	if err != nil {
		t.Fatalf("parseConfigFromMap() error = %v", err)
	}

	if got, want := cfg.ArchivePath, "/var/log/ct/archive"; got != want {
		t.Fatalf("ArchivePath = %q, want %q", got, want)
	}
	if got, want := cfg.ArchiveFolderPattern, "ct_*"; got != want {
		t.Fatalf("ArchiveFolderPattern = %q, want %q", got, want)
	}
	if got, want := cfg.ArchiveFolderPrefix, "ct_"; got != want {
		t.Fatalf("ArchiveFolderPrefix = %q, want %q", got, want)
	}

	if got, want := cfg.MonitorJSONRefreshInterval, 5*time.Minute; got != want {
		t.Fatalf("MonitorJSONRefreshInterval = %v, want %v", got, want)
	}
	if got, want := cfg.ArchiveRefreshInterval, 1*time.Minute; got != want {
		t.Fatalf("ArchiveRefreshInterval = %v, want %v", got, want)
	}
	if got, want := cfg.ZipCacheMaxOpen, 256; got != want {
		t.Fatalf("ZipCacheMaxOpen = %d, want %d", got, want)
	}
	if got, want := cfg.ZipIntegrityFailTTL, 5*time.Minute; got != want {
		t.Fatalf("ZipIntegrityFailTTL = %v, want %v", got, want)
	}

	if got, want := cfg.HTTPReadHeaderTimeout, 5*time.Second; got != want {
		t.Fatalf("HTTPReadHeaderTimeout = %v, want %v", got, want)
	}
	if got, want := cfg.HTTPIdleTimeout, 60*time.Second; got != want {
		t.Fatalf("HTTPIdleTimeout = %v, want %v", got, want)
	}
	if got, want := cfg.HTTPMaxHeaderBytes, 8192; got != want {
		t.Fatalf("HTTPMaxHeaderBytes = %d, want %d", got, want)
	}
	if got, want := cfg.HTTPWriteTimeout, time.Duration(0); got != want {
		t.Fatalf("HTTPWriteTimeout = %v, want %v", got, want)
	}
	if got, want := cfg.HTTPReadTimeout, time.Duration(0); got != want {
		t.Fatalf("HTTPReadTimeout = %v, want %v", got, want)
	}

	if len(cfg.HTTPTrustedSources) != 0 {
		t.Fatalf("HTTPTrustedSources length = %d, want 0", len(cfg.HTTPTrustedSources))
	}
}

func TestParseConfig_InvalidValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  map[string]string
	}{
		{
			name: "invalid folder pattern missing star",
			env:  map[string]string{"CT_ARCHIVE_FOLDER_PATTERN": "ct_"},
		},
		{
			name: "invalid folder pattern star not last",
			env:  map[string]string{"CT_ARCHIVE_FOLDER_PATTERN": "ct_*_oops"},
		},
		{
			name: "invalid folder pattern multiple stars",
			env:  map[string]string{"CT_ARCHIVE_FOLDER_PATTERN": "ct_**"},
		},
		{
			name: "invalid monitor refresh duration",
			env:  map[string]string{"CT_MONITOR_JSON_REFRESH_INTERVAL": "nope"},
		},
		{
			name: "invalid archive refresh duration",
			env:  map[string]string{"CT_ARCHIVE_REFRESH_INTERVAL": "nope"},
		},
		{
			name: "invalid zip cache max open",
			env:  map[string]string{"CT_ZIP_CACHE_MAX_OPEN": "nope"},
		},
		{
			name: "invalid zip cache max open zero",
			env:  map[string]string{"CT_ZIP_CACHE_MAX_OPEN": "0"},
		},
		{
			name: "invalid zip integrity fail ttl",
			env:  map[string]string{"CT_ZIP_INTEGRITY_FAIL_TTL": "nope"},
		},
		{
			name: "invalid http max header bytes",
			env:  map[string]string{"CT_HTTP_MAX_HEADER_BYTES": "nope"},
		},
		{
			name: "invalid http max header bytes zero",
			env:  map[string]string{"CT_HTTP_MAX_HEADER_BYTES": "0"},
		},
		{
			name: "invalid trusted sources entry",
			env:  map[string]string{"CT_HTTP_TRUSTED_SOURCES": "not-an-ip"},
		},
		{
			name: "invalid trusted sources prefix",
			env:  map[string]string{"CT_HTTP_TRUSTED_SOURCES": "10.0.0.0/not-a-prefix"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseConfigFromMap(tc.env)
			if err == nil {
				t.Fatalf("parseConfigFromMap() error = nil, want non-nil")
			}
		})
	}
}

func TestParseConfig_TrustedSources(t *testing.T) {
	t.Parallel()

	cfg, err := parseConfigFromMap(map[string]string{
		"CT_HTTP_TRUSTED_SOURCES": "127.0.0.1, 10.0.0.0/8, ,192.168.1.10/32",
	})
	if err != nil {
		t.Fatalf("parseConfigFromMap() error = %v", err)
	}
	if got, want := len(cfg.HTTPTrustedSources), 3; got != want {
		t.Fatalf("HTTPTrustedSources length = %d, want %d", got, want)
	}
}

