from __future__ import annotations

from typing import Any

import pytest

from shared.ai_sql import (
    _auto_repair_incomplete_cte_sql,
    _repair_truncated_in_clause_literals,
    _vanna_generate_named_queries,
    _vanna_generate_sql,
    _vanna_repair_sql,
)


def _catalog() -> dict[str, Any]:
    return {
        "chartTypes": {
            "bar": {
                "dataStructure": {
                    "type": "categorical_series",
                    "example": '[{"label":"svc-a","value":7}]',
                }
            }
        }
    }


def _timeout(_settings: dict[str, str]) -> int:
    return 17


def test_repair_truncated_in_clause_literals_closes_completed_literals() -> None:
    sql = "SELECT * FROM t WHERE name IN ('svc-a', 'svc-b', 'svc-"

    assert _repair_truncated_in_clause_literals(sql) == "SELECT * FROM t WHERE name IN ('svc-a','svc-b')"


def test_repair_truncated_in_clause_literals_leaves_unfixable_text_unchanged() -> None:
    assert _repair_truncated_in_clause_literals("SELECT * FROM t") == "SELECT * FROM t"
    assert _repair_truncated_in_clause_literals("SELECT * FROM t WHERE name IN (") == "SELECT * FROM t WHERE name IN ("


def test_auto_repair_incomplete_cte_sql_balances_and_adds_final_select() -> None:
    sql = "WITH broken_cte AS (SELECT 1 AS value"

    assert _auto_repair_incomplete_cte_sql(sql) == "WITH broken_cte AS (SELECT 1 AS value)\nSELECT * FROM broken_cte"


def test_auto_repair_incomplete_cte_sql_returns_empty_when_no_fix_is_possible() -> None:
    assert _auto_repair_incomplete_cte_sql("") == ""
    assert _auto_repair_incomplete_cte_sql("SELECT 1") == ""
    assert _auto_repair_incomplete_cte_sql("WITH broken AS (SELECT 'unterminated)") == ""
    assert _auto_repair_incomplete_cte_sql("WITH broken SELECT 1") == ""


def test_auto_repair_incomplete_cte_sql_returns_empty_when_cte_is_already_complete() -> None:
    sql = "WITH ready AS (SELECT 1 AS value) SELECT * FROM ready"

    assert _auto_repair_incomplete_cte_sql(sql) == ""


