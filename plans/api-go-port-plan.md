# API Port Plan: Python to Go

## Scope

This plan covers only machine-facing API behavior in `app.py`.

Excluded from scope:
- HTML page routes
- template rendering
- browser UI workflows
- static asset pages except where they directly support API consumers

Included in scope:
- ingest endpoints under `/v1/*`
- JSON APIs under `/api/*`
- SSE streaming via `/tail`
- health endpoints used for smoke checks
- auth and storage behavior required by those APIs

## Domain Context

SOBS is a single-binary observability service with embedded analytics storage.
It ingests OpenTelemetry logs, traces, and metrics, plus RUM, direct error events, AI transparency events, and operational metadata.
The API then exposes read and control surfaces for search, reporting, dashboards, enrichment, notifications, onboarding, and automation.

Core domain entities observed from the codebase:
- OTEL logs in `otel_logs`
- OTEL traces in `otel_traces`
- OTEL metrics in `otel_metrics_gauge`, `otel_metrics_sum`, `otel_metrics_histogram`
- RUM session events in `hyperdx_sessions`
- application and release registry records
- reports, dashboards, work items, tags, notification config, settings

## Current API Shape

### 1. Ingest APIs
- `/v1/logs`
- `/v1/traces`
- `/v1/metrics`
- `/v1/rum`
- `/v1/rum/assets`
- `/v1/rum/client-token`
- `/v1/ai`
- `/v1/errors`
- `/v1/apps`
- `/v1/apps/<app_id>`
- `/v1/apps/<app_id>/releases`
- `/v1/releases/<release_id>`
- `/v1/releases/<release_id>/artifacts`
- `/v1/releases/<release_id>/artifacts/meta`

### 2. Read and control APIs under `/api/*`
Grouped by capability:

- traces and evidence
  - `/api/traces/span/<span_id>`
- web traffic and enrichment
  - `/api/web-traffic/*`
  - `/api/enrichment/*`
- work items and automation
  - `/api/work-items`
  - `/api/issues/raise`
  - `/api/agent/runs*`
- AI and query
  - `/api/ai/*`
  - `/api/query/*`
- dashboards and reports
  - `/api/dashboards/*`
  - `/api/reports*`
- settings and metadata helpers
  - `/api/settings/masking/*`
  - `/api/settings/tags/*`
  - `/api/logs/*`
  - `/api/errors/validate-regex`
  - `/api/traces/validate-regex`
  - `/api/metrics/validate-regex`
  - `/api/rum/validate-regex`
  - `/api/chart-types`
  - `/api/table-explorer/*`
- notifications
  - `/api/notifications/*`
- kubernetes and data management
  - `/api/kubernetes/status`
  - `/api/data-management/*`
- setup and onboarding
  - `/api/setup-wizard/steps`
  - `/api/onboarding/*`
- tags write API
  - `/api/tags/<record_type>/<record_id>*`
- MCP config APIs
  - `/api/mcp/*`

### 3. Streaming and health
- `/tail`
- `/health`
- `/health/db`

## Observed Backend Architecture in Python

### Request handling
A single Quart app in `app.py` owns routing, validation, auth, SQL, file IO, background tasks, and response shaping.

### Storage model
The service uses embedded `chDB` with ClickHouse-style tables and JSONEachRow style writes.
The current Python code keeps a process-global DB connection and lazily initializes schema.

### Write path
Ingest routes commonly follow this pattern:
1. parse request
2. normalize to internal event rows
3. enqueue writes to a background write queue
4. optionally wait in test mode
5. broadcast SSE events for selected records
6. return small JSON acknowledgements

### Query path
Read APIs often:
1. parse query params or JSON body
2. build constrained SQL
3. execute directly against chDB
4. convert rows to JSON-safe payloads
5. apply masking before UI-facing responses when needed

### Auth split
Two auth families are important:
- ingest auth via `X-API-Key` and managed CI keys
- UI and many `/api/*` endpoints via basic auth or external bearer validation

This auth split should remain explicit in Go instead of being inferred indirectly from route decorators.

