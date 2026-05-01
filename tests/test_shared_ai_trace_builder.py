import json
from typing import Any, cast

import shared.ai_trace_builder as ai_trace_builder_module
from shared.ai_trace_builder import (
    _build_ai_trace_turn_cards,
    _dedupe_system_input_messages,
    _extract_messages_text,
    _first_message_content,
    _genai_message_content_to_text,
    _genai_message_reasoning_to_text,
    _genai_tool_calls_to_text,
    _normalize_for_dedupe,
    _normalize_genai_messages_for_display,
    _parse_genai_messages_json,
    _string_attr_truthy,
    _summarize_ai_tool_action,
)


def test_genai_tool_calls_to_text_renders_supported_shapes() -> None:
    rendered = _genai_tool_calls_to_text(
        [
            "ignore-me",
            {"function": {"name": "lookup_weather", "arguments": {"city": "Paris"}}},
            {"name": "open_page", "arguments": "logs"},
            {"name": "confirm_action"},
        ]
    )

    assert _genai_tool_calls_to_text({"bad": "shape"}) == ""
    assert 'tool_call:lookup_weather {"city": "Paris"}' in rendered
    assert "tool_call:open_page logs" in rendered
    assert rendered.endswith("tool_call:confirm_action")


def test_genai_message_content_to_text_prefers_content_variants() -> None:
    assert _genai_message_content_to_text({"content": "hello"}) == "hello"
    assert _genai_message_content_to_text({"content": [{"text": "alpha"}, "beta"]}) == "alpha beta"
    assert _genai_message_content_to_text({"content": 123}) == "123"


def test_genai_message_content_to_text_handles_parts_and_tool_fallbacks() -> None:
    message = {
        "parts": [
            "plain text",
            99,
            {"type": "text", "content": "hello"},
            {"type": "reasoning", "text": "because"},
            {"type": "tool_call", "function": {"name": "lookup", "arguments": {"id": 1}}},
            {"type": "tool_call_response", "response": "done"},
            {"type": "server_tool_call_response"},
            {"content": "fallback content"},
            {"other": "value"},
        ]
    }
    parts_text = _genai_message_content_to_text(message)

    assert "plain text" in parts_text
    assert "hello" in parts_text
    assert "because" in parts_text
    assert 'tool_call:lookup {"id": 1}' in parts_text
    assert "done" in parts_text
    assert "server_tool_call_response" in parts_text
    assert "fallback content" in parts_text
    assert '{"other": "value"}' in parts_text

    assert (
        _genai_message_content_to_text({"tool_calls": [{"function": {"name": "tool", "arguments": []}}]})
        == "tool_call:tool []"
    )
    assert (
        _genai_message_content_to_text({"function_call": {"name": "tool", "arguments": {"x": 1}}})
        == 'tool_call:tool {"x": 1}'
    )
    assert _genai_message_content_to_text({}) == ""


def test_genai_message_reasoning_to_text_handles_fields_and_parts() -> None:
    assert _genai_message_reasoning_to_text({"reasoning_content": "  think  "}) == "think"
    assert (
        _genai_message_reasoning_to_text({"reasoning": [" step 1 ", {"text": "step 2"}, 3, {"ignored": ""}]})
        == "step 1\nstep 2\n3"
    )
    assert _genai_message_reasoning_to_text({"thinking": 7}) == "7"
    assert _genai_message_reasoning_to_text({"thinking": {"content": "deep thought"}}) == "deep thought"
    assert _genai_message_reasoning_to_text({"thinking": {"other": "value"}}) == '{"other": "value"}'
    assert (
        _genai_message_reasoning_to_text(
            {
                "parts": [
                    {"type": "text", "content": "ignore"},
                    {"type": "reasoning", "content": ["alpha", {"content": "beta"}]},
                ]
            }
        )
        == "alpha\nbeta"
    )
    assert _genai_message_reasoning_to_text({"parts": ["ignore-me"]}) == ""


