# Requirements Quality Checklist: ct-archive-serve (Comprehensive Release Gate)

**Purpose**: Unit tests for requirements writing - validates quality, clarity, and completeness of all requirements before implementation begins.

**Created**: 2026-01-21  
**Depth**: Deep/Thorough  
**Audience**: Release Gate  
**Scope**: Comprehensive (All Areas)

### Audit (spec.md vs checklist)

| Metric | Value |
|--------|--------|
| **Audit date** | 2026-01-21 (run after spec/plan/tasks alignment); updated after U1/U2/U3 remediation |
| **Complete** | 135 items (spec satisfies the checklist question) |
| **Not complete** | 0 items |
| **Not complete IDs** | — |

**Remediated (U1/U2/U3):** Empty archive (CHK082): FR-006 now states `/logs.v3.json` returns 200 with empty `tiled_logs[]`. Very long log names (CHK088): FR-003a now sets `<log>` max 256 characters with truncation. Conflicting requirements (CHK122): spec now includes a Requirements consistency note.


---

## Requirement Completeness

- [x] CHK001 - Are all HTTP endpoint paths explicitly specified with their required Content-Type headers? [Completeness, Spec §FR-002]
- [x] CHK002 - Are HTTP method policies (GET, HEAD, 405 for unsupported methods) defined for all endpoints? [Completeness, Spec §FR-002a]
- [x] CHK003 - Are all required environment variables documented with their defaults and validation rules? [Completeness, Spec §FR-004, FR-007, FR-011, FR-012]
- [x] CHK004 - Are CLI flag requirements (help, verbose, debug) explicitly specified? [Completeness, Spec §FR-005]
- [x] CHK005 - Are all `/logs.v3.json` generation requirements specified, including refresh intervals, failure behavior, and concurrency protection? [Completeness, Spec §FR-006]
- [x] CHK006 - Are sub-requirements for `tiled_logs[]` entries (has_issuers, URL field handling, state field) explicitly defined? [Completeness, Spec §FR-006a, FR-006b, FR-006c]
- [x] CHK007 - Are tile path mapping requirements (zip part selection, level-2 tile splitting) mathematically specified? [Completeness, Spec §FR-008]
- [x] CHK008 - Are tile index encoding/decoding requirements (C2SP groups-of-three) explicitly defined with examples? [Completeness, Spec §FR-008a]
- [x] CHK009 - Are path traversal prevention requirements specified for all request paths? [Completeness, Spec §FR-009]
- [x] CHK010 - Are zip integrity verification requirements (structural checks, caching, TTL) completely specified? [Completeness, Spec §FR-013]
- [x] CHK011 - Are all Prometheus metrics explicitly defined with names, types, labels, and update triggers? [Completeness, Spec §NFR-009]
- [x] CHK012 - Are logging requirements (structured JSON, verbosity control, stdout/stderr split) completely specified? [Completeness, Spec §NFR-010]
- [x] CHK013 - Are code quality gates (golangci-lint, govulncheck, trivy) explicitly required? [Completeness, Spec §NFR-011]
- [x] CHK014 - Are container operation requirements (compose.yml, Prometheus config, README documentation) specified? [Completeness, Spec §NFR-014]
- [x] CHK015 - Are container security defaults (non-root user, port binding) explicitly defined? [Completeness, Spec §NFR-015]
- [x] CHK016 - Are archive discovery requirements (folder pattern, log derivation, collision handling) completely specified? [Completeness, Spec §FR-003, FR-003a, FR-003b]
- [x] CHK017 - Are all edge cases (invalid paths, invalid encodings, missing zips, temporarily unavailable zips) addressed in requirements? [Completeness, Spec §Edge Cases]
- [x] CHK018 - Are performance requirements (no per-request scanning, cached zip indices, bounded resource usage) explicitly specified? [Completeness, Spec §NFR-001, NFR-006]
- [x] CHK019 - Are reverse proxy deployment requirements (TLS termination, rate limiting, X-Forwarded-* handling) explicitly documented? [Completeness, Spec §NFR-008]
- [x] CHK020 - Are success criteria measurable and testable? [Completeness, Spec §Success Criteria]

## Requirement Clarity

