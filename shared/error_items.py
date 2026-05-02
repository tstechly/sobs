from __future__ import annotations

import json
import re
from functools import lru_cache
from typing import Any


def _compact_text(value: str, limit: int = 220) -> str:
    text = re.sub(r"\s+", " ", str(value or "")).strip()
    if len(text) <= limit:
        return text
    return text[: max(0, limit - 1)].rstrip() + "..."


@lru_cache(maxsize=4096)
def _try_pretty_json_text(raw_value: str) -> tuple[bool, str]:
    raw = str(raw_value or "").strip()
    if not raw or raw[:1] not in ("{", "["):
        return False, ""
    try:
        parsed = json.loads(raw)
    except json.JSONDecodeError:
        return False, ""
    return True, json.dumps(parsed, ensure_ascii=False, indent=2)


def _extract_structured_error_summary(message: str, raw_body: str) -> tuple[str, bool]:
    text_keys = {
        "message",
        "error",
        "error_message",
        "errormessage",
        "detail",
        "description",
        "reason",
        "body",
        "msg",
    }
    code_keys = {"code", "status", "status_code", "error_code", "errorcode"}
    type_keys = {"type", "error_type", "exception", "name"}

    def _first_scalar(value: Any, keyset: set[str], depth: int = 0) -> str:
        if depth > 5:
            return ""
        if isinstance(value, dict):
            for key, inner in value.items():
                if str(key).lower() in keyset and isinstance(inner, (str, int, float, bool)):
                    return str(inner).strip()
            for inner in value.values():
                found = _first_scalar(inner, keyset, depth + 1)
                if found:
                    return found
            return ""
        if isinstance(value, list):
            for inner in value:
                found = _first_scalar(inner, keyset, depth + 1)
                if found:
                    return found
            return ""
        return ""

    def _to_summary(parsed: Any) -> str:
        if isinstance(parsed, list):
            parsed = parsed[0] if parsed else {}
        if not isinstance(parsed, dict):
            return ""

        message_text = _first_scalar(parsed, text_keys)
        code_text = _first_scalar(parsed, code_keys)
        type_text = _first_scalar(parsed, type_keys)

        if message_text:
            summary = message_text
            extras = []
            if type_text and type_text.lower() not in summary.lower():
                extras.append(type_text)
            if code_text and code_text.lower() not in summary.lower():
                extras.append("code " + code_text)
            if extras:
                summary = summary + " [" + ", ".join(extras) + "]"
            return _compact_text(summary)
        if type_text and code_text:
            return _compact_text(type_text + " (code " + code_text + ")")
        if type_text:
            return _compact_text(type_text)
        if code_text:
            return _compact_text("code " + code_text)
        return ""

    for candidate in (message, raw_body):
        raw = str(candidate or "").strip()
        if not raw:
            continue
        if raw[:1] not in ("{", "["):
            continue
        try:
            parsed = json.loads(raw)
        except json.JSONDecodeError:
            continue
        summary = _to_summary(parsed)
        if summary:
            return summary, True
        return _compact_text(json.dumps(parsed, ensure_ascii=False)), True

    return _compact_text(message or raw_body), False


def _build_error_item(
    row: dict[str, Any],
    *,
    map_to_dict: Any,
    maybe_demangle_js_stack: Any,
    error_id: Any,
) -> dict[str, Any]:
    attrs = map_to_dict(row.get("LogAttributes"))
    ts = str(row.get("Timestamp", ""))
    service = str(row.get("ServiceName", ""))
    err_type = str(attrs.get("exception.type", "Error"))
    message = str(attrs.get("exception.message", row.get("Body", "")))
    raw_body = str(row.get("Body", ""))
    message_summary, summary_from_json = _extract_structured_error_summary(message, raw_body)
    message_is_json, message_pretty_json = _try_pretty_json_text(message)
    body_is_json, body_pretty_json = _try_pretty_json_text(raw_body)
    stack = maybe_demangle_js_stack(str(attrs.get("exception.stacktrace", "")))
    stack_is_json, stack_pretty_json = _try_pretty_json_text(stack)
    trace_id = str(row.get("TraceId", ""))
    span_id = str(row.get("SpanId", ""))
    eid = error_id(ts, service, err_type, message, trace_id, span_id)
    return {
        "id": eid,
        "ts": ts,
        "service": service,
        "err_type": err_type,
        "message": message,
        "message_summary": message_summary,
        "summary_from_json": summary_from_json,
        "message_is_json": message_is_json,
        "message_pretty_json": message_pretty_json,
        "raw_body": raw_body,
        "raw_body_is_json": body_is_json,
        "raw_body_pretty_json": body_pretty_json,
        "stack": stack,
        "stack_is_json": stack_is_json,
        "stack_pretty_json": stack_pretty_json,
        "trace_id": trace_id,
        "span_id": span_id,
        "url": str(attrs.get("url.full", "")),
        "error_source": str(attrs.get("error.source", "")),
        "page_title": str(attrs.get("browser.page.title", "")),
        "viewport": str(attrs.get("browser.viewport", "")),
        "artifact_type": str(attrs.get("artifact.type", "")),
        "artifact_id": str(attrs.get("artifact.id", "")),
        "artifact_url": str(attrs.get("artifact.url", "")),
        "replay_id": str(attrs.get("replay.id", "")),
        "replay_url": str(attrs.get("replay.url", "")),
    }


def _error_group_key(item: dict[str, Any]) -> tuple[str, str, str]:
    service = re.sub(r"\s+", " ", str(item.get("service", "") or "")).strip().lower()
    err_type = re.sub(r"\s+", " ", str(item.get("err_type", "") or "")).strip().lower()
    message_basis = str(item.get("message_summary") or item.get("message") or "")
    message = re.sub(r"\s+", " ", message_basis).strip().lower()[:220]
    return service, err_type, message