### Non-DB state
Some behavior depends on local process memory or filesystem state:
- SSE subscriber registry
- in-memory caches
- write queue and worker thread
- RUM browser context cache
- RUM asset files plus JSON metadata on disk

## Porting Recommendation

Do not port `app.py` route-by-route into one Go file.
Split the Go API into domain modules with shared infrastructure.

## Proposed Go Module Layout

```text
cmd/sobs-api/
internal/http/
internal/auth/
internal/config/
internal/storage/
internal/ingest/
internal/query/
internal/stream/
internal/rum/
internal/ai/
internal/apps/
internal/reports/
internal/dashboards/
internal/enrichment/
internal/notifications/
internal/tags/
internal/workitems/
internal/onboarding/
internal/kubernetes/
internal/datamgmt/
internal/mcp/
internal/health/
internal/masking/
internal/telemetry/
```

## Recommended Module Responsibilities

### `internal/config`
- env parsing
- runtime feature flags
- auth mode selection
- limits and tuning knobs

### `internal/http`
- router setup
- middleware chain
- JSON helpers
- error envelope helpers
- request ID and logging middleware

### `internal/auth`
- ingest API key validation
- managed CI key validation
- basic auth
- external bearer validation
- same-origin session fallback if kept
- route policy mapping

### `internal/storage`
- DB lifecycle
- schema bootstrap
- repository helpers
- SQL execution wrapper
- write queue abstraction
- transactions or serialized writes where needed

### `internal/stream`
- SSE subscriber broker
- keepalive handling
- event fanout for logs, traces, AI

### `internal/ingest`
- OTLP HTTP parse for logs, traces, metrics
- gzip and deflate request decoding
- protobuf and JSON decode
- ingest event normalization
- queue submission

### `internal/rum`
- RUM event validation and normalization
- traceparent extraction
- browser context delta cache
- client token issuance
- asset upload signature verification
- asset metadata storage and download

### `internal/ai`
- AI transparency ingest
- AI helper and conversation APIs
- LLM provider abstraction
- guard model checks
- prompt and response telemetry emission

### `internal/query`
- NL to SQL flow
- read-only SQL validation
- explain and repair loop
- table explorer and schema APIs
- field hints and filter validation

### `internal/apps`
- app registry CRUD
- release registry CRUD
- artifact metadata registration
- CI key linkage

### `internal/reports`
- saved report CRUD
- import and export

### `internal/dashboards`
- dashboard query APIs
- template and spec compile flows
- render and validation handlers
- AI build integration

### `internal/enrichment`
- web traffic rollups
- geo lookup
- CVE scan orchestration
- GitHub repo health queries

### `internal/notifications`
- channel config
- rules
- webhook and slack dispatch adapters
- browser push subscription endpoints

### `internal/tags`
- tag rule evaluation
- tag CRUD API
- auto-tag application hooks from ingest

### `internal/workitems`
- issue and run tracking
- GitHub-backed work item flows

### `internal/onboarding`
- setup wizard APIs
- repo inspection and repo import APIs
- issue creation bootstrap flows

### `internal/kubernetes`
- OTEL-backed cluster status queries
- filter parsing and pagination

### `internal/datamgmt`
- backup listing and run
- restore
- prune
- TTL policy helpers

### `internal/mcp`
- MCP key management APIs
- enable and disable status APIs
- MCP request authentication support

### `internal/health`
- liveness and DB readiness handlers

### `internal/masking`
- output masking
- regex safety checks
- shared response scrubbing hooks

### `internal/telemetry`
- internal spans and metrics for the Go service itself

## Suggested Delivery Phases

### Phase 1: foundational API platform
- config
- auth
- storage bootstrap
- health endpoints
- shared JSON and error helpers

### Phase 2: ingest parity
- `/v1/logs`
- `/v1/traces`
- `/v1/metrics`
- `/v1/errors`
- `/v1/ai`
- `/v1/rum`
- `/tail`

### Phase 3: machine-facing registry APIs
- apps and releases
- tags
- reports
- table explorer
- chart types

