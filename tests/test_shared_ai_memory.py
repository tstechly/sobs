import json
import re

import pytest

from shared.ai_memory import (
    _chat_label_from_first_turn,
    _coerce_summary_value,
    _consolidate_memory_candidates,
    _cosine_similarity,
    _derive_turn_summary,
    _embedding_from_json,
    _embedding_to_json,
    _extract_assistant_meta,
    _extract_memory_candidates,
    _load_chat_memories,
    _load_chat_tool_history,
    _load_recent_chat_turns,
    _load_recent_turn_summaries,
    _sanitize_chat_label_candidate,
    _semantic_memory_matches,
    _text_embedding,
    _tokenize_for_embedding,
    _tool_status_label,
    _upsert_ai_memory,
)

ASSISTANT_META_RE = re.compile(r"<assistant_meta\b[^>]*>\s*([\s\S]*?)\s*</assistant_meta>", re.IGNORECASE)
ASSISTANT_META_ESCAPED_RE = re.compile(
    r"&lt;assistant_meta\b[^&]*&gt;\s*([\s\S]*?)\s*&lt;/assistant_meta&gt;",
    re.IGNORECASE,
)


class _FakeResult:
    def __init__(self, rows):
        self._rows = rows

    def fetchall(self):
        return self._rows


class _FakeDb:
    def __init__(self, rows):
        self.rows = rows
        self.calls = []

    def execute(self, sql, params):
        self.calls.append((sql, params))
        return _FakeResult(self.rows)


def _extract_meta(text):
    return _extract_assistant_meta(
        text,
        assistant_meta_re=ASSISTANT_META_RE,
        assistant_meta_escaped_re=ASSISTANT_META_ESCAPED_RE,
    )


def test_embedding_helpers_cover_tokenization_similarity_and_json_round_trip():
    assert _tokenize_for_embedding("") == []
    assert _tokenize_for_embedding("API /Logs:error") == ["api", "/logs:error"]

    embedding = _text_embedding("api errors by service", dims=16, tokenize_for_embedding=_tokenize_for_embedding)
    assert len(embedding) == 16
    assert embedding == _text_embedding(
        "api errors by service", dims=16, tokenize_for_embedding=_tokenize_for_embedding
    )
    assert _text_embedding("", dims=8, tokenize_for_embedding=_tokenize_for_embedding) == [0.0] * 8

    assert _cosine_similarity([], [1.0]) == 0.0
    assert _cosine_similarity([1.0, 2.0], [1.0]) == 1.0

    raw = _embedding_to_json([1.0, 2.5])
    assert raw == "[1.0,2.5]"
    assert _embedding_from_json(raw) == [1.0, 2.5]
    assert _embedding_from_json('{"bad":true}') == []
    assert _embedding_from_json('[1, "x"]') == [1.0, 0.0]
    assert _embedding_from_json("not-json") == []


def test_extract_assistant_meta_handles_plain_smart_quote_and_escaped_cases():
    cleaned, meta = _extract_meta('All good.<assistant_meta>{"turn_summary":{"request":"show logs"}}</assistant_meta>')
    assert cleaned == "All good."
    assert meta["turn_summary"]["request"] == "show logs"

    cleaned, meta = _extract_meta("Question <assistant_meta >{“turn_summary”:{“request”:“help me”}}</assistant_meta>")
    assert cleaned == "Question"
    assert meta["turn_summary"]["request"] == "help me"

    cleaned, meta = _extract_meta(
        'Which page? &lt;assistant_meta&gt;{"memory_candidates":["pref"]}&lt;/assistant_meta&gt;'
    )
    assert cleaned == "Which page?"
    assert meta["memory_candidates"] == ["pref"]

    cleaned, meta = _extract_meta('Text only <assistant_meta>{"broken": true')
    assert cleaned == "Text only"
    assert meta == {}


