---

# Tasks: ct-archive-serve

**Feature**: `specs/001-ct-archive-serve/`  
**Spec**: `specs/001-ct-archive-serve/spec.md`  
**Plan**: `specs/001-ct-archive-serve/plan.md`  
**Contracts**: `specs/001-ct-archive-serve/contracts/http.md`

## Dependency Graph (User Stories)

- **User Story 1 (P1)**: `GET /logs.v3.json` for discovery of archived logs
  - Depends on: config + archive discovery + zip reading + base HTTP server
- **User Story 2 (P1)**: Serve Static-CT assets from archive zips under `/<log>/...`
  - Depends on: config + archive discovery + routing + zip reading + base HTTP server

## Parallel opportunities (examples)

- **After Phase 2 completes**:
  - [P] US1 loglist v3 json builder (`internal/ct-archive-serve/monitor_json.go`) can proceed in parallel with
  - [P] US2 asset handlers (`internal/ct-archive-serve/server.go`, `internal/ct-archive-serve/routing.go`)

---

## Phase 1: Setup (Shared Infrastructure)

- [x] T001 Create implementation file skeletons under `internal/ct-archive-serve/` (`config.go`, `logger.go`, `metrics.go`, `routing.go`, `archive_index.go`, `zip_cache.go`, `zip_reader.go`, `monitor_json.go`, `server.go`) and keep `cmd/ct-archive-serve/main.go` thin (wiring only)
- [x] T002 Create `.dockerignore` at repo root to exclude common build/test artifacts and local archive data (e.g. `bin/`, `*.out`, `.git/`, `archive/`, `coverage.*`)
- [x] T003 Add required Go module dependencies in `go.mod` (Prometheus client for `promhttp`) and run `go mod tidy`

---

## Phase 2: Foundational (Blocking Prerequisites)

**⚠️ CRITICAL**: This phase blocks all user story work.

### Configuration (env vars) + validation

- [x] T004 Add config parsing tests in `internal/ct-archive-serve/config_test.go` covering defaults and invalid values for: `CT_ARCHIVE_PATH`, `CT_ARCHIVE_FOLDER_PATTERN` (`<prefix>*` only), `CT_LOGLISTV3_JSON_REFRESH_INTERVAL`, `CT_ARCHIVE_REFRESH_INTERVAL`, `CT_ZIP_CACHE_MAX_OPEN`, `CT_ZIP_INTEGRITY_FAIL_TTL`, and `CT_HTTP_*` (incl. `CT_HTTP_TRUSTED_SOURCES`)
- [x] T005 Implement env config parsing/validation in `internal/ct-archive-serve/config.go` per `spec.md` (`FR-004`, `FR-007`, `FR-011`, `FR-012`, `FR-013`) including `CT_HTTP_TRUSTED_SOURCES` parsing as CSV of `netip.Addr` and/or `netip.Prefix`

### Structured logging (slog)

- [x] T006 Implement structured JSON logger setup in `internal/ct-archive-serve/logger.go` (stdout/stderr split; `-v/--verbose` and `-d/--debug` level control per `spec.md` `NFR-010`)
  - **Implementation note**: Added detailed startup debug logging when `-d/--debug` is enabled, including archive discovery progress, logs.v3.json snapshot building progress, and HTTP listener establishment (per `spec.md` `NFR-010` startup debug logging).

### Low-cardinality Prometheus metrics

- [x] T007 Add metrics unit tests in `internal/ct-archive-serve/metrics_test.go` asserting low-cardinality labels: only `/logs.v3.json` aggregate, and per-`<log>` aggregate for all `/<log>/...` combined (no full-path / endpoint / status labels per `spec.md` `NFR-009`)
- [x] T008 Implement metrics in `internal/ct-archive-serve/metrics.go` (counters + durations for `/logs.v3.json`, and counters + durations labeled by `log` for `/<log>/...`)
- [x] T048 Add resource observability metrics tests in `internal/ct-archive-serve/metrics_test.go` (assert low-cardinality gauges exist for cache/index/integrity state and have no per-request/path labels; aligns with constitution Principle IV) **[Note: Tests may initially fail until T049 is complete; this is acceptable for test-driven development]**
- [x] T049 Implement resource observability gauges in `internal/ct-archive-serve/metrics.go` (e.g., open zip parts, cache size/evictions, integrity passed/failed counts, discovered logs/zip parts) and update `server.go`/caches to keep them current **[Blocks: T048 tests will fail until this is complete]**

### Routing + path safety