### Phase 4: query and dashboard APIs
- `/api/query/*`
- `/api/dashboards/*`
- filter validation helpers

### Phase 5: ops and automation APIs
- enrichment
- notifications
- work items
- agent runs
- kubernetes
- data management
- onboarding
- MCP

## Migration Notes and Risks

### 1. Embedded storage behavior
Python currently assumes a single-process embedded `chDB` model.
In Go, confirm whether the target remains embedded ClickHouse-compatible storage or changes to a separate ClickHouse service.
This decision affects connection handling, migrations, concurrency, and packaging.

### 2. Async queue semantics
The write queue is part of ingest reliability and test determinism.
Go needs a deliberate replacement using buffered channels, worker goroutines, backpressure limits, and graceful shutdown.

### 3. Dual OTLP formats
`/v1/logs`, `/v1/traces`, and `/v1/metrics` accept protobuf and JSON with optional compression.
That parser path should be implemented once and reused.

### 4. Auth inconsistency by route family
Some machine-facing APIs use ingest auth while many `/api/*` routes use basic or external auth.
This is easy to break during a port.
Create an explicit route policy matrix before coding.

### 5. SSE semantics
`/tail` depends on in-memory subscribers and best-effort broadcast.
A single-instance Go service can match this easily.
A multi-instance deployment will require a broker if parity is expected.

### 6. Filesystem-backed RUM assets
RUM assets are not just DB rows.
They require file writes, metadata files, signature validation, and download serving.

### 7. LLM-dependent APIs
Query, AI helper, onboarding, and issue automation call external AI and GitHub services.
These modules should be isolated behind interfaces early.

### 8. Response compatibility
Some clients and tests depend on exact field names such as `accepted`, `ok`, `error`, `rows`, and `columns`.
Contract compatibility matters more than internal structure.

## Smoke Test Strategy for the Go Port

Target smoke tests should prove transport, auth, write path, read path, and streaming.
Keep them API-only.

### Minimum smoke suite

#### Health
- `GET /health` returns 200
- `GET /health/db` returns 200 and confirms DB ready

#### Ingest
- `POST /v1/logs` with OTLP JSON returns `accepted: 1`
- `POST /v1/traces` with OTLP JSON returns `accepted: 1`
- `POST /v1/metrics` with OTLP JSON returns `accepted: 1`
- `POST /v1/errors` returns `ok: true`
- `POST /v1/ai` returns `ok: true`
- `POST /v1/rum` returns `accepted: 1`

#### Read back and API correctness
- `GET /api/reports` returns 200 and JSON array
- `POST /api/reports` creates a report
- `DELETE /api/reports/<id>` deletes it
- `GET /api/table-explorer/tables` returns known tables
- `POST /api/query/run` with safe SQL returns rows
- `GET /api/kubernetes/status` returns JSON shape even when no cluster data exists

#### Streaming
- open `/tail`
- send one log or trace
- assert one SSE `data:` event arrives

#### Auth
- unauthorized ingest request fails without `X-API-Key` when configured
- unauthorized `/api/*` request fails without UI auth when configured

#### File-backed RUM asset path
- signed `POST /v1/rum/assets` stores an asset
- `GET /v1/rum/assets/<id>` returns bytes and correct content type

### Best execution style
- reuse the current curl-style request set from `examples/curl_examples.sh`
- mirror the existing integration coverage in `tests/test_integration.py`
- keep smoke tests black-box and HTTP-level
- run against a fresh temp data directory
- avoid UI/browser dependencies

## Suggested First Smoke Test Files in Go

```text
smoke/health_test.go
smoke/ingest_test.go
smoke/reports_test.go
smoke/query_test.go
smoke/tail_test.go
smoke/rum_assets_test.go
smoke/auth_test.go
```

## Recommended API Port Order

1. core runtime, config, auth, storage
2. health
3. OTLP ingest and direct ingest
4. SSE tail
5. apps and releases
6. reports and table explorer
7. query run and query ask
8. dashboards APIs
9. enrichment, notifications, work items, agent runs
10. kubernetes, data management, onboarding, MCP

## Mermaid Overview

