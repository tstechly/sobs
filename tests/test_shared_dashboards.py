import json

import pytest

from shared.dashboards import (
    _build_chart_record,
    _build_chart_tombstones,
    _build_dashboard_record,
    _build_dashboard_templates,
    _get_charts,
    _get_dashboard,
    _get_dashboards,
    _parse_chart_form_submission,
    _prepare_query_add_to_dashboard_chart,
    _serialize_chart_row,
    _serialize_dashboard_row,
)


class _FakeResult:
    def __init__(self, rows, one=None):
        self._rows = rows
        self._one = one

    def fetchall(self):
        return self._rows

    def fetchone(self):
        return self._one


class _FakeDb:
    def __init__(self, rows=None, one=None):
        self.rows = rows or []
        self.one = one
        self.calls: list[tuple[str, list[str] | None]] = []

    def execute(self, query, params=None):
        self.calls.append((query, params))
        return _FakeResult(self.rows, self.one)


def test_serialize_dashboard_and_chart_rows_cover_db_mapping_and_chart_spec_normalization():
    dashboard = _serialize_dashboard_row({"Id": 7, "Name": "Ops", "Description": "Main board"})
    assert dashboard == {"id": "7", "name": "Ops", "description": "Main board"}

    build_calls: list[tuple[str, str, str]] = []

    def _fake_build_raw_chart_spec(chart_type, query, options_json):
        build_calls.append((chart_type, query, options_json))
        return {"template_id": chart_type, "sql": {"mode": "raw", "override_sql": query}}

    chart = _serialize_chart_row(
        {
            "Id": 9,
            "Title": "Errors",
            "ChartType": "heatmap",
            "Query": "SELECT 1",
            "OptionsJson": "{}",
            "Position": 3,
        },
        build_raw_chart_spec=_fake_build_raw_chart_spec,
    )
    assert chart["id"] == "9"
    assert chart["title"] == "Errors"
    assert chart["position"] == 3
    assert chart["chart_spec"]["template_id"] == "heatmap"
    assert json.loads(chart["options_json"])["chart_spec"]["sql"]["override_sql"] == "SELECT 1"
    assert build_calls == [("heatmap", "SELECT 1", "{}")]


def test_get_dashboards_get_dashboard_and_get_charts_issue_expected_queries():
    dashboards_db = _FakeDb(rows=[{"Id": 1, "Name": "A", "Description": "First"}])
    assert _get_dashboards(dashboards_db) == [{"id": "1", "name": "A", "description": "First"}]
    assert "sobs_dashboards" in dashboards_db.calls[0][0]

    dashboard_db = _FakeDb(one={"Id": 2, "Name": "B", "Description": "Second"})
    assert _get_dashboard(dashboard_db, "db-2") == {"id": "2", "name": "B", "description": "Second"}
    assert dashboard_db.calls[0][1] == ["db-2"]

    missing_db = _FakeDb(one=None)
    assert _get_dashboard(missing_db, "missing") is None

    charts_db = _FakeDb(
        rows=[
            {
                "Id": 10,
                "Title": "Latency",
                "ChartType": "time_series_percentiles",
                "Query": "SELECT 1",
                "OptionsJson": "{}",
                "Position": 0,
            }
        ]
    )
    charts = _get_charts(
        charts_db,
        "db-3",
        build_raw_chart_spec=lambda chart_type, query, options_json: {"template_id": chart_type, "query": query},
    )
    assert charts[0]["chart_spec"] == {"template_id": "time_series_percentiles", "query": "SELECT 1"}
    assert charts_db.calls[0][1] == ["db-3"]


def test_build_dashboard_and_chart_records_cover_active_and_deleted_paths():
    dashboard_record = _build_dashboard_record("db-1", "Ops", "Main", version=123)
    assert dashboard_record == {
        "Id": "db-1",
        "Name": "Ops",
        "Description": "Main",
        "IsDeleted": 0,
        "Version": 123,
    }

    chart_record = _build_chart_record("chart-1", "db-1", "Latency", "heatmap", "SELECT 1", "{}", 4, version=456)
    assert chart_record["DashboardId"] == "db-1"
    assert chart_record["Position"] == 4
    assert chart_record["IsDeleted"] == 0

    tombstones = _build_chart_tombstones(
        [
            {
                "id": "chart-1",
                "title": "Latency",
                "chart_type": "heatmap",
                "query": "SELECT 1",
                "options_json": "{}",
                "position": 4,
            }
        ],
        "db-1",
        version=789,
    )
    assert tombstones == [
        {
            "Id": "chart-1",
            "DashboardId": "db-1",
            "Title": "Latency",
            "ChartType": "heatmap",
            "Query": "SELECT 1",
            "OptionsJson": "{}",
            "Position": 4,
            "IsDeleted": 1,
            "Version": 789,
        }
    ]