- [x] T009 Add routing unit tests in `internal/ct-archive-serve/routing_test.go` for: `<log>` extraction, traversal rejection (`..`, encoded traversal attempts), issuer fingerprint validation (lowercase hex), tile/data parsing including `.p/<W>` where `W` is 1..255, and tlog "groups-of-three" decoding for `<N>` per `spec.md` (`FR-008a`)
- [x] T010 Implement request path parsing + validation in `internal/ct-archive-serve/routing.go` including tlog "groups-of-three" `<N>` decoding and partial-tile parsing per `spec.md` (`FR-008a`) and edge cases

### Archive discovery + in-memory index

- [x] T011 Add archive index tests in `internal/ct-archive-serve/archive_index_test.go` for discovery under `CT_ARCHIVE_PATH`, mapping `<log>`→folder path via `CT_ARCHIVE_FOLDER_PATTERN` prefix stripping (`FR-003a`), and enumerating `NNN.zip` parts
- [x] T012 Implement archive discovery + in-memory index in `internal/ct-archive-serve/archive_index.go` (startup build + periodic refresh loop controlled by `CT_ARCHIVE_REFRESH_INTERVAL`; request hot path MUST use in-memory snapshot and MUST NOT rescan disk per `spec.md` `SC-006`)
  - **Implementation note**: Added mutex protection (`refreshMu`) to serialize refresh operations and prevent concurrent disk scans when refresh takes longer than `CT_ARCHIVE_REFRESH_INTERVAL` (per `spec.md` `FR-006` refresh concurrency protection).
- [x] T050 Add `<log>` collision handling tests in `internal/ct-archive-serve/archive_index_test.go` (two folders mapping to same `<log>` after `FR-003a` prefix stripping MUST fail startup with invalid configuration per `spec.md` `FR-003b`)
- [x] T051 Implement `<log>` collision detection in `internal/ct-archive-serve/archive_index.go` so startup fails deterministically with a clear error listing colliding folders per `spec.md` `FR-003b`

### Zip integrity verification (torrent-friendly) + zip entry streaming

- [x] T013 Add zip integrity tests in `internal/ct-archive-serve/zip_cache_test.go` for: structural validity check failures, failed TTL expiry (default 5m), passed-cache persistence, and eviction on subsequent open/read failure per `spec.md` (`FR-013`)
- [x] T014 Implement zip integrity verification + caching in `internal/ct-archive-serve/zip_cache.go` (structural validity: `zip.OpenReader` + iterate entries and `Open()`/`Close()` without payload reads; cache pass forever and fail with TTL; expose “is temporarily unavailable” signal for handlers per `spec.md` (`FR-013`, `FR-009a`))
- [x] T015 Add zip entry read tests in `internal/ct-archive-serve/zip_reader_test.go` using temp zips to ensure serving a single entry streams only that entry and respects 404 vs 503 rules
- [x] T016 Implement zip entry open+stream helper in `internal/ct-archive-serve/zip_reader.go` using `archive/zip` (random-access) and integrating zip integrity checks per `spec.md` (`NFR-003`, `NFR-004`, `FR-013`)

### Base HTTP server + method policy + `/metrics`

- [x] T017 Add HTTP method policy tests in `internal/ct-archive-serve/server_test.go` ensuring: supported routes accept `GET`+`HEAD`, other methods to supported routes return `405` with `Allow: GET, HEAD`, unknown routes return `404` regardless of method per `spec.md` (`FR-002a`)
- [x] T018 Implement base HTTP server/router in `internal/ct-archive-serve/server.go` with method policy enforcement (`FR-002a`) and `GET /metrics` served via `promhttp` (`FR-002`)

### CLI entrypoint + safe net/http server options

- [x] T019 Implement CLI flags and process wiring in `cmd/ct-archive-serve/main.go` (`-h|--help|-v|--verbose|-d|--debug`) and configure `http.Server` timeouts/limits from config per `spec.md` (`FR-001`, `FR-005`, `FR-012`) with default listen `:8080`
  - **Implementation note**: Added detailed startup debug logging when `-d/--debug` is enabled, including archive discovery progress, logs.v3.json snapshot building progress, and HTTP listener establishment (per `spec.md` `NFR-010` startup debug logging).

---

## Phase 3: User Story 1 — Discover archived logs via `logs.v3.json` (Priority: P1) 🎯 MVP

**Goal**: Serve `GET /logs.v3.json` (CT log list v3 compatible) derived from discovered archives.

**Independent Test**: With multiple `ct_*` folders present, `GET /logs.v3.json` returns `200` `application/json` and contains one `tiled_logs[]` entry per discovered folder with a valid `000.zip`→`log.v3.json`.

