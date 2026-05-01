from __future__ import annotations

from shared.metrics_anomaly import (
    METRICS_ANOMALY_DEFAULT_COLUMNS,
    build_metrics_anomaly_api_query,
    build_metrics_anomaly_detail_query,
    list_derived_signal_dimensions,
    parse_metrics_anomaly_hours,
    serialize_metrics_anomaly_api_rows,
    serialize_metrics_anomaly_detail_rows,
    signal_description,
    signal_label,
    source_label,
)


class _FakeResult:
    def __init__(self, rows):
        self._rows = rows

    def fetchall(self):
        return self._rows


class _FakeDb:
    def __init__(self, rows):
        self.rows = rows
        self.queries: list[str] = []

    def execute(self, query):
        self.queries.append(query)
        return _FakeResult(self.rows)


def test_signal_and_source_labels_use_registered_and_fallback_values():
    assert signal_label("logs", "log_volume") == "Log Volume"
    assert signal_description("logs", "log_volume") == "Log lines ingested per minute"
    assert signal_label("custom", "my_signal") == "My Signal"
    assert signal_description("custom", "my_signal") == ""
    assert source_label("rum_vitals") == "RUM Vitals"
    assert source_label("custom_source") == "Custom Source"


def test_list_derived_signal_dimensions_reads_services_and_returns_static_dimensions():
    db = _FakeDb([("svc-a",), ("svc-b",)])
    services, signals, sources = list_derived_signal_dimensions(db)
    assert services == ["svc-a", "svc-b"]
    assert "log_volume" in signals
    assert "latency_p95_ms" in signals
    assert sources == ["errors", "logs", "rum_vitals", "traces"]
    assert "FROM otel_logs" in db.queries[0]


def test_parse_metrics_anomaly_hours_clamps_and_defaults_invalid_values():
    assert parse_metrics_anomaly_hours(None) == 24
    assert parse_metrics_anomaly_hours("abc") == 24
    assert parse_metrics_anomaly_hours("0") == 1
    assert parse_metrics_anomaly_hours("999") == 168
    assert parse_metrics_anomaly_hours("6") == 6


def test_build_metrics_anomaly_api_query_adds_optional_attr_fingerprint_clause():
    query, params = build_metrics_anomaly_api_query("svc", "metric", 4)
    assert "AttrFingerprint = ?" not in query
    assert params == ["svc", "metric", 4]

    query, params = build_metrics_anomaly_api_query("svc", "metric", 4, "fingerprint")
    assert "AttrFingerprint = ?" in query
    assert params == ["svc", "metric", 4, "fingerprint"]


def test_serialize_metrics_anomaly_api_rows_uses_default_columns_and_masks_nan():
    columns, data = serialize_metrics_anomaly_api_rows([])
    assert columns == METRICS_ANOMALY_DEFAULT_COLUMNS
    assert data == []

    row = {
        "time": "2026-05-01 00:00:00",
        "value": 10.0,
        "sample_count": 2,
        "baseline_mean": float("nan"),
        "baseline_stddev": 1.0,
        "baseline_lower": 9.0,
        "baseline_upper": 11.0,
        "anomaly_score": 0.1,
        "anomaly_state": "normal",
        "metric_kind": "gauge",
        "attr_fp": "fp-1",
    }
    columns, data = serialize_metrics_anomaly_api_rows([row])
    assert columns == list(row.keys())
    assert data == [["2026-05-01 00:00:00", 10.0, 2, None, 1.0, 9.0, 11.0, 0.1, "normal", "gauge", "fp-1"]]


def test_detail_query_and_row_serialization_support_metric_and_signal_views():
    query = build_metrics_anomaly_detail_query(True, " WHERE ServiceName = ?")
    assert "FROM v_otel_metrics_anomaly" in query
    assert "MetricName AS Name" in query

    query = build_metrics_anomaly_detail_query(False, " WHERE SignalName = ?")
    assert "FROM v_derived_signals_anomaly" in query
    assert "SignalName AS Name" in query

    fetched = [
        {
            "time": "2026-05-01 00:00:00",
            "ServiceName": "svc-a",
            "Name": "latency_p95_ms",
            "Kind": "traces",
            "AttrFingerprint": "fp-2",
            "value": 123.0,
            "SampleCount": 3,
            "baseline_mean": 100.0,
            "baseline_stddev": 5.0,
            "baseline_lower": 90.0,
            "baseline_upper": 110.0,
            "anomaly_score": 2.6,
            "anomaly_state": "warning",
        }
    ]
    rows = serialize_metrics_anomaly_detail_rows(fetched, use_otel_metrics_view=False)
    assert rows == [
        {
            "time": "2026-05-01 00:00:00",
            "service": "svc-a",
            "metric": "latency_p95_ms",
            "metric_kind": "traces",
            "related_target": "traces",
            "attr_fp": "fp-2",
            "value": 123.0,
            "sample_count": 3,
            "baseline_mean": 100.0,
            "baseline_stddev": 5.0,
            "baseline_lower": 90.0,
            "baseline_upper": 110.0,
            "anomaly_score": 2.6,
            "anomaly_state": "warning",
        }
    ]

    rows = serialize_metrics_anomaly_detail_rows(fetched, use_otel_metrics_view=True)
    assert rows[0]["related_target"] == ""
