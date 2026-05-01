from __future__ import annotations

import json
import re
from typing import Any, cast


def _genai_tool_calls_to_text(tool_calls_value: Any) -> str:
    if not isinstance(tool_calls_value, list):
        return ""
    chunks: list[str] = []
    for item in tool_calls_value:
        if not isinstance(item, dict):
            continue
        function_value = item.get("function")
        function: dict[str, Any] = function_value if isinstance(function_value, dict) else {}
        name = str(item.get("name") or function.get("name") or "").strip()
        arguments = item.get("arguments")
        if arguments in (None, "", [], {}):
            arguments = function.get("arguments")
        label = f"tool_call:{name}" if name else "tool_call"
        if isinstance(arguments, (dict, list)) and arguments:
            chunks.append(f"{label} {json.dumps(arguments, ensure_ascii=False)}")
        elif arguments not in (None, ""):
            chunks.append(f"{label} {arguments}")
        else:
            chunks.append(label)
    return "\n".join(chunks).strip()


def _genai_message_content_to_text(message: dict[str, Any]) -> str:
    content = message.get("content")
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        return " ".join(part.get("text", "") if isinstance(part, dict) else str(part) for part in content).strip()
    if content not in (None, ""):
        return str(content)

    parts_value = message.get("parts")
    if isinstance(parts_value, list):
        chunks: list[str] = []
        for part in parts_value:
            if isinstance(part, str):
                if part:
                    chunks.append(part)
                continue
            if not isinstance(part, dict):
                continue
            part_type = str(part.get("type", "")).strip().lower()
            if part_type in {"text", "reasoning"}:
                text = part.get("content", "") or part.get("text", "")
                if text:
                    chunks.append(str(text))
                continue
            if part_type in {"tool_call", "server_tool_call"}:
                rendered = _genai_tool_calls_to_text([part])
                if rendered:
                    chunks.append(rendered)
                continue
            if part_type in {"tool_call_response", "server_tool_call_response"}:
                response = part.get("response")
                if response:
                    chunks.append(str(response))
                else:
                    chunks.append(part_type)
                continue
            part_content = part.get("content")
            if part_content:
                chunks.append(str(part_content))
                continue
            chunks.append(json.dumps(part, ensure_ascii=False))
        rendered_parts = "\n".join(chunks).strip()
        if rendered_parts:
            return rendered_parts

    tool_calls_text = _genai_tool_calls_to_text(message.get("tool_calls"))
    if tool_calls_text:
        return tool_calls_text

    function_call = message.get("function_call")
    if isinstance(function_call, dict):
        function_text = _genai_tool_calls_to_text([{"function": function_call}])
        if function_text:
            return function_text

    return ""


def _genai_message_reasoning_to_text(message: dict[str, Any]) -> str:
    def _coerce_reasoning_text(value: Any) -> str:
        if value in (None, ""):
            return ""
        if isinstance(value, str):
            return value.strip()
        if isinstance(value, list):
            chunks: list[str] = []
            for item in value:
                if isinstance(item, str):
                    text = item.strip()
                    if text:
                        chunks.append(text)
                    continue
                if isinstance(item, dict):
                    text = str(item.get("text") or item.get("content") or "").strip()
                    if text:
                        chunks.append(text)
                    continue
                text = str(item or "").strip()
                if text:
                    chunks.append(text)
            return "\n".join(chunks).strip()
        if isinstance(value, dict):
            direct = str(value.get("text") or value.get("content") or "").strip()
            if direct:
                return direct
            return json.dumps(value, ensure_ascii=False)
        return str(value).strip()

    for key in ("reasoning_content", "reasoning", "thinking"):
        text = _coerce_reasoning_text(message.get(key))
        if text:
            return text

    parts_value = message.get("parts")
    if isinstance(parts_value, list):
        reasoning_chunks: list[str] = []
        for part in parts_value:
            if not isinstance(part, dict):
                continue
            if str(part.get("type") or "").strip().lower() != "reasoning":
                continue
            text = _coerce_reasoning_text(part.get("content") or part.get("text"))
            if text:
                reasoning_chunks.append(text)
        if reasoning_chunks:
            return "\n".join(reasoning_chunks).strip()

    return ""


def _parse_genai_messages_json(messages_str: str) -> list[Any] | None:
    if not messages_str:
        return []
    try:
        parsed = json.loads(messages_str)
    except (json.JSONDecodeError, TypeError):
        return None
    if isinstance(parsed, list):
        return parsed
    if isinstance(parsed, dict):
        for key in ("messages", "input_messages", "output_messages", "items"):
            nested = parsed.get(key)
            if isinstance(nested, list):
                return nested
    return []


