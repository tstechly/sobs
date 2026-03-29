"""
SOBS - Simple Observe
A lightweight, single-user telemetry container supporting OpenTelemetry,
RUM, Logs, Errors, Traces, and AI transparency.
"""

import ast
import asyncio
import base64
import hashlib
import inspect
import json
import logging
import os
import queue
import re
import secrets
import threading
import time
import urllib.error
import urllib.request
import zlib
from dataclasses import dataclass
from datetime import datetime, timezone
from functools import wraps
from typing import Callable

import chdb.dbapi as chdb_driver
from google.protobuf.json_format import ParseDict
from hypercorn.asyncio import serve as hypercorn_serve
from hypercorn.config import Config as HypercornConfig
from opentelemetry.proto.collector.logs.v1.logs_service_pb2 import ExportLogsServiceRequest
from opentelemetry.proto.collector.metrics.v1.metrics_service_pb2 import ExportMetricsServiceRequest
from opentelemetry.proto.collector.trace.v1.trace_service_pb2 import ExportTraceServiceRequest
from quart import (
    Quart,
    jsonify,
    render_template,
    request,
    send_from_directory,
)

# ---------------------------------------------------------------------------
# App setup
# ---------------------------------------------------------------------------
app = Quart(__name__)


def _normalize_base_path(value: str) -> str:
    """Normalize base path values to either '' or '/segment[/subsegment]'."""
    if not value:
        return ""
    normalized = re.sub(r"/+", "/", str(value).strip())
    if not normalized or normalized == "/":
        return ""
    if not normalized.startswith("/"):
        normalized = "/" + normalized
    normalized = normalized.rstrip("/")
    return normalized if normalized != "/" else ""


def _merge_script_name(script_name: str, base_path: str) -> str:
    """Append base path to SCRIPT_NAME once."""
    if not base_path:
        return script_name or ""
    current = script_name or ""
    if current.endswith(base_path):
        return current
    if not current:
        return base_path
    return current.rstrip("/") + base_path


BASE_PATH = _normalize_base_path(os.environ.get("SOBS_BASE_PATH", ""))
app.config["APPLICATION_ROOT"] = BASE_PATH or "/"
app.config["SECRET_KEY"] = os.environ.get("SOBS_SECRET_KEY", "sobs-dev-secret-key")


class BasePathMiddleware:
    """ASGI middleware for deployment behind a path prefix and proxy prefix headers."""

    def __init__(self, wrapped_app, configured_base_path: str):
        self.wrapped_app = wrapped_app
        self.configured_base_path = configured_base_path

    @staticmethod
    def _merge_root_path(root_path: str, base_path: str) -> str:
        if not base_path:
            return root_path or ""
        current = root_path or ""
        if current.endswith(base_path):
            return current
        if not current:
            return base_path
        return current.rstrip("/") + base_path

    @staticmethod
    def _header_value(scope, header_name: str) -> str:
        needle = header_name.lower().encode("latin-1")
        for key, value in scope.get("headers", []):
            if key.lower() == needle:
                return value.decode("latin-1")
        return ""

    async def __call__(self, scope, receive, send):
        if scope.get("type") not in ("http", "websocket"):
            return await self.wrapped_app(scope, receive, send)

        scope = dict(scope)
        forwarded = _normalize_base_path(self._header_value(scope, "x-forwarded-prefix"))
        effective_base = forwarded or BASE_PATH  # read module-level var so monkeypatch works in tests
        if effective_base:
            path_info = scope.get("path", "") or "/"
            root_path = scope.get("root_path", "")

            if path_info.startswith(effective_base + "/") or path_info == effective_base:
                # Prefix is present in PATH_INFO.
                # Set root_path and leave scope["path"] intact — Quart's ASGI handler
                # strips root_path from scope["path"] internally before routing.
                scope["root_path"] = self._merge_root_path(root_path, effective_base)
            else:
                # Proxy already stripped the prefix.  Re-prepend it so Quart can
                # strip correctly via root_path (and url_for generates prefixed links).
                scope["root_path"] = self._merge_root_path(root_path, effective_base)
                scope["path"] = effective_base + (path_info if path_info.startswith("/") else "/" + path_info)

        return await self.wrapped_app(scope, receive, send)


app.asgi_app = BasePathMiddleware(app.asgi_app, BASE_PATH)  # type: ignore[method-assign]

DATA_DIR = os.environ.get("SOBS_DATA_DIR", os.path.join(os.path.dirname(__file__), "data"))
DB_PATH = os.path.join(DATA_DIR, "sobs.chdb")
API_KEY = os.environ.get("SOBS_API_KEY", "")  # empty = no auth required
BASIC_AUTH_USERNAME = os.environ.get("SOBS_BASIC_AUTH_USERNAME", "")  # empty = no basic auth
BASIC_AUTH_PASSWORD = os.environ.get("SOBS_BASIC_AUTH_PASSWORD", "")
EXTERNAL_AUTH_URL = os.environ.get("SOBS_EXTERNAL_AUTH_URL", "")  # empty = disabled

os.makedirs(DATA_DIR, exist_ok=True)

logging.basicConfig(level=logging.INFO, format="%(levelname)s %(message)s")
log = logging.getLogger("sobs")

# ---------------------------------------------------------------------------
# Database helpers
# ---------------------------------------------------------------------------
SCHEMA = """
CREATE TABLE IF NOT EXISTS otel_logs (
    Timestamp DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    TimestampTime DateTime DEFAULT toDateTime(Timestamp) CODEC(Delta(4), ZSTD(1)),
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1)),
    TraceFlags UInt8 CODEC(T64, ZSTD(1)),
    SeverityText LowCardinality(String) CODEC(ZSTD(1)),
    SeverityNumber UInt8 CODEC(T64, ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    Body String CODEC(ZSTD(1)),
    ResourceSchemaUrl LowCardinality(String) CODEC(ZSTD(1)),
    ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    ScopeSchemaUrl LowCardinality(String) CODEC(ZSTD(1)),
    ScopeName String CODEC(ZSTD(1)),
    ScopeVersion LowCardinality(String) CODEC(ZSTD(1)),
    ScopeAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    LogAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    EventName String CODEC(ZSTD(1))
) ENGINE = MergeTree()
PARTITION BY toDate(TimestampTime)
ORDER BY (ServiceName, TimestampTime, Timestamp)
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

CREATE TABLE IF NOT EXISTS otel_traces (
    Timestamp DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1)),
    ParentSpanId String CODEC(ZSTD(1)),
    TraceState String CODEC(ZSTD(1)),
    SpanName LowCardinality(String) CODEC(ZSTD(1)),
    SpanKind LowCardinality(String) CODEC(ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    ScopeName String CODEC(ZSTD(1)),
    ScopeVersion String CODEC(ZSTD(1)),
    SpanAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    Duration UInt64 CODEC(T64, ZSTD(1)),
    StatusCode LowCardinality(String) CODEC(ZSTD(1)),
    StatusMessage String CODEC(ZSTD(1)),
    Events Nested (
        Timestamp DateTime64(9),
        Name LowCardinality(String),
        Attributes Map(LowCardinality(String), String)
    ) CODEC(ZSTD(1)),
    Links Nested (
        TraceId String,
        SpanId String,
        TraceState String,
        Attributes Map(LowCardinality(String), String)
    ) CODEC(ZSTD(1))
) ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, SpanName, toDateTime(Timestamp))
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

CREATE TABLE IF NOT EXISTS hyperdx_sessions (
    Timestamp DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    TimestampTime DateTime DEFAULT toDateTime(Timestamp) CODEC(Delta(4), ZSTD(1)),
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1)),
    TraceFlags UInt8 CODEC(T64, ZSTD(1)),
    SeverityText LowCardinality(String) CODEC(ZSTD(1)),
    SeverityNumber UInt8 CODEC(T64, ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    Body String CODEC(ZSTD(1)),
    ResourceSchemaUrl LowCardinality(String) CODEC(ZSTD(1)),
    ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    ScopeSchemaUrl LowCardinality(String) CODEC(ZSTD(1)),
    ScopeName String CODEC(ZSTD(1)),
    ScopeVersion LowCardinality(String) CODEC(ZSTD(1)),
    ScopeAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    LogAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    EventName String CODEC(ZSTD(1))
) ENGINE = MergeTree()
PARTITION BY toDate(TimestampTime)
ORDER BY (ServiceName, TimestampTime, Timestamp)
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

CREATE TABLE IF NOT EXISTS sobs_error_resolutions (
    ErrorId String CODEC(ZSTD(1)),
    ResolvedAt DateTime64(3) DEFAULT now64(3) CODEC(Delta(8), ZSTD(1))
) ENGINE = MergeTree()
ORDER BY (ErrorId, ResolvedAt)
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;
"""


