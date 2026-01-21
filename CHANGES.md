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

