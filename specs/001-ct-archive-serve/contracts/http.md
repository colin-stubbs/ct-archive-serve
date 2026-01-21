# Contracts: ct-archive-serve HTTP surface

## Routing

All Static-CT assets are served under a log prefix:

- `/<log>/...`

Where `<log>` is derived from the discovered archive folder name by stripping the literal prefix implied by `CT_ARCHIVE_FOLDER_PATTERN` (e.g., for `ct_*`, strip `ct_`).

## Endpoints

For all endpoints:

- Missing content returns `404`.
- Invalid paths (including traversal attempts) return `404`.

### Prometheus metrics

- `GET /metrics`
  - Prometheus exposition format
  - `Content-Type: text/plain; version=0.0.4; charset=utf-8`

### Monitor list

- `GET /monitor.json`
  - `Content-Type: application/json`
  - `submission_url` and `monitoring_url` values are formed using the incoming request `Host`/`X-Forwarded-Host` and `X-Forwarded-Proto` (no configured public base URL).

### Checkpoint

- `GET /<log>/checkpoint`
  - `Content-Type: text/plain; charset=utf-8`

### Log info

- `GET /<log>/log.v3.json`
  - `Content-Type: application/json`

### Issuers

- `GET /<log>/issuer/<fingerprint>`
  - `Content-Type: application/pkix-cert`

### Hash tiles

- `GET /<log>/tile/<L>/<N>`
- `GET /<log>/tile/<L>/<N>.p/<W>`
  - `Content-Type: application/octet-stream`

### Data tiles

- `GET /<log>/tile/data/<N>`
- `GET /<log>/tile/data/<N>.p/<W>`
  - `Content-Type: application/octet-stream`

