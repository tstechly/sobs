from __future__ import annotations

from collections.abc import Callable
from typing import Any

from shared.events import ErrorEvent, LogEvent, SpanEvent, TypedMetricEvent


def _insert_log_events(
    db: Any,
    events: list[LogEvent],
    *,
    stringify_attrs: Callable[[dict[str, Any]], dict[str, Any]],
    severity_number: Callable[[str], int],
    insert_rows_json_each_row: Callable[[Any, str, list[dict[str, Any]]], int],
    remember_log_attr_keys: Callable[[Any, list[dict[str, Any]], str], None],
    remember_attr_keys: Callable[[Any, list[dict[str, Any]], str], None],
    extract_log_attr_maps: Callable[[list[dict[str, Any]]], list[dict[str, Any]]],
    extract_attr_maps: Callable[[list[dict[str, Any]], str], list[dict[str, Any]]],
    load_tag_rules: Callable[[Any], Any],
    apply_tag_rules: Callable[[Any, str, list[dict[str, Any]], Any], None],
    log_exception: Callable[[str], None],
) -> int:
    rows = []
    for event in events:
        rows.append(
            {
                "Timestamp": event.ts,
                "TraceId": event.trace_id,
                "SpanId": event.span_id,
                "TraceFlags": 0,
                "SeverityText": event.level,
                "SeverityNumber": severity_number(event.level),
                "ServiceName": event.service,
                "Body": event.body,
                "ResourceSchemaUrl": "",
                "ResourceAttributes": stringify_attrs(event.resource_attrs),
                "ScopeSchemaUrl": "",
                "ScopeName": "",
                "ScopeVersion": "",
                "ScopeAttributes": stringify_attrs(event.scope_attrs),
                "LogAttributes": stringify_attrs(event.attrs),
                "EventName": str(event.attrs.get("event.name", "")),
            }
        )
    count = insert_rows_json_each_row(db, "otel_logs", rows)
    remember_log_attr_keys(db, extract_log_attr_maps(rows), "log")
    remember_attr_keys(db, extract_attr_maps(rows, "ResourceAttributes"), "resource")
    remember_attr_keys(db, extract_attr_maps(rows, "ScopeAttributes"), "scope")
    try:
        rules = load_tag_rules(db)
        if rules:
            apply_tag_rules(db, "log", rows, rules)
    except Exception:
        log_exception("auto-tag application failed for logs")
    return count


def _insert_span_events(
    db: Any,
    span_events: list[SpanEvent],
    *,
    stringify_attrs: Callable[[dict[str, Any]], dict[str, Any]],
    trace_status_code: Callable[[str], str],
    insert_rows_json_each_row: Callable[[Any, str, list[dict[str, Any]]], int],
    remember_attr_keys: Callable[[Any, list[dict[str, Any]], str], None],
    extract_attr_maps: Callable[[list[dict[str, Any]], str], list[dict[str, Any]]],
    load_tag_rules: Callable[[Any], Any],
    apply_tag_rules: Callable[[Any, str, list[dict[str, Any]], Any], None],
    log_exception: Callable[[str], None],
) -> int:
    rows = []
    for event in span_events:
        rows.append(
            {
                "Timestamp": event.ts,
                "TraceId": event.trace_id,
                "SpanId": event.span_id,
                "ParentSpanId": event.parent_span_id,
                "TraceState": "",
                "SpanName": event.name,
                "SpanKind": event.attrs.get("span.kind", "INTERNAL"),
                "ServiceName": event.service,
                "ResourceAttributes": stringify_attrs(event.resource_attrs),
                "ScopeName": "",
                "ScopeVersion": "",
                "SpanAttributes": stringify_attrs(event.attrs),
                "Duration": max(0, int(event.duration_ms * 1_000_000)),
                "StatusCode": trace_status_code(event.status),
                "StatusMessage": str(event.attrs.get("status.message", "")),
                "Events": {"Timestamp": [], "Name": [], "Attributes": []},
                "Links": {"TraceId": [], "SpanId": [], "TraceState": [], "Attributes": []},
            }
        )
    count = insert_rows_json_each_row(db, "otel_traces", rows)
    remember_attr_keys(db, extract_attr_maps(rows, "SpanAttributes"), "span")
    remember_attr_keys(db, extract_attr_maps(rows, "ResourceAttributes"), "resource")
    try:
        rules = load_tag_rules(db)
        if rules:
            apply_tag_rules(db, "trace", rows, rules)
    except Exception:
        log_exception("auto-tag application failed for traces")
    return count


