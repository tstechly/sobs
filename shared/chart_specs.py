from __future__ import annotations

import json
import re
from typing import Mapping


def _validate_chart_query(query: str, *, query_deny_pattern: re.Pattern[str]) -> str | None:
    """Return an error message if the query is not a safe SELECT, otherwise None."""
    stripped = query.strip()
    if not stripped:
        return "Query cannot be empty"
    upper = stripped.upper()
    if not (upper.startswith("SELECT") or upper.startswith("WITH")):
        return "Only SELECT queries are allowed"
    if query_deny_pattern.search(stripped):
        return "Query contains a disallowed keyword"
    return None


def _sql_literal(value: object) -> str:
    return "'" + str(value).replace("'", "''") + "'"


def _coerce_positive_int(raw: object, default_value: int, min_value: int, max_value: int) -> int:
    try:
        parsed = int(str(raw))
    except (TypeError, ValueError):
        return default_value
    return max(min_value, min(max_value, parsed))


def _default_chart_spec(template_id: str = "derived_signal_overlay") -> dict[str, object]:
    if template_id == "custom_echarts":
        return {
            "template_id": template_id,
            "sql": {
                "mode": "raw",
                "override_sql": "SELECT toDateTime('2024-01-01 00:00:00') AS time, 1 AS value",
            },
            "data": {
                "source_view": "v_derived_signals_anomaly",
                "service": "",
                "signal_source": "traces",
                "signal_name": "trace_volume",
                "metric_name": "",
                "attr_fp": "",
                "window_hours": 6,
                "limit": 1000,
            },
            "visual": {
                "zoom_inside": True,
                "zoom_slider": False,
                "zoom_start_pct": 0,
                "zoom_end_pct": 100,
                "legend_show": True,
                "smooth_line": True,
                "value_color": "",
                "role_map": {},
                "custom_mapping_json": json.dumps({"points": {"from": "rows"}}, ensure_ascii=False),
                "custom_option_json": json.dumps(
                    {
                        "tooltip": {"trigger": "axis"},
                        "xAxis": {"type": "time"},
                        "yAxis": {"type": "value"},
                        "series": [
                            {
                                "name": "Value",
                                "type": "line",
                                "data": "{{points}}",
                                "showSymbol": False,
                                "smooth": True,
                            }
                        ],
                    },
                    ensure_ascii=False,
                ),
            },
        }

    return {
        "template_id": template_id,
        "sql": {"mode": "builder", "override_sql": ""},
        "data": {
            "source_view": "v_derived_signals_anomaly",
            "service": "",
            "signal_source": "traces",
            "signal_name": "trace_volume",
            "metric_name": "",
            "attr_fp": "",
            "window_hours": 6,
            "limit": 1000,
        },
        "visual": {
            "zoom_inside": True,
            "zoom_slider": False,
            "zoom_start_pct": 0,
            "zoom_end_pct": 100,
            "legend_show": True,
            "smooth_line": True,
            "value_color": "",
            "role_map": {},
        },
    }


def _build_raw_chart_spec(
    template_id: str,
    query: str,
    options_json: str = "",
    *,
    chart_templates: Mapping[str, object],
) -> dict[str, object]:
    try:
        parsed = json.loads(options_json) if options_json else {}
        if isinstance(parsed, dict):
            spec_candidate = parsed.get("chart_spec")
            if isinstance(spec_candidate, dict):
                return _normalize_chart_spec(spec_candidate, chart_templates=chart_templates)
    except Exception:
        pass

    spec = _default_chart_spec(template_id)
    spec["template_id"] = template_id
    spec["sql"] = {"mode": "raw", "override_sql": query}
    return spec


