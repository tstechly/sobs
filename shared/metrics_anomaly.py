from __future__ import annotations

from typing import Any

SIGNAL_LABELS: dict[tuple[str, str], dict[str, str]] = {
    ("logs", "log_volume"): {
        "label": "Log Volume",
        "description": "Log lines ingested per minute",
    },
    ("logs", "error_volume"): {
        "label": "Error Volume",
        "description": "Error-level log lines per minute",
    },
    ("logs", "error_ratio"): {
        "label": "Error Ratio",
        "description": "Fraction of log lines that are errors",
    },
    ("traces", "trace_volume"): {
        "label": "Trace Volume",
        "description": "Completed spans per minute",
    },
    ("traces", "trace_error_ratio"): {
        "label": "Trace Error Ratio",
        "description": "Fraction of spans with an error status",
    },
    ("traces", "latency_p95_ms"): {
        "label": "Latency p95",
        "description": "95th-percentile span duration (ms)",
    },
    ("errors", "exception_volume"): {
        "label": "Exception Volume",
        "description": "Exception events per minute",
    },
    ("rum_vitals", "LCP"): {
        "label": "Largest Contentful Paint",
        "description": "Core Web Vital: LCP (ms) – measures loading performance",
    },
    ("rum_vitals", "INP"): {
        "label": "Interaction to Next Paint",
        "description": "Core Web Vital: INP (ms) – measures interactivity",
    },
    ("rum_vitals", "CLS"): {
        "label": "Cumulative Layout Shift",
        "description": "Core Web Vital: CLS (unitless) – measures visual stability",
    },
    ("rum_vitals", "TTFB"): {
        "label": "Time to First Byte",
        "description": "Core Web Vital: TTFB (ms) – measures server response time",
    },
    ("rum_vitals", "FCP"): {
        "label": "First Contentful Paint",
        "description": "Core Web Vital: FCP (ms) – measures perceived load speed",
    },
    ("rum_vitals", "FID"): {
        "label": "First Input Delay",
        "description": "Core Web Vital: FID (ms) – measures input responsiveness",
    },
}

SOURCE_LABELS: dict[str, str] = {
    "logs": "Logs",
    "traces": "Traces",
    "errors": "Errors",
    "rum_vitals": "RUM Vitals",
    "metrics": "Metrics",
}

DERIVED_SIGNAL_NAMES = sorted(
    [
        "log_volume",
        "error_volume",
        "error_ratio",
        "trace_volume",
        "trace_error_ratio",
        "latency_p95_ms",
        "exception_volume",
        "LCP",
        "FID",
        "CLS",
        "INP",
        "TTFB",
        "FCP",
    ]
)
DERIVED_SIGNAL_SOURCES = ["errors", "logs", "rum_vitals", "traces"]
METRICS_ANOMALY_DEFAULT_COLUMNS = [
    "time",
    "value",
    "sample_count",
    "baseline_mean",
    "baseline_stddev",
    "baseline_lower",
    "baseline_upper",
    "anomaly_score",
    "anomaly_state",
    "metric_kind",
    "attr_fp",
]


def signal_label(source: str, signal: str) -> str:
    entry = SIGNAL_LABELS.get((source, signal))
    if entry:
        return entry["label"]
    return signal.replace("_", " ").title()


def signal_description(source: str, signal: str) -> str:
    entry = SIGNAL_LABELS.get((source, signal))
    return entry["description"] if entry else ""


def source_label(source: str) -> str:
    return SOURCE_LABELS.get(source, source.replace("_", " ").title())


def list_derived_signal_dimensions(db) -> tuple[list[str], list[str], list[str]]:
    services = [
        row[0]
        for row in db.execute(
            "SELECT DISTINCT ServiceName FROM otel_logs WHERE ServiceName != ''"
            " UNION DISTINCT SELECT DISTINCT ServiceName FROM otel_traces WHERE ServiceName != ''"
            " UNION DISTINCT SELECT DISTINCT ServiceName FROM hyperdx_sessions WHERE ServiceName != ''"
            " ORDER BY ServiceName"
        ).fetchall()
    ]
    return services, list(DERIVED_SIGNAL_NAMES), list(DERIVED_SIGNAL_SOURCES)


