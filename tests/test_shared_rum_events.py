import hashlib
import json
from typing import Any

from shared.rum_events import _build_rum_event_item, _rum_session_key_from_attrs


class _RowWithoutKeys:
    def __init__(self, values: dict[str, Any]) -> None:
        self._values = values

    def __getitem__(self, key: str) -> Any:
        return self._values[key]


def test_rum_session_key_from_attrs_prefers_existing_ids() -> None:
    assert _rum_session_key_from_attrs({"sessionId": "sess-123"}, "2026-01-01T00:00:00Z", "{}") == "sess-123"
    assert _rum_session_key_from_attrs({"session.id": "legacy-456"}, "2026-01-01T00:00:00Z", "{}") == "legacy-456"


def test_rum_session_key_from_attrs_falls_back_to_deterministic_hash() -> None:
    body_raw = '{"path":"/checkout"}'
    expected = hashlib.md5('2026-01-01T00:00:00Z|{"path":"/checkout"}'.encode("utf-8")).hexdigest()[:16]

    assert _rum_session_key_from_attrs({}, "2026-01-01T00:00:00Z", body_raw) == f"anon:{expected}"


def test_build_rum_event_item_uses_row_columns_and_body_dict() -> None:
    row = {
        "LogAttributes": {"sessionId": "sess-123", "url": "https://example.test/path"},
        "Body": json.dumps(
            {
                "artifact": {"id": "artifact-1"},
                "replay": {"url": "https://example.test/replay"},
            }
        ),
        "TraceId": "trace-123",
        "SpanId": "span-123",
        "ServiceName": "frontend",
        "Timestamp": "2026-01-01T00:00:00Z",
        "EventName": "pageview",
    }

    item = _build_rum_event_item(row, map_to_dict=lambda value: dict(value or {}))

    assert item == {
        "ts": "2026-01-01T00:00:00Z",
        "session_key": "sess-123",
        "session_id": "sess-123",
        "event_type": "pageview",
        "url": "https://example.test/path",
        "data": {
            "artifact": {"id": "artifact-1"},
            "replay": {"url": "https://example.test/replay"},
            "traceId": "trace-123",
            "spanId": "span-123",
        },
        "trace_id": "trace-123",
        "span_id": "span-123",
        "service": "frontend",
        "has_artifact": True,
        "has_replay": True,
    }


def test_build_rum_event_item_falls_back_to_body_fields_and_handles_non_dict_bodies() -> None:
    mapped_attrs = {"url.full": "https://example.test/fallback"}
    row = _RowWithoutKeys(
        {
            "LogAttributes": "ignored",
            "Body": '["event", 1]',
            "Timestamp": "2026-01-01T00:00:01Z",
            "EventName": "error",
        }
    )

    item = _build_rum_event_item(row, map_to_dict=lambda _value: mapped_attrs)

    expected_session = hashlib.md5('2026-01-01T00:00:01Z|["event", 1]'.encode("utf-8")).hexdigest()[:16]
    assert item == {
        "ts": "2026-01-01T00:00:01Z",
        "session_key": f"anon:{expected_session}",
        "session_id": f"anon:{expected_session}"[:8],
        "event_type": "error",
        "url": "https://example.test/fallback",
        "data": {"value": ["event", 1]},
        "trace_id": "",
        "span_id": "",
        "service": "",
        "has_artifact": False,
        "has_replay": False,
    }


def test_build_rum_event_item_ignores_invalid_json_and_preserves_existing_trace_fields() -> None:
    row = {
        "LogAttributes": {"session.id": "sess-legacy", "url.full": "https://example.test/body"},
        "Body": "{",
        "TraceId": "trace-row",
        "SpanId": "span-row",
        "ServiceName": "frontend",
        "Timestamp": "2026-01-01T00:00:02Z",
        "EventName": "resource",
    }

    item = _build_rum_event_item(row, map_to_dict=lambda value: dict(value or {}))

    assert item["session_key"] == "sess-legacy"
    assert item["data"] == {"traceId": "trace-row", "spanId": "span-row"}
    assert item["trace_id"] == "trace-row"
    assert item["span_id"] == "span-row"
    assert item["service"] == "frontend"