def _normalize_chart_spec(spec_raw: object, *, chart_templates: Mapping[str, object]) -> dict[str, object]:
    base = _default_chart_spec()
    raw = spec_raw if isinstance(spec_raw, dict) else {}

    template_id = str(raw.get("template_id") or base.get("template_id") or "time_series_percentiles").strip()
    if template_id not in chart_templates:
        raise ValueError(f"Unknown template: {template_id}")

    normalized = _default_chart_spec(template_id)
    normalized["template_id"] = template_id

    sql_raw = raw.get("sql") if isinstance(raw.get("sql"), dict) else {}
    sql_mode = str((sql_raw.get("mode") if isinstance(sql_raw, dict) else "builder") or "builder").strip().lower()
    if sql_mode not in {"builder", "raw"}:
        raise ValueError("sql.mode must be 'builder' or 'raw'")
    normalized["sql"] = {
        "mode": sql_mode,
        "override_sql": str((sql_raw.get("override_sql") if isinstance(sql_raw, dict) else "") or ""),
    }

    data_raw = raw.get("data") if isinstance(raw.get("data"), dict) else {}
    normalized_data = normalized.get("data")
    if isinstance(normalized_data, dict) and isinstance(data_raw, dict):
        merged_data = dict(normalized_data)
        merged_data.update(data_raw)
        normalized["data"] = merged_data

    visual_raw = raw.get("visual") if isinstance(raw.get("visual"), dict) else {}
    normalized_visual = normalized.get("visual")
    merged_visual = dict(normalized_visual) if isinstance(normalized_visual, dict) else {}
    if isinstance(visual_raw, dict):
        merged_visual.update(visual_raw)

    role_map_raw = merged_visual.get("role_map")
    role_map: dict[str, str] = {}
    if isinstance(role_map_raw, dict):
        for role, col_name in role_map_raw.items():
            role_name = str(role).strip()
            mapped = str(col_name).strip()
            if role_name and mapped:
                role_map[role_name] = mapped
    merged_visual["role_map"] = role_map
    normalized["visual"] = merged_visual

    named_queries_raw = raw.get("named_queries")
    named_queries: list[dict[str, str]] = []
    if isinstance(named_queries_raw, list):
        for item in named_queries_raw:
            if not isinstance(item, dict):
                continue
            name = str(item.get("name") or "").strip().lower()
            sql_text = str(item.get("sql") or "").strip().rstrip(";")
            purpose = str(item.get("purpose") or "").strip()
            if not name or not re.match(r"^[a-z][a-z0-9_]{0,31}$", name):
                continue
            if not sql_text:
                continue
            named_queries.append({"name": name, "sql": sql_text, "purpose": purpose})
    normalized["named_queries"] = named_queries

    return normalized


