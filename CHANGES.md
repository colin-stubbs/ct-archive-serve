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