def test_summary_and_label_helpers_cover_truncation_and_fallbacks():
    assert _coerce_summary_value("  hello  ", 10) == "hello"
    assert _coerce_summary_value("abcdefghij", 4) == "abcd"

    assert (
        _sanitize_chat_label_candidate(
            'User said "Show API errors by service" in the chat',
            extract_assistant_meta=_extract_meta,
        )
        == "Show API errors by service"
    )
    assert (
        _sanitize_chat_label_candidate(
            "Awaiting clarification from the user",
            extract_assistant_meta=_extract_meta,
        )
        == ""
    )
    assert _sanitize_chat_label_candidate("", extract_assistant_meta=_extract_meta) == ""

    assert (
        _chat_label_from_first_turn(
            "Primary question",
            "Fallback request",
            sanitize_chat_label_candidate=lambda value: str(value),
            coerce_summary_value=lambda value, max_len: str(value)[:max_len],
        )
        == "Primary question"
    )
    assert (
        _chat_label_from_first_turn(
            "",
            "Fallback request",
            sanitize_chat_label_candidate=lambda value: str(value),
            coerce_summary_value=lambda value, max_len: str(value)[:max_len],
        )
        == "Fallback request"
    )
    assert (
        _chat_label_from_first_turn(
            "",
            "",
            sanitize_chat_label_candidate=lambda value: str(value),
            coerce_summary_value=lambda value, max_len: str(value)[:max_len],
        )
        == "New chat"
    )

    assert _derive_turn_summary(question="q", answer="a", tool_summary="", meta_summary=None) == {
        "request": "q",
        "action": "answer_only",
        "result": "a",
    }
    assert _derive_turn_summary(
        question="q",
        answer="a",
        tool_summary="tool",
        meta_summary={"request": "rq", "action": "act", "result": "rs"},
    ) == {"request": "rq", "action": "act", "result": "rs"}


def test_memory_loading_matching_and_candidate_extraction_cover_branches():
    db = _FakeDb(
        [
            {
                "Id": "m1",
                "MemoryText": "api errors spike",
                "EmbeddingJson": "[1,2]",
                "SourceTurnId": "t1",
                "UpdatedAt": "2026-01-01T00:00:00+00:00",
            }
        ]
    )
    memories = _load_chat_memories(db, "chat-1", embedding_from_json=_embedding_from_json)
    assert memories == [
        {
            "id": "m1",
            "text": "api errors spike",
            "embedding": [1.0, 2.0],
            "source_turn_id": "t1",
            "updated_at": "2026-01-01T00:00:00+00:00",
        }
    ]

    memory_matches = _semantic_memory_matches(
        [
            {"id": "m1", "text": "api errors spike", "embedding": []},
            {"id": "m2", "text": "deploy pipeline status", "embedding": []},
        ],
        "show api errors",
        text_embedding=lambda text: _text_embedding(text, dims=32, tokenize_for_embedding=_tokenize_for_embedding),
        cosine_similarity=_cosine_similarity,
        min_score=0.0,
        max_results=1,
    )
    assert [item["id"] for item in memory_matches] == ["m1"]

    assert _extract_memory_candidates(
        {"memory_candidates": ["Pref A", "pref a", "pref b", "pref c", "pref d"]},
        coerce_summary_value=_coerce_summary_value,
    ) == ["Pref A", "pref b", "pref c"]
    assert _extract_memory_candidates(
        {"memory_candidates": "single"},
        coerce_summary_value=_coerce_summary_value,
    ) == ["single"]


def test_upsert_ai_memory_builds_expected_row_payload():
    inserted = []

    _upsert_ai_memory(
        object(),
        memory_id="m1",
        chat_id="c1",
        memory_text="api errors spike",
        source_turn_id="t1",
        is_deleted=False,
        embedding_to_json=_embedding_to_json,
        text_embedding=lambda text: _text_embedding(text, dims=8, tokenize_for_embedding=_tokenize_for_embedding),
        now_iso=lambda: "2026-05-01T00:00:00+00:00",
        time_ms=lambda: 123456,
        insert_rows_json_each_row=lambda _db, table, rows: inserted.append((table, rows)),
    )

    assert inserted[0][0] == "sobs_ai_memories"
    row = inserted[0][1][0]
    assert row["Id"] == "m1"
    assert row["Version"] == 123456
    assert row["IsDeleted"] == 0
    assert row["EmbeddingJson"].startswith("[")


