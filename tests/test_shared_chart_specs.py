import json
import re

import pytest

from shared.chart_specs import (
    _apply_chart_spec_visual_overrides,
    _build_raw_chart_spec,
    _coerce_positive_int,
    _compile_builder_sql,
    _compile_chart_spec,
    _default_chart_spec,
    _normalize_chart_spec,
    _parse_bool,
    _resolve_template_role_indices,
    _sql_literal,
    _validate_chart_query,
)

QUERY_DENY_PATTERN = re.compile(
    r"\b(INSERT|UPDATE|DELETE|DROP|ALTER|CREATE|TRUNCATE|REPLACE|RENAME|ATTACH|DETACH|GRANT|REVOKE)\b",
    re.IGNORECASE,
)


def _chart_templates():
    return {
        "derived_signal_overlay": {},
        "anomaly_overlay": {},
        "dual_axis_anomaly": {},
        "time_series_percentiles": {},
        "heatmap": {},
        "box_plot": {},
        "gauge_kpi": {},
        "custom_echarts": {},
    }


@pytest.mark.parametrize(
    ("query", "expected"),
    [
        ("", "Query cannot be empty"),
        ("  update widgets set value=1", "Only SELECT queries are allowed"),
        ("SELECT * FROM widgets; DROP TABLE widgets", "Query contains a disallowed keyword"),
        ("WITH sample AS (SELECT 1) SELECT * FROM sample", None),
    ],
)
def test_validate_chart_query_covers_empty_non_select_deny_and_success_paths(query, expected):
    assert _validate_chart_query(query, query_deny_pattern=QUERY_DENY_PATTERN) == expected


def test_sql_literal_and_coerce_positive_int_handle_escape_default_and_clamp_paths():
    assert _sql_literal("O'Reilly") == "'O''Reilly'"
    assert _coerce_positive_int("bad", 5, 1, 10) == 5
    assert _coerce_positive_int("0", 5, 1, 10) == 1
    assert _coerce_positive_int("99", 5, 1, 10) == 10


def test_default_chart_spec_returns_builder_and_custom_echarts_shapes():
    builder = _default_chart_spec()
    assert builder["template_id"] == "derived_signal_overlay"
    assert builder["sql"] == {"mode": "builder", "override_sql": ""}

    custom = _default_chart_spec("custom_echarts")
    assert custom["template_id"] == "custom_echarts"
    assert custom["sql"]["mode"] == "raw"
    assert "custom_option_json" in custom["visual"]


def test_build_raw_chart_spec_prefers_existing_chart_spec_and_falls_back_for_bad_options():
    preserved = _build_raw_chart_spec(
        "heatmap",
        "SELECT 1",
        options_json=json.dumps({"chart_spec": {"template_id": "box_plot", "sql": {"mode": "builder"}}}),
        chart_templates=_chart_templates(),
    )
    assert preserved["template_id"] == "box_plot"
    assert preserved["sql"]["mode"] == "builder"

    fallback = _build_raw_chart_spec(
        "heatmap",
        "SELECT 1",
        options_json='{"chart_spec": {"template_id": "missing"}}',
        chart_templates=_chart_templates(),
    )
    assert fallback["template_id"] == "heatmap"
    assert fallback["sql"] == {"mode": "raw", "override_sql": "SELECT 1"}


def test_normalize_chart_spec_merges_defaults_role_map_and_named_queries():
    normalized = _normalize_chart_spec(
        {
            "template_id": "custom_echarts",
            "sql": {"mode": "raw", "override_sql": "SELECT 1"},
            "data": {"service": "checkout", "limit": 25},
            "visual": {"role_map": {" time ": " ts ", "": "ignored", "value": ""}, "legend_show": False},
            "named_queries": [
                {"name": " detail_rows ", "sql": "SELECT 2;", "purpose": "extra"},
                {"name": "1bad", "sql": "SELECT 3"},
                {"name": "missing_sql", "sql": "  "},
                "skip-me",
            ],
        },
        chart_templates=_chart_templates(),
    )

    assert normalized["template_id"] == "custom_echarts"
    assert normalized["data"]["service"] == "checkout"
    assert normalized["data"]["limit"] == 25
    assert normalized["visual"]["role_map"] == {"time": "ts"}
    assert normalized["visual"]["legend_show"] is False
    assert normalized["named_queries"] == [{"name": "detail_rows", "sql": "SELECT 2", "purpose": "extra"}]


