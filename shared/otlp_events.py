from __future__ import annotations

import json
from collections.abc import Callable
from typing import Any

from opentelemetry.proto.collector.logs.v1.logs_service_pb2 import ExportLogsServiceRequest
from opentelemetry.proto.collector.metrics.v1.metrics_service_pb2 import ExportMetricsServiceRequest
from opentelemetry.proto.collector.trace.v1.trace_service_pb2 import ExportTraceServiceRequest

from shared.events import ErrorEvent, LogEvent, SpanEvent, TypedMetricEvent, _attr_fingerprint
from shared.otlp_attrs import _proto_any_value_to_python, _proto_kvlist_to_dict


def _proto_logs_to_events(
    msg: ExportLogsServiceRequest,
    *,
    ns_to_iso: Callable[[int], str],
) -> list[LogEvent]:
    events: list[LogEvent] = []
    for resource_log in msg.resource_logs:
        resource_attrs = _proto_kvlist_to_dict(resource_log.resource.attributes)
        service = str(resource_attrs.get("service.name", ""))
        for scope_log in resource_log.scope_logs:
            scope_attrs = _proto_kvlist_to_dict(scope_log.scope.attributes)
            for record in scope_log.log_records:
                record_attrs = _proto_kvlist_to_dict(record.attributes)
                merged_attrs = {**resource_attrs, **scope_attrs, **record_attrs}
                body_val = _proto_any_value_to_python(record.body)
                body_str = body_val if isinstance(body_val, str) else json.dumps(body_val, ensure_ascii=False)
                events.append(
                    LogEvent(
                        ts=ns_to_iso(int(record.time_unix_nano or 0)),
                        level=(record.severity_text or "INFO").upper(),
                        service=service,
                        body=body_str,
                        attrs=merged_attrs,
                        resource_attrs=resource_attrs,
                        scope_attrs=scope_attrs,
                        trace_id=record.trace_id.hex() if record.trace_id else "",
                        span_id=record.span_id.hex() if record.span_id else "",
                    )
                )
    return events


def _proto_traces_to_events(
    msg: ExportTraceServiceRequest,
    *,
    ns_to_iso: Callable[[int], str],
) -> tuple[list[SpanEvent], list[ErrorEvent]]:
    span_events: list[SpanEvent] = []
    error_events: list[ErrorEvent] = []
    for resource_span in msg.resource_spans:
        resource_attrs = _proto_kvlist_to_dict(resource_span.resource.attributes)
        service = str(resource_attrs.get("service.name", ""))
        for scope_span in resource_span.scope_spans:
            scope_attrs = _proto_kvlist_to_dict(scope_span.scope.attributes)
            for span in scope_span.spans:
                start_ns = int(span.start_time_unix_nano or 0)
                end_ns = int(span.end_time_unix_nano or 0)
                duration_ms = (end_ns - start_ns) / 1_000_000 if end_ns > start_ns else 0
                status = "OK" if span.status.code == 1 else ("ERROR" if span.status.code == 2 else "UNSET")
                span_attrs = _proto_kvlist_to_dict(span.attributes)
                merged_attrs = {**resource_attrs, **scope_attrs, **span_attrs}
                span_event = SpanEvent(
                    ts=ns_to_iso(start_ns),
                    trace_id=span.trace_id.hex() if span.trace_id else "",
                    span_id=span.span_id.hex() if span.span_id else "",
                    parent_span_id=span.parent_span_id.hex() if span.parent_span_id else "",
                    name=span.name,
                    service=service,
                    duration_ms=duration_ms,
                    status=status,
                    attrs=merged_attrs,
                    resource_attrs=resource_attrs,
                    scope_attrs=scope_attrs,
                )
                span_events.append(span_event)
                if "ERROR" in status.upper():
                    error_events.append(
                        ErrorEvent(
                            ts=span_event.ts,
                            service=service,
                            err_type=str(span_attrs.get("exception.type", "SpanError")),
                            message=str(
                                span_attrs.get(
                                    "exception.message",
                                    span_attrs.get("error.message", span.name),
                                )
                            ),
                            stack=str(span_attrs.get("exception.stacktrace", "")),
                            attrs=merged_attrs,
                            trace_id=span_event.trace_id,
                            span_id=span_event.span_id,
                        )
                    )
    return span_events, error_events


