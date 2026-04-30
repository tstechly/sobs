"""Shared AI action token and UI action helper logic."""

from __future__ import annotations

import base64
import hashlib
import json
import re
from typing import Any


def _ai_action_token_secret(secret_key: str | None) -> str:
    return str(secret_key or "sobs-dev-secret-key")


def _encode_ai_action_token(payload: dict[str, Any], *, ai_action_token_secret: str) -> str:
    body = json.dumps(payload, separators=(",", ":"), sort_keys=True, ensure_ascii=False).encode("utf-8")
    body_b64 = base64.urlsafe_b64encode(body).decode("ascii").rstrip("=")
    sig = hashlib.sha256((ai_action_token_secret + "." + body_b64).encode("utf-8")).hexdigest()
    return f"{body_b64}.{sig}"


def _decode_ai_action_token(
    token: str,
    *,
    ai_action_token_secret: str,
    compare_digest,
    now: int,
) -> dict[str, Any] | None:
    token = str(token or "").strip()
    if not token or "." not in token:
        return None
    body_b64, sig = token.rsplit(".", 1)
    expected = hashlib.sha256((ai_action_token_secret + "." + body_b64).encode("utf-8")).hexdigest()
    if not compare_digest(sig, expected):
        return None
    padded = body_b64 + "=" * (-len(body_b64) % 4)
    try:
        decoded = base64.urlsafe_b64decode(padded.encode("ascii")).decode("utf-8")
        payload = json.loads(decoded)
    except Exception:
        return None
    if not isinstance(payload, dict):
        return None
    exp = int(payload.get("exp") or 0)
    if exp <= now:
        return None
    return payload


def _issue_ai_action_token(
    *,
    action_id: str,
    target_page: str,
    action: dict[str, Any],
    requires_confirmation: bool,
    chat_id: str,
    turn_id: str,
    now: int,
    ai_action_token_ttl_seconds: int,
    encode_ai_action_token,
) -> str:
    payload = {
        "v": 1,
        "iat": now,
        "exp": now + ai_action_token_ttl_seconds,
        "action_id": action_id,
        "target_page": target_page,
        "action": action,
        "requires_confirmation": requires_confirmation,
        "chat_id": chat_id,
        "turn_id": turn_id,
    }
    return encode_ai_action_token(payload)


def _build_client_action(action_type: str, action_payload: dict[str, Any]) -> dict[str, Any] | None:
    if not action_type:
        return None
    if not isinstance(action_payload, dict):
        return None

    def _sanitize_value(value: Any, depth: int = 0, max_depth: int = 3) -> Any:
        if depth > max_depth:
            return None
        if value is None or isinstance(value, bool):
            return value
        if isinstance(value, (int, float)):
            return value
        if isinstance(value, str):
            text = str(value).strip()
            if len(text) > 4096:
                return text[:4096]
            return text
        if isinstance(value, dict):
            cleaned: dict[str, Any] = {}
            for key, nested_value in value.items():
                if len(cleaned) >= 50:
                    break
                clean_key = str(key or "").strip()
                if not clean_key:
                    continue
                cleaned[clean_key] = _sanitize_value(nested_value, depth + 1, max_depth)
            return cleaned
        if isinstance(value, (list, tuple)):
            sanitized: list[Any] = []
            for item in value:
                if len(sanitized) >= 100:
                    break
                sanitized.append(_sanitize_value(item, depth + 1, max_depth))
            return sanitized
        return None

    sanitized_payload: dict[str, Any] = {}
    for key, value in action_payload.items():
        if len(sanitized_payload) >= 50:
            break
        clean_key = str(key or "").strip()
        if not clean_key:
            continue
        sanitized_payload[clean_key] = _sanitize_value(value)

    return {
        "type": action_type,
        **sanitized_payload,
    }