@pytest.mark.parametrize(
    "spec_raw, error_text",
    [
        ({"template_id": "missing"}, "Unknown template"),
        ({"template_id": "heatmap", "sql": {"mode": "sideways"}}, "sql.mode must be 'builder' or 'raw'"),
    ],
)
def test_normalize_chart_spec_rejects_invalid_templates_and_sql_modes(spec_raw, error_text):
    with pytest.raises(ValueError, match=error_text):
        _normalize_chart_spec(spec_raw, chart_templates=_chart_templates())


@pytest.mark.parametrize(
    ("source_view", "expected_fragment"),
    [
        ("v_derived_signals_anomaly", "FROM v_derived_signals_anomaly"),
        ("v_otel_metrics_anomaly", "FROM v_otel_metrics_anomaly"),
        ("otel_metrics_histogram", "avg(toFloat64(if(Count = 0, 0.0, Sum / toFloat64(Count)))) AS value"),
        ("otel_logs", "FROM otel_logs"),
        ("otel_traces", "FROM otel_traces"),
        ("sobs_error_resolutions", "FROM sobs_error_resolutions"),
    ],
)
def test_compile_builder_sql_supports_each_source_branch(source_view, expected_fragment):
    query = _compile_builder_sql(
        "derived_signal_overlay",
        {
            "source_view": source_view,
            "service": "checkout",
            "signal_source": "traces",
            "signal_name": "trace_volume",
            "metric_name": "request_duration",
            "attr_fp": "fingerprint-1",
            "window_hours": 4,
            "limit": 77,
        },
    )

    assert expected_fragment in query
    assert "LIMIT 77" in query


@pytest.mark.parametrize(
    ("template_id", "expected_fragment"),
    [
        ("anomaly_overlay", "anomaly_state"),
        ("dual_axis_anomaly", "value AS metric"),
        ("time_series_percentiles", "baseline_upper AS p95"),
        ("heatmap", "AS x_category"),
        ("box_plot", "quantile(0.5)(value) AS median"),
        ("gauge_kpi", "avg(if(anomaly_state = 'normal', 1.0, 0.0))"),
    ],
)
def test_compile_builder_sql_supports_each_template_branch(template_id, expected_fragment):
    query = _compile_builder_sql(template_id, {"source_view": "otel_logs", "service": "checkout", "limit": 12})
    assert expected_fragment in query


def test_compile_builder_sql_uses_metric_defaults_when_signal_labels_are_missing():
    query = _compile_builder_sql("derived_signal_overlay", {"source_view": "otel_metrics_gauge", "limit": 9})
    assert "'metrics' AS source" in query
    assert "'value' AS signal" in query


@pytest.mark.parametrize(
    ("template_id", "data", "error_text"),
    [
        ("custom_echarts", {"source_view": "otel_logs"}, "custom_echarts requires sql.mode='raw'"),
        ("derived_signal_overlay", {"source_view": "not_real"}, "Unsupported source for builder mode"),
        ("not_real", {"source_view": "otel_logs"}, "Builder mode does not support template"),
    ],
)
def test_compile_builder_sql_rejects_unsupported_modes_sources_and_templates(template_id, data, error_text):
    with pytest.raises(ValueError, match=error_text):
        _compile_builder_sql(template_id, data)


def test_compile_chart_spec_supports_builder_and_raw_modes_and_named_query_validation():
    template_id, query, normalized = _compile_chart_spec(
        {
            "template_id": "heatmap",
            "data": {"source_view": "otel_logs", "service": "checkout", "limit": 33},
            "named_queries": [{"name": "detail", "sql": "SELECT 2;", "purpose": "details"}],
        },
        chart_templates=_chart_templates(),
        query_deny_pattern=QUERY_DENY_PATTERN,
    )
    assert template_id == "heatmap"
    assert "LIMIT 33" in query
    assert normalized["named_queries"] == [{"name": "detail", "sql": "SELECT 2", "purpose": "details"}]

    raw_template_id, raw_query, raw_spec = _compile_chart_spec(
        {
            "template_id": "custom_echarts",
            "sql": {"mode": "raw", "override_sql": "SELECT 1 AS value"},
        },
        chart_templates=_chart_templates(),
        query_deny_pattern=QUERY_DENY_PATTERN,
    )
    assert raw_template_id == "custom_echarts"
    assert raw_query == "SELECT 1 AS value"
    assert raw_spec["sql"]["mode"] == "raw"


