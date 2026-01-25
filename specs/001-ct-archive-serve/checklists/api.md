# API Requirements Quality Checklist: ct-archive-serve

**Purpose**: Unit-test the written requirements for `ct-archive-serve`’s HTTP API, routing, and archive-backed content serving (clarity, completeness, consistency, and measurability).
**Created**: 2026-01-20
**Feature**: `specs/001-ct-archive-serve/spec.md` (with supporting artifacts `plan.md`, `tasks.md`)

### Clarifying questions (resolved by defaults for this checklist)

- **Q1**: Is this checklist intended as a lightweight author self-check or a PR/reviewer gate? → **Default used**: Reviewer (PR gate).
- **Q2**: Should the checklist focus only on HTTP/API semantics or also include archive/zip performance + safety constraints? → **Default used**: Include both (API + zip/seekability + safety).
- **Q3**: Should compatibility with existing Static-CT client behavior be treated as a MUST-level quality gate or informational SHOULD? → **Default used**: Track as **[Gap]** because spec marks it SHOULD (`FR-010`).

## Requirement Completeness

- [x] CHK001 Are all **supported routes** enumerated in one place (including `/logs.v3.json`, `/metrics`, and every `/<log>/...` path) without omissions? [Completeness, Spec §FR-002]
  - Notes: Route list is explicit in `FR-002` and reinforced by `contracts/http.md`.
- [x] CHK002 Are **HTTP methods** explicitly constrained (e.g., support GET+HEAD; 405 for others) for every endpoint? [Completeness, Spec §FR-002a]
- [x] CHK003 Are **response status codes** defined for all endpoints for (a) success, (b) missing content, and (c) invalid inputs? [Completeness, Spec §FR-009; Spec §Edge Cases]
  - Notes: `404` for missing/invalid paths (`FR-009`, Edge Cases), `405` for wrong methods (`FR-002a`), `503` for bad zip integrity and log list v3 refresh failures (`FR-013`, `FR-006`).
- [x] CHK004 Are **Content-Type rules** specified for every served asset class (log list v3, checkpoint, tiles, issuer, JSON files)? [Completeness, Spec §FR-002]
- [x] CHK004a Are **HTTP server timeout/limit** knobs fully enumerated and justified for safety under slow/abusive clients (ReadHeaderTimeout, IdleTimeout, MaxHeaderBytes, etc.)? [Safety, Spec §FR-012]
- [x] CHK005 Are the **zip-part naming conventions** and required files (`000.zip`, `001.zip`, …; `checkpoint`, `log.v3.json`, `tile/...`, `issuer/...`) explicitly stated as requirements (not implied)? [Gap]
  - Notes: Zip part naming (`NNN.zip`) is now explicit under `FR-003`; request→zip-entry mapping is explicit under `FR-009`; `/logs.v3.json` uses `000.zip` + `log.v3.json` per `FR-006`.

## Requirement Clarity

- [x] CHK006 Is “**listen on TCP/8080 by default**” unambiguous about bind address, privilege expectations, and deployment constraints (reverse proxy / container port publishing)? [Clarity, Spec §FR-001; Assumption]
  - Notes: `FR-001` now calls out default `:8080`; deployment expectations are described in `NFR-008` and `NFR-015`.
- [x] CHK007 Is `<log>` derivation from `CT_ARCHIVE_FOLDER_PATTERN` defined precisely, including the supported pattern shape (`<prefix>*`) and the strip algorithm, and is invalid pattern behavior specified? [Clarity, Spec §FR-003a; Spec §Clarifications]
- [x] CHK008 Is the mapping from request path → **zip entry path** fully defined for each endpoint (including exact prefixes like `tile/` vs `tile/data/` and issuer path)? [Clarity, Spec §FR-002; Spec §FR-003]
  - Notes: Mapping is now explicit under `FR-009` examples; `.p/<W>` is treated as literal entry per Edge Cases.
