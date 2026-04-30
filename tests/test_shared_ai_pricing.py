import json

from shared.ai_pricing import (
    _coerce_ai_pricing_entry,
    _copy_ai_pricing_entry,
    _infer_ai_pricing_for_model,
    _is_sensitive_ai_setting_key,
    _load_ai_pricing,
    _load_ai_pricing_with_sources,
    _load_confirmed_ai_pricing_models,
    _load_observed_ai_models,
    _load_repo_scoped_github_token,
    _load_saved_ai_pricing,
    _normalize_ai_model_name,
    _save_repo_scoped_github_token,
)

DEFAULT_PRICING = {
    "gpt-4o": {"in": 2.5, "out": 10.0},
    "gpt-4o-mini": {"in": 0.15, "out": 0.6},
    "claude-3-5-sonnet": {"in": 3.0, "out": 15.0},
    "mistral-small": {"in": 0.2, "out": 0.6},
}
GENERIC_DEFAULT_KEY = "gpt-4o"
INFERENCE_RULES = (
    (("4o-mini",), "gpt-4o-mini"),
    (("sonnet",), "claude-3-5-sonnet"),
    (("mistral",), "mistral-small"),
)


class _FakeResult:
    def __init__(self, rows):
        self._rows = rows

    def fetchall(self):
        return self._rows


class _FakeDb:
    def __init__(self, rows=None, should_raise=False):
        self.rows = rows or []
        self.should_raise = should_raise
        self.queries = []

    def execute(self, query):
        self.queries.append(query)
        if self.should_raise:
            raise RuntimeError("db down")
        return _FakeResult(self.rows)


def _load_ai_setting_factory(values):
    def _load_ai_setting(_db, key, default=""):
        return values.get(key, default)

    return _load_ai_setting


def test_ai_pricing_helpers_cover_normalization_copy_and_coercion():
    assert _normalize_ai_model_name(" GPT-4O ") == "gpt-4o"
    assert _normalize_ai_model_name(None) == ""
    assert _copy_ai_pricing_entry({"in": 1, "out": "2.5"}) == {"in": 1.0, "out": 2.5}
    assert _coerce_ai_pricing_entry({"in": "1.2", "out": 3}) == {"in": 1.2, "out": 3.0}
    assert _coerce_ai_pricing_entry({"in": 1}) is None
    assert _coerce_ai_pricing_entry({"in": "bad", "out": 3}) is None
    assert _coerce_ai_pricing_entry([]) is None


def test_ai_pricing_saved_and_confirmed_loaders_cover_invalid_and_valid_inputs():
    db = object()
    load_ai_setting = _load_ai_setting_factory(
        {
            "ai.model_pricing": json.dumps(
                {
                    " GPT-4O ": {"in": "1.0", "out": 4},
                    "bad-model": {"in": "bad", "out": 3},
                    "": {"in": 1, "out": 2},
                }
            ),
            "ai.model_pricing_confirmed": json.dumps([" GPT-4O ", "", None, "custom-model"]),
        }
    )

    assert _load_saved_ai_pricing(db, load_ai_setting=load_ai_setting) == {"gpt-4o": {"in": 1.0, "out": 4.0}}
    assert _load_confirmed_ai_pricing_models(db, load_ai_setting=load_ai_setting) == {"gpt-4o", "custom-model"}

    bad_loader = _load_ai_setting_factory(
        {
            "ai.model_pricing": "not-json",
            "ai.model_pricing_confirmed": json.dumps({"not": "a list"}),
        }
    )
    assert _load_saved_ai_pricing(db, load_ai_setting=bad_loader) == {}
    assert _load_confirmed_ai_pricing_models(db, load_ai_setting=bad_loader) == set()

    empty_loader = _load_ai_setting_factory(
        {
            "ai.model_pricing": "",
            "ai.model_pricing_confirmed": "",
        }
    )
    assert _load_saved_ai_pricing(db, load_ai_setting=empty_loader) == {}
    assert _load_confirmed_ai_pricing_models(db, load_ai_setting=empty_loader) == set()

    non_dict_loader = _load_ai_setting_factory(
        {
            "ai.model_pricing": json.dumps(["not", "a", "dict"]),
            "ai.model_pricing_confirmed": "not-json",
        }
    )
    assert _load_saved_ai_pricing(db, load_ai_setting=non_dict_loader) == {}
    assert _load_confirmed_ai_pricing_models(db, load_ai_setting=non_dict_loader) == set()


