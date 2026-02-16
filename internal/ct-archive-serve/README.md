# ct-archive-serve Internal Package

This package contains the core implementation of `ct-archive-serve`, an HTTP server for serving Static-CT assets directly from `photocamera-archiver` zip archives.

## Environment Variables

See the top-level `README.md` for a complete list of environment variables. Key variables include:

- `CT_ARCHIVE_PATH`: Path to the archive directory containing log folders
- `CT_ARCHIVE_FOLDER_PATTERN`: Pattern for matching log folders (e.g., `ct_*`)
- `CT_LOGLISTV3_JSON_REFRESH_INTERVAL`: Interval for refreshing `/logs.v3.json` (default: 10m)
- `CT_ARCHIVE_REFRESH_INTERVAL`: Interval for refreshing archive index (default: 5m)
- `CT_ZIP_CACHE_MAX_OPEN`: Maximum number of open zip parts to cache (default: 2048)
- `CT_ZIP_CACHE_MAX_CONCURRENT_OPENS`: Maximum concurrent zip.OpenReader calls (default: 64)
- `CT_ENTRY_CACHE_MAX_BYTES`: Maximum bytes of decompressed entry content to cache (default: 256MiB, set to 0 to disable)
- `CT_ZIP_INTEGRITY_FAIL_TTL`: TTL for failed zip integrity checks (default: 5m)
- `CT_HTTP_*`: HTTP server timeout and limit configuration

## Routing Summary

The server supports the following routes:

- `GET /logs.v3.json`: CT log list v3 compatible JSON
- `GET /metrics`: Prometheus metrics
- `GET /<log>/checkpoint`: Log checkpoint (text/plain)
- `GET /<log>/log.v3.json`: Log metadata (application/json)
- `GET /<log>/tile/<L>/<N>[.p/<W>]`: Hash tiles (application/octet-stream)
- `GET /<log>/tile/data/<N>[.p/<W>]`: Data tiles (application/octet-stream)
- `GET /<log>/issuer/<fingerprint>`: Issuer certificates (application/pkix-cert)

All routes support `GET` and `HEAD` methods. Other methods return `405 Method Not Allowed`.

## Zip Integrity Behavior

When a zip part exists but fails structural integrity checks (e.g., still downloading via torrent), the server returns HTTP `503` (Service Temporarily Unavailable). Failed integrity results are cached for `CT_ZIP_INTEGRITY_FAIL_TTL` (default: 5 minutes) to allow re-testing once the zip part becomes complete.

## Logging Policy

- **Non-2xx responses**: Always logged (INFO for 3xx/4xx, WARN for 4xx, ERROR for 5xx)
- **2xx responses**: Logged only when verbose mode is enabled (`-v`/`--verbose` flag)
- **Log format**: Structured JSON with fields: method, path, status, duration_ms, log (when applicable), x_forwarded_host, x_forwarded_proto

## Metrics Policy

Metrics are low-cardinality to avoid metric explosion:

- `/logs.v3.json` requests: Aggregate counters and histograms (no labels)
- `/<log>/...` requests: Per-log aggregates (labeled by `log` only)
- Resource observability: Gauges for discovered logs/zip parts, cache size, integrity pass/fail counts (no per-request labels)

## Performance Tuning

### Benchmarks

Run performance benchmarks:

```bash
go test -bench=. ./internal/ct-archive-serve
```

### Profiling

Profile CPU usage:

```bash
go test -bench=BenchmarkZipReader_OpenEntry -cpuprofile=cpu.prof ./internal/ct-archive-serve
go tool pprof cpu.prof
```

Profile memory allocations:

```bash
go test -bench=BenchmarkZipReader_OpenEntry -memprofile=mem.prof ./internal/ct-archive-serve
go tool pprof mem.prof
```

Profile mutex contention:

```bash
go test -bench=BenchmarkZipReader_OpenEntry -mutexprofile=mutex.prof ./internal/ct-archive-serve
go tool pprof mutex.prof
```

### Cache Architecture

Both `ZipPartCache` and `EntryContentCache` are internally sharded (64 shards by default) using FNV-1a hashing. Under high concurrency (e.g., 45+ simultaneous log downloads), each goroutine almost always hits a distinct shard, reducing lock contention to near zero compared to a single global lock.

### Cache Tuning

Adjust `CT_ZIP_CACHE_MAX_OPEN` based on:
- Available file descriptors
- Working set size (number of distinct zip parts accessed)
- Memory constraints

Higher values improve performance for hot zip parts but increase memory and file descriptor usage.

Adjust `CT_ENTRY_CACHE_MAX_BYTES` based on:
- Available memory
- Working set size (number of frequently accessed decompressed entries)
- Set to `0` to disable entry caching entirely

The memory budget is distributed evenly across all 64 shards (e.g., 256MiB total = 4MiB per shard).
