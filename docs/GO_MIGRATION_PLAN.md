# SOBS Go Migration Plan (Keep Jinja-Style Templates)

## Goal

Migrate the backend from Python/Quart to Go incrementally while preserving:
- existing template-driven UI behavior
- route and API contracts
- data model and ClickHouse/chDB semantics

## Template Engine Decision

### Selected library: `github.com/flosch/pongo2`

Why:
- Django/Jinja-like syntax (`{% extends %}`, `{% block %}`, `{% include %}`, loops, filters)
- Actively used in Go ecosystems
- Fits current SOBS template structure better than Go `html/template`

Notes:
- `pongo2` is Jinja-like, not 100% Jinja2-compatible. Macro/filter parity must be validated page by page.
- Custom filters/tags will be needed for SOBS-specific helpers (timezone rendering helpers, shared UI macros, format helpers).

## Migration Strategy

### Phase 0: Safety Baseline (Current Python)

Objectives:
- Stabilize known auth/query risks before dual-run migration.
- Define parity test matrix for APIs and rendered pages.

Deliverables:
- Fix PRs for high-severity review findings.
- Golden snapshots for representative page renders and core JSON API responses.

### Phase 1: Go Skeleton + Compatibility Layer

Objectives:
- Introduce a Go service that can render existing templates with minimal edits.
- Keep Python service as source of truth for business logic.

Deliverables:
- New Go app scaffold (router, middleware, config, health endpoint).
- `pongo2` renderer with shared template search paths.
- Compatibility helpers for:
  - global template vars (for example mobile breakpoint token)
  - macro-like helpers currently provided by Jinja context
  - static URL/path helpers

Acceptance criteria:
- Go service renders base layout and at least 3 representative pages without template regressions.

### Phase 2: Read-Only API Porting

Objectives:
- Migrate low-risk read endpoints first while preserving payload shape.

Target endpoints (initial):
- summary/statistics APIs
- read-only list/detail views without write side effects

Deliverables:
- Contract tests that compare Python vs Go responses for the same fixture data.
- Side-by-side benchmark for p50/p95 latency.

Acceptance criteria:
- Response parity >= 99% (excluding known timestamp formatting tolerances).

### Phase 3: Ingest + Write Path Porting

Objectives:
- Move OTEL/RUM ingest and internal write queues to Go.
- Preserve schema compatibility and retention/window behavior.

Deliverables:
- Go equivalents for write queueing, retry behavior, and schema initialization.
- Backpressure controls equivalent to `WRITE_QUEUE_MAX`, `WRITE_BATCH_MAX`, and related safeguards.

Acceptance criteria:
- Ingest throughput and error rates are equal or better than Python baseline.

### Phase 4: AI/Agent Feature Porting

Objectives:
- Migrate AI assistant, guardrails, and GitHub automation flows with no security regression.

Deliverables:
- Endpoint compatibility for AI settings, chat flows, and guard checks.
- Explicit secret-handling parity (encryption, masking, redaction).

Acceptance criteria:
- Existing AI tests pass against Go endpoints in compatibility mode.

### Phase 5: Cutover and Decommission

Objectives:
- Switch default runtime to Go.
- Keep Python fallback until production stability window closes.

Deliverables:
- deployment flag to select runtime (`python`, `go`, `dual`)
- rollback playbook
- deprecation timeline for Python service path

Acceptance criteria:
- Two release cycles with no Sev1/Sev2 regressions on Go primary.

## Target Go Architecture

Proposed package layout:

- `cmd/sobs/main.go` bootstrap, config, startup checks
- `internal/web` router, middleware, auth, CSRF, static assets, template rendering
- `internal/templates` pongo2 environment, custom filters/tags, shared context providers
- `internal/db` query layer, schema/bootstrap, row mapping, retries
- `internal/features` domain modules (logs, traces, errors, rum, metrics, ai, settings)
- `internal/ingest` OTEL/RUM ingest pipelines and write batching
- `internal/agents` issue/PR orchestration and policy checks

## Contract and Parity Requirements

- Preserve existing endpoint paths and status codes where possible.
- Preserve JSON field names and error shapes.
- Keep template block structure and page layout behavior unchanged.
- Preserve responsive/mobile card behavior and existing helper macros.

## Risk Register

1. Template incompatibility (`pongo2` vs Jinja2).
- Mitigation: compatibility test suite on all templates, helper shim library.

2. Query behavior drift (type coercion/timezone/NULL semantics).
- Mitigation: golden-query fixtures and response diff tooling.

3. Auth/CSRF behavior mismatch during dual-run.
- Mitigation: shared auth conformance tests before any endpoint cutover.

4. Performance regressions in ingest path.
- Mitigation: load tests and staged shadow traffic.

## PR Roadmap

1. PR A: Security/reliability fixes from review findings.
2. PR B: Go skeleton + pongo2 renderer + health/template smoke tests.
3. PR C+: Domain-by-domain endpoint migration with parity gates.

## Definition of Done (Migration)

- Go service is default runtime.
- Python service retained only as rollback path for one stabilization window.
- Parity matrix green for APIs/templates across required viewports and auth modes.
- Runbooks, alerts, and operational docs updated.
