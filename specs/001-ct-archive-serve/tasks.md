# Tasks: ct-archive-serve

**Feature**: `001-ct-archive-serve`  
**Spec**: `specs/001-ct-archive-serve/spec.md`  
**Plan**: `specs/001-ct-archive-serve/plan.md`

**Note**: Task IDs are stable identifiers; they are not guaranteed to be numerically ordered in the file.

## Dependency Graph (User Stories)

- **US1 (P1)**: Serve `GET /monitor.json` generated from discovered archives
  - Depends on foundational archive discovery + zip entry reading
- **US2 (P1)**: Serve Static-CT assets from archive zips (`/<log>/...`)
  - Depends on foundations

## Parallel Execution Examples

- **US1**:
  - [P] Implement monitor list builder (`internal/ct-archive-serve/monitor_json.go`) in parallel with
  - [P] Implement archive discovery (`internal/ct-archive-serve/archive_index.go`)
- **US2**:
  - [P] Implement `<log>` path parsing + validation (`internal/ct-archive-serve/routing.go`) in parallel with
  - [P] Implement tile/data/issuer parsing + zip selection helpers (`internal/ct-archive-serve/routing.go`, `internal/ct-archive-serve/archive_index.go`)

## Phase 1 — Setup

- [ ] T001 Create ct-archive-serve directories `cmd/ct-archive-serve/` and `internal/ct-archive-serve/` (create placeholder files in `cmd/ct-archive-serve/main.go` and `internal/ct-archive-serve/README.md`)
- [ ] T002 Align documentation routes and examples with `<log>` prefix stripping in `specs/001-ct-archive-serve/contracts/http.md` and `specs/001-ct-archive-serve/quickstart.md`
- [x] T003 Remove duplicate FR numbering for clarity in `specs/001-ct-archive-serve/spec.md`

## Phase 2 — Foundational (blocking prerequisites)

