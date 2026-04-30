"""Shared AI embedding and memory helpers used by the SOBS assistant."""

from __future__ import annotations

import hashlib
import html
import json
import re
from typing import Any


def _tokenize_for_embedding(text: str) -> list[str]:
    if not text:
        return []
    return re.findall(r"[a-z0-9_./:-]+", text.lower())


def _text_embedding(text: str, *, dims: int, tokenize_for_embedding) -> list[float]:
    vector = [0.0] * dims
    tokens = tokenize_for_embedding(text)
    if not tokens:
        return vector
    for token in tokens:
        index = int(hashlib.sha256(token.encode("utf-8")).hexdigest(), 16) % dims
        vector[index] += 1.0
    norm = sum(value * value for value in vector) ** 0.5
    if norm <= 0:
        return vector
    return [value / norm for value in vector]


def _cosine_similarity(a: list[float], b: list[float]) -> float:
    if not a or not b:
        return 0.0
    size = min(len(a), len(b))
    if size == 0:
        return 0.0
    return sum(a[index] * b[index] for index in range(size))


def _embedding_to_json(vector: list[float]) -> str:
    return json.dumps(vector, separators=(",", ":"), ensure_ascii=False)


def _embedding_from_json(raw: str) -> list[float]:
    text = str(raw or "").strip()
    if not text:
        return []
    try:
        parsed = json.loads(text)
    except Exception:
        return []
    if not isinstance(parsed, list):
        return []
    values: list[float] = []
    for item in parsed:
        try:
            values.append(float(item))
        except Exception:
            values.append(0.0)
    return values


def _extract_assistant_meta(
    answer_text: str, *, assistant_meta_re, assistant_meta_escaped_re
) -> tuple[str, dict[str, Any]]:
    text = str(answer_text or "")

    def _strip_meta_blocks(raw_text: str) -> str:
        cleaned = assistant_meta_re.sub("", raw_text)
        cleaned = assistant_meta_escaped_re.sub("", cleaned)
        open_raw = cleaned.lower().find("<assistant_meta")
        open_escaped = cleaned.lower().find("&lt;assistant_meta")
        cut_index = -1
        if open_raw >= 0:
            cut_index = open_raw
        if open_escaped >= 0 and (cut_index < 0 or open_escaped < cut_index):
            cut_index = open_escaped
        if cut_index >= 0:
            cleaned = cleaned[:cut_index]
        return cleaned

    match = assistant_meta_re.search(text)
    if not match:
        match = assistant_meta_escaped_re.search(text)
    if not match:
        return _strip_meta_blocks(text).strip(), {}
    meta_raw = str(match.group(1) or "")
    meta: dict[str, Any] = {}
    try:
        normalized_meta_raw = (
            html.unescape(meta_raw)
            .replace("\u201c", '"')
            .replace("\u201d", '"')
            .replace("\u2018", "'")
            .replace("\u2019", "'")
        )
        parsed = json.loads(normalized_meta_raw)
        if isinstance(parsed, dict):
            meta = parsed
    except Exception:
        meta = {}
    cleaned = _strip_meta_blocks(text).strip()
    return cleaned, meta


def _coerce_summary_value(value: Any, max_len: int = 240) -> str:
    text = str(value or "").strip()
    if len(text) > max_len:
        return text[:max_len]
    return text


def _sanitize_chat_label_candidate(value: Any, *, extract_assistant_meta) -> str:
    text = str(value or "").strip()
    if not text:
        return ""
    text, _meta = extract_assistant_meta(text)
    lower = text.lower()
    quoted_match = re.match(r'^\s*user\s+(?:wrote|said)\s+"([^"]+)".*$', text, flags=re.IGNORECASE)
    if quoted_match:
        text = quoted_match.group(1).strip()
        lower = text.lower()
    noisy_markers = (
        "unclear intent",
        "without a clear request",
        "awaiting clarification",
    )
    if any(marker in lower for marker in noisy_markers):
        return ""
    return text


def _chat_label_from_first_turn(
    first_question: Any, first_request: Any, *, sanitize_chat_label_candidate, coerce_summary_value
) -> str:
    question_label = sanitize_chat_label_candidate(first_question)
    if question_label:
        return coerce_summary_value(question_label, 80)
    request_label = sanitize_chat_label_candidate(first_request)
    if request_label:
        return coerce_summary_value(request_label, 80)
    return "New chat"