def _validate_unsupported_storage_configuration() -> None:
    if os.environ.get("SOBS_CHDB_ENCRYPTION_KEY", "").strip():
        raise RuntimeError(
            "Embedded chDB does not support ClickHouse storage_configuration/custom_local_disks_base_directory "
            "for encrypted disks via the Python API. Keep schema compression enabled, but use an external "
            "ClickHouse server if you need encrypted-disk storage."
        )


class RowCompat(dict):
    """Row wrapper supporting both key and integer-index access."""

    def __init__(self, columns, values):
        super().__init__(zip(columns, values))
        self._values = tuple(values)

    def __getitem__(self, key):
        if isinstance(key, int):
            return self._values[key]
        return super().__getitem__(key)


class ChDbResult:
    """Pre-materialised query result; data fetched while the lock is held."""

    def __init__(self, columns, rows):
        self._columns = columns
        self._rows = rows
        self._idx = 0

    def fetchone(self):
        if self._idx >= len(self._rows):
            return None
        row = RowCompat(self._columns, self._rows[self._idx])
        self._idx += 1
        return row

    def fetchall(self):
        return [RowCompat(self._columns, r) for r in self._rows[self._idx :]]


class ChDbConnection:
    """Thread-safe global chDB connection wrapper."""

    def __init__(self, path: str):
        _validate_unsupported_storage_configuration()
        self._conn = chdb_driver.connect(path)
        self._lock = threading.Lock()

    def execute(self, query: str, params=None):
        with self._lock:
            cur = self._conn.cursor()
            if params:
                cur.execute(query, params)
            else:
                cur.execute(query)
            columns = [d[0] for d in (cur.description or [])]
            rows = cur.fetchall() or []
        return ChDbResult(columns, rows)

    def executescript(self, script: str):
        statements = [s.strip() for s in script.split(";") if s.strip()]
        with self._lock:
            cur = self._conn.cursor()
            for stmt in statements:
                cur.execute(stmt)

    def commit(self):
        return None  # ClickHouse auto-commits

    def close(self):
        self._conn.close()


_global_db: ChDbConnection | None = None
_db_init_lock = threading.Lock()
_schema_ready = False
_write_queue: queue.Queue["_WriteTask"] | None = None
_write_thread: threading.Thread | None = None
_write_worker_lock = threading.Lock()

WRITE_QUEUE_MAX = int(os.environ.get("SOBS_WRITE_QUEUE_MAX", 5000))
WRITE_BATCH_MAX = int(os.environ.get("SOBS_WRITE_BATCH_MAX", 200))
WRITE_BATCH_WAIT_MS = int(os.environ.get("SOBS_WRITE_BATCH_WAIT_MS", 20))


@dataclass
class _WriteTask:
    op: Callable[[ChDbConnection], None]
    done: threading.Event | None = None
    error: Exception | None = None


class WriteQueueFullError(RuntimeError):
    """Raised when ingest cannot enqueue a write within timeout."""


def get_db() -> ChDbConnection:
    global _global_db, _schema_ready
    if _global_db is None or not _schema_ready:
        with _db_init_lock:
            if _global_db is None:
                _global_db = ChDbConnection(DB_PATH)
            if not _schema_ready:
                _global_db.executescript(SCHEMA)
                _schema_ready = True
    return _global_db


def init_db():
    """(Re-)initialise the global DB connection and apply the schema."""
    global _global_db, _schema_ready
    with _db_init_lock:
        _global_db = ChDbConnection(DB_PATH)
        _global_db.executescript(SCHEMA)
        _schema_ready = True


def ensure_db_schema():
    """Create schema if tables are missing (fallback for fresh DB directories)."""
    global _global_db, _schema_ready
    if _schema_ready:
        return
    with _db_init_lock:
        if _global_db is None:
            _global_db = ChDbConnection(DB_PATH)
        try:
            has_logs = _global_db.execute(
                "SELECT 1 FROM system.tables WHERE database='default' AND name='otel_logs'"
            ).fetchone()
        except Exception:
            has_logs = None
        if has_logs is None:
            _global_db.executescript(SCHEMA)
        _schema_ready = True


def _run_write_batch(tasks: list[_WriteTask]) -> None:
    db = get_db()
    for task in tasks:
        try:
            task.op(db)
        except Exception as exc:
            task.error = exc
    db.commit()
    for task in tasks:
        if task.done is not None:
            task.done.set()


def _write_worker_main() -> None:
    assert _write_queue is not None
    while True:
        first = _write_queue.get()
        batch = [first]
        deadline = time.monotonic() + (max(1, WRITE_BATCH_WAIT_MS) / 1000.0)
        while len(batch) < max(1, WRITE_BATCH_MAX):
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                break
            try:
                batch.append(_write_queue.get(timeout=remaining))
            except queue.Empty:
                break
        _run_write_batch(batch)


def _ensure_write_worker() -> None:
    global _write_queue, _write_thread
    if _write_thread is not None and _write_thread.is_alive():
        return
    with _write_worker_lock:
        if _write_queue is None:
            _write_queue = queue.Queue(maxsize=max(1, WRITE_QUEUE_MAX))
        if _write_thread is None or not _write_thread.is_alive():
            _write_thread = threading.Thread(target=_write_worker_main, name="sobs-db-writer", daemon=True)
            _write_thread.start()


def _queue_write(op: Callable[[ChDbConnection], None], wait: bool = False) -> None:
    _ensure_write_worker()
    done = threading.Event() if wait else None
    task = _WriteTask(op=op, done=done)
    assert _write_queue is not None
    try:
        _write_queue.put(task, timeout=1)
    except queue.Full as exc:
        raise WriteQueueFullError("write queue is full") from exc
    if done is not None:
        # Intentionally best-effort wait: embedded chDB runs in single-process mode
        # and sustained bursts can delay writer completion. We avoid surfacing a hard
        # timeout to clients here to prevent avoidable 5xx responses under backpressure.
        done.wait(timeout=15)
        if task.error is not None:
            raise task.error


def _write_queue_depth() -> int:
    return _write_queue.qsize() if _write_queue is not None else 0


# ---------------------------------------------------------------------------
# Compression helpers
# ---------------------------------------------------------------------------
def compress(text: str) -> str:
    """Compress text and return as a base64-encoded string (chDB-safe)."""
    return base64.b64encode(zlib.compress(text.encode("utf-8"), level=9)).decode("ascii")


def decompress(data) -> str:
    """Decompress a base64-encoded compressed value. Returns '' for None/empty."""
    if not data:
        return ""
    raw = base64.b64decode(data) if isinstance(data, str) else data
    return zlib.decompress(raw).decode("utf-8")


def compress_json(obj) -> str:
    return compress(json.dumps(obj, ensure_ascii=False))


def decompress_json(data):
    if data is None:
        return {}
    return json.loads(decompress(data))


# ---------------------------------------------------------------------------
# Auth decorator (optional API key)
# ---------------------------------------------------------------------------
def _check_external_auth(authorization: str) -> bool:
    """Validate a Bearer token against the configured external auth service.

    Makes a POST to ``{EXTERNAL_AUTH_URL}/internal/auth/validate`` forwarding
    the ``Authorization`` header.  Returns ``True`` only on an HTTP 200 reply.
    """
    if not EXTERNAL_AUTH_URL:
        return False
    try:
        url = EXTERNAL_AUTH_URL.rstrip("/") + "/internal/auth/validate"
        req = urllib.request.Request(url, method="POST")
        req.add_header("Authorization", authorization)
        with urllib.request.urlopen(req, timeout=5) as resp:
            return resp.status == 200
    except (urllib.error.URLError, OSError):
        return False


