"""Shared AI SQL planning and repair helpers."""

from __future__ import annotations

import json
import re
from collections.abc import Awaitable, Callable, Collection
from typing import Any


def _strip_markdown_fences(text: str) -> str:
    cleaned = str(text or "").strip()
    if cleaned.startswith("```"):
        cleaned = re.sub(r"^```[a-zA-Z]*\n?", "", cleaned)
        cleaned = re.sub(r"\n?```$", "", cleaned)
    return cleaned.strip()


async def _vanna_generate_sql(
    question: str,
    schema_context: str,
    settings: dict[str, str],
    preferred_chart_type: str = "",
    chart_instruction: str = "",
    thinking_level: str = "off",
    *,
    call_llm_endpoint: Callable[..., Awaitable[tuple[str, dict[str, Any]]]],
    load_chart_types_catalog: Callable[[], dict[str, Any]],
    resolve_endpoint_timeout_seconds: Callable[[dict[str, str]], float | int],
    query_sql_system_prompt: str,
    query_allowed_tables: Collection[str],
    query_llm_max_tokens: int,
) -> tuple[str, str, dict[str, Any]]:
    """Ask the configured LLM to generate SQL for a question."""
    endpoint_url = str(settings.get("ai.endpoint_url") or "").strip()
    model = str(settings.get("ai.model") or "").strip()
    api_key = str(settings.get("ai.api_key") or "").strip()

    if not endpoint_url or not model:
        return "", "AI endpoint not configured. Visit Settings → AI Configuration.", {}

    system_prompt = query_sql_system_prompt.format(schema=schema_context)
    allowlist_hint = "\n".join(f"- {name}" for name in sorted(query_allowed_tables))
    user_content = (
        f"{question}\n\n" "Allowed queryable tables/views (must stay within this list):\n" f"{allowlist_hint}"
    )
    chart_guidance: list[str] = []
    if preferred_chart_type:
        chart_guidance.append(f"Preferred chart type: {preferred_chart_type}")
    if chart_instruction:
        chart_guidance.append(f"Chart instruction: {chart_instruction}")

    if preferred_chart_type:
        catalog = load_chart_types_catalog()
        chart_types = catalog.get("chartTypes") if isinstance(catalog, dict) else None
        chart_info = chart_types.get(preferred_chart_type) if isinstance(chart_types, dict) else None
        if isinstance(chart_info, dict):
            data_structure = chart_info.get("dataStructure") or {}
            if isinstance(data_structure, dict):
                ds_type = str(data_structure.get("type") or "").strip()
                ds_example = str(data_structure.get("example") or "").strip()
                if ds_type:
                    chart_guidance.append(f"Desired chart data shape: {ds_type}")
                if ds_example:
                    chart_guidance.append(f"Desired chart data example: {ds_example}")

    if chart_guidance:
        user_content = f"{user_content}\n\n" "Chart generation guidance (shape SQL output to fit this):\n" + "\n".join(
            [f"- {line}" for line in chart_guidance]
        )

    messages = [
        {"role": "system", "content": system_prompt},
        {"role": "user", "content": user_content},
    ]

    endpoint_timeout = resolve_endpoint_timeout_seconds(settings)
    sql_raw, stats = await call_llm_endpoint(
        endpoint_url,
        model,
        api_key,
        messages,
        max_tokens=query_llm_max_tokens,
        thinking_level=thinking_level,
        timeout=endpoint_timeout,
    )
    if not sql_raw:
        error_detail = str(stats.get("error") or "").strip()
        if error_detail:
            return "", f"LLM request failed: {error_detail}", stats
        return "", "LLM did not return a response. Check AI settings.", stats

    sql = _strip_markdown_fences(sql_raw)
    if not sql:
        return "", "LLM returned an empty SQL statement.", stats
    return sql, "", stats


async def _vanna_generate_named_queries(
    question: str,
    schema_context: str,
    base_sql: str,
    settings: dict[str, str],
    preferred_chart_type: str = "",
    chart_instruction: str = "",
    thinking_level: str = "off",
    *,
    call_llm_endpoint: Callable[..., Awaitable[tuple[str, dict[str, Any]]]],
    resolve_endpoint_timeout_seconds: Callable[[dict[str, str]], float | int],
    query_llm_max_tokens: int,
) -> tuple[list[dict[str, str]], str, dict[str, Any]]:
    """Ask the LLM for optional named dataset SQL queries for complex charts."""
    endpoint_url = str(settings.get("ai.endpoint_url") or "").strip()
    model = str(settings.get("ai.model") or "").strip()
    api_key = str(settings.get("ai.api_key") or "").strip()

    if not endpoint_url or not model:
        return [], "AI endpoint not configured.", {}

    preferred = preferred_chart_type or "auto"
    instruction = chart_instruction or ""
    system_prompt = (
        "You are a ClickHouse SQL planner for chart datasets. "
        "Return ONLY valid JSON with the shape: "
        '{"datasets":[{"name":"...","sql":"SELECT ...","purpose":"..."}]}. '
        "Rules: use only read-only SELECT/WITH queries; keep at most 3 datasets; "
        "names should be short snake_case identifiers; no markdown."
    )
    user_message = (
        f"Question: {question}\n\n"
        f"Preferred chart type: {preferred}\n"
        f"Chart instruction: {instruction}\n\n"
        f"Primary SQL:\n{base_sql}\n\n"
        f"Schema context:\n{schema_context}\n\n"
        "If one dataset is sufficient, return an empty datasets array. "
        "For network/flow charts (graph/sankey/chord), prefer separate nodes and links datasets."
    )
    messages = [
        {"role": "system", "content": system_prompt},
        {"role": "user", "content": user_message},
    ]

    endpoint_timeout = resolve_endpoint_timeout_seconds(settings)
    plan_raw, stats = await call_llm_endpoint(
        endpoint_url,
        model,
        api_key,
        messages,
        max_tokens=query_llm_max_tokens,
        thinking_level=thinking_level,
        timeout=endpoint_timeout,
    )
    if not plan_raw:
        return [], str(stats.get("error") or "").strip(), stats

    plan_text = _strip_markdown_fences(plan_raw)
    first_obj = plan_text.find("{")
    last_obj = plan_text.rfind("}")
    if first_obj >= 0 and last_obj > first_obj:
        plan_text = plan_text[first_obj : last_obj + 1].strip()

    try:
        parsed = json.loads(plan_text)
    except Exception:
        return [], "", stats

    raw_datasets = parsed.get("datasets") if isinstance(parsed, dict) else []
    if not isinstance(raw_datasets, list):
        return [], "", stats

    datasets: list[dict[str, str]] = []
    base_sql_normalized = str(base_sql).strip().rstrip(";")
    for item in raw_datasets[:3]:
        if not isinstance(item, dict):
            continue
        name = str(item.get("name") or "").strip().lower()
        sql = str(item.get("sql") or "").strip().rstrip(";")
        purpose = str(item.get("purpose") or "").strip()
        if not name or not re.match(r"^[a-z][a-z0-9_]{0,31}$", name):
            continue
        upper_sql = sql.upper().lstrip()
        if not (upper_sql.startswith("SELECT") or upper_sql.startswith("WITH")):
            continue
        if sql == base_sql_normalized:
            continue
        datasets.append({"name": name, "sql": sql, "purpose": purpose})

    return datasets, "", stats


