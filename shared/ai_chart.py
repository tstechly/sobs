"""Shared AI chart-spec parsing and generation helpers."""

from __future__ import annotations

import json
import re
from collections.abc import Awaitable, Callable
from typing import Any


def _normalize_chart_spec_text(spec_raw: str) -> str:
    """Extract a likely JSON object from a raw chart-spec model reply."""
    spec = str(spec_raw or "").strip()
    if spec.startswith("```"):
        spec = re.sub(r"^```[a-zA-Z]*\n?", "", spec)
        spec = re.sub(r"\n?```$", "", spec)
    spec = spec.strip()

    first_obj = spec.find("{")
    last_obj = spec.rfind("}")
    if first_obj >= 0 and last_obj > first_obj:
        spec = spec[first_obj : last_obj + 1].strip()
    return spec


_JSON_VALUE_TOKEN_PATTERN = r'"(?:\\.|[^"\\])*"|true|false|null|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?|\}|\]'


def _insert_missing_json_commas(text: str) -> str:
    """Best-effort repair for missing commas between JSON values/items."""
    repaired = str(text or "")
    if not repaired:
        return repaired

    object_member_pattern = re.compile(
        rf"({_JSON_VALUE_TOKEN_PATTERN})(\\s+)(\"(?:\\\\.|[^\"\\\\])*\"\\s*:)",
        flags=re.DOTALL,
    )
    array_item_pattern = re.compile(
        rf"({_JSON_VALUE_TOKEN_PATTERN})(\\s+)(\{{|\[|\"(?:\\\\.|[^\"\\\\])*\"|true|false|null|-?\d)",
        flags=re.DOTALL,
    )

    for _ in range(4):
        previous = repaired
        repaired = object_member_pattern.sub(r"\1,\2\3", repaired)
        repaired = array_item_pattern.sub(r"\1,\2\3", repaired)
        repaired = re.sub(r",\s*,+", ",", repaired)
        repaired = re.sub(
            r'([}\]"0-9eE])\s*(?="(?:\\.|[^"\\])*"\s*:)',
            r"\1, ",
            repaired,
        )
        if repaired == previous:
            break
    return repaired


def _parse_chart_spec_json(spec_raw: str) -> tuple[dict[str, Any] | None, str]:
    """Parse chart JSON with a lightweight local repair pass."""
    spec = _normalize_chart_spec_text(spec_raw)
    if not spec:
        return None, "empty chart spec"

    try:
        parsed = json.loads(spec)
    except Exception:
        repaired = re.sub(r"//[^\n]*", "", spec)
        repaired = re.sub(r"/\*.*?\*/", "", repaired, flags=re.DOTALL)
        repaired = re.sub(r",\s*([}\]])", r"\1", repaired)
        repaired = _insert_missing_json_commas(repaired)
        repaired = repaired.strip()
        try:
            parsed = json.loads(repaired)
        except Exception as exc2:
            return None, str(exc2)

    if not isinstance(parsed, dict):
        return None, "top-level chart spec must be a JSON object"
    return parsed, ""


async def _repair_chart_spec_json_with_llm(
    spec_raw: str,
    parse_error: str,
    settings: dict[str, str],
    *,
    call_llm_endpoint: Callable[..., Awaitable[tuple[str, dict[str, Any]]]],
    query_chart_json_repair_system_prompt: str,
    query_llm_max_tokens: int,
) -> tuple[dict[str, Any] | None, str, dict[str, Any]]:
    """Ask the LLM for a strict JSON repair when local parsing fails."""
    endpoint_url = str(settings.get("ai.endpoint_url") or "").strip()
    model = str(settings.get("ai.model") or "").strip()
    api_key = str(settings.get("ai.api_key") or "").strip()
    if not endpoint_url or not model:
        return None, "AI endpoint not configured.", {}

    user_message = (
        "The chart JSON below failed to parse. Repair it and return only valid JSON.\n\n"
        f"Parse error: {parse_error}\n\n"
        f"Malformed chart JSON:\n{spec_raw}"
    )
    messages = [
        {"role": "system", "content": query_chart_json_repair_system_prompt},
        {"role": "user", "content": user_message},
    ]
    repaired_raw, repair_stats = await call_llm_endpoint(
        endpoint_url,
        model,
        api_key,
        messages,
        max_tokens=query_llm_max_tokens,
        thinking_level="off",
    )
    if not repaired_raw:
        error_detail = str(repair_stats.get("error") or "").strip()
        if error_detail:
            return None, f"LLM JSON repair failed: {error_detail}", repair_stats
        return None, "LLM JSON repair returned empty content.", repair_stats

    parsed, parse_err = _parse_chart_spec_json(repaired_raw)
    if parsed is None:
        return None, f"LLM JSON repair output was still invalid: {parse_err}", repair_stats
    return parsed, "", repair_stats


