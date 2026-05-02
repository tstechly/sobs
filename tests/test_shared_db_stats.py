from __future__ import annotations

from collections.abc import Sequence
from typing import TypeAlias

from shared.db_stats import _active_part_rows, _fmt_bytes, _get_db_stats


class _FakeResult:
    def __init__(self, *, one: dict | None = None, many: Sequence[dict] | None = None):
        self._one = one
        self._many = list(many or [])

    def fetchone(self) -> dict | None:
        return self._one

    def fetchall(self) -> list[dict]:
        return list(self._many)


_FakeStep: TypeAlias = _FakeResult | Exception


class _FakeDb:
    def __init__(self, steps: Sequence[_FakeStep]):
        self._steps = list(steps)
        self.calls: list[tuple[str, list[object]]] = []

    def execute(self, query: str, params: list[object] | None = None) -> _FakeResult:
        self.calls.append((query, list(params or [])))
        step = self._steps.pop(0)
        if isinstance(step, Exception):
            raise step
        return step


def test_active_part_rows_returns_count_and_uses_table_param() -> None:
    db = _FakeDb([_FakeResult(one={"c": 42})])

    result = _active_part_rows(db, "otel_logs")

    assert result == 42
    assert db.calls == [
        (
            "SELECT COALESCE(sum(rows), 0) AS c FROM system.parts "
            "WHERE active = 1 AND database = currentDatabase() AND table = ?",
            ["otel_logs"],
        )
    ]


def test_active_part_rows_returns_zero_when_no_row() -> None:
    assert _active_part_rows(_FakeDb([_FakeResult(one=None)]), "otel_traces") == 0


def test_get_db_stats_collects_overall_table_and_process_metrics() -> None:
    db = _FakeDb(
        [
            _FakeResult(one={"comp": 100, "uncomp": 300, "rws": 7}),
            _FakeResult(
                many=[
                    {"table": "otel_logs", "comp": 50, "uncomp": 200, "rws": 5},
                    {"table": "otel_traces", "comp": 0, "uncomp": 20, "rws": 2},
                ]
            ),
            _FakeResult(one={"cnt": 3}),
        ]
    )
    debug_calls: list[tuple[str, bool]] = []

    result = _get_db_stats(db, log_debug=lambda message, exc_info=False: debug_calls.append((message, exc_info)))

    assert result == {
        "compressed_bytes": 100,
        "uncompressed_bytes": 300,
        "compression_ratio": 3.0,
        "total_rows": 7,
        "active_queries": 3,
        "tables": [
            {
                "table": "otel_logs",
                "compressed_bytes": 50,
                "uncompressed_bytes": 200,
                "rows": 5,
                "compression_ratio": 4.0,
            },
            {
                "table": "otel_traces",
                "compressed_bytes": 0,
                "uncompressed_bytes": 20,
                "rows": 2,
                "compression_ratio": None,
            },
        ],
    }
    assert debug_calls == []


def test_get_db_stats_preserves_defaults_when_rows_are_missing() -> None:
    result = _get_db_stats(
        _FakeDb(
            [
                _FakeResult(one=None),
                _FakeResult(many=[]),
                _FakeResult(one=None),
            ]
        ),
        log_debug=lambda *_args, **_kwargs: None,
    )

    assert result == {
        "compressed_bytes": None,
        "uncompressed_bytes": None,
        "compression_ratio": None,
        "total_rows": None,
        "active_queries": None,
        "tables": [],
    }


def test_get_db_stats_logs_and_returns_safe_defaults_on_failures() -> None:
    debug_calls: list[tuple[str, bool]] = []
    result = _get_db_stats(
        _FakeDb(
            [
                RuntimeError("parts failed"),
                RuntimeError("table failed"),
                RuntimeError("processes failed"),
            ]
        ),
        log_debug=lambda message, exc_info=False: debug_calls.append((message, exc_info)),
    )

    assert result == {
        "compressed_bytes": None,
        "uncompressed_bytes": None,
        "compression_ratio": None,
        "total_rows": None,
        "active_queries": None,
        "tables": [],
    }
    assert debug_calls == [
        ("db_stats: system.parts query failed", True),
        ("db_stats: per-table system.parts query failed", True),
        ("db_stats: system.processes query failed", True),
    ]


def test_fmt_bytes_formats_each_size_band() -> None:
    assert _fmt_bytes(None) == "—"
    assert _fmt_bytes(512) == "512 B"
    assert _fmt_bytes(2048) == "2.0 KB"
    assert _fmt_bytes(2 * 1024 * 1024) == "2.0 MB"
    assert _fmt_bytes(2 * 1024 * 1024 * 1024) == "2.0 GB"