- [x] CHK021 - Is "public base URL" derivation algorithm explicitly specified with step-by-step logic? [Clarity, Spec §FR-006]
- [x] CHK022 - Are X-Forwarded-* header trust conditions (CT_HTTP_TRUSTED_SOURCES) clearly defined with examples? [Clarity, Spec §FR-006, FR-012]
- [x] CHK023 - Is the `<log>` derivation algorithm (prefix stripping from folder pattern) mathematically precise? [Clarity, Spec §FR-003a]
- [x] CHK024 - Are tile zip index calculation formulas explicitly stated with examples? [Clarity, Spec §FR-008]
- [x] CHK025 - Is tile index encoding/decoding (groups-of-three) specified with concrete examples? [Clarity, Spec §FR-008a]
- [x] CHK026 - Are zip integrity check procedures (what is checked, what is not checked) explicitly defined? [Clarity, Spec §FR-013]
- [x] CHK027 - Are metric naming conventions (namespace, subsystem) clearly specified? [Clarity, Spec §NFR-009]
- [x] CHK028 - Are "clear context" requirements for error logs specified with concrete field examples? [Clarity, Spec §NFR-010]
- [x] CHK029 - Are logging verbosity levels (verbose, debug) clearly defined with what is logged at each level? [Clarity, Spec §NFR-010]
- [x] CHK030 - Are refresh interval defaults (10m for logs.v3.json, 5m for archive index) explicitly stated with rationale? [Clarity, Spec §FR-007, FR-011]
- [x] CHK031 - Are cache size limits (CT_ZIP_CACHE_MAX_OPEN default 256) explicitly specified with trade-off rationale? [Clarity, Spec §FR-011]
- [x] CHK032 - Are HTTP timeout defaults (read header, idle, max header bytes) explicitly stated? [Clarity, Spec §FR-012]
- [x] CHK033 - Is the zip integrity fail TTL (default 5m) explicitly specified with rationale? [Clarity, Spec §FR-013]
- [x] CHK034 - Are Content-Type mappings for all asset types explicitly listed? [Clarity, Spec §FR-002]
- [x] CHK035 - Is the "retired" state timestamp derivation (first discovery time) clearly specified? [Clarity, Spec §FR-006c]
- [x] CHK036 - Are mtime-based caching requirements (when to cache, when to invalidate) explicitly defined? [Clarity, Spec §FR-006]

## Requirement Consistency

- [x] CHK037 - Are Content-Type requirements consistent across all endpoint specifications? [Consistency, Spec §FR-002]
- [x] CHK038 - Do HTTP method policy requirements align across all endpoints? [Consistency, Spec §FR-002a]
- [x] CHK039 - Are error response codes (404 vs 503) consistently applied based on failure type? [Consistency, Spec §FR-009, FR-009a, FR-013]
- [x] CHK040 - Are metric naming conventions consistently applied across all metric definitions? [Consistency, Spec §NFR-009]
- [x] CHK041 - Are logging requirements consistent between HTTP request logging and error logging? [Consistency, Spec §NFR-010]
- [x] CHK042 - Do refresh interval requirements align between logs.v3.json and archive index? [Consistency, Spec §FR-007, FR-011]
- [x] CHK043 - Are path validation requirements (traversal prevention, encoding validation) consistently applied? [Consistency, Spec §FR-008a, FR-009]
- [x] CHK044 - Are reverse proxy assumptions consistent across security and URL formation requirements? [Consistency, Spec §NFR-008, FR-006]
- [x] CHK045 - Do tile encoding requirements align with C2SP spec while maintaining photocamera-archiver compatibility? [Consistency, Spec §FR-008a]

## Acceptance Criteria Quality

- [x] CHK046 - Can all success criteria (SC-001 through SC-006) be objectively measured? [Acceptance Criteria, Spec §Success Criteria]
- [x] CHK047 - Are acceptance scenarios for User Story 1 (logs.v3.json discovery) testable and measurable? [Acceptance Criteria, Spec §User Story 1]
- [x] CHK048 - Are acceptance scenarios for User Story 2 (Static-CT asset serving) testable and measurable? [Acceptance Criteria, Spec §User Story 2]
- [x] CHK049 - Can zip integrity verification requirements be validated through tests? [Acceptance Criteria, Spec §FR-013]
- [x] CHK050 - Can performance requirements (SC-006) be validated through profiling/benchmarks? [Acceptance Criteria, Spec §SC-006]
- [x] CHK051 - Can Static-CT compatibility requirements (FR-010) be validated through integration tests? [Acceptance Criteria, Spec §FR-010]
- [x] CHK052 - Can metric requirements be validated through Prometheus scraping? [Acceptance Criteria, Spec §NFR-009]
- [x] CHK053 - Can logging requirements be validated through log output inspection? [Acceptance Criteria, Spec §NFR-010]