def _compile_builder_sql(template_id: str, data: dict[str, object]) -> str:
    if template_id == "custom_echarts":
        raise ValueError("custom_echarts requires sql.mode='raw'")

    source_view = str(data.get("source_view") or "v_derived_signals_anomaly").strip()
    supported_sources = {
        "v_derived_signals_anomaly",
        "v_otel_metrics_anomaly",
        "otel_metrics_gauge",
        "otel_metrics_sum",
        "otel_metrics_histogram",
        "otel_logs",
        "otel_traces",
        "sobs_error_resolutions",
    }
    if source_view not in supported_sources:
        raise ValueError("Unsupported source for builder mode")

    service = str(data.get("service") or "").strip()
    signal_source = str(data.get("signal_source") or "").strip()
    signal_name = str(data.get("signal_name") or "").strip()
    metric_name = str(data.get("metric_name") or "").strip()
    attr_fp = str(data.get("attr_fp") or "").strip()
    window_hours = _coerce_positive_int(data.get("window_hours"), 6, 1, 168)
    limit = _coerce_positive_int(data.get("limit"), 1000, 1, 2000)

    def _default_source_label() -> str:
        if source_view in {"otel_logs"}:
            return "logs"
        if source_view in {"otel_traces"}:
            return "traces"
        if source_view in {"sobs_error_resolutions"}:
            return "errors"
        if source_view == "v_derived_signals_anomaly":
            return signal_source or "derived"
        return "metrics"

    def _default_signal_label() -> str:
        if signal_name:
            return signal_name
        if metric_name:
            return metric_name
        if source_view == "otel_logs":
            return "log_volume"
        if source_view == "otel_traces":
            return "trace_volume"
        if source_view == "sobs_error_resolutions":
            return "resolved_error_volume"
        return "value"

    def _build_series_sql() -> str:
        if source_view == "v_derived_signals_anomaly":
            where_parts: list[str] = [f"time >= now() - INTERVAL {window_hours} HOUR"]
            if service:
                where_parts.append(f"ServiceName = {_sql_literal(service)}")
            if attr_fp:
                where_parts.append(f"AttrFingerprint = {_sql_literal(attr_fp)}")
            if signal_source:
                where_parts.append(f"SignalSource = {_sql_literal(signal_source)}")
            if signal_name:
                where_parts.append(f"SignalName = {_sql_literal(signal_name)}")
            where_clause = " AND\n    ".join(where_parts)
            return (
                "SELECT\n"
                "  time,\n"
                "  value,\n"
                "  baseline_mean,\n"
                "  baseline_lower,\n"
                "  baseline_upper,\n"
                "  anomaly_state,\n"
                "  anomaly_score\n"
                "FROM v_derived_signals_anomaly\n"
                f"WHERE {where_clause}"
            )

        if source_view == "v_otel_metrics_anomaly":
            where_parts = [f"time >= now() - INTERVAL {window_hours} HOUR"]
            if service:
                where_parts.append(f"ServiceName = {_sql_literal(service)}")
            if metric_name:
                where_parts.append(f"MetricName = {_sql_literal(metric_name)}")
            if attr_fp:
                where_parts.append(f"AttrFingerprint = {_sql_literal(attr_fp)}")
            where_clause = " AND\n    ".join(where_parts)
            return (
                "SELECT\n"
                "  time,\n"
                "  value,\n"
                "  baseline_mean,\n"
                "  baseline_lower,\n"
                "  baseline_upper,\n"
                "  anomaly_state,\n"
                "  anomaly_score\n"
                "FROM v_otel_metrics_anomaly\n"
                f"WHERE {where_clause}"
            )

        if source_view in {"otel_metrics_gauge", "otel_metrics_sum", "otel_metrics_histogram"}:
            if source_view == "otel_metrics_histogram":
                value_expr = "if(Count = 0, 0.0, Sum / toFloat64(Count))"
            else:
                value_expr = "Value"
            where_parts = [f"TimeUnixMs >= now() - INTERVAL {window_hours} HOUR"]
            if service:
                where_parts.append(f"ServiceName = {_sql_literal(service)}")
            if metric_name:
                where_parts.append(f"MetricName = {_sql_literal(metric_name)}")
            if attr_fp:
                where_parts.append(f"AttrFingerprint = {_sql_literal(attr_fp)}")
            where_clause = " AND\n    ".join(where_parts)
            return (
                "WITH per_minute AS (\n"
                "  SELECT\n"
                "    toStartOfMinute(TimeUnixMs) AS time,\n"
                "    avg(toFloat64(" + value_expr + ")) AS value\n"
                f"  FROM {source_view}\n"
                f"  WHERE {where_clause}\n"
                "  GROUP BY time\n"
                "), scored AS (\n"
                "  SELECT\n"
                "    time,\n"
                "    value,\n"
                "    avg(value) OVER (\n"
                "      ORDER BY time\n"
                "      ROWS BETWEEN 59 PRECEDING AND CURRENT ROW\n"
                "    ) AS baseline_mean,\n"
                "    stddevPop(value) OVER (\n"
                "      ORDER BY time\n"
                "      ROWS BETWEEN 59 PRECEDING AND CURRENT ROW\n"
                "    ) AS baseline_stddev\n"
                "  FROM per_minute\n"
                ")\n"
                "SELECT\n"
                "  time,\n"
                "  value,\n"
                "  baseline_mean,\n"
                "  greatest(0.0, baseline_mean - (3.0 * ifNull(baseline_stddev, 0.0))) AS baseline_lower,\n"
                "  baseline_mean + (3.0 * ifNull(baseline_stddev, 0.0)) AS baseline_upper,\n"
                "  if(\n"
                "    abs(value - baseline_mean) / greatest(ifNull(baseline_stddev, 0.0), 1.0) >= 3.0,\n"
                "    'outlier',\n"
                "    'normal'\n"
                "  ) AS anomaly_state,\n"
                "  abs(value - baseline_mean) / greatest(ifNull(baseline_stddev, 0.0), 1.0) AS anomaly_score\n"
                "FROM scored"
            )

        if source_view == "otel_logs":
            where_parts = [f"TimestampTime >= now() - INTERVAL {window_hours} HOUR"]
            if service:
                where_parts.append(f"ServiceName = {_sql_literal(service)}")
            where_clause = " AND\n    ".join(where_parts)
            return (
                "WITH per_minute AS (\n"
                "  SELECT\n"
                "    toStartOfMinute(TimestampTime) AS time,\n"
                "    count() AS value\n"
                "  FROM otel_logs\n"
                f"  WHERE {where_clause}\n"
                "  GROUP BY time\n"
                "), scored AS (\n"
                "  SELECT\n"
                "    time,\n"
                "    toFloat64(value) AS value,\n"
                "    avg(toFloat64(value)) OVER (\n"
                "      ORDER BY time\n"
                "      ROWS BETWEEN 59 PRECEDING AND CURRENT ROW\n"
                "    ) AS baseline_mean,\n"
                "    stddevPop(toFloat64(value)) OVER (\n"
                "      ORDER BY time\n"
                "      ROWS BETWEEN 59 PRECEDING AND CURRENT ROW\n"
                "    ) AS baseline_stddev\n"
                "  FROM per_minute\n"
                ")\n"
                "SELECT\n"
                "  time,\n"
                "  value,\n"
                "  baseline_mean,\n"
                "  greatest(0.0, baseline_mean - (3.0 * ifNull(baseline_stddev, 0.0))) AS baseline_lower,\n"
                "  baseline_mean + (3.0 * ifNull(baseline_stddev, 0.0)) AS baseline_upper,\n"
                "  if(\n"
                "    abs(value - baseline_mean) / greatest(ifNull(baseline_stddev, 0.0), 1.0) >= 3.0,\n"
                "    'outlier',\n"
                "    'normal'\n"
                "  ) AS anomaly_state,\n"
                "  abs(value - baseline_mean) / greatest(ifNull(baseline_stddev, 0.0), 1.0) AS anomaly_score\n"
                "FROM scored"
            )

        if source_view == "otel_traces":
            where_parts = [f"TimestampTime >= now() - INTERVAL {window_hours} HOUR"]
            if service:
                where_parts.append(f"ServiceName = {_sql_literal(service)}")
            where_clause = " AND\n    ".join(where_parts)
            return (
                "WITH per_minute AS (\n"
                "  SELECT\n"
                "    toStartOfMinute(TimestampTime) AS time,\n"
                "    count() AS value\n"
                "  FROM otel_traces\n"
                f"  WHERE {where_clause}\n"
                "  GROUP BY time\n"
                "), scored AS (\n"
                "  SELECT\n"
                "    time,\n"
                "    toFloat64(value) AS value,\n"
                "    avg(toFloat64(value)) OVER (\n"
                "      ORDER BY time\n"
                "      ROWS BETWEEN 59 PRECEDING AND CURRENT ROW\n"
                "    ) AS baseline_mean,\n"
                "    stddevPop(toFloat64(value)) OVER (\n"
                "      ORDER BY time\n"
                "      ROWS BETWEEN 59 PRECEDING AND CURRENT ROW\n"
                "    ) AS baseline_stddev\n"
                "  FROM per_minute\n"
                ")\n"
                "SELECT\n"
                "  time,\n"
                "  value,\n"
                "  baseline_mean,\n"
                "  greatest(0.0, baseline_mean - (3.0 * ifNull(baseline_stddev, 0.0))) AS baseline_lower,\n"
                "  baseline_mean + (3.0 * ifNull(baseline_stddev, 0.0)) AS baseline_upper,\n"
                "  if(\n"
                "    abs(value - baseline_mean) / greatest(ifNull(baseline_stddev, 0.0), 1.0) >= 3.0,\n"
                "    'outlier',\n"
                "    'normal'\n"
                "  ) AS anomaly_state,\n"
                "  abs(value - baseline_mean) / greatest(ifNull(baseline_stddev, 0.0), 1.0) AS anomaly_score\n"
                "FROM scored"
            )

        where_clause = f"ResolvedAt >= now() - INTERVAL {window_hours} HOUR"
        return (
            "WITH per_minute AS (\n"
            "  SELECT\n"
            "    toStartOfMinute(ResolvedAt) AS time,\n"
            "    count() AS value\n"
            "  FROM sobs_error_resolutions\n"
            f"  WHERE {where_clause}\n"
            "  GROUP BY time\n"
            "), scored AS (\n"
            "  SELECT\n"
            "    time,\n"
            "    toFloat64(value) AS value,\n"
            "    avg(toFloat64(value)) OVER (\n"
            "      ORDER BY time\n"
            "      ROWS BETWEEN 59 PRECEDING AND CURRENT ROW\n"
            "    ) AS baseline_mean,\n"
            "    stddevPop(toFloat64(value)) OVER (\n"
            "      ORDER BY time\n"
            "      ROWS BETWEEN 59 PRECEDING AND CURRENT ROW\n"
            "    ) AS baseline_stddev\n"
            "  FROM per_minute\n"
            ")\n"
            "SELECT\n"
            "  time,\n"
            "  value,\n"
            "  baseline_mean,\n"
            "  greatest(0.0, baseline_mean - (3.0 * ifNull(baseline_stddev, 0.0))) AS baseline_lower,\n"
            "  baseline_mean + (3.0 * ifNull(baseline_stddev, 0.0)) AS baseline_upper,\n"
            "  if(\n"
            "    abs(value - baseline_mean) / greatest(ifNull(baseline_stddev, 0.0), 1.0) >= 3.0,\n"
            "    'outlier',\n"
            "    'normal'\n"
            "  ) AS anomaly_state,\n"
            "  abs(value - baseline_mean) / greatest(ifNull(baseline_stddev, 0.0), 1.0) AS anomaly_score\n"
            "FROM scored"
        )

    series_sql = _build_series_sql()

    if template_id == "derived_signal_overlay":
        return (
            "WITH series AS (\n"
            f"{series_sql}\n"
            ")\n"
            "SELECT\n"
            "  time,\n"
            f"  {_sql_literal(service or 'all')} AS service,\n"
            f"  {_sql_literal(_default_source_label())} AS source,\n"
            f"  {_sql_literal(_default_signal_label())} AS signal,\n"
            f"  {_sql_literal(attr_fp)} AS attr_fp,\n"
            "  value,\n"
            "  toUInt32(1) AS sample_count,\n"
            "  baseline_mean,\n"
            "  baseline_lower,\n"
            "  baseline_upper,\n"
            "  anomaly_state,\n"
            "  anomaly_score\n"
            "FROM series\n"
            "ORDER BY time\n"
            f"LIMIT {limit}"
        )

    if template_id == "anomaly_overlay":
        return (
            "WITH series AS (\n"
            f"{series_sql}\n"
            ")\n"
            "SELECT\n"
            "  time,\n"
            "  value,\n"
            "  baseline_mean,\n"
            "  baseline_lower,\n"
            "  baseline_upper,\n"
            "  anomaly_state\n"
            "FROM series\n"
            "ORDER BY time\n"
            f"LIMIT {limit}"
        )

    if template_id == "dual_axis_anomaly":
        return (
            "WITH series AS (\n"
            f"{series_sql}\n"
            ")\n"
            "SELECT\n"
            "  time,\n"
            "  value AS metric,\n"
            "  anomaly_score\n"
            "FROM series\n"
            "ORDER BY time\n"
            f"LIMIT {limit}"
        )

    if template_id == "time_series_percentiles":
        return (
            "WITH series AS (\n"
            f"{series_sql}\n"
            ")\n"
            "SELECT\n"
            "  time,\n"
            "  value,\n"
            "  baseline_upper AS p95,\n"
            "  greatest(baseline_upper, value) AS p99\n"
            "FROM series\n"
            "ORDER BY time\n"
            f"LIMIT {limit}"
        )

    if template_id == "heatmap":
        return (
            "WITH series AS (\n"
            f"{series_sql}\n"
            ")\n"
            "SELECT\n"
            f"  {_sql_literal(service or 'all')} AS x_category,\n"
            "  toStartOfFiveMinutes(time) AS y_category,\n"
            "  avg(value) AS value\n"
            "FROM series\n"
            "GROUP BY y_category\n"
            "ORDER BY y_category\n"
            f"LIMIT {limit}"
        )

    if template_id == "box_plot":
        return (
            "WITH series AS (\n"
            f"{series_sql}\n"
            ")\n"
            "SELECT\n"
            f"  {_sql_literal(_default_signal_label())} AS dimension,\n"
            "  min(value) AS min,\n"
            "  quantile(0.25)(value) AS q1,\n"
            "  quantile(0.5)(value) AS median,\n"
            "  quantile(0.75)(value) AS q3,\n"
            "  max(value) AS max\n"
            "FROM series"
        )

    if template_id == "gauge_kpi":
        return (
            "WITH series AS (\n"
            f"{series_sql}\n"
            ")\n"
            "SELECT round(100.0 * avg(if(anomaly_state = 'normal', 1.0, 0.0)), 2) AS value\n"
            "FROM series"
        )

    raise ValueError(f"Builder mode does not support template: {template_id}")


