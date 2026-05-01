from __future__ import annotations

import json
from collections.abc import Callable
from contextlib import nullcontext
from datetime import datetime, timezone
from typing import Any

_WRITABLE_TABLES: frozenset[str] = frozenset(
    [
        "otel_logs",
        "otel_traces",
        "otel_metrics_gauge",
        "otel_metrics_sum",
        "otel_metrics_histogram",
        "otel_metrics_gauge_pinned",
        "otel_metrics_sum_pinned",
        "otel_metrics_histogram_pinned",
        "hyperdx_sessions",
        "sobs_ai_memories",
        "sobs_ai_settings",
        "sobs_agent_rules",
        "sobs_agent_runs",
        "sobs_anomaly_rules",
        "sobs_app_releases",
        "sobs_app_settings",
        "sobs_apps",
        "sobs_chart_configs",
        "sobs_cve_dispositions",
        "sobs_cve_findings",
        "sobs_dashboards",
        "sobs_github_work_items",
        "sobs_log_attr_keys",
        "sobs_notification_channels",
        "sobs_notification_log",
        "sobs_notification_rules",
        "sobs_raw_window_copy_state",
        "sobs_raw_windows",
        "sobs_record_tags",
        "sobs_release_artifacts",
        "sobs_reports",
        "sobs_tag_rules",
    ]
)


def _normalize_ch_timestamp(value: Any, *, now_utc: Callable[[], datetime] | None = None) -> str:
    """Convert common timestamp forms to ClickHouse DateTime64-compatible strings."""
    if isinstance(value, datetime):
        dt = value.astimezone(timezone.utc) if value.tzinfo else value
    else:
        raw = str(value or "").strip()
        if not raw:
            now_factory = now_utc or (lambda: datetime.now(timezone.utc))
            dt = now_factory()
        else:
            try:
                dt = datetime.fromisoformat(raw.replace("Z", "+00:00"))
            except ValueError:
                return raw.replace("T", " ")
            if dt.tzinfo:
                dt = dt.astimezone(timezone.utc)
    return dt.strftime("%Y-%m-%d %H:%M:%S.%f")


def _insert_rows_json_each_row(
    db: Any,
    table_name: str,
    rows: list[dict[str, Any]],
    *,
    normalize_ch_timestamp: Callable[[Any], str] = _normalize_ch_timestamp,
    span_factory: Callable[[str, dict[str, Any]], Any] | None = None,
) -> int:
    if table_name not in _WRITABLE_TABLES:
        raise ValueError(
            f"Attempt to write to unregistered table '{table_name}'. "
            "Only tables in _WRITABLE_TABLES may be written via _insert_rows_json_each_row."
        )
    if not rows:
        return 0

    dt_keys = {
        "Timestamp",
        "TimeUnix",
        "UpdatedAt",
        "CreatedAt",
        "CompletedAt",
        "ReleasedAt",
        "UploadedAt",
        "ScannedAt",
    }
    normalized_rows: list[dict[str, Any]] = []
    for row in rows:
        item = dict(row)
        for key in dt_keys:
            if key in item:
                item[key] = normalize_ch_timestamp(item[key])
        if "Events" in item and isinstance(item["Events"], dict) and "Timestamp" in item["Events"]:
            item["Events"]["Timestamp"] = [normalize_ch_timestamp(v) for v in item["Events"]["Timestamp"]]
        normalized_rows.append(item)

    payload = "\n".join(json.dumps(row, ensure_ascii=False) for row in normalized_rows)
    span_context = (
        span_factory(
            "sobs.storage.write",
            {"storage.engine": "chdb", "table": table_name, "row.count": len(normalized_rows)},
        )
        if span_factory
        else nullcontext()
    )
    with span_context:
        db.execute(f"INSERT INTO {table_name} FORMAT JSONEachRow\n" + payload)
    return len(normalized_rows)