def _derive_turn_summary(
    *,
    question: str,
    answer: str,
    tool_summary: str,
    meta_summary: dict[str, Any] | None = None,
) -> dict[str, str]:
    summary = dict(meta_summary or {})
    request_text = _coerce_summary_value(summary.get("request") or question, 180)
    action_text = _coerce_summary_value(summary.get("action") or tool_summary or "answer_only", 180)
    result_text = _coerce_summary_value(summary.get("result") or answer, 280)
    return {
        "request": request_text,
        "action": action_text,
        "result": result_text,
    }


def _load_chat_memories(db: Any, chat_id: str, *, embedding_from_json) -> list[dict[str, Any]]:
    rows = db.execute(
        "SELECT Id, MemoryText, EmbeddingJson, SourceTurnId, UpdatedAt "
        "FROM sobs_ai_memories FINAL WHERE ChatId=? AND IsDeleted=0 ORDER BY UpdatedAt DESC LIMIT 200",
        [chat_id],
    ).fetchall()
    memories: list[dict[str, Any]] = []
    for row in rows:
        memories.append(
            {
                "id": str(row["Id"] or ""),
                "text": str(row["MemoryText"] or "").strip(),
                "embedding": embedding_from_json(str(row["EmbeddingJson"] or "")),
                "source_turn_id": str(row["SourceTurnId"] or ""),
                "updated_at": str(row["UpdatedAt"] or ""),
            }
        )
    return memories


def _semantic_memory_matches(
    memories: list[dict[str, Any]],
    query_text: str,
    *,
    text_embedding,
    cosine_similarity,
    max_results: int = 5,
    min_score: float = 0.26,
) -> list[dict[str, Any]]:
    query_embedding = text_embedding(query_text)
    scored: list[dict[str, Any]] = []
    for item in memories:
        embedding = item.get("embedding") or []
        if not embedding:
            embedding = text_embedding(str(item.get("text") or ""))
        score = cosine_similarity(query_embedding, embedding)
        if score < min_score:
            continue
        scored.append(
            {
                "id": str(item.get("id") or ""),
                "text": str(item.get("text") or ""),
                "score": round(score, 4),
                "source_turn_id": str(item.get("source_turn_id") or ""),
            }
        )
    scored.sort(key=lambda item: float(item.get("score") or 0), reverse=True)
    return scored[:max_results]


def _upsert_ai_memory(
    db: Any,
    *,
    memory_id: str,
    chat_id: str,
    memory_text: str,
    source_turn_id: str,
    is_deleted: bool,
    embedding_to_json,
    text_embedding,
    now_iso,
    time_ms,
    insert_rows_json_each_row,
) -> None:
    row = {
        "Id": memory_id,
        "ChatId": chat_id,
        "MemoryText": memory_text,
        "EmbeddingJson": embedding_to_json(text_embedding(memory_text)) if memory_text else "",
        "SourceTurnId": source_turn_id,
        "IsDeleted": 1 if is_deleted else 0,
        "Version": time_ms(),
        "UpdatedAt": now_iso(),
    }
    insert_rows_json_each_row(db, "sobs_ai_memories", [row])