@pytest.mark.asyncio
async def test_vanna_generate_sql_returns_configuration_error() -> None:
    async def _unexpected_call(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        raise AssertionError("LLM should not be called")

    result = await _vanna_generate_sql(
        "question",
        "schema",
        {},
        call_llm_endpoint=_unexpected_call,
        load_chart_types_catalog=_catalog,
        resolve_endpoint_timeout_seconds=_timeout,
        query_sql_system_prompt="Schema: {schema}",
        query_allowed_tables={"errors"},
        query_llm_max_tokens=512,
    )

    assert result == ("", "AI endpoint not configured. Visit Settings → AI Configuration.", {})


@pytest.mark.asyncio
async def test_vanna_generate_sql_builds_prompt_with_chart_guidance_and_strips_fences() -> None:
    seen: dict[str, Any] = {}

    async def _fake_call(
        endpoint: str, model: str, api_key: str, messages: list[dict[str, Any]], **kwargs: Any
    ) -> tuple[str, dict[str, Any]]:
        seen["endpoint"] = endpoint
        seen["model"] = model
        seen["api_key"] = api_key
        seen["messages"] = messages
        seen["kwargs"] = kwargs
        return "```sql\nSELECT 1\n```", {"elapsed_ms": 12}

    sql, error, stats = await _vanna_generate_sql(
        "show counts",
        "schema text",
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt", "ai.api_key": "key"},
        preferred_chart_type="bar",
        chart_instruction="keep one point per service",
        thinking_level="medium",
        call_llm_endpoint=_fake_call,
        load_chart_types_catalog=_catalog,
        resolve_endpoint_timeout_seconds=_timeout,
        query_sql_system_prompt="Schema:\n{schema}",
        query_allowed_tables={"errors", "traces"},
        query_llm_max_tokens=512,
    )

    assert (sql, error, stats) == ("SELECT 1", "", {"elapsed_ms": 12})
    assert seen["endpoint"] == "https://llm.example"
    assert seen["model"] == "gpt"
    assert seen["api_key"] == "key"
    assert seen["kwargs"] == {"max_tokens": 512, "thinking_level": "medium", "timeout": 17}
    assert seen["messages"][0]["content"] == "Schema:\nschema text"
    user_message = seen["messages"][1]["content"]
    assert "Allowed queryable tables/views" in user_message
    assert "- errors" in user_message
    assert "- traces" in user_message
    assert "Preferred chart type: bar" in user_message
    assert "Chart instruction: keep one point per service" in user_message
    assert "Desired chart data shape: categorical_series" in user_message
    assert 'Desired chart data example: [{"label":"svc-a","value":7}]' in user_message


@pytest.mark.asyncio
async def test_vanna_generate_sql_surfaces_llm_error_and_blank_sql() -> None:
    async def _error_call(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        return "", {"error": "gateway timeout"}

    async def _blank_call(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        return "```sql\n\n```", {}

    error_result = await _vanna_generate_sql(
        "question",
        "schema",
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt"},
        call_llm_endpoint=_error_call,
        load_chart_types_catalog=_catalog,
        resolve_endpoint_timeout_seconds=_timeout,
        query_sql_system_prompt="Schema: {schema}",
        query_allowed_tables={"errors"},
        query_llm_max_tokens=512,
    )
    blank_result = await _vanna_generate_sql(
        "question",
        "schema",
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt"},
        call_llm_endpoint=_blank_call,
        load_chart_types_catalog=_catalog,
        resolve_endpoint_timeout_seconds=_timeout,
        query_sql_system_prompt="Schema: {schema}",
        query_allowed_tables={"errors"},
        query_llm_max_tokens=512,
    )

    assert error_result == ("", "LLM request failed: gateway timeout", {"error": "gateway timeout"})
    assert blank_result == ("", "LLM returned an empty SQL statement.", {})


@pytest.mark.asyncio
async def test_vanna_generate_named_queries_returns_configuration_error() -> None:
    async def _unexpected_call(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        raise AssertionError("LLM should not be called")

    result = await _vanna_generate_named_queries(
        "question",
        "schema",
        "SELECT 1",
        {},
        call_llm_endpoint=_unexpected_call,
        resolve_endpoint_timeout_seconds=_timeout,
        query_llm_max_tokens=512,
    )

    assert result == ([], "AI endpoint not configured.", {})


@pytest.mark.asyncio
async def test_vanna_generate_named_queries_filters_invalid_datasets() -> None:
    seen: dict[str, Any] = {}

    async def _fake_call(
        _endpoint: str, _model: str, _api_key: str, messages: list[dict[str, Any]], **kwargs: Any
    ) -> tuple[str, dict[str, Any]]:
        seen["messages"] = messages
        seen["kwargs"] = kwargs
        return (
            "```json\n"
            '{"datasets":['
            '{"name":"good_ds","sql":"SELECT 2 AS n;","purpose":"good"},'
            '{"name":"Bad Name","sql":"SELECT 3","purpose":"bad name"},'
            '{"name":"same_sql","sql":"SELECT 1;","purpose":"duplicate"},'
            '{"name":"mutating","sql":"DELETE FROM x","purpose":"bad sql"}'
            "]}\n```",
            {"elapsed_ms": 8},
        )

    datasets, error, stats = await _vanna_generate_named_queries(
        "question",
        "schema",
        "SELECT 1",
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt"},
        preferred_chart_type="sankey",
        chart_instruction="show edges",
        thinking_level="low",
        call_llm_endpoint=_fake_call,
        resolve_endpoint_timeout_seconds=_timeout,
        query_llm_max_tokens=256,
    )

    assert datasets == [{"name": "good_ds", "sql": "SELECT 2 AS n", "purpose": "good"}]
    assert error == ""
    assert stats == {"elapsed_ms": 8}
    assert seen["kwargs"] == {"max_tokens": 256, "thinking_level": "low", "timeout": 17}
    assert "Preferred chart type: sankey" in seen["messages"][1]["content"]
    assert "Chart instruction: show edges" in seen["messages"][1]["content"]


@pytest.mark.asyncio
async def test_vanna_generate_named_queries_handles_empty_invalid_json_and_non_list_payloads() -> None:
    async def _empty_call(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        return "", {"error": "upstream failed"}

    async def _invalid_json_call(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        return "not-json", {"elapsed_ms": 1}

    async def _non_list_call(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        return '{"datasets": {"name": "bad"}}', {"elapsed_ms": 2}

    empty_result = await _vanna_generate_named_queries(
        "question",
        "schema",
        "SELECT 1",
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt"},
        call_llm_endpoint=_empty_call,
        resolve_endpoint_timeout_seconds=_timeout,
        query_llm_max_tokens=256,
    )
    invalid_result = await _vanna_generate_named_queries(
        "question",
        "schema",
        "SELECT 1",
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt"},
        call_llm_endpoint=_invalid_json_call,
        resolve_endpoint_timeout_seconds=_timeout,
        query_llm_max_tokens=256,
    )
    non_list_result = await _vanna_generate_named_queries(
        "question",
        "schema",
        "SELECT 1",
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt"},
        call_llm_endpoint=_non_list_call,
        resolve_endpoint_timeout_seconds=_timeout,
        query_llm_max_tokens=256,
    )

    assert empty_result == ([], "upstream failed", {"error": "upstream failed"})
    assert invalid_result == ([], "", {"elapsed_ms": 1})
    assert non_list_result == ([], "", {"elapsed_ms": 2})


@pytest.mark.asyncio
async def test_vanna_repair_sql_returns_configuration_error() -> None:
    async def _unexpected_call(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        raise AssertionError("LLM should not be called")

    result = await _vanna_repair_sql(
        "question",
        "schema",
        "SELECT 1",
        "bad column",
        {},
        2,
        call_llm_endpoint=_unexpected_call,
        resolve_endpoint_timeout_seconds=_timeout,
        query_sql_system_prompt="Schema: {schema}",
        query_llm_max_tokens=256,
    )

    assert result == ("", "AI endpoint not configured.", {})


@pytest.mark.asyncio
async def test_vanna_repair_sql_passes_retry_instruction_and_strips_fences() -> None:
    seen: dict[str, Any] = {}

    async def _fake_call(
        _endpoint: str, _model: str, _api_key: str, messages: list[dict[str, Any]], **kwargs: Any
    ) -> tuple[str, dict[str, Any]]:
        seen["messages"] = messages
        seen["kwargs"] = kwargs
        return "```sql\nSELECT fixed FROM table\n```", {"elapsed_ms": 4}

    result = await _vanna_repair_sql(
        "question",
        "schema",
        "SELECT broken",
        "unknown column",
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt"},
        3,
        thinking_level="high",
        call_llm_endpoint=_fake_call,
        resolve_endpoint_timeout_seconds=_timeout,
        query_sql_system_prompt="Schema: {schema}",
        query_llm_max_tokens=256,
    )

    assert result == ("SELECT fixed FROM table", "", {"elapsed_ms": 4})
    assert seen["kwargs"] == {
        "max_tokens": 256,
        "thinking_level": "high",
        "timeout": 17,
        "empty_content_retry_instruction": (
            "Return ONLY complete executable ClickHouse SQL. " "No reasoning, no markdown, no commentary."
        ),
    }
    assert "Previous SQL (attempt 3):" in seen["messages"][1]["content"]
    assert "Execution error:" in seen["messages"][1]["content"]


@pytest.mark.asyncio
async def test_vanna_repair_sql_surfaces_llm_errors_and_blank_sql() -> None:
    async def _error_call(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        return "", {"error": "model unavailable"}

    async def _blank_call(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        return "```sql\n\n```", {}

    error_result = await _vanna_repair_sql(
        "question",
        "schema",
        "SELECT broken",
        "error",
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt"},
        1,
        call_llm_endpoint=_error_call,
        resolve_endpoint_timeout_seconds=_timeout,
        query_sql_system_prompt="Schema: {schema}",
        query_llm_max_tokens=256,
    )
    blank_result = await _vanna_repair_sql(
        "question",
        "schema",
        "SELECT broken",
        "error",
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt"},
        1,
        call_llm_endpoint=_blank_call,
        resolve_endpoint_timeout_seconds=_timeout,
        query_sql_system_prompt="Schema: {schema}",
        query_llm_max_tokens=256,
    )

    assert error_result == ("", "LLM repair request failed: model unavailable", {"error": "model unavailable"})
    assert blank_result == ("", "LLM returned an empty repaired SQL statement.", {})
