from datetime import datetime, timezone

from shared.raw_metrics_window import (
    _ensure_raw_metrics_retention,
    _list_trace_overlapping_raw_windows,
    _register_raw_window,
    _run_raw_window_copy_worker,
    _window_copy_counts,
)


class _FakeCursor:
    def __init__(self, value):
        self.value = value

    def fetchall(self):
        return self.value

    def fetchone(self):
        if isinstance(self.value, list):
            return self.value[0] if self.value else None
        return self.value


class _ScriptedDb:
    def __init__(self, steps):
        self.steps = list(steps)
        self.calls = []

    def execute(self, query, params=None):
        self.calls.append((query, params))
        if not self.steps:
            raise AssertionError(f"Unexpected query: {query}")
        expected, result = self.steps.pop(0)
        assert expected in query, f"Expected {expected!r} in {query!r}"
        if isinstance(result, Exception):
            raise result
        return _FakeCursor(result)


class _Logger:
    def __init__(self):
        self.messages = []

    def debug(self, message, *args, **kwargs):
        self.messages.append((message, args, kwargs))


def test_shared_raw_metrics_window_ensure_retention_runs_all_statements_and_ignores_failures():
    logger = _Logger()
    db = _ScriptedDb(
        [
            ("ALTER TABLE otel_metrics_gauge", []),
            ("ALTER TABLE otel_metrics_sum", RuntimeError("skip")),
            ("ALTER TABLE otel_metrics_histogram", []),
            ("ALTER TABLE otel_metrics_gauge_pinned", []),
            ("ALTER TABLE otel_metrics_sum_pinned", []),
            ("ALTER TABLE otel_metrics_histogram_pinned", []),
        ]
    )

    _ensure_raw_metrics_retention(db, baseline_ttl_hours=48, pinned_ttl_days=14, logger=logger)

    assert len(db.calls) == 6
    assert any("raw metrics retention alter skipped" in message for message, _, _ in logger.messages)


def test_shared_raw_metrics_window_register_raw_window_inserts_trimmed_row():
    inserts = []
    signal_ts = datetime(2024, 6, 1, 12, 0, 0, tzinfo=timezone.utc)
    window_id = _register_raw_window(
        object(),
        signal_ts=signal_ts,
        signal_type="signal" * 20,
        signal_ref="ref" * 100,
        service_name="service" * 30,
        namespace="namespace" * 30,
        node_name="node" * 40,
        raw_metrics_window_minutes=5,
        insert_rows_json_each_row=lambda db, table, rows: inserts.append((table, rows)),
        now_ms=123456789,
    )

    assert len(window_id) == 32
    assert inserts[0][0] == "sobs_raw_windows"
    row = inserts[0][1][0]
    assert row["Version"] == 123456789
    assert len(row["SignalType"]) == 64
    assert len(row["SignalRef"]) == 256
    assert len(row["ServiceName"]) == 128
    assert len(row["Namespace"]) == 128
    assert len(row["NodeName"]) == 128
    assert row["WindowStart"].startswith("2024-06-01 11:55:00")
    assert row["WindowEnd"].startswith("2024-06-01 12:05:00")


def test_shared_raw_metrics_window_copy_counts_and_overlap_listing_cover_empty_and_complete_states():
    assert _window_copy_counts(object(), []) == {}

    db = _ScriptedDb(
        [
            (
                "FROM sobs_raw_windows FINAL",
                [
                    {
                        "Id": "win-1",
                        "SignalType": "anomaly",
                        "SignalRef": "sig-1",
                        "ServiceName": "svc",
                        "Namespace": "default",
                        "NodeName": "node-a",
                        "WindowStart": "2024-06-01 11:55:00.000",
                        "WindowEnd": "2024-06-01 12:05:00.000",
                    }
                ],
            ),
            ("FROM sobs_raw_window_copy_state FINAL", [{"WindowId": "win-1", "c": 3}]),
        ]
    )

    listed = _list_trace_overlapping_raw_windows(
        db,
        service_names=["svc"],
        start_ts="2024-06-01T11:59:00+00:00",
        end_ts="2024-06-01T12:01:00+00:00",
        limit=250,
        raw_metric_tables=("gauge", "sum", "hist"),
    )

    assert listed == [
        {
            "id": "win-1",
            "signal_type": "anomaly",
            "signal_ref": "sig-1",
            "service_name": "svc",
            "namespace": "default",
            "node_name": "node-a",
            "window_start": "2024-06-01 11:55:00.000",
            "window_end": "2024-06-01 12:05:00.000",
            "copied_count": 3,
            "expected_count": 3,
            "copy_complete": True,
        }
    ]
    assert db.calls[0][1][-1] == 100


