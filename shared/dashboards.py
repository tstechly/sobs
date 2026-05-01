from __future__ import annotations

import json
from collections.abc import Mapping


def _get_dashboards(db) -> list[dict[str, str]]:
    rows = db.execute(
        "SELECT Id, Name, Description FROM sobs_dashboards FINAL WHERE IsDeleted = 0 ORDER BY Name"
    ).fetchall()
    return [_serialize_dashboard_row(row) for row in rows]


def _get_dashboard(db, dashboard_id: str) -> dict[str, str] | None:
    row = db.execute(
        "SELECT Id, Name, Description FROM sobs_dashboards FINAL WHERE IsDeleted = 0 AND Id = ?",
        [dashboard_id],
    ).fetchone()
    if not row:
        return None
    return _serialize_dashboard_row(row)


def _get_charts(db, dashboard_id: str, *, build_raw_chart_spec) -> list[dict[str, object]]:
    rows = db.execute(
        "SELECT Id, Title, ChartType, Query, OptionsJson, Position "
        "FROM sobs_chart_configs FINAL WHERE IsDeleted = 0 AND DashboardId = ? "
        "ORDER BY Position, Id",
        [dashboard_id],
    ).fetchall()
    return [_serialize_chart_row(row, build_raw_chart_spec=build_raw_chart_spec) for row in rows]


def _serialize_dashboard_row(row) -> dict[str, str]:
    return {
        "id": str(row["Id"]),
        "name": str(row["Name"]),
        "description": str(row["Description"]),
    }


def _serialize_chart_row(row, *, build_raw_chart_spec) -> dict[str, object]:
    chart_type = str(row["ChartType"])
    query = str(row["Query"])
    options_json = str(row["OptionsJson"])
    chart_spec = build_raw_chart_spec(chart_type, query, options_json)
    return {
        "id": str(row["Id"]),
        "title": str(row["Title"]),
        "chart_type": chart_type,
        "query": query,
        "options_json": json.dumps({"chart_spec": chart_spec}, ensure_ascii=False),
        "position": int(row["Position"]),
        "chart_spec": chart_spec,
    }


def _build_dashboard_record(
    dashboard_id: str,
    name: str,
    description: str,
    *,
    version: int,
    is_deleted: int = 0,
) -> dict[str, object]:
    return {
        "Id": dashboard_id,
        "Name": name,
        "Description": description,
        "IsDeleted": is_deleted,
        "Version": version,
    }


def _build_chart_record(
    chart_id: str,
    dashboard_id: str,
    title: str,
    chart_type: str,
    query: str,
    options_json: str,
    position: int,
    *,
    version: int,
    is_deleted: int = 0,
) -> dict[str, object]:
    return {
        "Id": chart_id,
        "DashboardId": dashboard_id,
        "Title": title,
        "ChartType": chart_type,
        "Query": query,
        "OptionsJson": options_json,
        "Position": position,
        "IsDeleted": is_deleted,
        "Version": version,
    }


def _build_chart_tombstones(
    charts: list[dict[str, object]], dashboard_id: str, *, version: int
) -> list[dict[str, object]]:
    return [
        _build_chart_record(
            str(chart["id"]),
            dashboard_id,
            str(chart["title"]),
            str(chart["chart_type"]),
            str(chart["query"]),
            str(chart["options_json"]),
            int(str(chart["position"])),
            version=version,
            is_deleted=1,
        )
        for chart in charts
    ]


def _parse_chart_form_submission(form, *, compile_chart_spec) -> tuple[str, str, str, str]:
    title = str(form.get("title") or "").strip()
    chart_spec_json = str(form.get("chart_spec_json") or "").strip()

    if not title:
        raise ValueError("Chart title is required")
    if not chart_spec_json:
        raise ValueError("Chart spec is required")

    try:
        spec_raw = json.loads(chart_spec_json)
        template_id, query, normalized_spec = compile_chart_spec(spec_raw)
    except Exception as exc:
        raise ValueError(f"Chart spec error: {exc}") from exc

    options_json = json.dumps({"chart_spec": normalized_spec}, ensure_ascii=False)
    return title, template_id, query, options_json


def _prepare_query_add_to_dashboard_chart(
    payload: Mapping[str, object],
    *,
    compile_chart_spec,
    next_position: int,
    chart_id_factory,
    version: int,
) -> dict[str, object]:
    dashboard_id = str(payload.get("dashboard_id") or "").strip()
    title = str(payload.get("title") or "").strip() or "Query Chart"
    sql = str(payload.get("sql") or "").strip()
    chart_spec_raw = payload.get("chart_spec")

    if not dashboard_id:
        raise ValueError("dashboard_id is required")
    if not sql:
        raise ValueError("sql is required")
    if chart_spec_raw is None or (isinstance(chart_spec_raw, str) and not chart_spec_raw.strip()):
        raise ValueError("chart_spec is required")

    try:
        chart_option = json.loads(chart_spec_raw) if isinstance(chart_spec_raw, str) else chart_spec_raw
    except Exception as exc:
        raise ValueError(f"chart_spec must be valid JSON: {exc}") from exc
    if not isinstance(chart_option, dict):
        raise ValueError("chart_spec must be a JSON object")

    spec_raw = {
        "template_id": "custom_echarts",
        "sql": {"mode": "raw", "override_sql": sql},
        "visual": {
            "custom_option_json": json.dumps(chart_option, ensure_ascii=False),
            "custom_mapping_json": "{}",
        },
    }
    try:
        template_id, query, normalized_spec = compile_chart_spec(spec_raw)
    except Exception as exc:
        raise ValueError(f"Chart spec error: {exc}") from exc

    chart_id = str(chart_id_factory())
    options_json = json.dumps({"chart_spec": normalized_spec}, ensure_ascii=False)
    return {
        "dashboard_id": dashboard_id,
        "title": title,
        "chart_id": chart_id,
        "chart_type": template_id,
        "query": query,
        "options_json": options_json,
        "position": next_position,
        "version": version,
        "record": _build_chart_record(
            chart_id,
            dashboard_id,
            title,
            template_id,
            query,
            options_json,
            next_position,
            version=version,
        ),
    }


def _build_dashboard_templates(
    chart_templates: Mapping[str, Mapping[str, object]], *, default_chart_spec
) -> list[dict[str, object]]:
    return [
        {
            "id": template_id,
            "name": str(template["name"]),
            "description": str(template["description"]),
            "icon": str(template["icon"]),
            "query_shape": str(template.get("query_shape", "")),
            "sample_sql": str(template.get("sample_sql", "")),
            "drilldown": template.get("drilldown"),
            "default_spec": default_chart_spec(template_id),
        }
        for template_id, template in sorted(chart_templates.items())
    ]