- [x] T020 [P] [US1] Add `publicBaseURL` derivation tests in `internal/ct-archive-serve/server_test.go` (trusted-source gating via `CT_HTTP_TRUSTED_SOURCES`, comma-separated `X-Forwarded-*`, whitespace trimming, `Host` fallback, scheme lowercasing) per `spec.md` (`FR-006`, `FR-012`)
- [x] T021 [US1] Implement `publicBaseURL` derivation helper in `internal/ct-archive-serve/server.go` per `spec.md` (`FR-006`, `FR-012`)
- [x] T022 [P] [US1] Add loglist v3 json snapshot builder tests in `internal/ct-archive-serve/monitor_json_test.go` (extract+parse `log.v3.json` from `000.zip`, set `has_issuers` from presence of `issuer/`, remove `url`, set `submission_url`/`monitoring_url`, deterministic sort by `<log>`, validate generated JSON using `loglist3` library from `github.com/google/certificate-transparency-go/loglist3` per `spec.md` `FR-006` validation requirement)
- [x] T023 [US1] Implement loglist v3 json snapshot builder in `internal/ct-archive-serve/monitor_json.go` per `spec.md` (`FR-006`, `FR-006a`, `FR-006b`)
  - **Implementation note**: Optimized to open each `000.zip` file only once per log to extract both `log.v3.json` and check for `issuer/` entries (via `extractLogV3JSONAndCheckIssuers`), rather than opening the same ZIP file twice. This significantly reduces startup time for large archives (per `spec.md` `FR-006` ZIP optimization).
  - **Implementation note (mtime caching)**: Added mtime-based caching to avoid re-reading unchanged zip files. Before opening a `000.zip` file, the implementation checks the file's modification time. If the mtime matches the cached mtime, cached data is used without opening the ZIP file. Cache entries are automatically cleaned up when logs are removed from the archive index. This optimization significantly reduces disk I/O and CPU usage during periodic refreshes for large, stable archive sets (per `spec.md` `FR-006` mtime-based caching).
- [x] T024 [US1] Implement periodic refresh loop + atomic snapshot in `internal/ct-archive-serve/monitor_json.go` using `CT_LOGLISTV3_JSON_REFRESH_INTERVAL` and context-driven shutdown per `spec.md` (`FR-007`)
  - **Implementation note**: Added mutex protection (`refreshMu`) to serialize refresh operations and prevent concurrent refreshes when a refresh takes longer than `CT_LOGLISTV3_JSON_REFRESH_INTERVAL`, preventing resource waste from concurrent ZIP file opens (per `spec.md` `FR-006` refresh concurrency protection).
- [x] T025 [US1] Wire `GET /logs.v3.json` handler in `internal/ct-archive-serve/server.go` (render `version="3.0"`, one operator `{name:"ct-archive-serve", email:[], logs:[], tiled_logs:[...]}`, set `log_list_timestamp`, set `Content-Type: application/json`) per `spec.md` (`FR-006`)
- [x] T052 [P] [US1] Add `/logs.v3.json` refresh failure behavior tests in `internal/ct-archive-serve/monitor_json_test.go` (if the most recent refresh attempt fails, `GET /logs.v3.json` MUST return `503` until the next successful refresh per `spec.md` `FR-006`)
- [x] T053 [US1] Implement `/logs.v3.json` refresh failure behavior in `internal/ct-archive-serve/monitor_json.go` per `spec.md` `FR-006` (track last refresh error state; serve `503` when unhealthy; resume `200` after next successful refresh; log errors)

---

## Phase 4: User Story 2 — Serve Static-CT assets under `/<log>/...` (Priority: P1)

**Goal**: Serve checkpoint/log info/tiles/issuers directly from archive zip parts without extraction.

**Independent Test**: With one discovered log, clients can fetch checkpoint, at least one hash tile, at least one data tile, and an issuer (when present), each byte-for-byte from the zip entry and with correct `Content-Type`.