def _auth_mode() -> str:
    """Return auth mode: none, basic, external, or invalid."""
    has_user = bool(BASIC_AUTH_USERNAME)
    has_pass = bool(BASIC_AUTH_PASSWORD)
    has_external = bool(EXTERNAL_AUTH_URL)

    # Configuration is exclusive: use at most one auth type.
    if has_external and (has_user or has_pass):
        return "invalid"
    # Basic auth requires both username and password.
    if has_user != has_pass:
        return "invalid"
    if has_external:
        return "external"
    if has_user and has_pass:
        return "basic"
    return "none"


def require_api_key(f):
    @wraps(f)
    async def decorated(*args, **kwargs):
        if API_KEY:
            key = request.headers.get("X-API-Key") or request.args.get("api_key")
            if key != API_KEY:
                return jsonify({"error": "Unauthorized"}), 401
        result = f(*args, **kwargs)
        if inspect.isawaitable(result):
            return await result
        return result

    return decorated


# ---------------------------------------------------------------------------
# Auth decorator (optional Basic Auth for Web UI)
# ---------------------------------------------------------------------------
def require_basic_auth(f):
    @wraps(f)
    async def decorated(*args, **kwargs):
        mode = _auth_mode()
        if mode == "invalid":
            return jsonify({"error": "Server auth misconfiguration"}), 500
        if mode == "none":
            result = f(*args, **kwargs)
            if inspect.isawaitable(result):
                return await result
            return result
        auth = request.headers.get("Authorization", "")
        # Accept valid HTTP Basic credentials when configured.
        if mode == "basic" and auth.startswith("Basic "):
            try:
                decoded = base64.b64decode(auth[6:], validate=True).decode("utf-8")
                username, _, password = decoded.partition(":")
                user_ok = secrets.compare_digest(username, BASIC_AUTH_USERNAME)
                pass_ok = secrets.compare_digest(password, BASIC_AUTH_PASSWORD)
                if user_ok and pass_ok:
                    result = f(*args, **kwargs)
                    if inspect.isawaitable(result):
                        return await result
                    return result
            except Exception:
                pass
        # Accept a Bearer token validated by the external auth service.
        # Fall back to the `session` cookie for same-origin browser requests
        # that carry no explicit Authorization header.
        if mode == "external":
            if not auth.startswith("Bearer "):
                session_cookie = request.cookies.get("session")
                if session_cookie and "\r" not in session_cookie and "\n" not in session_cookie:
                    auth = "Bearer " + session_cookie
            if auth.startswith("Bearer ") and await asyncio.to_thread(_check_external_auth, auth):
                result = f(*args, **kwargs)
                if inspect.isawaitable(result):
                    return await result
                return result
        # Advertise the configured auth scheme.
        if mode == "basic":
            www_auth = 'Basic realm="SOBS"'
        else:
            www_auth = 'Bearer realm="SOBS"'
        return (
            "Unauthorized",
            401,
            {"WWW-Authenticate": www_auth},
        )

    return decorated


# ---------------------------------------------------------------------------
# Utility helpers
# ---------------------------------------------------------------------------
def _now_iso() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="milliseconds")


def _ns_to_iso(nanos: int) -> str:
    """Convert OpenTelemetry nanosecond timestamp to ISO-8601."""
    try:
        secs = nanos / 1_000_000_000
        return datetime.fromtimestamp(secs, tz=timezone.utc).isoformat(timespec="milliseconds")
    except Exception:
        return _now_iso()


def _parse_limit(default=200) -> int:
    try:
        return max(1, min(int(request.args.get("limit", default)), 5000))
    except (TypeError, ValueError):
        return default


def _parse_offset() -> int:
    try:
        return max(0, int(request.args.get("offset", 0)))
    except (TypeError, ValueError):
        return 0


def _hex(b) -> str:
    """Convert bytes or hex string to hex string."""
    if isinstance(b, (bytes, bytearray)):
        return b.hex()
    return str(b) if b else ""


def _stringify_attrs(values: dict | None) -> dict[str, str]:
    """Convert arbitrary attribute values to a string map suitable for OTel Map columns."""
    if not values:
        return {}
    out: dict[str, str] = {}
    for key, value in values.items():
        if value is None:
            continue
        if isinstance(value, (str, int, float, bool)):
            out[str(key)] = str(value)
        else:
            out[str(key)] = json.dumps(value, ensure_ascii=False)
    return out


def _map_to_dict(value) -> dict:
    """Best-effort conversion of ClickHouse Map values to Python dicts."""
    if isinstance(value, dict):
        return value
    if not value:
        return {}
    if isinstance(value, str):
        s = value.strip()
        if not s:
            return {}
        try:
            parsed = json.loads(s)
            return parsed if isinstance(parsed, dict) else {}
        except json.JSONDecodeError:
            pass
        try:
            parsed = ast.literal_eval(s)
            return parsed if isinstance(parsed, dict) else {}
        except (ValueError, SyntaxError):
            return {}
    return {}


def _severity_number(level: str) -> int:
    norm = (level or "").upper()
    mapping = {
        "TRACE": 1,
        "DEBUG": 5,
        "INFO": 9,
        "WARN": 13,
        "WARNING": 13,
        "ERROR": 17,
        "CRITICAL": 21,
        "FATAL": 21,
        "METRIC": 9,
    }
    return mapping.get(norm, 9)


def _trace_status_code(status: str) -> str:
    norm = (status or "").upper()
    if norm == "ERROR":
        return "STATUS_CODE_ERROR"
    if norm == "OK":
        return "STATUS_CODE_OK"
    return "STATUS_CODE_UNSET"


def _error_id(ts: str, service: str, err_type: str, message: str, trace_id: str, span_id: str) -> str:
    raw = "|".join([ts or "", service or "", err_type or "", message or "", trace_id or "", span_id or ""])
    return hashlib.md5(raw.encode("utf-8")).hexdigest()


def _insert_rows_json_each_row(db, table_name: str, rows: list[dict]) -> int:
    if not rows:
        return 0
    normalized_rows = []
    for row in rows:
        item = dict(row)
        if "Timestamp" in item:
            item["Timestamp"] = _normalize_ch_timestamp(item["Timestamp"])
        if "Events" in item and isinstance(item["Events"], dict) and "Timestamp" in item["Events"]:
            item["Events"]["Timestamp"] = [_normalize_ch_timestamp(v) for v in item["Events"]["Timestamp"]]
        normalized_rows.append(item)
    payload = "\n".join(json.dumps(row, ensure_ascii=False) for row in normalized_rows)
    db.execute(f"INSERT INTO {table_name} FORMAT JSONEachRow\n" + payload)
    return len(normalized_rows)


def _normalize_ch_timestamp(value) -> str:
    """Convert common timestamp forms to ClickHouse DateTime64-compatible strings."""
    if isinstance(value, datetime):
        dt = value.astimezone(timezone.utc) if value.tzinfo else value
    else:
        raw = str(value or "").strip()
        if not raw:
            dt = datetime.now(timezone.utc)
        else:
            try:
                dt = datetime.fromisoformat(raw.replace("Z", "+00:00"))
            except ValueError:
                # Last resort: preserve value and hope ClickHouse parser accepts it.
                return raw.replace("T", " ")
            if dt.tzinfo:
                dt = dt.astimezone(timezone.utc)
    return dt.strftime("%Y-%m-%d %H:%M:%S.%f")


def _attr_list_to_dict(attr_list: list) -> dict:
    """Convert OTLP attribute list [{key, value}] to plain dict."""
    out = {}
    for item in attr_list:
        key = item.get("key", "")
        val_obj = item.get("value", {})
        # OTLP uses typed value wrappers
        for vtype in ("stringValue", "intValue", "doubleValue", "boolValue", "bytesValue"):
            if vtype in val_obj:
                out[key] = val_obj[vtype]
                break
    return out


def _proto_any_value_to_python(val):
    """Convert OTLP AnyValue proto object to a plain Python value."""
    kind = val.WhichOneof("value")
    if kind == "string_value":
        return val.string_value
    if kind == "int_value":
        return val.int_value
    if kind == "double_value":
        return val.double_value
    if kind == "bool_value":
        return val.bool_value
    if kind == "bytes_value":
        return base64.b64encode(bytes(val.bytes_value)).decode("ascii")
    if kind == "array_value":
        return [_proto_any_value_to_python(v) for v in val.array_value.values]
    if kind == "kvlist_value":
        return {kv.key: _proto_any_value_to_python(kv.value) for kv in val.kvlist_value.values}
    return None


