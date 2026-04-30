"""Shared AI pricing and model-management helpers used by SOBS."""

from __future__ import annotations

import json
from typing import Any


def _normalize_ai_model_name(model: Any) -> str:
    return str(model or "").strip().lower()


def _copy_ai_pricing_entry(prices: dict[str, float]) -> dict[str, float]:
    return {"in": float(prices["in"]), "out": float(prices["out"])}


def _coerce_ai_pricing_entry(prices: Any) -> dict[str, float] | None:
    if not isinstance(prices, dict) or "in" not in prices or "out" not in prices:
        return None
    try:
        return {"in": float(prices["in"]), "out": float(prices["out"])}
    except (TypeError, ValueError):
        return None


def _load_saved_ai_pricing(db, *, load_ai_setting) -> dict[str, dict[str, float]]:
    saved: dict[str, dict[str, float]] = {}
    raw = load_ai_setting(db, "ai.model_pricing", "").strip()
    if not raw:
        return saved
    try:
        user_pricing = json.loads(raw)
    except (json.JSONDecodeError, ValueError):
        return saved
    if not isinstance(user_pricing, dict):
        return saved
    for model_key, prices in user_pricing.items():
        normalized_key = _normalize_ai_model_name(model_key)
        entry = _coerce_ai_pricing_entry(prices)
        if normalized_key and entry:
            saved[normalized_key] = entry
    return saved


def _load_confirmed_ai_pricing_models(db, *, load_ai_setting) -> set[str]:
    raw = load_ai_setting(db, "ai.model_pricing_confirmed", "").strip()
    if not raw:
        return set()
    try:
        parsed = json.loads(raw)
    except (json.JSONDecodeError, ValueError):
        return set()
    if not isinstance(parsed, list):
        return set()
    confirmed: set[str] = set()
    for model in parsed:
        model_key = _normalize_ai_model_name(model)
        if model_key:
            confirmed.add(model_key)
    return confirmed


def _infer_ai_pricing_for_model(
    model: str,
    *,
    default_ai_pricing: dict[str, dict[str, float]],
    generic_default_key: str,
    inference_rules: tuple[tuple[tuple[str, ...], str], ...],
) -> dict[str, float]:
    normalized = _normalize_ai_model_name(model)
    if not normalized:
        return _copy_ai_pricing_entry(default_ai_pricing[generic_default_key])
    if normalized in default_ai_pricing:
        return _copy_ai_pricing_entry(default_ai_pricing[normalized])
    for known_key, prices in default_ai_pricing.items():
        if normalized in known_key or known_key in normalized:
            return _copy_ai_pricing_entry(prices)
    for needles, base_key in inference_rules:
        if any(needle in normalized for needle in needles):
            return _copy_ai_pricing_entry(default_ai_pricing[base_key])
    return _copy_ai_pricing_entry(default_ai_pricing[generic_default_key])


def _load_observed_ai_models(db, *, ai_span_condition: str, limit: int = 200) -> list[str]:
    safe_limit = max(1, min(int(limit), 500))
    try:
        rows = db.execute(
            "SELECT DISTINCT SpanAttributes['gen_ai.request.model'] AS model "
            "FROM otel_traces "
            f"WHERE {ai_span_condition} AND SpanAttributes['gen_ai.request.model'] != '' "
            f"ORDER BY model LIMIT {safe_limit}"
        ).fetchall()
    except Exception:
        return []
    normalized_models: list[str] = []
    seen: set[str] = set()
    for row in rows:
        model_key = _normalize_ai_model_name(row[0] if row else "")
        if model_key and model_key not in seen:
            seen.add(model_key)
            normalized_models.append(model_key)
    return normalized_models


