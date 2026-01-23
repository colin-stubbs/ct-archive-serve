# Contracts: ct-archive-serve HTTP surface

## Routing

All Static-CT assets are served under a log prefix:

- `/<log>/...`

Where `<log>` is derived from the discovered archive folder name by stripping the literal prefix implied by `CT_ARCHIVE_FOLDER_PATTERN` (e.g., for `ct_*`, strip `ct_`).

## Endpoints

For all endpoints:

- Missing content returns `404`.
- Invalid paths (including traversal attempts) return `404`.
- If a required zip part exists but fails basic zip integrity checks (e.g., still downloading), return `503` (temporarily unavailable) per `spec.md` `FR-013`.
- Allowed methods are `GET` and `HEAD`. Other methods to a supported route return `405` with `Allow: GET, HEAD` (see `spec.md` `FR-002a`).

### Prometheus metrics

- `GET /metrics`
  - Prometheus exposition format
  - `Content-Type: text/plain; version=0.0.4; charset=utf-8`

### Log list v3

- `GET /logs.v3.json`
  - `Content-Type: application/json`
  - `submission_url` and `monitoring_url` values are formed using a “public base URL” derived from the incoming request (no configured public base URL), per `spec.md` `FR-006` / `FR-012`:
    - `X-Forwarded-Host` and `X-Forwarded-Proto` are **untrusted by default** and MUST be ignored unless the request source IP is trusted by `CT_HTTP_TRUSTED_SOURCES`.
    - When trusted, if a chosen `X-Forwarded-*` value contains a comma-separated list, the server uses the first non-empty element after trimming ASCII whitespace; the resulting scheme should be lowercased.
  - If the most recent `/logs.v3.json` refresh attempt failed (e.g., due to unreadable `000.zip` or invalid `log.v3.json`), return `503` (temporarily unavailable) per `spec.md` `FR-006`.

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
  - `<N>` uses the standard tlog "groups-of-three" decimal path encoding (see `spec.md` `FR-008a`); it may span multiple path segments (e.g. `001/234/567`).

### Data tiles

- `GET /<log>/tile/data/<N>`
- `GET /<log>/tile/data/<N>.p/<W>`
  - `Content-Type: application/octet-stream`
  - `<N>` uses the standard tlog "groups-of-three" decimal path encoding (see `spec.md` `FR-008a`); it may span multiple path segments (e.g. `001/234/567`).