## Scenario Coverage

### Primary Scenarios

- [x] CHK054 - Are requirements defined for successful log discovery via `/logs.v3.json`? [Coverage, Spec §User Story 1]
- [x] CHK055 - Are requirements defined for serving all Static-CT asset types (checkpoint, tiles, issuers)? [Coverage, Spec §User Story 2]
- [x] CHK056 - Are requirements defined for serving multiple archived logs simultaneously? [Coverage, Spec §FR-003]
- [x] CHK057 - Are requirements defined for periodic refresh of logs.v3.json and archive index? [Coverage, Spec §FR-006, FR-011]

### Alternate Scenarios

- [x] CHK058 - Are requirements defined for HEAD method requests (same as GET but no body)? [Coverage, Spec §FR-002a]
- [x] CHK059 - Are requirements defined for serving partial tiles (`.p/<W>` suffix)? [Coverage, Spec §FR-008a]
- [x] CHK060 - Are requirements defined for serving high-level tiles (L≥3) from shared metadata zip? [Coverage, Spec §FR-008]
- [x] CHK061 - Are requirements defined for logs with and without issuer entries? [Coverage, Spec §FR-006a]

### Exception/Error Scenarios

- [x] CHK062 - Are requirements defined for refresh failures (logs.v3.json returns 503)? [Coverage, Spec §FR-006]
- [x] CHK063 - Are requirements defined for zip integrity check failures (returns 503)? [Coverage, Spec §FR-013]
- [x] CHK064 - Are requirements defined for missing zip parts (returns 404)? [Coverage, Spec §FR-009, Edge Cases]
- [x] CHK065 - Are requirements defined for invalid request paths (returns 404)? [Coverage, Spec §FR-009, Edge Cases]
- [x] CHK066 - Are requirements defined for invalid tile encodings (returns 404)? [Coverage, Spec §FR-008a, Edge Cases]
- [x] CHK067 - Are requirements defined for invalid issuer fingerprints (returns 404)? [Coverage, Spec §Edge Cases]
- [x] CHK068 - Are requirements defined for path traversal attempts (returns 404)? [Coverage, Spec §FR-009, Edge Cases]
- [x] CHK069 - Are requirements defined for unsupported HTTP methods (returns 405)? [Coverage, Spec §FR-002a]
- [x] CHK070 - Are requirements defined for invalid configuration (startup failure)? [Coverage, Spec §FR-003a, FR-003b, FR-012]
- [x] CHK071 - Are requirements defined for log collision scenarios (startup failure)? [Coverage, Spec §FR-003b]
- [x] CHK072 - Are requirements defined for untrusted X-Forwarded-* headers (ignored for URL formation)? [Coverage, Spec §FR-006, FR-012]

### Recovery Scenarios

- [x] CHK073 - Are requirements defined for recovery after zip integrity check failure (retry after TTL)? [Recovery, Spec §FR-013]
- [x] CHK074 - Are requirements defined for recovery after logs.v3.json refresh failure (resume 200 after successful refresh)? [Recovery, Spec §FR-006]
- [x] CHK075 - Are requirements defined for handling zip parts that become available after initial failure? [Recovery, Spec §FR-013]
- [x] CHK076 - Are requirements defined for archive index refresh when archives are added/removed? [Recovery, Spec §FR-011]

### Non-Functional Scenarios

- [x] CHK077 - Are requirements defined for high-concurrency request handling? [Non-Functional, Spec §NFR-005]
- [x] CHK078 - Are requirements defined for large working set scenarios (many zip parts)? [Non-Functional, Spec §NFR-001, SC-006]
- [x] CHK079 - Are requirements defined for resource exhaustion prevention (bounded caches)? [Non-Functional, Spec §NFR-006]
- [x] CHK080 - Are requirements defined for slow/abusive client handling (timeouts)? [Non-Functional, Spec §FR-012]

