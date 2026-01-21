# ct-archive-serve

`ct-archive-serve` is a focused repository for building **`ct-archive-serve`**: an HTTP server that serves **Static-CT (tiled)** assets directly from **`photocamera-archiver`** zip parts on disk, without requiring extraction.

## What it does

- Serves **Static-CT assets** for each archived log under `/<log>/...` (checkpoint, tiles, issuers, etc.)
- Generates and serves a **discovery log list** at `GET /monitor.json` (compatible with common log list v3 consumers)
- Exposes Prometheus metrics at `GET /metrics` (low-cardinality by design)

## Repository workflow (spec-driven)

This repo uses `.specify/` + `specs/<feature>/` for spec-driven development.

- Current feature spec set: `specs/001-ct-archive-serve/`
  - `spec.md`: requirements and clarifications
  - `plan.md`: implementation plan
  - `tasks.md`: task breakdown
  - Supporting docs: `contracts/`, `checklists/`, `quickstart.md`, `research.md`, `data-model.md`

### Speckit usage

The `.specify/scripts/bash/check-prerequisites.sh` helper locates the active spec directory based on the **numeric branch prefix** (e.g. branch `001-...` maps to `specs/001-*`).

## Development (placeholder)

Implementation code will live under:

```text
cmd/ct-archive-serve/
internal/ct-archive-serve/
```

Go version:

- Target runtime: **Go 1.25.5+**

CI/CD and artifacts:

- GitHub Actions is used for CI (`.github/workflows/ci.yml`).
- A container image is built and published to GHCR (`ghcr.io/<owner>/<repo>`) on successful builds (`.github/workflows/image.yml`).

## Running via containers

You can operate `ct-archive-serve` entirely via containers. The server expects an archive directory on disk containing `ct_*` folders with `000.zip`, `001.zip`, etc.

If a zip part exists but fails basic zip integrity checks (common while a torrent download is still in progress), `ct-archive-serve` returns HTTP `503` for requests that require that zip part. Failed zip parts are re-tried after `CT_ZIP_INTEGRITY_FAIL_TTL` (default `5m`).

### docker run

- Ensure your archive is available on the host, e.g. `./archive/ct_example_log/000.zip`
- Run:

```bash
docker run --rm -p 8080:8080 \
  -v "$(pwd)/archive:/var/log/ct/archive:ro" \
  -e CT_ARCHIVE_PATH=/var/log/ct/archive \
  ghcr.io/<owner>/<repo>:latest
```

To expose it directly on host TCP/80:

```bash
docker run --rm -p 80:8080 \
  -v "$(pwd)/archive:/var/log/ct/archive:ro" \
  -e CT_ARCHIVE_PATH=/var/log/ct/archive \
  ghcr.io/<owner>/<repo>:latest
```

### docker compose / podman compose

This repository includes `compose.yml` as a quick-start example.

```bash
docker compose up --build
```

Or:

```bash
podman compose up --build
```

Then access:

- `GET http://localhost:8080/monitor.json`
- `GET http://localhost:8080/metrics`

Local tooling:

- `make test`
- `make lint` (requires `golangci-lint`)
- `make security` (optionally uses `govulncheck` and `trivy` if installed)

## License

TBD