def test_shared_raw_metrics_window_overlap_listing_returns_empty_when_no_rows_match():
    db = _ScriptedDb([("FROM sobs_raw_windows FINAL", [])])
    assert (
        _list_trace_overlapping_raw_windows(
            db,
            service_names=[],
            start_ts="2024-06-01T11:59:00+00:00",
            end_ts="2024-06-01T12:01:00+00:00",
            raw_metric_tables=("gauge", "sum", "hist"),
        )
        == []
    )


def test_shared_raw_metrics_window_copy_worker_handles_fetch_failure_and_empty_windows():
    logger = _Logger()

    failed_db = _ScriptedDb([("FROM sobs_raw_windows FINAL", RuntimeError("boom"))])
    assert _run_raw_window_copy_worker(
        failed_db,
        raw_window_copy_max_per_run=10,
        raw_metric_tables=("otel_metrics_gauge",),
        pinned_metric_tables=("otel_metrics_gauge_pinned",),
        insert_rows_json_each_row=lambda db, table, rows: None,
        now_ms=1,
        logger=logger,
    ) == {"windows_attempted": 0, "copies_ok": 0, "copies_error": 0}

    empty_db = _ScriptedDb([("FROM sobs_raw_windows FINAL", [])])
    assert _run_raw_window_copy_worker(
        empty_db,
        raw_window_copy_max_per_run=10,
        raw_metric_tables=("otel_metrics_gauge",),
        pinned_metric_tables=("otel_metrics_gauge_pinned",),
        insert_rows_json_each_row=lambda db, table, rows: None,
        now_ms=1,
        logger=logger,
    ) == {"windows_attempted": 0, "copies_ok": 0, "copies_error": 0}


def test_shared_raw_metrics_window_copy_worker_skips_already_copied_and_zero_matches():
    logger = _Logger()
    db = _ScriptedDb(
        [
            (
                "FROM sobs_raw_windows FINAL",
                [
                    {
                        "Id": "win-1",
                        "WindowStart": "2024-06-01 11:55:00.000",
                        "WindowEnd": "2024-06-01 12:05:00.000",
                        "ServiceName": "svc",
                        "Namespace": "",
                        "NodeName": "",
                    }
                ],
            ),
            ("FROM sobs_raw_window_copy_state FINAL", {"already": 1}),
            ("FROM sobs_raw_window_copy_state FINAL", None),
            ("SELECT count() AS cnt FROM otel_metrics_sum", {"cnt": 0}),
            ("FROM sobs_raw_window_copy_state FINAL", None),
            ("SELECT count() AS cnt FROM otel_metrics_histogram", {"cnt": 0}),
        ]
    )

    stats = _run_raw_window_copy_worker(
        db,
        raw_window_copy_max_per_run=10,
        raw_metric_tables=("otel_metrics_gauge", "otel_metrics_sum", "otel_metrics_histogram"),
        pinned_metric_tables=(
            "otel_metrics_gauge_pinned",
            "otel_metrics_sum_pinned",
            "otel_metrics_histogram_pinned",
        ),
        insert_rows_json_each_row=lambda db, table, rows: None,
        now_ms=1,
        logger=logger,
    )

    assert stats == {"windows_attempted": 2, "copies_ok": 0, "copies_error": 0}