def _insert_error_events(
    db: Any,
    error_events: list[ErrorEvent],
    *,
    stringify_attrs: Callable[[dict[str, Any]], dict[str, Any]],
    severity_number: Callable[[str], int],
    insert_rows_json_each_row: Callable[[Any, str, list[dict[str, Any]]], int],
    remember_log_attr_keys: Callable[[Any, list[dict[str, Any]], str], None],
    extract_log_attr_maps: Callable[[list[dict[str, Any]]], list[dict[str, Any]]],
    load_tag_rules: Callable[[Any], Any],
    apply_tag_rules: Callable[[Any, str, list[dict[str, Any]], Any], None],
    log_exception: Callable[[str], None],
) -> None:
    rows = []
    for event in error_events:
        attrs = stringify_attrs(event.attrs)
        attrs["exception.type"] = event.err_type
        attrs["exception.message"] = event.message
        if event.stack:
            attrs["exception.stacktrace"] = event.stack
        rows.append(
            {
                "Timestamp": event.ts,
                "TraceId": event.trace_id,
                "SpanId": event.span_id,
                "TraceFlags": 0,
                "SeverityText": "ERROR",
                "SeverityNumber": severity_number("ERROR"),
                "ServiceName": event.service,
                "Body": event.message,
                "ResourceSchemaUrl": "",
                "ResourceAttributes": {},
                "ScopeSchemaUrl": "",
                "ScopeName": "",
                "ScopeVersion": "",
                "ScopeAttributes": {},
                "LogAttributes": attrs,
                "EventName": "exception",
            }
        )
    insert_rows_json_each_row(db, "otel_logs", rows)
    remember_log_attr_keys(db, extract_log_attr_maps(rows), "log")
    try:
        rules = load_tag_rules(db)
        if rules:
            apply_tag_rules(db, "error", rows, rules)
    except Exception:
        log_exception("auto-tag application failed for errors")


def _insert_metric_events(
    db: Any,
    events: list[TypedMetricEvent],
    *,
    stringify_attrs: Callable[[dict[str, Any]], dict[str, Any]],
    insert_rows_json_each_row: Callable[[Any, str, list[dict[str, Any]]], int],
) -> int:
    return _insert_typed_metric_events(
        db,
        events,
        stringify_attrs=stringify_attrs,
        insert_rows_json_each_row=insert_rows_json_each_row,
    )


def _insert_typed_metric_events(
    db: Any,
    events: list[TypedMetricEvent],
    *,
    stringify_attrs: Callable[[dict[str, Any]], dict[str, Any]],
    insert_rows_json_each_row: Callable[[Any, str, list[dict[str, Any]]], int],
) -> int:
    gauge_rows: list[dict[str, Any]] = []
    sum_rows: list[dict[str, Any]] = []
    histogram_rows: list[dict[str, Any]] = []

    for event in events:
        base = {
            "TimeUnix": event.ts,
            "ServiceName": event.service,
            "MetricName": event.metric_name,
            "MetricDescription": event.metric_description,
            "MetricUnit": event.metric_unit,
            "Attributes": stringify_attrs(event.attrs),
            "Value": float(event.value),
            "Flags": 0,
            "AttrFingerprint": event.attr_fp,
        }
        if event.metric_kind == "gauge":
            gauge_rows.append(base)
        elif event.metric_kind == "sum":
            sum_rows.append(
                {
                    **base,
                    "IsMonotonic": event.is_monotonic,
                    "AggregationTemporality": event.aggregation_temporality,
                }
            )
        elif event.metric_kind == "histogram":
            histogram_rows.append(
                {
                    **{key: value for key, value in base.items() if key != "Value"},
                    "Count": event.histogram_count,
                    "Sum": float(event.histogram_sum),
                    "BucketCounts": event.histogram_buckets or [],
                    "ExplicitBounds": event.histogram_bounds or [],
                    "AggregationTemporality": event.aggregation_temporality,
                }
            )

    inserted = 0
    if gauge_rows:
        inserted += insert_rows_json_each_row(db, "otel_metrics_gauge", gauge_rows)
    if sum_rows:
        inserted += insert_rows_json_each_row(db, "otel_metrics_sum", sum_rows)
    if histogram_rows:
        inserted += insert_rows_json_each_row(db, "otel_metrics_histogram", histogram_rows)
    return inserted
