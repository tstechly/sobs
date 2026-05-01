from __future__ import annotations

import hashlib
import json
from typing import Any, Callable


def _rum_session_key_from_attrs(attrs: dict[str, str], ts: str, body_raw: str) -> str:
    session_id = str(attrs.get("sessionId", attrs.get("session.id", ""))).strip()
    if session_id:
        return session_id
    return f"anon:{hashlib.md5(f'{ts}|{body_raw}'.encode('utf-8')).hexdigest()[:16]}"


def _build_rum_event_item(
    row: Any,
    *,
    map_to_dict: Callable[[Any], dict[str, Any]],
) -> dict[str, Any]:
    attrs = map_to_dict(row["LogAttributes"])
    body_raw = str(row["Body"] or "")
    try:
        body_data = json.loads(body_raw) if body_raw else {}
    except json.JSONDecodeError:
        body_data = {}

    data = body_data if isinstance(body_data, dict) else {"value": body_data}
    keys = set(row.keys()) if hasattr(row, "keys") else set()
    trace_id = str(row["TraceId"]) if "TraceId" in keys else str(data.get("traceId", ""))
    span_id = str(row["SpanId"]) if "SpanId" in keys else str(data.get("spanId", ""))
    service = str(row["ServiceName"]) if "ServiceName" in keys else str(data.get("service", "") or "")
    if trace_id and not data.get("traceId"):
        data["traceId"] = trace_id
    if span_id and not data.get("spanId"):
        data["spanId"] = span_id

    ts = str(row["Timestamp"])
    session_key = _rum_session_key_from_attrs(attrs, ts, body_raw)
    artifact_raw = data.get("artifact")
    replay_raw = data.get("replay")
    artifact: dict[str, Any] = artifact_raw if isinstance(artifact_raw, dict) else {}
    replay: dict[str, Any] = replay_raw if isinstance(replay_raw, dict) else {}
    return {
        "ts": ts,
        "session_key": session_key,
        "session_id": session_key[:8],
        "event_type": str(row["EventName"]),
        "url": str(attrs.get("url", attrs.get("url.full", ""))),
        "data": data,
        "trace_id": trace_id,
        "span_id": span_id,
        "service": service,
        "has_artifact": bool(artifact.get("url") or artifact.get("id")),
        "has_replay": bool(replay.get("url") or replay.get("id")),
    }
