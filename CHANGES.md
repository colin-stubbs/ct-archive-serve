* 2026-01-21 - Add Prometheus configuration for metrics collection

- Created `prometheus/prometheus.yml` configuration file that automatically discovers and scrapes metrics from ct-archive-serve
- Updated `compose.yml` Prometheus service configuration (fixed duplicate comment, verified service connectivity)
- Updated `spec.md` NFR-014 to document Prometheus integration in compose.yml
- Updated `README.md` to mention Prometheus UI access and configuration

* 2026-01-21 - Rename MONITOR_JSON references to LOGLISTV3_JSON throughout codebase

- Renamed environment variable `CT_MONITOR_JSON_REFRESH_INTERVAL` to `CT_LOGLISTV3_JSON_REFRESH_INTERVAL`
- Renamed config field `MonitorJSONRefreshInterval` to `LogListV3JSONRefreshInterval`
- Renamed types: `MonitorJSONBuilder` → `LogListV3JSONBuilder`, `MonitorJSONSnapshot` → `LogListV3JSONSnapshot`, `MonitorJSONOperator` → `LogListV3JSONOperator`, `MonitorJSONTiledLog` → `LogListV3JSONTiledLog`
- Renamed function `NewMonitorJSONBuilder` → `NewLogListV3JSONBuilder`
- Renamed metrics: `monitorJSONRequestsTotal` → `logListV3JSONRequestsTotal`, `monitorJSONRequestDuration` → `logListV3JSONRequestDuration`
- Renamed metric method `ObserveMonitorJSONRequest` → `ObserveLogListV3JSONRequest`
- Updated all variable names from `monitorJSON` to `logListV3JSON`
- Renamed files: `monitor_json.go` → `loglistv3_json.go`, `monitor_json_test.go` → `loglistv3_json_test.go`
- Renamed test functions: `TestMonitorSnapshotBuilder_*` → `TestLogListV3JSONBuilder_*`
- Renamed test helper: `mustCreateZipForMonitor` → `mustCreateZipForLogListV3`
- Updated all file references in `specs/001-ct-archive-serve/tasks.md` and `plan.md`
- Updated documentation (spec.md, plan.md, tasks.md, README.md, quickstart.md, compose.yml)
- Updated terminology: "monitor JSON" → "loglist v3 JSON" throughout

* 2026-01-21 - Add loglist3 validation requirement and tests for logs.v3.json

- Updated `spec.md` `FR-006` to require validation of generated `/logs.v3.json` output using the `loglist3` library from `github.com/google/certificate-transparency-go/loglist3`
- Added `TestLogListV3JSONBuilder_LogListV3Validation` test that validates generated logs.v3.json can be parsed by loglist3 library
- Updated `TestCompatibility_SmokeTest` to use loglist3 validation for logs.v3.json endpoint
- Updated test data to use valid base64-encoded values for `log_id` and `key` fields (required by loglist3 schema)
- Added dependency on `github.com/google/certificate-transparency-go` v1.3.2

* 2026-01-21 - Align ZIP integrity verification scope; document trusted forwarded headers

- Updated `spec.md` `FR-013` to require ZIP structural validity checks including central directory/EOCD and local file header verification (without reading entry bodies)
- Updated `specs/001-ct-archive-serve/contracts/http.md` to document `CT_HTTP_TRUSTED_SOURCES` gating for `X-Forwarded-*` in `/logs.v3.json` URL formation

* 2026-01-21 - Clarify forwarded-header trust gating; specify issuer fingerprint validation

- Updated `specs/001-ct-archive-serve/plan.md` to explicitly describe `CT_HTTP_TRUSTED_SOURCES` gating for `X-Forwarded-*` during `/logs.v3.json` URL formation
- Updated `specs/001-ct-archive-serve/spec.md` Edge Cases to specify issuer `<fingerprint>` validation (non-empty lowercase hex; otherwise `404`)

* 2026-01-22 - Complete Phase 4-5 implementation: tile/issuer handlers, request logging, compatibility tests, ZipPartCache performance optimization