def parse_metrics_anomaly_hours(raw_hours: Any, *, default: int = 24) -> int:
    try:
        return max(1, min(168, int(raw_hours or default)))
    except (TypeError, ValueError):
        return default


def build_metrics_anomaly_api_query(
    service: str,
    metric: str,
    hours: int,
    attr_fp: str = "",
) -> tuple[str, list[object]]:
    fp_clause = " AND AttrFingerprint = ?" if attr_fp else ""
    params: list[object] = [service, metric, hours]
    if attr_fp:
        params.append(attr_fp)
    return (
        "SELECT"
        "  time,"
        "  value,"
        "  SampleCount AS sample_count,"
        "  baseline_mean,"
        "  baseline_stddev,"
        "  baseline_lower,"
        "  baseline_upper,"
        "  anomaly_score,"
        "  anomaly_state,"
        "  MetricKind AS metric_kind,"
        "  AttrFingerprint AS attr_fp"
        " FROM v_otel_metrics_anomaly"
        " WHERE ServiceName = ?"
        "   AND MetricName = ?"
        "   AND time >= now() - INTERVAL ? HOUR"
        f"{fp_clause}"
        " ORDER BY time"
        " LIMIT 1440",
        params,
    )


def serialize_metrics_anomaly_api_rows(rows: list[Any]) -> tuple[list[str], list[list[object | None]]]:
    columns = list(rows[0].keys()) if rows else list(METRICS_ANOMALY_DEFAULT_COLUMNS)
    data = [[_safe_metrics_anomaly_value(row[column]) for column in columns] for row in rows]
    return columns, data


def build_metrics_anomaly_detail_query(use_otel_metrics_view: bool, where_clause: str) -> str:
    return (
        (
            "SELECT"
            "  time,"
            "  ServiceName,"
            "  MetricName AS Name,"
            "  MetricKind AS Kind,"
            "  AttrFingerprint,"
            "  value,"
            "  SampleCount,"
            "  baseline_mean,"
            "  baseline_stddev,"
            "  baseline_lower,"
            "  baseline_upper,"
            "  anomaly_score,"
            "  anomaly_state"
            " FROM v_otel_metrics_anomaly"
        )
        if use_otel_metrics_view
        else (
            "SELECT"
            "  time,"
            "  ServiceName,"
            "  SignalName AS Name,"
            "  SignalSource AS Kind,"
            "  AttrFingerprint,"
            "  value,"
            "  SampleCount,"
            "  baseline_mean,"
            "  baseline_stddev,"
            "  baseline_lower,"
            "  baseline_upper,"
            "  anomaly_score,"
            "  anomaly_state"
            " FROM v_derived_signals_anomaly"
        )
        + f"{where_clause}"
        + " ORDER BY time DESC"
        + " LIMIT 500"
    )


def serialize_metrics_anomaly_detail_rows(
    fetched: list[Any],
    *,
    use_otel_metrics_view: bool,
) -> list[dict[str, object]]:
    rows: list[dict[str, object]] = []
    for row in fetched:
        rows.append(
            {
                "time": str(row["time"]),
                "service": str(row["ServiceName"]),
                "metric": str(row["Name"]),
                "metric_kind": str(row["Kind"]),
                "related_target": "" if use_otel_metrics_view else str(row["Kind"]),
                "attr_fp": str(row["AttrFingerprint"]),
                "value": row["value"],
                "sample_count": row["SampleCount"],
                "baseline_mean": row["baseline_mean"],
                "baseline_stddev": row["baseline_stddev"],
                "baseline_lower": row["baseline_lower"],
                "baseline_upper": row["baseline_upper"],
                "anomaly_score": row["anomaly_score"],
                "anomaly_state": str(row["anomaly_state"]),
            }
        )
    return rows


def _safe_metrics_anomaly_value(value: object) -> object | None:
    if isinstance(value, float) and value != value:
        return None
    return value
