# POC: Add Optional Self-Telemetry Instrumentation to Sobs

## Objective

Add optional OpenTelemetry-based self-telemetry to Sobs so we can measure hot paths before attempting optimization work.

This issue should add a lightweight, no-op-by-default instrumentation layer that records useful traces and basic metrics when explicitly enabled. The goal is to understand where Sobs spends CPU, memory, and wall-clock time during ingest, normalization, storage, querying, and dashboard/API request handling.

## Background

The previous Go conversion POC did not preserve existing functionality well enough. Before attempting Nuitka, PyO3/Rust, msgspec, chDB pushdown, or Python free-threaded work, we need baseline measurements from the existing Python implementation.

This issue should add the initial telemetry foundation only. Do not optimize anything yet.

OpenTelemetry should be used because it supports Python manual instrumentation and SDK-based configuration, while Flask instrumentation can automatically trace Flask web requests and add route-aware span information.

## Design Principles

- Telemetry must be optional.
- If telemetry is not configured, it must behave as a no-op.
- Do not require an external OpenTelemetry collector for normal development.
- Do not break local startup if telemetry packages, endpoints, or exporters are missing.
- Do not record raw event payloads, log bodies, user data, secrets, headers, tokens, or PII.
- Prefer small helper functions, decorators, or context managers so future hot paths can be instrumented consistently.
- Keep the implementation compatible with the current Flask/Python architecture.
- Avoid dashboard/UI work in this issue.

## Non-Goals

- Do not build a polished self-observability dashboard.
- Do not add a new required backend service.
- Do not rewrite ingest, query, dashboard, or storage logic.
- Do not change public API behavior.
- Do not add optimization changes yet.
- Do not instrument every function in the codebase.

## Configuration Requirements

Add optional configuration using environment variables.

Suggested variables:

```env
SOBS_TELEMETRY_ENABLED=false
SOBS_TELEMETRY_SERVICE_NAME=sobs
SOBS_TELEMETRY_ENVIRONMENT=local
SOBS_TELEMETRY_EXPORTER=none
SOBS_TELEMETRY_OTLP_ENDPOINT=
SOBS_TELEMETRY_CONSOLE_EXPORT=false
SOBS_TELEMETRY_SAMPLE_RATE=1.0
```

Also respect the standard OpenTelemetry variable:

```env
OTEL_SDK_DISABLED=true
```

Expected behavior:

- If `SOBS_TELEMETRY_ENABLED` is not `true`, telemetry should be no-op.
- If `OTEL_SDK_DISABLED=true`, telemetry should be no-op even if Sobs telemetry is enabled.
- If `SOBS_TELEMETRY_EXPORTER=none`, use no-op behavior.
- If `SOBS_TELEMETRY_EXPORTER=console`, emit traces/metrics to console for local debugging.
- If `SOBS_TELEMETRY_EXPORTER=otlp`, use OTLP export only when `SOBS_TELEMETRY_OTLP_ENDPOINT` is set.
- Missing or invalid telemetry config should log a warning and continue app startup without telemetry.

## Dependency Requirements

Add OpenTelemetry dependencies in the appropriate project dependency file.

Likely dependencies:

```text
opentelemetry-api
opentelemetry-sdk
opentelemetry-instrumentation-flask
opentelemetry-exporter-otlp
```

If the project has optional dependency groups, put these under an optional telemetry/dev extras group if practical.

Example:

```bash
pip install -e ".[telemetry]"
```

Do not make telemetry dependencies required for the core runtime unless the current project structure makes optional extras impractical.

## Implementation Requirements

Create a small telemetry module. Suggested structure:

```text
sobs/telemetry/
  __init__.py
  config.py
  setup.py
  spans.py
  metrics.py
```

The exact path can be adjusted to match the existing repo structure.

### Required Helper Behavior

Provide helper functions similar to:

```python
def telemetry_enabled() -> bool:
    ...


def configure_telemetry(app=None) -> None:
    ...


def get_tracer(name: str = "sobs"):
    ...


def get_meter(name: str = "sobs"):
    ...


@contextmanager
def span(name: str, **attributes):
    ...
```

The `span(...)` helper must be safe to call even when telemetry is disabled.

When disabled, calls like this should not fail:

```python
with span("sobs.ingest.normalize", event_type="rum"):
    normalize_event(event)
```

## Flask Instrumentation

When telemetry is enabled, instrument Flask request handling.

Requirements:

- Instrument the Flask app during app startup or app factory initialization.
- Ensure request tracing does not record sensitive headers or bodies.
- Exclude health/static/internal noise routes if practical.

Suggested excluded routes:

```text
/health
/healthz
/static/.*
/favicon.ico
```

If the project has existing health endpoints, use those.

## Manual Spans to Add

Add manual spans around a small number of high-value hot paths. Use actual function names/routes in the repo.

At minimum, instrument these logical areas if they exist.

### 1. Ingest Request Handling

Span name:

```text
sobs.ingest.request
```

Suggested attributes:

```text
event.type
event.count
payload.bytes
route
```

Do not include raw payload content.

