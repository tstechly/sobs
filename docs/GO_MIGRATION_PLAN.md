# SOBS Go Migration Plan (Keep Jinja-Style Templates)

## Goal

Migrate the backend from Python/Quart to Go incrementally while preserving:
- existing template-driven UI behavior
- route and API contracts
- data model and ClickHouse/chDB semantics
- SOBS as an OTLP receiver/service for traces, metrics, and logs ingest

Migration scope assumption:
- This is a forward-only migration. We are not carrying backward-compatibility obligations for existing installations.
- We can make clean breaking changes to runtime/bootstrap/deployment shape as needed, as long as the new Go path is well documented.

## Template Engine Decision

### Selected library: `github.com/flosch/pongo2`

Why:
- Django/Jinja-like syntax (`{% extends %}`, `{% block %}`, `{% include %}`, loops, filters)
- Actively used in Go ecosystems
- Fits current SOBS template structure better than Go `html/template`

Notes:
- `pongo2` is Jinja-like, not 100% Jinja2-compatible. Macro/filter parity must be validated page by page.
- Custom filters/tags will be needed for SOBS-specific helpers (timezone rendering helpers, shared UI macros, format helpers).

## OTEL Receiver Libraries (Go)

Use OTEL receiver/server-side libraries (not app instrumentation middleware):
- `google.golang.org/grpc`
- `go.opentelemetry.io/proto/otlp/collector/trace/v1`
- `go.opentelemetry.io/proto/otlp/collector/metrics/v1`
- `go.opentelemetry.io/proto/otlp/collector/logs/v1`
- `go.opentelemetry.io/proto/otlp/common/v1`
- `go.opentelemetry.io/proto/otlp/resource/v1`
- `go.opentelemetry.io/collector/pdata/plog`
- `go.opentelemetry.io/collector/pdata/pmetric`
- `go.opentelemetry.io/collector/pdata/ptrace`

## Migration Strategy (Single PR)

### Phase 1: Go Skeleton + Compatibility Layer

Objectives:
- Introduce a Go service that can render existing templates with minimal edits.
- Build Go as the primary and only target runtime (no dual-run dependency).

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
- Move OTLP receiver handlers, RUM ingest, and internal write queues to Go.
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
- Complete one-way cutover to Go runtime.

Deliverables:
- production deployment artifacts for Go runtime only
- installation and operations docs updated for Go-first deployment
- explicit removal/deprecation plan for Python runtime paths in repository

Acceptance criteria:
- Go runtime is the only supported runtime for new deployments.
- Python runtime path is removed or marked unsupported for forward releases.

## Target Go Architecture

Proposed package layout:

- `cmd/sobs/main.go` bootstrap, config, startup checks
- `internal/web` router, middleware, auth, CSRF, static assets, template rendering
- `internal/templates` pongo2 environment, custom filters/tags, shared context providers
- `internal/db` query layer, schema/bootstrap, row mapping, retries
- `internal/features` domain modules (logs, traces, errors, rum, metrics, ai, settings)
- `internal/ingest` OTEL/RUM ingest pipelines and write batching
- `internal/agents` issue/PR orchestration and policy checks

## Extension Point Signatures

```go
package extensionpoints

import (
  "context"
  "net/http"
)

type Identity struct {
  Subject string
  Email   string
  Roles   []string
}

type AuthProvider interface {
  Authenticate(ctx context.Context, r *http.Request) (Identity, error)
  Authorize(ctx context.Context, id Identity, permission string) error
}

type RowIterator interface {
  Next() bool
  Scan(dest ...any) error
  Err() error
  Close() error
}

type Result interface {
  RowsAffected() (int64, error)
}

type ClickHouseStore interface {
  Ping(ctx context.Context) error
  Query(ctx context.Context, query string, args ...any) (RowIterator, error)
  Exec(ctx context.Context, query string, args ...any) (Result, error)
  Close() error
}

type StoreFactory interface {
  Open(ctx context.Context) (ClickHouseStore, error)
}
```

## Contract and Parity Requirements

- Preserve existing endpoint paths and status codes where possible.
- Preserve JSON field names and error shapes.
- Keep template block structure and page layout behavior unchanged.
- Preserve responsive/mobile card behavior and existing helper macros.
- Ensure Go auth/session handling always uses configured session cookie name (no hardcoded cookie key fallback behavior).
- Ensure Go CSRF origin checking does not trust forwarded host/proto headers unless trusted-proxy mode is explicitly enabled.

## Risk Register

1. Template incompatibility (`pongo2` vs Jinja2).
- Mitigation: compatibility test suite on all templates, helper shim library.

2. Query behavior drift (type coercion/timezone/NULL semantics).
- Mitigation: golden-query fixtures and response diff tooling.

3. Auth/CSRF behavior mismatch during migration.
- Mitigation: enforce explicit conformance tests for cookie-name configuration and trusted-proxy-gated forwarded-header behavior in Go implementation.

4. Performance regressions in ingest path.
- Mitigation: load tests and staged shadow traffic.

## PR Roadmap

1. Single PR: Complete migration end-to-end in one change set.
2. The PR includes Go skeleton, template migration, endpoint migration, ingest/write migration, AI/agent migration, parity tests, and Go-side auth/CSRF non-regression coverage.

## Definition of Done (Migration)

- Go service is default runtime.
- No dual-runtime requirement or rollback obligation for existing installations.
- Parity matrix green for APIs/templates across required viewports and auth modes.
- Runbooks, alerts, and operational docs updated.
- Go implementation includes explicit coverage for:
  - configured session cookie-name handling in external-auth/browser flows
  - trusted-proxy boundary enforcement for forwarded-header-based origin checks
