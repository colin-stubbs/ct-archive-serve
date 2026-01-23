# ct-archive-serve

`ct-archive-serve` is a focused repository for building **`ct-archive-serve`**: an HTTP server that serves **Static-CT (tiled)** assets directly from **`photocamera-archiver`** zip parts on disk, without requiring extraction.

## What it does

- Serves **Static-CT assets** for each archived log under `/<log>/...` (checkpoint, tiles, issuers, etc.)
- Generates and serves a **discovery log list** at `GET /logs.v3.json` (compatible with common log list v3 consumers)
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

## Development

Implementation code lives under:

```text
cmd/ct-archive-serve/        # CLI entrypoint
internal/ct-archive-serve/   # Core implementation
```

Go version:

- Target runtime: **Go 1.25.5+**

### CLI Flags

- `-h`, `--help`: Show help and exit
- `-v`, `--verbose`: Enable verbose logging (log successful HTTP 2xx responses)
- `-d`, `--debug`: Enable debug logging (slog DEBUG level)

### Configuration

All configuration is via environment variables. See `internal/ct-archive-serve/README.md` for details.

CI/CD and artifacts:

- GitHub Actions is used for CI (`.github/workflows/ci.yml`).
- A container image is built and published to GHCR (`ghcr.io/<owner>/<repo>`) on successful builds (`.github/workflows/image.yml`).

## Running via containers

You can operate `ct-archive-serve` entirely via containers. The server expects an archive directory on disk containing `ct_*` folders with `000.zip`, `001.zip`, etc.

If a zip part exists but fails basic zip integrity checks (common while a torrent download is still in progress), `ct-archive-serve` returns HTTP `503` for requests that require that zip part. Failed zip parts are re-tried after `CT_ZIP_INTEGRITY_FAIL_TTL` (default `5m`).

If `/logs.v3.json` refresh fails (e.g., due to unreadable `000.zip` or invalid `log.v3.json`), `ct-archive-serve` returns HTTP `503` for `GET /logs.v3.json` until the next successful refresh.

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

This repository includes `compose.yml` as a quick-start example for `ct-archive-serve` on it's own.

The repository also includes `compose-all.yml` as a quick-start example for running:
1. `ct-archive-serve`
2. `qBittorrent` (server only, no X) to download the CT archives
3. `Prometheus` to collect and monitor the `ct-archive-serve` metrics

```bash
docker compose up
```

```bash
docker compose -f ./compose-all.yml up
```

Or:

```bash
podman compose up
```

```bash
podman compose -f ./compose-all.yml up
```

Then access:

- `GET http://localhost:8080/logs.v3.json`
- `GET http://localhost:8080/metrics`
- Prometheus UI: `http://localhost:9090` (if Prometheus service is enabled, e.g. `compose-all.yml`)
- qBittorrent UI: `http://localhost:8081` (if qBittorrent service is enabled, e.g. `compose-all.yml`)

The `compose-all.yml` includes an optional Prometheus service that automatically scrapes metrics from `ct-archive-serve`. The Prometheus configuration is located in `prometheus/prometheus.yml` and is automatically loaded when the Prometheus container starts.

Local tooling:

- `make test`
- `make lint` (requires `golangci-lint`)
- `make security` (optionally uses `govulncheck` and `trivy` if installed)

# Podman/systemd Quadlets

Example systemd quadlets, intended for use with Podman based containers running as an unprivileged non-root user, are available under the `systemd` folder.

## License

TBD
