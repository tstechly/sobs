from __future__ import annotations

import json
import re
from collections.abc import Callable, Mapping

SUPPORTED_CHART_OPTION_SOURCES = {
    "v_derived_signals_anomaly",
    "v_otel_metrics_anomaly",
    "otel_metrics_gauge",
    "otel_metrics_sum",
    "otel_metrics_histogram",
    "otel_logs",
    "otel_traces",
    "sobs_error_resolutions",
}


def _apply_query_limit(query: str, *, default_limit: int) -> str:
    if re.search(r"\bLIMIT\b", query, re.IGNORECASE):
        return query
    return query.rstrip(";") + f" LIMIT {default_limit}"


def _rows_to_columns_and_data(rows: list[Mapping[str, object]]) -> tuple[list[str], list[list[object]]]:
    if not rows:
        return [], []
    first_row = rows[0]
    columns = [str(column) for column in first_row.keys()]
    return columns, [[row[column] for column in columns] for row in rows]


def _execute_chart_query_result(
    db,
    query: str,
    *,
    default_limit: int,
    include_rows: bool,
    include_records: bool,
) -> dict[str, object]:
    run_query = _apply_query_limit(query, default_limit=default_limit)
    raw_rows = db.execute(run_query).fetchall()
    columns, rows = _rows_to_columns_and_data(raw_rows)
    payload: dict[str, object] = {
        "columns": columns,
    }
    if include_rows:
        payload["rows"] = rows
    if include_records:
        payload["records"] = [dict(row) for row in raw_rows]
    return payload


def _build_chart_spec_template_api_payload(
    chart_templates: Mapping[str, Mapping[str, object]],
    *,
    default_chart_spec,
) -> list[dict[str, object]]:
    return [
        {
            "id": template_id,
            "name": str(template["name"]),
            "description": str(template["description"]),
            "query_shape": str(template.get("query_shape", "")),
            "sample_sql": str(template.get("sample_sql", "")),
            "default_spec": default_chart_spec(template_id),
            "min_columns": template.get("min_columns", 0),
            "max_columns": template.get("max_columns"),
            "column_roles": template.get("column_roles", {}),
        }
        for template_id, template in sorted(chart_templates.items())
    ]


def _build_chart_spec_options(
    source_view: str,
    signal_source: str,
    limit: int,
    *,
    distinct_values: Callable[[str], list[str]],
    sql_literal,
) -> dict[str, object]:
    if source_view not in SUPPORTED_CHART_OPTION_SOURCES:
        raise ValueError("Unsupported source for options")

    services: list[str] = []
    signals: list[str] = []
    metrics: list[str] = []

    if source_view == "v_derived_signals_anomaly":
        services = distinct_values(
            "SELECT DISTINCT ServiceName AS v "
            "FROM v_derived_signals_anomaly "
            "WHERE time >= now() - INTERVAL 24 HOUR "
            "ORDER BY v "
            f"LIMIT {limit}"
        )
        signals = distinct_values(
            "SELECT DISTINCT SignalName AS v "
            "FROM v_derived_signals_anomaly "
            "WHERE time >= now() - INTERVAL 24 HOUR"
            + (f" AND SignalSource = {sql_literal(signal_source)}" if signal_source else "")
            + " ORDER BY v "
            f"LIMIT {limit}"
        )
    elif source_view in {"otel_logs", "otel_traces"}:
        services = distinct_values(
            "SELECT DISTINCT ServiceName AS v " f"FROM {source_view} " "ORDER BY v " f"LIMIT {limit}"
        )
        signals = ["log_volume"] if source_view == "otel_logs" else ["trace_volume"]
    elif source_view == "sobs_error_resolutions":
        signals = ["resolved_error_volume"]
    else:
        services = distinct_values(
            "SELECT DISTINCT ServiceName AS v " f"FROM {source_view} " "ORDER BY v " f"LIMIT {limit}"
        )
        metrics = distinct_values(
            "SELECT DISTINCT MetricName AS v " f"FROM {source_view} " "ORDER BY v " f"LIMIT {limit}"
        )

    return {
        "source_view": source_view,
        "services": services,
        "signals": signals,
        "metrics": metrics,
    }


