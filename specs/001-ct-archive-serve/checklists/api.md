# API Requirements Quality Checklist: ct-archive-serve

**Purpose**: Unit-test the written requirements for `ct-archive-serve`’s HTTP API, routing, and archive-backed content serving (clarity, completeness, consistency, and measurability).
**Created**: 2026-01-20
**Feature**: `specs/001-ct-archive-serve/spec.md` (with supporting artifacts `plan.md`, `tasks.md`)

### Clarifying questions (resolved by defaults for this checklist)

- **Q1**: Is this checklist intended as a lightweight author self-check or a PR/reviewer gate? → **Default used**: Reviewer (PR gate).
- **Q2**: Should the checklist focus only on HTTP/API semantics or also include archive/zip performance + safety constraints? → **Default used**: Include both (API + zip/seekability + safety).
- **Q3**: Should compatibility with existing Static-CT client behavior be treated as a MUST-level quality gate or informational SHOULD? → **Default used**: Track as **[Gap]** because spec marks it SHOULD (`FR-010`).

## Requirement Completeness

- [ ] CHK001 Are all **supported routes** enumerated in one place (including `/monitor.json`, `/metrics`, and every `/<log>/...` path) without omissions? [Completeness, Spec §FR-002]
- [ ] CHK002 Are **HTTP methods** explicitly constrained (e.g., GET-only; behavior for HEAD/others) for every endpoint? [Gap]
- [ ] CHK003 Are **response status codes** defined for all endpoints for (a) success, (b) missing content, and (c) invalid inputs? [Completeness, Spec §FR-009; Spec §Edge Cases]
- [ ] CHK004 Are **Content-Type rules** specified for every served asset class (monitor list, checkpoint, tiles, issuer, JSON files)? [Completeness, Spec §FR-002]
- [ ] CHK004a Are **HTTP server timeout/limit** knobs fully enumerated and justified for safety under slow/abusive clients (ReadHeaderTimeout, IdleTimeout, MaxHeaderBytes, etc.)? [Safety, Spec §FR-012]
- [ ] CHK005 Are the **zip-part naming conventions** and required files (`000.zip`, `001.zip`, …; `checkpoint`, `log.v3.json`, `tile/...`, `issuer/...`) explicitly stated as requirements (not implied)? [Gap]

## Requirement Clarity

- [ ] CHK006 Is “**listen on TCP/8080 by default**” unambiguous about bind address, privilege expectations, and deployment constraints (reverse proxy / container port publishing)? [Clarity, Spec §FR-001; Assumption]
- [ ] CHK007 Is `<log>` derivation from `CT_ARCHIVE_FOLDER_PATTERN` defined precisely, including the supported pattern shape (`<prefix>*`) and the strip algorithm, and is invalid pattern behavior specified? [Clarity, Spec §FR-003a; Spec §Clarifications]
- [ ] CHK008 Is the mapping from request path → **zip entry path** fully defined for each endpoint (including exact prefixes like `tile/` vs `tile/data/` and issuer path)? [Clarity, Spec §FR-002; Spec §FR-003]
- [ ] CHK008a Is the mapping from (tile level/index) → **subtree zip index** deterministic and testable (explicit formula, including L=0/1/2/data and L>=3 shared-metadata rule)? [Clarity, Spec §FR-008]
- [ ] CHK009 Is “`monitor.json` compatible with common log list v3 consumers” backed by a clear statement of the **minimum required JSON fields and structure**? [Gap, Spec §FR-006]
- [ ] CHK010 Is “periodically refresh” precisely defined (startup build, interval behavior, clock skew tolerance) and measurable? [Clarity, Spec §FR-006; Spec §FR-007]

## Requirement Consistency

- [ ] CHK011 Are requirement IDs and references **unique and non-duplicated**, so traceability is reliable? [Consistency, Spec §FR-*]
- [ ] CHK012 Are user story labels/numbering consistent across artifacts (spec vs tasks) so acceptance criteria map cleanly to implementation tasks? [Consistency, Spec §User Story 1–2; Tasks §US1–US2]
- [ ] CHK013 Do the Content-Type requirements avoid duplication/contradiction between the “explicit endpoint list” and the “general rule”? [Consistency, Spec §FR-002]
- [ ] CHK014 Do “404 for missing entries” and “no path traversal” requirements align with the edge-case list and success criteria phrasing (no conflicting exceptions)? [Consistency, Spec §FR-009; Spec §Edge Cases; Spec §SC-003]

## Acceptance Criteria Quality (Measurability)