def _compile_chart_spec(
    spec_raw: object,
    *,
    chart_templates: Mapping[str, object],
    query_deny_pattern: re.Pattern[str],
) -> tuple[str, str, dict[str, object]]:
    spec = _normalize_chart_spec(spec_raw, chart_templates=chart_templates)

    template_id = str(spec.get("template_id") or "time_series_percentiles").strip()

    sql_block = spec.get("sql") if isinstance(spec.get("sql"), dict) else {}
    sql_mode = str((sql_block.get("mode") if isinstance(sql_block, dict) else "builder") or "builder").strip().lower()

    if sql_mode == "raw":
        query = str((sql_block.get("override_sql") if isinstance(sql_block, dict) else "") or "").strip()
    else:
        if template_id == "custom_echarts":
            raise ValueError("custom_echarts requires sql.mode='raw'")
        data = spec.get("data") if isinstance(spec.get("data"), dict) else {}
        query = _compile_builder_sql(template_id, data if isinstance(data, dict) else {})

    err = _validate_chart_query(query, query_deny_pattern=query_deny_pattern)
    if err:
        raise ValueError(err)

    named_queries = spec.get("named_queries")
    if isinstance(named_queries, list):
        for nq in named_queries:
            if not isinstance(nq, dict):
                continue
            nq_sql = str(nq.get("sql") or "").strip()
            nq_name = str(nq.get("name") or "").strip()
            if nq_sql:
                nq_err = _validate_chart_query(nq_sql, query_deny_pattern=query_deny_pattern)
                if nq_err:
                    raise ValueError(f"Named query '{nq_name}': {nq_err}")

    return template_id, query, spec