- Implemented hash tile handler `/<log>/tile/<L>/<N>[.p/<W>]` (T031): supports all tile levels, partial tiles, correct zip selection math
- Implemented data tile handler `/<log>/tile/data/<N>[.p/<W>]` (T032): correct zip selection, partial tile support
- Implemented issuer handler `/<log>/issuer/<fingerprint>` (T034): validation, correct Content-Type, 404/503 handling
- Implemented HTTP request logging (T035): structured JSON logs, verbose mode for 2xx, always log non-2xx, includes X-Forwarded-* headers
- Added compatibility smoke test (T036): verifies all major endpoints work correctly
- Implemented ZipPartCache (T037-T038): bounded LRU cache for open zip handles and entry indices, avoids repeated central directory parsing
- Added concurrent cache tests with race detection (T039): verifies thread-safety under concurrent access
- Added performance benchmarks (T040): zip reader and cache benchmarks with pprof guidance
- Updated internal README.md (T041): comprehensive documentation of env vars, routing, logging, metrics, performance tuning
- Updated top-level README.md (T042): added CLI flags section, improved development documentation

* 2026-01-21 - Implement Phase 2-3 core functionality: HTTP server, monitor.json, checkpoint/log.v3.json handlers

- Implemented HTTP method policy enforcement (T017-T018): GET/HEAD support, 405 for unsupported methods, 404 for unknown routes
- Implemented CLI flags and process wiring (T019): -h/--help, -v/--verbose, -d/--debug, HTTP server timeouts/limits configuration
- Implemented publicBaseURL derivation (T020-T021): trusted-source gating, X-Forwarded-* header handling, comma-separated list parsing
- Implemented logs.v3.json builder (T022-T025): extract log.v3.json, check has_issuers, deterministic sort, periodic refresh loop, 503 on refresh failure
- Implemented zip selection math (T026-T027): hash tiles (L=0/1/2), data tiles, shared metadata (L>=3) selection
- Implemented checkpoint and log.v3.json handlers (T028-T029): serve from 000.zip with correct Content-Type, 404/503 handling, HEAD support
- Added GetAllLogs() method to ArchiveIndex for logs.v3.json building
- Added SelectZipPart() method to ArchiveIndex for tile zip selection

* 2026-01-21 - Clean up plan.md: reduce duplication and clarify implementation notes

- Consolidated duplicate zip integrity descriptions across Summary, Constraints, Implementation Notes, and Core components sections
- Clarified X-Forwarded-* header trust mechanism in Implementation Notes (explains `CT_HTTP_TRUSTED_SOURCES` IP matching)
- Improved formatting and consistency in Implementation Notes section (added bold labels for each note)
- Made zip integrity verification description more concise while maintaining clarity

* 2026-01-21 - Enhance LoadConfig documentation with usage pattern

- Enhanced `LoadConfig()` function documentation in `internal/ct-archive-serve/config.go` with detailed usage example, error handling guidance, and clarification on when to use it vs `parseConfigFromMap` for testing

* 2026-01-21 - Recreate missing zip_reader.go; add public LoadConfig API

- Recreated `internal/ct-archive-serve/zip_reader.go` with `NewZipReader` and `OpenEntry` implementation matching test API (fixes blocking tasks T015/T016/T031/T032/T038)
- Added public `LoadConfig()` function in `internal/ct-archive-serve/config.go` for production use (reads from `os.LookupEnv`)

* 2026-01-21 - Complete API requirements checklist; clarify archive layout and request→zip entry mapping

- Updated `specs/001-ct-archive-serve/spec.md` to clarify: `NNN.zip` naming convention, startup refresh expectation for `/monitor.json`, tile level `<L>` bounds, and explicit request-suffix → zip-entry mapping for `/<log>/...` endpoints
- Completed `specs/001-ct-archive-serve/checklists/api.md` (PR gate) with notes linking each checklist item to the relevant spec sections

* 2026-01-21 - Clarify monitor.json refresh and log collision behavior; add observability tasks

- Updated `spec.md` to define `/monitor.json` refresh failure behavior (`503` until the next successful refresh) and to define `<log>` collision handling as a startup configuration error
- Updated `plan.md` and `tasks.md` to add explicit tasks for resource observability gauges and for tests covering refresh failures and `<log>` collisions
- Updated CI to run `go test -race ./...` to align with concurrency-safety verification in the task plan

* 2026-01-21 - Enforce Trivy scanning in CI and close NFR-013/014/015 task gaps

- Updated `.github/workflows/ci.yml` to run Trivy (`aquasecurity/trivy-action`) as part of CI, alongside `golangci-lint` and `govulncheck` (NFR-011)
- Updated `specs/001-ct-archive-serve/tasks.md` to explicitly track/verify existing CI workflows and container artifacts (NFR-013/014/015)