def test_ai_pricing_inference_covers_exact_partial_rule_and_default_paths():
    assert (
        _infer_ai_pricing_for_model(
            "gpt-4o",
            default_ai_pricing=DEFAULT_PRICING,
            generic_default_key=GENERIC_DEFAULT_KEY,
            inference_rules=INFERENCE_RULES,
        )
        == DEFAULT_PRICING["gpt-4o"]
    )
    assert (
        _infer_ai_pricing_for_model(
            "gpt-4o-audio-preview",
            default_ai_pricing=DEFAULT_PRICING,
            generic_default_key=GENERIC_DEFAULT_KEY,
            inference_rules=INFERENCE_RULES,
        )
        == DEFAULT_PRICING["gpt-4o"]
    )
    assert (
        _infer_ai_pricing_for_model(
            "claude-3-7-sonnet",
            default_ai_pricing=DEFAULT_PRICING,
            generic_default_key=GENERIC_DEFAULT_KEY,
            inference_rules=INFERENCE_RULES,
        )
        == DEFAULT_PRICING["claude-3-5-sonnet"]
    )
    assert (
        _infer_ai_pricing_for_model(
            "brand-new-model",
            default_ai_pricing=DEFAULT_PRICING,
            generic_default_key=GENERIC_DEFAULT_KEY,
            inference_rules=INFERENCE_RULES,
        )
        == DEFAULT_PRICING["gpt-4o"]
    )
    assert (
        _infer_ai_pricing_for_model(
            "",
            default_ai_pricing=DEFAULT_PRICING,
            generic_default_key=GENERIC_DEFAULT_KEY,
            inference_rules=INFERENCE_RULES,
        )
        == DEFAULT_PRICING["gpt-4o"]
    )


def test_ai_pricing_observed_models_loader_covers_limit_query_dedupe_and_failure():
    db = _FakeDb(rows=[(" GPT-4O ",), ("gpt-4o",), ("claude-3-7-sonnet",), ("",), None])
    models = _load_observed_ai_models(db, ai_span_condition="ServiceName='sobs'", limit=999)
    assert models == ["gpt-4o", "claude-3-7-sonnet"]
    assert "LIMIT 500" in db.queries[0]
    assert "ServiceName='sobs'" in db.queries[0]

    db = _FakeDb(rows=[("gpt-4o",)])
    _load_observed_ai_models(db, ai_span_condition="1=1", limit=0)
    assert "LIMIT 1" in db.queries[0]

    db = _FakeDb(should_raise=True)
    assert _load_observed_ai_models(db, ai_span_condition="1=1") == []


def test_ai_pricing_merge_helpers_cover_default_inferred_confirmed_and_custom_sources():
    db = _FakeDb(rows=[("claude-3-7-sonnet",), ("brand-new-model",)])
    load_ai_setting = _load_ai_setting_factory(
        {
            "ai.model_pricing": json.dumps(
                {
                    "gpt-4o": {"in": 99, "out": 99},
                    "claude-3-7-sonnet": {"in": 4, "out": 16},
                    "custom-model": {"in": 1, "out": 2},
                }
            ),
            "ai.model_pricing_confirmed": json.dumps(["claude-3-7-sonnet"]),
        }
    )

    pricing, sources = _load_ai_pricing_with_sources(
        db,
        default_ai_pricing=DEFAULT_PRICING,
        generic_default_key=GENERIC_DEFAULT_KEY,
        inference_rules=INFERENCE_RULES,
        load_ai_setting=load_ai_setting,
        ai_span_condition="1=1",
    )

    assert pricing["gpt-4o"] == {"in": 99.0, "out": 99.0}
    assert pricing["claude-3-7-sonnet"] == {"in": 4.0, "out": 16.0}
    assert pricing["brand-new-model"] == DEFAULT_PRICING["gpt-4o"]
    assert pricing["custom-model"] == {"in": 1.0, "out": 2.0}
    assert sources["gpt-4o"] == "default"
    assert sources["claude-3-7-sonnet"] == "confirmed"
    assert sources["brand-new-model"] == "inferred"
    assert sources["custom-model"] == "custom"

    merged = _load_ai_pricing(
        db,
        default_ai_pricing=DEFAULT_PRICING,
        generic_default_key=GENERIC_DEFAULT_KEY,
        inference_rules=INFERENCE_RULES,
        load_ai_setting=load_ai_setting,
        ai_span_condition="1=1",
    )
    assert merged == pricing


