import re

from shared.sql_where import (
    _append_regex_expression_clauses,
    _append_time_window_filter,
    _normalize_ai_sql_where,
    _replace_sql_outside_single_quotes,
    _time_window_conditions,
    _validate_user_sql_where,
    _where_clause,
)


def test_shared_sql_where_time_window_and_where_helpers_cover_empty_and_populated_paths():
    assert _time_window_conditions("Timestamp", "", "") == ([], [])
    assert _time_window_conditions("Timestamp", "2026-04-30T10:00:00Z", "2026-04-30T11:00:00Z") == (
        [
            "Timestamp >= parseDateTime64BestEffort(?, 9)",
            "Timestamp < parseDateTime64BestEffort(?, 9)",
        ],
        ["2026-04-30T10:00:00Z", "2026-04-30T11:00:00Z"],
    )

    conditions = ["SeverityText = 'ERROR'"]
    params = ["existing"]
    _append_time_window_filter(
        conditions,
        params,
        "Timestamp",
        "2026-04-30T10:00:00Z",
        "",
        time_window_conditions=_time_window_conditions,
    )
    assert conditions == [
        "SeverityText = 'ERROR'",
        "Timestamp >= parseDateTime64BestEffort(?, 9)",
    ]
    assert params == ["existing", "2026-04-30T10:00:00Z"]
    assert _where_clause([]) == ""
    assert _where_clause(["a = 1", "b = 2"]) == "WHERE a = 1 AND b = 2"


def test_shared_sql_where_regex_clause_builder_appends_include_and_exclude_patterns():
    conditions: list[str] = []
    params: list[str] = []
    _append_regex_expression_clauses(
        conditions=conditions,
        params=params,
        column="Body",
        include_patterns=["error", "timeout"],
        exclude_patterns=["healthcheck"],
    )
    assert conditions == [
        "match(Body, ?)",
        "match(Body, ?)",
        "NOT match(Body, ?)",
    ]
    assert params == ["error", "timeout", "healthcheck"]


def test_shared_sql_where_replace_preserves_single_quoted_literals_and_escaped_quotes():
    sql = "prompt = 'service' AND service = 'api''s' AND response = prompt"
    replaced = _replace_sql_outside_single_quotes(
        sql,
        [
            (r"\bservice\b", "ServiceName"),
            (r"\bprompt\b", "PromptExpr"),
            (r"\bresponse\b", "ResponseExpr"),
        ],
    )
    assert replaced == "PromptExpr = 'service' AND ServiceName = 'api''s' AND ResponseExpr = PromptExpr"


def test_shared_sql_where_normalize_applies_replacements_and_strips_semicolons():
    validated = []

    def validate_user_sql_where(value):
        validated.append(value)

    raw_sql = (
        "service = 'prompt'; AND prompt = 'response' AND response = prompt "
        "AND LogAttributes['foo'] != '' AND tokens_in > 1"
    )
    normalized = _normalize_ai_sql_where(
        raw_sql,
        validate_user_sql_where=validate_user_sql_where,
        ai_trace_prompt_sql="PROMPT_SQL",
        ai_trace_response_sql="RESPONSE_SQL",
        replace_sql_outside_single_quotes=_replace_sql_outside_single_quotes,
    )
    assert validated == [raw_sql]
    assert ";" not in normalized
    assert normalized == (
        "ServiceName = 'prompt' AND PROMPT_SQL = 'response' AND RESPONSE_SQL = PROMPT_SQL "
        "AND SpanAttributes['foo'] != '' AND toUInt64OrZero(SpanAttributes['gen_ai.usage.input_tokens']) > 1"
    )


def test_shared_sql_where_validate_blocks_unsafe_keywords_only():
    unsafe_where_patterns = re.compile(
        r"\b(insert|update|delete|drop|truncate|alter|create|replace|rename|attach|detach|"
        r"grant|revoke|system\s+stop|system\s+start|system\s+reload|kill|optimize|exchange)\b",
        re.IGNORECASE,
    )
    _validate_user_sql_where("ServiceName = 'api' UNION ALL SELECT 1", unsafe_where_patterns=unsafe_where_patterns)
    try:
        _validate_user_sql_where("1=1 DROP TABLE otel_logs", unsafe_where_patterns=unsafe_where_patterns)
    except ValueError as exc:
        assert "disallowed keyword" in str(exc)
    else:
        raise AssertionError("Expected ValueError for unsafe SQL filter")