async def _consolidate_memory_candidates(
    settings: dict[str, str],
    *,
    new_memory: str,
    related: list[dict[str, Any]],
    call_llm_endpoint,
    coerce_summary_value,
) -> dict[str, Any]:
    endpoint_url = str(settings.get("ai.endpoint_url") or "").strip()
    model = str(settings.get("ai.model") or "").strip()
    api_key = str(settings.get("ai.api_key") or "").strip()
    if not endpoint_url or not model:
        return {"action": "keep_new", "memory": new_memory, "drop_ids": []}
    related_payload = [
        {
            "id": str(item.get("id") or ""),
            "text": str(item.get("text") or ""),
            "score": float(item.get("score") or 0),
        }
        for item in related
    ]
    messages = [
        {
            "role": "system",
            "content": (
                "You reconcile short AI memories. Return ONLY strict JSON with keys: "
                "action (merge|keep_new|ignore), memory (string), drop_ids (array of ids). "
                "Merge overlapping/conflicting memories into one concise, current fact. "
                "If new memory is noise/duplicate, use ignore."
            ),
        },
        {
            "role": "user",
            "content": json.dumps({"new_memory": new_memory, "related": related_payload}, ensure_ascii=False),
        },
    ]
    answer, _stats = await call_llm_endpoint(
        endpoint_url,
        model,
        api_key,
        messages,
        thinking_level="off",
        max_tokens=220,
        timeout=20,
    )
    if not answer:
        return {"action": "keep_new", "memory": new_memory, "drop_ids": []}
    try:
        parsed = json.loads(answer)
    except Exception:
        parsed = {}
    if not isinstance(parsed, dict):
        return {"action": "keep_new", "memory": new_memory, "drop_ids": []}
    action = str(parsed.get("action") or "keep_new").strip().lower()
    if action not in {"merge", "keep_new", "ignore"}:
        action = "keep_new"
    memory_text = coerce_summary_value(parsed.get("memory") or new_memory, 280)
    raw_drop_ids = parsed.get("drop_ids")
    drop_ids: list[str] = []
    if isinstance(raw_drop_ids, list):
        for item in raw_drop_ids:
            memory_id = str(item or "").strip()
            if memory_id:
                drop_ids.append(memory_id)
    return {"action": action, "memory": memory_text, "drop_ids": drop_ids}


def _extract_memory_candidates(meta: dict[str, Any], *, coerce_summary_value) -> list[str]:
    candidates: list[str] = []
    raw = meta.get("memory_candidates")
    if isinstance(raw, list):
        for item in raw:
            text = coerce_summary_value(item, 280)
            if text:
                candidates.append(text)
    elif isinstance(raw, str):
        text = coerce_summary_value(raw, 280)
        if text:
            candidates.append(text)
    deduped: list[str] = []
    seen: set[str] = set()
    for text in candidates:
        dedupe_key = text.lower()
        if dedupe_key in seen:
            continue
        seen.add(dedupe_key)
        deduped.append(text)
        if len(deduped) >= 3:
            break
    return deduped


def _load_recent_turn_summaries(
    db: Any,
    chat_id: str,
    query: str,
    *,
    helper_service_name: str,
    text_embedding,
    cosine_similarity,
    coerce_summary_value,
    limit: int = 4,
) -> list[dict[str, str]]:
    rows = db.execute(
        "SELECT Timestamp, LogAttributes['gen_ai.turn.summary.request'] AS request, "
        "LogAttributes['gen_ai.turn.summary.action'] AS action, "
        "LogAttributes['gen_ai.turn.summary.result'] AS result, "
        "LogAttributes['gen_ai.turn_id'] AS turn_id "
        "FROM otel_logs WHERE ServiceName=? AND EventName='turn.summary' AND LogAttributes['gen_ai.chat_id']=? "
        "ORDER BY Timestamp DESC LIMIT 100",
        [helper_service_name, chat_id],
    ).fetchall()
    scored: list[dict[str, Any]] = []
    query_embedding = text_embedding(query)
    for row in rows:
        request = str(row["request"] or "").strip()
        action = str(row["action"] or "").strip()
        result = str(row["result"] or "").strip()
        if not request and not result:
            continue
        candidate_text = f"{request} {action} {result}".strip()
        score = cosine_similarity(query_embedding, text_embedding(candidate_text))
        if score < 0.2:
            continue
        scored.append(
            {
                "turn_id": str(row["turn_id"] or ""),
                "request": coerce_summary_value(request, 180),
                "action": coerce_summary_value(action, 180),
                "result": coerce_summary_value(result, 220),
                "score": score,
            }
        )
    scored.sort(key=lambda item: float(item.get("score") or 0), reverse=True)
    output: list[dict[str, str]] = []
    for item in scored[:limit]:
        output.append(
            {
                "turn_id": str(item.get("turn_id") or ""),
                "request": str(item.get("request") or ""),
                "action": str(item.get("action") or ""),
                "result": str(item.get("result") or ""),
            }
        )
    return output


