"""
Tests for SOBS – Simple Observe.
Run with:  pytest tests/
"""

import base64
import json
import os
import sqlite3
import tempfile
import time

import pytest

# Point to a temp DB before importing the app
os.environ["SOBS_DATA_DIR"] = tempfile.mkdtemp()

import app as sobs_app  # noqa: E402
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


class TestDbBootstrap:
    def test_first_dashboard_and_rum_request_bootstrap_schema(self, monkeypatch, tmp_path):
        db_path = tmp_path / "fresh-sobs.db"
        monkeypatch.setattr(sobs_app, "DB_PATH", str(db_path))

        with app.test_client() as c:
            dashboard = c.get("/")
            assert dashboard.status_code == 200

            rum = c.post(
                "/v1/rum",
                json={
                    "session_id": "first-session",
                    "event_type": "pageview",
                    "url": "/",
                    "data": {"boot": True},
                },
            )
            assert rum.status_code == 200

        with sqlite3.connect(str(db_path)) as db:
            tables = {
                row[0]
                for row in db.execute(
                    "SELECT name FROM sqlite_master WHERE type='table' AND name IN ('logs', 'rum_events')"
                ).fetchall()
            }
        assert {"logs", "rum_events"}.issubset(tables)


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
# OTLP protobuf ingest
# ---------------------------------------------------------------------------
class TestOtlpProtobufIngest:
    """Verify that application/x-protobuf payloads are accepted and persisted."""

    PROTOBUF_CT = "application/x-protobuf"

    def _make_log_proto_bytes(self, message="proto log", level="INFO", service="proto-svc"):
        from opentelemetry.proto.collector.logs.v1.logs_service_pb2 import ExportLogsServiceRequest
        from opentelemetry.proto.common.v1.common_pb2 import AnyValue, KeyValue
        from opentelemetry.proto.logs.v1.logs_pb2 import LogRecord, ResourceLogs, ScopeLogs
        from opentelemetry.proto.resource.v1.resource_pb2 import Resource

        ts_ns = int(time.time() * 1_000_000_000)
        record = LogRecord(
            time_unix_nano=ts_ns,
            severity_text=level,
            body=AnyValue(string_value=message),
            attributes=[KeyValue(key="env", value=AnyValue(string_value="test"))],
            trace_id=bytes.fromhex("aabbccdd11223344aabbccdd11223344"),
            span_id=bytes.fromhex("1122334455667788"),
        )
        resource = Resource(
            attributes=[KeyValue(key="service.name", value=AnyValue(string_value=service))]
        )
        msg = ExportLogsServiceRequest(
            resource_logs=[ResourceLogs(resource=resource, scope_logs=[ScopeLogs(log_records=[record])])]
        )
        return msg.SerializeToString()

    def _make_trace_proto_bytes(self, name="proto-span", status_code=1, service="proto-trace-svc"):
        from opentelemetry.proto.collector.trace.v1.trace_service_pb2 import ExportTraceServiceRequest
        from opentelemetry.proto.common.v1.common_pb2 import AnyValue, KeyValue
        from opentelemetry.proto.resource.v1.resource_pb2 import Resource
        from opentelemetry.proto.trace.v1.trace_pb2 import ResourceSpans, ScopeSpans, Span, Status

        start_ns = int(time.time() * 1_000_000_000)
        span = Span(
            trace_id=bytes.fromhex("deadbeefdeadbeefdeadbeefdeadbeef"),
            span_id=bytes.fromhex("cafebabe12345678"),
            name=name,
            start_time_unix_nano=start_ns,
            end_time_unix_nano=start_ns + 50_000_000,
            status=Status(code=status_code),
            attributes=[KeyValue(key="http.method", value=AnyValue(string_value="GET"))],
        )
        resource = Resource(
            attributes=[KeyValue(key="service.name", value=AnyValue(string_value=service))]
        )
        msg = ExportTraceServiceRequest(
            resource_spans=[ResourceSpans(resource=resource, scope_spans=[ScopeSpans(spans=[span])])]
        )
        return msg.SerializeToString()

    def test_protobuf_log_ingest_accepted(self, client):
        body = self._make_log_proto_bytes()
        r = client.post("/v1/logs", data=body, content_type=self.PROTOBUF_CT)
        assert r.status_code == 200
        assert json.loads(r.data)["accepted"] == 1

    def test_protobuf_log_persisted_in_db(self, client):
        body = self._make_log_proto_bytes(message="hello protobuf", service="proto-db-svc")
        r = client.post("/v1/logs", data=body, content_type=self.PROTOBUF_CT)
        assert r.status_code == 200
        assert json.loads(r.data)["accepted"] == 1
        # Verify the row exists in the database
        conn = sqlite3.connect(sobs_app.DB_PATH)
        row = conn.execute("SELECT service FROM logs WHERE service=? ORDER BY rowid DESC LIMIT 1", ("proto-db-svc",)).fetchone()
        conn.close()
        assert row is not None, "Log row not found in DB"
        assert row[0] == "proto-db-svc"

    def test_protobuf_trace_ingest_accepted(self, client):
        body = self._make_trace_proto_bytes()
        r = client.post("/v1/traces", data=body, content_type=self.PROTOBUF_CT)
        assert r.status_code == 200
        assert json.loads(r.data)["accepted"] == 1

    def test_protobuf_trace_persisted_in_db(self, client):
        body = self._make_trace_proto_bytes(name="my-span", service="proto-trace-db-svc")
        r = client.post("/v1/traces", data=body, content_type=self.PROTOBUF_CT)
        assert r.status_code == 200
        assert json.loads(r.data)["accepted"] == 1
        # Verify the row exists in the database
        conn = sqlite3.connect(sobs_app.DB_PATH)
        row = conn.execute("SELECT name, service FROM spans WHERE service=? ORDER BY rowid DESC LIMIT 1", ("proto-trace-db-svc",)).fetchone()
        conn.close()
        assert row is not None, "Span row not found in DB"
        assert row[0] == "my-span"
        assert row[1] == "proto-trace-db-svc"

    def test_protobuf_error_span_creates_error(self, client):
        """An ERROR span sent via protobuf should also create an errors table entry."""
        from opentelemetry.proto.collector.trace.v1.trace_service_pb2 import ExportTraceServiceRequest
        from opentelemetry.proto.common.v1.common_pb2 import AnyValue, KeyValue
        from opentelemetry.proto.resource.v1.resource_pb2 import Resource
        from opentelemetry.proto.trace.v1.trace_pb2 import ResourceSpans, ScopeSpans, Span, Status

        start_ns = int(time.time() * 1_000_000_000)
        span = Span(
            trace_id=bytes.fromhex("deadbeefdeadbeefdeadbeefdeadbeef"),
            span_id=bytes.fromhex("cafebabe12345678"),
            name="failing-proto-op",
            start_time_unix_nano=start_ns,
            end_time_unix_nano=start_ns + 10_000_000,
            status=Status(code=2),  # STATUS_CODE_ERROR
            attributes=[
                KeyValue(key="exception.type", value=AnyValue(string_value="ValueError")),
                KeyValue(key="exception.message", value=AnyValue(string_value="proto bad input")),
            ],
        )
        resource = Resource(
            attributes=[KeyValue(key="service.name", value=AnyValue(string_value="proto-err-svc"))]
        )
        msg = ExportTraceServiceRequest(
            resource_spans=[ResourceSpans(resource=resource, scope_spans=[ScopeSpans(spans=[span])])]
        )
        r = client.post("/v1/traces", data=msg.SerializeToString(), content_type=self.PROTOBUF_CT)
        assert r.status_code == 200
        assert json.loads(r.data)["accepted"] == 1
        # Verify an errors row was created
        conn = sqlite3.connect(sobs_app.DB_PATH)
        row = conn.execute("SELECT service, err_type FROM errors WHERE service=? ORDER BY rowid DESC LIMIT 1", ("proto-err-svc",)).fetchone()
        conn.close()
        assert row is not None, "Error row not found in DB"
        assert row[1] == "ValueError"

    def test_protobuf_invalid_body_returns_400(self, client):
        r = client.post("/v1/logs", data=b"not valid protobuf", content_type=self.PROTOBUF_CT)
        assert r.status_code == 400
        assert "error" in json.loads(r.data)

    def test_protobuf_invalid_traces_body_returns_400(self, client):
        r = client.post("/v1/traces", data=b"\xff\xfe garbage", content_type=self.PROTOBUF_CT)
        assert r.status_code == 400
        assert "error" in json.loads(r.data)

    def test_json_ingest_still_works_alongside_protobuf(self, client):
        """Regression: JSON ingest path must remain functional."""
        ts_ns = str(int(time.time() * 1_000_000_000))
        payload = {
            "resourceLogs": [
                {
                    "resource": {"attributes": [{"key": "service.name", "value": {"stringValue": "json-svc"}}]},
                    "scopeLogs": [
                        {"logRecords": [{"timeUnixNano": ts_ns, "severityText": "INFO", "body": {"stringValue": "json ok"}}]}
                    ],
                }
            ]
        }
        r = client.post("/v1/logs", json=payload)
        assert r.status_code == 200
        assert json.loads(r.data)["accepted"] == 1
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

    def test_root_mode_uses_root_relative_links(self, client):
        """Default deployment should generate links/assets without a path prefix."""
        r = client.get("/")
        assert r.status_code == 200
        assert b'href="/logs"' in r.data
        assert b'href="/errors"' in r.data
        assert b'src="/static/bootstrap.bundle.min.js"' in r.data