def _normalize_generic_ui_action_tool_call(
    args: dict[str, Any],
    current_page: str,
    *,
    helper_action_manifest_for_page,
    build_client_action,
) -> dict[str, Any] | None:
    action_id = str(args.get("action_id") or "").strip()
    if not action_id:
        return None

    template_manifest = {item.get("action_id"): item for item in helper_action_manifest_for_page(current_page)}
    template_action = template_manifest.get(action_id) or {}
    template_args_pre = (template_action.get("arguments") or {}) if isinstance(template_action, dict) else {}
    explicit_target = str(args.get("target_page") or "").strip()
    default_target = str(template_args_pre.get("target_page") or "").strip()
    target_page = explicit_target or default_target or str(current_page or "").strip() or current_page
    action_arguments = (args.get("arguments") or {}) if isinstance(args.get("arguments") or {}, dict) else {}
    notes = str(args.get("notes") or "").strip()

    action_meta = template_manifest.get(action_id)
    if not action_meta:
        target_manifest = {item.get("action_id"): item for item in helper_action_manifest_for_page(target_page)}
        action_meta = target_manifest.get(action_id)

    if not action_meta:
        return {
            "tool": "propose_ui_action",
            "action_id": action_id,
            "summary": notes or f"Unsupported action: {action_id}",
            "requires_confirmation": True,
            "unsupported": True,
            "action": {
                "type": "unsupported",
                "action_id": action_id,
                "target_page": target_page,
            },
        }

    action_type = str(action_meta.get("action_type") or "").strip().lower()
    requires_confirmation = target_page != current_page or bool(action_meta.get("requires_confirmation", True))
    template_args = (action_meta.get("arguments") or {}) if isinstance(action_meta.get("arguments") or {}, dict) else {}

    if action_type == "apply_form_filters":
        requested_filters = (
            (action_arguments.get("filters") or {}) if isinstance(action_arguments.get("filters") or {}, dict) else {}
        )
        allowed_filter_values = template_args.get("filter_fields") or []
        allowed_filters = {str(item or "").strip() for item in allowed_filter_values if str(item or "").strip()}
        if allowed_filters and requested_filters:
            filtered_filters = {
                key: value for key, value in requested_filters.items() if str(key or "").strip() in allowed_filters
            }
            if not filtered_filters:
                return {
                    "tool": "propose_ui_action",
                    "action_id": action_id,
                    "summary": notes or "Requested filters are not available on this page",
                    "requires_confirmation": False,
                    "unsupported": True,
                    "action": {
                        "type": "unsupported",
                        "action_id": action_id,
                        "target_page": target_page,
                    },
                }
            action_arguments = {
                **action_arguments,
                "filters": filtered_filters,
            }

    if action_type == "apply_sql_filter":
        sql_where = str(action_arguments.get("sql_where") or "").strip()
        if not sql_where:
            for alt_key in ("sql", "where", "filter", "expression", "query"):
                candidate = action_arguments.get(alt_key)
                if isinstance(candidate, str) and candidate.strip():
                    sql_where = candidate.strip()
                    break
                if isinstance(candidate, dict):
                    nested = str(
                        candidate.get("sql_where") or candidate.get("sql") or candidate.get("where") or ""
                    ).strip()
                    if nested:
                        sql_where = nested
                        break
        if not sql_where and notes:
            note_sql_match = re.search(r"\bwith\s+sql\s+(.+)$", notes, re.IGNORECASE)
            if note_sql_match:
                sql_where = str(note_sql_match.group(1) or "").strip()
        if sql_where:
            action_arguments = {
                **action_arguments,
                "sql_where": sql_where,
            }

    action_payload = {
        "target_page": target_page,
        **action_arguments,
    }
    for key, default_value in template_args.items():
        if key not in action_payload:
            action_payload[key] = default_value

    client_action = build_client_action(action_type, action_payload)
    if not client_action:
        return {
            "tool": "propose_ui_action",
            "action_id": action_id,
            "summary": notes or f"Invalid arguments for action: {action_id}",
            "requires_confirmation": True,
            "unsupported": True,
            "action": {
                "type": "unsupported",
                "action_id": action_id,
                "target_page": target_page,
            },
        }

    return {
        "tool": "propose_ui_action",
        "action_id": action_id,
        "summary": notes or str(action_meta.get("label") or action_id),
        "requires_confirmation": requires_confirmation,
        "unsupported": not bool(action_meta.get("implemented", False)),
        "action": client_action,
    }


def _suggest_chart_dashboard_pivot_tool(
    question: str,
    current_page: str,
    *,
    ai_chart_request_keywords,
    normalize_generic_ui_action_tool_call,
) -> dict[str, Any] | None:
    lower_question = str(question or "").strip().lower()
    if not lower_question:
        return None
    if not any(keyword in lower_question for keyword in ai_chart_request_keywords):
        return None
    if current_page.startswith("/dashboards"):
        return None
    if "ai" not in lower_question and "trace" not in lower_question and "response" not in lower_question:
        return None
    return normalize_generic_ui_action_tool_call(
        {
            "action_id": "dashboards.modal.new.open",
            "target_page": "/dashboards",
            "arguments": {},
            "notes": "Open the new dashboard modal to create the requested chart",
        },
        current_page,
    )