def _proto_metrics_to_events(
    msg: ExportMetricsServiceRequest,
    *,
    ns_to_iso: Callable[[int], str],
    now_iso: Callable[[], str],
    attr_fingerprint: Callable[[dict[str, Any]], str] = _attr_fingerprint,
) -> list[TypedMetricEvent]:
    events: list[TypedMetricEvent] = []
    for resource_metric in msg.resource_metrics:
        resource_attrs = _proto_kvlist_to_dict(resource_metric.resource.attributes)
        service = str(resource_attrs.get("service.name", "metrics"))
        for scope_metric in resource_metric.scope_metrics:
            for metric in scope_metric.metrics:
                name = metric.name
                desc = metric.description
                unit = metric.unit
                which = metric.WhichOneof("data")

                if which == "gauge":
                    for dp in metric.gauge.data_points:
                        dp_attrs = _proto_kvlist_to_dict(dp.attributes)
                        vfield = dp.WhichOneof("value")
                        value = float(dp.as_int) if vfield == "as_int" else dp.as_double
                        ts = ns_to_iso(int(dp.time_unix_nano)) if dp.time_unix_nano else now_iso()
                        events.append(
                            TypedMetricEvent(
                                ts=ts,
                                service=service,
                                metric_name=name,
                                metric_description=desc,
                                metric_unit=unit,
                                metric_kind="gauge",
                                value=value,
                                attrs=dp_attrs,
                                attr_fp=attr_fingerprint(dp_attrs),
                            )
                        )
                elif which == "sum":
                    for dp in metric.sum.data_points:
                        dp_attrs = _proto_kvlist_to_dict(dp.attributes)
                        vfield = dp.WhichOneof("value")
                        value = float(dp.as_int) if vfield == "as_int" else dp.as_double
                        ts = ns_to_iso(int(dp.time_unix_nano)) if dp.time_unix_nano else now_iso()
                        events.append(
                            TypedMetricEvent(
                                ts=ts,
                                service=service,
                                metric_name=name,
                                metric_description=desc,
                                metric_unit=unit,
                                metric_kind="sum",
                                value=value,
                                attrs=dp_attrs,
                                attr_fp=attr_fingerprint(dp_attrs),
                                is_monotonic=1 if metric.sum.is_monotonic else 0,
                                aggregation_temporality=int(metric.sum.aggregation_temporality),
                            )
                        )
                elif which == "histogram":
                    for dp in metric.histogram.data_points:
                        dp_attrs = _proto_kvlist_to_dict(dp.attributes)
                        count = int(dp.count)
                        hist_sum = float(dp.sum)
                        mean_val = hist_sum / count if count > 0 else 0.0
                        ts = ns_to_iso(int(dp.time_unix_nano)) if dp.time_unix_nano else now_iso()
                        events.append(
                            TypedMetricEvent(
                                ts=ts,
                                service=service,
                                metric_name=name,
                                metric_description=desc,
                                metric_unit=unit,
                                metric_kind="histogram",
                                value=mean_val,
                                attrs=dp_attrs,
                                attr_fp=attr_fingerprint(dp_attrs),
                                aggregation_temporality=int(metric.histogram.aggregation_temporality),
                                histogram_count=count,
                                histogram_sum=hist_sum,
                                histogram_buckets=list(dp.bucket_counts),
                                histogram_bounds=list(dp.explicit_bounds),
                            )
                        )
                else:
                    events.append(
                        TypedMetricEvent(
                            ts=now_iso(),
                            service=service,
                            metric_name=name,
                            metric_description=desc,
                            metric_unit=unit,
                            metric_kind="gauge",
                            value=0.0,
                            attrs={},
                            attr_fp=attr_fingerprint({}),
                        )
                    )
    return events
