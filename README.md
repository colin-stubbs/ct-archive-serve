# ct-archive-serve

`ct-archive-serve` is an HTTP server that provides seamless access to Certificate Transparency (CT) log archives stored in the **Static-CT (tiled)** format. It serves data directly from one or more zip archive parts produced by the **`photocamera-archiver`** now available via the [ct-archive](https://github.com/geomys/ct-archive) repository, completely eliminating the need for extraction of the content from any zip file.

It is specifically designed for the CT log archives distributed via torrents available under the [ct-archive](https://github.com/geomys/ct-archive) repository as [torrents.rss](https://github.com/geomys/ct-archive/blob/main/torrents.rss).

By serving files directly from original zip archives, it offers several major advantages:
- **Storage Efficiency:** Avoid copying and unzipping massive datasets (currently 10TB+), more than halving your storage requirements. Just download the torrent data and serve it directly from where it landed.
- **Compatibility:** Provides a standard HTTP interface for existing CT log monitoring clients without code changes.
- **Frictionless Seeding:** Incentivise contributors to the CT archive torrent to remain on the network as a seed by reducing the friction created by the need to maintain a separate, extracted copy of the CT log archive data.
```

## What it does

- Serves **Static-CT assets** for each archived log under `/<log>/...` (checkpoint, tiles, issuers, etc.)
- Generates and serves a **discovery log list** at `GET /logs.v3.json` (compatible with common log list v3 consumers)
- Exposes Prometheus metrics at `GET /metrics` (low-cardinality by design)
- Provides qBittorrent configuration to automate download of torrents

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

You can operate `ct-archive-serve` entirely via containers. The server expects an archive directory, which is expected to be the same location that your torrent client stores downloads in, on disk containing `ct_*` folders with `000.zip`, `001.zip`, etc.

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

## Real Example

Start the container,

```
user@box ct-archive-serve % docker compose -f ./compose.yml up  
[+] Running 2/2
 ✔ Network ct-archive-serve_default               Created                                                                                             0.0s 
 ✔ Container ct-archive-serve-ct-archive-serve-1  Created                                                                                             0.1s 
Attaching to ct-archive-serve-1
ct-archive-serve-1  | {"time":"2026-01-23T11:55:57.478677053Z","level":"INFO","msg":"Starting ct-archive-serve","addr":":8080"}
```

Check logs.v3.json, initially you won't have anything,

```
user@box ct-archive-serve % curl -s http://localhost:8082/logs.v3.json | jq
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
user@box ct-archive-serve % 
```

If you download one of the available CT log archives `ct-archive-serve` will auto-detect it once a folder named ct_%{SOMETHING}% is available with a valid 000.zip file (at minimum) within it.

If you're testing things out the two smallest logs to first download and work with are the Symantec "Vega" and "Sirius" logs.

As below, once I downloaded "Sirius", `ct-archive-serve` detected it and added it to the logs.v3.json content.

```
user@box ct-archive-serve % curl -s http://localhost:8082/logs.v3.json | jq
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
          "state": null,
          "submission_url": "http://localhost:8082/symantec_sirius",
          "monitoring_url": "http://localhost:8082/symantec_sirius",
          "has_issuers": false
        }
      ]
    }
  ]
}
user@box ct-archive-serve % 
```

As additional logs are downloaded and become available as verified .zip files, they'll be served:

```
user@box archive % curl -s http://localhost:8082/logs.v3.json | jq       
{
  "version": "3.0",
  "log_list_timestamp": "2026-01-23T12:15:57Z",
  "operators": [
    {
      "name": "ct-archive-serve",
      "email": [],
      "logs": [],
      "tiled_logs": [
        {
          "description": "Sectigo 'Mammoth2024h1'",
          "log_id": "KdA6G7Z0qnEc0wNbZVfBT4qni0/oOJRJ7KRT+US9JGg=",
          "key": "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEpFmQ83EkJPfDVSdWnKNZHve3n86rThlmTdCK+p1ipCTwOyDkHRRnyPzkN/JLOFRaz59rB5DQDn49TIey6D8HzA==",
          "mmd": 0,
          "log_type": "",
          "state": null,
          "submission_url": "http://localhost:8082/sectigo_mammoth2024h1",
          "monitoring_url": "http://localhost:8082/sectigo_mammoth2024h1",
          "has_issuers": true
        },
        {
          "description": "Symantec 'Sirius' log",
          "log_id": "FZcEiNe5l6Bb61JRKt7o0ui0oxZSZBIan6v71fha2T8=",
          "key": "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEowJkhCK7JewN47zCyYl93UXQ7uYVhY/Z5xcbE4Dq7bKFN61qxdglnfr0tPNuFiglN+qjN2Syxwv9UeXBBfQOtQ==",
          "mmd": 0,
          "log_type": "",
          "state": null,
          "submission_url": "http://localhost:8082/symantec_sirius",
          "monitoring_url": "http://localhost:8082/symantec_sirius",
          "has_issuers": false
        },
        {
          "description": "Symantec 'Vega' log",
          "log_id": "vHjh38X2PGhGSTNNoQ+hXwl5aSAJwIG08/aRfz7ZuKU=",
          "key": "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE6pWeAv/u8TNtS4e8zf0ZF2L/lNPQWQc/Ai0ckP7IRzA78d0NuBEMXR2G3avTK0Zm+25ltzv9WWis36b4ztIYTQ==",
          "mmd": 0,
          "log_type": "",
          "state": null,
          "submission_url": "http://localhost:8082/symantec_vega",
          "monitoring_url": "http://localhost:8082/symantec_vega",
          "has_issuers": false
        }
      ]
    }
  ]
}
user@box archive % 
```

# Development

## Workflow (spec-driven)

This repo uses `.specify/` + `specs/<feature>/` for spec-driven development.

- Current feature spec set: `specs/001-ct-archive-serve/`
  - `spec.md`: requirements and clarifications
  - `plan.md`: implementation plan
  - `tasks.md`: task breakdown
  - Supporting docs: `contracts/`, `checklists/`, `quickstart.md`, `research.md`, `data-model.md`

### Speckit Usage

The `.specify/scripts/bash/check-prerequisites.sh` helper locates the active spec directory based on the **numeric branch prefix** (e.g. branch `001-...` maps to `specs/001-*`).

## Code

Implementation code lives under:

```text
cmd/ct-archive-serve/        # CLI entrypoint
internal/ct-archive-serve/   # Core implementation
```

Go version:

- Target runtime: **Go 1.25.5+**

## Security

`ct-archive-serve` is designed with security in mind, implementing multiple layers of protection against common attack vectors. While it is intended for container-based operation behind a reverse proxy, the code includes built-in security measures that protect even when running outside a container.

### Path Traversal Protection

`ct-archive-serve` implements strict path validation to prevent directory traversal attacks:

- **Percent-encoded attacks**: Any request path containing `%` characters is immediately rejected with `404`. This prevents percent-encoded traversal attempts (e.g., `%2e%2e` for `..`).
- **Directory traversal**: Any request path containing `..` is rejected with `404`, preventing attempts to escape the archive namespace (e.g., `/valid_log/../../etc/passwd`).
- **Log name validation**: The `<log>` path segment is validated to reject empty strings, `.`, and `..` as log names.
- **EntryPath construction**: Zip entry paths (`EntryPath`) are constructed from validated segments, never from raw user input. This ensures that even if a malicious zip file contains entries with traversal sequences, they cannot be accessed via HTTP requests.

### Input Validation

All request parameters are strictly validated before processing:

- **Tile level validation**: Hash tile level `<L>` must be a base-10 integer in the range 0-255. Invalid values return `404`.
- **Tile index validation**: Tile indices use C2SP "groups-of-three" decimal encoding with strict format validation. Invalid segments (wrong length, non-decimal characters, missing `x` prefix where required) return `404`.
- **Partial tile width**: For `.p/<W>` requests, `<W>` must be a base-10 integer in the range 1-255. Invalid values return `404`.
- **Issuer fingerprint validation**: Issuer fingerprints must be non-empty lowercase hexadecimal strings (`0-9a-f`). Uppercase hex or non-hex characters return `404`.
- **HTTP method policy**: Only `GET` and `HEAD` are allowed for supported routes. Other methods return `405 Method Not Allowed` with `Allow: GET, HEAD` header.

### Zip Entry Access Security

Zip entry access is secured through multiple mechanisms:

- **Exact string matching**: Zip entries are looked up using exact string matching against the validated `EntryPath`. This prevents accessing entries with malicious names even if they exist in zip files.
- **No filesystem access**: The server never constructs filesystem paths from user input. All zip file paths are derived from the validated archive index, which is built from discovered folders matching the configured pattern.
- **Archive namespace isolation**: Requests can only access content within the configured archive directory. There is no mechanism to access files outside this directory, even if path traversal sequences are attempted.

### HTTP Security Configuration

`ct-archive-serve` configures safe HTTP server defaults to prevent resource exhaustion:

- **Read header timeout**: Configurable via `CT_HTTP_READ_HEADER_TIMEOUT` (default: `5s`) to prevent slow clients from holding connections open.
- **Idle timeout**: Configurable via `CT_HTTP_IDLE_TIMEOUT` (default: `60s`) to close idle connections.
- **Max header bytes**: Configurable via `CT_HTTP_MAX_HEADER_BYTES` (default: `8192`) to limit request header size.
- **Read/Write timeouts**: Configurable via `CT_HTTP_READ_TIMEOUT` and `CT_HTTP_WRITE_TIMEOUT` (default: `0`, disabled) for additional protection against slow clients.

### Trusted Source Validation

For `/logs.v3.json` URL formation, `ct-archive-serve` validates `X-Forwarded-*` headers:

- **Untrusted by default**: `X-Forwarded-Host` and `X-Forwarded-Proto` are **ignored by default** and only used when the request source IP matches a trusted source configured via `CT_HTTP_TRUSTED_SOURCES`.
- **Source IP validation**: The request source IP is extracted from `RemoteAddr` and validated against configured IP addresses or CIDR ranges.
- **Header logging**: Even when ignored, `X-Forwarded-*` headers are logged for security auditing purposes.
- **Comma-separated handling**: When multiple values are present (e.g., from multiple proxies), the first non-empty value after trimming whitespace is used.

### Container Security Defaults

When running in a container, `ct-archive-serve` uses secure defaults:

- **Non-root user**: The container runs as `nobody/nogroup` (non-root) by default.
- **Read-only archive mount**: Archive directories should be mounted read-only (`:ro`) to prevent modification.
- **Port binding**: Listens on TCP/8080 by default. Operators can publish to host port 80 via `-p 80:8080` if needed.

### Security Responsibilities: Reverse Proxy

`ct-archive-serve` is designed to run behind a reverse proxy that handles edge security controls:

- **TLS termination**: The reverse proxy should terminate TLS/HTTPS. `ct-archive-serve` serves plain HTTP only.
- **Rate limiting**: Rate limiting should be implemented at the reverse proxy level to prevent abuse.
- **WAF rules**: Web Application Firewall (WAF) rules, if needed, should be applied at the reverse proxy.
- **Authentication/Authorization**: Any authentication or authorization requirements should be handled by the reverse proxy.
- **Request Logging*: `ct-archive-serve` logs request failures to stdout. Use standard container output logging mechanisms, or stdout based redirection if running without a container, in order to log request failures. If you require logging of successful requests be aware that this will likely be very high volume and should be performed by the reverse proxy.

**Important**: When using a reverse proxy, configure `CT_HTTP_TRUSTED_SOURCES` to include the proxy's source IPs/CIDRs so `X-Forwarded-*` headers are only honored from that boundary.

### Error Handling

Security-conscious error handling:

- **No information leakage**: Error messages do not expose filesystem paths, internal structure, or sensitive information.
- **Consistent 404 responses**: Invalid paths, missing entries, and traversal attempts all return `404 Not Found` without distinguishing between them, preventing information disclosure.
- **503 for unavailable content**: When zip parts fail integrity checks (e.g., incomplete downloads), the server returns `503 Service Unavailable` rather than `404`, indicating temporary unavailability rather than permanent absence.

## License

TBD
