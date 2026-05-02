from __future__ import annotations

from shared.error_items import (
    _build_error_item,
    _compact_text,
    _error_group_key,
    _extract_structured_error_summary,
    _try_pretty_json_text,
)


def test_compact_text_preserves_short_text_and_truncates_long_text() -> None:
    assert _compact_text("  short   text  ") == "short text"
    assert _compact_text("abcdefghij", limit=6) == "abcde..."


def test_try_pretty_json_text_handles_non_json_and_valid_json() -> None:
    _try_pretty_json_text.cache_clear()

    assert _try_pretty_json_text("plain text") == (False, "")

    is_json, pretty = _try_pretty_json_text('{"a":1,"b":{"c":2}}')
    assert is_json is True
    assert '"a": 1' in pretty
    assert '"c": 2' in pretty


def test_try_pretty_json_text_rejects_invalid_json_that_starts_like_json() -> None:
    _try_pretty_json_text.cache_clear()

    assert _try_pretty_json_text('{"broken": ') == (False, "")


def test_extract_structured_error_summary_prefers_message_with_type_and_code() -> None:
    summary, from_json = _extract_structured_error_summary(
        '{"outer":{"message":"Bad request","type":"ValidationError","code":400}}',
        "",
    )

    assert summary == "Bad request [ValidationError, code 400]"
    assert from_json is True


def test_extract_structured_error_summary_handles_list_and_scalar_fallbacks() -> None:
    summary, from_json = _extract_structured_error_summary(
        "",
        '[{"name":"TimeoutError","status_code":504}]',
    )

    assert summary == "TimeoutError (code 504)"
    assert from_json is True


def test_extract_structured_error_summary_falls_back_to_json_dump_and_plain_text() -> None:
    summary, from_json = _extract_structured_error_summary('{"unexpected": true}', "")
    assert summary == '{"unexpected": true}'
    assert from_json is True

    summary, from_json = _extract_structured_error_summary("plain failure", "")
    assert summary == "plain failure"
    assert from_json is False


def test_extract_structured_error_summary_handles_type_only_code_only_empty_list_and_invalid_json() -> None:
    summary, from_json = _extract_structured_error_summary('{"type":"TimeoutError"}', "")
    assert summary == "TimeoutError"
    assert from_json is True

    summary, from_json = _extract_structured_error_summary('{"status":504}', "")
    assert summary == "code 504"
    assert from_json is True

    summary, from_json = _extract_structured_error_summary("[]", "")
    assert summary == "[]"
    assert from_json is True

    summary, from_json = _extract_structured_error_summary('{"broken": ', "fallback text")
    assert summary == '{"broken":'
    assert from_json is False


def test_extract_structured_error_summary_handles_nested_lists_and_scalar_top_level_lists() -> None:
    summary, from_json = _extract_structured_error_summary('{"errors":[{"message":"list failure"}]}', "")
    assert summary == "list failure"
    assert from_json is True

    summary, from_json = _extract_structured_error_summary("[1]", "")
    assert summary == "[1]"
    assert from_json is True


def test_extract_structured_error_summary_stops_deep_recursion_without_crashing() -> None:
    deep_payload = '{"a":{"b":{"c":{"d":{"e":{"f":{"message":"too deep"}}}}}}}'

    summary, from_json = _extract_structured_error_summary(deep_payload, "")

    assert summary == '{"a": {"b": {"c": {"d": {"e": {"f": {"message": "too deep"}}}}}}}'
    assert from_json is True


def test_build_error_item_shapes_metadata_and_pretty_json_fields() -> None:
    _try_pretty_json_text.cache_clear()

    row = {
        "Timestamp": "2024-01-01T00:00:00Z",
        "ServiceName": "checkout",
        "Body": '{"error":"raw body","status":503}',
        "TraceId": "trace-1",
        "SpanId": "span-1",
        "LogAttributes": {
            "exception.type": "ValueError",
            "exception.message": '{"message":"bad input","code":400}',
            "exception.stacktrace": '{"stack":"trace"}',
            "url.full": "https://example.test",
            "error.source": "window.onerror",
            "browser.page.title": "Checkout",
            "browser.viewport": "1280x720",
            "artifact.type": "sourcemap",
            "artifact.id": "artifact-1",
            "artifact.url": "https://artifact.test",
            "replay.id": "replay-1",
            "replay.url": "https://replay.test",
        },
    }

    item = _build_error_item(
        row,
        map_to_dict=lambda value: dict(value or {}),
        maybe_demangle_js_stack=lambda stack: stack,
        error_id=lambda ts, service, err_type, message, trace_id, span_id: ":".join(
            [ts, service, err_type, message, trace_id, span_id]
        ),
    )

    assert item["id"] == ('2024-01-01T00:00:00Z:checkout:ValueError:{"message":"bad input","code":400}:trace-1:span-1')
    assert item["message_summary"] == "bad input [code 400]"
    assert item["summary_from_json"] is True
    assert item["message_is_json"] is True
    assert item["raw_body_is_json"] is True
    assert item["stack_is_json"] is True
    assert '"message": "bad input"' in item["message_pretty_json"]
    assert item["url"] == "https://example.test"
    assert item["artifact_type"] == "sourcemap"
    assert item["replay_url"] == "https://replay.test"


def test_build_error_item_uses_defaults_when_optional_fields_are_missing() -> None:
    _try_pretty_json_text.cache_clear()

    item = _build_error_item(
        {"Body": "plain body", "LogAttributes": None},
        map_to_dict=lambda value: dict(value or {}),
        maybe_demangle_js_stack=lambda stack: stack,
        error_id=lambda *parts: "|".join(str(part) for part in parts),
    )

    assert item["err_type"] == "Error"
    assert item["message"] == "plain body"
    assert item["summary_from_json"] is False
    assert item["message_is_json"] is False
    assert item["raw_body_is_json"] is False
    assert item["stack"] == ""


def test_error_group_key_normalizes_whitespace_and_prefers_message_summary() -> None:
    key = _error_group_key(
        {
            "service": "  Checkout   Service  ",
            "err_type": " ValueError ",
            "message_summary": "  Bad   Input  ",
            "message": "ignored",
        }
    )
    assert key == ("checkout service", "valueerror", "bad input")

    fallback_key = _error_group_key({"service": "svc", "err_type": "Err", "message": "Raw  Message"})
    assert fallback_key == ("svc", "err", "raw message")
