"""
SOBS - Simple Observe
A lightweight, single-user telemetry container supporting OpenTelemetry,
RUM, Logs, Errors, Traces, and AI transparency.
"""

import base64
import json
import logging
import os
import re
import secrets
import sqlite3
import urllib.error
import urllib.request
import zlib
from datetime import datetime, timezone
from functools import wraps

from flask import (
    Flask,
    g,
    jsonify,
    render_template,
    request,
    send_from_directory,
)

# ---------------------------------------------------------------------------
# App setup
# ---------------------------------------------------------------------------
app = Flask(__name__)


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


class BasePathMiddleware:
    """Support deployment behind a path prefix and reverse-proxy prefix headers."""

    def __init__(self, wrapped_app):
        self.wrapped_app = wrapped_app

    def __call__(self, environ, start_response):
        forwarded = _normalize_base_path(environ.get("HTTP_X_FORWARDED_PREFIX", ""))
        effective_base = forwarded or BASE_PATH

        if effective_base:
            path_info = environ.get("PATH_INFO", "") or "/"
            script_name = environ.get("SCRIPT_NAME", "")

            # Proxy kept the base path in PATH_INFO: strip it for route matching.
            if path_info == effective_base:
                environ["SCRIPT_NAME"] = _merge_script_name(script_name, effective_base)
                environ["PATH_INFO"] = "/"
            elif path_info.startswith(effective_base + "/"):
                environ["SCRIPT_NAME"] = _merge_script_name(script_name, effective_base)
                trimmed = path_info[len(effective_base) :]
                environ["PATH_INFO"] = trimmed or "/"
            else:
                # Proxy already stripped prefix; still publish it for url_for generation.
                environ["SCRIPT_NAME"] = _merge_script_name(script_name, effective_base)

        return self.wrapped_app(environ, start_response)


app.wsgi_app = BasePathMiddleware(app.wsgi_app)  # type: ignore[method-assign]

DATA_DIR = os.environ.get("SOBS_DATA_DIR", os.path.join(os.path.dirname(__file__), "data"))
DB_PATH = os.path.join(DATA_DIR, "sobs.db")
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
CREATE TABLE IF NOT EXISTS logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    ts          TEXT    NOT NULL,
    level       TEXT    NOT NULL DEFAULT 'INFO',
    service     TEXT    NOT NULL DEFAULT '',
    body        BLOB    NOT NULL,          -- zlib-compressed UTF-8 message
    attrs       BLOB,                      -- zlib-compressed JSON attributes
    trace_id    TEXT    DEFAULT '',
    span_id     TEXT    DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_logs_ts      ON logs(ts);
CREATE INDEX IF NOT EXISTS idx_logs_level   ON logs(level);
CREATE INDEX IF NOT EXISTS idx_logs_service ON logs(service);

CREATE TABLE IF NOT EXISTS errors (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    ts          TEXT    NOT NULL,
    service     TEXT    NOT NULL DEFAULT '',
    err_type    TEXT    NOT NULL DEFAULT '',
    message     TEXT    NOT NULL DEFAULT '',
    stack       BLOB,                      -- zlib-compressed stack trace
    attrs       BLOB,                      -- zlib-compressed JSON
    trace_id    TEXT    DEFAULT '',
    span_id     TEXT    DEFAULT '',
    resolved    INTEGER DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_errors_ts      ON errors(ts);
CREATE INDEX IF NOT EXISTS idx_errors_service ON errors(service);

CREATE TABLE IF NOT EXISTS spans (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    ts              TEXT    NOT NULL,
    trace_id        TEXT    NOT NULL DEFAULT '',
    span_id         TEXT    NOT NULL DEFAULT '',
    parent_span_id  TEXT    DEFAULT '',
    name            TEXT    NOT NULL DEFAULT '',
    service         TEXT    NOT NULL DEFAULT '',
    duration_ms     REAL    DEFAULT 0,
    status          TEXT    DEFAULT 'OK',
    attrs           BLOB                       -- zlib-compressed JSON
);
CREATE INDEX IF NOT EXISTS idx_spans_ts       ON spans(ts);
CREATE INDEX IF NOT EXISTS idx_spans_trace_id ON spans(trace_id);
CREATE INDEX IF NOT EXISTS idx_spans_service  ON spans(service);

CREATE TABLE IF NOT EXISTS rum_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    ts          TEXT    NOT NULL,
    session_id  TEXT    NOT NULL DEFAULT '',
    event_type  TEXT    NOT NULL DEFAULT '',
    url         TEXT    DEFAULT '',
    data        BLOB                           -- zlib-compressed JSON
);
CREATE INDEX IF NOT EXISTS idx_rum_ts         ON rum_events(ts);
CREATE INDEX IF NOT EXISTS idx_rum_session    ON rum_events(session_id);
CREATE INDEX IF NOT EXISTS idx_rum_event_type ON rum_events(event_type);

