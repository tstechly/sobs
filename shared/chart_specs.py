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
