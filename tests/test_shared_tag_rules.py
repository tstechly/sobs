from shared.tag_rules import (
    _load_tag_rules,
    _match_single_condition,
    _match_tag_rule,
    _parse_tag_rule_conditions_json,
    _record_id_for_log,
    _record_id_for_span,
    _tag_rule_attribute_key_suggestions,
)


class _FakeResult:
    def __init__(self, rows=None):
        self._rows = rows or []

    def fetchall(self):
        return self._rows


class _FakeDb:
    def __init__(self, rows=None):
        self.rows = rows or []
        self.calls = []

    def execute(self, query, params=None):
        self.calls.append((query, params))
        return _FakeResult(self.rows)


def test_shared_tag_rules_record_ids_are_stable_and_sensitive_to_input_fields():
    log_id_one = _record_id_for_log("2026-01-01T00:00:00", "svc", "traceid", "spanid")
    log_id_two = _record_id_for_log("2026-01-01T00:00:00", "svc", "traceid", "spanid")
    log_id_three = _record_id_for_log("2026-01-01T00:00:00", "svc-b", "traceid", "spanid")
    span_id_one = _record_id_for_span("traceid", "spanid")
    span_id_two = _record_id_for_span("traceid", "spanid")

    assert log_id_one == log_id_two
    assert log_id_one != log_id_three
    assert len(log_id_one) == 32
    assert span_id_one == span_id_two
    assert len(span_id_one) == 32


def test_shared_tag_rules_parse_conditions_handles_invalid_and_partial_payloads():
    assert _parse_tag_rule_conditions_json("") == []
    assert _parse_tag_rule_conditions_json("{bad-json") == []
    assert _parse_tag_rule_conditions_json('{"match_field":"severity"}') == []
    assert _parse_tag_rule_conditions_json(
        '[null, {"match_field":"severity","match_operator":"eq","match_value":"ERROR"}]'
    ) == [
        {
            "match_field": "severity",
            "match_operator": "eq",
            "match_value": "ERROR",
            "match_attr_key": "",
        }
    ]


def test_shared_tag_rules_load_rules_supports_conditions_and_legacy_fallback():
    db = _FakeDb(
        rows=[
            {
                "Id": "legacy-1",
                "Name": "legacy",
                "RecordTypes": "log, trace",
                "MatchField": "service_name",
                "MatchOperator": "contains",
                "MatchValue": "checkout",
                "MatchAttrKey": "",
                "TagKey": "team",
                "TagValue": "payments",
                "ConditionsJson": "",
            },
            {
                "Id": "composite-1",
                "Name": "composite",
                "RecordTypes": "all",
                "MatchField": "severity",
                "MatchOperator": "eq",
                "MatchValue": "WARN",
                "MatchAttrKey": "",
                "TagKey": "env",
                "TagValue": "prod",
                "ConditionsJson": '[{"match_field":"severity","match_operator":"eq","match_value":"ERROR"}]',
            },
        ]
    )

    loaded = _load_tag_rules(db, parse_tag_rule_conditions_json=_parse_tag_rule_conditions_json)
    assert loaded == [
        {
            "id": "legacy-1",
            "name": "legacy",
            "record_types": ["log", "trace"],
            "match_field": "service_name",
            "match_operator": "contains",
            "match_value": "checkout",
            "match_attr_key": "",
            "tag_key": "team",
            "tag_value": "payments",
            "conditions": [
                {
                    "match_field": "service_name",
                    "match_operator": "contains",
                    "match_value": "checkout",
                    "match_attr_key": "",
                }
            ],
        },
        {
            "id": "composite-1",
            "name": "composite",
            "record_types": ["all"],
            "match_field": "severity",
            "match_operator": "eq",
            "match_value": "WARN",
            "match_attr_key": "",
            "tag_key": "env",
            "tag_value": "prod",
            "conditions": [
                {
                    "match_field": "severity",
                    "match_operator": "eq",
                    "match_value": "ERROR",
                    "match_attr_key": "",
                }
            ],
        },
    ]


