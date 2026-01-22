# Implementation Plan: ct-archive-serve (Static CT archive HTTP server)

**Branch**: `001-ct-archive-serve` | **Date**: 2026-01-21 | **Spec**: `specs/001-ct-archive-serve/spec.md`
**Input**: Feature specification from `specs/001-ct-archive-serve/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Implement `ct-archive-serve`, a native `net/http` server that serves Static-CT monitoring endpoints directly from `photocamera-archiver` zip archives on disk. This is intended for archives downloaded via torrents (e.g., `ct_<LOG_NAME>/NNN.zip` folders from `torrents.rss` in `geomys/ct-archive`) so CT readers can consume them without unzipping. The server listens on TCP/8080 by default and uses environment variables for configuration; CLI flags only toggle logging/help. `ct-archive-serve` is intended to be deployed behind a reverse proxy that provides TLS termination and rate limiting. If a required zip part exists but fails structural zip integrity checks (often because it is still downloading), requests return `503` until the zip part passes integrity verification.

## Technical Context

**Language/Version**: Go 1.25.5+  
**Primary Dependencies**: Standard library + Prometheus client (`net/http`, `archive/zip`, `os`, `path/filepath`, `context`, `log/slog`, `github.com/prometheus/client_golang/prometheus/promhttp`)  
**Storage**: Local filesystem (read-only archives)  
**Testing**: `go test ./...` (unit tests + integration tests using temp dirs and generated zip files)  
**CI/CD**: GitHub Actions (public GitHub repo); build outputs published to GHCR  
**Target Platform**: Linux server/container  
**Project Type**: Single project (Go CLI tool under `cmd/`)  
**Performance Goals**: Perform under extreme load such that scaling is primarily constrained by hardware (CPU, memory bandwidth, disk throughput), not avoidable software overhead. Optimize for a **large working set** (requests spread across many zip parts/tiles).  
**Constraints**:
- Must listen on HTTP TCP/8080 by default
- Must be fully configurable via environment variables (no config files)
- Must configure safe HTTP server timeouts/limits to avoid resource exhaustion under slow/abusive clients
- Must rely on a reverse proxy for TLS termination and rate limiting (no in-process rate limiting)
- Must be implemented independently in this repository (no reuse of internal codebases as a dependency; stdlib preferred; upstream CT libraries allowed)
- Must support multiple archived logs under a top-level archive directory
- Must open and serve zip entries via seekable/random-access reads (avoid whole-zip decompression)
- Must enforce HTTP method policy per `spec.md` `FR-002a` (support `GET`+`HEAD`; other methods to supported routes return `405` with `Allow: GET, HEAD`)
- Must return `503` (temporarily unavailable) when a required zip part exists but fails structural zip integrity checks, with cached pass/fail results and a failed TTL per `spec.md` `FR-013`
**Scale/Scope**: 10s–100s of archives; archives may contain 1–1000 zip parts per log

## Implementation Notes

- **Zip entry serving**: Use Go standard library `archive/zip` with `zip.OpenReader` (or `zip.NewReader` over an `io.ReaderAt`) to ensure random-access reads via the central directory and streaming decompression of only the requested entry.
- **`/monitor.json` URL formation**: Derive the “public base URL” from incoming request headers. Use `Host` header by default. If `CT_HTTP_TRUSTED_SOURCES` is set (CSV of IP addresses/CIDR ranges), trust `X-Forwarded-Host`/`X-Forwarded-Proto` only when the request source IP matches a trusted source; otherwise, log but ignore these headers. For comma-separated `X-Forwarded-*` values, use the first non-empty element after trimming ASCII whitespace. Lowercase the scheme for URL construction. The server does not validate or require a configured hostname/transport.
- **`/monitor.json` output**: Must be deterministic with `tiled_logs` sorted by `<log>` ascending per `spec.md` `FR-006`.
- **`/monitor.json` refresh failure**: If the most recent refresh attempt fails, `GET /monitor.json` returns `503` (temporarily unavailable) until the next successful refresh per `spec.md` `FR-006`.
- **Tile index parsing**: `<N>` must follow the tlog "groups-of-three" decimal path encoding per `spec.md` `FR-008a` (so `<N>` may span multiple path segments).
- **Zip integrity verification**: For torrent-downloaded archives, use a structural validity check (no decompression): open the zip with `zip.OpenReader` (validates central directory/EOCD) and iterate entries, calling `Open()`/`Close()` on each to validate local file headers/offsets without reading entry bodies. Results are cached per `spec.md` `FR-013`; failed zip parts cause handlers to return `503` until a re-check succeeds.

## Performance Architecture (Extreme Load)

### Design goals

- **No request-path rescans**: directory walking/globbing is performed at startup and via a periodic refresh loop only.
- **Bounded, reusable zip state**: avoid repeated open+central-directory parse for hot zip parts by caching open `*os.File` + parsed zip reader/index.
- **Bounded resource usage**: caches are capped and evict (LRU) to handle large working sets without unbounded growth.
- **Low contention**: avoid a single global lock on hot paths; shard caches/index by log and/or zip part.
- **Low-cardinality metrics**: expose Prometheus metrics only for `/monitor.json` and per-`<log>` aggregates (no per-path, per-endpoint, or per-status labeling).

### Core components (internal/ct-archive-serve)

- **ArchiveIndex**: in-memory index mapping `<log>` → archive folder path + known zip parts, refreshed periodically (`CT_ARCHIVE_REFRESH_INTERVAL`).
- **ZipIntegrityCache**: in-memory zip integrity results used to tolerate in-progress torrent downloads. Uses structural validity checks (central directory + local file headers, no decompression). Maintains a permanent "passed" set (until read failures evict entries) and a TTL "failed" set (`CT_ZIP_INTEGRITY_FAIL_TTL`, default 5m). When a required zip part fails integrity checks, requests return `503` (temporarily unavailable) per `spec.md` `FR-013`.
- **ZipPartCache** (Phase 5 performance optimization): Bounded cache (config: `CT_ZIP_CACHE_MAX_OPEN`) that retains open file handles and a prebuilt entry index for each zip part. This avoids repeated central-directory parsing for hot zip parts. Baseline functionality (Phase 2-4) can work without this cache by opening zip parts on-demand, but `ZipPartCache` is required to meet `SC-006` performance goals under extreme load.
- **ZipEntryIndex**: Map-like structure created from the zip central directory (entry name → `*zip.File`/metadata) to make per-request lookup O(1) for an already-cached zip part. This is created as part of `ZipPartCache` when a zip part is cached.

**Component responsibilities clarification**:
- **ZipIntegrityCache**: Validates zip structural integrity (central directory + local headers). Returns pass/fail with caching. Used before attempting to read from a zip part. Required for baseline functionality (Phase 2).
- **ZipPartCache**: Performance optimization (Phase 5) that caches open file handles and pre-parsed zip readers to avoid repeated `zip.OpenReader` calls for hot zip parts. Works in conjunction with `ZipEntryIndex`.
- **ZipEntryIndex**: O(1) lookup structure (entry name → `*zip.File`) created from a zip's central directory. Created when a zip part is added to `ZipPartCache`. Not a standalone component; part of the `ZipPartCache` optimization.

### Request hot path (intended)

- Parse + validate request path
- Lookup `<log>` in `ArchiveIndex` (no filesystem traversal)
- Select zip part for the asset (math for tiles/data; prefer `000.zip` for shared metadata)
- Get zip part from `ZipPartCache` (open+index on miss; reuse on hit; evict LRU on pressure) **[Phase 5: baseline opens zip on-demand]**
- Stream-decompress only the requested entry to the HTTP response writer

## Performance Validation Approach

- Add **benchmarks** that simulate a large working set (many distinct tile/data requests across many zip parts) and ensure overhead is not dominated by avoidable work (archive rescans, repeated index parsing, lock contention).
- Use Go profiling (`pprof`) during benchmark runs to confirm the dominant costs are zip decompression, disk reads, and network writes.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- Use explicit error handling and wrapped errors (`fmt.Errorf("context: %w", err)`).
- Avoid global mutable state; inject dependencies via constructors where practical.
- Prevent path traversal; never serve filesystem paths outside the configured archive root.
- Keep `cmd/ct-archive-serve` thin; core logic in `internal/ct-archive-serve`.
- Provide unit tests for path parsing, routing, zip selection, and 404 behavior.

## Project Structure

### Documentation (this feature)

```text
specs/001-ct-archive-serve/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)
```text
cmd/
└── ct-archive-serve/
    └── main.go

internal/
└── ct-archive-serve/
    ├── config.go
    ├── logger.go
    ├── metrics.go
    ├── server.go
    ├── routing.go
    ├── archive_index.go
    ├── monitor_json.go
    ├── zip_cache.go
    ├── zip_reader.go
    └── *_test.go
```

**Structure Decision**: Add a new CLI entrypoint under `cmd/ct-archive-serve/` and implement archive discovery, request routing, and zip-backed content serving in `internal/ct-archive-serve/`.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| None | N/A | N/A |