class TestBasePathSupport:
    def test_prefixed_mode_routes_and_generates_prefixed_links(self, monkeypatch):
        """When SOBS base path is configured, both routing and generated links should honor it."""
        import app as app_module

        monkeypatch.setattr(app_module, "BASE_PATH", "/sobs")
        app.config["TESTING"] = True

        with app.test_client() as c:
            dashboard = c.get("/sobs/")
            assert dashboard.status_code == 200
            assert b'href="/sobs/logs"' in dashboard.data
            assert b'href="/sobs/errors"' in dashboard.data
            assert b'src="/sobs/static/bootstrap.bundle.min.js"' in dashboard.data

            logs_ingest = c.post("/sobs/v1/logs", json={})
            assert logs_ingest.status_code == 200

            rum_script = c.get("/sobs/static/rum.js")
            assert rum_script.status_code == 200

    def test_forwarded_prefix_generates_prefixed_links(self, client):
        """X-Forwarded-Prefix should influence generated links even when backend paths are unprefixed."""
        r = client.get("/", headers={"X-Forwarded-Prefix": "/sobs"})
        assert r.status_code == 200
        assert b'href="/sobs/logs"' in r.data
        assert b'href="/sobs/errors"' in r.data
        assert b'src="/sobs/static/bootstrap.bundle.min.js"' in r.data