def _extract_messages_text(messages_str: str) -> str:
    if not messages_str:
        return ""

    try:
        messages = _parse_genai_messages_json(messages_str)
        if messages is None:
            return messages_str
        if isinstance(messages, list):
            parts = []
            for msg in messages:
                if isinstance(msg, dict):
                    role = msg.get("role", "")
                    content = _genai_message_content_to_text(msg)
                    if content:
                        parts.append(f"[{role}] {content}" if role else str(content))
                elif isinstance(msg, str):
                    parts.append(msg)
            return "\n".join(parts)
        return messages_str
    except (json.JSONDecodeError, TypeError):
        return messages_str


def _normalize_genai_messages_for_display(messages: Any) -> list[dict[str, Any]]:
    if not isinstance(messages, list):
        return []

    role_labels = {
        "system": "system instruction",
        "user": "user",
        "assistant": "assistant",
        "tool": "tool",
    }

    normalized: list[dict[str, Any]] = []
    for message in messages:
        if isinstance(message, dict):
            msg = dict(message)
            role = str(msg.get("role") or "").strip().lower()
            if role:
                msg["role"] = role
                msg["role_label"] = role_labels.get(role, role)
            content = _genai_message_content_to_text(msg)
            reasoning = _genai_message_reasoning_to_text(msg)
            if content:
                msg["content"] = content
            if reasoning:
                msg["thinking_content"] = reasoning
            if msg.get("content") is None:
                msg["content"] = ""
            normalized.append(msg)
            continue

        if isinstance(message, str):
            normalized.append({"role": "", "content": message})
            continue

        normalized.append({"role": "", "content": json.dumps(message, ensure_ascii=False)})

    return normalized


def _normalize_for_dedupe(value: Any) -> str:
    text = str(value or "").strip().lower()
    if not text:
        return ""
    return re.sub(r"\s+", " ", text)


def _dedupe_system_input_messages(
    input_messages: list[dict[str, Any]], system_instructions: str
) -> tuple[list[dict[str, Any]], int]:
    canonical_system = _normalize_for_dedupe(system_instructions)
    if not canonical_system:
        return input_messages, 0

    filtered_messages: list[dict[str, Any]] = []
    duplicate_count = 0
    for msg in input_messages:
        role = str(msg.get("role") or "").strip().lower()
        if role == "system":
            content = _normalize_for_dedupe(msg.get("content") or "")
            if content and content == canonical_system:
                duplicate_count += 1
                continue
        filtered_messages.append(msg)
    return filtered_messages, duplicate_count


def _string_attr_truthy(value: Any) -> bool:
    return str(value or "").strip().lower() in {"1", "true", "yes", "y", "on"}


def _first_message_content(messages: list[dict[str, Any]], roles: tuple[str, ...]) -> str:
    target_roles = {role.strip().lower() for role in roles}
    for message in messages:
        role = str(message.get("role") or "").strip().lower()
        if role not in target_roles:
            continue
        content = str(message.get("content") or "").strip()
        if content:
            return content
    return ""


def _summarize_ai_tool_action(raw_action: str) -> str:
    text = str(raw_action or "").strip()
    if not text:
        return ""
    try:
        parsed = json.loads(text)
    except (json.JSONDecodeError, TypeError):
        return text[:180]
    if not isinstance(parsed, dict):
        return text[:180]
    action_type = str(parsed.get("type") or "").strip()
    sql_where = str(parsed.get("sql_where") or "").strip()
    target_page = str(parsed.get("target_page") or "").strip()
    if sql_where:
        return f"{action_type or 'action'}: {sql_where}"[:180]
    if target_page:
        return f"{action_type or 'action'} -> {target_page}"[:180]
    return action_type[:180]