def _proto_kvlist_to_dict(attributes) -> dict:
    return {kv.key: _proto_any_value_to_python(kv.value) for kv in attributes}


@dataclass
class LogEvent:
    ts: str
    level: str
    service: str
    body: str
    attrs: dict
    trace_id: str
    span_id: str


@dataclass
class SpanEvent:
    ts: str
    trace_id: str
    span_id: str
    parent_span_id: str
    name: str
    service: str
    duration_ms: float
    status: str
    attrs: dict


@dataclass
class ErrorEvent:
    ts: str
    service: str
    err_type: str
    message: str
    stack: str
    attrs: dict
    trace_id: str
    span_id: str


@dataclass
class MetricEvent:
    ts: str
    service: str
    name: str
    attrs: dict


def _proto_logs_to_events(msg: ExportLogsServiceRequest) -> list[LogEvent]:
    events: list[LogEvent] = []
    for resource_log in msg.resource_logs:
        resource_attrs = _proto_kvlist_to_dict(resource_log.resource.attributes)
        service = str(resource_attrs.get("service.name", ""))
        for scope_log in resource_log.scope_logs:
            for record in scope_log.log_records:
                record_attrs = _proto_kvlist_to_dict(record.attributes)
                merged_attrs = {**resource_attrs, **record_attrs}
                body_val = _proto_any_value_to_python(record.body)
                body_str = body_val if isinstance(body_val, str) else json.dumps(body_val, ensure_ascii=False)
                events.append(
                    LogEvent(
                        ts=_ns_to_iso(int(record.time_unix_nano or 0)),
                        level=(record.severity_text or "INFO").upper(),
                        service=service,
                        body=body_str,
                        attrs=merged_attrs,
                        trace_id=record.trace_id.hex() if record.trace_id else "",
                        span_id=record.span_id.hex() if record.span_id else "",
                    )
                )
    return events


def _proto_traces_to_events(msg: ExportTraceServiceRequest) -> tuple[list[SpanEvent], list[ErrorEvent]]:
    span_events: list[SpanEvent] = []
    error_events: list[ErrorEvent] = []
    for resource_span in msg.resource_spans:
        resource_attrs = _proto_kvlist_to_dict(resource_span.resource.attributes)
        service = str(resource_attrs.get("service.name", ""))
        for scope_span in resource_span.scope_spans:
            for span in scope_span.spans:
                start_ns = int(span.start_time_unix_nano or 0)
                end_ns = int(span.end_time_unix_nano or 0)
                duration_ms = (end_ns - start_ns) / 1_000_000 if end_ns > start_ns else 0
                status = "OK" if span.status.code == 1 else ("ERROR" if span.status.code == 2 else "UNSET")
                span_attrs = _proto_kvlist_to_dict(span.attributes)
                merged_attrs = {**resource_attrs, **span_attrs}
                span_event = SpanEvent(
                    ts=_ns_to_iso(start_ns),
                    trace_id=span.trace_id.hex() if span.trace_id else "",
                    span_id=span.span_id.hex() if span.span_id else "",
                    parent_span_id=span.parent_span_id.hex() if span.parent_span_id else "",
                    name=span.name,
                    service=service,
                    duration_ms=duration_ms,
                    status=status,
                    attrs=merged_attrs,
                )
                span_events.append(span_event)
                if "ERROR" in status.upper():
                    error_events.append(
                        ErrorEvent(
                            ts=span_event.ts,
                            service=service,
                            err_type=str(span_attrs.get("exception.type", "SpanError")),
                            message=str(
                                span_attrs.get("exception.message", span_attrs.get("error.message", span.name))
                            ),
                            stack=str(span_attrs.get("exception.stacktrace", "")),
                            attrs=merged_attrs,
                            trace_id=span_event.trace_id,
                            span_id=span_event.span_id,
                        )
                    )
    return span_events, error_events


def _proto_metrics_to_events(msg: ExportMetricsServiceRequest) -> list[MetricEvent]:
    events: list[MetricEvent] = []
    for resource_metric in msg.resource_metrics:
        resource_attrs = _proto_kvlist_to_dict(resource_metric.resource.attributes)
        service = str(resource_attrs.get("service.name", "metrics"))
        for scope_metric in resource_metric.scope_metrics:
            for metric in scope_metric.metrics:
                events.append(
                    MetricEvent(
                        ts=_now_iso(),
                        service=service,
                        name=metric.name,
                        attrs={**resource_attrs, "metric": metric.name},
                    )
                )
    return events


def _insert_log_events(db, events: list[LogEvent]) -> int:
    rows = []
    for event in events:
        rows.append(
            {
                "Timestamp": event.ts,
                "TraceId": event.trace_id,
                "SpanId": event.span_id,
                "TraceFlags": 0,
                "SeverityText": event.level,
                "SeverityNumber": _severity_number(event.level),
                "ServiceName": event.service,
                "Body": event.body,
                "ResourceSchemaUrl": "",
                "ResourceAttributes": {},
                "ScopeSchemaUrl": "",
                "ScopeName": "",
                "ScopeVersion": "",
                "ScopeAttributes": {},
                "LogAttributes": _stringify_attrs(event.attrs),
                "EventName": str(event.attrs.get("event.name", "")),
            }
        )
    return _insert_rows_json_each_row(db, "otel_logs", rows)


def _insert_span_events(db, span_events: list[SpanEvent]) -> int:
    rows = []
    for event in span_events:
        rows.append(
            {
                "Timestamp": event.ts,
                "TraceId": event.trace_id,
                "SpanId": event.span_id,
                "ParentSpanId": event.parent_span_id,
                "TraceState": "",
                "SpanName": event.name,
                "SpanKind": event.attrs.get("span.kind", "INTERNAL"),
                "ServiceName": event.service,
                "ResourceAttributes": {"service.name": event.service} if event.service else {},
                "ScopeName": "",
                "ScopeVersion": "",
                "SpanAttributes": _stringify_attrs(event.attrs),
                "Duration": max(0, int(event.duration_ms * 1_000_000)),
                "StatusCode": _trace_status_code(event.status),
                "StatusMessage": str(event.attrs.get("status.message", "")),
                "Events": {"Timestamp": [], "Name": [], "Attributes": []},
                "Links": {"TraceId": [], "SpanId": [], "TraceState": [], "Attributes": []},
            }
        )
    return _insert_rows_json_each_row(db, "otel_traces", rows)


def _insert_error_events(db, error_events: list[ErrorEvent]):
    rows = []
    for event in error_events:
        attrs = _stringify_attrs(event.attrs)
        attrs["exception.type"] = event.err_type
        attrs["exception.message"] = event.message
        if event.stack:
            attrs["exception.stacktrace"] = event.stack
        rows.append(
            {
                "Timestamp": event.ts,
                "TraceId": event.trace_id,
                "SpanId": event.span_id,
                "TraceFlags": 0,
                "SeverityText": "ERROR",
                "SeverityNumber": _severity_number("ERROR"),
                "ServiceName": event.service,
                "Body": event.message,
                "ResourceSchemaUrl": "",
                "ResourceAttributes": {},
                "ScopeSchemaUrl": "",
                "ScopeName": "",
                "ScopeVersion": "",
                "ScopeAttributes": {},
                "LogAttributes": attrs,
                "EventName": "exception",
            }
        )
    _insert_rows_json_each_row(db, "otel_logs", rows)


def _insert_metric_events(db, events: list[MetricEvent]) -> int:
    rows = []
    for event in events:
        attrs = _stringify_attrs(event.attrs)
        attrs["metric.name"] = event.name
        rows.append(
            {
                "Timestamp": event.ts,
                "TraceId": "",
                "SpanId": "",
                "TraceFlags": 0,
                "SeverityText": "INFO",
                "SeverityNumber": _severity_number("INFO"),
                "ServiceName": event.service,
                "Body": f"METRIC {event.name}",
                "ResourceSchemaUrl": "",
                "ResourceAttributes": {},
                "ScopeSchemaUrl": "",
                "ScopeName": "",
                "ScopeVersion": "",
                "ScopeAttributes": {},
                "LogAttributes": attrs,
                "EventName": "metric",
            }
        )
    return _insert_rows_json_each_row(db, "otel_logs", rows)


_PROTOBUF_CONTENT_TYPE = "application/x-protobuf"


