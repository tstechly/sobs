from __future__ import annotations

import json
from datetime import datetime, timezone
from typing import Any

from shared.storage_write import _WRITABLE_TABLES, _insert_rows_json_each_row, _normalize_ch_timestamp


class _Db:
    def __init__(self) -> None:
        self.queries: list[str] = []

    def execute(self, query: str) -> None:
        self.queries.append(query)


class _SpanRecorder:
    def __init__(self, calls: list[tuple[str, dict[str, Any]]]) -> None:
        self.calls = calls

    def __enter__(self) -> None:
        return None

    def __exit__(self, exc_type, exc, tb) -> None:
        return None


def test_allowlist_contains_expected_tables() -> None:
    assert "otel_traces" in _WRITABLE_TABLES
    assert "sobs_release_artifacts" in _WRITABLE_TABLES


def test_normalize_ch_timestamp_handles_datetime_blank_and_invalid_values() -> None:
    aware = datetime(2024, 1, 1, 12, 30, tzinfo=timezone.utc)
    blank_now = datetime(2025, 2, 3, 4, 5, 6, 789000, tzinfo=timezone.utc)

    assert _normalize_ch_timestamp(aware) == "2024-01-01 12:30:00.000000"
    assert _normalize_ch_timestamp("", now_utc=lambda: blank_now) == "2025-02-03 04:05:06.789000"
    assert _normalize_ch_timestamp("2024-01-01T00:00:00Z") == "2024-01-01 00:00:00.000000"
    assert _normalize_ch_timestamp("2024-01-01T12:00:00+02:00") == "2024-01-01 10:00:00.000000"
    assert _normalize_ch_timestamp("2024-01-01T12:00:00") == "2024-01-01 12:00:00.000000"
    assert _normalize_ch_timestamp("not-a-timestamp") == "not-a-timestamp"


def test_insert_rows_json_each_row_returns_zero_for_empty_rows() -> None:
    db = _Db()

    assert _insert_rows_json_each_row(db, "otel_logs", []) == 0
    assert db.queries == []


def test_insert_rows_json_each_row_rejects_unregistered_tables() -> None:
    db = _Db()

    try:
        _insert_rows_json_each_row(db, "not_allowed", [{"x": 1}])
    except ValueError as exc:
        assert "unregistered table" in str(exc)
    else:
        raise AssertionError("expected ValueError for unregistered table")


def test_insert_rows_json_each_row_normalizes_rows_and_records_span() -> None:
    db = _Db()
    spans: list[tuple[str, dict[str, Any]]] = []

    def _record_span(name: str, attrs: dict[str, Any]) -> _SpanRecorder:
        spans.append((name, attrs))
        return _SpanRecorder(spans)

    inserted = _insert_rows_json_each_row(
        db,
        "otel_logs",
        [
            {
                "Timestamp": "2024-01-01T00:00:00Z",
                "Events": {"Timestamp": ["2024-01-01T00:00:01Z", "2024-01-01T00:00:02+00:00"]},
                "Message": "hello",
            }
        ],
        span_factory=_record_span,
    )

    assert inserted == 1
    assert spans == [
        (
            "sobs.storage.write",
            {"storage.engine": "chdb", "table": "otel_logs", "row.count": 1},
        )
    ]
    assert len(db.queries) == 1
    payload = db.queries[0].split("\n", 1)[1]
    row = json.loads(payload)
    assert row["Timestamp"] == "2024-01-01 00:00:00.000000"
    assert row["Events"]["Timestamp"] == ["2024-01-01 00:00:01.000000", "2024-01-01 00:00:02.000000"]
    assert row["Message"] == "hello"
