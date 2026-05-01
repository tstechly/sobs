import pytest

from shared.dashboard_api import (
    SUPPORTED_CHART_OPTION_SOURCES,
    _apply_query_limit,
    _build_chart_spec_options,
    _build_chart_spec_template_api_payload,
    _build_named_datasets,
    _execute_chart_spec_named_queries,
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