def test_ai_pricing_merge_helpers_cover_injected_app_wrapper_paths():
    observed_calls = []
    confirmed_calls = []
    saved_calls = []
    inferred_calls = []

    def _observed_loader(db):
        observed_calls.append(db)
        return ["wrapped-model"]

    def _confirmed_loader(db):
        confirmed_calls.append(db)
        return {"wrapped-model"}

    def _saved_loader(db):
        saved_calls.append(db)
        return {"wrapped-model": {"in": 5.0, "out": 6.0}}

    def _infer_loader(model):
        inferred_calls.append(model)
        return {"in": 1.5, "out": 2.5}

    db = object()
    pricing, sources = _load_ai_pricing_with_sources(
        db,
        default_ai_pricing=DEFAULT_PRICING,
        generic_default_key=GENERIC_DEFAULT_KEY,
        inference_rules=INFERENCE_RULES,
        load_ai_setting=_load_ai_setting_factory({}),
        ai_span_condition="1=1",
        load_observed_ai_models_fn=_observed_loader,
        load_confirmed_ai_pricing_models_fn=_confirmed_loader,
        load_saved_ai_pricing_fn=_saved_loader,
        infer_ai_pricing_for_model_fn=_infer_loader,
    )
    assert pricing["wrapped-model"] == {"in": 5.0, "out": 6.0}
    assert sources["wrapped-model"] == "confirmed"
    assert observed_calls == [db]
    assert confirmed_calls == [db]
    assert saved_calls == [db]
    assert inferred_calls == ["wrapped-model"]


def test_ai_pricing_sensitive_key_and_repo_token_helpers_cover_expected_paths():
    assert _is_sensitive_ai_setting_key(
        "ai.api_key", sensitive_setting_keys=frozenset({"ai.api_key", "ai.github_token"})
    )
    assert _is_sensitive_ai_setting_key(
        "ai.github_token.repo.owner.repo", sensitive_setting_keys=frozenset({"ai.api_key", "ai.github_token"})
    )
    assert not _is_sensitive_ai_setting_key(
        "ai.model", sensitive_setting_keys=frozenset({"ai.api_key", "ai.github_token"})
    )

    values = {"repo:owner/name": " token "}
    load_ai_setting = _load_ai_setting_factory(values)

    def _repo_key(owner, repo):
        return f"repo:{owner}/{repo}"

    assert (
        _load_repo_scoped_github_token(
            object(),
            "owner",
            "name",
            load_ai_setting=load_ai_setting,
            github_repo_token_key=_repo_key,
        )
        == "token"
    )
    assert (
        _load_repo_scoped_github_token(
            object(),
            "",
            "name",
            load_ai_setting=load_ai_setting,
            github_repo_token_key=_repo_key,
        )
        == ""
    )

    saved = []

    def _save_ai_setting(_db, key, value):
        saved.append((key, value))

    _save_repo_scoped_github_token(
        object(),
        "owner",
        "name",
        " token ",
        save_ai_setting=_save_ai_setting,
        github_repo_token_key=_repo_key,
    )
    _save_repo_scoped_github_token(
        object(),
        "owner",
        "",
        " token ",
        save_ai_setting=_save_ai_setting,
        github_repo_token_key=_repo_key,
    )
    _save_repo_scoped_github_token(
        object(),
        "owner",
        "name",
        "   ",
        save_ai_setting=_save_ai_setting,
        github_repo_token_key=_repo_key,
    )
    assert saved == [("repo:owner/name", "token")]
