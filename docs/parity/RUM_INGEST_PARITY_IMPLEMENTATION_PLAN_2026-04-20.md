# RUM Ingestion Parity Implementation Plan

## Purpose

This plan converts the Python vs Go RUM ingestion parity audit into an execution sequence that can be implemented and verified in the Go runtime.

It is scoped to the remaining material parity gaps in the ingestion path, not the `/rum` page rendering path.

## Status

- Phase 1 implemented: async worker failures can now be surfaced in tests and are logged during async execution.
- Phase 2 implemented: Go now performs source-map based stack remapping for event and breadcrumb console stacks.
- Phase 3 implemented: configured API keys now protect `/v1/*` ingest routes consistently without depending on a separate toggle.
- Phase 4 implemented: Go now preserves Python-style explicit empty `service` and `errorType` values and logs tag-rule failures instead of swallowing them.
- Plan complete for RUM ingestion parity work covered by this document.

## Source of truth

- Python route: `app.py`
- Go ingest path: `internal/ingest/otlpreceiver/`
- Existing backend audit: `docs/GO_PYTHON_PARITY_AUDIT_2026-04-20.md`

## Target outcome

After this plan is complete, Go RUM ingestion should match Python for:

1. request acceptance and auth semantics
2. stack processing behavior
3. persistence failure visibility and operator diagnostics
4. persisted row shape and downstream error indexing

## Out of scope

- `/rum` page query semantics already tracked separately
- broader OTLP logs, metrics, traces parity
- front-end RUM SDK changes unless required to validate ingest behavior

## Remaining parity gaps

### Gap 1: Async write worker failures are not observable in Go

Python queues writes in `_queue_write(...)` and can surface worker failures in test mode. Go enqueues work in `queuedPipeline`, but `runBatch(...)` currently discards worker errors.

### Gap 2: JS stack demangling is implemented in Python but stubbed in Go

Python performs source-map based remapping for browser error stacks and console breadcrumb stacks. Go follows the same call path but `maybeDemangleJSStack(...)` is currently a no-op.

### Gap 3: API-key enforcement semantics are only partially aligned

The production Go server does protect `/v1/rum` through `wrapSecurity(...)`, but its enforcement is gated by `SOBS_ENFORCE_API_AUTH`. Python applies `require_api_key` directly on the route and enforces based on configured keys rather than a separate global switch.

### Deferred minor drift

- empty `service` fallback behavior
- empty `errorType` fallback behavior
- tag-rule failure handling and logging symmetry

These should not block the three primary items above.

## Implementation order

Implement in this order:

1. async worker failure visibility
2. JS stack demangling parity
3. API-key semantic alignment
4. minor drift cleanup

This order is deliberate. Item 1 improves diagnosability for every later change. Item 2 is a pure behavior gap. Item 3 may require a policy decision. Item 4 is low risk and should be done only after the higher-value gaps are closed.

## Phase 0: Lock the current baseline

### Goals

- preserve the current passing ingest behavior
- add focused parity tests before altering behavior further

### Files

- `internal/ingest/otlpreceiver/http_test.go`
- `internal/ingest/otlpreceiver/store_pipeline_test.go`
- `internal/ingest/otlpreceiver/services_test.go`

### Tasks

1. Add a regression test that verifies RUM requests returning `200` actually produce persisted `hyperdx_sessions` rows under the real pipeline path.
2. Add a regression test around the async queue path so worker-side failures are currently reproducible in test code.
3. Add a test fixture for a browser error stack and breadcrumb console stack that should be remapped once Go source-map support exists.
4. Add an auth-behavior test matrix for `/v1/rum` covering:
   - no API key configured
   - static API key configured
   - managed CI key configured
   - `SOBS_ENFORCE_API_AUTH` enabled and disabled

### Acceptance criteria

- tests clearly document current behavior before refactors begin
- failures localize the gap to queueing, demangling, or auth behavior

## Phase 1: Make async RUM write failures visible

### Goals

- stop losing worker errors silently
- make queue-backed ingest failures observable during development and operations

### Files

- `internal/ingest/otlpreceiver/log_queue.go`
- `internal/ingest/otlpreceiver/http.go`
- `internal/ingest/otlpreceiver/store_pipeline.go`
- optional: `internal/ingest/otlpreceiver/services.go`

### Tasks

1. Change the queued request model so worker execution can record failure instead of dropping it.
2. Add one of these mechanisms:
   - a test-mode synchronous wait path matching Python behavior
   - a worker error sink or last-error recorder for diagnostics
   - structured logging for worker failures with route and record type context
3. Ensure RUM, AI, and `/v1/errors` use the same queue error visibility pattern.
4. Update HTTP tests so queue-full and worker-failure scenarios are distinct and asserted separately.

### Recommended implementation

Prefer a small, explicit queued task result model:

- add optional `done chan error` to queued requests
- allow callers or tests to request completion signaling
- have `runBatch(...)` send the actual execution error back when requested
- log all worker errors even when the caller is not waiting

This preserves async throughput while eliminating silent failure.

### Acceptance criteria