```mermaid
flowchart TD
    A[HTTP request] --> B[Router and auth]
    B --> C{Route family}
    C --> D[Ingest handlers]
    C --> E[Query and read handlers]
    C --> F[Control and config handlers]
    D --> G[Normalizer]
    G --> H[Write queue]
    H --> I[chDB or storage]
    D --> J[SSE broker]
    E --> I
    F --> I
    E --> K[JSON response]
    F --> K
    J --> L[/tail stream]
```

## Recommended Acceptance Criteria for Planning

- route inventory is grouped by domain rather than by one giant file
- auth policy is mapped per endpoint family
- ingest parser is shared across logs, traces, and metrics
- storage writes are serialized or otherwise proven safe
- API contracts for core endpoints are smoke-tested before wider feature work
- GUI routes remain out of scope for the first Go delivery

## Endpoint-by-Endpoint Migration Matrix

### Priority Legend
- `P0` required for first usable API platform
- `P1` high-value next wave
- `P2` secondary but important machine-facing functionality
- `P3` defer until core parity is stable

### Status Legend
- `Port first` include in initial Go implementation
- `Port after core` schedule after ingest and shared infrastructure are stable
- `Defer` keep out of first delivery

### Ingest, Streaming, and Health

| Endpoint | Method | Auth | Primary Go module | Storage or side effects | Priority | Status | Notes |
|---|---|---|---|---|---|---|---|
| `/health` | `GET` | none | `internal/health` | process health only | P0 | Port first | simplest liveness probe |
| `/health/db` | `GET` | none | `internal/health` | DB readiness query | P0 | Port first | confirms storage bootstrap |
| `/v1/logs` | `OPTIONS` | none | `internal/http` | none | P0 | Port first | shared CORS or preflight behavior |
| `/v1/traces` | `OPTIONS` | none | `internal/http` | none | P0 | Port first | shared CORS or preflight behavior |
| `/v1/metrics` | `OPTIONS` | none | `internal/http` | none | P0 | Port first | shared CORS or preflight behavior |
| `/v1/rum/assets` | `OPTIONS` | none | `internal/http` | none | P1 | Port after core | preflight for signed asset upload |
| `/v1/logs` | `POST` | ingest API key | `internal/ingest` | enqueue writes to `otel_logs`, SSE publish | P0 | Port first | supports OTLP JSON and protobuf plus compression |
| `/v1/traces` | `POST` | ingest API key | `internal/ingest` | enqueue writes to `otel_traces` and derived error rows, SSE publish | P0 | Port first | also emits AI-style SSE for GenAI spans |
| `/v1/metrics` | `POST` | ingest API key | `internal/ingest` | enqueue writes to `otel_metrics_*` | P0 | Port first | split by gauge, sum, histogram |
| `/v1/errors` | `POST` | ingest API key | `internal/ingest` | write direct exception rows to `otel_logs` | P0 | Port first | lower complexity than OTLP paths |
| `/v1/ai` | `POST` | ingest API key | `internal/ai` | write AI spans to `otel_traces`, SSE publish | P0 | Port first | keep contract fields stable |
| `/v1/rum` | `POST` | ingest API key plus optional client token checks | `internal/rum` | write to `hyperdx_sessions`, optionally `otel_logs` for browser errors | P0 | Port first | includes traceparent extraction and browser context caching |
| `/tail` | `GET` | basic or external auth | `internal/stream` | in-memory SSE subscriber registration | P0 | Port first | required for smoke testing streaming parity |
| `/v1/rum/assets` | `POST` | ingest API key plus HMAC signature | `internal/rum` | write bytes to asset dir and metadata JSON files | P1 | Port after core | filesystem-backed flow |
| `/v1/rum/assets/<asset_id>` | `GET` | basic or external auth | `internal/rum` | file read by asset ID | P1 | Port after core | serves captured asset bytes |
| `/v1/rum/client-token` | `POST` | ingest API key | `internal/rum` | signed token issuance only | P1 | Port after core | depends on client auth mode config |

### App and Release Registry APIs

