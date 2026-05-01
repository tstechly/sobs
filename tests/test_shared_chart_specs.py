import json
import re
from datetime import datetime, timezone

import pytest

from shared.chart_specs import (
    _apply_chart_spec_visual_overrides,
    _attach_drilldown_metadata,
    _build_custom_drilldown,
    _build_raw_chart_spec,
    _coerce_positive_int,
    _compile_builder_sql,
    _compile_chart_spec,
    _deep_substitute,
    _default_chart_spec,
    _extract_bindings,
    _format_drilldown_time,
    _infer_column_types,
    _normalize_chart_spec,
    _normalize_custom_series_point_order,
    _parse_bool,
    _parse_custom_json_config,
    _prepare_template_rows,
    _public_dashboard_query_error,
    _render_chart_from_template,
    _render_custom_echarts,
    _resolve_custom_binding_expr,
    _resolve_template_role_indices,
    _resolve_template_string,
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


def test_infer_column_types_and_public_dashboard_query_error_cover_null_strip_hint_and_truncation_paths():
    assert _infer_column_types(["time", "value", "label"], [[None, 2], [None, None, "ok"]]) == ["null", "int", "str"]

    hinted = _public_dashboard_query_error(Exception("Code: 53. DB::Exception: TYPE_MISMATCH: incompatible types"))
    assert "Check casts and column types." in hinted

    long_error = "Code: 53. DB::Exception: TYPE_MISMATCH: " + ("x" * 320) + ". Stack trace: ignored"
    rendered = _public_dashboard_query_error(Exception(long_error))
    assert rendered.endswith("...")
    assert len(rendered) == 280

    assert _public_dashboard_query_error(Exception("   ")) == "Query execution failed"


def test_deep_substitute_replaces_nested_placeholders_and_leaves_none_bindings_literal():
    substituted = _deep_substitute(
        {
            "series": "{{points}}",
            "nested": [{"count": "{{count}}"}, "{{missing}}", "literal"],
        },
        {"points": [[1, 2]], "count": 3, "missing": None},
    )
    assert substituted == {
        "series": [[1, 2]],
        "nested": [{"count": 3}, "{{missing}}", "literal"],
    }


def test_extract_bindings_builds_heatmap_boxplot_and_gauge_helpers():
    heatmap_bindings = _extract_bindings(
        {"column_roles": {"x_category": 0, "y_category": 1, "value": 2, "effective_state": 3}},
        ["service", "bucket", "value", "state"],
        [
            ["svc-b", "2024-01-02", 5, "warning"],
            ["svc-a", "2024-01-01", 2, "outlier"],
            ["svc-a", "2024-01-02", 4, "normal"],
        ],
    )
    assert heatmap_bindings["x_unique_values"] == ["svc-a", "svc-b"]
    assert heatmap_bindings["y_unique_values"] == ["2024-01-01", "2024-01-02"]
    assert heatmap_bindings["heatmap_data"] == [[0, 0, 2], [0, 1, 4], [1, 1, 5]]
    assert heatmap_bindings["value_min"] == 2
    assert heatmap_bindings["value_max"] == 5
    assert heatmap_bindings["value_first"] == 5
    assert heatmap_bindings["anomaly_point_color"] == ["#ffc107", "#dc3545", "#0d6efd"]
    assert heatmap_bindings["anomaly_symbol_size"] == [7, 10, 4]

    boxplot_bindings = _extract_bindings(
        {"column_roles": {"dimension": 0, "min": 1, "q1": 2, "median": 3, "q3": 4, "max": 5}},
        ["dimension", "min", "q1", "median", "q3", "max"],
        [["latency", 1, 2, 3, 4, 5]],
    )
    assert boxplot_bindings["boxplot_data"] == [[1, 2, 3, 4, 5]]
    assert boxplot_bindings["dimension_values"] == ["latency"]


def test_extract_bindings_handles_derived_signal_overlay_delta_and_ratio_modes():
    overlay_template = {
        "id": "derived_signal_overlay",
        "column_roles": {
            "time": 0,
            "signal": 1,
            "value": 2,
            "baseline_mean": 3,
            "baseline_lower": 4,
            "baseline_upper": 5,
            "anomaly_state": 6,
        },
    }

    delta_bindings = _extract_bindings(
        overlay_template,
        ["time", "signal", "value", "baseline_mean", "baseline_lower", "baseline_upper", "anomaly_state"],
        [
            ["2024-01-01T00:00:00Z", "trace_volume", 120.0, 100.0, 90.0, 110.0, "warning"],
            ["2024-01-01T00:01:00Z", "trace_volume", 130.0, 100.0, 95.0, 115.0, "outlier"],
        ],
    )
    assert delta_bindings["y_axis_name"] == "Delta %"
    assert delta_bindings["value_axis_min"] < 0
    assert delta_bindings["value_axis_max"] > 0
    assert delta_bindings["value_points"] == [
        ["2024-01-01T00:00:00Z", 20.0, 1],
        ["2024-01-01T00:01:00Z", 30.0, 2],
    ]
    assert len(delta_bindings["anomaly_mark_areas"]) == 2
    assert delta_bindings["warning_points"] == [["2024-01-01T00:00:00Z", 20.0]]
    assert delta_bindings["outlier_points"] == [["2024-01-01T00:01:00Z", 30.0]]
    assert "warn 1 | outlier 1" in delta_bindings["signal_summary"]

    ratio_bindings = _extract_bindings(
        overlay_template,
        ["time", "signal", "value", "baseline_mean", "baseline_lower", "baseline_upper", "anomaly_state"],
        [["2024-01-01T00:00:00Z", "trace_error_ratio", 0.33, 0.2, 0.1, 0.4, "normal"]],
    )
    assert ratio_bindings["y_axis_name"] == "Value"
    assert ratio_bindings["value_axis_min"] == 0
    assert ratio_bindings["value_axis_max"] == 1


def test_format_drilldown_time_handles_datetime_iso_clickhouse_and_invalid_inputs():
    assert (
        _format_drilldown_time(
            datetime(2024, 1, 1, 12, 0, tzinfo=timezone.utc),
            normalize_ch_timestamp=lambda value: value,
        )
        == "2024-01-01T12:00:00Z"
    )
    assert (
        _format_drilldown_time("2024-01-01T12:00:00Z", normalize_ch_timestamp=lambda value: value)
        == "2024-01-01T12:00:00Z"
    )
    assert (
        _format_drilldown_time(
            "2024-01-01 12:00:00",
            normalize_ch_timestamp=lambda value: value.replace(" ", "T"),
        )
        == "2024-01-01T12:00:00Z"
    )
    assert _format_drilldown_time("not-a-timestamp", normalize_ch_timestamp=lambda value: value) == "not-a-timestamp"
    assert _format_drilldown_time("", normalize_ch_timestamp=lambda value: value) == ""


def test_attach_drilldown_metadata_handles_time_series_and_heatmap_templates():
    option = {"series": [{"name": "Value", "data": [10, 20]}, {"name": "Baseline", "data": [8, 9]}]}
    enriched = _attach_drilldown_metadata(
        {"id": "derived_signal_overlay", "drilldown": {"bucket_seconds": 60}},
        {
            "time": ["2024-01-01T00:00:00Z", "2024-01-01T00:01:00Z"],
            "anomaly_state": ["warning", "outlier"],
            "anomaly_score": [1.2, 3.4],
            "rule_state": ["warning", "outlier"],
            "rule_name": ["rule-a", "rule-b"],
            "rule_reason": ["high", "higher"],
            "effective_state": ["warning", "outlier"],
            "service": ["checkout", "checkout"],
            "source": ["traces", "traces"],
            "signal": ["trace_volume", "trace_volume"],
            "attr_fp": ["", ""],
        },
        option,
        format_drilldown_time=lambda value: f"fmt:{value}",
    )
    assert enriched["series"][0]["data"][0]["drilldown"]["from_ts"] == "fmt:2024-01-01T00:00:00Z"
    assert enriched["series"][0]["data"][1]["drilldown"]["_anomaly_score"] == 3.4
    assert enriched["series"][0]["data"][1]["drilldown"]["service"] == "checkout"
    assert enriched["series"][1]["data"][0]["drilldown"] == {"from_ts": "fmt:2024-01-01T00:00:00Z", "window_s": 60}

    heatmap_option = {"series": [{"data": [[0, 0, 10], [1, 0, 20], "skip"]}]}
    heatmap = _attach_drilldown_metadata(
        {"id": "heatmap", "drilldown": {"bucket_seconds": 300}},
        {"x_unique_values": ["checkout", "payments"], "y_unique_values": ["2024-01-01T00:05:00Z"]},
        heatmap_option,
        format_drilldown_time=lambda value: f"fmt:{value}",
    )
    assert heatmap["series"][0]["data"][0]["drilldown"] == {
        "from_ts": "fmt:2024-01-01T00:05:00Z",
        "window_s": 300,
        "service": "checkout",
    }
    assert heatmap["series"][0]["data"][2] == "skip"


def test_parse_custom_json_config_and_binding_expr_handle_json_sources_and_errors():
    assert _parse_custom_json_config({"a": 1}, "field") == {"a": 1}
    assert _parse_custom_json_config(None, "field") == {}
    assert _parse_custom_json_config("", "field") == {}
    assert _parse_custom_json_config("[1, 2]", "field") == [1, 2]

    with pytest.raises(ValueError, match="field must be valid JSON"):
        _parse_custom_json_config("{bad", "field")

    records = [{"svc": "checkout", "value": 10}, {"svc": "payments", "value": 20}]
    rows = [["checkout", 10], ["payments", 20]]
    columns = ["svc", "value"]

    assert _resolve_custom_binding_expr("", columns, records, rows) is None
    assert _resolve_custom_binding_expr("columns", columns, records, rows) == columns
    assert _resolve_custom_binding_expr("svc", columns, records, rows) == ["checkout", "payments"]
    assert _resolve_custom_binding_expr({"from": "rows"}, columns, records, rows) == rows
    assert _resolve_custom_binding_expr({"from": "records"}, columns, records, rows) == records
    assert _resolve_custom_binding_expr({"from": "literal", "value": 7}, columns, records, rows) == 7
    assert _resolve_custom_binding_expr({"from": "column", "name": "value"}, columns, records, rows) == [10, 20]

    with pytest.raises(ValueError, match="strings or objects"):
        _resolve_custom_binding_expr(7, columns, records, rows)

    with pytest.raises(ValueError, match="non-empty 'name'"):
        _resolve_custom_binding_expr({"from": "column", "name": ""}, columns, records, rows)

    with pytest.raises(ValueError, match="Unsupported custom mapping mode"):
        _resolve_custom_binding_expr({"from": "nope"}, columns, records, rows)


def test_template_string_drilldown_and_custom_point_order_cover_rendering_helpers():
    assert _resolve_template_string("svc={{ svc }};missing={{missing}}", {"svc": "checkout"}) == "svc=checkout;missing="
    assert _build_custom_drilldown({}, []) is None

    drilldown = _build_custom_drilldown(
        {
            "_drilldown": {
                "target": "logs",
                "label": "Open logs",
                "bucket_seconds": 60,
                "time_axis": "ts",
                "service_axis": "service",
                "extra": {"service": "{{service}}", "from_ts": "{{ts}}", "fixed": 3},
            }
        },
        [{"service": "checkout", "ts": "2024-01-01T00:00:00Z"}],
    )
    assert drilldown == {
        "target": "logs",
        "label": "Open logs",
        "bucket_seconds": 60,
        "time_axis": "ts",
        "service_axis": "service",
        "extra": {"service": "checkout", "from_ts": "2024-01-01T00:00:00Z", "fixed": 3},
    }
    assert _build_custom_drilldown({"_drilldown": {"target": "bad"}}, []) is None

    option = {
        "series": [
            {
                "data": [
                    ["2024-01-01T00:02:00Z", 2],
                    ["2024-01-01T00:01:00Z", 1],
                    [1, 4],
                    ["non-iso", 3],
                ]
            },
            {"data": [["only-one"]]},
            {"data": ["skip-me", ["2024-01-01T00:00:00Z", 0]]},
        ]
    }
    _normalize_custom_series_point_order(option)
    assert option["series"][0]["data"] == [
        ["2024-01-01T00:01:00Z", 1],
        ["2024-01-01T00:02:00Z", 2],
        [1, 4],
        ["non-iso", 3],
    ]


def test_normalize_custom_series_point_order_handles_missing_series_and_mixed_non_series_entries():
    option = {"series": ["skip", {"data": [[2, "b"], [1, "a"]]}]}
    _normalize_custom_series_point_order(option)
    assert option["series"][1]["data"] == [[1, "a"], [2, "b"]]

    empty = {}
    _normalize_custom_series_point_order(empty)
    assert empty == {}


def test_render_custom_echarts_supports_named_datasets_defaults_and_custom_drilldown():
    option = _render_custom_echarts(
        {
            "echarts_option_template": {
                "title": {"text": "{{columns:details}}"},
                "series": [{"type": "line", "data": "{{points}}"}],
            }
        },
        ["ts", "service", "value"],
        [["2024-01-01T00:02:00Z", "checkout", 2], ["2024-01-01T00:01:00Z", "checkout", 1]],
        {
            "visual": {
                "custom_mapping_json": json.dumps(
                    {
                        "points": {"from": "rows"},
                        "_drilldown": {
                            "target": "logs",
                            "extra": {"service": "{{service}}", "from_ts": "{{ts}}"},
                        },
                    }
                ),
                "custom_option_json": json.dumps(
                    {
                        "title": {"text": "{{columns:details}}"},
                        "series": [{"type": "line", "data": "{{points}}"}],
                    }
                ),
            }
        },
        named_datasets={"details": {"columns": ["ts", "value"], "rows": [[1, 2]], "records": [{"ts": 1}]}},
    )

    assert option["backgroundColor"] == "transparent"
    assert option["textStyle"] == {"color": "#adb5bd"}
    assert option["title"]["text"] == ["ts", "value"]
    assert option["series"][0]["data"] == [
        ["2024-01-01T00:01:00Z", "checkout", 1],
        ["2024-01-01T00:02:00Z", "checkout", 2],
    ]
    assert option["_customDrilldown"] == {
        "target": "logs",
        "label": "Open Source View",
        "extra": {"service": "checkout", "from_ts": "2024-01-01T00:02:00Z"},
    }

    with pytest.raises(ValueError, match="visual.custom_mapping_json must be a JSON object"):
        _render_custom_echarts(
            {"echarts_option_template": {}},
            ["value"],
            [[1]],
            {"visual": {"custom_mapping_json": "[]", "custom_option_json": "{}"}},
        )


def test_render_custom_echarts_handles_template_fallback_dict_rows_and_invalid_custom_option_shapes():
    option = _render_custom_echarts(
        {
            "echarts_option_template": {
                "series": [{"type": "line", "data": "{{rows}}"}],
            }
        },
        ["ts", "value"],
        [{"ts": "2024-01-01T00:00:00Z", "value": 1}],
        {"visual": {"custom_mapping_json": json.dumps({}), "custom_option_json": "  "}},
        named_datasets={"skip": []},
    )
    assert option["series"][0]["data"] == [["2024-01-01T00:00:00Z", 1]]

    with pytest.raises(ValueError, match="visual.custom_option_json must be a JSON object"):
        _render_custom_echarts(
            {"echarts_option_template": {}},
            ["value"],
            [[1]],
            {"visual": {"custom_mapping_json": "{}", "custom_option_json": "[]"}},
        )


def test_prepare_template_rows_handles_passthrough_short_rows_and_derived_signal_overlay_mapping():
    passthrough_columns, passthrough_rows = _prepare_template_rows(
        "heatmap",
        ["time", "value"],
        [{"time": "2024-01-01T00:00:00Z", "value": 1}],
        annotate_rows_with_rules=lambda *args, **kwargs: None,
        anomaly_rules=[],
    )
    assert passthrough_columns == ["time", "value"]
    assert passthrough_rows == [{"time": "2024-01-01T00:00:00Z", "value": 1}]

    short_columns, short_rows = _prepare_template_rows(
        "derived_signal_overlay",
        ["time", "value"],
        [{"time": "2024-01-01T00:00:00Z", "value": 1}],
        annotate_rows_with_rules=lambda *args, **kwargs: None,
        anomaly_rules=[],
    )
    assert short_columns == ["time", "value"]
    assert short_rows == [{"time": "2024-01-01T00:00:00Z", "value": 1}]

    def _fake_annotate(rows, rules, **kwargs):
        assert rules == [{"name": "rule-a"}]
        assert kwargs["source_key"] == "source"
        rows[0]["rule_state"] = "warning"
        rows[0]["rule_name"] = "rule-a"
        rows[0]["rule_reason"] = "high"
        rows[0]["effective_state"] = "warning"

    prepared_columns, prepared_rows = _prepare_template_rows(
        "derived_signal_overlay",
        [
            "metric",
            "svc",
            "src",
            "sig",
            "fingerprint",
            "val",
            "samples",
            "mean",
            "low",
            "high",
            "state",
            "score",
        ],
        [
            {
                "metric": "2024-01-01T00:00:00Z",
                "svc": "checkout",
                "src": "traces",
                "sig": "trace_volume",
                "fingerprint": "fp-1",
                "val": 12.0,
                "samples": 4,
                "mean": 10.0,
                "low": 8.0,
                "high": 14.0,
                "state": "warning",
                "score": 2.4,
            }
        ],
        {
            "time": 0,
            "service": 1,
            "source": 2,
            "signal": 3,
            "attr_fp": 4,
            "value": 5,
            "sample_count": 6,
            "baseline_mean": 7,
            "baseline_lower": 8,
            "baseline_upper": 9,
            "anomaly_state": 10,
            "anomaly_score": 11,
        },
        annotate_rows_with_rules=_fake_annotate,
        anomaly_rules=[{"name": "rule-a"}],
    )

    assert prepared_columns[-4:] == ["rule_state", "rule_name", "rule_reason", "effective_state"]
    assert prepared_rows == [
        {
            "time": "2024-01-01T00:00:00Z",
            "service": "checkout",
            "source": "traces",
            "signal": "trace_volume",
            "attr_fp": "fp-1",
            "value": 12.0,
            "sample_count": 4,
            "baseline_mean": 10.0,
            "baseline_lower": 8.0,
            "baseline_upper": 14.0,
            "anomaly_state": "warning",
            "anomaly_score": 2.4,
            "rule_state": "warning",
            "rule_name": "rule-a",
            "rule_reason": "high",
            "effective_state": "warning",
        }
    ]


def test_render_chart_from_template_handles_unknown_empty_custom_and_validation_paths():
    with pytest.raises(ValueError, match="Unknown template"):
        _render_chart_from_template(
            "missing",
            ["time"],
            [[1]],
            chart_templates={},
            resolve_template_role_indices=lambda *args, **kwargs: {},
            prepare_template_rows=lambda *args, **kwargs: args[1:3],
            extract_bindings=lambda *args, **kwargs: {},
            deep_substitute=lambda *args, **kwargs: {},
            attach_drilldown_metadata=lambda *args, **kwargs: {},
            render_custom_echarts=lambda *args, **kwargs: {},
        )

    empty = _render_chart_from_template(
        "heatmap",
        ["time", "value"],
        [],
        chart_templates={"heatmap": {"echarts_option_template": {}, "min_columns": 1}},
        resolve_template_role_indices=lambda *args, **kwargs: {},
        prepare_template_rows=lambda *args, **kwargs: args[1:3],
        extract_bindings=lambda *args, **kwargs: {},
        deep_substitute=lambda *args, **kwargs: {},
        attach_drilldown_metadata=lambda *args, **kwargs: {},
        render_custom_echarts=lambda *args, **kwargs: {},
    )
    assert empty["title"]["text"] == "No data for selected query/time window"

    custom = _render_chart_from_template(
        "custom_echarts",
        ["time"],
        [[1]],
        chart_templates={"custom_echarts": {"echarts_option_template": {}}},
        resolve_template_role_indices=lambda *args, **kwargs: {},
        prepare_template_rows=lambda *args, **kwargs: args[1:3],
        extract_bindings=lambda *args, **kwargs: {},
        deep_substitute=lambda *args, **kwargs: {},
        attach_drilldown_metadata=lambda *args, **kwargs: {},
        render_custom_echarts=lambda template, columns, rows, spec, named_datasets=None: {"ok": True, "rows": rows},
    )
    assert custom == {"ok": True, "rows": [[1]]}

    with pytest.raises(ValueError, match="requires at least 2 columns"):
        _render_chart_from_template(
            "heatmap",
            ["time"],
            [[1]],
            chart_templates={"heatmap": {"echarts_option_template": {}, "min_columns": 2}},
            resolve_template_role_indices=lambda *args, **kwargs: {},
            prepare_template_rows=lambda *args, **kwargs: args[1:3],
            extract_bindings=lambda *args, **kwargs: {},
            deep_substitute=lambda *args, **kwargs: {},
            attach_drilldown_metadata=lambda *args, **kwargs: {},
            render_custom_echarts=lambda *args, **kwargs: {},
        )

    with pytest.raises(ValueError, match="accepts maximum 1 columns"):
        _render_chart_from_template(
            "heatmap",
            ["time", "value"],
            [[1, 2]],
            chart_templates={"heatmap": {"echarts_option_template": {}, "min_columns": 1, "max_columns": 1}},
            resolve_template_role_indices=lambda *args, **kwargs: {},
            prepare_template_rows=lambda *args, **kwargs: args[1:3],
            extract_bindings=lambda *args, **kwargs: {},
            deep_substitute=lambda *args, **kwargs: {},
            attach_drilldown_metadata=lambda *args, **kwargs: {},
            render_custom_echarts=lambda *args, **kwargs: {},
        )


def test_render_chart_from_template_prepares_dict_rows_and_applies_defaults():
    calls: dict[str, object] = {}

    def _fake_prepare(template_id, columns, rows, role_indices):
        calls["prepared"] = (template_id, columns, rows, role_indices)
        return ["time", "value"], [["2024-01-01T00:00:00Z", 1]]

    def _fake_extract(template, columns, rows, role_indices):
        calls["bindings"] = (template, columns, rows, role_indices)
        return {"points": rows}

    rendered = _render_chart_from_template(
        "heatmap",
        ["svc", "val"],
        [{"svc": "checkout", "val": 1}],
        chart_templates={"heatmap": {"echarts_option_template": {"series": "{{points}}"}, "min_columns": 1}},
        resolve_template_role_indices=lambda *args, **kwargs: {"time": 0},
        prepare_template_rows=_fake_prepare,
        extract_bindings=_fake_extract,
        deep_substitute=lambda template, bindings: {"series": bindings["points"]},
        attach_drilldown_metadata=lambda template, bindings, option: {**option, "drilldown": True},
        render_custom_echarts=lambda *args, **kwargs: {},
    )

    assert calls["prepared"][0] == "heatmap"
    assert calls["bindings"][1] == ["time", "value"]
    assert rendered == {
        "series": [["2024-01-01T00:00:00Z", 1]],
        "drilldown": True,
        "backgroundColor": "transparent",
        "textStyle": {"color": "#adb5bd"},
    }