def test_compile_chart_spec_defaults_missing_sql_mode_to_builder():
    template_id, query, normalized = _compile_chart_spec(
        {"template_id": "heatmap", "sql": {}, "data": {"source_view": "otel_logs", "limit": 7}},
        chart_templates=_chart_templates(),
        query_deny_pattern=QUERY_DENY_PATTERN,
    )
    assert template_id == "heatmap"
    assert "LIMIT 7" in query
    assert normalized["sql"]["mode"] == "builder"


@pytest.mark.parametrize(
    "spec_raw",
    [
        {"template_id": "custom_echarts", "sql": {"mode": "builder"}},
        {"template_id": "heatmap", "sql": {"mode": "raw", "override_sql": "DELETE FROM x"}},
        {"template_id": "heatmap", "named_queries": [{"name": "detail", "sql": "DROP TABLE x"}]},
    ],
)
def test_compile_chart_spec_rejects_invalid_builder_raw_and_named_queries(spec_raw):
    with pytest.raises(ValueError):
        _compile_chart_spec(spec_raw, chart_templates=_chart_templates(), query_deny_pattern=QUERY_DENY_PATTERN)


def test_resolve_template_role_indices_supports_defaults_none_case_insensitive_overrides_and_errors():
    template = {"column_roles": {"time": 0, "value": 1}}
    columns = ["Time", "Value", "Anomaly"]

    assert _resolve_template_role_indices("heatmap", template, columns, None) == {"time": 0, "value": 1}

    remapped = _resolve_template_role_indices(
        "heatmap",
        template,
        columns,
        {"visual": {"role_map": {"value": "anomaly", "time": "time"}}},
    )
    assert remapped == {"time": 0, "value": 2}

    with pytest.raises(ValueError, match="Unknown role 'missing'"):
        _resolve_template_role_indices(
            "heatmap",
            template,
            columns,
            {"visual": {"role_map": {"missing": "Value"}}},
        )

    with pytest.raises(ValueError, match="maps to unknown column 'missing_column'"):
        _resolve_template_role_indices(
            "heatmap",
            template,
            columns,
            {"visual": {"role_map": {"value": "missing_column"}}},
        )

    assert _resolve_template_role_indices("heatmap", template, columns, {"visual": {"role_map": []}}) == {
        "time": 0,
        "value": 1,
    }


@pytest.mark.parametrize(
    ("value", "default", "expected"),
    [
        (True, False, True),
        (None, True, True),
        ("yes", False, True),
        ("off", True, False),
        ("unknown", False, False),
    ],
)
def test_parse_bool_handles_boolean_none_and_string_forms(value, default, expected):
    assert _parse_bool(value, default) is expected


def test_apply_chart_spec_visual_overrides_updates_legend_zoom_and_value_series_styles():
    option = {
        "legend": {"show": True},
        "dataZoom": [{"type": "inside", "start": 10, "end": 20}],
        "series": [
            {"name": "Value", "type": "line", "lineStyle": {"width": 2}, "itemStyle": {"opacity": 0.7}},
            {"name": "Other", "type": "bar"},
        ],
    }
    updated = _apply_chart_spec_visual_overrides(
        "heatmap",
        option,
        {
            "visual": {
                "legend_show": "0",
                "zoom_inside": "true",
                "zoom_slider": "true",
                "zoom_start_pct": 80,
                "zoom_end_pct": 10,
                "smooth_line": "false",
                "value_color": "#123456",
            }
        },
    )

    assert updated["legend"]["show"] is False
    assert len(updated["dataZoom"]) == 2
    assert updated["dataZoom"][0]["end"] == 80
    assert updated["dataZoom"][1]["type"] == "slider"
    assert updated["series"][0]["smooth"] is False
    assert updated["series"][0]["lineStyle"] == {"width": 2, "color": "#123456"}
    assert updated["series"][0]["itemStyle"] == {"opacity": 0.7, "color": "#123456"}
    assert "smooth" not in updated["series"][1]

    passthrough = {"series": [{"name": "Value", "type": "line"}]}
    assert _apply_chart_spec_visual_overrides("custom_echarts", passthrough, {"visual": {}}) is passthrough


def test_apply_chart_spec_visual_overrides_preserves_existing_zoom_when_disabled():
    option = {"dataZoom": [{"type": "inside", "start": 5, "end": 25}], "series": [{"name": "Value", "type": "bar"}]}
    updated = _apply_chart_spec_visual_overrides(
        "heatmap",
        option,
        {"visual": {"zoom_inside": "false", "zoom_slider": "false"}},
    )
    assert updated["dataZoom"] == [{"type": "inside", "start": 5, "end": 25}]
    assert "smooth" not in updated["series"][0]
