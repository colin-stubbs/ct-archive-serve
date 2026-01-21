* 2026-01-21 - Align ZIP integrity verification scope; document trusted forwarded headers

- Updated `spec.md` `FR-013` to require ZIP structural validity checks including central directory/EOCD and local file header verification (without reading entry bodies)
- Updated `specs/001-ct-archive-serve/contracts/http.md` to document `CT_HTTP_TRUSTED_SOURCES` gating for `X-Forwarded-*` in `/monitor.json` URL formation

* 2026-01-21 - Clarify forwarded-header trust gating; specify issuer fingerprint validation

- Updated `specs/001-ct-archive-serve/plan.md` to explicitly describe `CT_HTTP_TRUSTED_SOURCES` gating for `X-Forwarded-*` during `/monitor.json` URL formation
- Updated `specs/001-ct-archive-serve/spec.md` Edge Cases to specify issuer `<fingerprint>` validation (non-empty lowercase hex; otherwise `404`)

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

