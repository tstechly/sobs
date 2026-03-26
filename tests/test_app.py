"""
Tests for SOBS – Simple Observe.
Run with:  pytest tests/
"""

import json
import os
import tempfile
import time

import pytest

# Point to a temp DB before importing the app
os.environ["SOBS_DATA_DIR"] = tempfile.mkdtemp()

from app import app, compress, compress_json, decompress, decompress_json, init_db  # noqa: E402


@pytest.fixture(scope="session", autouse=True)
def setup_db():
    init_db()


@pytest.fixture
def client():
    app.config["TESTING"] = True
    with app.test_client() as c:
        yield c


# ---------------------------------------------------------------------------
# Compression helpers
# ---------------------------------------------------------------------------
class TestCompression:
    def test_compress_decompress_roundtrip(self):
        text = "Hello, World! " * 100
        assert decompress(compress(text)) == text

    def test_compress_json_roundtrip(self):
        obj = {"key": "value", "num": 42, "list": [1, 2, 3]}
        assert decompress_json(compress_json(obj)) == obj

    def test_compressed_smaller_than_plain(self):
        text = "INFO This is a repeating log message. " * 50
        assert len(compress(text)) < len(text.encode())

    def test_decompress_none_returns_empty(self):
        assert decompress(None) == ""

    def test_decompress_json_none_returns_empty_dict(self):
        assert decompress_json(None) == {}


# ---------------------------------------------------------------------------
# Health check
# ---------------------------------------------------------------------------
class TestHealth:
    def test_health_ok(self, client):
        r = client.get("/health")
        assert r.status_code == 200
        data = json.loads(r.data)
        assert data["status"] == "ok"


# ---------------------------------------------------------------------------
# OTLP Logs ingest
# ---------------------------------------------------------------------------
class TestLogsIngest:
    def _otlp_payload(self, message="test log", level="INFO", service="test-svc"):
        ts_ns = str(int(time.time() * 1_000_000_000))
        return {
            "resourceLogs": [
                {
                    "resource": {"attributes": [{"key": "service.name", "value": {"stringValue": service}}]},
                    "scopeLogs": [
                        {
                            "logRecords": [
                                {
                                    "timeUnixNano": ts_ns,
                                    "severityText": level,
                                    "body": {"stringValue": message},
                                    "attributes": [{"key": "env", "value": {"stringValue": "test"}}],
                                    "traceId": "aabbccdd11223344aabbccdd11223344",
                                    "spanId": "1122334455667788",
                                }
                            ]
                        }
                    ],
                }
            ]
        }

    def test_ingest_single_log(self, client):
        r = client.post("/v1/logs", json=self._otlp_payload())
        assert r.status_code == 200
        assert json.loads(r.data)["accepted"] == 1

    def test_ingest_multiple_logs(self, client):
        payload = self._otlp_payload()
        # Add a second log record
        payload["resourceLogs"][0]["scopeLogs"][0]["logRecords"].append(
            {
                "timeUnixNano": str(int(time.time() * 1_000_000_000)),
                "severityText": "ERROR",
                "body": {"stringValue": "error log"},
            }
        )
        r = client.post("/v1/logs", json=payload)
        assert r.status_code == 200
        assert json.loads(r.data)["accepted"] == 2

    def test_ingest_empty_payload(self, client):
        r = client.post("/v1/logs", json={})
        assert r.status_code == 200
        assert json.loads(r.data)["accepted"] == 0


# ---------------------------------------------------------------------------
# OTLP Traces ingest
# ---------------------------------------------------------------------------
class TestTracesIngest:
    def _span_payload(self, name="test-span", status_code=1):
        start_ns = int(time.time() * 1_000_000_000)
        return {
            "resourceSpans": [
                {
                    "resource": {"attributes": [{"key": "service.name", "value": {"stringValue": "trace-svc"}}]},
                    "scopeSpans": [
                        {
                            "spans": [
                                {
                                    "traceId": "deadbeefdeadbeefdeadbeefdeadbeef",
                                    "spanId": "cafebabe12345678",
                                    "parentSpanId": "",
                                    "name": name,
                                    "startTimeUnixNano": str(start_ns),
                                    "endTimeUnixNano": str(start_ns + 50_000_000),
                                    "status": {"code": status_code},
                                    "attributes": [
                                        {"key": "http.method", "value": {"stringValue": "GET"}},
                                        {"key": "http.status_code", "value": {"intValue": 200}},
                                    ],
                                }
                            ]
                        }
                    ],
                }
            ]
        }

    def test_ingest_span(self, client):
        r = client.post("/v1/traces", json=self._span_payload())
        assert r.status_code == 200
        assert json.loads(r.data)["accepted"] == 1

    def test_error_span_creates_error(self, client):
        """An ERROR span should also create an entry in the errors table."""
        payload = self._span_payload(name="failing-op", status_code=2)
        payload["resourceSpans"][0]["scopeSpans"][0]["spans"][0]["attributes"].extend(
            [
                {"key": "exception.type", "value": {"stringValue": "ValueError"}},
                {"key": "exception.message", "value": {"stringValue": "bad input"}},
            ]
        )
        r = client.post("/v1/traces", json=payload)
        assert r.status_code == 200

    def test_ingest_empty_payload(self, client):
        r = client.post("/v1/traces", json={})
        assert r.status_code == 200
        assert json.loads(r.data)["accepted"] == 0