def test_parse_chart_form_submission_validates_required_fields_and_compiles_spec():
    compile_calls: list[object] = []

    def _fake_compile(spec_raw):
        compile_calls.append(spec_raw)
        return "gauge_kpi", "SELECT 1 AS value", {"template_id": "gauge_kpi"}

    title, template_id, query, options_json = _parse_chart_form_submission(
        {"title": "KPI", "chart_spec_json": '{"template_id": "gauge_kpi"}'},
        compile_chart_spec=_fake_compile,
    )
    assert title == "KPI"
    assert template_id == "gauge_kpi"
    assert query == "SELECT 1 AS value"
    assert json.loads(options_json) == {"chart_spec": {"template_id": "gauge_kpi"}}
    assert compile_calls == [{"template_id": "gauge_kpi"}]

    with pytest.raises(ValueError, match="Chart title is required"):
        _parse_chart_form_submission({}, compile_chart_spec=_fake_compile)

    with pytest.raises(ValueError, match="Chart spec is required"):
        _parse_chart_form_submission({"title": "X"}, compile_chart_spec=_fake_compile)

    with pytest.raises(ValueError, match="Chart spec error"):
        _parse_chart_form_submission(
            {"title": "X", "chart_spec_json": "{bad json"},
            compile_chart_spec=_fake_compile,
        )


def test_prepare_query_add_to_dashboard_chart_validates_payload_and_builds_record():
    compile_calls: list[object] = []

    def _fake_compile(spec_raw):
        compile_calls.append(spec_raw)
        return "custom_echarts", "SELECT 1 AS value", {"template_id": "custom_echarts", "visual": {}}

    prepared = _prepare_query_add_to_dashboard_chart(
        {
            "dashboard_id": "db-1",
            "title": "Saved Query",
            "sql": "SELECT * FROM system.tables",
            "chart_spec": {"title": {"text": "Tables"}},
        },
        compile_chart_spec=_fake_compile,
        next_position=5,
        chart_id_factory=lambda: "chart-9",
        version=1234,
    )
    assert prepared["dashboard_id"] == "db-1"
    assert prepared["chart_id"] == "chart-9"
    assert prepared["position"] == 5
    assert prepared["record"]["Version"] == 1234
    assert prepared["record"]["Title"] == "Saved Query"
    assert compile_calls[0]["template_id"] == "custom_echarts"
    assert compile_calls[0]["visual"]["custom_mapping_json"] == "{}"

    defaulted = _prepare_query_add_to_dashboard_chart(
        {"dashboard_id": "db-1", "sql": "SELECT 1", "chart_spec": '{"title": {"text": "T"}}'},
        compile_chart_spec=_fake_compile,
        next_position=0,
        chart_id_factory=lambda: "chart-10",
        version=1,
    )
    assert defaulted["title"] == "Query Chart"

    for payload, message in [
        ({}, "dashboard_id is required"),
        ({"dashboard_id": "db-1"}, "sql is required"),
        ({"dashboard_id": "db-1", "sql": "SELECT 1"}, "chart_spec is required"),
    ]:
        with pytest.raises(ValueError, match=message):
            _prepare_query_add_to_dashboard_chart(
                payload,
                compile_chart_spec=_fake_compile,
                next_position=0,
                chart_id_factory=lambda: "chart-11",
                version=1,
            )

    with pytest.raises(ValueError, match="chart_spec must be valid JSON"):
        _prepare_query_add_to_dashboard_chart(
            {"dashboard_id": "db-1", "sql": "SELECT 1", "chart_spec": "{bad"},
            compile_chart_spec=_fake_compile,
            next_position=0,
            chart_id_factory=lambda: "chart-11",
            version=1,
        )

    with pytest.raises(ValueError, match="chart_spec must be a JSON object"):
        _prepare_query_add_to_dashboard_chart(
            {"dashboard_id": "db-1", "sql": "SELECT 1", "chart_spec": []},
            compile_chart_spec=_fake_compile,
            next_position=0,
            chart_id_factory=lambda: "chart-11",
            version=1,
        )

    with pytest.raises(ValueError, match="Chart spec error"):
        _prepare_query_add_to_dashboard_chart(
            {"dashboard_id": "db-1", "sql": "SELECT 1", "chart_spec": {}},
            compile_chart_spec=lambda spec_raw: (_ for _ in ()).throw(RuntimeError("boom")),
            next_position=0,
            chart_id_factory=lambda: "chart-11",
            version=1,
        )


def test_build_dashboard_templates_maps_template_metadata_and_defaults():
    templates = _build_dashboard_templates(
        {
            "heatmap": {
                "name": "Heatmap",
                "description": "Colored grid",
                "icon": "bi bi-grid",
                "query_shape": "x/y/value",
                "sample_sql": "SELECT 1",
                "drilldown": {"target": "logs"},
            },
            "gauge_kpi": {
                "name": "Gauge",
                "description": "Single value",
                "icon": "bi bi-speedometer2",
            },
        },
        default_chart_spec=lambda template_id: {"template_id": template_id},
    )
    assert templates == [
        {
            "id": "gauge_kpi",
            "name": "Gauge",
            "description": "Single value",
            "icon": "bi bi-speedometer2",
            "query_shape": "",
            "sample_sql": "",
            "drilldown": None,
            "default_spec": {"template_id": "gauge_kpi"},
        },
        {
            "id": "heatmap",
            "name": "Heatmap",
            "description": "Colored grid",
            "icon": "bi bi-grid",
            "query_shape": "x/y/value",
            "sample_sql": "SELECT 1",
            "drilldown": {"target": "logs"},
            "default_spec": {"template_id": "heatmap"},
        },
    ]
