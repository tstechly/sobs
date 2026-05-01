import pytest

from shared.dashboard_api import (
    SUPPORTED_CHART_OPTION_SOURCES,
    _apply_query_limit,
    _build_ai_chart_datasets,
    _build_ai_chart_spec_response,
    _build_chart_spec_options,
    _build_chart_spec_template_api_payload,
    _build_named_datasets,
    _execute_chart_query_result,
    _execute_chart_spec_named_queries,
    _finalize_ai_chart_generation,
    _rows_to_columns_and_data,
)


def test_apply_query_limit_preserves_existing_limit_and_adds_default_limit_when_missing():
    assert _apply_query_limit("SELECT 1", default_limit=1000) == "SELECT 1 LIMIT 1000"
    assert _apply_query_limit("SELECT 1;", default_limit=50) == "SELECT 1 LIMIT 50"
    assert _apply_query_limit("SELECT 1 LIMIT 5", default_limit=1000) == "SELECT 1 LIMIT 5"


def test_rows_to_columns_and_data_handles_empty_and_non_empty_rows():
    assert _rows_to_columns_and_data([]) == ([], [])
    columns, data = _rows_to_columns_and_data(
        [
            {"service": "checkout", "count": 2},
            {"service": "payments", "count": 3},
        ]
    )
    assert columns == ["service", "count"]
    assert data == [["checkout", 2], ["payments", 3]]


def test_build_chart_spec_template_api_payload_maps_template_metadata_and_defaults():
    payload = _build_chart_spec_template_api_payload(
        {
            "heatmap": {
                "name": "Heatmap",
                "description": "Grid",
                "query_shape": "x/y/value",
                "sample_sql": "SELECT 1",
                "min_columns": 3,
                "max_columns": 5,
                "column_roles": {"x_category": 0},
            },
            "gauge_kpi": {
                "name": "Gauge",
                "description": "Single value",
            },
        },
        default_chart_spec=lambda template_id: {"template_id": template_id},
    )
    assert payload == [
        {
            "id": "gauge_kpi",
            "name": "Gauge",
            "description": "Single value",
            "query_shape": "",
            "sample_sql": "",
            "default_spec": {"template_id": "gauge_kpi"},
            "min_columns": 0,
            "max_columns": None,
            "column_roles": {},
        },
        {
            "id": "heatmap",
            "name": "Heatmap",
            "description": "Grid",
            "query_shape": "x/y/value",
            "sample_sql": "SELECT 1",
            "default_spec": {"template_id": "heatmap"},
            "min_columns": 3,
            "max_columns": 5,
            "column_roles": {"x_category": 0},
        },
    ]


def test_build_chart_spec_options_covers_all_supported_source_branches_and_errors():
    seen_queries: list[str] = []

    def _fake_distinct_values(query: str) -> list[str]:
        seen_queries.append(query)
        if "ServiceName" in query:
            return ["checkout", "payments"]
        if "SignalName" in query:
            return ["trace_volume"]
        if "MetricName" in query:
            return ["http.server.duration"]
        return []

    derived = _build_chart_spec_options(
        "v_derived_signals_anomaly",
        "traces",
        25,
        distinct_values=_fake_distinct_values,
        sql_literal=lambda value: f"'lit:{value}'",
    )
    assert derived == {
        "source_view": "v_derived_signals_anomaly",
        "services": ["checkout", "payments"],
        "signals": ["trace_volume"],
        "metrics": [],
    }
    assert "SignalSource = 'lit:traces'" in seen_queries[1]

    logs = _build_chart_spec_options(
        "otel_logs",
        "",
        10,
        distinct_values=_fake_distinct_values,
        sql_literal=lambda value: f"'{value}'",
    )
    assert logs["signals"] == ["log_volume"]

    traces = _build_chart_spec_options(
        "otel_traces",
        "",
        10,
        distinct_values=_fake_distinct_values,
        sql_literal=lambda value: f"'{value}'",
    )
    assert traces["signals"] == ["trace_volume"]

    errors = _build_chart_spec_options(
        "sobs_error_resolutions",
        "",
        10,
        distinct_values=_fake_distinct_values,
        sql_literal=lambda value: f"'{value}'",
    )
    assert errors["signals"] == ["resolved_error_volume"]

    metrics = _build_chart_spec_options(
        "otel_metrics_gauge",
        "",
        10,
        distinct_values=_fake_distinct_values,
        sql_literal=lambda value: f"'{value}'",
    )
    assert metrics["services"] == ["checkout", "payments"]
    assert metrics["metrics"] == ["http.server.duration"]

    assert "otel_metrics_sum" in SUPPORTED_CHART_OPTION_SOURCES
    assert "otel_metrics_histogram" in SUPPORTED_CHART_OPTION_SOURCES
    assert "v_otel_metrics_anomaly" in SUPPORTED_CHART_OPTION_SOURCES

    with pytest.raises(ValueError, match="Unsupported source for options"):
        _build_chart_spec_options(
            "missing",
            "",
            10,
            distinct_values=_fake_distinct_values,
            sql_literal=lambda value: f"'{value}'",
        )