# ---------------------------------------------------------------------------
# Basic Auth
# ---------------------------------------------------------------------------
class TestBasicAuth:
    """Tests for optional Basic Auth on Web UI routes."""

    _TEST_USER = "admin"
    _TEST_PASS = "secret"

    def _auth_header(self, username=None, password=None):
        u = username if username is not None else self._TEST_USER
        p = password if password is not None else self._TEST_PASS
        token = base64.b64encode(f"{u}:{p}".encode()).decode()
        return {"Authorization": f"Basic {token}"}

    @pytest.fixture
    def authed_client(self, monkeypatch):
        """Client with Basic Auth enabled via env vars."""
        import app as app_module

        monkeypatch.setattr(app_module, "BASIC_AUTH_USERNAME", self._TEST_USER)
        monkeypatch.setattr(app_module, "BASIC_AUTH_PASSWORD", self._TEST_PASS)
        app.config["TESTING"] = True
        with app.test_client() as c:
            yield c

    def test_ui_requires_auth_when_configured(self, authed_client):
        """Web UI should return 401 when Basic Auth is configured and no credentials sent."""
        r = authed_client.get("/")
        assert r.status_code == 401
        assert r.headers.get("WWW-Authenticate") == 'Basic realm="SOBS"'

    def test_ui_accessible_with_correct_credentials(self, authed_client):
        """Web UI should be accessible with correct Basic Auth credentials."""
        r = authed_client.get("/", headers=self._auth_header())
        assert r.status_code == 200

    def test_ui_rejects_wrong_password(self, authed_client):
        """Web UI should return 401 when password is wrong."""
        r = authed_client.get("/", headers=self._auth_header(password="wrong"))
        assert r.status_code == 401

    def test_ui_rejects_wrong_username(self, authed_client):
        """Web UI should return 401 when username is wrong."""
        r = authed_client.get("/", headers=self._auth_header(username="nobody"))
        assert r.status_code == 401

    def test_ui_no_auth_without_config(self, client):
        """Web UI should be freely accessible when Basic Auth is not configured."""
        r = client.get("/")
        assert r.status_code == 200

    def test_all_ui_routes_protected(self, authed_client):
        """All Web UI routes should require auth when Basic Auth is configured."""
        ui_routes = ["/", "/logs", "/errors", "/traces", "/rum", "/ai"]
        for route in ui_routes:
            r = authed_client.get(route)
            assert r.status_code == 401, f"Expected 401 for {route}, got {r.status_code}"

    def test_api_endpoints_unaffected(self, authed_client):
        """Ingest API endpoints (/v1/*) should not be gated by Basic Auth."""
        r = authed_client.post("/v1/logs", json={})
        assert r.status_code == 200

    def test_health_endpoint_unaffected(self, authed_client):
        """/health should remain accessible regardless of Basic Auth config."""
        r = authed_client.get("/health")
        assert r.status_code == 200

    def test_partial_basic_auth_config_is_error(self, monkeypatch):
        """Supplying only one Basic Auth credential should be treated as misconfiguration."""
        import app as app_module

        monkeypatch.setattr(app_module, "BASIC_AUTH_USERNAME", self._TEST_USER)
        monkeypatch.setattr(app_module, "BASIC_AUTH_PASSWORD", "")
        monkeypatch.setattr(app_module, "EXTERNAL_AUTH_URL", "")
        app.config["TESTING"] = True
        with app.test_client() as c:
            r = c.get("/")
        assert r.status_code == 500
        assert r.get_json() == {"error": "Server auth misconfiguration"}


