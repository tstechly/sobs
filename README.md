# SOBS – Simple Observe Stack

**SOBS** is a lightweight, single-user OpenTelemetry-compatible telemetry container focused on simplicity and transparency. It collects **Logs**, **Errors**, **Traces**, **RUM** (Real User Monitoring), and **AI call transparency** — all in one tiny container you can run as a standalone pod or sidecar.

![Dashboard](https://github.com/user-attachments/assets/fab68924-3526-49a9-9c03-d3f994bca3dd)

## Features

- 📦 **Tiny** – single Python service + embedded chDB, ~256 MB RAM target
- 🗜️ **Compressed storage** – MergeTree schema uses ZSTD with selective Delta/T64 codecs
- 🔭 **OpenTelemetry** – accepts OTLP (JSON and protobuf) for logs, traces, metrics
- 🌐 **RUM** – client-side JS snippet with Web Vitals (LCP, CLS, INP, TTFB, FCP)
- 🐛 **Error tracking** – with stack traces and one-click resolve
- 🤖 **AI transparency** – record LLM prompts, responses and token usage
- 🔍 **Search** – grep (regex) and SQL WHERE clause filtering on logs
- 🎨 **Bootstrap 5 dark UI** – served locally, no CDN required
- 🐳 **Docker ready** – Dockerfile + docker-compose + Kubernetes manifests

## Quick Start

```bash
# Docker
docker run -p 4317:4317 -v sobs_data:/data ghcr.io/abartrim/sobs:latest

# docker-compose
docker-compose up -d

# Python (dev)
pip install -r requirements.txt
python app.py
```

Note: `python app.py` runs Hypercorn with a Quart ASGI app in single-process mode.

Open `http://localhost:4317` in your browser.

Prebuilt image published by CI:

`ghcr.io/abartrim/sobs:latest`

## Runtime Modes

- Local and production process manager:
  - `python app.py` starts Hypercorn.
  - With embedded chDB, keep a single process by default.
  - Equivalent explicit command:

```bash
hypercorn --workers 1 --bind 0.0.0.0:${PORT:-4317} app:app
```

Why: embedded chDB is process-sensitive. Multiple process workers can trigger DB lock/stall behavior in embedded mode.

## Sending Data

### Python – OpenTelemetry SDK

```bash
pip install opentelemetry-sdk opentelemetry-exporter-otlp-proto-http
python examples/python/otel_example.py
```

### Flask auto-instrumentation

```bash
pip install opentelemetry-instrumentation-flask opentelemetry-exporter-otlp-proto-http
python examples/python/flask_example.py
```

### Node.js / Express

```bash
cd examples/nodejs && npm install && node example.js
```

### curl (no SDK)

```bash
bash examples/curl_examples.sh
```

### Client-side RUM

```html
<script src="http://YOUR_SOBS_HOST/static/rum.js"></script>
<script>
  SOBS.init({ endpoint: 'http://YOUR_SOBS_HOST/v1/rum', appName: 'my-app' });
</script>
```

## OTLP Endpoints

| Endpoint       | Method | Description                        |
|----------------|--------|------------------------------------|
| `/v1/logs`     | POST   | OTLP/JSON logs                     |
| `/v1/traces`   | POST   | OTLP/JSON traces                   |
| `/v1/metrics`  | POST   | OTLP/JSON metrics (stored as logs) |
| `/v1/rum`      | POST   | RUM events (JSON array)            |
| `/v1/errors`   | POST   | Direct error submission            |
| `/v1/ai`       | POST   | AI/LLM call transparency           |
| `/health`      | GET    | Liveness check                     |
| `/health/db`   | GET    | DB readiness check (touches chDB) |

Ingest writes are queued and flushed by a single background DB writer thread.

- Default runtime behavior: ingest endpoints acknowledge once the write is queued.
- Test behavior (`app.config["TESTING"] = True`): writes wait for batch completion so tests assert committed state deterministically.
- If the queue is saturated, ingest returns `503` so clients can retry/backoff.

This model favors client latency under burst traffic. It does not guarantee synchronous commit-per-request in normal runtime.

Fresh chDB databases are created with schema compression tuned using ZSTD plus selective Delta/T64 codecs. Embedded chDB via the Python API does not currently expose ClickHouse `storage_configuration`/encrypted-disk setup reliably, so encrypted-disk storage should be handled by an external ClickHouse server if required.

Use `/health/db` for readiness checks in orchestrated deployments when you need the probe to exercise DB availability as well as process liveness.

## Configuration

| Variable                    | Default        | Description                                      |
|-----------------------------|----------------|--------------------------------------------------|
| `SOBS_DATA_DIR`             | `./data`       | Directory for embedded chDB state                |
| `SOBS_API_KEY`              | _(empty)_      | Optional auth key for ingest endpoints           |
| `SOBS_BASIC_AUTH_USERNAME`  | _(empty)_      | Optional Basic Auth username for the Web UI      |
| `SOBS_BASIC_AUTH_PASSWORD`  | _(empty)_      | Optional Basic Auth password for the Web UI      |
| `SOBS_EXTERNAL_AUTH_URL`    | _(empty)_      | Optional external Bearer validator for the Web UI |
| `SOBS_BASE_PATH`            | _(empty)_      | Optional URL prefix (for example `/sobs`) for UI/API routing and generated links |
| `SOBS_SECRET_KEY`           | `sobs-dev-secret-key` | Secret key used by Quart session handling (set explicitly in production) |
| `PORT`                      | `4317`         | Listen port                                      |
| `SOBS_WRITE_QUEUE_MAX`      | `5000`         | Max buffered write operations before ingest returns `503` |
| `SOBS_WRITE_BATCH_MAX`      | `200`          | Max writes processed per DB batch |
| `SOBS_WRITE_BATCH_WAIT_MS`  | `20`           | Max milliseconds to wait for filling a write batch |
| `HYPERCORN_WORKERS`         | `1`            | Hypercorn worker process count (forced to 1 for embedded chDB safety) |
| `HYPERCORN_BIND`            | `0.0.0.0:$PORT` | Hypercorn bind address override |

Authentication details and setup examples are documented in [AUTHENTICATION.md](AUTHENTICATION.md).

The Web UI supports exactly one mode at a time:

- no auth
- basic auth (requires both `SOBS_BASIC_AUTH_USERNAME` and `SOBS_BASIC_AUTH_PASSWORD`)
- external bearer validation (`SOBS_EXTERNAL_AUTH_URL`)

Ingest API endpoints (`/v1/*`) use the separate `SOBS_API_KEY` mechanism.

For reverse proxies, SOBS also honors `X-Forwarded-Prefix` for URL generation and prefixed routing.

## Kubernetes

Deploy as a standalone pod:

```bash
kubectl apply -f k8s/deployment.yaml
```

Or as a **sidecar** – see `k8s/sidecar.yaml` for instructions.

## Screenshots

| Logs (grep + SQL search) | AI Transparency |
|---|---|
| ![Logs](https://github.com/user-attachments/assets/f6eb544c-11c0-4836-a337-864a46e13e29) | ![AI](https://github.com/user-attachments/assets/caf5a401-d86b-4bea-b6db-28ef983879ba) |

## Running Tests

```bash
pip install -r requirements.txt -r requirements-integration.txt
pytest tests/
```

## Running Benchmarks

```bash
# Start SOBS first (for example: python app.py)
./scripts/benchmark.sh

# Or target a custom endpoint
./scripts/benchmark.sh http://127.0.0.1:44318
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for local development setup and quality checks.

## Git Pre-Commit Hook (Python)

This repository includes a version-controlled Git pre-commit hook at `.githooks/pre-commit`.

It runs on staged Python files and performs:

- `isort`
- `black`
- `flake8`
- `mypy`

Enable it once per clone:

```bash
git config core.hooksPath .githooks
chmod +x .githooks/pre-commit
```

If formatting changes are applied, the hook re-stages those Python files before commit.