- [x] CHK008a Is the mapping from (tile level/index) → **subtree zip index** deterministic and testable (explicit formula, including L=0/1/2/data and L>=3 shared-metadata rule)? [Clarity, Spec §FR-008]
- [x] CHK009 Is “`logs.v3.json` compatible with common log list v3 consumers” backed by a clear statement of the **minimum required JSON fields and structure**? [Gap, Spec §FR-006]
  - Notes: `FR-006` specifies minimum required top-level shape + deterministic sort + example.
- [x] CHK010 Is “periodically refresh” precisely defined (startup build, interval behavior, clock skew tolerance) and measurable? [Clarity, Spec §FR-006; Spec §FR-007]
  - Notes: Refresh interval is `CT_LOGLISTV3_JSON_REFRESH_INTERVAL` and startup refresh expectation is now described in `FR-006`.

## Requirement Consistency

- [x] CHK011 Are requirement IDs and references **unique and non-duplicated**, so traceability is reliable? [Consistency, Spec §FR-*]
- [x] CHK012 Are user story labels/numbering consistent across artifacts (spec vs tasks) so acceptance criteria map cleanly to implementation tasks? [Consistency, Spec §User Story 1–2; Tasks §US1–US2]
- [x] CHK013 Do the Content-Type requirements avoid duplication/contradiction between the “explicit endpoint list” and the “general rule”? [Consistency, Spec §FR-002]
- [x] CHK014 Do “404 for missing entries” and “no path traversal” requirements align with the edge-case list and success criteria phrasing (no conflicting exceptions)? [Consistency, Spec §FR-009; Spec §Edge Cases; Spec §SC-003]

## Acceptance Criteria Quality (Measurability)

- [x] CHK015 Is checkpoint success defined as a **byte-for-byte match** (and is the comparison source defined) for `/<log>/checkpoint`? [Measurability, Spec §SC-001]
- [x] CHK016 Is tile success defined as **byte-for-byte match** for at least one hash tile and one data tile (and are example tile paths/inputs specified)? [Measurability, Spec §SC-002]
- [x] CHK017 Is `/logs.v3.json` success measurable in terms of **one entry per discovered log**, and does the spec define how “discovered” is determined (folder pattern, required files)? [Measurability, Spec §SC-004; Spec §FR-003; Spec §FR-006]
- [x] CHK018 Is `has_issuers` success measurable and unambiguous (metadata-only check; definition of “any issuer entry”)? [Measurability, Spec §FR-006a; Spec §SC-005]

## Scenario Coverage (Primary / Alternate / Error / Recovery)

- [x] CHK019 Are primary flows specified for each endpoint class: log list v3, checkpoint, log info, hash tiles, data tiles, issuer? [Coverage, Spec §FR-002; Spec §User Stories]
- [x] CHK020 Are alternate flows specified for **multiple logs** (ambiguity when `<log>` not found; behavior when multiple folders could map to same `<log>`)? [Gap, Spec §FR-003; Spec §FR-003a]
  - Notes: `<log>` not found → `404` (`FR-009`); collisions are a startup error (`FR-003b`).
- [x] CHK021 Are error flows specified for “zip part exists but entry missing” vs “zip part missing” vs “zip part present but fails integrity checks (503 temporarily unavailable; cached with TTL)” vs “entry unreadable/corrupt zip”? [Coverage, Spec §FR-013]
- [x] CHK022 Are recovery flows specified for `/logs.v3.json` refresh failures (serve last good snapshot vs empty vs error; log/metrics expectations)? [Gap, Spec §FR-006; Spec §FR-007]
  - Notes: Chosen contract is `503`-until-healthy (not “serve last-good”) per `FR-006` refresh failure behavior.

## Edge Case Coverage