| Endpoint | Method | Auth | Primary Go module | Storage or side effects | Priority | Status | Notes |
|---|---|---|---|---|---|---|---|
| `/v1/apps` | `GET` | ingest API key | `internal/apps` | read app registry | P1 | Port after core | machine-facing registry list |
| `/v1/apps` | `POST` | ingest API key | `internal/apps` | create app row | P1 | Port after core | preserve CI automation fields |
| `/v1/apps/<app_id>` | `GET` | ingest API key | `internal/apps` | read app row | P1 | Port after core | |
| `/v1/apps/<app_id>` | `PATCH` | ingest API key | `internal/apps` | update app row | P1 | Port after core | needs partial update semantics |
| `/v1/apps/<app_id>/releases` | `GET` | ingest API key | `internal/apps` | read release rows | P1 | Port after core | |
| `/v1/apps/<app_id>/releases` | `POST` | ingest API key | `internal/apps` | create release row | P1 | Port after core | |
| `/v1/releases/<release_id>` | `GET` | ingest API key | `internal/apps` | read release detail | P1 | Port after core | |
| `/v1/releases/<release_id>/artifacts` | `GET` | ingest API key | `internal/apps` | list artifact metadata | P1 | Port after core | |
| `/v1/releases/<release_id>/artifacts/meta` | `POST` | ingest API key | `internal/apps` | register artifact metadata | P1 | Port after core | ties into release provenance |

### Query, Schema, and Explorer APIs

| Endpoint | Method | Auth | Primary Go module | Storage or side effects | Priority | Status | Notes |
|---|---|---|---|---|---|---|---|
| `/api/query/run` | `POST` | basic or external auth | `internal/query` | read-only SQL execution | P1 | Port after core | easiest non-LLM query endpoint for early value |
| `/api/query/schema` | `GET` | basic or external auth | `internal/query` | introspect allowed tables | P1 | Port after core | useful before NL to SQL |
| `/api/table-explorer/tables` | `GET` | basic or external auth | `internal/query` | list browsable tables | P1 | Port after core | strong smoke-test candidate |
| `/api/table-explorer/table/<name>` | `GET` | basic or external auth | `internal/query` | read schema and samples | P1 | Port after core | |
| `/api/chart-types` | `GET` | basic or external auth | `internal/query` | static metadata read | P1 | Port after core | no DB complexity |
| `/api/logs/field-hints` | `GET` | basic or external auth | `internal/query` | query attribute-key metadata | P1 | Port after core | depends on attr-key cache table behavior |
| `/api/logs/validate-filter` | `POST` | basic or external auth | `internal/query` | validate SQL where clause via DB | P1 | Port after core | |
| `/api/logs/validate-regex` | `POST` | basic or external auth | `internal/query` | regex compile only | P1 | Port after core | cheap helper endpoint |
| `/api/errors/validate-regex` | `POST` | basic or external auth | `internal/query` | regex compile only | P1 | Port after core | |
| `/api/traces/validate-regex` | `POST` | basic or external auth | `internal/query` | regex compile only | P1 | Port after core | |
| `/api/metrics/validate-regex` | `POST` | basic or external auth | `internal/query` | regex compile only | P1 | Port after core | |
| `/api/rum/validate-regex` | `POST` | basic or external auth | `internal/query` | regex compile only | P1 | Port after core | |
| `/api/query/ask` | `POST` | basic or external auth | `internal/query` | LLM calls plus optional DB execution | P2 | Port after core | depends on AI provider abstraction and guardrail flow |
| `/api/query/refine-chart` | `POST` | basic or external auth | `internal/query` | LLM calls plus spec repair | P2 | Port after core | |

### Reports and Dashboard APIs