### 2. Event Parsing / Validation

Span name:

```text
sobs.ingest.parse
```

Suggested attributes:

```text
event.type
event.count
parser
```

### 3. Event Normalization

Span name:

```text
sobs.ingest.normalize
```

Suggested attributes:

```text
event.type
event.count
```

### 4. chDB Write / Persistence

Span name:

```text
sobs.storage.write
```

Suggested attributes:

```text
storage.engine=chdb
table
row.count
batch.size
```

### 5. chDB Query Execution

Span name:

```text
sobs.storage.query
```

Suggested attributes:

```text
storage.engine=chdb
query.name
```

Do not record raw SQL if it may include user-provided values. Prefer a stable query name.

### 6. Dashboard/API Data Assembly

Span name:

```text
sobs.dashboard.query
```

Suggested attributes:

```text
dashboard.name
route
```

### 7. Rule/Tag/SLA Evaluation, If Present

Span name:

```text
sobs.rules.evaluate
```

Suggested attributes:

```text
rule.count
event.count
```

## Metrics to Add

Add basic OpenTelemetry metrics if straightforward. If metrics are too invasive for this first pass, prioritize traces first and leave TODO comments for metrics.

Suggested metrics:

```text
sobs.ingest.events.total
sobs.ingest.batch.size
sobs.ingest.duration.ms
sobs.ingest.parse.duration.ms
sobs.ingest.normalize.duration.ms
sobs.storage.write.duration.ms
sobs.storage.query.duration.ms
sobs.dashboard.request.duration.ms
sobs.rules.evaluate.duration.ms
```

Also add process metrics if there is already a lightweight dependency available. If adding process metrics requires extra complexity, skip process metrics for this issue.

## Logging

Add startup logging that clearly states telemetry mode.

Examples:

```text
Sobs telemetry disabled; using no-op telemetry.
Sobs telemetry enabled with console exporter.
Sobs telemetry enabled with OTLP exporter.
Sobs telemetry configuration invalid; continuing with no-op telemetry.
```

Do not log secrets, tokens, full headers, raw payloads, or OTLP auth values.

## Local Development Documentation

Add a short documentation file:

```text
docs/telemetry.md
```

Include these examples.

### Disabled / Default

```bash
SOBS_TELEMETRY_ENABLED=false python -m sobs
```

### Explicit OpenTelemetry Disabled

```bash
OTEL_SDK_DISABLED=true SOBS_TELEMETRY_ENABLED=true python -m sobs
```

### Console Telemetry

```bash
SOBS_TELEMETRY_ENABLED=true \
SOBS_TELEMETRY_EXPORTER=console \
SOBS_TELEMETRY_CONSOLE_EXPORT=true \
python -m sobs
```

### OTLP Telemetry

```bash
SOBS_TELEMETRY_ENABLED=true \
SOBS_TELEMETRY_EXPORTER=otlp \
SOBS_TELEMETRY_OTLP_ENDPOINT=http://localhost:4317 \
python -m sobs
```

## Testing Requirements

Add or update tests to verify:

1. App starts with telemetry disabled.
2. App starts with `SOBS_TELEMETRY_ENABLED=false`.
3. App starts with `OTEL_SDK_DISABLED=true`.
4. `span(...)` helper is safe/no-op when telemetry is disabled.
5. Invalid exporter config does not crash app startup.
6. Flask instrumentation is only initialized when telemetry is enabled.
7. Manual instrumentation does not leak raw payloads into span attributes.

Where full OpenTelemetry assertions are cumbersome, use monkeypatching/mocking to verify setup paths and no-op behavior.

## Acceptance Criteria

- [ ] Telemetry is disabled by default.
- [ ] Disabled telemetry has no required external collector.
- [ ] Disabled telemetry does not crash and behaves as no-op.
- [ ] `OTEL_SDK_DISABLED=true` is respected.
- [ ] Flask request instrumentation is enabled only when configured.
- [ ] Manual spans are added around ingest, parse, normalize, storage write, storage query, and dashboard/query paths where those paths exist.
- [ ] No raw payloads, secrets, tokens, auth headers, or PII are emitted as telemetry attributes.
- [ ] `docs/telemetry.md` explains how to enable/disable telemetry.
- [ ] Tests cover enabled/disabled/no-op behavior.
- [ ] Existing tests continue to pass.

## Implementation Notes for Copilot

Start by locating the app startup/app factory code and the ingest/query/storage code paths.

Suggested approach:

1. Add telemetry config parsing.
2. Add a safe no-op telemetry helper module.
3. Wire telemetry setup into Flask app startup.
4. Add Flask instrumentation only when enabled.
5. Add manual spans around the highest-value hot paths.
6. Add tests.
7. Add short docs.

Keep the PR small and focused. Do not refactor unrelated code.

## Future Follow-Up Issues

After this lands, follow-up work will add:

- Benchmark/replay harness.
- CPU flamegraph profiling.
- Memory profiling.
- msgspec ingest parsing POC.
- chDB query pushdown POC.
- PyO3/Maturin native hot-path POC.
- Nuitka packaging benchmark.