async def _vanna_repair_sql(
    question: str,
    schema_context: str,
    previous_sql: str,
    execution_error: str,
    settings: dict[str, str],
    attempt_number: int,
    thinking_level: str = "off",
    *,
    call_llm_endpoint: Callable[..., Awaitable[tuple[str, dict[str, Any]]]],
    resolve_endpoint_timeout_seconds: Callable[[dict[str, str]], float | int],
    query_sql_system_prompt: str,
    query_llm_max_tokens: int,
) -> tuple[str, str, dict[str, Any]]:
    """Ask the LLM to fix SQL after an execution failure."""
    endpoint_url = str(settings.get("ai.endpoint_url") or "").strip()
    model = str(settings.get("ai.model") or "").strip()
    api_key = str(settings.get("ai.api_key") or "").strip()

    if not endpoint_url or not model:
        return "", "AI endpoint not configured.", {}

    system_prompt = query_sql_system_prompt.format(schema=schema_context)
    user_message = (
        f"Original question: {question}\n\n"
        f"Previous SQL (attempt {attempt_number}):\n{previous_sql}\n\n"
        f"Execution error:\n{execution_error}\n\n"
        "Rewrite the SQL so it is valid for this schema and still answers the question. "
        "Return ONLY raw SQL."
    )
    messages = [
        {"role": "system", "content": system_prompt},
        {"role": "user", "content": user_message},
    ]

    endpoint_timeout = resolve_endpoint_timeout_seconds(settings)
    sql_raw, stats = await call_llm_endpoint(
        endpoint_url,
        model,
        api_key,
        messages,
        max_tokens=query_llm_max_tokens,
        thinking_level=thinking_level,
        timeout=endpoint_timeout,
        empty_content_retry_instruction=(
            "Return ONLY complete executable ClickHouse SQL. No reasoning, no markdown, no commentary."
        ),
    )
    if not sql_raw:
        error_detail = str(stats.get("error") or "").strip()
        if error_detail:
            return "", f"LLM repair request failed: {error_detail}", stats
        return "", "LLM did not return a repaired SQL statement.", stats

    sql = _strip_markdown_fences(sql_raw)
    if not sql:
        return "", "LLM returned an empty repaired SQL statement.", stats
    return sql, "", stats


def _repair_truncated_in_clause_literals(sql: str) -> str:
    """Best-effort fix for a truncated trailing ``IN (...)`` literal list."""
    text = str(sql or "")
    match = re.search(r"\bIN\s*\(([^)]*)$", text, flags=re.IGNORECASE | re.DOTALL)
    if not match:
        return text

    items_raw = match.group(1)
    if not items_raw.strip():
        return text

    cleaned_items: list[str] = []
    for item in items_raw.split(","):
        token = item.strip()
        if not token:
            continue
        if token.count("'") % 2 != 0:
            break
        cleaned_items.append(token)

    if not cleaned_items:
        return text

    return text[: match.start(1)] + ",".join(cleaned_items) + ")"


def _auto_repair_incomplete_cte_sql(sql: str) -> str:
    """Best-effort local fix for truncated CTE SQL."""
    text = str(sql or "").strip().rstrip(";")
    if not text:
        return ""

    if not re.match(r"^\s*with\b", text, flags=re.IGNORECASE):
        return ""

    text = _repair_truncated_in_clause_literals(text)
    if text.count("'") % 2 != 0:
        return ""

    cte_match = re.match(r"^\s*with\s+([a-zA-Z_]\w*)\s+as\s*\(", text, flags=re.IGNORECASE)
    if not cte_match:
        return ""

    has_final_select = re.search(r"\)\s*select\b", text, flags=re.IGNORECASE | re.DOTALL) is not None
    open_parens = text.count("(")
    close_parens = text.count(")")

    if has_final_select and open_parens <= close_parens:
        return ""

    fixed = text
    if open_parens > close_parens:
        fixed += ")" * (open_parens - close_parens)

    if re.search(r"\)\s*select\b", fixed, flags=re.IGNORECASE | re.DOTALL) is None:
        fixed += f"\nSELECT * FROM {cte_match.group(1)}"

    return fixed