- [ ] T004 Define environment configuration contract in `internal/ct-archive-serve/config.go` (CT_ARCHIVE_PATH default `/var/log/ct/archive`, CT_ARCHIVE_FOLDER_PATTERN default `ct_*`, validate `CT_ARCHIVE_FOLDER_PATTERN` is of supported `<prefix>*` form and derive strip-prefix behavior per `spec.md` (`FR-003a`), `CT_MONITOR_JSON_REFRESH_INTERVAL` per `spec.md` (`FR-007`), performance env vars per `spec.md` (`FR-011`), zip integrity env vars per `spec.md` (`FR-013`) including `CT_ZIP_INTEGRITY_FAIL_TTL`, HTTP server timeout/limit env vars per `spec.md` (`FR-012`) including `CT_HTTP_TRUSTED_SOURCES` parsing/validation, verbose/debug flags plumbing)
- [ ] T005 Implement archive discovery + index in `internal/ct-archive-serve/archive_index.go` (discover folders under CT_ARCHIVE_PATH matching pattern; map request `<log>` → folder path; enumerate zip parts like `000.zip`)
- [ ] T006 Implement safe request parsing + routing primitives in `internal/ct-archive-serve/routing.go` (extract `<log>` segment, normalize remaining path, reject traversal, map `<log>` → archive folder)
- [ ] T007 Implement seekable zip entry streaming helper in `internal/ct-archive-serve/zip_reader.go` using `archive/zip` (`zip.OpenReader`) to locate entry by name and stream only that entry’s decompressed bytes
- [ ] T008 Wire HTTP handler skeleton, metrics endpoint, and Content-Type mapping in `internal/ct-archive-serve/server.go` (serve `/metrics` for Prometheus; enforce HTTP method policy per `spec.md` `FR-002a` (allow `GET`/`HEAD`, return `405` + `Allow: GET, HEAD` for others); set appropriate `Content-Type`: `*.json` → `application/json`, `/checkpoint` → `text/plain; charset=utf-8`, tiles → `application/octet-stream`, issuers → `application/pkix-cert`; return `404` on missing content)
- [ ] T009 Add CLI entrypoint in `cmd/ct-archive-serve/main.go` (support `-h|--help|-v|--verbose|-d|--debug`; start server on `:8080` by default; configure safe `http.Server` timeouts/limits from env vars per `spec.md` (`FR-012`); print help text including env vars)
- [ ] T010 Add unit tests for routing + traversal rejection in `internal/ct-archive-serve/routing_test.go`
- [ ] T011 Add unit tests for tile index parsing/validation in `internal/ct-archive-serve/routing_test.go`
- [ ] T012 Add unit tests for zip entry read behavior using temp zip files in `internal/ct-archive-serve/zip_reader_test.go`
- [ ] T056 Add unit tests for HTTP method policy in `internal/ct-archive-serve/server_test.go` (GET+HEAD supported; other methods return `405` and include `Allow: GET, HEAD` per `spec.md` `FR-002a`)
- [ ] T054 Add zip integrity verification and caching for zip parts in `internal/ct-archive-serve/zip_cache.go` / `internal/ct-archive-serve/zip_reader.go` (structural validity check: `zip.OpenReader` + iterate `r.File` and `Open()`/`Close()` each entry to validate local headers without reading payload; maintain passed set for process lifetime; maintain failed set with TTL `CT_ZIP_INTEGRITY_FAIL_TTL` default `5m`; on integrity failure return HTTP `503` for requests that require the zip part per `spec.md` (`FR-013`))
- [ ] T055 Add unit tests for zip integrity behavior in `internal/ct-archive-serve/zip_reader_test.go` / `internal/ct-archive-serve/zip_cache_test.go` (simulate incomplete/corrupt zip; ensure `503` is returned; ensure failed-zip TTL allows re-test; ensure passed entries persist until a read failure evicts them)
- [ ] T045 Implement low-cardinality Prometheus metrics in `internal/ct-archive-serve/metrics.go` and integrate into `internal/ct-archive-serve/server.go` (track `/monitor.json` request count+duration; track per-`<log>` aggregate request count+duration for all `/<log>/...` serving combined; metrics MUST NOT be labeled by full path, endpoint, tile coordinates, or status code per `spec.md` (`NFR-009`))
- [ ] T046 Add unit tests for `/metrics` output and metric label cardinality in `internal/ct-archive-serve/metrics_test.go` (assert metrics exist; assert only `log` is used as a label where applicable)
- [ ] T047 Add structured JSON logging for `ct-archive-serve` using `log/slog` in `internal/ct-archive-serve/logger.go` and wire it through `cmd/ct-archive-serve/main.go` / `internal/ct-archive-serve/server.go` (log to stdout/stderr; `-v/-d` control log level; request logging policy per `spec.md` `NFR-010`: only log HTTP 2xx requests when `-v` is enabled; always log non-2xx; include `<log>`, selected zip part, status, duration, `X-Forwarded-Host`/`X-Forwarded-Proto` (when present), and errors without leaking sensitive data)
- [ ] T041 Add unit tests for config parsing defaults + validation in `internal/ct-archive-serve/config_test.go` (include `CT_HTTP_*` including `CT_HTTP_TRUSTED_SOURCES` and `CT_ZIP_CACHE_MAX_OPEN` parsing)
- [ ] T042 Add unit tests for subtree zip selection math in `internal/ct-archive-serve/archive_index_test.go` (verify `(L,N) -> zipIndex` for L=0/1/2 and data tiles per `spec.md` `FR-008`)

## Phase 3 — User Story 1 (P1): Serve monitor.json for discovery

**Goal**: Serve `GET /monitor.json` containing a log list generated from each discovered archive folder’s `000.zip` → `log.v3.json`.

**Independent test criteria**:
- With multiple `ct_*` folders present, `GET /monitor.json` returns `200` `application/json` and includes one `tiled_logs` entry per folder (with `<log>` prefix stripped).
- With no valid `000.zip` + `log.v3.json`, `GET /monitor.json` returns `200` `application/json` with empty `tiled_logs`.

- [ ] T013 [P] [US1] Wire the `CT_MONITOR_JSON_REFRESH_INTERVAL` configuration into monitor.json generation/refresh behavior in `internal/ct-archive-serve/monitor_json.go` (parsing/validation lives in `T004`; this task ensures the refresh loop uses the configured value and tests cover the behavior)
- [ ] T014 [US1] Implement monitor.json source builder in `internal/ct-archive-serve/monitor_json.go` (for each discovered folder: open `000.zip`, extract+parse `log.v3.json`, ensure the entry is static-ct-api only by removing/clearing `url`, set `has_issuers=true` iff `000.zip` contains any `issuer/` entries, and preserve other fields required by log list v3 consumers; do NOT hardcode/validate hostname or scheme)
- [ ] T015 [US1] Implement periodic refresh loop + atomic snapshot in `internal/ct-archive-serve/monitor_json.go` (build at startup, then refresh per interval; store a snapshot of per-log metadata needed to render a valid log list v3 JSON document including the snapshot time for `log_list_timestamp`; loop MUST be `context.Context`-driven and stop cleanly on shutdown)
- [ ] T016 [US1] Add `GET /monitor.json` handler in `internal/ct-archive-serve/server.go` (derive `publicBaseURL` from request `Host`/`X-Forwarded-Host` and `X-Forwarded-Proto` per `spec.md` (`FR-006`) and `CT_HTTP_TRUSTED_SOURCES`, render log list v3 JSON with `version="3.0"` and a single operator `name="ct-archive-serve"`/`email=[]`/`logs=[]`, and serve with `application/json`)
- [ ] T048 [US1] Add unit tests for `publicBaseURL` derivation in `internal/ct-archive-serve/server_test.go` (comma-separated `X-Forwarded-Host`/`X-Forwarded-Proto` parsing, whitespace trimming, fallback to `Host`, scheme lowercasing, and `CT_HTTP_TRUSTED_SOURCES`-gated trust of `X-Forwarded-*` per `spec.md` `FR-006`)