def test_shared_raw_metrics_window_copy_worker_backfills_state_and_records_copy_errors():
    logger = _Logger()
    inserts = []
    db = _ScriptedDb(
        [
            (
                "FROM sobs_raw_windows FINAL",
                [
                    {
                        "Id": "win-1",
                        "WindowStart": "2024-06-01 11:55:00.000",
                        "WindowEnd": "2024-06-01 12:05:00.000",
                        "ServiceName": "svc",
                        "Namespace": "ns",
                        "NodeName": "node-a",
                    }
                ],
            ),
            ("FROM sobs_raw_window_copy_state FINAL", RuntimeError("state lookup failed")),
            ("FROM sobs_raw_window_copy_state FINAL", None),
            ("SELECT count() AS cnt FROM otel_metrics_sum", {"cnt": 1}),
            ("FROM otel_metrics_sum_pinned", {"cnt": 1}),
            ("INSERT INTO otel_metrics_sum_pinned", []),
            ("FROM sobs_raw_window_copy_state FINAL", None),
            ("SELECT count() AS cnt FROM otel_metrics_histogram", {"cnt": 1}),
            ("FROM otel_metrics_histogram_pinned", {"cnt": 1}),
            ("INSERT INTO otel_metrics_histogram_pinned", RuntimeError("copy failed")),
        ]
    )

    stats = _run_raw_window_copy_worker(
        db,
        raw_window_copy_max_per_run=10,
        raw_metric_tables=("otel_metrics_gauge", "otel_metrics_sum", "otel_metrics_histogram"),
        pinned_metric_tables=(
            "otel_metrics_gauge_pinned",
            "otel_metrics_sum_pinned",
            "otel_metrics_histogram_pinned",
        ),
        insert_rows_json_each_row=lambda db, table, rows: inserts.append((table, rows)),
        now_ms=222,
        logger=logger,
    )

    assert stats == {"windows_attempted": 2, "copies_ok": 1, "copies_error": 1}
    assert inserts == [
        (
            "sobs_raw_window_copy_state",
            [{"WindowId": "win-1", "SourceTable": "otel_metrics_sum", "Version": 222}],
        )
    ]
    assert any("failed to check copy state" in message for message, _, _ in logger.messages)
    assert any("raw window copy error" in message for message, _, _ in logger.messages)


def test_shared_raw_metrics_window_copy_worker_stops_after_max_per_run():
    logger = _Logger()
    inserts = []
    db = _ScriptedDb(
        [
            (
                "FROM sobs_raw_windows FINAL",
                [
                    {
                        "Id": "win-1",
                        "WindowStart": "2024-06-01 11:55:00.000",
                        "WindowEnd": "2024-06-01 12:05:00.000",
                        "ServiceName": "",
                        "Namespace": "",
                        "NodeName": "",
                    },
                    {
                        "Id": "win-2",
                        "WindowStart": "2024-06-01 12:55:00.000",
                        "WindowEnd": "2024-06-01 13:05:00.000",
                        "ServiceName": "",
                        "Namespace": "",
                        "NodeName": "",
                    },
                ],
            ),
            ("FROM sobs_raw_window_copy_state FINAL", None),
            ("SELECT count() AS cnt FROM otel_metrics_gauge", {"cnt": 1}),
            ("FROM otel_metrics_gauge_pinned", {"cnt": 0}),
        ]
    )

    stats = _run_raw_window_copy_worker(
        db,
        raw_window_copy_max_per_run=1,
        raw_metric_tables=("otel_metrics_gauge", "otel_metrics_sum"),
        pinned_metric_tables=("otel_metrics_gauge_pinned", "otel_metrics_sum_pinned"),
        insert_rows_json_each_row=lambda db, table, rows: inserts.append((table, rows)),
        now_ms=444,
        logger=logger,
    )

    assert stats == {"windows_attempted": 1, "copies_ok": 1, "copies_error": 0}
    assert inserts == [
        (
            "sobs_raw_window_copy_state",
            [{"WindowId": "win-1", "SourceTable": "otel_metrics_gauge", "Version": 444}],
        )
    ]


def test_shared_raw_metrics_window_copy_worker_backfills_state_when_missing_rows_are_zero():
    logger = _Logger()
    inserts = []
    db = _ScriptedDb(
        [
            (
                "FROM sobs_raw_windows FINAL",
                [
                    {
                        "Id": "win-2",
                        "WindowStart": "2024-06-01 11:55:00.000",
                        "WindowEnd": "2024-06-01 12:05:00.000",
                        "ServiceName": "",
                        "Namespace": "",
                        "NodeName": "",
                    }
                ],
            ),
            ("FROM sobs_raw_window_copy_state FINAL", None),
            ("SELECT count() AS cnt FROM otel_metrics_gauge", {"cnt": 2}),
            ("FROM otel_metrics_gauge_pinned", {"cnt": 0}),
        ]
    )

    stats = _run_raw_window_copy_worker(
        db,
        raw_window_copy_max_per_run=10,
        raw_metric_tables=("otel_metrics_gauge",),
        pinned_metric_tables=("otel_metrics_gauge_pinned",),
        insert_rows_json_each_row=lambda db, table, rows: inserts.append((table, rows)),
        now_ms=333,
        logger=logger,
    )

    assert stats == {"windows_attempted": 1, "copies_ok": 1, "copies_error": 0}
    assert inserts == [
        (
            "sobs_raw_window_copy_state",
            [{"WindowId": "win-2", "SourceTable": "otel_metrics_gauge", "Version": 333}],
        )
    ]