| Endpoint | Method | Auth | Primary Go module | Storage or side effects | Priority | Status | Notes |
|---|---|---|---|---|---|---|---|
| `/api/reports` | `GET` | basic or external auth | `internal/reports` | read `sobs_reports` | P1 | Port after core | strong CRUD smoke-test target |
| `/api/reports` | `POST` | basic or external auth | `internal/reports` | create `sobs_reports` row | P1 | Port after core | |
| `/api/reports/<report_id>` | `DELETE` | basic or external auth | `internal/reports` | soft delete report row | P1 | Port after core | |
| `/api/reports/export` | `GET` | basic or external auth | `internal/reports` | render JSON attachment | P2 | Port after core | |
| `/api/reports/import` | `POST` | basic or external auth | `internal/reports` | bulk create reports | P2 | Port after core | body size and count limits matter |
| `/api/dashboards/list` | `GET` | basic or external auth | `internal/dashboards` | read dashboards | P2 | Port after core | |
| `/api/query/add-to-dashboard` | `POST` | basic or external auth | `internal/dashboards` | create dashboard chart from query output | P2 | Port after core | crosses query and dashboard modules |
| `/api/dashboards/query` | `POST` | basic or external auth | `internal/dashboards` | execute dashboard data queries | P2 | Port after core | |
| `/api/dashboards/spec/templates` | `GET` | basic or external auth | `internal/dashboards` | static or DB-backed spec templates | P2 | Port after core | |
| `/api/dashboards/spec/options` | `GET` | basic or external auth | `internal/dashboards` | compile-time options metadata | P2 | Port after core | |
| `/api/dashboards/spec/compile` | `POST` | basic or external auth | `internal/dashboards` | compile spec payload | P2 | Port after core | |
| `/api/dashboards/spec/dry-run` | `POST` | basic or external auth | `internal/dashboards` | validate and preview query plan | P2 | Port after core | |
| `/api/dashboards/spec/validate` | `POST` | basic or external auth | `internal/dashboards` | spec validation only | P2 | Port after core | |
| `/api/dashboards/spec/render` | `POST` | basic or external auth | `internal/dashboards` | execute render pipeline | P2 | Port after core | |
| `/api/dashboards/render` | `POST` | basic or external auth | `internal/dashboards` | dashboard chart rendering | P2 | Port after core | |
| `/api/dashboards/spec/ai-build` | `POST` | basic or external auth | `internal/dashboards` | LLM-assisted dashboard generation | P3 | Defer | high coupling to AI helper flows |
| `/api/dashboards/<dashboard_id>/charts/<chart_id>/export` | `GET` | basic or external auth | `internal/dashboards` | export chart config | P2 | Port after core | |
| `/api/dashboards/<dashboard_id>/charts/import` | `POST` | basic or external auth | `internal/dashboards` | import chart config | P2 | Port after core | |
| `/api/metrics/anomaly` | `GET` | basic or external auth | `internal/dashboards` or `internal/query` | anomaly read query | P2 | Port after core | read-only metrics analytics surface |

### Trace, RUM, AI, and Work Item Read APIs