## Phase 4 — User Story 2 (P1): Serve Static-CT assets from archive zips

**Goal**: Serve `GET /<log>/...` assets directly from archive zip parts:
- `/<log>/checkpoint`
- `/<log>/log.v3.json`
- `/<log>/tile/<L>/<N>[.p/<W>]`
- `/<log>/tile/data/<N>[.p/<W>]`
- `/<log>/issuer/<fingerprint>`

**Independent test criteria**:
- With a folder like `ct_trustasia_log2024/000.zip` present under `CT_ARCHIVE_PATH`, `GET /trustasia_log2024/checkpoint` returns `200` with `Content-Type: text/plain; charset=utf-8`.
- With at least one existing tile entry in the archive, a matching `GET /<log>/tile/...` returns `200` with `Content-Type: application/octet-stream`.
- With at least one existing data tile and issuer entry in the archive, matching `GET /<log>/tile/data/...` and `GET /<log>/issuer/...` return `200` with correct types.
- Missing `<log>` or missing entries return `404`.

- [ ] T017 [US2] Implement entry selection policy for shared metadata in `internal/ct-archive-serve/archive_index.go` (prefer `000.zip`, else lowest available zip part)
- [ ] T018 [US2] Implement `/<log>/checkpoint` handler in `internal/ct-archive-serve/server.go` (serve zip entry `checkpoint`)
- [ ] T019 [US2] Implement `/<log>/log.v3.json` handler in `internal/ct-archive-serve/server.go` (serve zip entry `log.v3.json`)
- [ ] T020 [US2] Add request logging behavior in `internal/ct-archive-serve/server.go` for `/<log>/...` handlers using the structured JSON logger introduced in `T047` (ensure it complies with `spec.md` `NFR-010`)

- [ ] T021 [P] [US2] Implement tile index decoding (tlog "groups-of-three" decimal path encoding; see `spec.md` `FR-008a`) in `internal/ct-archive-serve/routing.go`
- [ ] T022 [P] [US2] Implement hash-tile path parsing (including `.p/<W>`) in `internal/ct-archive-serve/routing.go` (validate partial width: `W` must be 1..255; full tiles use the non-`.p/` form)
- [ ] T023 [US2] Implement zip-part selection for hash tiles in `internal/ct-archive-serve/archive_index.go` per `spec.md` (`FR-008`) (L0: `zipIndex=N/65536`; L1: `zipIndex=N/256`; L2: `zipIndex=N`; L>=3: use shared-metadata zip selection)
- [ ] T024 [US2] Implement `/<log>/tile/<L>/...` handler in `internal/ct-archive-serve/server.go` (map request → zip entry `tile/<L>/...` and stream via `zip_reader.go`)
- [ ] T025 [P] [US2] Implement data-tile path parsing (including `.p/<W>`) in `internal/ct-archive-serve/routing.go` (validate partial width: `W` must be 1..255; full tiles use the non-`.p/` form)
- [ ] T026 [US2] Implement zip-part selection for data tiles in `internal/ct-archive-serve/archive_index.go` per `spec.md` (`FR-008`) (`zipIndex = N/65536`)
- [ ] T027 [US2] Implement `/<log>/tile/data/...` handler in `internal/ct-archive-serve/server.go` (map request → zip entry `tile/data/...` and stream)
- [ ] T028 [US2] Implement issuer path validation + routing in `internal/ct-archive-serve/routing.go` (accept lowercase hex SHA-256 fingerprint; map to `issuer/<fingerprint>`)
- [ ] T029 [US2] Implement `/<log>/issuer/...` handler in `internal/ct-archive-serve/server.go` (serve zip entry `issuer/<fingerprint>`)
- [ ] T030 [US2] Add compatibility smoke test using an **independent** minimal Static-CT (C2SP/tiled) HTTP client against `ct-archive-serve` handler in `internal/ct-archive-serve/compat_test.go` (use `httptest` + temp zip archives; fetch checkpoint + at least one tile/issuer when present; do not import code from other internal/external tool repositories per `spec.md` `NFR-012`)
- [ ] T043 [US2] Add right-edge partial tile contract tests in `internal/ct-archive-serve/routing_test.go` / `internal/ct-archive-serve/server_test.go` (reject invalid `.p/<W>` widths; `200` only when the exact `tile/.../.p/<W>` entry exists; no synthesis based on checkpoint size)