def test_parse_extract_and_normalize_genai_messages() -> None:
    wrapped = json.dumps({"messages": [{"role": "user", "content": "Hello"}, "tail"]})

    assert _parse_genai_messages_json("") == []
    assert _parse_genai_messages_json("not json") is None
    assert _parse_genai_messages_json(wrapped) == [{"role": "user", "content": "Hello"}, "tail"]
    assert _parse_genai_messages_json(json.dumps({"other": []})) == []
    assert _extract_messages_text("") == ""

    extracted = _extract_messages_text(
        json.dumps(
            [
                {"role": "user", "content": "Hello"},
                {"role": "assistant", "tool_calls": [{"function": {"name": "lookup", "arguments": "x"}}]},
                "tail",
            ]
        )
    )
    assert "[user] Hello" in extracted
    assert "tool_call:lookup x" in extracted
    assert extracted.endswith("tail")
    assert _extract_messages_text("plain text") == "plain text"

    original_parser = ai_trace_builder_module._parse_genai_messages_json
    try:
        ai_trace_builder_module._parse_genai_messages_json = cast(Any, lambda _value: {"unexpected": True})
        assert _extract_messages_text("[]") == "[]"
        ai_trace_builder_module._parse_genai_messages_json = cast(
            Any, lambda _value: (_ for _ in ()).throw(TypeError("boom"))
        )
        assert _extract_messages_text("[]") == "[]"
    finally:
        ai_trace_builder_module._parse_genai_messages_json = original_parser

    normalized = _normalize_genai_messages_for_display(
        [
            {
                "role": "System",
                "parts": [{"type": "text", "content": "Prompt"}],
                "reasoning": "step",
                "content": None,
            },
            "hello",
            42,
        ]
    )
    assert normalized[0]["role"] == "system"
    assert normalized[0]["role_label"] == "system instruction"
    assert normalized[0]["content"] == "Prompt"
    assert normalized[0]["thinking_content"] == "step"
    assert normalized[1] == {"role": "", "content": "hello"}
    assert normalized[2] == {"role": "", "content": "42"}
    assert _normalize_genai_messages_for_display([{"role": "assistant", "content": None}])[0]["content"] == ""
    assert _normalize_genai_messages_for_display({"bad": "shape"}) == []


def test_dedupe_and_simple_ai_trace_helpers() -> None:
    filtered, duplicate_count = _dedupe_system_input_messages(
        [
            {"role": "system", "content": " You are helpful. "},
            {"role": "user", "content": "Show logs"},
            {"role": "system", "content": "Different"},
        ],
        "you   are helpful.",
    )

    assert duplicate_count == 1
    assert filtered == [
        {"role": "user", "content": "Show logs"},
        {"role": "system", "content": "Different"},
    ]
    assert _dedupe_system_input_messages(filtered, "") == (filtered, 0)
    assert _normalize_for_dedupe("") == ""
    assert _normalize_for_dedupe("  MIXED\nCase  Value ") == "mixed case value"
    assert _string_attr_truthy("Yes") is True
    assert _string_attr_truthy("off") is False
    assert (
        _first_message_content(
            [{"role": "assistant", "content": " "}, {"role": "user", "content": "Question"}],
            ("user",),
        )
        == "Question"
    )
    assert _first_message_content([], ("assistant",)) == ""
    assert _summarize_ai_tool_action("") == ""
    assert _summarize_ai_tool_action("not-json") == "not-json"
    assert _summarize_ai_tool_action(json.dumps([1, 2, 3])) == "[1, 2, 3]"
    assert _summarize_ai_tool_action(
        json.dumps({"type": "apply_sql_filter", "sql_where": "SeverityText = 'ERROR'"})
    ) == ("apply_sql_filter: SeverityText = 'ERROR'")
    assert _summarize_ai_tool_action(json.dumps({"type": "navigate", "target_page": "/logs"})) == ("navigate -> /logs")
    assert _summarize_ai_tool_action(json.dumps({"type": "open_modal"})) == "open_modal"