async def _parse_otlp_request(proto_class):
    """
    Parse an OTLP HTTP request body.

    Returns ``(proto_message, error_response)`` where ``error_response`` is
    ``None`` on success or a ``(flask_response, status_code)`` tuple on failure.

    - ``Content-Type: application/x-protobuf`` → deserialise with *proto_class*.
    - Any other content-type (including ``application/json``) → parse JSON and
      map into the same protobuf class via protobuf JSON mapping.
    """
    mimetype = (request.mimetype or "").lower()
    msg = proto_class()
    if mimetype == _PROTOBUF_CONTENT_TYPE:
        app.logger.debug("OTLP ingest: parse_path=protobuf endpoint=%s", request.path)
        try:
            msg.ParseFromString(await request.get_data())
        except Exception as exc:
            app.logger.warning("OTLP protobuf parse error [%s]: %s", request.path, exc)
            return None, (jsonify({"error": "failed to parse protobuf body"}), 400)
        return msg, None
    app.logger.debug("OTLP ingest: parse_path=json endpoint=%s", request.path)
    payload = await request.get_json(force=True, silent=True)
    if payload is None:
        payload = {}
    try:
        ParseDict(payload, msg)
    except Exception as exc:
        app.logger.warning("OTLP json parse error [%s]: %s", request.path, exc)
        return None, (jsonify({"error": "failed to parse json body"}), 400)
    return msg, None


# ---------------------------------------------------------------------------
# OTLP Ingest – Logs  POST /v1/logs
# ---------------------------------------------------------------------------
@app.route("/v1/logs", methods=["POST"])
@require_api_key
async def ingest_logs():
    msg, err = await _parse_otlp_request(ExportLogsServiceRequest)
    if err:
        return err
    events = _proto_logs_to_events(msg)
    wait = bool(app.config.get("TESTING", False))
    try:
        _queue_write(lambda db: _insert_log_events(db, events), wait=wait)
    except WriteQueueFullError as exc:
        return jsonify({"error": str(exc)}), 503
    except Exception as exc:
        app.logger.exception("log ingest write failed")
        return jsonify({"error": str(exc)}), 500
    count = len(events)
    return jsonify({"accepted": count}), 200


# ---------------------------------------------------------------------------
# OTLP Ingest – Traces  POST /v1/traces
# ---------------------------------------------------------------------------
@app.route("/v1/traces", methods=["POST"])
@require_api_key
async def ingest_traces():
    msg, err = await _parse_otlp_request(ExportTraceServiceRequest)
    if err:
        return err
    span_events, error_events = _proto_traces_to_events(msg)
    wait = bool(app.config.get("TESTING", False))

    def _op(db: ChDbConnection) -> None:
        _insert_span_events(db, span_events)
        _insert_error_events(db, error_events)

    try:
        _queue_write(_op, wait=wait)
    except WriteQueueFullError as exc:
        return jsonify({"error": str(exc)}), 503
    except Exception as exc:
        app.logger.exception("trace ingest write failed")
        return jsonify({"error": str(exc)}), 500
    count = len(span_events)
    return jsonify({"accepted": count}), 200


# ---------------------------------------------------------------------------
# OTLP Ingest – Metrics  POST /v1/metrics  (stored as logs for simplicity)
# ---------------------------------------------------------------------------
@app.route("/v1/metrics", methods=["POST"])
@require_api_key
async def ingest_metrics():
    msg, err = await _parse_otlp_request(ExportMetricsServiceRequest)
    if err:
        return err
    events = _proto_metrics_to_events(msg)
    wait = bool(app.config.get("TESTING", False))
    try:
        _queue_write(lambda db: _insert_metric_events(db, events), wait=wait)
    except WriteQueueFullError as exc:
        return jsonify({"error": str(exc)}), 503
    except Exception as exc:
        app.logger.exception("metric ingest write failed")
        return jsonify({"error": str(exc)}), 500
    count = len(events)
    return jsonify({"accepted": count}), 200


# ---------------------------------------------------------------------------
# RUM Ingest  POST /v1/rum
# ---------------------------------------------------------------------------
@app.route("/v1/rum", methods=["POST"])
@require_api_key
async def ingest_rum():
    payload = await request.get_json(force=True, silent=True)
    if payload is None:
        payload = {}
    if isinstance(payload, list):
        events = payload
    else:
        events = payload.get("events", [payload])
    session_rows = []
    error_rows = []
    for event in events:
        ts = event.get("timestamp", _now_iso())
        session_id = event.get("sessionId", "")
        event_type = event.get("type", "unknown")
        url = event.get("url", "")
        attrs = _stringify_attrs(event)
        session_rows.append(
            {
                "Timestamp": ts,
                "TraceId": str(event.get("traceId", "")),
                "SpanId": str(event.get("spanId", "")),
                "TraceFlags": 0,
                "SeverityText": "ERROR" if event_type in ("error", "unhandledrejection") else "INFO",
                "SeverityNumber": _severity_number(
                    "ERROR" if event_type in ("error", "unhandledrejection") else "INFO"
                ),
                "ServiceName": str(event.get("service", "browser")),
                "Body": json.dumps(event, ensure_ascii=False),
                "ResourceSchemaUrl": "",
                "ResourceAttributes": {},
                "ScopeSchemaUrl": "",
                "ScopeName": "browser-rum",
                "ScopeVersion": "",
                "ScopeAttributes": {},
                "LogAttributes": attrs,
                "EventName": event_type,
            }
        )

        # Also index browser exceptions into otel_logs for unified error views.
        if event_type in ("error", "unhandledrejection"):
            err_attrs = {
                "exception.type": str(event.get("errorType", "JSError")),
                "exception.message": str(event.get("message", "")),
                "url.full": url,
                "session.id": session_id,
            }
            if event.get("stack"):
                err_attrs["exception.stacktrace"] = str(event.get("stack"))
            error_rows.append(
                {
                    "Timestamp": ts,
                    "TraceId": str(event.get("traceId", "")),
                    "SpanId": str(event.get("spanId", "")),
                    "TraceFlags": 0,
                    "SeverityText": "ERROR",
                    "SeverityNumber": _severity_number("ERROR"),
                    "ServiceName": "rum",
                    "Body": str(event.get("message", "")),
                    "ResourceSchemaUrl": "",
                    "ResourceAttributes": {},
                    "ScopeSchemaUrl": "",
                    "ScopeName": "browser-rum",
                    "ScopeVersion": "",
                    "ScopeAttributes": {},
                    "LogAttributes": err_attrs,
                    "EventName": "exception",
                }
            )
    wait = bool(app.config.get("TESTING", False))

    def _op(db: ChDbConnection) -> None:
        _insert_rows_json_each_row(db, "hyperdx_sessions", session_rows)
        _insert_rows_json_each_row(db, "otel_logs", error_rows)

    try:
        _queue_write(_op, wait=wait)
    except WriteQueueFullError as exc:
        return jsonify({"error": str(exc)}), 503
    except Exception as exc:
        app.logger.exception("rum ingest write failed")
        return jsonify({"error": str(exc)}), 500
    return jsonify({"accepted": len(session_rows)}), 200


# ---------------------------------------------------------------------------
# AI Transparency  POST /v1/ai
# ---------------------------------------------------------------------------
@app.route("/v1/ai", methods=["POST"])
@require_api_key
async def ingest_ai():
    payload = await request.get_json(force=True, silent=True) or {}
    ts = payload.get("timestamp", _now_iso())
    model = str(payload.get("model", ""))
    operation = str(payload.get("operation", "chat"))
    duration_ms = float(payload.get("duration_ms", 0) or 0)
    span_name = f"{operation} {model}".strip()
    span_attrs = {
        "gen_ai.operation.name": operation,
        "gen_ai.provider.name": str(payload.get("provider", "")),
        "gen_ai.request.model": model,
        "gen_ai.usage.input_tokens": int(payload.get("tokens_in", 0) or 0),
        "gen_ai.usage.output_tokens": int(payload.get("tokens_out", 0) or 0),
    }
    if payload.get("prompt"):
        span_attrs["sobs.gen_ai.prompt"] = str(payload.get("prompt"))
    if payload.get("response"):
        span_attrs["sobs.gen_ai.response"] = str(payload.get("response"))
    row = {
        "Timestamp": ts,
        "TraceId": str(payload.get("trace_id", "")),
        "SpanId": str(payload.get("span_id", "")),
        "ParentSpanId": "",
        "TraceState": "",
        "SpanName": span_name,
        "SpanKind": "CLIENT",
        "ServiceName": str(payload.get("service", "")),
        "ResourceAttributes": {},
        "ScopeName": "sobs-ai",
        "ScopeVersion": "",
        "SpanAttributes": _stringify_attrs(span_attrs),
        "Duration": max(0, int(duration_ms * 1_000_000)),
        "StatusCode": "STATUS_CODE_OK",
        "StatusMessage": "",
        "Events": {"Timestamp": [], "Name": [], "Attributes": []},
        "Links": {"TraceId": [], "SpanId": [], "TraceState": [], "Attributes": []},
    }
    wait = bool(app.config.get("TESTING", False))
    try:
        _queue_write(lambda db: _insert_rows_json_each_row(db, "otel_traces", [row]), wait=wait)
    except WriteQueueFullError as exc:
        return jsonify({"error": str(exc)}), 503
    except Exception as exc:
        app.logger.exception("ai ingest write failed")
        return jsonify({"error": str(exc)}), 500
    return jsonify({"ok": True}), 200