## Phase 5 — Performance (Extreme Load)

**Goal**: Ensure `ct-archive-serve` remains hardware-limited under extreme load, including large working-set request patterns.

**Independent test criteria**:
- Benchmarks and/or profiles show request-path time is dominated by zip decompression + disk I/O + network writes, not avoidable overhead (directory rescans, central-directory re-parsing, lock contention).

- [ ] T031 [P] Plumb performance tuning configuration into the performance subsystems (use `CT_ZIP_CACHE_MAX_OPEN` and `CT_ARCHIVE_REFRESH_INTERVAL` values from config parsing noted in `T004`) and document defaults per `spec.md` (`FR-011`)
- [ ] T032 Implement archive index refresh loop in `internal/ct-archive-serve/archive_index.go` (periodic refresh controlled by `CT_ARCHIVE_REFRESH_INTERVAL`; request hot path uses an in-memory snapshot and MUST NOT rescan disk; loop MUST be `context.Context`-driven and stop cleanly on shutdown)
- [ ] T033 [P] Implement bounded `ZipPartCache` in `internal/ct-archive-serve/zip_cache.go` (LRU/eviction; cap open zip parts via `CT_ZIP_CACHE_MAX_OPEN`; cache central-directory-derived entry index per zip part)
- [ ] T034 Integrate `ZipPartCache` into `internal/ct-archive-serve/server.go` and `internal/ct-archive-serve/zip_reader.go` (serve entries via cached zip state; avoid repeated central directory parsing on hot parts; preserve streaming behavior)
- [ ] T044 [P] Add explicit concurrency-safety verification for caches/indices (run `go test -race ./...` in CI and add at least one targeted concurrent test for `ZipPartCache` in `internal/ct-archive-serve/zip_cache_test.go`)
- [ ] T035 Add large-working-set benchmarks in `internal/ct-archive-serve/perf_bench_test.go` (many distinct requests across many zip parts) and include `pprof` guidance in comments for CPU/alloc profiling
- [ ] T036 Document performance tuning + benchmark commands in `internal/ct-archive-serve/README.md` (what the env vars do; how to validate `SC-006`)

## Final Phase — Polish & cross-cutting

- [ ] T037 Add minimal package-level documentation in `internal/ct-archive-serve/README.md` (env vars, routing, zip seekability expectations, monitor.json behavior, and how to run lint/security checks for this tool: `make lint` and `make security`)
- [ ] T038 Update top-level docs to mention new tool in `README.md` (add `ct-archive-serve` under Tools with environment variables and routing shape)
- [ ] T039 Add build tooling for this repository in `Makefile` (targets: `build-ct-archive-serve`, `test`, `lint`, `security`)
- [ ] T049 Add GitHub Actions CI workflow in `.github/workflows/ci.yml` (public GitHub repo; run `go test ./...` + linting on pull requests and pushes per `spec.md` `NFR-013`)
- [ ] T050 Add container build+publish workflow to GHCR in `.github/workflows/image.yml` (build and push `ghcr.io/<owner>/<repo>` on default branch and tags per `spec.md` `NFR-013`)
- [ ] T051 Add `Dockerfile` for building a `ct-archive-serve` container image (multi-stage; uses Go 1.25.5+ toolchain; produces runnable image; runs as `nobody/nogroup`; exposes/listens on TCP/8080 by default per `spec.md` `NFR-015`)
- [ ] T052 Add repo-root `compose.yml` to demonstrate running `ct-archive-serve` via `docker compose` or `podman compose` (volume mount for `CT_ARCHIVE_PATH`, port mapping to container TCP/8080, env var examples; per `spec.md` `NFR-014`/`NFR-015`)
- [ ] T053 Update `README.md` to document container-based operation (GHCR image pull, `docker run`, `docker compose` and `podman compose` examples; per `spec.md` `NFR-014`)
- [ ] T040 Record completion notes in `CHANGES.md` (most recent entry, with date and summary)