def _load_ai_pricing_with_sources(
    db,
    *,
    default_ai_pricing: dict[str, dict[str, float]],
    generic_default_key: str,
    inference_rules: tuple[tuple[tuple[str, ...], str], ...],
    load_ai_setting,
    ai_span_condition: str,
    load_observed_ai_models_fn=None,
    load_confirmed_ai_pricing_models_fn=None,
    load_saved_ai_pricing_fn=None,
    infer_ai_pricing_for_model_fn=None,
) -> tuple[dict[str, dict[str, float]], dict[str, str]]:
    merged: dict[str, dict[str, float]] = {
        model_key: _copy_ai_pricing_entry(prices) for model_key, prices in default_ai_pricing.items()
    }
    sources: dict[str, str] = {model_key: "default" for model_key in default_ai_pricing}

    if load_observed_ai_models_fn is None:
        observed_models = _load_observed_ai_models(db, ai_span_condition=ai_span_condition)
    else:
        observed_models = load_observed_ai_models_fn(db)

    if load_confirmed_ai_pricing_models_fn is None:
        confirmed_models = _load_confirmed_ai_pricing_models(db, load_ai_setting=load_ai_setting)
    else:
        confirmed_models = load_confirmed_ai_pricing_models_fn(db)

    if load_saved_ai_pricing_fn is None:
        saved_pricing = _load_saved_ai_pricing(db, load_ai_setting=load_ai_setting)
    else:
        saved_pricing = load_saved_ai_pricing_fn(db)

    def infer_pricing(model_key: str) -> dict[str, float]:
        if infer_ai_pricing_for_model_fn is None:
            return _infer_ai_pricing_for_model(
                model_key,
                default_ai_pricing=default_ai_pricing,
                generic_default_key=generic_default_key,
                inference_rules=inference_rules,
            )
        return infer_ai_pricing_for_model_fn(model_key)

    for model_key in observed_models:
        if model_key not in merged:
            merged[model_key] = infer_pricing(model_key)
            sources[model_key] = "inferred"

    for model_key, prices in saved_pricing.items():
        merged[model_key] = prices
        if sources.get(model_key) == "inferred":
            if model_key in confirmed_models:
                sources[model_key] = "confirmed"
        elif model_key not in sources:
            sources[model_key] = "custom"

    return merged, sources


def _load_ai_pricing(
    db,
    *,
    default_ai_pricing: dict[str, dict[str, float]],
    generic_default_key: str,
    inference_rules: tuple[tuple[tuple[str, ...], str], ...],
    load_ai_setting,
    ai_span_condition: str,
    load_observed_ai_models_fn=None,
    load_confirmed_ai_pricing_models_fn=None,
    load_saved_ai_pricing_fn=None,
    infer_ai_pricing_for_model_fn=None,
) -> dict[str, dict[str, float]]:
    merged, _sources = _load_ai_pricing_with_sources(
        db,
        default_ai_pricing=default_ai_pricing,
        generic_default_key=generic_default_key,
        inference_rules=inference_rules,
        load_ai_setting=load_ai_setting,
        ai_span_condition=ai_span_condition,
        load_observed_ai_models_fn=load_observed_ai_models_fn,
        load_confirmed_ai_pricing_models_fn=load_confirmed_ai_pricing_models_fn,
        load_saved_ai_pricing_fn=load_saved_ai_pricing_fn,
        infer_ai_pricing_for_model_fn=infer_ai_pricing_for_model_fn,
    )
    return merged


def _is_sensitive_ai_setting_key(key: str, *, sensitive_setting_keys: frozenset[str]) -> bool:
    normalized = str(key or "").strip().lower()
    return normalized in sensitive_setting_keys or normalized.startswith("ai.github_token.repo.")


def _load_repo_scoped_github_token(db, owner: str, repo: str, *, load_ai_setting, github_repo_token_key) -> str:
    if not owner or not repo:
        return ""
    return load_ai_setting(db, github_repo_token_key(owner, repo), "").strip()


def _save_repo_scoped_github_token(
    db, owner: str, repo: str, token: str, *, save_ai_setting, github_repo_token_key
) -> None:
    if not owner or not repo or not token.strip():
        return
    save_ai_setting(db, github_repo_token_key(owner, repo), token.strip())