- a worker-side insert failure is visible in tests
- a worker-side insert failure is logged in non-test async execution
- queue-full remains a separate `503` path
- no regression to current successful RUM persistence

## Phase 2: Implement JS stack demangling parity

### Goals

- make Go stack processing match Python for browser RUM errors
- support mapped stack frames in both the main event stack and breadcrumb console stacks

### Files

- `internal/ingest/otlpreceiver/rum_ingest.go`
- new helper file under `internal/ingest/otlpreceiver/` if needed
- `internal/features/rum/` if shared source-map behavior belongs there

### Python behavior to match

Python currently provides:

- source-map enable flag
- source-map directory lookup
- file candidate resolution for `.map` files
- cached source-map loading
- line and column remapping
- `[mapped]` frame formatting in output stacks

### Tasks

1. Decide whether Go should implement the same source-map resolution rules directly or centralize them behind a small helper package.
2. Mirror Python's environment-controlled enablement behavior.
3. Apply remapping in both locations already used by Go:
   - event `stack`
   - breadcrumb console `stack`
4. Add test fixtures for:
   - unmapped stack remains unchanged
   - mapped stack includes `[mapped]`
   - breadcrumb console stack remapping
   - missing source map does not fail ingest

### Acceptance criteria

- Go emits mapped stack text for the same fixture that Python remaps
- missing or invalid source maps do not block ingest
- console breadcrumb stacks are remapped, not just the main error stack

## Phase 3: Align API-key enforcement semantics

### Goals

- remove ambiguity between route-level Python behavior and Go's global security switch
- ensure `/v1/rum` protection semantics are an intentional match, not an accidental near-match

### Files

- `internal/web/middleware_security.go`
- `internal/web/server.go`
- `internal/ingest/otlpreceiver/http.go`
- `internal/web/middleware_security_permissions_test.go`
- `internal/ingest/otlpreceiver/http_test.go`
- `app.py` for parity reference only

### Decision required

Pick one model and implement it consistently:

1. strict Python parity
   - enforce based on configured keys for `/v1/rum` and other protected `/v1/*` routes without requiring a separate global toggle
2. deliberate Go divergence
   - keep `SOBS_ENFORCE_API_AUTH` as the controlling switch and document the difference clearly

### Recommended direction

Choose strict Python parity unless there is an explicit product reason to keep Go's extra gate. The current split is easy to misunderstand and creates environment-specific behavior differences between runtimes.

### Tasks for strict parity

1. Refactor `allowV1APIKey(...)` so protected `/v1/*` routes enforce when a static or managed key is configured, even if `SOBS_ENFORCE_API_AUTH` is unset.
2. Retain `SOBS_ENFORCE_API_AUTH` only if it has a separate, clearly defined meaning after the refactor.
3. Verify `/v1/rum`, `/v1/ai`, `/v1/errors`, `/v1/logs`, `/v1/traces`, `/v1/metrics`, `/v1/rum/assets`, and `/v1/rum/client-token` all behave consistently.
4. Add explicit parity tests for configured and unconfigured key scenarios.

### Acceptance criteria

- `/v1/rum` auth behavior is documented and test-covered
- Go and Python agree for configured-key and no-key scenarios
- there is no hidden dependency on whether the server was launched with a separate enforcement toggle

## Phase 4: Clean up minor drift

### Goals

- remove low-risk behavioral mismatches after the primary gaps are closed

### Files

- `internal/ingest/otlpreceiver/store_pipeline.go`
- `internal/ingest/otlpreceiver/rum_ingest.go`
- `internal/ingest/otlpreceiver/store_pipeline_test.go`

### Tasks

1. Decide whether Go should preserve explicit empty `service` values like Python or continue normalizing them to `browser`.
2. Decide whether Go should preserve explicit empty `errorType` values like Python or continue normalizing them to `JSError`.
3. Decide whether tag-rule failures should be logged like Python instead of being ignored.

### Acceptance criteria

- all remaining drifts are either closed or explicitly documented as intentional divergence

## Validation plan

Run this validation after each phase:

1. `go test ./internal/ingest/otlpreceiver ./internal/web`
2. targeted RUM parity tests added in Phase 0
3. live verification using the existing demo flow:
   - `SOBS_RUNTIME=go ./scripts/start_ollama_ai_test.sh`
   - generate browser actions against the demo app
   - verify `hyperdx_sessions` rows exist
   - verify `/rum?view=events` shows the generated events
4. for Phase 2, verify mapped stack frames render from source-map fixtures
5. for Phase 3, verify unauthorized and authorized `/v1/rum` requests under both configured and unconfigured key scenarios

## Deliverables

Phase completion should produce:

1. code changes
2. regression tests
3. a short update to `docs/GO_PYTHON_PARITY_AUDIT_2026-04-20.md` or a linked follow-up note documenting the closed gap

## Recommended execution split

Use four implementation PR slices even if they land on the same branch:

1. queue failure visibility
2. source-map stack demangling
3. auth semantic alignment
4. minor drift cleanup and documentation

This keeps review scope small and makes parity regressions easier to isolate.