def _execute_chart_spec_named_queries(
    db,
    named_queries: object,
    *,
    default_limit: int,
    include_records: bool,
    public_query_error: Callable[[Exception], str],
) -> list[dict[str, object]]:
    results: list[dict[str, object]] = []
    if not isinstance(named_queries, list):
        return results
    for named_query in named_queries:
        if not isinstance(named_query, dict):
            continue
        query_name = str(named_query.get("name") or "").strip()
        query_sql = str(named_query.get("sql") or "").strip()
        if not query_name or not query_sql:
            continue
        run_sql = _apply_query_limit(query_sql, default_limit=default_limit)
        try:
            query_rows = db.execute(run_sql).fetchall()
            columns, rows = _rows_to_columns_and_data(query_rows)
            item: dict[str, object] = {
                "name": query_name,
                "purpose": str(named_query.get("purpose") or ""),
                "columns": columns,
                "rows": rows,
                "error": "",
            }
            if include_records:
                item["records"] = [dict(row) for row in query_rows]
            results.append(item)
        except Exception as exc:
            item = {
                "name": query_name,
                "purpose": str(named_query.get("purpose") or ""),
                "columns": [],
                "rows": [],
                "error": public_query_error(exc),
            }
            if include_records:
                item["records"] = []
            results.append(item)
    return results


def _build_named_datasets(
    named_query_results: list[Mapping[str, object]],
    *,
    warn_named_query_failure: Callable[[str, str], None] | None = None,
) -> dict[str, dict[str, object]]:
    datasets: dict[str, dict[str, object]] = {}
    for result in named_query_results:
        query_name = str(result.get("name") or "").strip()
        if not query_name:
            continue
        error = str(result.get("error") or "")
        if error and warn_named_query_failure is not None:
            warn_named_query_failure(query_name, error)
        datasets[query_name] = {
            "columns": result.get("columns") or [],
            "records": result.get("records") or [],
            "rows": result.get("rows") or [],
        }
    return datasets


def _build_ai_chart_datasets(
    sql: str,
    columns: list[str],
    rows: list[list[object]],
    named_query_results: list[Mapping[str, object]],
) -> list[dict[str, object]]:
    datasets: list[dict[str, object]] = [
        {
            "name": "main",
            "purpose": "primary dataset",
            "sql": sql,
            "columns": columns,
            "rows": rows,
        }
    ]
    for named_query in named_query_results:
        if str(named_query.get("error") or ""):
            continue
        datasets.append(
            {
                "name": str(named_query.get("name") or ""),
                "purpose": str(named_query.get("purpose") or ""),
                "sql": str(named_query.get("sql") or ""),
                "columns": named_query.get("columns") or [],
                "rows": named_query.get("rows") or [],
            }
        )
    return datasets


def _finalize_ai_chart_generation(
    chart_spec_json: str,
    chart_error: str,
    columns: list[str],
    *,
    infer_custom_mapping_from_option: Callable[[str, list[str]], Mapping[str, object] | None],
    build_fallback_custom_option_json: Callable[[], str],
) -> tuple[str, str, str]:
    if chart_spec_json:
        inferred_mapping = infer_custom_mapping_from_option(chart_spec_json, columns)
        custom_mapping_json = json.dumps(inferred_mapping, ensure_ascii=False) if inferred_mapping else "{}"
        return chart_spec_json, custom_mapping_json, chart_error

    custom_mapping_json = json.dumps({"points": {"from": "rows"}}, ensure_ascii=False)
    chart_error = (
        f"{chart_error} Using fallback chart option template."
        if chart_error
        else "Chart generation failed; using fallback chart option template."
    )
    return build_fallback_custom_option_json(), custom_mapping_json, chart_error


def _build_ai_chart_spec_response(
    sql: str,
    sql_retry_count: int,
    columns: list[str],
    named_query_results: list[Mapping[str, object]],
    chart_spec_json: str,
    custom_mapping_json: str,
    chart_error: str,
) -> dict[str, object]:
    named_queries = [
        {
            "name": str(named_query.get("name") or ""),
            "sql": str(named_query.get("sql") or ""),
            "purpose": str(named_query.get("purpose") or ""),
        }
        for named_query in named_query_results
        if not str(named_query.get("error") or "") and named_query.get("name") and named_query.get("sql")
    ]

    return {
        "ok": True,
        "spec": {
            "template_id": "custom_echarts",
            "sql": {"mode": "raw", "override_sql": sql},
            "named_queries": named_queries,
            "visual": {
                "custom_option_json": chart_spec_json or "{}",
                "custom_mapping_json": custom_mapping_json,
            },
        },
        "sql": sql,
        "retry_count": sql_retry_count,
        "columns": columns,
        "named_queries": named_queries,
        "named_query_results": named_query_results,
        "chart_error": chart_error,
    }
