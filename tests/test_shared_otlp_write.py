from __future__ import annotations

from typing import Any

from shared.events import ErrorEvent, LogEvent, SpanEvent, TypedMetricEvent
from shared.otlp_write import (
    _insert_error_events,
    _insert_log_events,
    _insert_metric_events,
    _insert_span_events,
    _insert_typed_metric_events,
)


def test_insert_log_events_shapes_rows_and_logs_tag_rule_failures() -> None:
    inserts: list[tuple[str, list[dict[str, Any]]]] = []
    remember_log_calls: list[tuple[str, list[dict[str, Any]]]] = []
    remember_attr_calls: list[tuple[str, list[dict[str, Any]]]] = []
    tag_calls: list[tuple[str, list[dict[str, Any]]]] = []
    exception_messages: list[str] = []

    def insert_rows_json_each_row(_: Any, table: str, rows: list[dict[str, Any]]) -> int:
        inserts.append((table, rows))
        return len(rows)

    def remember_log_attr_keys(_: Any, attrs_maps: list[dict[str, Any]], record_type: str) -> None:
        remember_log_calls.append((record_type, attrs_maps))

    def remember_attr_keys(_: Any, attrs_maps: list[dict[str, Any]], record_type: str) -> None:
        remember_attr_calls.append((record_type, attrs_maps))

    def extract_attr_maps(rows: list[dict[str, Any]], attr_field: str) -> list[dict[str, Any]]:
        return [row[attr_field] for row in rows]

    def apply_tag_rules(_: Any, signal_type: str, rows: list[dict[str, Any]], rules: Any) -> None:
        assert rules == ["tag-rule"]
        tag_calls.append((signal_type, rows))
        raise RuntimeError("boom")

    count = _insert_log_events(
        object(),
        [
            LogEvent(
                ts="2024-01-01T00:00:00Z",
                level="WARN",
                service="checkout",
                body="disk full",
                attrs={"event.name": "deploy", "foo": "bar"},
                resource_attrs={"service.name": "checkout"},
                scope_attrs={"scope.name": "worker"},
                trace_id="trace-1",
                span_id="span-1",
            )
        ],
        stringify_attrs=lambda attrs: {**attrs, "_stringified": True},
        severity_number=lambda level: 13 if level == "WARN" else 0,
        insert_rows_json_each_row=insert_rows_json_each_row,
        remember_log_attr_keys=remember_log_attr_keys,
        remember_attr_keys=remember_attr_keys,
        extract_log_attr_maps=lambda rows: [row["LogAttributes"] for row in rows],
        extract_attr_maps=extract_attr_maps,
        load_tag_rules=lambda _: ["tag-rule"],
        apply_tag_rules=apply_tag_rules,
        log_exception=exception_messages.append,
    )

    assert count == 1
    assert inserts[0][0] == "otel_logs"
    assert inserts[0][1][0]["SeverityNumber"] == 13
    assert inserts[0][1][0]["EventName"] == "deploy"
    assert inserts[0][1][0]["ResourceAttributes"]["_stringified"] is True
    assert remember_log_calls == [("log", [{"event.name": "deploy", "foo": "bar", "_stringified": True}])]
    assert remember_attr_calls == [
        ("resource", [{"service.name": "checkout", "_stringified": True}]),
        ("scope", [{"scope.name": "worker", "_stringified": True}]),
    ]
    assert tag_calls[0][0] == "log"
    assert exception_messages == ["auto-tag application failed for logs"]


def test_insert_span_events_shapes_rows_and_logs_tag_rule_failures() -> None:
    inserts: list[tuple[str, list[dict[str, Any]]]] = []
    remember_attr_calls: list[tuple[str, list[dict[str, Any]]]] = []
    tag_calls: list[tuple[str, list[dict[str, Any]]]] = []
    exception_messages: list[str] = []

    def insert_rows_json_each_row(_: Any, table: str, rows: list[dict[str, Any]]) -> int:
        inserts.append((table, rows))
        return len(rows)

    def remember_attr_keys(_: Any, attrs_maps: list[dict[str, Any]], record_type: str) -> None:
        remember_attr_calls.append((record_type, attrs_maps))

    def extract_attr_maps(rows: list[dict[str, Any]], attr_field: str) -> list[dict[str, Any]]:
        return [row[attr_field] for row in rows]

    def apply_tag_rules(_: Any, signal_type: str, rows: list[dict[str, Any]], rules: Any) -> None:
        assert rules == ["trace-rule"]
        tag_calls.append((signal_type, rows))
        raise RuntimeError("boom")

    count = _insert_span_events(
        object(),
        [
            SpanEvent(
                ts="2024-01-01T00:00:00Z",
                trace_id="trace-1",
                span_id="span-1",
                parent_span_id="parent-1",
                name="GET /health",
                service="api",
                duration_ms=-4.5,
                status="ERROR",
                attrs={"span.kind": "SERVER", "status.message": "bad"},
                resource_attrs={"service.name": "api"},
                scope_attrs={"scope.name": "requests"},
            )
        ],
        stringify_attrs=lambda attrs: {**attrs, "_stringified": True},
        trace_status_code=lambda status: f"code-{status}",
        insert_rows_json_each_row=insert_rows_json_each_row,
        remember_attr_keys=remember_attr_keys,
        extract_attr_maps=extract_attr_maps,
        load_tag_rules=lambda _: ["trace-rule"],
        apply_tag_rules=apply_tag_rules,
        log_exception=exception_messages.append,
    )

    assert count == 1
    assert inserts[0][0] == "otel_traces"
    assert inserts[0][1][0]["Duration"] == 0
    assert inserts[0][1][0]["StatusCode"] == "code-ERROR"
    assert inserts[0][1][0]["SpanKind"] == "SERVER"
    assert remember_attr_calls == [
        ("span", [{"span.kind": "SERVER", "status.message": "bad", "_stringified": True}]),
        ("resource", [{"service.name": "api", "_stringified": True}]),
    ]
    assert tag_calls[0][0] == "trace"
    assert exception_messages == ["auto-tag application failed for traces"]