async def _vanna_generate_chart_spec(
    columns: list[str],
    sample_rows: list[dict[str, Any]],
    question: str,
    settings: dict[str, str],
    preferred_chart_type: str = "",
    chart_instruction: str = "",
    named_datasets: list[dict[str, Any]] | None = None,
    thinking_level: str = "off",
    *,
    call_llm_endpoint: Callable[..., Awaitable[tuple[str, dict[str, Any]]]],
    resolve_endpoint_timeout_seconds: Callable[[dict[str, str]], float | int],
    query_chart_system_prompt: str,
    query_chart_json_repair_system_prompt: str,
    query_llm_max_tokens: int,
    repair_chart_spec_json_with_llm: (
        Callable[[str, str, dict[str, str]], Awaitable[tuple[dict[str, Any] | None, str, dict[str, Any]]]] | None
    ) = None,
) -> tuple[str, str, dict[str, Any]]:
    """Ask the LLM to produce an ECharts option JSON for the result set."""
    endpoint_url = str(settings.get("ai.endpoint_url") or "").strip()
    model = str(settings.get("ai.model") or "").strip()
    api_key = str(settings.get("ai.api_key") or "").strip()

    if not endpoint_url or not model:
        return "", "AI endpoint not configured.", {}

    sample_str = json.dumps({"columns": columns, "rows": sample_rows[:20]}, ensure_ascii=False, default=str)
    named_datasets_str = ""
    if named_datasets:
        condensed = []
        for dataset in named_datasets:
            if not isinstance(dataset, dict):
                continue
            condensed.append(
                {
                    "name": dataset.get("name", ""),
                    "purpose": dataset.get("purpose", ""),
                    "columns": dataset.get("columns", []),
                    "rows": (dataset.get("rows", []) or [])[:20],
                }
            )
        if condensed:
            named_datasets_str = (
                "\n\nNamed datasets (use when multi-dataset chart structures are needed):\n"
                + json.dumps(condensed, ensure_ascii=False, default=str)
            )
    preference_lines: list[str] = []
    if preferred_chart_type:
        preference_lines.append(f"Preferred chart type: {preferred_chart_type}")
    if chart_instruction:
        preference_lines.append(f"Chart instruction: {chart_instruction}")
    preference_block = "\n".join(preference_lines)
    if preference_block:
        preference_block = f"\n\nChart preferences:\n{preference_block}"

    user_message = (
        f"Original question: {question}\n\n"
        f"Result set (columns + up to 20 sample rows):\n{sample_str}\n\n"
        f"{named_datasets_str}"
        f"{preference_block}"
        "Produce an ECharts option JSON object for this data."
    )
    messages = [
        {"role": "system", "content": query_chart_system_prompt},
        {"role": "user", "content": user_message},
    ]

    endpoint_timeout = resolve_endpoint_timeout_seconds(settings)
    spec_raw, stats = await call_llm_endpoint(
        endpoint_url,
        model,
        api_key,
        messages,
        max_tokens=query_llm_max_tokens,
        thinking_level=thinking_level,
        timeout=endpoint_timeout,
    )
    if not spec_raw:
        error_detail = str(stats.get("error") or "").strip()
        if error_detail:
            return "", f"LLM chart request failed: {error_detail}", stats
        return "", "LLM did not return a chart spec.", stats

    parsed, parse_err = _parse_chart_spec_json(spec_raw)
    if parsed is not None:
        if parsed == {}:
            return "", "LLM returned an empty chart spec object.", stats
        return json.dumps(parsed, ensure_ascii=False), "", stats

    if repair_chart_spec_json_with_llm is None:
        repaired_parsed, repair_error, repair_stats = await _repair_chart_spec_json_with_llm(
            spec_raw,
            parse_err,
            settings,
            call_llm_endpoint=call_llm_endpoint,
            query_chart_json_repair_system_prompt=query_chart_json_repair_system_prompt,
            query_llm_max_tokens=query_llm_max_tokens,
        )
    else:
        repaired_parsed, repair_error, repair_stats = await repair_chart_spec_json_with_llm(
            spec_raw,
            parse_err,
            settings,
        )
    if repaired_parsed is None:
        if repair_error:
            return "", f"Chart spec JSON parse error: {parse_err}. {repair_error}", stats
        return "", f"Chart spec JSON parse error: {parse_err}", stats

    if repaired_parsed == {}:
        return "", "LLM JSON repair returned an empty chart spec object.", stats

    merged_stats: dict[str, Any] = dict(stats)
    merged_stats["chart_json_repair"] = 1
    if repair_stats:
        merged_stats["chart_json_repair_stats"] = repair_stats
    return json.dumps(repaired_parsed, ensure_ascii=False), "", merged_stats