| Endpoint | Method | Auth | Primary Go module | Storage or side effects | Priority | Status | Notes |
|---|---|---|---|---|---|---|---|
| `/api/traces/span/<span_id>` | `GET` | basic or external auth | `internal/query` | read span detail | P1 | Port after core | good trace parity check |
| `/api/web-traffic/geo` | `GET` | basic or external auth | `internal/enrichment` | aggregate RUM geo data | P2 | Port after core | |
| `/api/web-traffic/browsers` | `GET` | basic or external auth | `internal/enrichment` | aggregate RUM browser data | P2 | Port after core | |
| `/api/web-traffic/os` | `GET` | basic or external auth | `internal/enrichment` | aggregate RUM OS data | P2 | Port after core | |
| `/api/web-traffic/timezones` | `GET` | basic or external auth | `internal/enrichment` | aggregate RUM timezone data | P2 | Port after core | |
| `/api/web-traffic/languages` | `GET` | basic or external auth | `internal/enrichment` | aggregate RUM language data | P2 | Port after core | |
| `/api/web-traffic/devices` | `GET` | basic or external auth | `internal/enrichment` | aggregate RUM device data | P2 | Port after core | |
| `/api/work-items` | `GET` | basic or external auth | `internal/workitems` | read work item projections | P2 | Port after core | cached page behavior may be simplified first |
| `/api/ai/span-attributes` | `GET` | basic or external auth | `internal/ai` | query AI span attributes | P2 | Port after core | |
| `/api/ai/conversation` | `GET` | basic or external auth | `internal/ai` | read AI conversation data | P2 | Port after core | |
| `/api/ai/export` | `GET` | basic or external auth | `internal/ai` | export AI records | P2 | Port after core | |
| `/api/ai/field-hints` | `GET` | basic or external auth | `internal/ai` | query AI filter metadata | P2 | Port after core | |
| `/api/ai/validate-filter` | `POST` | basic or external auth | `internal/ai` | validate AI SQL filter | P2 | Port after core | |
| `/api/ai/helper/capabilities` | `GET` | basic or external auth | `internal/ai` | capability manifest | P3 | Defer | helper UX support API |
| `/api/ai/helper/actions/manifest` | `GET` | basic or external auth | `internal/ai` | action manifest | P3 | Defer | |
| `/api/ai/helper/chats` | `GET` | basic or external auth | `internal/ai` | list helper chats | P3 | Defer | |
| `/api/ai/helper/chats/<chat_id>` | `GET` | basic or external auth | `internal/ai` | fetch helper chat detail | P3 | Defer | |
| `/api/ai/helper/feedback` | `POST` | basic or external auth | `internal/ai` | persist feedback | P3 | Defer | |
| `/api/ai/helper` | `POST` | basic or external auth | `internal/ai` | LLM call plus action planning | P3 | Defer | complex conversational workflow |
| `/api/ai/helper/actions/execute` | `POST` | basic or external auth | `internal/ai` | server-side action execution | P3 | Defer | highest coupling and safety surface |

### Enrichment and Automation APIs

| Endpoint | Method | Auth | Primary Go module | Storage or side effects | Priority | Status | Notes |
|---|---|---|---|---|---|---|---|
| `/api/enrichment/libraries` | `GET` | basic or external auth | `internal/enrichment` | query release and library metadata | P2 | Port after core | |
| `/api/enrichment/github/repo-health` | `GET` | basic or external auth | `internal/enrichment` | GitHub API read calls plus cache | P2 | Port after core | external dependency |
| `/api/enrichment/cve/findings` | `GET` | basic or external auth | `internal/enrichment` | query stored CVE findings | P2 | Port after core | |
| `/api/enrichment/cve/findings/<osv_id>/disposition` | `POST` | basic or external auth | `internal/enrichment` | write finding disposition | P2 | Port after core | |
| `/api/enrichment/cve/scan` | `POST` | basic or external auth | `internal/enrichment` | background OSV scan plus DB writes | P3 | Defer | external API plus scheduled behavior |
| `/api/issues/raise` | `POST` | basic or external auth | `internal/workitems` | GitHub issue creation | P3 | Defer | external side effects and AI assistance |
| `/api/agent/runs` | `GET` | basic or external auth | `internal/workitems` | read automation runs | P3 | Defer | |
| `/api/agent/runs` | `POST` | basic or external auth | `internal/workitems` | create automation run and external side effects | P3 | Defer | |
| `/api/agent/runs/<run_id>/dismiss` | `POST` | basic or external auth | `internal/workitems` | dismiss persisted run | P3 | Defer | |

### Settings, Tags, Notifications, and Ops APIs

