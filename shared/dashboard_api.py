from __future__ import annotations

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