def _build_ai_trace_turn_cards(spans: list[dict[str, Any]]) -> list[dict[str, Any]]:
    turns: dict[str, dict[str, Any]] = {}
    for item in spans:
        turn_id = str(item.get("turn_id") or "").strip()
        if not turn_id:
            continue
        turn = turns.setdefault(
            turn_id,
            {
                "turn_id": turn_id,
                "chat_id": str(item.get("chat_id") or "").strip(),
                "model": str(item.get("model") or "").strip(),
                "provider": str(item.get("provider") or "").strip(),
                "status": "in_progress",
                "user_message": "",
                "assistant_message": "",
                "request_summary": "",
                "action_summary": "",
                "result_summary": "",
                "guard_allowed": None,
                "guard_reason": "",
                "tools": [],
                "tool_count": 0,
                "tokens_in": 0,
                "tokens_out": 0,
                "thinking_tokens": 0,
                "duration_ms": 0.0,
                "started_at": str(item.get("ts") or ""),
                "completed_at": "",
                "event_names": [],
                "trace_id": str(item.get("trace_id") or "").strip(),
            },
        )

        event_name = str(item.get("event_name") or "").strip()
        if event_name and event_name not in turn["event_names"]:
            turn["event_names"].append(event_name)

        if not turn["model"]:
            turn["model"] = str(item.get("model") or "").strip()
        if not turn["provider"]:
            turn["provider"] = str(item.get("provider") or "").strip()
        if not turn["chat_id"]:
            turn["chat_id"] = str(item.get("chat_id") or "").strip()
        if not turn["trace_id"]:
            turn["trace_id"] = str(item.get("trace_id") or "").strip()

        ts = str(item.get("ts") or "")
        if ts and (not turn["started_at"] or ts < turn["started_at"]):
            turn["started_at"] = ts
        if ts and (not turn["completed_at"] or ts > turn["completed_at"]):
            turn["completed_at"] = ts

        turn["tokens_in"] += int(item.get("tokens_in") or 0)
        turn["tokens_out"] += int(item.get("tokens_out") or 0)
        turn["thinking_tokens"] += int(item.get("thinking_tokens") or 0)
        turn["duration_ms"] = round(float(turn["duration_ms"] or 0) + float(item.get("duration_ms") or 0), 1)

        user_candidate = (
            str(item.get("input_question") or "").strip()
            or _first_message_content(cast(list[dict[str, Any]], item.get("input_messages") or []), ("user",))
            or str(item.get("prompt") or "").strip()
        )
        if user_candidate and not turn["user_message"]:
            turn["user_message"] = user_candidate

        assistant_candidate = (
            _first_message_content(cast(list[dict[str, Any]], item.get("output_messages") or []), ("assistant",))
            or str(item.get("response") or "").strip()
        )
        if assistant_candidate and (event_name == "turn.complete" or not turn["assistant_message"]):
            turn["assistant_message"] = assistant_candidate

        request_summary = str(item.get("turn_summary_request") or "").strip()
        action_summary = str(item.get("turn_summary_action") or "").strip()
        result_summary = str(item.get("turn_summary_result") or "").strip()
        if request_summary and not turn["request_summary"]:
            turn["request_summary"] = request_summary
        if action_summary and not turn["action_summary"]:
            turn["action_summary"] = action_summary
        if result_summary and not turn["result_summary"]:
            turn["result_summary"] = result_summary

        if event_name == "guard.result":
            turn["guard_allowed"] = _string_attr_truthy(item.get("guard_allowed"))
            turn["guard_reason"] = str(item.get("guard_reason") or "").strip()
        elif event_name == "turn.blocked":
            turn["status"] = "blocked"
            turn["guard_reason"] = str(item.get("guard_reason") or item.get("error_message") or "").strip()
        elif event_name == "turn.error":
            turn["status"] = "failed"
        elif event_name == "turn.cancelled":
            turn["status"] = "cancelled"
        elif event_name == "turn.complete" and turn["status"] == "in_progress":
            turn["status"] = "completed"

        if event_name in {"tool.proposed", "tool.executed"}:
            tool_name = str(item.get("tool_name") or "propose_ui_action").strip()
            tool_status = str(
                item.get("tool_status") or ("executed" if event_name == "tool.executed" else "proposed")
            ).strip()
            tool_summary = str(item.get("tool_summary") or "").strip() or _summarize_ai_tool_action(
                str(item.get("tool_action") or "")
            )
            tool_key = (
                str(item.get("tool_action_id") or "").strip(),
                tool_name,
                tool_status,
                tool_summary,
            )
            if tool_key not in {
                (
                    str(existing.get("action_id") or "").strip(),
                    str(existing.get("name") or "").strip(),
                    str(existing.get("status") or "").strip(),
                    str(existing.get("summary") or "").strip(),
                )
                for existing in turn["tools"]
            }:
                turn["tools"].append(
                    {
                        "name": tool_name,
                        "status": tool_status,
                        "summary": tool_summary,
                        "action_id": str(item.get("tool_action_id") or "").strip(),
                    }
                )

    turn_cards = sorted(
        turns.values(), key=lambda item: (str(item.get("started_at") or ""), str(item.get("turn_id") or ""))
    )
    for index, turn in enumerate(turn_cards, start=1):
        turn["index"] = index
        turn["tool_count"] = len(cast(list[dict[str, Any]], turn.get("tools") or []))
        if not str(turn.get("request_summary") or "").strip():
            turn["request_summary"] = str(turn.get("user_message") or "").strip()
        if not str(turn.get("result_summary") or "").strip():
            turn["result_summary"] = str(turn.get("assistant_message") or "").strip()
    return turn_cards