def _load_recent_chat_turns(
    db: Any,
    chat_id: str,
    *,
    helper_service_name: str,
    coerce_summary_value,
    limit: int = 8,
) -> list[dict[str, str]]:
    if not str(chat_id or "").strip():
        return []
    rows = db.execute(
        "SELECT Timestamp, LogAttributes['gen_ai.turn.summary.request'] AS request, "
        "LogAttributes['gen_ai.turn.summary.action'] AS action, "
        "LogAttributes['gen_ai.turn.summary.result'] AS result, "
        "LogAttributes['gen_ai.turn_id'] AS turn_id "
        "FROM otel_logs "
        "WHERE ServiceName=? AND EventName='turn.summary' AND LogAttributes['gen_ai.chat_id']=? "
        "ORDER BY Timestamp DESC LIMIT ?",
        [helper_service_name, chat_id, int(max(1, limit))],
    ).fetchall()
    output: list[dict[str, str]] = []
    for row in rows:
        request = str(row["request"] or "").strip()
        action = str(row["action"] or "").strip()
        result = str(row["result"] or "").strip()
        if not request and not action and not result:
            continue
        output.append(
            {
                "turn_id": str(row["turn_id"] or ""),
                "request": coerce_summary_value(request, 180),
                "action": coerce_summary_value(action, 180),
                "result": coerce_summary_value(result, 220),
            }
        )
    return output


def _tool_status_label(status: str, requires_confirmation: bool) -> str:
    normalized = str(status or "").strip().lower()
    if normalized == "executed":
        return "Executed"
    if normalized == "unsupported":
        return "Not available in this page action manifest"
    if requires_confirmation:
        return "Awaiting confirmation"
    return "Queued"


def _load_chat_tool_history(
    db: Any, chat_id: str, *, helper_service_name: str, tool_status_label
) -> dict[str, list[dict[str, Any]]]:
    rows = db.execute(
        "SELECT Timestamp, EventName, LogAttributes['gen_ai.turn_id'] AS turn_id, "
        "LogAttributes['sobs.ai.action_id'] AS action_id, "
        "LogAttributes['sobs.ai.tool.summary'] AS summary, "
        "LogAttributes['sobs.ai.tool.action'] AS action_json, "
        "LogAttributes['sobs.ai.action.status'] AS action_status, "
        "LogAttributes['sobs.ai.action.requires_confirmation'] AS requires_confirmation "
        "FROM otel_logs "
        "WHERE ServiceName=? AND EventName IN ('tool.proposed', 'tool.executed') "
        "AND LogAttributes['gen_ai.chat_id']=? "
        "ORDER BY Timestamp ASC LIMIT 500",
        [helper_service_name, chat_id],
    ).fetchall()

    grouped: dict[str, dict[str, dict[str, Any]]] = {}
    for row in rows:
        turn_id = str(row["turn_id"] or "").strip()
        if not turn_id:
            continue
        action_id = str(row["action_id"] or "").strip() or f"anon-{row['Timestamp']}"
        turn_actions = grouped.setdefault(turn_id, {})
        action_entry = turn_actions.get(action_id)
        if not action_entry:
            action_payload: dict[str, Any] = {}
            raw_action = str(row["action_json"] or "").strip()
            if raw_action:
                try:
                    parsed_action = json.loads(raw_action)
                    if isinstance(parsed_action, dict):
                        action_payload = parsed_action
                except (TypeError, json.JSONDecodeError):
                    action_payload = {}
            action_entry = {
                "kind": "tool",
                "turn_id": turn_id,
                "action_id": action_id,
                "summary": str(row["summary"] or "").strip(),
                "action": action_payload,
                "status": str(row["action_status"] or "proposed").strip().lower() or "proposed",
                "requires_confirmation": str(row["requires_confirmation"] or "").strip().lower()
                in {"1", "true", "yes", "on"},
                "ts": str(row["Timestamp"] or ""),
            }
            turn_actions[action_id] = action_entry

        if str(row["EventName"] or "") == "tool.executed":
            action_entry["status"] = "executed"

    output: dict[str, list[dict[str, Any]]] = {}
    for turn_id, action_map in grouped.items():
        turn_items = list(action_map.values())
        turn_items.sort(key=lambda item: str(item.get("ts") or ""))
        for item in turn_items:
            item["status_label"] = tool_status_label(
                str(item.get("status") or ""),
                bool(item.get("requires_confirmation")),
            )
        output[turn_id] = turn_items
    return output