def test_build_ai_trace_turn_cards_aggregates_and_sorts_turns() -> None:
    spans: list[dict[str, Any]] = [
        {"event_name": "turn.start"},
        {
            "turn_id": "turn-b",
            "chat_id": "chat-2",
            "provider": "openai",
            "model": "gpt-4o-mini",
            "trace_id": "trace-b",
            "event_name": "turn.blocked",
            "ts": "2026-01-01T00:00:03Z",
            "error_message": "unsafe request",
            "tokens_in": 1,
            "tokens_out": 2,
            "thinking_tokens": 3,
            "duration_ms": 12,
            "prompt": "blocked prompt",
            "response": "blocked response",
        },
        {
            "turn_id": "turn-a",
            "chat_id": "chat-1",
            "provider": "openai",
            "model": "gpt-4o",
            "trace_id": "trace-a",
            "event_name": "turn.start",
            "ts": "2026-01-01T00:00:00Z",
            "input_question": "Why is latency high?",
            "tokens_in": 10,
            "duration_ms": 100,
        },
        {
            "turn_id": "turn-a",
            "event_name": "guard.result",
            "ts": "2026-01-01T00:00:01Z",
            "guard_allowed": "yes",
            "guard_reason": "safe",
            "thinking_tokens": 4,
        },
        {
            "turn_id": "turn-a",
            "event_name": "tool.proposed",
            "ts": "2026-01-01T00:00:02Z",
            "tool_name": "propose_ui_action",
            "tool_action_id": "action-1",
            "tool_action": json.dumps({"type": "apply_sql_filter", "sql_where": "SeverityText = 'ERROR'"}),
            "tokens_out": 3,
            "duration_ms": 25.4,
        },
        {
            "turn_id": "turn-a",
            "event_name": "tool.proposed",
            "ts": "2026-01-01T00:00:02Z",
            "tool_name": "propose_ui_action",
            "tool_action_id": "action-1",
            "tool_action": json.dumps({"type": "apply_sql_filter", "sql_where": "SeverityText = 'ERROR'"}),
        },
        {
            "turn_id": "turn-a",
            "event_name": "tool.executed",
            "ts": "2026-01-01T00:00:03Z",
            "tool_action_id": "action-1",
            "tool_summary": "Filter logs to errors",
        },
        {
            "turn_id": "turn-a",
            "event_name": "turn.complete",
            "ts": "2026-01-01T00:00:04Z",
            "output_messages": [{"role": "assistant", "content": "Filter applied."}],
            "turn_summary_action": "Inspect logs",
            "tokens_out": 7,
            "duration_ms": 80.2,
        },
        {
            "turn_id": "turn-c",
            "chat_id": "chat-3",
            "provider": "anthropic",
            "model": "claude",
            "trace_id": "trace-c",
            "event_name": "turn.cancelled",
            "ts": "2026-01-01T00:00:05Z",
            "input_messages": [{"role": "user", "content": "cancel this"}],
        },
        {
            "turn_id": "turn-d",
            "chat_id": "chat-4",
            "provider": "anthropic",
            "model": "claude",
            "trace_id": "trace-d",
            "event_name": "turn.error",
            "ts": "2026-01-01T00:00:06Z",
            "prompt": "fail please",
            "response": "temporary response",
            "turn_summary_request": "Explicit request",
            "turn_summary_result": "Explicit result",
        },
        {
            "turn_id": "turn-e",
            "event_name": "tool.proposed",
            "ts": "2026-01-01T00:00:08Z",
            "tool_action": json.dumps({"type": "navigate", "target_page": "/traces"}),
        },
        {
            "turn_id": "turn-e",
            "chat_id": "chat-5",
            "provider": "openai",
            "model": "gpt-4.1",
            "trace_id": "trace-e",
            "event_name": "turn.complete",
            "ts": "2026-01-01T00:00:07Z",
            "input_messages": [{"role": "user", "content": "Backfill metadata"}],
            "response": "Done",
        },
    ]

    cards = _build_ai_trace_turn_cards(spans)

    assert [card["turn_id"] for card in cards] == ["turn-a", "turn-b", "turn-c", "turn-d", "turn-e"]

    first = cards[0]
    assert first["index"] == 1
    assert first["status"] == "completed"
    assert first["chat_id"] == "chat-1"
    assert first["trace_id"] == "trace-a"
    assert first["user_message"] == "Why is latency high?"
    assert first["assistant_message"] == "Filter applied."
    assert first["request_summary"] == "Why is latency high?"
    assert first["action_summary"] == "Inspect logs"
    assert first["result_summary"] == "Filter applied."
    assert first["guard_allowed"] is True
    assert first["guard_reason"] == "safe"
    assert first["tokens_in"] == 10
    assert first["tokens_out"] == 10
    assert first["thinking_tokens"] == 4
    assert first["duration_ms"] == 205.6
    assert first["tool_count"] == 2
    assert first["event_names"] == ["turn.start", "guard.result", "tool.proposed", "tool.executed", "turn.complete"]
    assert first["tools"] == [
        {
            "name": "propose_ui_action",
            "status": "proposed",
            "summary": "apply_sql_filter: SeverityText = 'ERROR'",
            "action_id": "action-1",
        },
        {
            "name": "propose_ui_action",
            "status": "executed",
            "summary": "Filter logs to errors",
            "action_id": "action-1",
        },
    ]

    blocked = cards[1]
    assert blocked["status"] == "blocked"
    assert blocked["guard_reason"] == "unsafe request"
    assert blocked["request_summary"] == "blocked prompt"
    assert blocked["result_summary"] == "blocked response"

    cancelled = cards[2]
    assert cancelled["status"] == "cancelled"
    assert cancelled["request_summary"] == "cancel this"
    assert cancelled["result_summary"] == ""

    failed = cards[3]
    assert failed["status"] == "failed"
    assert failed["request_summary"] == "Explicit request"
    assert failed["result_summary"] == "Explicit result"

    filled = cards[4]
    assert filled["status"] == "completed"
    assert filled["chat_id"] == "chat-5"
    assert filled["provider"] == "openai"
    assert filled["model"] == "gpt-4.1"
    assert filled["trace_id"] == "trace-e"
    assert filled["started_at"] == "2026-01-01T00:00:07Z"
    assert filled["tools"] == [
        {
            "name": "propose_ui_action",
            "status": "proposed",
            "summary": "navigate -> /traces",
            "action_id": "",
        }
    ]