| Endpoint | Method | Auth | Primary Go module | Storage or side effects | Priority | Status | Notes |
|---|---|---|---|---|---|---|---|
| `/api/settings/masking/preview` | `POST` | basic or external auth | `internal/masking` | pure transform preview | P2 | Port after core | useful shared utility endpoint |
| `/api/settings/masking/rules` | `GET` | basic or external auth | `internal/masking` | read masking config | P2 | Port after core | |
| `/api/settings/tags/condition-suggestions` | `GET` | basic or external auth | `internal/tags` | query suggestion metadata | P2 | Port after core | |
| `/api/tags/<record_type>/<record_id>` | `GET` | ingest API key | `internal/tags` | read applied tags | P1 | Port after core | notable exception inside `/api/*` using ingest auth |
| `/api/tags/<record_type>/<record_id>` | `POST` | ingest API key | `internal/tags` | write tags | P1 | Port after core | |
| `/api/tags/<record_type>/<record_id>/<tag_key>` | `DELETE` | ingest API key | `internal/tags` | delete tag | P1 | Port after core | |
| `/api/notifications/channels/<channel_id>/test` | `POST` | basic or external auth | `internal/notifications` | outbound notification send | P3 | Defer | external side effects |
| `/api/notifications/rules/auto-generate` | `POST` | basic or external auth | `internal/notifications` | generate rules from signals | P3 | Defer | |
| `/api/notifications/check` | `POST` | basic or external auth | `internal/notifications` | evaluate and possibly dispatch notifications | P3 | Defer | |
| `/api/notifications/vapid-public-key` | `GET` | basic or external auth | `internal/notifications` | read push config | P3 | Defer | |
| `/api/notifications/subscribe` | `POST` | basic or external auth | `internal/notifications` | store browser push subscription | P3 | Defer | |
| `/api/notifications/vapid-keygen` | `POST` | basic or external auth | `internal/notifications` | create VAPID keys | P3 | Defer | crypto side effects |
| `/api/notifications/vapid-keys` | `DELETE` | basic or external auth | `internal/notifications` | delete VAPID keys | P3 | Defer | |
| `/api/kubernetes/status` | `GET` | basic or external auth | `internal/kubernetes` | metrics-backed cluster query | P2 | Port after core | no cluster write path |
| `/api/data-management/backup/list` | `GET` | basic or external auth | `internal/datamgmt` | inspect backup files or S3 metadata | P3 | Defer | ops-heavy |
| `/api/data-management/backup/run` | `POST` | basic or external auth | `internal/datamgmt` | backup side effects | P3 | Defer | |
| `/api/data-management/restore` | `POST` | basic or external auth | `internal/datamgmt` | destructive restore path | P3 | Defer | |
| `/api/data-management/prune` | `POST` | basic or external auth | `internal/datamgmt` | destructive prune path | P3 | Defer | |
| `/api/setup-wizard/steps` | `GET` | basic or external auth | `internal/onboarding` | static plus config-driven guidance | P2 | Port after core | machine-facing and low side effect |
| `/api/onboarding/create-repo` | `POST` | basic or external auth | `internal/onboarding` | GitHub repo creation | P3 | Defer | external side effects |
| `/api/onboarding/import-repo` | `POST` | basic or external auth | `internal/onboarding` | repo registration | P3 | Defer | |
| `/api/onboarding/list-repos` | `POST` | basic or external auth | `internal/onboarding` | external GitHub read calls | P3 | Defer | |
| `/api/onboarding/inspect-repo` | `GET` | basic or external auth | `internal/onboarding` | repo inspection | P3 | Defer | |
| `/api/onboarding/create-issues` | `POST` | basic or external auth | `internal/onboarding` | bulk issue creation | P3 | Defer | |
| `/api/mcp/keys` | `GET` | basic or external auth | `internal/mcp` | read MCP key metadata | P3 | Defer | currently tested in `tests/test_mcp.py` |
| `/api/mcp/keys` | `POST` | basic or external auth | `internal/mcp` | create MCP key | P3 | Defer | |
| `/api/mcp/keys/<key_id>` | `DELETE` | basic or external auth | `internal/mcp` | delete MCP key | P3 | Defer | inferred from tests |
| `/api/mcp/enabled` | `POST` | basic or external auth | `internal/mcp` | set MCP enabled flag | P3 | Defer | inferred from tests |

## First Go Milestone Cut Line

Include in first milestone:
- `/health`
- `/health/db`
- `/v1/logs`
- `/v1/traces`
- `/v1/metrics`
- `/v1/errors`
- `/v1/ai`
- `/v1/rum`
- `/tail`

Strong candidates for immediate next wave:
- `/v1/apps*`
- `/v1/releases*`
- `/api/reports*`
- `/api/query/run`
- `/api/query/schema`
- `/api/table-explorer/*`
- `/api/chart-types`
- `/api/tags/*`