def test_insert_error_events_shapes_rows_and_logs_tag_rule_failures() -> None:
    inserts: list[tuple[str, list[dict[str, Any]]]] = []
    remember_log_calls: list[tuple[str, list[dict[str, Any]]]] = []
    tag_calls: list[tuple[str, list[dict[str, Any]]]] = []
    exception_messages: list[str] = []

    def insert_rows_json_each_row(_: Any, table: str, rows: list[dict[str, Any]]) -> int:
        inserts.append((table, rows))
        return len(rows)

    def remember_log_attr_keys(_: Any, attrs_maps: list[dict[str, Any]], record_type: str) -> None:
        remember_log_calls.append((record_type, attrs_maps))

    def apply_tag_rules(_: Any, signal_type: str, rows: list[dict[str, Any]], rules: Any) -> None:
        assert rules == ["error-rule"]
        tag_calls.append((signal_type, rows))
        raise RuntimeError("boom")

    _insert_error_events(
        object(),
        [
            ErrorEvent(
                ts="2024-01-01T00:00:00Z",
                service="api",
                err_type="ValueError",
                message="bad input",
                stack="traceback",
                attrs={"foo": "bar"},
                trace_id="trace-1",
                span_id="span-1",
            )
        ],
        stringify_attrs=lambda attrs: {**attrs, "_stringified": True},
        severity_number=lambda level: 17 if level == "ERROR" else 0,
        insert_rows_json_each_row=insert_rows_json_each_row,
        remember_log_attr_keys=remember_log_attr_keys,
        extract_log_attr_maps=lambda rows: [row["LogAttributes"] for row in rows],
        load_tag_rules=lambda _: ["error-rule"],
        apply_tag_rules=apply_tag_rules,
        log_exception=exception_messages.append,
    )

    assert inserts[0][0] == "otel_logs"
    assert inserts[0][1][0]["SeverityNumber"] == 17
    assert inserts[0][1][0]["LogAttributes"] == {
        "foo": "bar",
        "_stringified": True,
        "exception.type": "ValueError",
        "exception.message": "bad input",
        "exception.stacktrace": "traceback",
    }
    assert remember_log_calls == [
        (
            "log",
            [
                {
                    "foo": "bar",
                    "_stringified": True,
                    "exception.type": "ValueError",
                    "exception.message": "bad input",
                    "exception.stacktrace": "traceback",
                }
            ],
        )
    ]
    assert tag_calls[0][0] == "error"
    assert exception_messages == ["auto-tag application failed for errors"]


def test_insert_metric_events_routes_gauge_sum_and_histogram_rows() -> None:
    inserts: list[tuple[str, list[dict[str, Any]]]] = []

    def insert_rows_json_each_row(_: Any, table: str, rows: list[dict[str, Any]]) -> int:
        inserts.append((table, rows))
        return len(rows)

    count = _insert_metric_events(
        object(),
        [
            TypedMetricEvent(
                ts="2024-01-01T00:00:00Z",
                service="metrics",
                metric_name="cpu.usage",
                metric_description="CPU",
                metric_unit="%",
                metric_kind="gauge",
                value=12.5,
                attrs={"host": "a"},
                attr_fp="fp-gauge",
            ),
            TypedMetricEvent(
                ts="2024-01-01T00:00:01Z",
                service="metrics",
                metric_name="http.requests",
                metric_description="Requests",
                metric_unit="1",
                metric_kind="sum",
                value=4.0,
                attrs={"route": "/health"},
                attr_fp="fp-sum",
                is_monotonic=1,
                aggregation_temporality=2,
            ),
            TypedMetricEvent(
                ts="2024-01-01T00:00:02Z",
                service="metrics",
                metric_name="request.duration",
                metric_description="Duration",
                metric_unit="ms",
                metric_kind="histogram",
                value=7.5,
                attrs={"route": "/checkout"},
                attr_fp="fp-hist",
                aggregation_temporality=2,
                histogram_count=3,
                histogram_sum=22.5,
                histogram_buckets=[1, 1, 1],
                histogram_bounds=[5.0, 10.0],
            ),
        ],
        stringify_attrs=lambda attrs: {**attrs, "_stringified": True},
        insert_rows_json_each_row=insert_rows_json_each_row,
    )

    assert count == 3
    assert [table for table, _ in inserts] == [
        "otel_metrics_gauge",
        "otel_metrics_sum",
        "otel_metrics_histogram",
    ]
    assert inserts[0][1][0]["Value"] == 12.5
    assert inserts[1][1][0]["IsMonotonic"] == 1
    assert inserts[1][1][0]["AggregationTemporality"] == 2
    assert "Value" not in inserts[2][1][0]
    assert inserts[2][1][0]["Count"] == 3
    assert inserts[2][1][0]["BucketCounts"] == [1, 1, 1]


def test_insert_typed_metric_events_returns_zero_for_empty_input() -> None:
    count = _insert_typed_metric_events(
        object(),
        [],
        stringify_attrs=lambda attrs: attrs,
        insert_rows_json_each_row=lambda _db, _table, _rows: 99,
    )

    assert count == 0
