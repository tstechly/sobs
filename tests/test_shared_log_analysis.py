from __future__ import annotations

from collections.abc import Sequence

from shared.log_analysis import _compute_advanced_log_analysis, _compute_log_stats, _fingerprint_log_message


class _FakeCursor:
    def __init__(self, rows: Sequence[dict]):
        self._rows = list(rows)

    def fetchall(self) -> list[dict]:
        return list(self._rows)


class _FakeDb:
    def __init__(self, responses: list[list[dict]]):
        self._responses = list(responses)
        self.calls: list[tuple[str, list[object]]] = []

    def execute(self, query: str, params: list[object]) -> _FakeCursor:
        self.calls.append((query, list(params)))
        return _FakeCursor(self._responses.pop(0))


def test_compute_log_stats_with_where_clause_uses_scoped_queries() -> None:
    db = _FakeDb(
        [
            [{"SeverityText": "ERROR", "cnt": 2}, {"SeverityText": None, "cnt": 1}],
            [{"ServiceName": "svc-a", "cnt": 3}],
        ]
    )

    level_stats, service_stats = _compute_log_stats(db, "WHERE ServiceName = ?", ["svc-a"])

    assert level_stats == {"ERROR": 2, "UNKNOWN": 1}
    assert service_stats == {"svc-a": 3}
    assert len(db.calls) == 2
    assert "FROM otel_logs WHERE ServiceName = ?" in db.calls[0][0]
    assert "AND ServiceName!=''" in db.calls[1][0]
    assert db.calls[0][1] == ["svc-a"]
    assert db.calls[1][1] == ["svc-a"]


def test_compute_log_stats_without_where_clause_adds_service_filter() -> None:
    db = _FakeDb(
        [
            [{"SeverityText": "INFO", "cnt": 4}],
            [{"ServiceName": "svc-b", "cnt": 4}],
        ]
    )

    level_stats, service_stats = _compute_log_stats(db, "", [])

    assert level_stats == {"INFO": 4}
    assert service_stats == {"svc-b": 4}
    assert "FROM otel_logs  WHERE ServiceName!=''" in db.calls[1][0]


def test_fingerprint_log_message_normalizes_dynamic_values() -> None:
    message = (
        " User 1234 hit request id 550e8400-e29b-41d4-a716-446655440000 "
        "with hash abcdef0123456789 and addr 0xdeadbeef saying \"boom\" and 'oops'"
    )

    fingerprint = _fingerprint_log_message(message)

    assert fingerprint == (
        "user <num> hit request id <uuid> with hash <hash> and addr <hex> " "saying \"<text>\" and '<text>'"
    )


def test_fingerprint_log_message_empty_and_truncated() -> None:
    assert _fingerprint_log_message("") == "(empty message)"
    assert len(_fingerprint_log_message("x" * 300)) == 160


def test_compute_advanced_log_analysis_returns_empty_shape_without_messages() -> None:
    result = _compute_advanced_log_analysis([], {}, {}, map_to_dict=lambda value: value or {})

    assert result == {
        "top_patterns": [],
        "top_keywords": [],
        "error_families": [],
        "hints": [],
    }


def test_compute_advanced_log_analysis_builds_families_keywords_and_hints() -> None:
    rows = [
        {
            "Body": "Timeout waiting for db request 1234 ConnectionRefusedFailure",
            "LogAttributes": {"exception.type": "BackendUnavailableError"},
        },
        {
            "Body": "Timeout waiting for db request 5678 ConnectionRefusedFailure",
            "LogAttributes": {},
        },
        {
            "Body": "Timeout waiting for db request 9999 ConnectionRefusedFailure",
            "LogAttributes": {},
        },
        {
            "Body": "Background sync completed",
            "LogAttributes": {},
        },
    ]

    result = _compute_advanced_log_analysis(
        rows,
        {"ERROR": 3, "INFO": 1},
        {"svc-a": 4},
        map_to_dict=lambda value: value or {},
    )

    assert result["top_patterns"][0] == {
        "pattern": "timeout waiting for db request <num> connectionrefusedfailure",
        "count": 3,
    }
    assert {item["family"] for item in result["error_families"]} >= {
        "BackendUnavailableError",
        "ConnectionRefusedFailure",
    }
    assert result["top_keywords"][0] == {"keyword": "timeout", "count": 3}
    assert any("High severe-log ratio (75%)" in hint for hint in result["hints"])
    assert any("Most frequent message pattern repeats 3 times" in hint for hint in result["hints"])
    assert any("Timeout-related logs are common" in hint for hint in result["hints"])
    assert any("Most events come from svc-a" in hint for hint in result["hints"])
