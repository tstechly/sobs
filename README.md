# SOBS – Simple Observe

**SOBS** is a lightweight, single-user OpenTelemetry-compatible telemetry container focused on simplicity and transparency. It collects **Logs**, **Errors**, **Traces**, **RUM** (Real User Monitoring), and **AI call transparency** — all in one tiny container you can run as a standalone pod or sidecar.

![Dashboard](https://github.com/user-attachments/assets/fab68924-3526-49a9-9c03-d3f994bca3dd)

## Features

- 📦 **Tiny** – single Python file + SQLite, ~256 MB RAM limit
- 🗜️ **Compressed storage** – all log/trace bodies stored with zlib (level-9)
- 🔭 **OpenTelemetry** – accepts OTLP/JSON for logs, traces, metrics
- 🌐 **RUM** – client-side JS snippet with Web Vitals (LCP, CLS, INP, TTFB, FCP)
- 🐛 **Error tracking** – with stack traces and one-click resolve
- 🤖 **AI transparency** – record LLM prompts, responses and token usage
- 🔍 **Search** – grep (regex) and SQL WHERE clause filtering on logs
- 🎨 **Bootstrap 5 dark UI** – served locally, no CDN required
- 🐳 **Docker ready** – Dockerfile + docker-compose + Kubernetes manifests

## Quick Start

```bash
# Docker
docker run -p 4317:4317 -v sobs_data:/data sobs:latest

# docker-compose
docker-compose up -d

# Python (dev)
pip install flask
python app.py
```

Open `http://localhost:4317` in your browser.

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
| `/health`      | GET    | Health check                       |

## Configuration

| Variable                    | Default        | Description                                      |
|-----------------------------|----------------|--------------------------------------------------|
| `SOBS_DATA_DIR`             | `./data`       | Directory for the SQLite DB                      |
| `SOBS_API_KEY`              | _(empty)_      | Optional auth key for ingest endpoints           |
| `SOBS_BASIC_AUTH_USERNAME`  | _(empty)_      | Optional Basic Auth username for the Web UI      |
| `SOBS_BASIC_AUTH_PASSWORD`  | _(empty)_      | Optional Basic Auth password for the Web UI      |
| `SOBS_EXTERNAL_AUTH_URL`    | _(empty)_      | Optional external Bearer validator for the Web UI |
| `PORT`                      | `4317`         | Listen port                                      |
| `FLASK_DEBUG`               | `0`            | Enable Flask debug mode                          |

Authentication details and setup examples are documented in [AUTHENTICATION.md](AUTHENTICATION.md).

The Web UI supports exactly one mode at a time:

- no auth
- basic auth (requires both `SOBS_BASIC_AUTH_USERNAME` and `SOBS_BASIC_AUTH_PASSWORD`)
- external bearer validation (`SOBS_EXTERNAL_AUTH_URL`)

Ingest API endpoints (`/v1/*`) use the separate `SOBS_API_KEY` mechanism.

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
pip install flask pytest
pytest tests/
```