# ---------------------------------------------------------------------------
# Error ingest  POST /v1/errors  (direct error submission)
# ---------------------------------------------------------------------------
@app.route("/v1/errors", methods=["POST"])
@require_api_key
async def ingest_errors():
    payload = await request.get_json(force=True, silent=True) or {}
    ts = payload.get("timestamp", _now_iso())
    attrs = _stringify_attrs(payload.get("attributes", {}))
    attrs["exception.type"] = str(payload.get("type", "Error"))
    attrs["exception.message"] = str(payload.get("message", ""))
    if payload.get("stack"):
        attrs["exception.stacktrace"] = str(payload.get("stack"))
    row = {
        "Timestamp": ts,
        "TraceId": str(payload.get("trace_id", "")),
        "SpanId": str(payload.get("span_id", "")),
        "TraceFlags": 0,
        "SeverityText": "ERROR",
        "SeverityNumber": _severity_number("ERROR"),
        "ServiceName": str(payload.get("service", "")),
        "Body": str(payload.get("message", "")),
        "ResourceSchemaUrl": "",
        "ResourceAttributes": {},
        "ScopeSchemaUrl": "",
        "ScopeName": "",
        "ScopeVersion": "",
        "ScopeAttributes": {},
        "LogAttributes": attrs,
        "EventName": "exception",
    }
    wait = bool(app.config.get("TESTING", False))
    try:
        _queue_write(lambda db: _insert_rows_json_each_row(db, "otel_logs", [row]), wait=wait)
    except WriteQueueFullError as exc:
        return jsonify({"error": str(exc)}), 503
    except Exception as exc:
        app.logger.exception("error ingest write failed")
        return jsonify({"error": str(exc)}), 500
    return jsonify({"ok": True}), 200


ERROR_SOURCES_SQL = """
SELECT Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes
FROM otel_logs
WHERE EventName = 'exception'
   OR SeverityNumber >= 17
   OR SeverityText IN ('ERROR', 'CRITICAL', 'FATAL')
   OR LogAttributes['exception.type'] != ''
UNION ALL
SELECT Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes
FROM hyperdx_sessions
WHERE EventName IN ('error', 'unhandledrejection', 'exception')
   OR SeverityNumber >= 17
   OR SeverityText IN ('ERROR', 'CRITICAL', 'FATAL')
   OR LogAttributes['exception.type'] != ''
"""


def _build_error_item(row: dict) -> dict:
    attrs = _map_to_dict(row.get("LogAttributes"))
    ts = str(row.get("Timestamp", ""))
    service = str(row.get("ServiceName", ""))
    err_type = str(attrs.get("exception.type", "Error"))
    message = str(attrs.get("exception.message", row.get("Body", "")))
    stack = str(attrs.get("exception.stacktrace", ""))
    trace_id = str(row.get("TraceId", ""))
    span_id = str(row.get("SpanId", ""))
    eid = _error_id(ts, service, err_type, message, trace_id, span_id)
    return {
        "id": eid,
        "ts": ts,
        "service": service,
        "err_type": err_type,
        "message": message,
        "stack": stack,
        "trace_id": trace_id,
        "span_id": span_id,
    }


def _get_resolved_error_ids(db) -> set[str]:
    return {str(r[0]) for r in db.execute("SELECT ErrorId FROM sobs_error_resolutions GROUP BY ErrorId").fetchall()}


# ---------------------------------------------------------------------------
# Web UI – Dashboard
# ---------------------------------------------------------------------------
@app.route("/")
@require_basic_auth
async def dashboard():
    db = get_db()
    resolved_ids = _get_resolved_error_ids(db)
    error_items = []
    for row in db.execute(f"SELECT * FROM ({ERROR_SOURCES_SQL}) ORDER BY Timestamp DESC").fetchall():
        item = _build_error_item(dict(row))
        item["resolved"] = item["id"] in resolved_ids
        error_items.append(item)

    unresolved_count = sum(0 if item["resolved"] else 1 for item in error_items)
    stats = {
        "logs": db.execute("SELECT COUNT(*) FROM otel_logs").fetchone()[0],
        "errors": unresolved_count,
        "errors_total": len(error_items),
        "spans": db.execute("SELECT COUNT(*) FROM otel_traces").fetchone()[0],
        "rum": db.execute("SELECT COUNT(*) FROM hyperdx_sessions").fetchone()[0],
        "ai": db.execute(
            "SELECT COUNT(*) FROM otel_traces WHERE SpanAttributes['gen_ai.provider.name'] != ''"
        ).fetchone()[0],
        "services": [
            r[0]
            for r in db.execute(
                "SELECT DISTINCT ServiceName FROM otel_logs WHERE ServiceName!='' "
                "UNION DISTINCT SELECT DISTINCT ServiceName FROM otel_traces WHERE ServiceName!='' "
                "UNION DISTINCT SELECT DISTINCT ServiceName FROM hyperdx_sessions WHERE ServiceName!=''"
            ).fetchall()
        ],
    }
    # Recent errors (last 5)
    recent_errors = [
        {
            "id": item["id"],
            "ts": item["ts"],
            "service": item["service"],
            "err_type": item["err_type"],
            "message": item["message"],
        }
        for item in error_items
        if not item["resolved"]
    ][:5]
    # Recent logs (last 10)
    recent_logs = []
    for r in db.execute(
        "SELECT Timestamp, SeverityText, ServiceName, Body FROM otel_logs ORDER BY Timestamp DESC LIMIT 10"
    ).fetchall():
        recent_logs.append(
            {
                "ts": str(r["Timestamp"]),
                "level": r["SeverityText"],
                "service": r["ServiceName"],
                "body": r["Body"],
            }
        )
    # RUM summary – page views last 24h
    rum_summary = db.execute(
        "SELECT EventName, COUNT(*) as cnt FROM hyperdx_sessions GROUP BY EventName ORDER BY cnt DESC"
    ).fetchall()
    # AI summary
    ai_summary = db.execute(
        "SELECT SpanAttributes['gen_ai.request.model'] AS model, "
        "COUNT(*) cnt, "
        "SUM(toUInt64OrZero(SpanAttributes['gen_ai.usage.input_tokens'])) ti, "
        "SUM(toUInt64OrZero(SpanAttributes['gen_ai.usage.output_tokens'])) to_ "
        "FROM otel_traces "
        "WHERE SpanAttributes['gen_ai.provider.name'] != '' "
        "GROUP BY model"
    ).fetchall()
    return await render_template(
        "dashboard.html",
        stats=stats,
        recent_errors=recent_errors,
        recent_logs=recent_logs,
        rum_summary=rum_summary,
        ai_summary=ai_summary,
    )


