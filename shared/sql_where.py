"""Shared SQL WHERE and time-window helper logic."""

from __future__ import annotations

import re


def _time_window_conditions(column: str, from_ts: str, to_ts: str) -> tuple[list[str], list[str]]:
    conditions: list[str] = []
    params: list[str] = []
    if from_ts:
        conditions.append(f"{column} >= parseDateTime64BestEffort(?, 9)")
        params.append(from_ts)
    if to_ts:
        conditions.append(f"{column} < parseDateTime64BestEffort(?, 9)")
        params.append(to_ts)
    return conditions, params


def _append_time_window_filter(
    conditions: list[str],
    params: list[str],
    column: str,
    from_ts: str,
    to_ts: str,
    *,
    time_window_conditions,
) -> None:
    time_conditions, time_params = time_window_conditions(column, from_ts, to_ts)
    conditions.extend(time_conditions)
    params.extend(time_params)


def _where_clause(conditions: list[str]) -> str:
    return ("WHERE " + " AND ".join(conditions)) if conditions else ""


def _append_regex_expression_clauses(
    *,
    conditions: list[str],
    params: list[str],
    column: str,
    include_patterns: list[str],
    exclude_patterns: list[str],
) -> None:
    for pattern in include_patterns:
        conditions.append(f"match({column}, ?)")
        params.append(pattern)
    for pattern in exclude_patterns:
        conditions.append(f"NOT match({column}, ?)")
        params.append(pattern)


def _replace_sql_outside_single_quotes(sql: str, replacements: list[tuple[str, str]]) -> str:
    placeholders: list[str] = []
    masked_parts: list[str] = []
    index = 0
    while index < len(sql):
        char = sql[index]
        if char != "'":
            masked_parts.append(char)
            index += 1
            continue

        start = index
        index += 1
        while index < len(sql):
            if sql[index] == "'":
                if index + 1 < len(sql) and sql[index + 1] == "'":
                    index += 2
                    continue
                index += 1
                break
            index += 1

        literal = sql[start:index]
        token = f"__SQL_LITERAL_{len(placeholders)}__"
        placeholders.append(literal)
        masked_parts.append(token)

    masked = "".join(masked_parts)
    for pattern, replacement in replacements:
        masked = re.sub(pattern, replacement, masked, flags=re.IGNORECASE)
    for idx, literal in enumerate(placeholders):
        masked = masked.replace(f"__SQL_LITERAL_{idx}__", literal)
    return masked


def _normalize_ai_sql_where(
    sql_where: str,
    *,
    validate_user_sql_where,
    ai_trace_prompt_sql: str,
    ai_trace_response_sql: str,
    replace_sql_outside_single_quotes,
) -> str:
    validate_user_sql_where(sql_where)
    safe_sql = str(sql_where or "").replace(";", "")
    replacements = [
        (r"\bLogAttributes\s*\[", "SpanAttributes["),
        (r"SpanAttributes\s*\[\s*'prompt'\s*\]", ai_trace_prompt_sql),
        (r"SpanAttributes\s*\[\s*'response'\s*\]", ai_trace_response_sql),
        (r"\bservice\b", "ServiceName"),
        (r"\bmodel\b", "SpanAttributes['gen_ai.request.model']"),
        (r"\bprovider\b", "SpanAttributes['gen_ai.provider.name']"),
        (r"\boperation\b", "SpanAttributes['gen_ai.operation.name']"),
        (r"\bprompt\b", ai_trace_prompt_sql),
        (r"\bresponse\b", ai_trace_response_sql),
        (r"\btrace_id\b", "TraceId"),
        (r"\bspan_id\b", "SpanId"),
        (r"\bspan_name\b", "SpanName"),
        (r"\brow_type\b", "if(SpanAttributes['gen_ai.request.model'] != '', 'llm', 'system')"),
        (r"\bts\b", "Timestamp"),
        (r"\bstatus\b", "StatusCode"),
        (r"\berror_type\b", "SpanAttributes['error.type']"),
        (r"\btokens_in\b", "toUInt64OrZero(SpanAttributes['gen_ai.usage.input_tokens'])"),
        (r"\btokens_out\b", "toUInt64OrZero(SpanAttributes['gen_ai.usage.output_tokens'])"),
        (r"\bthinking_tokens\b", "toUInt64OrZero(SpanAttributes['gen_ai.usage.thinking_tokens'])"),
        (r"\bduration_ms\b", "(Duration / 1000000.0)"),
    ]
    return replace_sql_outside_single_quotes(safe_sql, replacements)


def _validate_user_sql_where(sql_where: str, *, unsafe_where_patterns) -> None:
    if unsafe_where_patterns.search(sql_where):
        raise ValueError(
            "SQL filter contains a disallowed keyword. "
            "Only comparison and logical expressions are permitted in filter fields."
        )
