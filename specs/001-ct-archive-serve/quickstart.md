# Quickstart: ct-archive-serve

## Environment variables

- `CT_ARCHIVE_PATH` (default: `/var/log/ct/archive`)
  - Top-level directory containing per-log archive folders.
- `CT_ARCHIVE_FOLDER_PATTERN` (default: `ct_*`)
  - Glob pattern (matched within `CT_ARCHIVE_PATH`) selecting folders to treat as individual log archives.
- `CT_MONITOR_JSON_REFRESH_INTERVAL` (default: `5m`)
  - How often to regenerate `GET /monitor.json` from each discovered log’s `000.zip` → `log.v3.json`.
- `CT_ZIP_CACHE_MAX_OPEN` (default: `256`)
  - Maximum number of simultaneously-open zip parts across all logs (bounded cache for extreme-load performance).
- `CT_ARCHIVE_REFRESH_INTERVAL` (default: `1m`)
  - How frequently to refresh the on-disk archive folder/zip-part index (must not run on the request hot path).
- `CT_HTTP_READ_HEADER_TIMEOUT` (default: `5s`)
  - Maximum time to read request headers (slowloris protection).
- `CT_HTTP_IDLE_TIMEOUT` (default: `60s`)
  - Keep-alive idle timeout.
- `CT_HTTP_MAX_HEADER_BYTES` (default: `8192`)
  - Maximum size of request headers.
- `CT_HTTP_WRITE_TIMEOUT` (default: `0`)
  - Response write timeout (`0` = no explicit write timeout).
- `CT_HTTP_READ_TIMEOUT` (default: `0`)
  - Full-request read timeout (`0` = no explicit read timeout).
- `CT_HTTP_TRUSTED_SOURCES` (default: empty)
  - CSV list of trusted request source IPs and/or CIDR ranges (e.g., `127.0.0.1,10.0.0.0/8`).
  - When empty/unset, `X-Forwarded-Host` and `X-Forwarded-Proto` are logged but ignored for `/monitor.json` URL formation.

## Folder layout example

```text
/var/log/ct/archive/
├── ct_venafi_ctlog_gen2/
│   ├── 000.zip
│   ├── 001.zip
│   └── ...
└── ct_trustasia_log2024/
    ├── 000.zip
    ├── 001.zip
    └── ...
```

## Run

`ct-archive-serve` listens on TCP/8080 by default.

All runtime behavior is configured via environment variables; CLI flags are limited to help output and logging verbosity/debug (`-h/-v/-d`).

`GET /monitor.json` URL formation derives `<publicBaseURL>` from the incoming request headers. `X-Forwarded-Host`/`X-Forwarded-Proto` are only honored when the request source IP matches `CT_HTTP_TRUSTED_SOURCES`; otherwise they are logged but ignored. In production, run `ct-archive-serve` behind a reverse proxy that performs TLS termination and rate limiting and forwards the appropriate `X-Forwarded-*` headers from a trusted source IP/CIDR.

```bash
CT_ARCHIVE_PATH=/var/log/ct/archive \
CT_ARCHIVE_FOLDER_PATTERN='ct_*' \
CT_MONITOR_JSON_REFRESH_INTERVAL='5m' \
CT_ZIP_CACHE_MAX_OPEN='256' \
CT_ARCHIVE_REFRESH_INTERVAL='1m' \
CT_HTTP_READ_HEADER_TIMEOUT='5s' \
CT_HTTP_IDLE_TIMEOUT='60s' \
CT_HTTP_MAX_HEADER_BYTES='8192' \
ct-archive-serve -v
```

## Example requests

- `GET /monitor.json`
- `GET /metrics`
- `GET /trustasia_log2024/checkpoint`
- `GET /trustasia_log2024/tile/0/000`
- `GET /trustasia_log2024/tile/data/000`
- `GET /trustasia_log2024/issuer/<sha256hex>`

