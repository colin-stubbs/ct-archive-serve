<!--
Sync Impact Report

- Version change: 1.0.1 → 1.0.2
- Modified principles: align integrity verification to ct-archive-serve’s role (ZIP structural integrity only), and make outbound-network-call guidance conditional while retaining reverse-proxy delegation for edge security controls
- Added sections: Constraints & Standards; Development Workflow & Quality Gates
- Removed sections: none
- Templates requiring updates:
  - None (templates are present in this repo under `.specify/templates/`)
- Follow-up TODOs: none
-->

# ct-archive-serve Constitution

## Core Principles

### I. Go-first, clean boundaries
- Code MUST be idiomatic Go and formatted with `gofmt`.
- Public, reusable code MUST live in `pkg/`; application code MUST live in `internal/`; entrypoints MUST live in `cmd/`.
- Dependencies MUST be explicit (constructor injection); avoid package-level mutable globals.
- Concurrency MUST be context-driven and cancellation-safe (no leaked goroutines).

Rationale: This repository focuses on `ct-archive-serve`; clear boundaries keep it maintainable and testable.

### II. CT correctness is non-negotiable
- Implementations MUST be correct with respect to Certificate Transparency specifications and operator behavior
  (RFC 6962 + Static CT/tiled logs).
- Data integrity MUST be verified where feasible **within the service’s role and inputs**.
  - For `ct-archive-serve`, integrity verification is limited to **ZIP structural validity checks** (central directory + local headers) for zip parts being accessed, with pass/fail caching and temporary-unavailability behavior defined by the spec.
  - Merkle/inclusion verification is **out of scope** for `ct-archive-serve` serving paths and is the responsibility of downstream CT clients.
- Wire formats and responses MUST be deterministic and precisely specified (especially where other tooling depends
  on them).
- When a behavior is ambiguous in upstream ecosystems, we MUST document the chosen contract and test it.

Rationale: Consumers rely on CT-derived data for security and monitoring; correctness beats convenience.

### III. Test-first changes for critical paths (non-negotiable)
- New or changed behavior in CT download/storage/verification/serving paths MUST include tests that fail before
  the fix and pass after.
- Tests MUST be table-driven where appropriate and MUST run in parallel when safe.
- Boundary-heavy code (HTTP clients, parsers, storage) MUST be interface-driven so it can be mocked cleanly.

Rationale: This project interacts with hostile/variable inputs and large datasets; regressions are costly.

### IV. Observability and operational safety by default
- Long-running processes MUST expose actionable metrics (Prometheus) and structured logs with clear error context.
- If the service makes outbound network calls, they MUST have timeouts, retries with backoff, and clear error classification.
- Resource usage MUST be bounded and observable (memory, goroutine count, queue depth, open files/db handles).

Rationale: CT workloads are continuous and spiky; safe defaults and visibility are required for production ops.

### V. Secure-by-default inputs and limits
- All external inputs (HTTP requests, environment variables, files) MUST be validated and size-limited.
- APIs MUST apply explicit limits (request size, header size, query complexity, rate limiting) and return safe
  error messages (no secrets). These limits MUST be enforced either:
  - in-process by the service, OR
  - by a reverse proxy that the design explicitly REQUIRES for safe operation.
- **Exception (reverse proxy security boundary)**: If the design requires a reverse proxy to implement edge security
  controls (e.g., TLS termination, rate limiting, WAF rules, inbound authentication), then the service is NOT
  required to implement those edge security controls in-process. In that case, the service MUST:
  - document the required reverse proxy controls and assumptions (what is enforced at the proxy boundary),
  - treat `X-Forwarded-*` headers as trustworthy only when they come from that proxy boundary,
  - continue to validate all inputs and enforce internal safety limits that remain relevant behind a proxy
    (e.g., timeouts, header-size limits, path traversal prevention, bounded resource usage).
- Cryptographic material and credentials MUST NOT be logged; secrets MUST be passed via environment/config
  mechanisms appropriate to deployment.

Rationale: The system sits on the network boundary and processes untrusted data at scale.

## Constraints & Standards

- **Supported runtime**: Go 1.25.5+ (align with tooling and CI).
- **External calls**: If the service makes outbound network calls, every request MUST have a timeout; retries (if used) MUST use exponential backoff and jitter.
- **Configuration**: Environment variables are the primary configuration mechanism; flags are reserved for UX/help
  and non-secret toggles unless explicitly documented otherwise.
- **Storage**: Data layout and on-disk formats MUST remain backwards compatible unless a migration plan is
  provided and versioned.
- **Documentation**: Public packages MUST have GoDoc; operator-facing features MUST be documented in `README.md`
  and/or `docs/`.

## Development Workflow & Quality Gates

- **Before merge**:
  - `go test ./...` MUST pass (and relevant integration/contract tests when the change touches protocols).
  - `golangci-lint run` SHOULD pass (or a scoped, documented justification MUST be provided).
  - Security checks (e.g., `govulncheck`, `trivy`) SHOULD be run for dependency-impacting changes.
- **Design changes**:
  - Significant architectural changes MUST update `docs/ARCHITECTURE.md`.
  - Spec-driven work MUST keep spec docs and checklists consistent with implementation.
- **Change tracking**:
  - `CHANGES.md` MUST be updated for user-visible behavior changes and governance/principle updates.

## Governance

- This constitution is the highest-level guidance for this repository; when conflicts arise, this document wins.
- Amendments MUST:
  - Update this document (including Sync Impact Report, version, and dates),
  - Explain the rationale and any migration/transition plan,
  - Update affected docs/templates/checklists where they exist.
- Versioning policy:
  - **MAJOR**: backward-incompatible governance/principle changes.
  - **MINOR**: new principle/section or materially expanded mandatory guidance.
  - **PATCH**: clarifications, wording, typo fixes, and non-semantic refinements.
- Compliance review expectation:
  - PR reviews MUST check changes against these principles.
  - For implementation guidance, also refer to `AGENTS.md` and `docs/ARCHITECTURE.md`.

**Version**: 1.0.2 | **Ratified**: 2025-09-18 | **Last Amended**: 2026-01-21