# ---------------------------------------------------------------------------
# Web UI – Logs
# ---------------------------------------------------------------------------
@app.route("/logs")
@require_basic_auth
async def view_logs():
    db = get_db()
    q = request.args.get("q", "").strip()
    level = request.args.get("level", "").strip().upper()
    service = request.args.get("service", "").strip()
    sql_where = request.args.get("sql", "").strip()
    limit = _parse_limit(200)
    offset = _parse_offset()

    rows = []
    total = 0
    error_msg = ""

    if sql_where:
        # Allow raw WHERE clause (SQL search)
        try:
            safe_sql = sql_where.replace(";", "")
            safe_sql = re.sub(r"\blevel\b", "SeverityText", safe_sql, flags=re.IGNORECASE)
            safe_sql = re.sub(r"\bservice\b", "ServiceName", safe_sql, flags=re.IGNORECASE)
            safe_sql = re.sub(r"\btrace_id\b", "TraceId", safe_sql, flags=re.IGNORECASE)
            safe_sql = re.sub(r"\bspan_id\b", "SpanId", safe_sql, flags=re.IGNORECASE)
            safe_sql = re.sub(r"\bts\b", "Timestamp", safe_sql, flags=re.IGNORECASE)
            safe_sql = re.sub(r"\bbody\b", "Body", safe_sql, flags=re.IGNORECASE)
            query = (
                f"SELECT Timestamp, SeverityText, ServiceName, Body, TraceId, SpanId FROM otel_logs "
                f"WHERE {safe_sql} ORDER BY Timestamp DESC LIMIT ? OFFSET ?"
            )
            rows = db.execute(query, (limit, offset)).fetchall()
            total = db.execute(f"SELECT COUNT(*) FROM otel_logs WHERE {safe_sql}").fetchone()[0]
        except Exception as exc:
            error_msg = f"SQL error: {exc}"
            rows = []
    else:
        conditions = []
        params = []
        if level:
            conditions.append("SeverityText=?")
            params.append(level)
        if service:
            conditions.append("ServiceName=?")
            params.append(service)
        where = ("WHERE " + " AND ".join(conditions)) if conditions else ""
        total = db.execute(f"SELECT COUNT(*) FROM otel_logs {where}", params).fetchone()[0]
        rows = db.execute(
            f"SELECT Timestamp, SeverityText, ServiceName, Body, TraceId, SpanId FROM otel_logs {where} "
            "ORDER BY Timestamp DESC LIMIT ? OFFSET ?",
            params + [limit, offset],
        ).fetchall()

    log_rows = []
    grep_pat = re.compile(q, re.IGNORECASE) if q else None
    for r in rows:
        body = r["Body"]
        if grep_pat and not grep_pat.search(body):
            continue
        log_rows.append(
            {
                "ts": str(r["Timestamp"]),
                "level": r["SeverityText"],
                "service": r["ServiceName"],
                "body": body,
                "trace_id": r["TraceId"],
                "span_id": r["SpanId"],
            }
        )

    services = [
        row[0]
        for row in db.execute(
            "SELECT DISTINCT ServiceName FROM otel_logs WHERE ServiceName!='' ORDER BY ServiceName"
        ).fetchall()
    ]
    levels = [
        row[0] for row in db.execute("SELECT DISTINCT SeverityText FROM otel_logs ORDER BY SeverityText").fetchall()
    ]

    return await render_template(
        "logs.html",
        logs=log_rows,
        total=total,
        limit=limit,
        offset=offset,
        q=q,
        level=level,
        service=service,
        sql_where=sql_where,
        services=services,
        levels=levels,
        error_msg=error_msg,
    )


# ---------------------------------------------------------------------------
# Web UI – Errors
# ---------------------------------------------------------------------------
@app.route("/errors")
@require_basic_auth
async def view_errors():
    db = get_db()
    service = request.args.get("service", "").strip()
    resolved = request.args.get("resolved", "0").strip()
    limit = _parse_limit(100)
    offset = _parse_offset()
    resolved_ids = _get_resolved_error_ids(db)
    where_parts = []
    where_params = []
    if service:
        where_parts.append("ServiceName=?")
        where_params.append(service)
    where_sql = ("WHERE " + " AND ".join(where_parts)) if where_parts else ""
    source_sql = (
        "SELECT Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes "
        f"FROM ({ERROR_SOURCES_SQL}) {where_sql} "
        "ORDER BY Timestamp DESC LIMIT ? OFFSET ?"
    )

    if resolved not in ("0", "1"):
        total = db.execute(
            f"SELECT COUNT(*) FROM ({ERROR_SOURCES_SQL}) {where_sql}",
            where_params,
        ).fetchone()[0]
        rows = db.execute(source_sql, where_params + [limit, offset]).fetchall()
        errors = []
        for row in rows:
            item = _build_error_item(dict(row))
            item["resolved"] = item["id"] in resolved_ids
            errors.append(item)
    else:
        # Keep behavior identical while avoiding full in-memory materialization.
        target_resolved = resolved == "1"
        scan_batch = max(200, limit)
        scan_offset = 0
        total = 0
        errors = []
        while True:
            batch = db.execute(source_sql, where_params + [scan_batch, scan_offset]).fetchall()
            if not batch:
                break
            for row in batch:
                item = _build_error_item(dict(row))
                item["resolved"] = item["id"] in resolved_ids
                if item["resolved"] != target_resolved:
                    continue
                if total >= offset and len(errors) < limit:
                    errors.append(item)
                total += 1
            scan_offset += scan_batch

    services = [
        row[0]
        for row in db.execute(
            "SELECT DISTINCT ServiceName FROM (" + ERROR_SOURCES_SQL + ") WHERE ServiceName!='' ORDER BY ServiceName"
        ).fetchall()
    ]

    return await render_template(
        "errors.html",
        errors=errors,
        total=total,
        limit=limit,
        offset=offset,
        service=service,
        resolved=resolved,
        services=services,
    )


@app.route("/errors/<string:error_id>/resolve", methods=["POST"])
@require_basic_auth
async def resolve_error(error_id: str):
    try:

        def _op(db: ChDbConnection) -> None:
            db.execute("INSERT INTO sobs_error_resolutions(ErrorId) VALUES(?)", (error_id,))

        _queue_write(_op, wait=True)
    except Exception as exc:
        app.logger.exception("resolve error write failed")
        return jsonify({"error": str(exc)}), 500
    return jsonify({"ok": True})


# ---------------------------------------------------------------------------
# Web UI – Traces
# ---------------------------------------------------------------------------
@app.route("/traces")
@require_basic_auth
async def view_traces():
    db = get_db()
    service = request.args.get("service", "").strip()
    trace_id = request.args.get("trace_id", "").strip()
    limit = _parse_limit(100)
    offset = _parse_offset()

    conditions = []
    params = []
    if service:
        conditions.append("ServiceName=?")
        params.append(service)
    if trace_id:
        conditions.append("TraceId=?")
        params.append(trace_id)
    where = ("WHERE " + " AND ".join(conditions)) if conditions else ""

    total = db.execute(f"SELECT COUNT(*) FROM otel_traces {where}", params).fetchone()[0]
    rows = db.execute(
        f"SELECT Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration, StatusCode, SpanAttributes "
        f"FROM otel_traces {where} ORDER BY Timestamp DESC LIMIT ? OFFSET ?",
        params + [limit, offset],
    ).fetchall()

    spans = []
    for r in rows:
        attrs = _map_to_dict(r["SpanAttributes"])
        spans.append(
            {
                "ts": str(r["Timestamp"]),
                "trace_id": r["TraceId"],
                "span_id": r["SpanId"],
                "parent_span_id": r["ParentSpanId"],
                "name": r["SpanName"],
                "service": r["ServiceName"],
                "duration_ms": round(float(r["Duration"]) / 1_000_000, 2),
                "status": r["StatusCode"],
                "http_method": attrs.get("http.method", attrs.get("http.request.method", "")),
                "http_url": attrs.get("http.url", attrs.get("url.full", "")),
                "http_status": attrs.get("http.status_code", attrs.get("http.response.status_code", "")),
            }
        )

    services = [
        row[0]
        for row in db.execute(
            "SELECT DISTINCT ServiceName FROM otel_traces WHERE ServiceName!='' ORDER BY ServiceName"
        ).fetchall()
    ]

    return await render_template(
        "traces.html",
        spans=spans,
        total=total,
        limit=limit,
        offset=offset,
        service=service,
        trace_id=trace_id,
        services=services,
    )


