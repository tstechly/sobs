# SOBS Architecture Overview

> **Status:** Phase 1 (Foundation) complete — see
> [`docs/REFACTORING_IMPLEMENTATION_PLAN.md`](REFACTORING_IMPLEMENTATION_PLAN.md)
> for the full multi-phase roadmap.

## Module Organisation

```
sobs/
├── app.py                  # Quart app, route registration, request handlers
├── config.py               # Environment reading, encryption, runtime constants
├── masking.py              # PII/secret masking rules (shared filter)
├── mcp.py                  # MCP server Blueprint (Model Context Protocol)
│
├── shared/                 # Shared utilities — no SOBS-specific imports
│   ├── __init__.py         # Re-exports from sub-modules
│   ├── serialization.py    # zlib/base64 compression helpers
│   └── events.py           # OTEL signal dataclasses + attr fingerprinting
│
└── telemetry/              # Optional OpenTelemetry self-instrumentation
    ├── __init__.py
    ├── config.py
    ├── metrics.py
    ├── setup.py
    └── spans.py
```

## Module Responsibilities

### `app.py`
The main application file.  Contains:
- Quart `app` object creation and middleware setup
- All route handler functions (views, API endpoints, ingest handlers)
- Application-level background workers (write queue, anomaly detection, etc.)
- Jinja2 template globals and context processors

Import of config constants, shared utilities, and event types is done via
the extracted modules below.  All extracted symbols are **re-exported** from
`app.py` for full backwards compatibility.

### `config.py`
All environment-variable reading and runtime-constant derivation.  Provides:

| Export | Description |
|--------|-------------|
| `_env_flag(name, default)` | Parse boolean env var |
| `_normalize_base_path(value)` | Normalise URL path prefix |
| `_merge_script_name(script_name, base_path)` | Append path prefix once |
| `_read_env_or_file(env_var, file_env_var)` | Read value or fallback to file |
| `_read_file_or_env(env_var, file_env_var)` | Read file or fallback to env |
| `_encrypt_secret_value(value)` | Fernet-encrypt a settings value |
| `_decrypt_secret_value(value)` | Fernet-decrypt a settings value |
| `DATA_DIR`, `DB_PATH`, `RUM_ASSET_DIR` | Data storage paths |
| `API_KEY`, `BASIC_AUTH_USERNAME/PASSWORD` | Authentication config |
| `BASE_PATH`, `MOBILE_BREAKPOINT_MAX` | UI / routing config |
| `CHDB_*_ENV` | chDB tuning env-var name constants |

**No imports from `app.py`** — `config.py` depends only on stdlib and the
optional `cryptography` package.

### `shared/serialization.py`
Pure-Python zlib/base64 compression helpers.  Used to store large payloads
(SQL, JSON blobs, chart specs) inside chDB string columns.

| Function | Description |
|----------|-------------|
| `compress(text)` | Compress a string → base64-encoded ASCII |
| `decompress(data)` | Decompress base64 or bytes → string |
| `compress_json(obj)` | JSON-serialise then compress |
| `decompress_json(data)` | Decompress then JSON-deserialise |

**No SOBS-specific imports** — stdlib only (`base64`, `json`, `zlib`).

### `shared/events.py`
Typed dataclasses for normalised in-memory OTEL signals.

| Type | Description |
|------|-------------|
| `LogEvent` | A single OTEL log record |
| `SpanEvent` | A single OTEL trace span |
| `ErrorEvent` | An error extracted from a span or direct ingest |
| `MetricEvent` | Lightweight metric event (name + attrs) |
| `TypedMetricEvent` | Full metric data point with kind, value, histogram |
| `_attr_fingerprint(attrs)` | 16-hex cardinality-reduction fingerprint |
| `_FINGERPRINT_SKIP_PREFIXES` | Prefixes excluded from fingerprinting |

**No SOBS-specific imports** — stdlib only (`dataclasses`, `hashlib`).

### `masking.py`
Standalone PII/secret masking module.  Provides pattern-based and
key-name-based masking for log output, notification messages, and GitHub
issue bodies.  See the module docstring for the full extension guide.

### `mcp.py`
Quart Blueprint providing the MCP (Model Context Protocol) server.  Exposes
read-only tool endpoints for Copilot and other MCP-compatible clients.
Registered on the main `app` via `app.register_blueprint(_mcp.mcp_bp)`.

### `telemetry/`
Optional OpenTelemetry self-instrumentation.  Enabled via
`SOBS_TELEMETRY_ENABLED=true`.  All exports are no-ops when disabled.

---

## Dependency Graph

```
                    app.py
                      │
        ══════════════╬══════════════
        │             │              │
     config.py   masking.py      mcp.py
        │                            │
        └──────────────────┐         │
                           ▼         │
                        shared/      │
                  (serialization,    │
                     events)         │
                                     ▼
                              telemetry/
```

**Rules:**
- `config.py` ← stdlib only
- `shared/*` ← stdlib only
- `telemetry/` ← stdlib + opentelemetry SDK
- `mcp.py` ← quart (Blueprint), stdlib
- `app.py` ← everything above (no circular imports)

---

## Adding a New Feature

1. **Determine the domain** — ingest, logs, traces, metrics, errors, AI, etc.
2. **Add business logic** in `app.py` (current) or a future `features/` module
3. **Add shared utilities** (new dataclasses, parsers) to the appropriate
   `shared/` sub-module
4. **Add config constants** for new environment variables to `config.py`
5. **Write tests** in `tests/test_app.py` (integration) or a dedicated
   `tests/test_config.py` / `tests/test_shared.py` for pure-unit tests
6. **Run the test suite:**
   ```bash
   pytest tests/test_config.py tests/test_shared.py tests/test_telemetry.py -v
   ```

---

## Future Phases

See [`docs/REFACTORING_IMPLEMENTATION_PLAN.md`](REFACTORING_IMPLEMENTATION_PLAN.md)
for the full roadmap.  The planned next phases are:

| Phase | Goal | Key Modules |
|-------|------|-------------|
| **2** | Database layer | `database/connection.py`, `database/schema.py` |
| **3** | AI subsystem | `ai/llm.py`, `ai/guards.py`, `ai/memory.py` |
| **4** | Feature domains | `features/ingest.py`, `features/logs.py`, … |
| **5** | Agent framework | `agents/rules.py`, `agents/executor.py` |