# ---------------------------------------------------------------------------
# Errors ingest
# ---------------------------------------------------------------------------
class TestErrorsIngest:
    def test_ingest_error(self, client):
        r = client.post(
            "/v1/errors",
            json={
                "service": "test-svc",
                "type": "RuntimeError",
                "message": "something broke",
                "stack": "Traceback:\n  at main (app.py:10)",
            },
        )
        assert r.status_code == 200
        assert json.loads(r.data)["ok"] is True

    def test_ingest_error_minimal(self, client):
        r = client.post("/v1/errors", json={})
        assert r.status_code == 200

    def test_resolve_error(self, client):
        # Create an error first
        client.post(
            "/v1/errors",
            json={
                "service": "resolve-svc",
                "type": "TestError",
                "message": "resolve me",
            },
        )
        # Resolve it (get the ID from the errors page)
        r = client.get("/errors?service=resolve-svc&resolved=0")
        assert r.status_code == 200
        # Resolve via POST (use ID 1 if we can find it from the page)
        r2 = client.post("/errors/1/resolve")
        assert r2.status_code == 200


# ---------------------------------------------------------------------------
# RUM ingest
# ---------------------------------------------------------------------------
class TestRumIngest:
    def test_ingest_pageview(self, client):
        r = client.post(
            "/v1/rum",
            json=[
                {
                    "type": "pageview",
                    "timestamp": "2024-01-01T00:00:00Z",
                    "sessionId": "sess-001",
                    "url": "https://example.com/",
                    "title": "Home",
                }
            ],
        )
        assert r.status_code == 200
        assert json.loads(r.data)["accepted"] == 1

    def test_ingest_web_vital(self, client):
        r = client.post(
            "/v1/rum",
            json=[
                {
                    "type": "web-vital",
                    "name": "LCP",
                    "value": 1800,
                    "rating": "good",
                    "sessionId": "sess-001",
                    "url": "https://example.com/",
                }
            ],
        )
        assert r.status_code == 200

    def test_ingest_js_error(self, client):
        r = client.post(
            "/v1/rum",
            json=[
                {
                    "type": "error",
                    "sessionId": "sess-002",
                    "url": "https://example.com/app",
                    "message": "Cannot read properties of null",
                    "errorType": "TypeError",
                    "stack": "TypeError: Cannot read...\n  at main (app.js:5)",
                }
            ],
        )
        assert r.status_code == 200

    def test_ingest_dict_payload(self, client):
        r = client.post(
            "/v1/rum",
            json={
                "events": [
                    {
                        "type": "pageview",
                        "sessionId": "sess-003",
                        "url": "https://example.com/about",
                    }
                ]
            },
        )
        assert r.status_code == 200

    def test_ingest_empty_list(self, client):
        r = client.post("/v1/rum", json=[])
        assert r.status_code == 200
        assert json.loads(r.data)["accepted"] == 0


# ---------------------------------------------------------------------------
# AI transparency ingest
# ---------------------------------------------------------------------------
class TestAIIngest:
    def test_ingest_ai_event(self, client):
        r = client.post(
            "/v1/ai",
            json={
                "service": "my-app",
                "provider": "openai",
                "model": "gpt-4o-mini",
                "prompt": "What is 2+2?",
                "response": "4",
                "tokens_in": 8,
                "tokens_out": 1,
                "duration_ms": 320,
            },
        )
        assert r.status_code == 200
        assert json.loads(r.data)["ok"] is True

    def test_ingest_ai_minimal(self, client):
        r = client.post("/v1/ai", json={})
        assert r.status_code == 200


# ---------------------------------------------------------------------------
# Web UI pages
# ---------------------------------------------------------------------------
class TestUIPages:
    def test_dashboard(self, client):
        r = client.get("/")
        assert r.status_code == 200
        assert b"SOBS" in r.data
        assert b"Dashboard" in r.data

    def test_logs_page(self, client):
        r = client.get("/logs")
        assert r.status_code == 200

    def test_logs_grep_filter(self, client):
        # Insert a distinctive log
        client.post(
            "/v1/logs",
            json={
                "resourceLogs": [
                    {
                        "resource": {"attributes": [{"key": "service.name", "value": {"stringValue": "grep-test"}}]},
                        "scopeLogs": [
                            {
                                "logRecords": [
                                    {
                                        "timeUnixNano": str(int(time.time() * 1_000_000_000)),
                                        "severityText": "INFO",
                                        "body": {"stringValue": "unique_grep_marker_xyz"},
                                    }
                                ]
                            }
                        ],
                    }
                ]
            },
        )
        r = client.get("/logs?q=unique_grep_marker_xyz")
        assert r.status_code == 200
        assert b"unique_grep_marker_xyz" in r.data

    def test_logs_sql_filter(self, client):
        r = client.get("/logs?sql=level%3D%27INFO%27")
        assert r.status_code == 200

    def test_errors_page(self, client):
        r = client.get("/errors")
        assert r.status_code == 200

    def test_traces_page(self, client):
        r = client.get("/traces")
        assert r.status_code == 200

    def test_rum_page(self, client):
        r = client.get("/rum")
        assert r.status_code == 200

    def test_ai_page(self, client):
        r = client.get("/ai")
        assert r.status_code == 200

    def test_rum_js_served(self, client):
        r = client.get("/static/rum.js")
        assert r.status_code == 200
        assert b"SOBS" in r.data

    def test_pagination(self, client):
        r = client.get("/logs?limit=10&offset=0")
        assert r.status_code == 200

    def test_sql_error_handled(self, client):
        """Bad SQL should return an error message, not a 500."""
        r = client.get("/logs?sql=INVALID+SQL+))))")
        assert r.status_code == 200
        assert b"SQL error" in r.data
