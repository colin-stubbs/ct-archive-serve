# Research: ct-archive-serve

## Source archive format

`ct-archive-serve` targets zip archives produced by `photocamera-archiver` (Sunlight), which creates a set of zip files containing Static-CT monitoring assets:

- `checkpoint`
- `log.v3.json`
- `issuer/<sha256hex>`
- `tile/<L>/<N>[.p/<W>]` (where `<N>` uses tlog "groups-of-three" decimal path encoding and may span multiple path segments)
- `tile/data/<N>[.p/<W>]` (where `<N>` uses tlog "groups-of-three" decimal path encoding and may span multiple path segments)

The archiver splits a log into subtree zip files. Each zip contains one level-2 tile (and everything below it), plus shared metadata (checkpoint/log info/issuers/high-level tiles).

## Seekable zip access requirement

`photocamera-archiver` intentionally creates seekable zip files so consumers can retrieve individual entries without reading or decompressing the entire zip.

`ct-archive-serve` must therefore:

- open zip parts from a seekable backing store (random access),
- use zip metadata to locate the entry,
- read/decompress only the bytes for the requested entry (plus required metadata).

In Go, the standard-library `archive/zip` package is designed for this pattern:

- `zip.OpenReader(path)` opens the file and uses `io.ReaderAt` internally.
- The reader uses the zip central directory at the end of the file to map entry names → offsets.
- `(*zip.File).Open()` seeks to the entry data offset and streams decompression for that entry only.

## Zip integrity verification during torrent downloads

Because archive zip parts may be **present but incomplete** while a torrent client is still downloading, `ct-archive-serve` treats zip integrity as a structural validity problem (not content verification):

- Open the zip with `zip.OpenReader(path)` to parse the end-of-central-directory record and the central directory.
- Then iterate `r.File` and call `Open()` + immediately `Close()` each file to force validation of each local file header and offset metadata **without reading the entry payload**.

This approach is fast relative to full extraction/decompression, and it detects truncated/cut-off zips reliably. Results are cached per `spec.md` `FR-013` (passed set kept for process lifetime; failed set expires after `CT_ZIP_INTEGRITY_FAIL_TTL` so the server can retry once a download completes).

## Zip selection for tile requests

Static-CT tiles use tile height 8, width 256. For hash tiles:

- A tile at level \(L\) and index \(N\) covers leaf range:
  - start = \(N \cdot 256^{L+1}\)
  - end   = \((N+1) \cdot 256^{L+1}\)

`photocamera-archiver` splits the log by level-2 tiles, each spanning \(256^3\) leaves.

Therefore, for requests where \(L \le 2\) (and for data tiles), the subtree zip index can be derived from the leaf start:

- `zipIndex = floor(leafStart / 256^3)`
- equivalently:
  - for data tiles (`tile/data/<N>`): `zipIndex = N / 65536`
  - for level 0 tiles: `zipIndex = N / 65536`
  - for level 1 tiles: `zipIndex = N / 256`
  - for level 2 tiles: `zipIndex = N`

For \(L \ge 3\), `photocamera-archiver` includes those tiles in every zip, so `ct-archive-serve` can serve them from any zip (prefer `000.zip` when present).

## Real-world directory layout

An observed archive layout is:

- `CT_ARCHIVE_PATH/<logFolder>/000.zip`
- `CT_ARCHIVE_PATH/<logFolder>/001.zip`
- …

There may also be non-zip files (e.g. `*_meta.sqlite`, `*_meta.xml`, padding files). These should be ignored by archive discovery.

## monitor.json generation

`ct-archive-serve` also exposes `GET /monitor.json` as a generated log list for the archives currently present on disk.

The intended output shape matches a CT log list v3 schema used by Static-CT tooling, at minimum:

- `operators[0].logs` = `[]`
- `operators[0].tiled_logs` populated from each discovered archive folder’s `000.zip` → `log.v3.json`, with:
  - `submission_url` and `monitoring_url` set to `<publicBaseURL>/<log>` where `<publicBaseURL>` is derived from the incoming `/monitor.json` request headers (`Host`, and `X-Forwarded-Host`/`X-Forwarded-Proto` when present)
  - `<log>` derived by stripping the `CT_ARCHIVE_FOLDER_PATTERN` prefix (e.g. `ct_*` → strip `ct_`)

Operator identity fields (`operators[].name`, `operators[].email`) are informational for this use-case; consumer validation focuses on log entry URL fields and key/log_id consistency.

A reference example exists in `testing/ct-monitor-archive.json`.

### has_issuers rule

For parity with the proof-of-concept script, `has_issuers` is derived from the `000.zip` contents:

- `has_issuers = true` if and only if `000.zip` contains at least one entry with name prefix `issuer/`
- otherwise `has_issuers = false`

## Performance under extreme load (large working set)

The performance goal for `ct-archive-serve` is to remain “hardware-limited” under extreme load (primarily constrained by disk throughput, zip decompression CPU, and network writes), including **large working-set** access patterns where requests are spread across many zip parts and many distinct tiles.

Implications for design:

- **No request-path rescans**: archive discovery and zip part enumeration must happen outside the request hot path (startup and periodic refresh), so serving a request does not perform directory walking/globbing.
- **Bounded zip state reuse**: repeatedly opening zip parts and re-parsing central directories can become an avoidable bottleneck; cache (bounded/LRU) the open zip parts and their entry indices to reduce per-request overhead.
- **Bounded resources**: any caching must be capped (e.g., maximum open zip parts) and must evict cleanly to support large working sets without unbounded growth.