# ---------------------------------------------------------------------------
# External Auth
# ---------------------------------------------------------------------------
class TestExternalAuth:
    """Tests for optional external auth handler on Web UI routes."""

    _EXT_AUTH_URL = "http://auth-service"

    @pytest.fixture
    def ext_auth_client(self, monkeypatch):
        """Client with external auth URL configured."""
        import app as app_module

        monkeypatch.setattr(app_module, "EXTERNAL_AUTH_URL", self._EXT_AUTH_URL)
        app.config["TESTING"] = True
        with app.test_client() as c:
            yield c

    def test_rejects_request_without_bearer_token(self, ext_auth_client):
        """Web UI should return 401 with Bearer challenge when external auth is configured and no token is sent."""
        r = ext_auth_client.get("/")
        assert r.status_code == 401
        assert r.headers.get("WWW-Authenticate") == 'Bearer realm="SOBS"'

    def test_allows_request_with_valid_bearer_token(self, ext_auth_client, monkeypatch):
        """Web UI should allow requests when external auth service approves the token."""
        import app as app_module

        monkeypatch.setattr(app_module, "_check_external_auth", lambda _auth: True)
        r = ext_auth_client.get("/", headers={"Authorization": "Bearer valid-token"})
        assert r.status_code == 200

    def test_rejects_request_with_invalid_bearer_token(self, ext_auth_client, monkeypatch):
        """Web UI should return 401 when external auth service rejects the token."""
        import app as app_module

        monkeypatch.setattr(app_module, "_check_external_auth", lambda _auth: False)
        r = ext_auth_client.get("/", headers={"Authorization": "Bearer bad-token"})
        assert r.status_code == 401

    def test_basic_and_external_together_is_error(self, monkeypatch):
        """Basic and external auth configured together should be treated as misconfiguration."""
        import app as app_module

        monkeypatch.setattr(app_module, "BASIC_AUTH_USERNAME", "admin")
        monkeypatch.setattr(app_module, "BASIC_AUTH_PASSWORD", "secret")
        monkeypatch.setattr(app_module, "EXTERNAL_AUTH_URL", self._EXT_AUTH_URL)
        app.config["TESTING"] = True
        with app.test_client() as c:
            r = c.get("/")
        assert r.status_code == 500
        assert r.get_json() == {"error": "Server auth misconfiguration"}

    def test_ingest_endpoints_unaffected_by_external_auth(self, ext_auth_client):
        """Ingest API endpoints (/v1/*) should not be gated by external auth."""
        r = ext_auth_client.post("/v1/logs", json={})
        assert r.status_code == 200

    def test_ui_no_auth_required_when_not_configured(self, client):
        """Web UI should be freely accessible when external auth is not configured."""
        r = client.get("/")
        assert r.status_code == 200

    def test_all_ui_routes_protected(self, ext_auth_client, monkeypatch):
        """All Web UI routes should require auth when external auth is configured."""
        import app as app_module

        monkeypatch.setattr(app_module, "_check_external_auth", lambda _auth: False)
        ui_routes = ["/", "/logs", "/errors", "/traces", "/rum", "/ai"]
        for route in ui_routes:
            r = ext_auth_client.get(route, headers={"Authorization": "Bearer bad-token"})
            assert r.status_code == 401, f"Expected 401 for {route}, got {r.status_code}"

    def test_check_external_auth_makes_correct_request(self, monkeypatch):
        """_check_external_auth should POST to /internal/auth/validate with the Authorization header."""
        import urllib.request

        import app as app_module

        monkeypatch.setattr(app_module, "EXTERNAL_AUTH_URL", self._EXT_AUTH_URL)

        captured = {}

        class _FakeResponse:
            status = 200

            def __enter__(self):
                return self

            def __exit__(self, *_):
                pass

        def fake_urlopen(req, timeout=None):
            captured["url"] = req.full_url
            captured["method"] = req.method
            captured["auth"] = req.get_header("Authorization")
            return _FakeResponse()

        monkeypatch.setattr(urllib.request, "urlopen", fake_urlopen)

        result = app_module._check_external_auth("Bearer my-token")

        assert result is True
        assert captured["url"] == self._EXT_AUTH_URL + "/internal/auth/validate"
        assert captured["method"] == "POST"
        assert captured["auth"] == "Bearer my-token"

    def test_check_external_auth_returns_false_on_non_200(self, monkeypatch):
        """_check_external_auth should return False when the external service returns non-200."""
        import urllib.request

        import app as app_module

        monkeypatch.setattr(app_module, "EXTERNAL_AUTH_URL", self._EXT_AUTH_URL)

        class _FakeResponse:
            status = 401

            def __enter__(self):
                return self

            def __exit__(self, *_):
                pass

        monkeypatch.setattr(urllib.request, "urlopen", lambda req, timeout=None: _FakeResponse())

        assert app_module._check_external_auth("Bearer bad-token") is False

    def test_check_external_auth_returns_false_on_network_error(self, monkeypatch):
        """_check_external_auth should return False when the external service is unreachable."""
        import urllib.request

        import app as app_module

        monkeypatch.setattr(app_module, "EXTERNAL_AUTH_URL", self._EXT_AUTH_URL)

        def _raise_network_error(req, timeout=None):
            raise OSError("unreachable")

        monkeypatch.setattr(urllib.request, "urlopen", _raise_network_error)

        assert app_module._check_external_auth("Bearer any-token") is False

    def test_check_external_auth_returns_false_when_url_not_configured(self):
        """_check_external_auth should return False immediately when EXTERNAL_AUTH_URL is empty."""
        import app as app_module

        # EXTERNAL_AUTH_URL is empty in the default test environment
        assert app_module._check_external_auth("Bearer token") is False

    def test_session_cookie_used_as_bearer_fallback_when_valid(self, ext_auth_client, monkeypatch):
        """When no Bearer header is present, a valid session cookie should be accepted via external auth."""
        import app as app_module

        monkeypatch.setattr(app_module, "_check_external_auth", lambda _auth: True)
        ext_auth_client.set_cookie("session", "valid-session-token")
        r = ext_auth_client.get("/")
        assert r.status_code == 200

    def test_session_cookie_denied_when_validator_rejects(self, ext_auth_client, monkeypatch):
        """When session cookie is present but the external validator rejects it, return 401."""
        import app as app_module

        monkeypatch.setattr(app_module, "_check_external_auth", lambda _auth: False)
        ext_auth_client.set_cookie("session", "invalid-session-token")
        r = ext_auth_client.get("/")
        assert r.status_code == 401

    def test_session_cookie_synthesizes_bearer_header(self, ext_auth_client, monkeypatch):
        """The session cookie value should be forwarded as a Bearer token to the external validator."""
        import app as app_module

        captured = {}

        def capturing_check(auth):
            captured["auth"] = auth
            return True

        monkeypatch.setattr(app_module, "_check_external_auth", capturing_check)
        ext_auth_client.set_cookie("session", "my-session-value")
        r = ext_auth_client.get("/")
        assert r.status_code == 200
        assert captured.get("auth") == "Bearer my-session-value"

    def test_no_bearer_no_cookie_returns_401_with_bearer_challenge(self, ext_auth_client):
        """Requests with neither Authorization header nor session cookie should get 401 + Bearer challenge."""
        r = ext_auth_client.get("/")
        assert r.status_code == 401
        assert r.headers.get("WWW-Authenticate") == 'Bearer realm="SOBS"'