## Edge Case Coverage

- [x] CHK081 - Are requirements defined for right-edge partial tiles (literal path, no synthesis)? [Edge Case, Spec §Edge Cases]
- [x] CHK082 - Are requirements defined for empty archive directories? [Edge Case, Spec §FR-006 Empty archive]
- [x] CHK083 - Are requirements defined for archive directories with no valid 000.zip files? [Edge Case, Spec §FR-006]
- [x] CHK084 - Are requirements defined for archive directories with malformed zip files? [Edge Case, Spec §FR-013]
- [x] CHK085 - Are requirements defined for very large tile indices (multi-segment encoding)? [Edge Case, Spec §FR-008a]
- [x] CHK086 - Are requirements defined for tile level 255 (maximum valid level)? [Edge Case, Spec §FR-008a, Edge Cases]
- [x] CHK087 - Are requirements defined for partial tile width 255 (maximum valid width)? [Edge Case, Spec §FR-008a, Edge Cases]
- [x] CHK088 - Are requirements defined for logs with very long names (after prefix stripping)? [Edge Case, Spec §FR-003a max 256 chars, truncation]
- [x] CHK089 - Are requirements defined for archive folders with non-standard zip part naming? [Edge Case, Spec §FR-003]
- [x] CHK090 - Are requirements defined for concurrent refresh attempts (mutex protection)? [Edge Case, Spec §FR-006]
- [x] CHK091 - Are requirements defined for mtime cache invalidation when archives are removed? [Edge Case, Spec §FR-006]
- [x] CHK092 - Are requirements defined for zip integrity cache eviction on read failures? [Edge Case, Spec §FR-013]

## Non-Functional Requirements

### Performance

- [x] CHK093 - Are performance requirements quantified with specific constraints (no per-request scanning, cached indices)? [Non-Functional, Spec §NFR-001]
- [x] CHK094 - Are performance optimization requirements (mtime caching, single ZIP open) explicitly specified? [Non-Functional, Spec §FR-006]
- [x] CHK095 - Are resource bounds (cache size limits, eviction policies) explicitly defined? [Non-Functional, Spec §NFR-006]
- [x] CHK096 - Are streaming response requirements (no full-entry buffering) explicitly specified? [Non-Functional, Spec §NFR-007]
- [x] CHK097 - Are random-access zip reading requirements (seekable, no full decompression) explicitly specified? [Non-Functional, Spec §NFR-003, NFR-004]

### Security

- [x] CHK098 - Are path traversal prevention requirements explicitly specified for all request paths? [Security, Spec §FR-009, NFR-002]
- [x] CHK099 - Are input validation requirements (tile encoding, issuer fingerprint) explicitly specified? [Security, Spec §FR-008a, Edge Cases]
- [x] CHK100 - Are X-Forwarded-* header trust requirements (CT_HTTP_TRUSTED_SOURCES) explicitly specified? [Security, Spec §FR-006, FR-012]
- [x] CHK101 - Are reverse proxy security boundary requirements (TLS, rate limiting delegation) explicitly documented? [Security, Spec §NFR-008]
- [x] CHK102 - Are secret leakage prevention requirements (no cryptographic material in logs) explicitly specified? [Security, Spec §NFR-010]
- [x] CHK103 - Are filesystem boundary requirements (archive directory only, no escape) explicitly specified? [Security, Spec §NFR-002]

### Observability

- [x] CHK104 - Are all Prometheus metrics explicitly defined with names, types, labels, and update timing? [Observability, Spec §NFR-009]
- [x] CHK105 - Are low-cardinality metric requirements (no per-path/endpoint/status labels) explicitly specified? [Observability, Spec §NFR-009]
- [x] CHK106 - Are per-log metrics (valid and nonexistent logs) explicitly specified with all required counters? [Observability, Spec §NFR-009]
- [x] CHK107 - Are resource observability metrics (cache state, integrity checks, discovered counts) explicitly specified? [Observability, Spec §NFR-009]
- [x] CHK108 - Are structured logging requirements (JSON format, field names) explicitly specified? [Observability, Spec §NFR-010]
- [x] CHK109 - Are logging verbosity requirements (verbose mode, debug mode) explicitly specified? [Observability, Spec §NFR-010]
- [x] CHK110 - Are startup debug logging requirements explicitly specified? [Observability, Spec §NFR-010]