def _resolve_template_role_indices(
    template_id: str,
    template: dict[str, object],
    columns: list[str],
    spec: dict[str, object] | None,
) -> dict[str, int]:
    raw_roles_raw = template.get("column_roles") if isinstance(template.get("column_roles"), dict) else {}
    role_indices: dict[str, int] = {}
    if isinstance(raw_roles_raw, dict):
        for role, idx_raw in raw_roles_raw.items():
            role_name = str(role)
            if isinstance(idx_raw, (int, float)):
                role_indices[role_name] = int(idx_raw)

    if not spec:
        return role_indices

    visual = spec.get("visual") if isinstance(spec.get("visual"), dict) else {}
    role_map_raw = visual.get("role_map") if isinstance(visual, dict) else {}
    if not isinstance(role_map_raw, dict):
        return role_indices

    col_index_by_name = {name: idx for idx, name in enumerate(columns)}
    lower_name_to_index: dict[str, int] = {}
    for idx, name in enumerate(columns):
        lower = name.lower()
        if lower not in lower_name_to_index:
            lower_name_to_index[lower] = idx

    for role, mapped_col in role_map_raw.items():
        role_name = str(role).strip()
        col_name = str(mapped_col).strip()
        if not role_name or not col_name:
            continue
        if role_name not in role_indices:
            raise ValueError(f"Unknown role '{role_name}' for template {template_id}")

        if col_name in col_index_by_name:
            role_indices[role_name] = col_index_by_name[col_name]
            continue

        lowered = col_name.lower()
        if lowered in lower_name_to_index:
            role_indices[role_name] = lower_name_to_index[lowered]
            continue

        raise ValueError(f"Role '{role_name}' maps to unknown column '{col_name}'")

    return role_indices