- [x] CHK023 Are path traversal rules comprehensive (encoding tricks, repeated slashes, leading dots, percent-encoding) or at least explicitly scoped? [Coverage, Spec §Edge Cases; Spec §NFR-002]
  - Notes: Spec requires traversal safety; implementation tests cover encoded traversal attempts (`T009`).
- [x] CHK024 Are tile parameter validity rules complete: allowed `<L>` range, tile index encoding rules (tlog "groups-of-three" path encoding; `FR-008a`), and allowed partial widths? [Completeness, Spec §Edge Cases; Spec §FR-008a]
  - Notes: `<L>` bounds are now explicit in Edge Cases; `<N>` encoding is defined in `FR-008a`; `<W>` is defined in Edge Cases.
- [x] CHK025 Are “right-edge partial tile” semantics sufficiently defined to make 404 vs 200 deterministic without implementation guesswork (validate `.p/<W>` where `W` is 1..255; treat `.p/<W>` as a literal zip entry path; `200` iff entry exists, else `404`; no checkpoint-based synthesis)? [Clarity, Spec §Edge Cases]
- [x] CHK026 Is behavior defined for unknown suffixes/paths under `/<log>/...` (strict 404 vs fallback)? [Gap]
  - Notes: Unknown/unsupported routes are `404` per `FR-002a` + `FR-009`.

## Non-Functional Requirements (Performance / Safety / Concurrency)

- [x] CHK027 Is the seekable zip requirement written in a way that is **technology-agnostic but verifiable** (no whole-zip decompression; central directory metadata use)? [Measurability, Spec §NFR-003]
- [x] CHK028 Is the “random-access `archive/zip` mode” requirement consistent with the “standard library only” requirement and avoid over-prescribing implementation details? [Consistency, Spec §NFR-004; Spec §FR-001]
- [x] CHK029 Are concurrency expectations defined (thread-safety; acceptable parallel requests; any per-request locks) or explicitly deferred? [Completeness, Spec §NFR-005]
- [x] CHK030 Are resource limits defined (max response size, timeouts, max open files/zip handles) or explicitly excluded? [Completeness, Spec §FR-011; Spec §FR-012; Spec §NFR-006; Spec §NFR-007]

## Dependencies & Assumptions

- [x] CHK031 Are assumptions about archive layout versioning captured (photocamera-archiver output stability; how to handle unexpected entries/structure changes)? [Assumption, Spec §FR-003; Spec §FR-008]
  - Notes: Layout assumptions are captured in `FR-008` + `FR-009` mapping; unexpected/missing content manifests as `404` (or `503` for integrity failures).
- [x] CHK032 Are environment variables fully enumerated with defaults and descriptions (including logs.v3.json vars, performance tuning vars, and HTTP server timeout/limit vars)? [Completeness, Spec §FR-004; Spec §FR-007; Spec §FR-011; Spec §FR-012]
  - Notes: Env vars and defaults are specified across `FR-007`, `FR-011`, `FR-012`, and `FR-013`.
- [x] CHK033 Is compatibility with Static-CT (C2SP/tiled) clients specified with concrete expectations (what “compatible” means) rather than a vague SHOULD? [Gap, Spec §FR-010]
  - Notes: Compatibility is demonstrated concretely by `SC-001`/`SC-002` and the planned compatibility smoke test (`T036`), while `FR-010` remains a SHOULD.

## Ambiguities & Conflicts (Targeted)

- [x] CHK034 Is “caching behavior” mentioned in the asset-serving story defined (headers, semantics), or removed to avoid ambiguity? [Ambiguity, Spec §User Story 2]
  - Notes: No HTTP caching headers/semantics are required by the spec.
- [x] CHK035 Do story/test statements avoid introducing requirements not present elsewhere (e.g., logs.v3.json fields beyond those required for tiled log discovery) unless explicitly specified? [Conflict, Spec §FR-006; Spec §FR-006a; Tasks §T014]

## Notes

- Check items off as completed: `[x]`
- Record findings inline beneath the relevant item, with links/line references
