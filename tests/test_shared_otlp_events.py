from __future__ import annotations

from opentelemetry.proto.collector.logs.v1.logs_service_pb2 import ExportLogsServiceRequest
from opentelemetry.proto.collector.metrics.v1.metrics_service_pb2 import ExportMetricsServiceRequest
from opentelemetry.proto.collector.trace.v1.trace_service_pb2 import ExportTraceServiceRequest
from opentelemetry.proto.common.v1.common_pb2 import AnyValue, KeyValue, KeyValueList
from opentelemetry.proto.logs.v1.logs_pb2 import LogRecord, ResourceLogs, ScopeLogs
from opentelemetry.proto.metrics.v1.metrics_pb2 import (
    AggregationTemporality,
    Gauge,
    Histogram,
    HistogramDataPoint,
    Metric,
    NumberDataPoint,
    ResourceMetrics,
    ScopeMetrics,
    Sum,
)
from opentelemetry.proto.resource.v1.resource_pb2 import Resource
from opentelemetry.proto.trace.v1.trace_pb2 import ResourceSpans, ScopeSpans, Span, Status

from shared.otlp_events import _proto_logs_to_events, _proto_metrics_to_events, _proto_traces_to_events


def test_proto_logs_to_events_shapes_log_records() -> None:
    msg = ExportLogsServiceRequest(
        resource_logs=[
            ResourceLogs(
                resource=Resource(attributes=[KeyValue(key="service.name", value=AnyValue(string_value="checkout"))]),
                scope_logs=[
                    ScopeLogs(
                        log_records=[
                            LogRecord(
                                time_unix_nano=10,
                                severity_text="warn",
                                body=AnyValue(
                                    kvlist_value=KeyValueList(
                                        values=[
                                            KeyValue(
                                                key="ignored",
                                                value=AnyValue(string_value="by-proto"),
                                            )
                                        ]
                                    )
                                ),
                                attributes=[KeyValue(key="env", value=AnyValue(string_value="prod"))],
                                trace_id=bytes.fromhex("aabbccdd11223344aabbccdd11223344"),
                                span_id=bytes.fromhex("1122334455667788"),
                            )
                        ]
                    )
                ],
            )
        ]
    )

    events = _proto_logs_to_events(msg, ns_to_iso=lambda nanos: f"ts-{nanos}")

    assert len(events) == 1
    event = events[0]
    assert event.ts == "ts-10"
    assert event.level == "WARN"
    assert event.service == "checkout"
    assert event.body == '{"ignored": "by-proto"}'
    assert event.attrs["env"] == "prod"
    assert event.trace_id == "aabbccdd11223344aabbccdd11223344"
    assert event.span_id == "1122334455667788"


def test_proto_traces_to_events_shapes_spans_and_errors() -> None:
    msg = ExportTraceServiceRequest(
        resource_spans=[
            ResourceSpans(
                resource=Resource(attributes=[KeyValue(key="service.name", value=AnyValue(string_value="trace-svc"))]),
                scope_spans=[
                    ScopeSpans(
                        spans=[
                            Span(
                                trace_id=bytes.fromhex("deadbeefdeadbeefdeadbeefdeadbeef"),
                                span_id=bytes.fromhex("cafebabe12345678"),
                                name="checkout",
                                start_time_unix_nano=100,
                                end_time_unix_nano=250,
                                status=Status(code=Status.STATUS_CODE_ERROR),
                                attributes=[
                                    KeyValue(key="exception.type", value=AnyValue(string_value="ValueError")),
                                    KeyValue(key="exception.message", value=AnyValue(string_value="bad input")),
                                ],
                            )
                        ]
                    )
                ],
            )
        ]
    )

    span_events, error_events = _proto_traces_to_events(msg, ns_to_iso=lambda nanos: f"ts-{nanos}")

    assert len(span_events) == 1
    assert len(error_events) == 1
    assert span_events[0].duration_ms == 0.00015
    assert span_events[0].status == "ERROR"
    assert error_events[0].err_type == "ValueError"
    assert error_events[0].message == "bad input"


def test_proto_metrics_to_events_handles_gauge_sum_histogram_and_fallback() -> None:
    ts_ns = 123
    msg = ExportMetricsServiceRequest(
        resource_metrics=[
            ResourceMetrics(
                resource=Resource(
                    attributes=[KeyValue(key="service.name", value=AnyValue(string_value="metrics-svc"))]
                ),
                scope_metrics=[
                    ScopeMetrics(
                        metrics=[
                            Metric(
                                name="cpu.usage",
                                description="CPU",
                                unit="%",
                                gauge=Gauge(data_points=[NumberDataPoint(time_unix_nano=ts_ns, as_double=75.5)]),
                            ),
                            Metric(
                                name="http.requests",
                                description="Requests",
                                unit="1",
                                sum=Sum(
                                    data_points=[NumberDataPoint(time_unix_nano=ts_ns, as_int=15)],
                                    is_monotonic=True,
                                    aggregation_temporality=AggregationTemporality.AGGREGATION_TEMPORALITY_CUMULATIVE,
                                ),
                            ),
                            Metric(
                                name="request.duration",
                                description="Duration",
                                unit="ms",
                                histogram=Histogram(
                                    data_points=[
                                        HistogramDataPoint(
                                            time_unix_nano=ts_ns,
                                            count=4,
                                            sum=20.0,
                                            bucket_counts=[1, 2, 1],
                                            explicit_bounds=[5.0, 10.0],
                                        )
                                    ],
                                    aggregation_temporality=AggregationTemporality.AGGREGATION_TEMPORALITY_CUMULATIVE,
                                ),
                            ),
                            Metric(name="summary.metric", description="Summary", unit="1"),
                        ]
                    )
                ],
            )
        ]
    )

    events = _proto_metrics_to_events(
        msg,
        ns_to_iso=lambda nanos: f"ts-{nanos}",
        now_iso=lambda: "now-ts",
        attr_fingerprint=lambda attrs: f"fp-{len(attrs)}",
    )

    assert [event.metric_name for event in events] == [
        "cpu.usage",
        "http.requests",
        "request.duration",
        "summary.metric",
    ]
    assert events[0].metric_kind == "gauge"
    assert events[0].value == 75.5
    assert events[1].metric_kind == "sum"
    assert events[1].is_monotonic == 1
    assert events[2].metric_kind == "histogram"
    assert events[2].value == 5.0
    assert events[2].histogram_buckets == [1, 2, 1]
    assert events[3].ts == "now-ts"
    assert events[3].metric_kind == "gauge"
    assert events[3].value == 0.0