- [x] T026 [P] [US2] Add zip selection math tests in `internal/ct-archive-serve/archive_index_test.go` for hash tiles and data tiles per `spec.md` (`FR-008`) and shared-metadata selection (prefer `000.zip`, else lowest zip)
- [x] T027 [US2] Implement zip-part selection helpers in `internal/ct-archive-serve/archive_index.go` per `spec.md` (`FR-008`)
- [x] T028 [P] [US2] Add asset handler tests in `internal/ct-archive-serve/server_test.go` for `/<log>/checkpoint` and `/<log>/log.v3.json` (200 vs 404 vs 503; correct `Content-Type`; HEAD behavior)
- [x] T029 [US2] Implement `/<log>/checkpoint` and `/<log>/log.v3.json` handlers in `internal/ct-archive-serve/server.go` (zip entry `checkpoint` / `log.v3.json`) per `spec.md` (`FR-002`, `FR-009`, `FR-009a`)
- [x] T030 [P] [US2] Add tile handler tests in `internal/ct-archive-serve/server_test.go` for hash/data tiles including right-edge partial tiles (`.p/<W>` treated as literal entry path; 200 iff entry exists, else 404; 503 on bad zip part) per `spec.md` (Edge Cases, `FR-008a`, `FR-013`)
- [x] T031 [US2] Implement hash tile handler `/<log>/tile/<L>/<N>[.p/<W>]` in `internal/ct-archive-serve/server.go` mapping request → zip entry `tile/<L>/<N>[.p/<W>]` and serving via `zip_reader.go`
- [x] T032 [US2] Implement data tile handler `/<log>/tile/data/<N>[.p/<W>]` in `internal/ct-archive-serve/server.go` mapping request → zip entry `tile/data/<N>[.p/<W>]` and serving via `zip_reader.go`
- [x] T033 [P] [US2] Add issuer handler tests in `internal/ct-archive-serve/server_test.go` for `/<log>/issuer/<fingerprint>` (validation, 200/404/503, `Content-Type: application/pkix-cert`, HEAD behavior)
- [x] T034 [US2] Implement issuer handler `/<log>/issuer/<fingerprint>` in `internal/ct-archive-serve/server.go` serving zip entry `issuer/<fingerprint>` per `spec.md` (`FR-002`, `FR-009`, `FR-009a`)
- [x] T035 [US2] Implement HTTP request logging in `internal/ct-archive-serve/server.go` per `spec.md` (`NFR-010`) (always log non-2xx; log 2xx only when `-v`; include `<log>` when applicable, selected zip part, status, duration, `X-Forwarded-Host`/`X-Forwarded-Proto` when present)
- [x] T036 [US2] Add compatibility smoke test in `internal/ct-archive-serve/compat_test.go` using `httptest` and an **independent** minimal HTTP client (no reuse of other internal repositories) per `spec.md` (`NFR-012`)

---

## Phase 5: Performance (Extreme Load)

**Goal**: Keep request hot path hardware-limited under large working set.

**Independent Test**: Benchmarks/profiles show request-path CPU time dominated by zip decompression + disk reads + network writes, not avoidable overhead (rescans, central-dir reparse, lock contention) per `spec.md` (`SC-006`).

- [x] T037 [P] Implement bounded `ZipPartCache` (LRU/eviction; cap open zip parts via `CT_ZIP_CACHE_MAX_OPEN`) in `internal/ct-archive-serve/zip_cache.go`
- [x] T038 Integrate `ZipPartCache` into `internal/ct-archive-serve/zip_reader.go` and `internal/ct-archive-serve/server.go` to avoid repeated central directory parsing on hot zip parts per `spec.md` (`SC-006`)
- [x] T039 [P] Add a targeted concurrent cache test in `internal/ct-archive-serve/zip_cache_test.go` and verify CI runs `go test -race ./...` (at least on linux/amd64)
- [x] T040 Add large working-set benchmarks in `internal/ct-archive-serve/perf_bench_test.go` and include `pprof` guidance in comments for CPU/alloc/mutex profiling

---

## Final Phase: Polish & Cross-Cutting

- [x] T041 Update `internal/ct-archive-serve/README.md` with: env vars, routing summary, zip integrity behavior (503), logging policy, metrics policy, and performance tuning/benchmark commands
- [x] T042 Update top-level `README.md` if needed to reflect any CLI or operational changes introduced during implementation
- [x] T043 Record completion notes in `CHANGES.md` (most recent entry first, with date and bullet list)
- [x] T044 Verify NFR-011 security gates are executed in CI (`.github/workflows/ci.yml` runs `golangci-lint`, `govulncheck`, and `trivy`)
- [x] T045 Verify NFR-013 build/release workflows are present (`.github/workflows/ci.yml`, `.github/workflows/image.yml`) and image publishing targets `ghcr.io/${{ github.repository }}`
- [x] T046 Verify NFR-014 container operation examples exist and are documented (`compose.yml`, `README.md` docker run + compose/podman compose examples)
- [x] T047 Verify NFR-015 container defaults are safe-by-default (`Dockerfile` runs as `nobody/nogroup` and exposes/listens on TCP/8080; README documents `-p 80:8080`)

---

## Dependencies & Execution Order

- **Phase 1 → Phase 2**: required
- **Phase 2 → US1/US2**: required
- **US1 and US2**: can be implemented in parallel after Phase 2, but both depend on Phase 2
- **Performance phase**: depends on functional correctness of US1/US2 (especially hot-path routing and zip entry serving)