@pytest.mark.asyncio
async def test_consolidate_memory_candidates_handles_fallback_invalid_and_valid_responses():
    keep_new = await _consolidate_memory_candidates(
        {},
        new_memory="new fact",
        related=[],
        call_llm_endpoint=None,
        coerce_summary_value=_coerce_summary_value,
    )
    assert keep_new == {"action": "keep_new", "memory": "new fact", "drop_ids": []}

    async def _empty_answer(*_args, **_kwargs):
        return "", {}

    empty = await _consolidate_memory_candidates(
        {"ai.endpoint_url": "https://example.com", "ai.model": "gpt-test"},
        new_memory="new fact",
        related=[],
        call_llm_endpoint=_empty_answer,
        coerce_summary_value=_coerce_summary_value,
    )
    assert empty == {"action": "keep_new", "memory": "new fact", "drop_ids": []}

    async def _invalid_json(*_args, **_kwargs):
        return "[]", {}

    invalid = await _consolidate_memory_candidates(
        {"ai.endpoint_url": "https://example.com", "ai.model": "gpt-test", "ai.api_key": "key"},
        new_memory="new fact",
        related=[{"id": "m1", "text": "old fact", "score": 0.9}],
        call_llm_endpoint=_invalid_json,
        coerce_summary_value=_coerce_summary_value,
    )
    assert invalid == {"action": "keep_new", "memory": "new fact", "drop_ids": []}

    async def _valid_json(*_args, **_kwargs):
        return json.dumps({"action": "merge", "memory": "merged fact", "drop_ids": ["m1", "", None]}), {}

    merged = await _consolidate_memory_candidates(
        {"ai.endpoint_url": "https://example.com", "ai.model": "gpt-test", "ai.api_key": "key"},
        new_memory="new fact",
        related=[{"id": "m1", "text": "old fact", "score": 0.9}],
        call_llm_endpoint=_valid_json,
        coerce_summary_value=_coerce_summary_value,
    )
    assert merged == {"action": "merge", "memory": "merged fact", "drop_ids": ["m1"]}


def test_recent_turn_and_tool_history_helpers_cover_filters_and_statuses():
    summaries_db = _FakeDb(
        [
            {"request": "api errors", "action": "filter logs", "result": "applied", "turn_id": "t1"},
            {"request": "", "action": "ignored", "result": "", "turn_id": "t2"},
            {"request": "deploy status", "action": "show", "result": "done", "turn_id": "t3"},
        ]
    )
    summaries = _load_recent_turn_summaries(
        summaries_db,
        "chat-1",
        "api errors",
        helper_service_name="svc",
        text_embedding=lambda text: _text_embedding(text, dims=32, tokenize_for_embedding=_tokenize_for_embedding),
        cosine_similarity=_cosine_similarity,
        coerce_summary_value=_coerce_summary_value,
        limit=2,
    )
    assert summaries[0]["turn_id"] == "t1"
    assert all(item["turn_id"] != "t2" for item in summaries)

    recent_chat_turns = _load_recent_chat_turns(
        _FakeDb(
            [
                {"request": "api errors", "action": "filter", "result": "done", "turn_id": "t1"},
                {"request": "", "action": "", "result": "", "turn_id": "t2"},
            ]
        ),
        "chat-1",
        helper_service_name="svc",
        coerce_summary_value=_coerce_summary_value,
        limit=4,
    )
    assert recent_chat_turns == [{"turn_id": "t1", "request": "api errors", "action": "filter", "result": "done"}]
    assert (
        _load_recent_chat_turns(_FakeDb([]), "", helper_service_name="svc", coerce_summary_value=_coerce_summary_value)
        == []
    )

    assert _tool_status_label("executed", False) == "Executed"
    assert _tool_status_label("unsupported", False) == "Not available in this page action manifest"
    assert _tool_status_label("proposed", True) == "Awaiting confirmation"
    assert _tool_status_label("queued", False) == "Queued"

    tool_history = _load_chat_tool_history(
        _FakeDb(
            [
                {
                    "Timestamp": "2026-01-01T00:00:00+00:00",
                    "EventName": "tool.proposed",
                    "turn_id": "t1",
                    "action_id": "a1",
                    "summary": "Filter logs",
                    "action_json": '{"kind":"filter"}',
                    "action_status": "proposed",
                    "requires_confirmation": "true",
                },
                {
                    "Timestamp": "2026-01-01T00:00:01+00:00",
                    "EventName": "tool.executed",
                    "turn_id": "t1",
                    "action_id": "a1",
                    "summary": "Filter logs",
                    "action_json": "",
                    "action_status": "proposed",
                    "requires_confirmation": "true",
                },
                {
                    "Timestamp": "2026-01-01T00:00:02+00:00",
                    "EventName": "tool.proposed",
                    "turn_id": "",
                    "action_id": "ignored",
                    "summary": "Ignore",
                    "action_json": "bad-json",
                    "action_status": "unsupported",
                    "requires_confirmation": "false",
                },
            ]
        ),
        "chat-1",
        helper_service_name="svc",
        tool_status_label=_tool_status_label,
    )
    assert list(tool_history) == ["t1"]
    assert tool_history["t1"][0]["status"] == "executed"
    assert tool_history["t1"][0]["status_label"] == "Executed"
    assert tool_history["t1"][0]["action"] == {"kind": "filter"}
