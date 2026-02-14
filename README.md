# ct-archive-serve

> **Serve Certificate Transparency log archives directly from zip files—no extraction required.**

`ct-archive-serve` is an HTTP server that provides seamless access to Certificate Transparency (CT) log archives stored in the **Static-CT (tiled)** format. It serves data directly from zip archive parts produced by [`photocamera-archiver`](https://github.com/geomys/ct-archive), eliminating the need to extract or duplicate the very large datasets involved.

**Perfect for**: CT log archives distributed via torrents from the [ct-archive](https://github.com/geomys/ct-archive) repository's [torrents.rss](https://github.com/geomys/ct-archive/blob/main/torrents.rss) feed.

**NOTE**: At present a full download of all available archives requires approximately 10-12TB+ of storage **before** extraction of any content from the zip files representing each archive. Simultaneous storage of torrent data and extracted CT log archive data will require 25TB or more of storage space. `ct-archive-serve` allows you to avoid storing anything more than the zip files themselves.

## Table of Contents

- [Quick Start](#quick-start)
- [Features](#features)
- [Installation & Running](#installation--running)
  - [Docker](#docker)
  - [Docker Compose / Podman Compose](#docker-compose--podman-compose)
  - [Systemd Quadlets](#systemd-quadlets)
- [Configuration](#configuration)
  - [Environment Variables](#environment-variables)
  - [CLI Flags](#cli-flags)
- [API Reference](#api-reference)
- [Usage Examples](#usage-examples)
- [Development](#development)
- [Security](#security)
- [License](#license)

## Quick Start

Get `ct-archive-serve` running in 30 seconds:

```bash
docker run --rm -p 8080:8080 \
  -v "$(pwd)/archive:/var/log/ct/archive:ro" \
  -e CT_ARCHIVE_PATH=/var/log/ct/archive \
  ghcr.io/colin-stubbs/ct-archive-serve:latest
```

Then access:
- **Log list**: `http://localhost:8080/logs.v3.json`
- **Metrics**: `http://localhost:8080/metrics`

The server automatically discovers CT log archives in folders matching the pattern `ct_*` containing `000.zip`, `001.zip`, etc.

## Features

### Core Capabilities

- **Static-CT Asset Serving**: Serves checkpoint, tiles, issuers, and other Static-CT assets for each archived log under `/<log>/...`
- **Auto-Discovery**: Generates and serves a discovery log list at `GET /logs.v3.json` (compatible with common log list v3 consumers)
- **Observability**: Exposes Prometheus metrics at `GET /metrics` (low-cardinality by design)
- **Torrent Integration**: Includes qBittorrent configuration to automate download of CT archive torrents

### Key Benefits

- **Storage Efficiency**: Avoid copying and unzipping massive datasets (currently 10TB+), more than halving your storage requirements. Serve directly from where torrent downloads land.
- **Compatibility**: Provides a standard HTTP interface for existing CT log monitoring clients without code changes.
- **Frictionless Seeding**: Encourages contributors to remain on the torrent network as seeds by eliminating the need to maintain a separate, extracted copy of the archive data.

### Behavior Notes

- **Incomplete Downloads**: If a zip part exists but fails basic zip integrity checks (common while a torrent download is in progress), `ct-archive-serve` returns HTTP `503` for requests requiring that zip part. Failed zip parts are re-tried after `CT_ZIP_INTEGRITY_FAIL_TTL` (default `5m`).
- **Refresh Failures**: If `/logs.v3.json` refresh fails (e.g., due to unreadable `000.zip` or invalid `log.v3.json`), `ct-archive-serve` returns HTTP `503` for `GET /logs.v3.json` until the next successful refresh.

## Installation & Running

### Docker

Ensure your archive is available on the host (e.g., `./archive/ct_example_log/000.zip`), then run:

```bash
docker run --rm -p 8080:8080 \
  -v "$(pwd)/archive:/var/log/ct/archive:ro" \
  -e CT_ARCHIVE_PATH=/var/log/ct/archive \
  ghcr.io/colin-stubbs/ct-archive-serve:latest
```

To expose on host TCP/80:

```bash
docker run --rm -p 80:8080 \
  -v "$(pwd)/archive:/var/log/ct/archive:ro" \
  -e CT_ARCHIVE_PATH=/var/log/ct/archive \
  ghcr.io/colin-stubbs/ct-archive-serve:latest
```

### Docker Compose / Podman Compose

This repository includes two compose files:

- **`compose.yml`**: Quick-start example for `ct-archive-serve` standalone
- **`compose-all.yml`**: Full stack including:
  1. `ct-archive-serve` (HTTP server)
  2. `qBittorrent` (server only, no X) to download CT archives
  3. `Prometheus` to collect and monitor `ct-archive-serve` metrics

**Docker Compose:**

```bash
# Standalone
docker compose up

# Full stack
docker compose -f ./compose-all.yml up
```

**Podman Compose:**

```bash
# Standalone
podman compose up

# Full stack
podman compose -f ./compose-all.yml up
```

**Access Points:**

- `ct-archive-serve`: `http://localhost:8080/logs.v3.json` and `http://localhost:8080/metrics`
- Prometheus UI: `http://localhost:9090` (if Prometheus service is enabled, e.g., `compose-all.yml`)
- qBittorrent UI: `http://localhost:8081` (if qBittorrent service is enabled, e.g., `compose-all.yml`)

The `compose-all.yml` includes an optional Prometheus service that automatically scrapes metrics from `ct-archive-serve`. The Prometheus configuration is located in `prometheus/prometheus.yml` and is automatically loaded when the Prometheus container starts.

### Systemd Quadlets

Example systemd quadlets, intended for use with Podman-based containers running as an unprivileged non-root user, are available under the `systemd` folder.

## Configuration

### Environment Variables

All configuration is via environment variables. See `internal/ct-archive-serve/README.md` for complete details.

**Key Configuration Variables:**

- `CT_ARCHIVE_PATH`: Path to archive directory (default: `/var/log/ct/archive`)
- `CT_ARCHIVE_FOLDER_PATTERN`: Pattern for archive folders (default: `ct_*`)
- `CT_LOGLISTV3_JSON_REFRESH_INTERVAL`: Refresh interval for logs.v3.json (default: `10m`)
- `CT_ARCHIVE_REFRESH_INTERVAL`: Archive index refresh interval (default: `5m`)
- `CT_ZIP_CACHE_MAX_OPEN`: Maximum open zip parts (default: `256`)
- `CT_ZIP_INTEGRITY_FAIL_TTL`: TTL for failed zip integrity checks (default: `5m`)
- `CT_HTTP_*`: HTTP server timeouts and limits (see security section)

### CLI Flags

- `-h`, `--help`: Show help and exit
- `-v`, `--verbose`: Enable verbose logging (log successful HTTP 2xx responses)
- `-d`, `--debug`: Enable debug logging (slog DEBUG level)

## API Reference

### Endpoints

- **`GET /logs.v3.json`**: Returns a CT log list v3 compatible JSON document listing all discovered archived logs
- **`GET /metrics`**: Prometheus metrics endpoint (text/plain; version=0.0.4)
- **`GET /<log>/checkpoint`**: Serves the checkpoint for the specified log
- **`GET /<log>/log.v3.json`**: Serves the log's v3 JSON metadata
- **`GET /<log>/tile/<L>/<N>[.p/<W>]`**: Serves hash tiles (level L, index N, optional partial width W)
- **`GET /<log>/tile/data/<N>[.p/<W>]`**: Serves data tiles (index N, optional partial width W)
- **`GET /<log>/issuer/<fingerprint>`**: Serves issuer certificates (fingerprint must be lowercase hex)

All endpoints support both `GET` and `HEAD` methods. Other methods return `405 Method Not Allowed`.

### Response Formats

- **Content Types**: Automatically set based on asset type:
  - JSON: `application/json`
  - Checkpoint: `text/plain; charset=utf-8`
  - Tiles: `application/octet-stream`
  - Issuers: `application/pkix-cert`
- **Error Responses**:
  - `404 Not Found`: Invalid path, missing entry, or traversal attempt
  - `503 Service Unavailable`: Zip part temporarily unavailable (integrity check failed) or logs.v3.json refresh failed

## Usage Examples

### Basic Discovery

Start the container:

```bash
docker compose -f ./compose.yml up
```

Initially, `logs.v3.json` will be empty:

```bash
$ curl -s http://localhost:8080/logs.v3.json | jq
{
  "version": "3.0",
  "log_list_timestamp": "2026-01-23T11:55:57Z",
  "operators": [
    {
      "name": "ct-archive-serve",
      "email": [],
      "logs": [],
      "tiled_logs": []
    }
  ]
}
```

### Auto-Discovery

`ct-archive-serve` automatically detects CT log archives once a folder named `ct_*` is available with a valid `000.zip` file (at minimum) within it.

**Recommended for Testing**: The two smallest logs to start with are the Symantec "Vega" and "Sirius" logs.

After downloading "Sirius", it's automatically detected:

```bash
$ curl -s http://localhost:8080/logs.v3.json | jq
{
  "version": "3.0",
  "log_list_timestamp": "2026-01-23T11:53:16Z",
  "operators": [
    {
      "name": "ct-archive-serve",
      "email": [],
      "logs": [],
      "tiled_logs": [
        {
          "description": "Symantec 'Sirius' log",
          "log_id": "FZcEiNe5l6Bb61JRKt7o0ui0oxZSZBIan6v71fha2T8=",
          "key": "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEowJkhCK7JewN47zCyYl93UXQ7uYVhY/Z5xcbE4Dq7bKFN61qxdglnfr0tPNuFiglN+qjN2Syxwv9UeXBBfQOtQ==",
          "mmd": 0,
          "log_type": "",
          "state": {
            "retired": {
              "timestamp": "2026-01-23T11:53:16Z"
            }
          },
          "submission_url": "http://localhost:8080/symantec_sirius",
          "monitoring_url": "http://localhost:8080/symantec_sirius",
          "has_issuers": false
        }
      ]
    }
  ]
}
```

### Multiple Logs

As additional logs are downloaded and become available as verified zip files, they're automatically added to the log list:

```bash
$ curl -s http://localhost:8080/logs.v3.json | jq '.operators[0].tiled_logs | length'
3
```

## Development

### Workflow (Spec-Driven)

This repo uses `.specify/` + `specs/<feature>/` for spec-driven development.

**Current feature spec set**: `specs/001-ct-archive-serve/`
- `spec.md`: Requirements and clarifications
- `plan.md`: Implementation plan
- `tasks.md`: Task breakdown
- Supporting docs: `contracts/`, `checklists/`, `quickstart.md`, `research.md`, `data-model.md`

**Speckit Usage**: The `.specify/scripts/bash/check-prerequisites.sh` helper locates the active spec directory based on the **numeric branch prefix** (e.g., branch `001-...` maps to `specs/001-*`).

### Code Structure

```
cmd/ct-archive-serve/        # CLI entrypoint
internal/ct-archive-serve/   # Core implementation
```

**Go Version**: Target runtime **Go 1.25.6+**

### Local Tooling

- `make test`: Run tests
- `make lint`: Run linters (requires `golangci-lint`)
- `make security`: Run security checks (optionally uses `govulncheck` and `trivy` if installed)

### CI/CD

- **GitHub Actions**: CI runs on `.github/workflows/ci.yml`
- **Container Images**: Built and published to GHCR (`ghcr.io/colin-stubbs/ct-archive-serve`) on successful builds via `.github/workflows/image.yml`

## Security

`ct-archive-serve` implements multiple layers of security protection. While designed for container-based operation behind a reverse proxy, it includes built-in security measures that protect even when running standalone.

### Security Summary

- ✅ **Path Traversal Protection**: Strict validation prevents directory traversal attacks
- ✅ **Input Validation**: All request parameters validated before processing
- ✅ **Zip Entry Security**: Secure access controls for zip file contents
- ✅ **HTTP Security**: Safe timeouts and limits to prevent resource exhaustion
- ✅ **Trusted Source Validation**: `X-Forwarded-*` headers validated against trusted sources
- ✅ **Container Security**: Non-root user, read-only mounts, secure defaults
- ✅ **Error Handling**: No information leakage, consistent error responses

### Path Traversal Protection

Strict path validation prevents directory traversal attacks:

- **Percent-encoded attacks**: Any request path containing `%` characters is immediately rejected with `404`
- **Directory traversal**: Any request path containing `..` is rejected with `404`
- **Log name validation**: The `<log>` path segment is validated to reject empty strings, `.`, and `..`
- **EntryPath construction**: Zip entry paths are constructed from validated segments, never from raw user input

### Input Validation

All request parameters are strictly validated:

- **Tile level**: Hash tile level `<L>` must be a base-10 integer in range 0-255
- **Tile index**: Uses C2SP "groups-of-three" decimal encoding with strict format validation
- **Partial tile width**: For `.p/<W>` requests, `<W>` must be 1-255
- **Issuer fingerprint**: Must be non-empty lowercase hexadecimal (`0-9a-f`)
- **HTTP method policy**: Only `GET` and `HEAD` allowed; others return `405 Method Not Allowed`

### Zip Entry Access Security

- **Exact string matching**: Zip entries looked up using exact string matching against validated paths
- **No filesystem access**: Server never constructs filesystem paths from user input
- **Archive namespace isolation**: Requests can only access content within the configured archive directory

### HTTP Security Configuration

Safe HTTP server defaults prevent resource exhaustion:

- `CT_HTTP_READ_HEADER_TIMEOUT` (default: `5s`): Prevents slow clients from holding connections
- `CT_HTTP_IDLE_TIMEOUT` (default: `60s`): Closes idle connections
- `CT_HTTP_MAX_HEADER_BYTES` (default: `8192`): Limits request header size
- `CT_HTTP_WRITE_TIMEOUT` (default: `60s`): Prevents goroutine accumulation from slow or disconnected clients
- `CT_HTTP_READ_TIMEOUT` (default: `0`, disabled): Additional protection for request body reads

### Trusted Source Validation

For `/logs.v3.json` URL formation, `X-Forwarded-*` headers are validated:

- **Untrusted by default**: `X-Forwarded-Host` and `X-Forwarded-Proto` are **ignored by default**
- **Source IP validation**: Only used when request source IP matches `CT_HTTP_TRUSTED_SOURCES` (CSV of IPs/CIDRs)
- **Header logging**: Even when ignored, headers are logged for security auditing
- **Comma-separated handling**: First non-empty value after trimming whitespace is used

### Container Security Defaults

- **Non-root user**: Container runs as `nobody/nogroup` by default
- **Read-only mounts**: Archive directories should be mounted read-only (`:ro`)
- **Port binding**: Listens on TCP/8080; operators can publish to host port 80 via `-p 80:8080`

### Security Responsibilities: Reverse Proxy

`ct-archive-serve` is designed to run behind a reverse proxy that handles edge security controls:

- **TLS termination**: Reverse proxy should terminate TLS/HTTPS (`ct-archive-serve` serves plain HTTP only) if required
- **Rate limiting**: Should be implemented at the reverse proxy level if required
- **WAF rules**: Web Application Firewall rules, if needed, should be applied at the reverse proxy if required
- **Authentication/Authorization**: Should be handled by the reverse proxy if required
- **Request logging**: `ct-archive-serve` logs request failures to stdout and errors to stderr. For successful request logging (bear in mind it will likely be in high volume bursts), use the reverse proxy.

**Important**: When using a reverse proxy, configure `CT_HTTP_TRUSTED_SOURCES` to include the proxy's source IPs/CIDRs so `X-Forwarded-*` headers are only honored from that boundary.

### Error Handling

Security-conscious error handling:

- **No information leakage**: Error messages don't expose filesystem paths, internal structure, or sensitive information
- **Consistent 404 responses**: Invalid paths, missing entries, and traversal attempts all return `404 Not Found` without distinguishing between them
- **503 for unavailable content**: When zip parts fail integrity checks, server returns `503 Service Unavailable` (not `404`) to indicate temporary unavailability

## License

This project is licensed under the **GNU General Public License v3.0** (GPL-3.0).

See the [LICENSE](LICENSE) file in this repository for the full license text, or visit [https://www.gnu.org/licenses/gpl-3.0.html](https://www.gnu.org/licenses/gpl-3.0.html) for more information.