def _parse_bool(value: object, default_value: bool) -> bool:
    if isinstance(value, bool):
        return value
    if value is None:
        return default_value
    raw = str(value).strip().lower()
    if raw in {"1", "true", "yes", "on"}:
        return True
    if raw in {"0", "false", "no", "off"}:
        return False
    return default_value


def _apply_chart_spec_visual_overrides(template_id: str, option: dict, spec: dict[str, object]) -> dict[str, object]:
    if template_id == "custom_echarts":
        return option

    visual = spec.get("visual") if isinstance(spec.get("visual"), dict) else {}
    if not isinstance(visual, dict):
        return option

    legend_show = _parse_bool(visual.get("legend_show"), True)
    if isinstance(option.get("legend"), dict):
        option["legend"]["show"] = legend_show

    zoom_inside = _parse_bool(visual.get("zoom_inside"), True)
    zoom_slider = _parse_bool(visual.get("zoom_slider"), False)
    data_zoom = option.get("dataZoom") if isinstance(option.get("dataZoom"), list) else []
    zoom_start = _coerce_positive_int(visual.get("zoom_start_pct"), 0, 0, 100)
    zoom_end = _coerce_positive_int(visual.get("zoom_end_pct"), 100, 0, 100)
    next_data_zoom: list[dict[str, object]] = []
    if zoom_inside:
        next_data_zoom.append(
            {
                "type": "inside",
                "xAxisIndex": 0,
                "filterMode": "none",
                "start": zoom_start,
                "end": max(zoom_start, zoom_end),
            }
        )
    if zoom_slider:
        next_data_zoom.append(
            {
                "type": "slider",
                "xAxisIndex": 0,
                "start": zoom_start,
                "end": max(zoom_start, zoom_end),
                "height": 16,
                "bottom": 30,
                "borderColor": "#495057",
                "fillerColor": "rgba(13, 110, 253, 0.20)",
                "handleStyle": {"color": "#0d6efd"},
            }
        )
    option["dataZoom"] = next_data_zoom if next_data_zoom else data_zoom

    smooth_line = _parse_bool(visual.get("smooth_line"), True)
    value_color = str(visual.get("value_color") or "").strip()
    series = option.get("series")
    if isinstance(series, list):
        for item in series:
            if not isinstance(item, dict):
                continue
            if str(item.get("name", "")) != "Value":
                continue
            if "type" in item and str(item.get("type")) == "line":
                item["smooth"] = smooth_line
            if value_color:
                line_style: dict[str, object] = {}
                item_style: dict[str, object] = {}
                existing_line_style = item.get("lineStyle")
                existing_item_style = item.get("itemStyle")
                if isinstance(existing_line_style, dict):
                    for key, val in existing_line_style.items():
                        line_style[str(key)] = val
                if isinstance(existing_item_style, dict):
                    for key, val in existing_item_style.items():
                        item_style[str(key)] = val
                line_style["color"] = value_color
                item_style["color"] = value_color
                item["lineStyle"] = line_style
                item["itemStyle"] = item_style

    _ = template_id
    return option


def _infer_column_types(columns: list[str], rows: list[list[object]]) -> list[str]:
    inferred: list[str] = []
    for idx, _col in enumerate(columns):
        detected = "null"
        for row in rows:
            if idx >= len(row):
                continue
            value = row[idx]
            if value is None:
                continue
            detected = type(value).__name__
            break
        inferred.append(detected)
    return inferred


def _public_dashboard_query_error(exc: Exception) -> str:
    """Extract a concise, user-safe error message from a database exception."""
    raw = str(exc).strip()
    if not raw:
        return "Query execution failed"
    message = raw.splitlines()[0].strip()
    message = re.sub(r"^Code:\s*\d+\.\s*DB::Exception:\s*", "", message)
    message = re.sub(r"^DB::Exception:\s*", "", message)
    message = message.split(": while executing function", 1)[0].strip()
    message = message.split(". Stack trace", 1)[0].strip()
    if not message:
        return "Query execution failed"
    if (
        any(code in raw for code in ("NO_COMMON_TYPE", "TYPE_MISMATCH"))
        and "Check casts and column types." not in message
    ):
        message = f"{message}. Check casts and column types."
    if len(message) > 280:
        message = message[:277].rstrip() + "..."
    return message