- [ ] CHK015 Is checkpoint success defined as a **byte-for-byte match** (and is the comparison source defined) for `/<log>/checkpoint`? [Measurability, Spec §SC-001]
- [ ] CHK016 Is tile success defined as **byte-for-byte match** for at least one hash tile and one data tile (and are example tile paths/inputs specified)? [Measurability, Spec §SC-002]
- [ ] CHK017 Is `/monitor.json` success measurable in terms of **one entry per discovered log**, and does the spec define how “discovered” is determined (folder pattern, required files)? [Measurability, Spec §SC-004; Spec §FR-003; Spec §FR-006]
- [ ] CHK018 Is `has_issuers` success measurable and unambiguous (metadata-only check; definition of “any issuer entry”)? [Measurability, Spec §FR-006a; Spec §SC-005]

## Scenario Coverage (Primary / Alternate / Error / Recovery)

- [ ] CHK019 Are primary flows specified for each endpoint class: monitor list, checkpoint, log info, hash tiles, data tiles, issuer? [Coverage, Spec §FR-002; Spec §User Stories]
- [ ] CHK020 Are alternate flows specified for **multiple logs** (ambiguity when `<log>` not found; behavior when multiple folders could map to same `<log>`)? [Gap, Spec §FR-003; Spec §FR-003a]
- [ ] CHK021 Are error flows specified for “zip part exists but entry missing” vs “zip part missing” vs “entry unreadable/corrupt zip”? [Coverage, Gap]
- [ ] CHK022 Are recovery flows specified for `/monitor.json` refresh failures (serve last good snapshot vs empty vs error; log/metrics expectations)? [Gap, Spec §FR-006; Spec §FR-007]

## Edge Case Coverage

- [ ] CHK023 Are path traversal rules comprehensive (encoding tricks, repeated slashes, leading dots, percent-encoding) or at least explicitly scoped? [Coverage, Spec §Edge Cases; Spec §NFR-002]
- [ ] CHK024 Are tile parameter validity rules complete: allowed `<L>` range, tile index encoding rules, and allowed partial widths? [Completeness, Spec §Edge Cases]
- [ ] CHK025 Are “right-edge partial tile” semantics sufficiently defined to make 404 vs 200 deterministic without implementation guesswork (validate `.p/<W>` where `W` is 1..255; treat `.p/<W>` as a literal zip entry path; `200` iff entry exists, else `404`; no checkpoint-based synthesis)? [Clarity, Spec §Edge Cases]
- [ ] CHK026 Is behavior defined for unknown suffixes/paths under `/<log>/...` (strict 404 vs fallback)? [Gap]

## Non-Functional Requirements (Performance / Safety / Concurrency)

- [ ] CHK027 Is the seekable zip requirement written in a way that is **technology-agnostic but verifiable** (no whole-zip decompression; central directory metadata use)? [Measurability, Spec §NFR-003]
- [ ] CHK028 Is the “random-access `archive/zip` mode” requirement consistent with the “standard library only” requirement and avoid over-prescribing implementation details? [Consistency, Spec §NFR-004; Spec §FR-001]
- [ ] CHK029 Are concurrency expectations defined (thread-safety; acceptable parallel requests; any per-request locks) or explicitly deferred? [Completeness, Spec §NFR-005]
- [ ] CHK030 Are resource limits defined (max response size, timeouts, max open files/zip handles) or explicitly excluded? [Completeness, Spec §FR-011; Spec §FR-012; Spec §NFR-006; Spec §NFR-007]

## Dependencies & Assumptions

- [ ] CHK031 Are assumptions about archive layout versioning captured (photocamera-archiver output stability; how to handle unexpected entries/structure changes)? [Assumption, Spec §FR-003; Spec §FR-008]
- [ ] CHK032 Are environment variables fully enumerated with defaults and descriptions (including monitor.json vars, performance tuning vars, and HTTP server timeout/limit vars)? [Completeness, Spec §FR-004; Spec §FR-007; Spec §FR-011; Spec §FR-012]
- [ ] CHK033 Is compatibility with Static-CT (C2SP/tiled) clients specified with concrete expectations (what “compatible” means) rather than a vague SHOULD? [Gap, Spec §FR-010]

## Ambiguities & Conflicts (Targeted)

- [ ] CHK034 Is “caching behavior” mentioned in the asset-serving story defined (headers, semantics), or removed to avoid ambiguity? [Ambiguity, Spec §User Story 2]
- [ ] CHK035 Do story/test statements avoid introducing requirements not present elsewhere (e.g., monitor.json fields beyond those required for tiled log discovery) unless explicitly specified? [Conflict, Spec §FR-006; Spec §FR-006a; Tasks §T014]

## Notes

- Check items off as completed: `[x]`
- Record findings inline beneath the relevant item, with links/line references
