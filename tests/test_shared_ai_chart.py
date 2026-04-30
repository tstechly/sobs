from __future__ import annotations

import json
from typing import Any

import pytest

from shared.ai_chart import (
    _insert_missing_json_commas,
    _normalize_chart_spec_text,
    _parse_chart_spec_json,
    _repair_chart_spec_json_with_llm,
    _vanna_generate_chart_spec,
)


def _timeout(_settings: dict[str, str]) -> int:
    return 19


def test_normalize_chart_spec_text_extracts_json_from_fences_and_surrounding_text() -> None:
    assert _normalize_chart_spec_text('```json\n{"series": []}\n```') == '{"series": []}'
    assert _normalize_chart_spec_text('prefix {"series": []} suffix') == '{"series": []}'


def test_insert_missing_json_commas_repairs_adjacent_members_and_items() -> None:
    repaired_object = _insert_missing_json_commas('{"title":{"text":"A"}\n"series":[{"type":"bar"}]}')
    assert json.loads(repaired_object) == {"title": {"text": "A"}, "series": [{"type": "bar"}]}


def test_parse_chart_spec_json_handles_empty_comments_trailing_commas_and_non_object() -> None:
    assert _parse_chart_spec_json("") == (None, "empty chart spec")

    parsed, error = _parse_chart_spec_json('```json\n{\n  // comment\n  "series": [{"type": "bar",}],\n}\n```')
    assert error == ""
    assert parsed == {"series": [{"type": "bar"}]}

    repaired, repaired_error = _parse_chart_spec_json('{"title":{"text":"A"}\n"series":[{"type":"bar"}]}')
    assert repaired_error == ""
    assert repaired == {"title": {"text": "A"}, "series": [{"type": "bar"}]}

    non_object, non_object_error = _parse_chart_spec_json("[1, 2, 3]")
    assert non_object is None
    assert non_object_error == "top-level chart spec must be a JSON object"