def _deep_substitute(obj: object, bindings: dict) -> object:
    """Recursively substitute {{key}} placeholders in a JSON object."""
    if isinstance(obj, dict):
        return {key: _deep_substitute(value, bindings) for key, value in obj.items()}
    if isinstance(obj, list):
        return [_deep_substitute(item, bindings) for item in obj]
    if isinstance(obj, str):
        for key, value in bindings.items():
            placeholder = f"{{{{{key}}}}}"
            if placeholder in obj:
                return value if value is not None else obj
        return obj
    return obj


def _extract_bindings(
    template: dict,
    columns: list[str],
    rows: list,
    role_indices: dict[str, int] | None = None,
) -> dict:
    """Extract data bindings from query results based on column roles."""
    column_roles = role_indices if isinstance(role_indices, dict) else template.get("column_roles", {})
    bindings: dict[str, object] = {}

    for role, col_idx_raw in column_roles.items():
        col_idx = int(col_idx_raw) if isinstance(col_idx_raw, (int, float)) else 0
        if col_idx < len(columns):
            values = [row[col_idx] if isinstance(row, (list, tuple)) else row.get(columns[col_idx]) for row in rows]
            bindings[role] = values

    if "time" in bindings:
        bindings["time"] = bindings["time"]

    if "x_category" in bindings and "y_category" in bindings and "value" in bindings:
        x_vals = bindings["x_category"]
        y_vals = bindings["y_category"]
        v_vals = bindings["value"]
        if isinstance(x_vals, list) and isinstance(y_vals, list) and isinstance(v_vals, list):
            x_unique = sorted(set(x_vals))
            y_unique = sorted(set(y_vals))
            bindings["x_unique_values"] = x_unique
            bindings["y_unique_values"] = y_unique

            heatmap_data = []
            for i, x_val in enumerate(x_unique):
                for j, y_val in enumerate(y_unique):
                    for x_item, y_item, val in zip(x_vals, y_vals, v_vals):
                        if x_item == x_val and y_item == y_val:
                            heatmap_data.append([i, j, val])
                            break
            bindings["heatmap_data"] = heatmap_data
            v_nums = [value for value in v_vals if isinstance(value, (int, float))]
            bindings["value_min"] = min(v_nums) if v_nums else 0
            bindings["value_max"] = max(v_nums) if v_nums else 1

    if "min" in bindings and "max" in bindings:
        min_vals = bindings["min"]
        q1_vals = bindings["q1"]
        med_vals = bindings["median"]
        q3_vals = bindings["q3"]
        max_vals = bindings["max"]
        if (
            isinstance(min_vals, list)
            and isinstance(q1_vals, list)
            and isinstance(med_vals, list)
            and isinstance(q3_vals, list)
            and isinstance(max_vals, list)
        ):
            bindings["boxplot_data"] = [
                [item[0], item[1], item[2], item[3], item[4]]
                for item in zip(min_vals, q1_vals, med_vals, q3_vals, max_vals)
            ]
            bindings["dimension_values"] = bindings.get("dimension", [])

    if "value" in bindings and isinstance(bindings["value"], list) and bindings["value"]:
        value_list = bindings["value"]
        if isinstance(value_list, list) and value_list:
            bindings["value_first"] = value_list[0]

    state_binding = bindings.get("effective_state", bindings.get("anomaly_state"))
    if isinstance(state_binding, list):
        state_colors = {"outlier": "#dc3545", "warning": "#ffc107", "normal": "#0d6efd"}
        state_sizes = {"outlier": 10, "warning": 7, "normal": 4}
        bindings["anomaly_point_color"] = [state_colors.get(str(state), "#0d6efd") for state in state_binding]
        bindings["anomaly_symbol_size"] = [state_sizes.get(str(state), 4) for state in state_binding]

    if str(template.get("id", "")) == "derived_signal_overlay":
        bindings["value_axis_min"] = "dataMin"
        bindings["value_axis_max"] = "dataMax"
        bindings["zoom_start_pct"] = 0
        bindings["signal_summary"] = ""
        bindings["y_axis_name"] = "Value"

        signal_binding = bindings.get("signal")
        signal_name = ""
        if isinstance(signal_binding, list) and signal_binding:
            signal_name = str(signal_binding[0]).lower()

        if "ratio" in signal_name:
            bindings["value_axis_min"] = 0
            bindings["value_axis_max"] = 1
        elif any(token in signal_name for token in ("volume", "count", "latency", "duration", "p95", "p99")):
            bindings["value_axis_min"] = 0

        time_values = bindings.get("time")
        value_values = bindings.get("value")
        baseline_mean_values = bindings.get("baseline_mean")
        baseline_lower_values = bindings.get("baseline_lower")
        baseline_upper_values = bindings.get("baseline_upper")
        effective_states = bindings.get("effective_state", bindings.get("anomaly_state"))

        if (
            isinstance(time_values, list)
            and isinstance(value_values, list)
            and isinstance(baseline_mean_values, list)
            and isinstance(baseline_lower_values, list)
            and isinstance(baseline_upper_values, list)
        ):
            state_to_rank = {"normal": 0, "warning": 1, "outlier": 2}
            rank_series: list[int] = []
            if isinstance(effective_states, list):
                rank_series = [state_to_rank.get(str(state), 0) for state in effective_states]
            if not rank_series:
                rank_series = [0 for _ in value_values]

            use_delta_mode = "ratio" not in signal_name
            plot_values: list[float] = []
            plot_baseline: list[float] = []
            plot_lower: list[float] = []
            plot_upper: list[float] = []
            if use_delta_mode:
                bindings["y_axis_name"] = "Delta %"
                for idx in range(
                    min(
                        len(value_values),
                        len(baseline_mean_values),
                        len(baseline_lower_values),
                        len(baseline_upper_values),
                    )
                ):
                    base = float(baseline_mean_values[idx])
                    val = float(value_values[idx])
                    low = float(baseline_lower_values[idx])
                    up = float(baseline_upper_values[idx])
                    if abs(base) < 1e-9:
                        plot_values.append(0.0)
                        plot_baseline.append(0.0)
                        plot_lower.append(0.0)
                        plot_upper.append(0.0)
                    else:
                        denom = abs(base)
                        plot_values.append(((val - base) / denom) * 100.0)
                        plot_baseline.append(0.0)
                        plot_lower.append(((low - base) / denom) * 100.0)
                        plot_upper.append(((up - base) / denom) * 100.0)
                if plot_values:
                    min_bound = min(plot_lower + plot_values)
                    max_bound = max(plot_upper + plot_values)
                    span = max(5.0, (max_bound - min_bound) * 0.15)
                    bindings["value_axis_min"] = round(min_bound - span, 2)
                    bindings["value_axis_max"] = round(max_bound + span, 2)
            else:
                plot_values = [float(value) for value in value_values]
                plot_baseline = [float(value) for value in baseline_mean_values]
                plot_lower = [max(0.0, float(value)) for value in baseline_lower_values]
                plot_upper = [float(value) for value in baseline_upper_values]

            value_points = [
                [time_values[idx], plot_values[idx], rank_series[idx] if idx < len(rank_series) else 0]
                for idx in range(min(len(time_values), len(plot_values)))
            ]
            bindings["baseline_mean_points"] = [
                [time_values[idx], plot_baseline[idx]] for idx in range(min(len(time_values), len(plot_baseline)))
            ]
            bindings["baseline_lower_points"] = [
                [time_values[idx], plot_lower[idx]] for idx in range(min(len(time_values), len(plot_lower)))
            ]
            bindings["baseline_upper_points"] = [
                [time_values[idx], max(0.0, float(plot_upper[idx]) - float(plot_lower[idx]))]
                for idx in range(min(len(time_values), len(plot_upper), len(plot_lower)))
            ]

            mark_areas: list[list[dict[str, object]]] = []
            warning_points = [point[:2] for point in value_points if len(point) >= 3 and int(point[2]) == 1]
            outlier_points = [point[:2] for point in value_points if len(point) >= 3 and int(point[2]) == 2]
            if isinstance(effective_states, list) and time_values:
                index = 0
                while index < min(len(effective_states), len(time_values)):
                    state = str(effective_states[index])
                    if state == "normal":
                        index += 1
                        continue
                    start_idx = index
                    while index + 1 < len(effective_states) and str(effective_states[index + 1]) == state:
                        index += 1
                    end_idx = index
                    shade = "rgba(255, 193, 7, 0.15)" if state == "warning" else "rgba(220, 53, 69, 0.15)"
                    mark_areas.append(
                        [
                            {"name": state.title(), "itemStyle": {"color": shade}, "xAxis": time_values[start_idx]},
                            {"xAxis": time_values[end_idx]},
                        ]
                    )
                    index += 1

            latest_value = float(value_values[-1]) if value_values else 0.0
            latest_baseline = float(baseline_mean_values[-1]) if baseline_mean_values else 0.0
            delta_pct = 0.0
            if abs(latest_baseline) > 1e-9:
                delta_pct = ((latest_value - latest_baseline) / abs(latest_baseline)) * 100.0
            warning_count = len(warning_points)
            outlier_count = len(outlier_points)
            bindings["signal_summary"] = (
                f"now {latest_value:.1f} | baseline {latest_baseline:.1f} | "
                f"Δ {delta_pct:+.0f}% | warn {warning_count} | outlier {outlier_count}"
            )

            bindings["value_points"] = value_points
            bindings["anomaly_mark_areas"] = mark_areas
            bindings["warning_points"] = warning_points
            bindings["outlier_points"] = outlier_points

    return bindings