* 2026-01-21 - Clean up constitution scope for ct-archive-serve

- Updated `.specify/memory/constitution.md` to align “integrity verification” with `ct-archive-serve` (ZIP structural integrity checks only; Merkle/inclusion verification is out of scope for serving paths)
- Made outbound network-call guidance conditional to avoid imposing CT-client retry/backoff requirements on a filesystem-backed HTTP server
- Deleted legacy `.speckit.constitution` (unused by speckit commands and contained ctlogtools-specific placeholders/guidance)

* 2026-01-21 - Regenerate implementation task list for ct-archive-serve

- Rewrote `specs/001-ct-archive-serve/tasks.md` to be dependency-ordered by phase/user story, with strictly sequential task IDs and required `[P]`/`[US#]` labels
- Added/clarified tasks to align with current `spec.md` and `contracts/http.md` (method policy, trusted forwarded headers, zip integrity 503 behavior, and low-cardinality metrics)

* 2026-01-21 - Restore plan.md alignment with latest spec decisions

- Restored missing `plan.md` details for HTTP method policy (`FR-002a`), deterministic `/monitor.json` ordering, tile `<N>` encoding (`FR-008a`), and zip-integrity-driven `503` behavior (`FR-013`)
- Updated the planned source file list in `plan.md` to include `zip_cache.go`, `metrics.go`, and `logger.go`
- Updated the `plan.md` Summary to explicitly mention the `503` behavior for incomplete zip parts

* 2026-01-21 - Define zip integrity verification and 503 behavior for in-progress torrent downloads

- Added `FR-013` zip integrity rules (passed/failed caches, 5m failed TTL via `CT_ZIP_INTEGRITY_FAIL_TTL`, 503 on integrity failure)
- Updated HTTP contract and checklist to document 503 behavior for temporarily unavailable zip parts
- Added tasks to implement and test zip integrity caching and expiry semantics
- Updated spec docs (`plan.md`, `research.md`, `quickstart.md`) to describe the structural validity check approach for zip integrity

* 2026-01-21 - Define HTTP method policy and reduce task duplication

- Added explicit HTTP method policy (`GET`/`HEAD`; other methods return `405` with `Allow: GET, HEAD`)
- Deduplicated tasks by making `T013` and `T031` focus on wiring/usage rather than env var definition/parsing

* 2026-01-21 - Clarify Static-CT tile URL encoding and deterministic monitor.json output

- Made `/monitor.json` output deterministic by requiring a stable `tiled_logs` ordering
- Fixed `/monitor.json` example URLs to match the specified `publicBaseURL + "/<log>"` rule
- Specified the standard tlog "groups-of-three" tile index path encoding and aligned contracts/tasks/checklist

* 2026-01-21 - Default container port 8080 and non-root runtime

- Updated `ct-archive-serve` container defaults to listen on TCP/8080 and run as `nobody/nogroup`
- Updated `Dockerfile` to `EXPOSE 8080` and set `USER 65534:65534`
- Updated `compose.yml`, `README.md`, and spec/plan/tasks docs to reflect port publishing (e.g. `-p 80:8080`)

* 2026-01-21 - Migrate ct-archive-serve spec set into ct-archive-serve repo

- Relocated the ct-archive-serve specification, plan, tasks, and supporting docs into `specs/001-ct-archive-serve/`
- Updated internal references from the prior `007-*` naming to `001-ct-archive-serve` across spec artifacts
- Aligned requirements to permit upstream CT libraries for Static-CT/C2SP interactions while keeping stdlib-first guidance
- Added `go.mod`, `.golangci.yml`, and a minimal `Makefile` to support `go test`, `golangci-lint`, and optional `govulncheck`/`trivy` checks in this repository
- Updated documented Go target/runtime to Go 1.25.5+ (spec plan, constitution, README, tooling comments)
- Added GitHub Actions workflows to validate/test/lint and to build+publish a container image to GHCR
- Added a `Dockerfile` and minimal placeholder `cmd/` + `internal` scaffolding to support early CI and image builds
- Added `compose.yml` and documented container-based operation in `README.md` (docker run + docker/podman compose examples)
- Updated repo `README.md` to describe purpose and spec-driven workflow