# ---------------------------------------------------------------------------
# Web UI – RUM
# ---------------------------------------------------------------------------
@app.route("/rum")
@require_basic_auth
async def view_rum():
    db = get_db()
    event_type = request.args.get("type", "").strip()
    limit = _parse_limit(200)
    offset = _parse_offset()

    conditions = []
    params = []
    if event_type:
        conditions.append("EventName=?")
        params.append(event_type)
    where = ("WHERE " + " AND ".join(conditions)) if conditions else ""

    total = db.execute(f"SELECT COUNT(*) FROM hyperdx_sessions {where}", params).fetchone()[0]
    rows = db.execute(
        f"SELECT Timestamp, EventName, Body, LogAttributes FROM hyperdx_sessions {where} "
        "ORDER BY Timestamp DESC LIMIT ? OFFSET ?",
        params + [limit, offset],
    ).fetchall()

    events = []
    for r in rows:
        attrs = _map_to_dict(r["LogAttributes"])
        try:
            body_data = json.loads(r["Body"]) if r["Body"] else {}
        except json.JSONDecodeError:
            body_data = {}
        data = body_data if isinstance(body_data, dict) else {"value": body_data}
        events.append(
            {
                "ts": str(r["Timestamp"]),
                "session_id": str(attrs.get("sessionId", attrs.get("session.id", "")))[:8],
                "event_type": r["EventName"],
                "url": str(attrs.get("url", attrs.get("url.full", ""))),
                "data": data,
            }
        )

    event_types = [
        row[0] for row in db.execute("SELECT DISTINCT EventName FROM hyperdx_sessions ORDER BY EventName").fetchall()
    ]

    # Web vitals summary
    vitals_rows = db.execute(
        "SELECT Body, LogAttributes FROM hyperdx_sessions WHERE EventName='web-vital' "
        "ORDER BY Timestamp DESC LIMIT 500"
    ).fetchall()
    vitals = {}
    for vr in vitals_rows:
        attrs = _map_to_dict(vr["LogAttributes"])
        try:
            d = json.loads(vr["Body"]) if vr["Body"] else {}
        except json.JSONDecodeError:
            d = {}
        if not isinstance(d, dict):
            d = {}
        name = d.get("name", "")
        val = d.get("value", attrs.get("value"))
        try:
            val = float(val) if val is not None else None
        except (TypeError, ValueError):
            val = None
        if name and val is not None:
            vitals.setdefault(name, []).append(val)
    vitals_summary = {}
    for name, vals in vitals.items():
        vitals_summary[name] = {
            "avg": round(sum(vals) / len(vals), 1),
            "p75": round(sorted(vals)[int(len(vals) * 0.75)], 1),
            "count": len(vals),
        }

    return await render_template(
        "rum.html",
        events=events,
        total=total,
        limit=limit,
        offset=offset,
        event_type=event_type,
        event_types=event_types,
        vitals_summary=vitals_summary,
    )


# ---------------------------------------------------------------------------
# Web UI – AI Transparency
# ---------------------------------------------------------------------------
@app.route("/ai")
@require_basic_auth
async def view_ai():
    db = get_db()
    service = request.args.get("service", "").strip()
    model = request.args.get("model", "").strip()
    limit = _parse_limit(50)
    offset = _parse_offset()

    conditions = []
    params = []
    if service:
        conditions.append("ServiceName=?")
        params.append(service)
    if model:
        conditions.append("SpanAttributes['gen_ai.request.model']=?")
        params.append(model)
    conditions.append("SpanAttributes['gen_ai.provider.name'] != ''")
    where = ("WHERE " + " AND ".join(conditions)) if conditions else ""

    total = db.execute(f"SELECT COUNT(*) FROM otel_traces {where}", params).fetchone()[0]
    rows = db.execute(
        f"SELECT Timestamp, ServiceName, TraceId, Duration, SpanAttributes "
        f"FROM otel_traces {where} ORDER BY Timestamp DESC LIMIT ? OFFSET ?",
        params + [limit, offset],
    ).fetchall()

    ai_items = []
    for r in rows:
        attrs = _map_to_dict(r["SpanAttributes"])
        ts = str(r["Timestamp"])
        provider = str(attrs.get("gen_ai.provider.name", ""))
        req_model = str(attrs.get("gen_ai.request.model", ""))
        prompt = str(attrs.get("sobs.gen_ai.prompt", ""))
        response = str(attrs.get("sobs.gen_ai.response", ""))
        tokens_in = int(float(attrs.get("gen_ai.usage.input_tokens", "0") or 0))
        tokens_out = int(float(attrs.get("gen_ai.usage.output_tokens", "0") or 0))
        err_type = str(attrs.get("error.type", ""))
        msg = str(attrs.get("exception.message", ""))
        row_id = _error_id(ts, r["ServiceName"], provider, req_model + err_type + msg, r["TraceId"], "")
        ai_items.append(
            {
                "id": row_id,
                "ts": ts,
                "service": r["ServiceName"],
                "provider": provider,
                "model": req_model,
                "prompt": prompt,
                "response": response,
                "tokens_in": tokens_in,
                "tokens_out": tokens_out,
                "duration_ms": round(float(r["Duration"]) / 1_000_000, 1),
                "trace_id": r["TraceId"],
            }
        )

    services = [
        row[0]
        for row in db.execute(
            "SELECT DISTINCT ServiceName FROM otel_traces "
            "WHERE SpanAttributes['gen_ai.provider.name'] != '' AND ServiceName!='' ORDER BY ServiceName"
        ).fetchall()
    ]
    models = [
        row[0]
        for row in db.execute(
            "SELECT DISTINCT SpanAttributes['gen_ai.request.model'] AS model FROM otel_traces "
            "WHERE SpanAttributes['gen_ai.provider.name'] != '' "
            "AND SpanAttributes['gen_ai.request.model'] != '' ORDER BY model"
        ).fetchall()
    ]

    # Token usage totals
    totals = db.execute(
        "SELECT "
        "SUM(toUInt64OrZero(SpanAttributes['gen_ai.usage.input_tokens'])) ti, "
        "SUM(toUInt64OrZero(SpanAttributes['gen_ai.usage.output_tokens'])) to_, "
        "COUNT(*) cnt "
        "FROM otel_traces WHERE SpanAttributes['gen_ai.provider.name'] != ''"
    ).fetchone()

    return await render_template(
        "ai.html",
        ai_items=ai_items,
        total=total,
        limit=limit,
        offset=offset,
        service=service,
        model=model,
        services=services,
        models=models,
        total_tokens_in=totals["ti"] or 0,
        total_tokens_out=totals["to_"] or 0,
        total_calls=totals["cnt"] or 0,
    )


# ---------------------------------------------------------------------------
# Static RUM script
# ---------------------------------------------------------------------------
@app.route("/static/rum.js")
async def rum_js():
    return await send_from_directory(
        os.path.join(os.path.dirname(__file__), "static"), "rum.js", mimetype="application/javascript"
    )


# ---------------------------------------------------------------------------
# Health check
# ---------------------------------------------------------------------------
@app.route("/health")
async def health():
    return jsonify({"status": "ok", "version": "1.0.0"})


@app.route("/health/db")
async def health_db():
    started = time.perf_counter()
    try:
        ensure_db_schema()
        get_db().execute("SELECT 1").fetchone()
    except Exception as exc:
        app.logger.exception("DB readiness probe failed")
        return (
            jsonify(
                {
                    "status": "degraded",
                    "db": "error",
                    "error": str(exc),
                    "write_queue_depth": _write_queue_depth(),
                    "version": "1.0.0",
                }
            ),
            503,
        )

    latency_ms = round((time.perf_counter() - started) * 1000, 2)
    return jsonify(
        {
            "status": "ok",
            "db": "ok",
            "latency_ms": latency_ms,
            "write_queue_depth": _write_queue_depth(),
            "version": "1.0.0",
        }
    )


# ---------------------------------------------------------------------------
# Entrypoint
# ---------------------------------------------------------------------------
if __name__ == "__main__":
    port = int(os.environ.get("PORT", 4317))
    requested_workers = max(
        1,
        int(
            os.environ.get(
                "HYPERCORN_WORKERS",
                os.environ.get("GUNICORN_WORKERS", "1"),
            )
        ),
    )
    if requested_workers != 1:
        log.warning("Embedded chDB requires single-process mode; forcing worker count to 1")
    bind = os.environ.get("HYPERCORN_BIND", os.environ.get("GUNICORN_BIND", f"0.0.0.0:{port}"))

    config = HypercornConfig()
    config.bind = [bind]
    config.workers = 1
    config.use_reloader = False

    asyncio.run(hypercorn_serve(app, config))