def test_shared_tag_rules_match_single_condition_covers_supported_fields_and_invalid_regex():
    attrs = {"http.status_code": "500"}
    assert (
        _match_single_condition(
            {"match_field": "service_name", "match_operator": "eq", "match_value": "svc"}, "svc", "ERROR", "body", attrs
        )
        is True
    )
    assert (
        _match_single_condition(
            {"match_field": "body", "match_operator": "contains", "match_value": "timeout"},
            "svc",
            "ERROR",
            "Connection Timeout",
            attrs,
        )
        is True
    )
    assert (
        _match_single_condition(
            {"match_field": "span_name", "match_operator": "regex", "match_value": r"^checkout"},
            "svc",
            "ERROR",
            "body",
            attrs,
            "checkout request",
        )
        is True
    )
    assert (
        _match_single_condition(
            {"match_field": "event_type", "match_operator": "eq", "match_value": "pageview"},
            "svc",
            "ERROR",
            "body",
            attrs,
            "",
            "pageview",
        )
        is True
    )
    assert (
        _match_single_condition(
            {
                "match_field": "attribute",
                "match_operator": "eq",
                "match_value": "500",
                "match_attr_key": "http.status_code",
            },
            "svc",
            "ERROR",
            "body",
            attrs,
        )
        is True
    )
    assert (
        _match_single_condition(
            {"match_field": "body", "match_operator": "regex", "match_value": "[invalid"}, "svc", "ERROR", "body", attrs
        )
        is False
    )
    assert (
        _match_single_condition(
            {"match_field": "unknown", "match_operator": "eq", "match_value": "x"}, "svc", "ERROR", "body", attrs
        )
        is False
    )
    assert (
        _match_single_condition(
            {"match_field": "service_name", "match_operator": "wat", "match_value": "svc"},
            "svc",
            "ERROR",
            "body",
            attrs,
        )
        is False
    )


def test_shared_tag_rules_match_rule_respects_record_types_and_composite_precedence():
    simple_rule = {
        "record_types": ["trace"],
        "match_field": "severity",
        "match_operator": "eq",
        "match_value": "ERROR",
        "match_attr_key": "",
    }
    composite_rule = {
        "record_types": ["all"],
        "match_field": "severity",
        "match_operator": "eq",
        "match_value": "WARN",
        "match_attr_key": "",
        "conditions": [
            {"match_field": "severity", "match_operator": "eq", "match_value": "ERROR", "match_attr_key": ""},
            {"match_field": "body", "match_operator": "contains", "match_value": "timeout", "match_attr_key": ""},
        ],
    }

    assert (
        _match_tag_rule(simple_rule, "log", "svc", "ERROR", "body", {}, match_single_condition=_match_single_condition)
        is False
    )
    assert (
        _match_tag_rule(
            simple_rule, "trace", "svc", "ERROR", "body", {}, match_single_condition=_match_single_condition
        )
        is True
    )
    assert (
        _match_tag_rule(
            composite_rule, "log", "svc", "ERROR", "timeout body", {}, match_single_condition=_match_single_condition
        )
        is True
    )
    assert (
        _match_tag_rule(
            composite_rule, "log", "svc", "ERROR", "healthy body", {}, match_single_condition=_match_single_condition
        )
        is False
    )


def test_shared_tag_rules_attribute_key_suggestions_rank_and_filter_matches():
    suggestions = _tag_rule_attribute_key_suggestions(
        object(),
        "http.",
        3,
        attr_key_record_types=("log", "trace"),
        get_cached_attr_keys=lambda db, record_type: {
            "log": {"http.route", "service.version", "http.method"},
            "trace": {"http.status_code", "db.statement", "http.route"},
        }[record_type],
    )
    assert suggestions == ["http.method", "http.route", "http.status_code"]

    unfiltered = _tag_rule_attribute_key_suggestions(
        object(),
        "",
        2,
        attr_key_record_types=("log",),
        get_cached_attr_keys=lambda db, record_type: {"zeta", "alpha", "beta"},
    )
    assert unfiltered == ["alpha", "beta"]