class _FakeResult:
    def __init__(self, rows):
        self._rows = rows

    def fetchall(self):
        return self._rows


class _FakeDb:
    def __init__(self, results, failing_queries=None):
        self.results = results
        self.failing_queries = set(failing_queries or [])
        self.executed = []

    def execute(self, query):
        self.executed.append(query)
        if query in self.failing_queries:
            raise RuntimeError("boom")
        return _FakeResult(self.results.get(query, []))


def test_execute_chart_spec_named_queries_executes_limited_queries_and_collects_records():
    db = _FakeDb(
        {
            "SELECT 1 AS num LIMIT 20": [{"num": 1}],
            "SELECT 2 AS num LIMIT 5": [{"num": 2}],
        }
    )

    results = _execute_chart_spec_named_queries(
        db,
        [
            {"name": "first", "sql": "SELECT 1 AS num", "purpose": "alpha"},
            {"name": "second", "sql": "SELECT 2 AS num LIMIT 5", "purpose": "beta"},
            {"name": "", "sql": "SELECT ignored"},
            "bad-entry",
        ],
        default_limit=20,
        include_records=True,
        public_query_error=lambda exc: f"sanitized:{exc}",
    )

    assert db.executed == ["SELECT 1 AS num LIMIT 20", "SELECT 2 AS num LIMIT 5"]
    assert results == [
        {
            "name": "first",
            "purpose": "alpha",
            "columns": ["num"],
            "rows": [[1]],
            "error": "",
            "records": [{"num": 1}],
        },
        {
            "name": "second",
            "purpose": "beta",
            "columns": ["num"],
            "rows": [[2]],
            "error": "",
            "records": [{"num": 2}],
        },
    ]


def test_execute_chart_spec_named_queries_sanitizes_failures_and_build_named_datasets_warns():
    query = "SELECT nope LIMIT 10"
    db = _FakeDb({}, failing_queries={query})
    warnings = []

    results = _execute_chart_spec_named_queries(
        db,
        [{"name": "bad", "sql": "SELECT nope", "purpose": "broken"}],
        default_limit=10,
        include_records=False,
        public_query_error=lambda exc: f"sanitized:{exc}",
    )

    assert results == [
        {
            "name": "bad",
            "purpose": "broken",
            "columns": [],
            "rows": [],
            "error": "sanitized:boom",
        }
    ]

    datasets = _build_named_datasets(
        results + [{"name": "ok", "columns": ["value"], "rows": [[3]], "records": [{"value": 3}]}],
        warn_named_query_failure=lambda name, error: warnings.append((name, error)),
    )

    assert warnings == [("bad", "sanitized:boom")]
    assert datasets == {
        "bad": {"columns": [], "records": [], "rows": []},
        "ok": {"columns": ["value"], "records": [{"value": 3}], "rows": [[3]]},
    }


def test_execute_chart_query_result_shapes_rows_and_records_with_default_limit():
    db = _FakeDb(
        {
            "SELECT service, count FROM metrics LIMIT 25": [
                {"service": "checkout", "count": 2},
                {"service": "payments", "count": 3},
            ]
        }
    )

    payload = _execute_chart_query_result(
        db,
        "SELECT service, count FROM metrics",
        default_limit=25,
        include_rows=True,
        include_records=True,
    )

    assert db.executed == ["SELECT service, count FROM metrics LIMIT 25"]
    assert payload == {
        "columns": ["service", "count"],
        "rows": [["checkout", 2], ["payments", 3]],
        "records": [
            {"service": "checkout", "count": 2},
            {"service": "payments", "count": 3},
        ],
    }