### Accessibility/Compatibility

- [x] CHK111 - Are Static-CT (C2SP) compatibility requirements explicitly specified with acceptance criteria? [Compatibility, Spec §FR-010]
- [x] CHK112 - Are log list v3 JSON schema validation requirements explicitly specified? [Compatibility, Spec §FR-006]
- [x] CHK113 - Are photocamera-archiver compatibility requirements (x-prefix on all segments) explicitly specified? [Compatibility, Spec §FR-008a]

## Dependencies & Assumptions

- [x] CHK114 - Is the assumption of reverse proxy for TLS/rate limiting explicitly documented? [Assumption, Spec §NFR-008]
- [x] CHK115 - Is the assumption of archive format (photocamera-archiver output) explicitly documented? [Assumption, Spec §Overview]
- [x] CHK116 - Are external library dependencies (loglist3, certificate-transparency-go, sunlight) explicitly documented? [Dependency, Spec §NFR-012]
- [x] CHK117 - Is the dependency on Go standard library archive/zip explicitly documented? [Dependency, Spec §NFR-004]
- [x] CHK118 - Are CI/CD dependencies (GitHub Actions, GHCR) explicitly documented? [Dependency, Spec §NFR-013]
- [x] CHK119 - Is the assumption of container runtime (Docker/Podman) explicitly documented? [Assumption, Spec §NFR-014]

## Ambiguities & Conflicts

- [x] CHK120 - Are all vague terms (e.g., "reasonable scrape interval") quantified or clarified? [Ambiguity, Spec §NFR-014]
- [x] CHK121 - Are all "SHOULD" requirements clearly distinguished from "MUST" requirements? [Clarity, Spec §Multiple]
- [x] CHK122 - Are conflicting requirements (if any) explicitly resolved or documented? [Conflict, Spec §Requirements consistency]
- [x] CHK123 - Are placeholder values in examples clearly marked as illustrative? [Clarity, Spec §FR-006]

## Traceability

- [x] CHK124 - Are all functional requirements (FR-001 through FR-013) traceable to user stories or acceptance criteria? [Traceability, Spec §Requirements]
- [x] CHK125 - Are all non-functional requirements (NFR-001 through NFR-015) traceable to success criteria or operational needs? [Traceability, Spec §Requirements]
- [x] CHK126 - Are all edge cases traceable to specific requirement sections? [Traceability, Spec §Edge Cases]
- [x] CHK127 - Are all sub-requirements (FR-006a, FR-006b, FR-006c) clearly grouped under parent requirements? [Traceability, Spec §FR-006]

## Container & Deployment

- [x] CHK128 - Are container runtime requirements (non-root user, port binding) explicitly specified? [Deployment, Spec §NFR-015]
- [x] CHK129 - Are compose.yml requirements (Prometheus integration, service networking) explicitly specified? [Deployment, Spec §NFR-014]
- [x] CHK130 - Are README documentation requirements (container operation examples) explicitly specified? [Deployment, Spec §NFR-014]
- [x] CHK131 - Are build/release workflow requirements (GitHub Actions, GHCR publishing) explicitly specified? [Deployment, Spec §NFR-013]

## Configuration & Environment

- [x] CHK132 - Are all environment variable defaults explicitly stated? [Configuration, Spec §FR-004, FR-007, FR-011, FR-012]
- [x] CHK133 - Are environment variable validation rules (invalid values fail startup) explicitly specified? [Configuration, Spec §FR-003a, FR-012, FR-013]
- [x] CHK134 - Are CLI flag requirements (help, verbose, debug) explicitly specified? [Configuration, Spec §FR-005]
- [x] CHK135 - Are configuration error handling requirements (startup failure with clear message) explicitly specified? [Configuration, Spec §FR-003a, FR-003b]

---

**Total Items**: 135  
**Focus Areas**: API/HTTP, Performance, Security, Observability, Data Integrity, Configuration, Container/Deployment, Edge Cases, Exception Flows, Recovery Scenarios  
**Depth**: Deep/Thorough  
**Purpose**: Release Gate Validation