@pytest.mark.asyncio
async def test_repair_chart_spec_json_with_llm_requires_configuration() -> None:
    async def _unexpected_call(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        raise AssertionError("LLM should not be called")

    result = await _repair_chart_spec_json_with_llm(
        '{"series": [}',
        "bad json",
        {},
        call_llm_endpoint=_unexpected_call,
        query_chart_json_repair_system_prompt="repair json",
        query_llm_max_tokens=256,
    )

    assert result == (None, "AI endpoint not configured.", {})


@pytest.mark.asyncio
async def test_repair_chart_spec_json_with_llm_surfaces_llm_errors_and_empty_content() -> None:
    async def _error_call(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        return "", {"error": "timeout"}

    async def _empty_call(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        return "", {}

    error_result = await _repair_chart_spec_json_with_llm(
        '{"series": [}',
        "bad json",
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt"},
        call_llm_endpoint=_error_call,
        query_chart_json_repair_system_prompt="repair json",
        query_llm_max_tokens=256,
    )
    empty_result = await _repair_chart_spec_json_with_llm(
        '{"series": [}',
        "bad json",
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt"},
        call_llm_endpoint=_empty_call,
        query_chart_json_repair_system_prompt="repair json",
        query_llm_max_tokens=256,
    )

    assert error_result == (None, "LLM JSON repair failed: timeout", {"error": "timeout"})
    assert empty_result == (None, "LLM JSON repair returned empty content.", {})


@pytest.mark.asyncio
async def test_repair_chart_spec_json_with_llm_repairs_and_rejects_still_invalid_output() -> None:
    async def _valid_call(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        return '{"series":[{"type":"bar"}]}', {"elapsed_ms": 3}

    async def _invalid_call(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        return '{"series": [}', {"elapsed_ms": 4}

    valid_result = await _repair_chart_spec_json_with_llm(
        '{"series": [}',
        "bad json",
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt"},
        call_llm_endpoint=_valid_call,
        query_chart_json_repair_system_prompt="repair json",
        query_llm_max_tokens=256,
    )
    invalid_result = await _repair_chart_spec_json_with_llm(
        '{"series": [}',
        "bad json",
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt"},
        call_llm_endpoint=_invalid_call,
        query_chart_json_repair_system_prompt="repair json",
        query_llm_max_tokens=256,
    )

    assert valid_result == ({"series": [{"type": "bar"}]}, "", {"elapsed_ms": 3})
    assert invalid_result[0] is None
    assert invalid_result[1].startswith("LLM JSON repair output was still invalid:")


@pytest.mark.asyncio
async def test_vanna_generate_chart_spec_requires_configuration() -> None:
    async def _unexpected_call(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        raise AssertionError("LLM should not be called")

    result = await _vanna_generate_chart_spec(
        ["name", "count"],
        [{"name": "otel_logs", "count": 1}],
        "list tables",
        {},
        call_llm_endpoint=_unexpected_call,
        resolve_endpoint_timeout_seconds=_timeout,
        query_chart_system_prompt="chart prompt",
        query_chart_json_repair_system_prompt="repair prompt",
        query_llm_max_tokens=256,
    )

    assert result == ("", "AI endpoint not configured.", {})


@pytest.mark.asyncio
async def test_vanna_generate_chart_spec_builds_prompt_and_returns_parsed_json() -> None:
    seen: dict[str, Any] = {}

    async def _fake_call(
        endpoint: str, model: str, api_key: str, messages: list[dict[str, Any]], **kwargs: Any
    ) -> tuple[str, dict[str, Any]]:
        seen["endpoint"] = endpoint
        seen["model"] = model
        seen["api_key"] = api_key
        seen["messages"] = messages
        seen["kwargs"] = kwargs
        return '{"series":[{"type":"bar","data":[1,2]}]}', {"elapsed_ms": 7}

    named_datasets: Any = [
        {"name": "nodes", "purpose": "entity list", "columns": ["id"], "rows": [{"id": "svc-a"}]},
        "skip-me",
    ]

    spec, error, stats = await _vanna_generate_chart_spec(
        ["name", "count"],
        [{"name": "otel_logs", "count": 1}],
        "list tables",
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt", "ai.api_key": "key"},
        preferred_chart_type="boxplot",
        chart_instruction="show top series",
        named_datasets=named_datasets,
        thinking_level="high",
        call_llm_endpoint=_fake_call,
        resolve_endpoint_timeout_seconds=_timeout,
        query_chart_system_prompt="chart prompt",
        query_chart_json_repair_system_prompt="repair prompt",
        query_llm_max_tokens=256,
    )

    assert json.loads(spec)["series"][0]["type"] == "bar"
    assert error == ""
    assert stats == {"elapsed_ms": 7}
    assert seen["endpoint"] == "https://llm.example"
    assert seen["model"] == "gpt"
    assert seen["api_key"] == "key"
    assert seen["kwargs"] == {"max_tokens": 256, "thinking_level": "high", "timeout": 19}
    assert seen["messages"][0]["content"] == "chart prompt"
    user_message = seen["messages"][1]["content"]
    assert "Original question: list tables" in user_message
    assert '"columns": ["name", "count"]' in user_message
    assert "Named datasets (use when multi-dataset chart structures are needed):" in user_message
    assert '"name": "nodes"' in user_message
    assert "Preferred chart type: boxplot" in user_message
    assert "Chart instruction: show top series" in user_message


@pytest.mark.asyncio
async def test_vanna_generate_chart_spec_surfaces_llm_errors_and_empty_responses() -> None:
    async def _error_call(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        return "", {"error": "timeout"}

    async def _empty_call(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        return "", {}

    error_result = await _vanna_generate_chart_spec(
        ["name"],
        [{"name": "otel_logs"}],
        "list tables",
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt"},
        call_llm_endpoint=_error_call,
        resolve_endpoint_timeout_seconds=_timeout,
        query_chart_system_prompt="chart prompt",
        query_chart_json_repair_system_prompt="repair prompt",
        query_llm_max_tokens=256,
    )
    empty_result = await _vanna_generate_chart_spec(
        ["name"],
        [{"name": "otel_logs"}],
        "list tables",
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt"},
        call_llm_endpoint=_empty_call,
        resolve_endpoint_timeout_seconds=_timeout,
        query_chart_system_prompt="chart prompt",
        query_chart_json_repair_system_prompt="repair prompt",
        query_llm_max_tokens=256,
    )

    assert error_result == ("", "LLM chart request failed: timeout", {"error": "timeout"})
    assert empty_result == ("", "LLM did not return a chart spec.", {})


@pytest.mark.asyncio
async def test_vanna_generate_chart_spec_rejects_empty_object_and_uses_injected_repair() -> None:
    repair_calls: list[tuple[str, str, dict[str, str]]] = []

    async def _empty_object_call(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        return "{}", {"elapsed_ms": 1}

    async def _needs_repair_call(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        return '{"series":[{"type":"bar","data":[1,2]}', {"elapsed_ms": 2}

    async def _repair(
        raw: str, parse_err: str, settings: dict[str, str]
    ) -> tuple[dict[str, Any] | None, str, dict[str, Any]]:
        repair_calls.append((raw, parse_err, settings))
        return {"series": [{"type": "bar", "data": [1, 2]}]}, "", {"elapsed_ms": 3}

    empty_result = await _vanna_generate_chart_spec(
        ["name"],
        [{"name": "otel_logs"}],
        "list tables",
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt"},
        call_llm_endpoint=_empty_object_call,
        resolve_endpoint_timeout_seconds=_timeout,
        query_chart_system_prompt="chart prompt",
        query_chart_json_repair_system_prompt="repair prompt",
        query_llm_max_tokens=256,
    )
    repaired_result = await _vanna_generate_chart_spec(
        ["name"],
        [{"name": "otel_logs"}],
        "list tables",
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt"},
        call_llm_endpoint=_needs_repair_call,
        resolve_endpoint_timeout_seconds=_timeout,
        query_chart_system_prompt="chart prompt",
        query_chart_json_repair_system_prompt="repair prompt",
        query_llm_max_tokens=256,
        repair_chart_spec_json_with_llm=_repair,
    )

    assert empty_result == ("", "LLM returned an empty chart spec object.", {"elapsed_ms": 1})
    assert json.loads(repaired_result[0])["series"][0]["type"] == "bar"
    assert repaired_result[1] == ""
    assert repaired_result[2]["chart_json_repair"] == 1
    assert repaired_result[2]["chart_json_repair_stats"] == {"elapsed_ms": 3}
    assert repair_calls[0][0].startswith('{"series"')


@pytest.mark.asyncio
async def test_vanna_generate_chart_spec_returns_parse_errors_when_repair_fails() -> None:
    async def _bad_json_call(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        return '{"series":[{"type":"bar","data":[1,2]}', {}

    async def _repair_failure(
        _raw: str, _parse_err: str, _settings: dict[str, str]
    ) -> tuple[dict[str, Any] | None, str, dict[str, Any]]:
        return None, "LLM JSON repair failed: bad output", {"error": "bad output"}

    async def _empty_repair(
        _raw: str, _parse_err: str, _settings: dict[str, str]
    ) -> tuple[dict[str, Any] | None, str, dict[str, Any]]:
        return {}, "", {"elapsed_ms": 4}

    failed_result = await _vanna_generate_chart_spec(
        ["name"],
        [{"name": "otel_logs"}],
        "list tables",
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt"},
        call_llm_endpoint=_bad_json_call,
        resolve_endpoint_timeout_seconds=_timeout,
        query_chart_system_prompt="chart prompt",
        query_chart_json_repair_system_prompt="repair prompt",
        query_llm_max_tokens=256,
        repair_chart_spec_json_with_llm=_repair_failure,
    )
    empty_repair_result = await _vanna_generate_chart_spec(
        ["name"],
        [{"name": "otel_logs"}],
        "list tables",
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt"},
        call_llm_endpoint=_bad_json_call,
        resolve_endpoint_timeout_seconds=_timeout,
        query_chart_system_prompt="chart prompt",
        query_chart_json_repair_system_prompt="repair prompt",
        query_llm_max_tokens=256,
        repair_chart_spec_json_with_llm=_empty_repair,
    )

    assert failed_result == (
        "",
        (
            "Chart spec JSON parse error: Expecting ',' delimiter: "
            "line 1 column 39 (char 38). LLM JSON repair failed: bad output"
        ),
        {},
    )
    assert empty_repair_result == (
        "",
        "LLM JSON repair returned an empty chart spec object.",
        {},
    )