def test_execute_chart_query_result_preserves_existing_limit_and_optional_shapes():
    db = _FakeDb(
        {
            "SELECT value FROM metrics LIMIT 5": [{"value": 7}],
        }
    )

    payload = _execute_chart_query_result(
        db,
        "SELECT value FROM metrics LIMIT 5",
        default_limit=20,
        include_rows=False,
        include_records=True,
    )

    assert db.executed == ["SELECT value FROM metrics LIMIT 5"]
    assert payload == {
        "columns": ["value"],
        "records": [{"value": 7}],
    }


def test_build_ai_chart_datasets_keeps_main_dataset_and_filters_failed_named_queries():
    datasets = _build_ai_chart_datasets(
        "SELECT 1 AS value",
        ["value"],
        [[1]],
        [
            {"name": "good", "purpose": "ok", "sql": "SELECT 2 AS n", "columns": ["n"], "rows": [[2]]},
            {"name": "bad", "purpose": "broken", "sql": "SELECT nope", "columns": ["n"], "rows": [], "error": "boom"},
        ],
    )

    assert datasets == [
        {
            "name": "main",
            "purpose": "primary dataset",
            "sql": "SELECT 1 AS value",
            "columns": ["value"],
            "rows": [[1]],
        },
        {
            "name": "good",
            "purpose": "ok",
            "sql": "SELECT 2 AS n",
            "columns": ["n"],
            "rows": [[2]],
        },
    ]


def test_finalize_ai_chart_generation_uses_inferred_mapping_or_fallback():
    option_json, mapping_json, chart_error = _finalize_ai_chart_generation(
        '{"series":[{"data":"{{rows}}"}]}',
        "",
        ["service", "value"],
        infer_custom_mapping_from_option=lambda _json, columns: {"points": {"columns": columns}},
        build_fallback_custom_option_json=lambda: "fallback",
    )
    assert option_json == '{"series":[{"data":"{{rows}}"}]}'
    assert mapping_json == '{"points": {"columns": ["service", "value"]}}'
    assert chart_error == ""

    fallback_option, fallback_mapping, fallback_error = _finalize_ai_chart_generation(
        "",
        "Chart spec JSON parse error: bad json",
        ["service", "value"],
        infer_custom_mapping_from_option=lambda _json, columns: {"points": {"columns": columns}},
        build_fallback_custom_option_json=lambda: "fallback",
    )
    assert fallback_option == "fallback"
    assert fallback_mapping == '{"points": {"from": "rows"}}'
    assert "fallback chart option template" in fallback_error.lower()


def test_build_ai_chart_spec_response_filters_named_queries_and_builds_spec_payload():
    payload = _build_ai_chart_spec_response(
        "SELECT 1 AS value",
        2,
        ["value"],
        [
            {"name": "good", "sql": "SELECT 2 AS n", "purpose": "ok", "error": ""},
            {"name": "bad", "sql": "SELECT nope", "purpose": "broken", "error": "boom"},
        ],
        '{"series":[]}',
        '{"points":{"from":"rows"}}',
        "",
    )

    assert payload == {
        "ok": True,
        "spec": {
            "template_id": "custom_echarts",
            "sql": {"mode": "raw", "override_sql": "SELECT 1 AS value"},
            "named_queries": [{"name": "good", "sql": "SELECT 2 AS n", "purpose": "ok"}],
            "visual": {
                "custom_option_json": '{"series":[]}',
                "custom_mapping_json": '{"points":{"from":"rows"}}',
            },
        },
        "sql": "SELECT 1 AS value",
        "retry_count": 2,
        "columns": ["value"],
        "named_queries": [{"name": "good", "sql": "SELECT 2 AS n", "purpose": "ok"}],
        "named_query_results": [
            {"name": "good", "sql": "SELECT 2 AS n", "purpose": "ok", "error": ""},
            {"name": "bad", "sql": "SELECT nope", "purpose": "broken", "error": "boom"},
        ],
        "chart_error": "",
    }