CREATE TABLE IF NOT EXISTS ai_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    ts          TEXT    NOT NULL,
    service     TEXT    NOT NULL DEFAULT '',
    provider    TEXT    NOT NULL DEFAULT '',
    model       TEXT    NOT NULL DEFAULT '',
    prompt      BLOB,                          -- zlib-compressed
    response    BLOB,                          -- zlib-compressed
    tokens_in   INTEGER DEFAULT 0,
    tokens_out  INTEGER DEFAULT 0,
    duration_ms REAL    DEFAULT 0,
    trace_id    TEXT    DEFAULT '',
    span_id     TEXT    DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_ai_ts      ON ai_events(ts);
CREATE INDEX IF NOT EXISTS idx_ai_service ON ai_events(service);
"""


def get_db():
    if "db" not in g:
        conn = sqlite3.connect(DB_PATH, detect_types=sqlite3.PARSE_DECLTYPES)
        conn.row_factory = sqlite3.Row
        conn.execute("PRAGMA journal_mode=WAL")
        conn.execute("PRAGMA synchronous=NORMAL")
        g.db = conn
    return g.db


@app.teardown_appcontext
def close_db(exc=None):
    db = g.pop("db", None)
    if db is not None:
        db.close()


def init_db():
    with app.app_context():
        db = get_db()
        db.executescript(SCHEMA)
        db.commit()


def ensure_db_schema():
    """Create schema if the active DB is empty or tables are missing."""
    db = get_db()
    try:
        has_logs_table = db.execute("SELECT 1 FROM sqlite_master WHERE type='table' AND name='logs'").fetchone()
    except sqlite3.OperationalError:
        has_logs_table = None

    if has_logs_table is None:
        db.executescript(SCHEMA)
        db.commit()


# Initialize schema at import time so WSGI/sidecar startups are covered.
init_db()


# ---------------------------------------------------------------------------
# Compression helpers
# ---------------------------------------------------------------------------
def compress(text: str) -> bytes:
    return zlib.compress(text.encode("utf-8"), level=9)


def decompress(data: bytes) -> str:
    if data is None:
        return ""
    return zlib.decompress(data).decode("utf-8")


def compress_json(obj) -> bytes:
    return compress(json.dumps(obj, ensure_ascii=False))


def decompress_json(data: bytes):
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
    def decorated(*args, **kwargs):
        if API_KEY:
            key = request.headers.get("X-API-Key") or request.args.get("api_key")
            if key != API_KEY:
                return jsonify({"error": "Unauthorized"}), 401
        return f(*args, **kwargs)

    return decorated


# ---------------------------------------------------------------------------
# Auth decorator (optional Basic Auth for Web UI)
# ---------------------------------------------------------------------------
def require_basic_auth(f):
    @wraps(f)
    def decorated(*args, **kwargs):
        mode = _auth_mode()
        if mode == "invalid":
            return jsonify({"error": "Server auth misconfiguration"}), 500
        if mode == "none":
            return f(*args, **kwargs)
        auth = request.headers.get("Authorization", "")
        # Accept valid HTTP Basic credentials when configured.
        if mode == "basic" and auth.startswith("Basic "):
            try:
                decoded = base64.b64decode(auth[6:], validate=True).decode("utf-8")
                username, _, password = decoded.partition(":")
                user_ok = secrets.compare_digest(username, BASIC_AUTH_USERNAME)
                pass_ok = secrets.compare_digest(password, BASIC_AUTH_PASSWORD)
                if user_ok and pass_ok:
                    return f(*args, **kwargs)
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
            if auth.startswith("Bearer ") and _check_external_auth(auth):
                return f(*args, **kwargs)
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


@app.before_request
def ensure_schema_before_request():
    # Fallback guard for runtimes where DB files appear after startup.
    ensure_db_schema()


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


# ---------------------------------------------------------------------------
# OTLP Ingest – Logs  POST /v1/logs
# ---------------------------------------------------------------------------
@app.route("/v1/logs", methods=["POST"])
@require_api_key
def ingest_logs():
    payload = request.get_json(force=True, silent=True) or {}
    db = get_db()
    count = 0
    for resource_log in payload.get("resourceLogs", []):
        resource_attrs = _attr_list_to_dict(resource_log.get("resource", {}).get("attributes", []))
        service = resource_attrs.get("service.name", "")
        for scope_log in resource_log.get("scopeLogs", []):
            for record in scope_log.get("logRecords", []):
                ts = _ns_to_iso(int(record.get("timeUnixNano", 0)))
                level = record.get("severityText", "INFO").upper()
                body_val = record.get("body", {})
                body_str = body_val.get("stringValue", str(body_val)) if isinstance(body_val, dict) else str(body_val)
                record_attrs = _attr_list_to_dict(record.get("attributes", []))
                merged_attrs = {**resource_attrs, **record_attrs}
                trace_id = _hex(record.get("traceId", ""))
                span_id = _hex(record.get("spanId", ""))
                db.execute(
                    "INSERT INTO logs(ts, level, service, body, attrs, trace_id, span_id) VALUES(?,?,?,?,?,?,?)",
                    (ts, level, service, compress(body_str), compress_json(merged_attrs), trace_id, span_id),
                )
                count += 1
    db.commit()
    return jsonify({"accepted": count}), 200


# ---------------------------------------------------------------------------
# OTLP Ingest – Traces  POST /v1/traces
# ---------------------------------------------------------------------------
@app.route("/v1/traces", methods=["POST"])
@require_api_key
def ingest_traces():
    payload = request.get_json(force=True, silent=True) or {}
    db = get_db()
    count = 0
    for resource_span in payload.get("resourceSpans", []):
        resource_attrs = _attr_list_to_dict(resource_span.get("resource", {}).get("attributes", []))
        service = resource_attrs.get("service.name", "")
        for scope_span in resource_span.get("scopeSpans", []):
            for span in scope_span.get("spans", []):
                ts = _ns_to_iso(int(span.get("startTimeUnixNano", 0)))
                trace_id = _hex(span.get("traceId", ""))
                span_id = _hex(span.get("spanId", ""))
                parent_id = _hex(span.get("parentSpanId", ""))
                name = span.get("name", "")
                start_ns = int(span.get("startTimeUnixNano", 0))
                end_ns = int(span.get("endTimeUnixNano", 0))
                duration_ms = (end_ns - start_ns) / 1_000_000 if end_ns > start_ns else 0
                status = span.get("status", {}).get("code", "STATUS_CODE_OK")
                if isinstance(status, int):
                    status = "OK" if status == 1 else ("ERROR" if status == 2 else "UNSET")
                span_attrs = _attr_list_to_dict(span.get("attributes", []))
                merged_attrs = {**resource_attrs, **span_attrs}
                db.execute(
                    "INSERT INTO spans(ts, trace_id, span_id, parent_span_id, name, service, duration_ms, status, attrs) "  # noqa: E501
                    "VALUES(?,?,?,?,?,?,?,?,?)",
                    (
                        ts,
                        trace_id,
                        span_id,
                        parent_id,
                        name,
                        service,
                        duration_ms,
                        str(status),
                        compress_json(merged_attrs),
                    ),
                )
                count += 1
                # Detect errors from span status
                if "ERROR" in str(status).upper():
                    err_msg = span_attrs.get("exception.message", span_attrs.get("error.message", name))
                    err_type = span_attrs.get("exception.type", "SpanError")
                    stack = span_attrs.get("exception.stacktrace", "")
                    db.execute(
                        "INSERT INTO errors(ts, service, err_type, message, stack, attrs, trace_id, span_id) "
                        "VALUES(?,?,?,?,?,?,?,?)",
                        (
                            ts,
                            service,
                            err_type,
                            err_msg,
                            compress(stack) if stack else None,
                            compress_json(merged_attrs),
                            trace_id,
                            span_id,
                        ),
                    )
    db.commit()
    return jsonify({"accepted": count}), 200


# ---------------------------------------------------------------------------
# OTLP Ingest – Metrics  POST /v1/metrics  (stored as logs for simplicity)
# ---------------------------------------------------------------------------
@app.route("/v1/metrics", methods=["POST"])
@require_api_key
def ingest_metrics():
    payload = request.get_json(force=True, silent=True) or {}
    db = get_db()
    count = 0
    for resource_metric in payload.get("resourceMetrics", []):
        resource_attrs = _attr_list_to_dict(resource_metric.get("resource", {}).get("attributes", []))
        service = resource_attrs.get("service.name", "metrics")
        for scope_metric in resource_metric.get("scopeMetrics", []):
            for metric in scope_metric.get("metrics", []):
                name = metric.get("name", "")
                ts = _now_iso()
                body = f"METRIC {name}"
                db.execute(
                    "INSERT INTO logs(ts, level, service, body, attrs, trace_id, span_id) VALUES(?,?,?,?,?,?,?)",
                    (ts, "METRIC", service, compress(body), compress_json({**resource_attrs, "metric": name}), "", ""),
                )
                count += 1
    db.commit()
    return jsonify({"accepted": count}), 200


# ---------------------------------------------------------------------------
# RUM Ingest  POST /v1/rum
# ---------------------------------------------------------------------------
@app.route("/v1/rum", methods=["POST"])
@require_api_key
def ingest_rum():
    payload = request.get_json(force=True, silent=True)
    if payload is None:
        payload = {}
    if isinstance(payload, list):
        events = payload
    else:
        events = payload.get("events", [payload])
    db = get_db()
    count = 0
    for event in events:
        ts = event.get("timestamp", _now_iso())
        session_id = event.get("sessionId", "")
        event_type = event.get("type", "unknown")
        url = event.get("url", "")
        db.execute(
            "INSERT INTO rum_events(ts, session_id, event_type, url, data) VALUES(?,?,?,?,?)",
            (ts, session_id, event_type, url, compress_json(event)),
        )
        count += 1
        # Persist JS errors into the errors table too
        if event_type in ("error", "unhandledrejection"):
            db.execute(
                "INSERT INTO errors(ts, service, err_type, message, stack, attrs, trace_id, span_id) VALUES(?,?,?,?,?,?,?,?)",  # noqa: E501
                (
                    ts,
                    "rum",
                    event.get("errorType", "JSError"),
                    event.get("message", ""),
                    compress(event.get("stack", "")) if event.get("stack") else None,
                    compress_json({"url": url, "sessionId": session_id}),
                    "",
                    "",
                ),
            )
    db.commit()
    return jsonify({"accepted": count}), 200


# ---------------------------------------------------------------------------
# AI Transparency  POST /v1/ai
# ---------------------------------------------------------------------------
@app.route("/v1/ai", methods=["POST"])
@require_api_key
def ingest_ai():
    payload = request.get_json(force=True, silent=True) or {}
    db = get_db()
    ts = payload.get("timestamp", _now_iso())
    db.execute(
        "INSERT INTO ai_events(ts, service, provider, model, prompt, response, "
        "tokens_in, tokens_out, duration_ms, trace_id, span_id) VALUES(?,?,?,?,?,?,?,?,?,?,?)",
        (
            ts,
            payload.get("service", ""),
            payload.get("provider", ""),
            payload.get("model", ""),
            compress(payload.get("prompt", "")) if payload.get("prompt") else None,
            compress(payload.get("response", "")) if payload.get("response") else None,
            payload.get("tokens_in", 0),
            payload.get("tokens_out", 0),
            payload.get("duration_ms", 0),
            payload.get("trace_id", ""),
            payload.get("span_id", ""),
        ),
    )
    db.commit()
    return jsonify({"ok": True}), 200


# ---------------------------------------------------------------------------
# Error ingest  POST /v1/errors  (direct error submission)
# ---------------------------------------------------------------------------
@app.route("/v1/errors", methods=["POST"])
@require_api_key
def ingest_errors():
    payload = request.get_json(force=True, silent=True) or {}
    db = get_db()
    ts = payload.get("timestamp", _now_iso())
    db.execute(
        "INSERT INTO errors(ts, service, err_type, message, stack, attrs, trace_id, span_id) VALUES(?,?,?,?,?,?,?,?)",
        (
            ts,
            payload.get("service", ""),
            payload.get("type", "Error"),
            payload.get("message", ""),
            compress(payload.get("stack", "")) if payload.get("stack") else None,
            compress_json(payload.get("attributes", {})),
            payload.get("trace_id", ""),
            payload.get("span_id", ""),
        ),
    )
    db.commit()
    return jsonify({"ok": True}), 200


# ---------------------------------------------------------------------------
# Web UI – Dashboard
# ---------------------------------------------------------------------------
@app.route("/")
@require_basic_auth
def dashboard():
    db = get_db()
    stats = {
        "logs": db.execute("SELECT COUNT(*) FROM logs").fetchone()[0],
        "errors": db.execute("SELECT COUNT(*) FROM errors WHERE resolved=0").fetchone()[0],
        "errors_total": db.execute("SELECT COUNT(*) FROM errors").fetchone()[0],
        "spans": db.execute("SELECT COUNT(*) FROM spans").fetchone()[0],
        "rum": db.execute("SELECT COUNT(*) FROM rum_events").fetchone()[0],
        "ai": db.execute("SELECT COUNT(*) FROM ai_events").fetchone()[0],
        "services": [
            r[0]
            for r in db.execute(
                "SELECT DISTINCT service FROM logs WHERE service!='' "
                "UNION SELECT DISTINCT service FROM spans WHERE service!='' "
                "UNION SELECT DISTINCT service FROM errors WHERE service!=''"
            ).fetchall()
        ],
    }
    # Recent errors (last 5)
    recent_errors = [
        dict(r)
        for r in db.execute(
            "SELECT id, ts, service, err_type, message FROM errors WHERE resolved=0 ORDER BY ts DESC LIMIT 5"
        ).fetchall()
    ]
    # Recent logs (last 10)
    recent_logs = []
    for r in db.execute("SELECT ts, level, service, body FROM logs ORDER BY ts DESC LIMIT 10").fetchall():
        recent_logs.append(
            {
                "ts": r["ts"],
                "level": r["level"],
                "service": r["service"],
                "body": decompress(r["body"]),
            }
        )
    # RUM summary – page views last 24h
    rum_summary = db.execute(
        "SELECT event_type, COUNT(*) as cnt FROM rum_events GROUP BY event_type ORDER BY cnt DESC"
    ).fetchall()
    # AI summary
    ai_summary = db.execute(
        "SELECT model, COUNT(*) cnt, SUM(tokens_in) ti, SUM(tokens_out) to_ FROM ai_events GROUP BY model"
    ).fetchall()
    return render_template(
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
def view_logs():
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
            query = f"SELECT ts, level, service, body, trace_id, span_id FROM logs WHERE {safe_sql} ORDER BY ts DESC LIMIT ? OFFSET ?"  # noqa: E501
            rows = db.execute(query, (limit, offset)).fetchall()
            total = db.execute(f"SELECT COUNT(*) FROM logs WHERE {safe_sql}").fetchone()[0]
        except Exception as exc:
            error_msg = f"SQL error: {exc}"
            rows = []
    else:
        conditions = []
        params = []
        if level:
            conditions.append("level=?")
            params.append(level)
        if service:
            conditions.append("service=?")
            params.append(service)
        where = ("WHERE " + " AND ".join(conditions)) if conditions else ""
        total = db.execute(f"SELECT COUNT(*) FROM logs {where}", params).fetchone()[0]
        rows = db.execute(
            f"SELECT ts, level, service, body, trace_id, span_id FROM logs {where} ORDER BY ts DESC LIMIT ? OFFSET ?",
            params + [limit, offset],
        ).fetchall()

    log_rows = []
    grep_pat = re.compile(q, re.IGNORECASE) if q else None
    for r in rows:
        body = decompress(r["body"])
        if grep_pat and not grep_pat.search(body):
            continue
        log_rows.append(
            {
                "ts": r["ts"],
                "level": r["level"],
                "service": r["service"],
                "body": body,
                "trace_id": r["trace_id"],
                "span_id": r["span_id"],
            }
        )

    services = [
        row[0] for row in db.execute("SELECT DISTINCT service FROM logs WHERE service!='' ORDER BY service").fetchall()
    ]
    levels = [row[0] for row in db.execute("SELECT DISTINCT level FROM logs ORDER BY level").fetchall()]

    return render_template(
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
def view_errors():
    db = get_db()
    service = request.args.get("service", "").strip()
    resolved = request.args.get("resolved", "0").strip()
    limit = _parse_limit(100)
    offset = _parse_offset()

    conditions = []
    params = []
    if service:
        conditions.append("service=?")
        params.append(service)
    if resolved in ("0", "1"):
        conditions.append("resolved=?")
        params.append(int(resolved))
    where = ("WHERE " + " AND ".join(conditions)) if conditions else ""

    total = db.execute(f"SELECT COUNT(*) FROM errors {where}", params).fetchone()[0]
    rows = db.execute(
        f"SELECT id, ts, service, err_type, message, stack, trace_id, resolved FROM errors {where} ORDER BY ts DESC LIMIT ? OFFSET ?",  # noqa: E501
        params + [limit, offset],
    ).fetchall()

    errors = []
    for r in rows:
        errors.append(
            {
                "id": r["id"],
                "ts": r["ts"],
                "service": r["service"],
                "err_type": r["err_type"],
                "message": r["message"],
                "stack": decompress(r["stack"]) if r["stack"] else "",
                "trace_id": r["trace_id"],
                "resolved": bool(r["resolved"]),
            }
        )

    services = [
        row[0]
        for row in db.execute("SELECT DISTINCT service FROM errors WHERE service!='' ORDER BY service").fetchall()
    ]

    return render_template(
        "errors.html",
        errors=errors,
        total=total,
        limit=limit,
        offset=offset,
        service=service,
        resolved=resolved,
        services=services,
    )


@app.route("/errors/<int:error_id>/resolve", methods=["POST"])
@require_basic_auth
def resolve_error(error_id: int):
    db = get_db()
    db.execute("UPDATE errors SET resolved=1 WHERE id=?", (error_id,))
    db.commit()
    return jsonify({"ok": True})


# ---------------------------------------------------------------------------
# Web UI – Traces
# ---------------------------------------------------------------------------
@app.route("/traces")
@require_basic_auth
def view_traces():
    db = get_db()
    service = request.args.get("service", "").strip()
    trace_id = request.args.get("trace_id", "").strip()
    limit = _parse_limit(100)
    offset = _parse_offset()

    conditions = []
    params = []
    if service:
        conditions.append("service=?")
        params.append(service)
    if trace_id:
        conditions.append("trace_id=?")
        params.append(trace_id)
    where = ("WHERE " + " AND ".join(conditions)) if conditions else ""

    total = db.execute(f"SELECT COUNT(*) FROM spans {where}", params).fetchone()[0]
    rows = db.execute(
        f"SELECT ts, trace_id, span_id, parent_span_id, name, service, duration_ms, status, attrs "
        f"FROM spans {where} ORDER BY ts DESC LIMIT ? OFFSET ?",
        params + [limit, offset],
    ).fetchall()

    spans = []
    for r in rows:
        attrs = decompress_json(r["attrs"])
        spans.append(
            {
                "ts": r["ts"],
                "trace_id": r["trace_id"],
                "span_id": r["span_id"],
                "parent_span_id": r["parent_span_id"],
                "name": r["name"],
                "service": r["service"],
                "duration_ms": round(r["duration_ms"], 2),
                "status": r["status"],
                "http_method": attrs.get("http.method", attrs.get("http.request.method", "")),
                "http_url": attrs.get("http.url", attrs.get("url.full", "")),
                "http_status": attrs.get("http.status_code", attrs.get("http.response.status_code", "")),
            }
        )

    services = [
        row[0] for row in db.execute("SELECT DISTINCT service FROM spans WHERE service!='' ORDER BY service").fetchall()
    ]

    return render_template(
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
def view_rum():
    db = get_db()
    event_type = request.args.get("type", "").strip()
    limit = _parse_limit(200)
    offset = _parse_offset()

    conditions = []
    params = []
    if event_type:
        conditions.append("event_type=?")
        params.append(event_type)
    where = ("WHERE " + " AND ".join(conditions)) if conditions else ""

    total = db.execute(f"SELECT COUNT(*) FROM rum_events {where}", params).fetchone()[0]
    rows = db.execute(
        f"SELECT ts, session_id, event_type, url, data FROM rum_events {where} ORDER BY ts DESC LIMIT ? OFFSET ?",
        params + [limit, offset],
    ).fetchall()

    events = []
    for r in rows:
        data = decompress_json(r["data"])
        events.append(
            {
                "ts": r["ts"],
                "session_id": r["session_id"][:8] if r["session_id"] else "",
                "event_type": r["event_type"],
                "url": r["url"],
                "data": data,
            }
        )

    event_types = [
        row[0] for row in db.execute("SELECT DISTINCT event_type FROM rum_events ORDER BY event_type").fetchall()
    ]

    # Web vitals summary
    vitals_rows = db.execute(
        "SELECT data FROM rum_events WHERE event_type='web-vital' ORDER BY ts DESC LIMIT 500"
    ).fetchall()
    vitals = {}
    for vr in vitals_rows:
        d = decompress_json(vr["data"])
        name = d.get("name", "")
        val = d.get("value")
        if name and val is not None:
            vitals.setdefault(name, []).append(val)
    vitals_summary = {}
    for name, vals in vitals.items():
        vitals_summary[name] = {
            "avg": round(sum(vals) / len(vals), 1),
            "p75": round(sorted(vals)[int(len(vals) * 0.75)], 1),
            "count": len(vals),
        }

    return render_template(
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
def view_ai():
    db = get_db()
    service = request.args.get("service", "").strip()
    model = request.args.get("model", "").strip()
    limit = _parse_limit(50)
    offset = _parse_offset()

    conditions = []
    params = []
    if service:
        conditions.append("service=?")
        params.append(service)
    if model:
        conditions.append("model=?")
        params.append(model)
    where = ("WHERE " + " AND ".join(conditions)) if conditions else ""

    total = db.execute(f"SELECT COUNT(*) FROM ai_events {where}", params).fetchone()[0]
    rows = db.execute(
        f"SELECT id, ts, service, provider, model, prompt, response, tokens_in, tokens_out, duration_ms, trace_id "
        f"FROM ai_events {where} ORDER BY ts DESC LIMIT ? OFFSET ?",
        params + [limit, offset],
    ).fetchall()

    ai_items = []
    for r in rows:
        ai_items.append(
            {
                "id": r["id"],
                "ts": r["ts"],
                "service": r["service"],
                "provider": r["provider"],
                "model": r["model"],
                "prompt": decompress(r["prompt"]) if r["prompt"] else "",
                "response": decompress(r["response"]) if r["response"] else "",
                "tokens_in": r["tokens_in"],
                "tokens_out": r["tokens_out"],
                "duration_ms": round(r["duration_ms"], 1),
                "trace_id": r["trace_id"],
            }
        )

    services = [
        row[0]
        for row in db.execute("SELECT DISTINCT service FROM ai_events WHERE service!='' ORDER BY service").fetchall()
    ]
    models = [
        row[0] for row in db.execute("SELECT DISTINCT model FROM ai_events WHERE model!='' ORDER BY model").fetchall()
    ]

    # Token usage totals
    totals = db.execute("SELECT SUM(tokens_in) ti, SUM(tokens_out) to_, COUNT(*) cnt FROM ai_events").fetchone()

    return render_template(
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
def rum_js():
    return send_from_directory(
        os.path.join(os.path.dirname(__file__), "static"), "rum.js", mimetype="application/javascript"
    )


# ---------------------------------------------------------------------------
# Health check
# ---------------------------------------------------------------------------
@app.route("/health")
def health():
    return jsonify({"status": "ok", "version": "1.0.0"})


# ---------------------------------------------------------------------------
# Entrypoint
# ---------------------------------------------------------------------------
if __name__ == "__main__":
    init_db()
    port = int(os.environ.get("PORT", 4317))
    debug = os.environ.get("FLASK_DEBUG", "0") == "1"
    if debug:
        app.run(host="0.0.0.0", port=port, debug=debug)
    else:
        import multiprocessing

        from gunicorn.app.base import BaseApplication

        class _StandaloneApplication(BaseApplication):
            def __init__(self, wsgi_app, options=None):
                self.options = options or {}
                self.application = wsgi_app
                super().__init__()

            def load_config(self):
                for key, value in self.options.items():
                    self.cfg.set(key.lower(), value)

            def load(self):
                return self.application

        workers = int(os.environ.get("GUNICORN_WORKERS", multiprocessing.cpu_count() * 2 + 1))
        threads = int(os.environ.get("GUNICORN_THREADS", 4))
        _StandaloneApplication(
            app,
            {
                "bind": f"0.0.0.0:{port}",
                "workers": workers,
                "threads": threads,
                "worker_class": "gthread",
            },
        ).run()
