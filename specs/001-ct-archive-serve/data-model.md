# Data Model: ct-archive-serve

## Entities

- **ArchiveRoot**
  - Represents the configured top-level directory (`CT_ARCHIVE_PATH`).

- **LogArchive**
  - Represents one archived CT log under `ArchiveRoot`, discovered by `CT_ARCHIVE_FOLDER_PATTERN`.
  - Key attributes: `Name` (folder name), `Path` (absolute path), `ZipFiles` (indexed by zip number).

- **ZipPart**
  - Represents one subtree zip file (e.g. `000.zip`, `001.zip`) within a `LogArchive`.
  - Key attributes: `Index` (0-based), `Path`, `Entries` (lookup by zip entry name).

- **Request**
  - HTTP request mapped to a `LogArchive` (first URL path segment) and a `ZipEntryPath` (remaining segments).
  - Special case: `GET /logs.v3.json` is global (no `<log>` prefix).

- **LogListV3**
  - Represents the generated `GET /logs.v3.json` payload.
  - Key attributes: `GeneratedAt` (time), `Payload` (JSON bytes), `Entries` (one per discovered `LogArchive` derived from `000.zip` → `log.v3.json`).

## Relationships

- `ArchiveRoot` contains many `LogArchive`.
- Each `LogArchive` contains many `ZipPart`.
- A `Request` resolves to exactly one `LogArchive`, and then to either:
  - one `ZipPart` (for tiles/data tiles), or
  - a preferred `ZipPart` (for shared metadata like `checkpoint`, `log.v3.json`, `issuer/*`, and `tile/3+/*`).

