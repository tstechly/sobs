import httpx
import pytest

from shared.ai_runtime import (
    _build_llama_guard_prompt,
    _build_oss_safeguard_prompt,
    _call_llm_endpoint,
    _check_dlp_endpoint,
    _check_guard_model,
    _coerce_llm_content,
    _extract_stream_delta,
    _extract_stream_finish_reason,
    _extract_stream_tool_call_deltas,
    _heuristic_guard_check,
    _is_benign_ai_usage_question,
    _is_benign_observability_question,
    _is_benign_ui_navigation_request,
    _is_gpt_oss_safeguard_model,
    _llm_chat_completions_url,
    _llm_reasoning_payload,
    _llm_request_headers,
    _llm_usage_stats,
    _model_supports_thinking,
    _model_supports_tools,
    _normalize_thinking_level,
    _parse_guard_reply,
    _parse_oss_safeguard_reply,
    _resolve_endpoint_timeout_seconds,
    _resolve_guard_max_tokens,
    _resolve_guard_thinking_level,
    _resolve_guard_timeout_seconds,
    _stream_llm_endpoint,
)

THINKING_LEVELS = ("off", "low", "medium", "high")
GUARD_CATEGORIES = {"S1": "Violence", "S2": "Crime", "S8": "Privacy"}
HIGH_RISK = frozenset({"exploit", "exfiltrate", "weapon"})
BENIGN_OBSERVABILITY = frozenset({"error", "spike", "latency", "service", "deployment", "traces"})
USAGE_INTENT = frozenset({"list", "show", "count"})
USAGE_ANALYTICS = frozenset({"gpt", "model", "calls", "usage"})
NAV_INTENT = frozenset({"navigate", "open", "go"})
NAV_SURFACE = frozenset({"page", "screen", "modal", "tab"})
GUARD_BLOCK_KEYWORDS = frozenset({"ignore previous instructions", "reveal all data"})
GUARD_NOISY = frozenset({"S1", "S2", "S8"})


class _Logger:
    def __init__(self):
        self.warnings = []
        self.infos = []

    def warning(self, *args):
        self.warnings.append(args)

    def info(self, *args):
        self.infos.append(args)


class _SpanEmitter:
    def __init__(self):
        self.calls = []

    async def __call__(self, **kwargs):
        self.calls.append(kwargs)


_emit_span = _SpanEmitter()


async def _maybe_await(value):
    if hasattr(value, "__await__"):
        return await value
    return value


def test_ai_runtime_pure_helpers_cover_normalization_reasoning_and_stream_parsing():
    assert _llm_chat_completions_url("https://api.example.com/v1/") == "https://api.example.com/v1/chat/completions"
    assert (
        _llm_chat_completions_url("https://api.example.com/chat/completions")
        == "https://api.example.com/chat/completions"
    )
    assert _llm_request_headers("")["Authorization"] == "Bearer no-key"
    assert _normalize_thinking_level("HIGH", thinking_levels=THINKING_LEVELS) == "high"
    assert _normalize_thinking_level("weird", thinking_levels=THINKING_LEVELS) == "off"
    assert _model_supports_thinking("gpt-oss-120b") is True
    assert _model_supports_thinking("") is False
    assert _model_supports_tools("mistral-instruct") is True
    assert _model_supports_tools("") is False
    assert _llm_reasoning_payload("gpt-oss-120b", "low", thinking_levels=THINKING_LEVELS) == {
        "reasoning": {"effort": "low"},
        "reasoning_effort": "low",
    }
    assert _llm_reasoning_payload("plain-model", "low", thinking_levels=THINKING_LEVELS) == {}
    assert _llm_usage_stats({"prompt_tokens": 3, "completion_tokens": 4, "reasoning_tokens": 2}, 9) == {
        "prompt_tokens": 3,
        "completion_tokens": 4,
        "thinking_tokens": 2,
        "elapsed_ms": 9,
    }
    assert _llm_usage_stats({"output_tokens_details": {"reasoning_tokens": 5}}, 7)["thinking_tokens"] == 5

    event = {
        "choices": [
            {
                "delta": {"tool_calls": [{"index": 1, "function": {"name": "tool", "arguments": '{"a":1}'}}]},
                "finish_reason": "tool_calls",
            }
        ]
    }
    assert _extract_stream_tool_call_deltas(event) == [{"index": 1, "name": "tool", "arguments": '{"a":1}'}]
    assert _extract_stream_finish_reason(event) == "tool_calls"
    assert _coerce_llm_content([{"text": "hello"}, {"text": " world"}]) == "hello world"
    assert _extract_stream_delta({"choices": [{"delta": {"content": [{"text": "hi"}]}}]}) == "hi"
    assert _extract_stream_delta({"choices": [{"message": {"content": "fallback"}}]}) == "fallback"


def test_ai_runtime_guard_prompt_parsing_and_timeout_helpers_cover_branches():
    system_msg, messages, retry = _build_llama_guard_prompt("show errors", "/logs", guard_categories=GUARD_CATEGORIES)
    assert "Follow the prompt format exactly" in system_msg
    assert "<BEGIN UNSAFE CONTENT CATEGORIES>" in messages[1]["content"]
    assert "Context: /logs" in messages[1]["content"]
    assert "safe or unsafe" in retry

    system_msg, messages, retry = _build_oss_safeguard_prompt("show errors", "/logs")
    assert "## OUTPUT FORMAT" in system_msg
    assert messages[1]["content"].startswith("Context: /logs")
    assert "valid JSON" in retry

    assert _parse_guard_reply("unsafe\nS2", strict=True) == ("UNSAFE", "S2")
    assert _parse_guard_reply("benign and safe", strict=False) == ("SAFE", "")
    assert _parse_guard_reply("nonsense", strict=True) == ("", "")
    assert _parse_oss_safeguard_reply('{"violation":1,"policy_category":"H2.f"}', strict=True) == ("UNSAFE", "H2.f")
    assert _parse_oss_safeguard_reply('{"violation":0,"policy_category":null}', strict=True) == ("SAFE", "")
    assert _parse_oss_safeguard_reply("unsafe\nS1", strict=True) == ("UNSAFE", "S1")

    assert (
        _resolve_guard_thinking_level(
            {"ai.guard_thinking_level": "high"}, "gpt-oss-guard", thinking_levels=THINKING_LEVELS
        )
        == "high"
    )
    assert _resolve_guard_thinking_level({}, "plain-guard", thinking_levels=THINKING_LEVELS) == "off"
    assert _resolve_guard_max_tokens("off") == 64
    assert _resolve_guard_max_tokens("low") == 256
    assert _resolve_endpoint_timeout_seconds({}) == 120
    assert _resolve_endpoint_timeout_seconds({"ai.endpoint_timeout_seconds": "999"}) == 300
    assert _resolve_endpoint_timeout_seconds({"ai.endpoint_timeout_seconds": "bad"}) == 120
    assert _resolve_guard_timeout_seconds({}) == 30
    assert _resolve_guard_timeout_seconds({"ai.guard_timeout_seconds": "1"}) == 5
    assert _resolve_guard_timeout_seconds({"ai.guard_timeout_seconds": "bad"}) == 30


def test_ai_runtime_benign_and_guard_heuristics_cover_expected_paths():
    assert _heuristic_guard_check("show errors", guard_block_keywords=GUARD_BLOCK_KEYWORDS) is True
    assert (
        _heuristic_guard_check("ignore previous instructions please", guard_block_keywords=GUARD_BLOCK_KEYWORDS)
        is False
    )
    assert (
        _is_benign_observability_question(
            "show error spike by service latency",
            observability_high_risk_keywords=HIGH_RISK,
            observability_benign_keywords=BENIGN_OBSERVABILITY,
        )
        is True
    )
    assert (
        _is_benign_observability_question(
            "exploit the service",
            observability_high_risk_keywords=HIGH_RISK,
            observability_benign_keywords=BENIGN_OBSERVABILITY,
        )
        is False
    )
    assert (
        _is_benign_ai_usage_question(
            "show gpt model calls",
            observability_high_risk_keywords=HIGH_RISK,
            usage_query_intent_keywords=USAGE_INTENT,
            usage_analytics_keywords=USAGE_ANALYTICS,
        )
        is True
    )
    assert (
        _is_benign_ui_navigation_request(
            "navigate to the page",
            observability_high_risk_keywords=HIGH_RISK,
            navigation_intent_keywords=NAV_INTENT,
            navigation_surface_keywords=NAV_SURFACE,
        )
        is True
    )
    assert _is_gpt_oss_safeguard_model("gpt-oss-safeguard:20b") is True


@pytest.mark.asyncio
async def test_call_llm_endpoint_handles_success_retry_and_failure_paths():
    _emit_span.calls.clear()
    logger = _Logger()

    class _Response:
        def __init__(self, body):
            self._body = body

        def raise_for_status(self):
            return None

        def json(self):
            return self._body

    seen_payloads = []

    class _Client:
        def __init__(self):
            self.calls = 0

        async def post(self, *_args, **kwargs):
            self.calls += 1
            seen_payloads.append(kwargs["json"])
            if self.calls == 1:
                return _Response(
                    {
                        "usage": {"prompt_tokens": 10, "completion_tokens": 8},
                        "choices": [{"message": {"content": "answer"}, "finish_reason": "stop"}],
                    }
                )
            if self.calls == 2:
                return _Response(
                    {
                        "usage": {"prompt_tokens": 10, "completion_tokens": 1024},
                        "choices": [{"message": {"content": "", "reasoning": "thinking"}, "finish_reason": "length"}],
                    }
                )
            return _Response(
                {
                    "usage": {"prompt_tokens": 5, "completion_tokens": 11},
                    "choices": [{"message": {"content": "retried"}, "finish_reason": "stop"}],
                }
            )

    client = _Client()

    async def _get_client():
        return client

    reply, stats = await _call_llm_endpoint(
        "https://api.example.com/v1",
        "gpt-oss-120b",
        "token",
        [{"role": "user", "content": "hi"}],
        thinking_levels=THINKING_LEVELS,
        get_async_http_client=_get_client,
        emit_internal_genai_span=_emit_span,
        logger=logger,
    )
    assert reply == "answer"
    assert stats["completion_tokens"] == 8

    reply, stats = await _call_llm_endpoint(
        "https://api.example.com/v1",
        "gpt-oss-120b",
        "token",
        [{"role": "user", "content": "retry me"}],
        thinking_levels=THINKING_LEVELS,
        get_async_http_client=_get_client,
        emit_internal_genai_span=_emit_span,
        logger=logger,
        max_tokens=1024,
    )
    assert reply == "retried"
    assert seen_payloads[2]["max_tokens"] == 2048

    class _FailClient:
        async def post(self, *_args, **_kwargs):
            raise RuntimeError("boom")

    async def _get_fail_client():
        return _FailClient()

    reply, stats = await _call_llm_endpoint(
        "https://api.example.com/v1",
        "gpt-oss-120b",
        "token",
        [{"role": "user", "content": "fail"}],
        thinking_levels=THINKING_LEVELS,
        get_async_http_client=_get_fail_client,
        emit_internal_genai_span=_emit_span,
        logger=logger,
    )
    assert reply == ""
    assert "boom" in str(stats["error"])
    assert _emit_span.calls[-1]["error_type"] == "RuntimeError"


@pytest.mark.asyncio
async def test_stream_llm_endpoint_handles_deltas_and_tool_calls():
    _emit_span.calls.clear()

    class _StreamResponse:
        def raise_for_status(self):
            return None

        async def aiter_lines(self):
            yield 'data: {"choices":[{"delta":{"content":"Hello "}}]}'
            yield (
                'data: {"choices":[{"delta":{"tool_calls":[{"index":0,'
                '"function":{"name":"tool","arguments":"{\\"a\\":1}"}}]},'
                '"finish_reason":"tool_calls"}]}'
            )
            yield (
                'data: {"usage":{"prompt_tokens":9,"completion_tokens":4},'
                '"choices":[{"delta":{"content":"world"},"finish_reason":"stop"}]}'
            )
            yield "data: [DONE]"

    class _StreamContext:
        async def __aenter__(self):
            return _StreamResponse()

        async def __aexit__(self, _exc_type, _exc, _tb):
            return False

    class _Client:
        def stream(self, *_args, **_kwargs):
            return _StreamContext()

    async def _get_client():
        return _Client()

    events = []
    async for event in _stream_llm_endpoint(
        "https://api.example.com/v1",
        "gpt-oss-120b",
        "token",
        [{"role": "user", "content": "hi"}],
        thinking_levels=THINKING_LEVELS,
        get_async_http_client=_get_client,
        emit_internal_genai_span=_emit_span,
    ):
        events.append(event)

    assert [event["text"] for event in events if event["type"] == "delta"] == ["Hello ", "world"]
    tool_event = next(event for event in events if event["type"] == "tool")
    assert tool_event["tool_call"]["name"] == "tool"
    assert tool_event["tool_call"]["arguments"] == {"a": 1}
    done_event = next(event for event in events if event["type"] == "done")
    assert done_event["stats"]["prompt_tokens"] == 9
    assert _emit_span.calls[-1]["output_messages"][0]["content"] == "Hello world"


@pytest.mark.asyncio
async def test_check_guard_model_covers_safe_blocked_override_and_unavailable_paths():
    logger = _Logger()

    async def _safe_llm(*_args, **_kwargs):
        return "safe", {}

    allowed, reason, stats = await _check_guard_model(
        {"ai.guard_endpoint_url": "https://guard.example.com", "ai.guard_model": "llama-guard", "ai.api_key": ""},
        "show me recent errors",
        "/logs",
        thinking_levels=THINKING_LEVELS,
        guard_block_keywords=GUARD_BLOCK_KEYWORDS,
        guard_noisy_categories=GUARD_NOISY,
        guard_categories=GUARD_CATEGORIES,
        observability_high_risk_keywords=HIGH_RISK,
        observability_benign_keywords=BENIGN_OBSERVABILITY,
        usage_query_intent_keywords=USAGE_INTENT,
        usage_analytics_keywords=USAGE_ANALYTICS,
        navigation_intent_keywords=NAV_INTENT,
        navigation_surface_keywords=NAV_SURFACE,
        call_llm_endpoint=_safe_llm,
        maybe_await=_maybe_await,
        logger=logger,
    )
    assert allowed is True
    assert reason == "allowed"
    assert "system_instructions" in stats

    async def _blocked_llm(*_args, **_kwargs):
        return "unsafe\nS2", {}

    allowed, reason, _stats = await _check_guard_model(
        {"ai.guard_endpoint_url": "https://guard.example.com", "ai.guard_model": "llama-guard", "ai.api_key": ""},
        "show error spike by service latency",
        "/logs",
        thinking_levels=THINKING_LEVELS,
        guard_block_keywords=GUARD_BLOCK_KEYWORDS,
        guard_noisy_categories=GUARD_NOISY,
        guard_categories=GUARD_CATEGORIES,
        observability_high_risk_keywords=HIGH_RISK,
        observability_benign_keywords=BENIGN_OBSERVABILITY,
        usage_query_intent_keywords=USAGE_INTENT,
        usage_analytics_keywords=USAGE_ANALYTICS,
        navigation_intent_keywords=NAV_INTENT,
        navigation_surface_keywords=NAV_SURFACE,
        call_llm_endpoint=_blocked_llm,
        maybe_await=_maybe_await,
        logger=logger,
    )
    assert allowed is True
    assert reason == "allowed"

    async def _safeguard_llm(*_args, **_kwargs):
        return '{"violation":1,"policy_category":"H2.f","rule_ids":["H2.f"]}', {}

    allowed, reason, _stats = await _check_guard_model(
        {
            "ai.guard_endpoint_url": "https://guard.example.com",
            "ai.guard_model": "gpt-oss-safeguard:20b",
            "ai.api_key": "",
        },
        "show me recent errors",
        "",
        thinking_levels=THINKING_LEVELS,
        guard_block_keywords=GUARD_BLOCK_KEYWORDS,
        guard_noisy_categories=GUARD_NOISY,
        guard_categories=GUARD_CATEGORIES,
        observability_high_risk_keywords=HIGH_RISK,
        observability_benign_keywords=BENIGN_OBSERVABILITY,
        usage_query_intent_keywords=USAGE_INTENT,
        usage_analytics_keywords=USAGE_ANALYTICS,
        navigation_intent_keywords=NAV_INTENT,
        navigation_surface_keywords=NAV_SURFACE,
        call_llm_endpoint=_safeguard_llm,
        maybe_await=_maybe_await,
        logger=logger,
    )
    assert allowed is False
    assert reason == "blocked (policy_category=H2.f)"

    async def _empty_llm(*_args, **_kwargs):
        return "", {"error": "no verdict here"}

    allowed, reason, _stats = await _check_guard_model(
        {"ai.guard_endpoint_url": "https://guard.example.com", "ai.guard_model": "llama-guard", "ai.api_key": ""},
        "show me recent errors",
        "",
        thinking_levels=THINKING_LEVELS,
        guard_block_keywords=GUARD_BLOCK_KEYWORDS,
        guard_noisy_categories=GUARD_NOISY,
        guard_categories=GUARD_CATEGORIES,
        observability_high_risk_keywords=HIGH_RISK,
        observability_benign_keywords=BENIGN_OBSERVABILITY,
        usage_query_intent_keywords=USAGE_INTENT,
        usage_analytics_keywords=USAGE_ANALYTICS,
        navigation_intent_keywords=NAV_INTENT,
        navigation_surface_keywords=NAV_SURFACE,
        call_llm_endpoint=_empty_llm,
        maybe_await=_maybe_await,
        logger=logger,
    )
    assert allowed is False
    assert reason == "guard_unavailable"


@pytest.mark.asyncio
async def test_check_dlp_endpoint_covers_skipped_flagged_and_unavailable():
    logger = _Logger()

    clean, detail = await _check_dlp_endpoint("", "text", get_async_http_client=None, logger=logger)
    assert (clean, detail) == (True, "skipped")

    class _Response:
        def __init__(self, body):
            self._body = body

        def raise_for_status(self):
            return None

        def json(self):
            return self._body

    class _Client:
        async def post(self, *_args, **_kwargs):
            return _Response({"flagged": True, "detail": "pii"})

    async def _get_client():
        return _Client()

    clean, detail = await _check_dlp_endpoint(
        "https://dlp.example.com",
        "secret",
        "token",
        get_async_http_client=_get_client,
        logger=logger,
    )
    assert (clean, detail) == (False, "pii")

    class _FailClient:
        async def post(self, *_args, **_kwargs):
            raise httpx.HTTPError("down")

    async def _get_fail_client():
        return _FailClient()

    clean, detail = await _check_dlp_endpoint(
        "https://dlp.example.com",
        "secret",
        "token",
        get_async_http_client=_get_fail_client,
        logger=logger,
    )
    assert (clean, detail) == (True, "dlp_unavailable")


def test_ai_runtime_pure_helper_edge_cases_cover_remaining_small_branches():
    assert _extract_stream_tool_call_deltas({}) == []
    assert _extract_stream_tool_call_deltas({"choices": [{"delta": {"tool_calls": "bad"}}]}) == []
    assert _extract_stream_tool_call_deltas(
        {"choices": [{"delta": {"tool_calls": ["bad", {"function": {"name": "x", "arguments": 3}}]}}]}
    ) == [{"index": 0, "name": "x", "arguments": "3"}]
    assert _extract_stream_finish_reason({}) == ""
    assert _coerce_llm_content([{"text": "ok"}, "skip", {"text": 4}, {}]) == "ok"
    assert _coerce_llm_content(None) == ""
    assert _extract_stream_delta({}) == ""
    assert _parse_guard_reply("blocked by policy", strict=False) == ("UNSAFE", "")
    assert _parse_guard_reply("this seems denied", strict=False) == ("UNSAFE", "")
    assert _parse_guard_reply("first line\ns99", strict=False) == ("", "S99")
    assert _parse_guard_reply("first line\nSX", strict=False) == ("", "SX")
    assert _parse_guard_reply("", strict=False) == ("", "")
    assert _parse_oss_safeguard_reply('{"violation":true,"rule_ids":["S8"]}', strict=True) == ("UNSAFE", "S8")
    assert _parse_oss_safeguard_reply('{"violation":"allowed","policy_category":""}', strict=True) == ("SAFE", "")
    assert _parse_oss_safeguard_reply('prefix {"violation":"blocked","rule_ids":["S2"]} suffix', strict=True) == (
        "UNSAFE",
        "S2",
    )
    assert _parse_oss_safeguard_reply("{not json}", strict=True) == ("", "")
    assert _parse_oss_safeguard_reply("", strict=True) == ("", "")
    assert (
        _resolve_guard_thinking_level(
            {"ai.guard_thinking_level": "bogus"}, "gpt-oss-guard", thinking_levels=THINKING_LEVELS
        )
        == "off"
    )
    assert (
        _is_benign_ai_usage_question(
            "exploit model usage",
            observability_high_risk_keywords=HIGH_RISK,
            usage_query_intent_keywords=USAGE_INTENT,
            usage_analytics_keywords=USAGE_ANALYTICS,
        )
        is False
    )
    assert (
        _is_benign_ui_navigation_request(
            "exploit the page",
            observability_high_risk_keywords=HIGH_RISK,
            navigation_intent_keywords=NAV_INTENT,
            navigation_surface_keywords=NAV_SURFACE,
        )
        is False
    )


@pytest.mark.asyncio
async def test_call_llm_endpoint_additional_error_and_empty_retry_paths():
    _emit_span.calls.clear()
    logger = _Logger()

    class _Response:
        def __init__(self, body):
            self._body = body

        def raise_for_status(self):
            return None

        def json(self):
            return self._body

    class _RetryEmptyClient:
        def __init__(self):
            self.calls = 0

        async def post(self, *_args, **kwargs):
            self.calls += 1
            if self.calls == 1:
                assert kwargs["json"]["max_tokens"] == 32
                return _Response(
                    {
                        "usage": {"prompt_tokens": 2, "completion_tokens": 1},
                        "choices": [{"message": {"content": ""}, "finish_reason": "stop"}],
                    }
                )
            assert kwargs["json"]["max_tokens"] == 32
            return _Response(
                {
                    "usage": {"prompt_tokens": 2, "completion_tokens": 1},
                    "choices": [
                        {
                            "message": {"content": "", "reasoning": "still thinking", "refusal": "no"},
                            "finish_reason": "stop",
                        }
                    ],
                }
            )

    retry_client = _RetryEmptyClient()

    async def _get_retry_client():
        return retry_client

    reply, stats = await _call_llm_endpoint(
        "https://api.example.com/v1",
        "plain-model",
        "token",
        [{"role": "user", "content": "retry me"}],
        thinking_levels=THINKING_LEVELS,
        get_async_http_client=_get_retry_client,
        emit_internal_genai_span=_emit_span,
        logger=logger,
        max_tokens=32,
    )
    assert reply == ""
    assert stats["retry_max_tokens"] == 32
    assert "initial: finish_reason=stop" in stats["error"]
    assert "retry: reasoning=still thinking" in stats["error"]
    assert _emit_span.calls[-1]["error_type"] == "empty_content"

    request = httpx.Request("POST", "https://api.example.com/v1/chat/completions")
    response = httpx.Response(503, request=request, text="service unavailable")

    class _HttpErrorClient:
        async def post(self, *_args, **_kwargs):
            raise httpx.HTTPStatusError("bad", request=request, response=response)

    async def _get_http_error_client():
        return _HttpErrorClient()

    reply, stats = await _call_llm_endpoint(
        "https://api.example.com/v1",
        "plain-model",
        "token",
        [{"role": "user", "content": "fail"}],
        thinking_levels=THINKING_LEVELS,
        get_async_http_client=_get_http_error_client,
        emit_internal_genai_span=_emit_span,
        logger=logger,
    )
    assert reply == ""
    assert stats["error"] == "HTTP 503: service unavailable"

    class _BrokenTextResponse:
        status_code = 500

        @property
        def text(self):
            raise RuntimeError("text failed")

    class _BrokenHttpErrorClient:
        async def post(self, *_args, **_kwargs):
            raise httpx.HTTPStatusError("bad2", request=request, response=_BrokenTextResponse())

    async def _get_broken_http_error_client():
        return _BrokenHttpErrorClient()

    reply, stats = await _call_llm_endpoint(
        "https://api.example.com/v1",
        "plain-model",
        "token",
        [{"role": "user", "content": "fail"}],
        thinking_levels=THINKING_LEVELS,
        get_async_http_client=_get_broken_http_error_client,
        emit_internal_genai_span=_emit_span,
        logger=logger,
    )
    assert reply == ""
    assert stats["error"] == "bad2"

    reply, stats = await _call_llm_endpoint(
        "",
        "",
        "token",
        [],
        thinking_levels=THINKING_LEVELS,
        get_async_http_client=_get_broken_http_error_client,
        emit_internal_genai_span=_emit_span,
        logger=logger,
    )
    assert (reply, stats) == ("", {})


@pytest.mark.asyncio
async def test_stream_llm_endpoint_edge_cases_and_error_path():
    _emit_span.calls.clear()
    seen_payloads = []

    class _StreamResponse:
        def raise_for_status(self):
            return None

        async def aiter_lines(self):
            yield ""
            yield ": keepalive"
            yield "event: message"
            yield "data:"
            yield "data: {bad json}"
            yield (
                'data: {"choices":[{"delta":{"tool_calls":[{"index":0,'
                '"function":{"name":"tool","arguments":"{"}}]}}]}'
            )
            yield (
                'data: {"choices":[{"delta":{"tool_calls":[{"index":1,'
                '"function":{"name":"late","arguments":"{"}}]}}]}'
            )
            yield 'data: {"usage":{"prompt_tokens":1,"completion_tokens":2}}'
            yield 'data: {"choices":[{"delta":{"content":"tail"}}]}'
            yield "data: [DONE]"

    class _StreamContext:
        async def __aenter__(self):
            return _StreamResponse()

        async def __aexit__(self, _exc_type, _exc, _tb):
            return False

    class _Client:
        def stream(self, *_args, **kwargs):
            seen_payloads.append(kwargs["json"])
            return _StreamContext()

    async def _get_client():
        return _Client()

    events = []
    async for event in _stream_llm_endpoint(
        "https://api.example.com/v1",
        "gpt-oss-120b",
        "token",
        [{"role": "user", "content": "hi"}],
        thinking_levels=THINKING_LEVELS,
        get_async_http_client=_get_client,
        emit_internal_genai_span=_emit_span,
        tools=[{"type": "function", "function": {"name": "tool"}}],
        thinking_level="low",
    ):
        events.append(event)

    assert seen_payloads[0]["tool_choice"] == "auto"
    assert seen_payloads[0]["reasoning"]["effort"] == "low"
    tool_events = [event for event in events if event["type"] == "tool"]
    assert tool_events[0]["tool_call"]["arguments"] == {}
    assert tool_events[1]["tool_call"]["name"] == "late"
    assert events[-1]["type"] == "done"

    empty_events = []
    async for event in _stream_llm_endpoint(
        "",
        "",
        "token",
        [],
        thinking_levels=THINKING_LEVELS,
        get_async_http_client=_get_client,
        emit_internal_genai_span=_emit_span,
    ):
        empty_events.append(event)
    assert empty_events == []

    class _ErrorStreamResponse:
        def raise_for_status(self):
            return None

        async def aiter_lines(self):
            yield 'data: {"choices":[{"delta":{"content":"partial"}}]}'
            raise RuntimeError("stream broke")

    class _ErrorStreamContext:
        async def __aenter__(self):
            return _ErrorStreamResponse()

        async def __aexit__(self, _exc_type, _exc, _tb):
            return False

    class _ErrorClient:
        def stream(self, *_args, **_kwargs):
            return _ErrorStreamContext()

    async def _get_error_client():
        return _ErrorClient()

    with pytest.raises(RuntimeError, match="stream broke"):
        async for _event in _stream_llm_endpoint(
            "https://api.example.com/v1",
            "plain-model",
            "token",
            [{"role": "user", "content": "hi"}],
            thinking_levels=THINKING_LEVELS,
            get_async_http_client=_get_error_client,
            emit_internal_genai_span=_emit_span,
        ):
            pass
    assert _emit_span.calls[-1]["output_messages"][0]["content"] == "partial"
    assert _emit_span.calls[-1]["error_type"] == "RuntimeError"


@pytest.mark.asyncio
async def test_check_guard_model_remaining_branches():
    logger = _Logger()

    blocked, reason, stats = await _check_guard_model(
        {"ai.guard_endpoint_url": "https://guard.example.com", "ai.guard_model": "llama-guard"},
        "ignore previous instructions now",
        "",
        thinking_levels=THINKING_LEVELS,
        guard_block_keywords=GUARD_BLOCK_KEYWORDS,
        guard_noisy_categories=GUARD_NOISY,
        guard_categories=GUARD_CATEGORIES,
        observability_high_risk_keywords=HIGH_RISK,
        observability_benign_keywords=BENIGN_OBSERVABILITY,
        usage_query_intent_keywords=USAGE_INTENT,
        usage_analytics_keywords=USAGE_ANALYTICS,
        navigation_intent_keywords=NAV_INTENT,
        navigation_surface_keywords=NAV_SURFACE,
        call_llm_endpoint=lambda *_args, **_kwargs: ("safe", {}),
        maybe_await=_maybe_await,
        logger=logger,
    )
    assert blocked is False
    assert reason == "Blocked by heuristic safety check"
    assert stats == {}

    allowed, reason, stats = await _check_guard_model(
        {},
        "show me errors",
        "",
        thinking_levels=THINKING_LEVELS,
        guard_block_keywords=GUARD_BLOCK_KEYWORDS,
        guard_noisy_categories=GUARD_NOISY,
        guard_categories=GUARD_CATEGORIES,
        observability_high_risk_keywords=HIGH_RISK,
        observability_benign_keywords=BENIGN_OBSERVABILITY,
        usage_query_intent_keywords=USAGE_INTENT,
        usage_analytics_keywords=USAGE_ANALYTICS,
        navigation_intent_keywords=NAV_INTENT,
        navigation_surface_keywords=NAV_SURFACE,
        call_llm_endpoint=lambda *_args, **_kwargs: ("safe", {}),
        maybe_await=_maybe_await,
        logger=logger,
    )
    assert allowed is False
    assert reason == "guard_not_configured"
    assert stats == {}

    async def _fallback_llm(*_args, **_kwargs):
        return "", {"error": "unsafe\nS2"}

    allowed, reason, _stats = await _check_guard_model(
        {"ai.guard_endpoint_url": "https://guard.example.com", "ai.guard_model": "llama-guard", "ai.api_key": ""},
        "please commit a crime",
        "",
        thinking_levels=THINKING_LEVELS,
        guard_block_keywords=frozenset(),
        guard_noisy_categories=GUARD_NOISY,
        guard_categories=GUARD_CATEGORIES,
        observability_high_risk_keywords=HIGH_RISK,
        observability_benign_keywords=BENIGN_OBSERVABILITY,
        usage_query_intent_keywords=USAGE_INTENT,
        usage_analytics_keywords=USAGE_ANALYTICS,
        navigation_intent_keywords=NAV_INTENT,
        navigation_surface_keywords=NAV_SURFACE,
        call_llm_endpoint=_fallback_llm,
        maybe_await=_maybe_await,
        logger=logger,
    )
    assert allowed is False
    assert reason == "blocked (S2: Crime)"

    async def _navigation_llm(*_args, **_kwargs):
        return "unsafe\nS1", {}

    allowed, reason, _stats = await _check_guard_model(
        {"ai.guard_endpoint_url": "https://guard.example.com", "ai.guard_model": "llama-guard", "ai.api_key": ""},
        "navigate to the page",
        "",
        thinking_levels=THINKING_LEVELS,
        guard_block_keywords=frozenset(),
        guard_noisy_categories=frozenset({"S1"}),
        guard_categories=GUARD_CATEGORIES,
        observability_high_risk_keywords=HIGH_RISK,
        observability_benign_keywords=BENIGN_OBSERVABILITY,
        usage_query_intent_keywords=USAGE_INTENT,
        usage_analytics_keywords=USAGE_ANALYTICS,
        navigation_intent_keywords=NAV_INTENT,
        navigation_surface_keywords=NAV_SURFACE,
        call_llm_endpoint=_navigation_llm,
        maybe_await=_maybe_await,
        logger=logger,
    )
    assert allowed is True
    assert reason == "allowed"

    async def _usage_llm(*_args, **_kwargs):
        return "unsafe\nS8", {}

    allowed, reason, _stats = await _check_guard_model(
        {"ai.guard_endpoint_url": "https://guard.example.com", "ai.guard_model": "llama-guard", "ai.api_key": ""},
        "show gpt usage",
        "",
        thinking_levels=THINKING_LEVELS,
        guard_block_keywords=frozenset(),
        guard_noisy_categories=frozenset(),
        guard_categories=GUARD_CATEGORIES,
        observability_high_risk_keywords=HIGH_RISK,
        observability_benign_keywords=BENIGN_OBSERVABILITY,
        usage_query_intent_keywords=USAGE_INTENT,
        usage_analytics_keywords=USAGE_ANALYTICS,
        navigation_intent_keywords=NAV_INTENT,
        navigation_surface_keywords=NAV_SURFACE,
        call_llm_endpoint=_usage_llm,
        maybe_await=_maybe_await,
        logger=logger,
    )
    assert allowed is True
    assert reason == "allowed"

    async def _code_only_llm(*_args, **_kwargs):
        return "unsafe\nH7", {}

    allowed, reason, _stats = await _check_guard_model(
        {"ai.guard_endpoint_url": "https://guard.example.com", "ai.guard_model": "llama-guard", "ai.api_key": ""},
        "do harm",
        "",
        thinking_levels=THINKING_LEVELS,
        guard_block_keywords=frozenset(),
        guard_noisy_categories=frozenset(),
        guard_categories=GUARD_CATEGORIES,
        observability_high_risk_keywords=HIGH_RISK,
        observability_benign_keywords=BENIGN_OBSERVABILITY,
        usage_query_intent_keywords=USAGE_INTENT,
        usage_analytics_keywords=USAGE_ANALYTICS,
        navigation_intent_keywords=NAV_INTENT,
        navigation_surface_keywords=NAV_SURFACE,
        call_llm_endpoint=_code_only_llm,
        maybe_await=_maybe_await,
        logger=logger,
    )
    assert allowed is False
    assert reason == "blocked"

    async def _plain_block_llm(*_args, **_kwargs):
        return "unsafe", {}

    allowed, reason, _stats = await _check_guard_model(
        {"ai.guard_endpoint_url": "https://guard.example.com", "ai.guard_model": "llama-guard", "ai.api_key": ""},
        "do harm",
        "",
        thinking_levels=THINKING_LEVELS,
        guard_block_keywords=frozenset(),
        guard_noisy_categories=frozenset(),
        guard_categories=GUARD_CATEGORIES,
        observability_high_risk_keywords=HIGH_RISK,
        observability_benign_keywords=BENIGN_OBSERVABILITY,
        usage_query_intent_keywords=USAGE_INTENT,
        usage_analytics_keywords=USAGE_ANALYTICS,
        navigation_intent_keywords=NAV_INTENT,
        navigation_surface_keywords=NAV_SURFACE,
        call_llm_endpoint=_plain_block_llm,
        maybe_await=_maybe_await,
        logger=logger,
    )
    assert allowed is False
    assert reason == "blocked"

    async def _invalid_llm(*_args, **_kwargs):
        return "maybe later", {}

    allowed, reason, _stats = await _check_guard_model(
        {"ai.guard_endpoint_url": "https://guard.example.com", "ai.guard_model": "llama-guard", "ai.api_key": ""},
        "show me errors",
        "",
        thinking_levels=THINKING_LEVELS,
        guard_block_keywords=frozenset(),
        guard_noisy_categories=frozenset(),
        guard_categories=GUARD_CATEGORIES,
        observability_high_risk_keywords=HIGH_RISK,
        observability_benign_keywords=BENIGN_OBSERVABILITY,
        usage_query_intent_keywords=USAGE_INTENT,
        usage_analytics_keywords=USAGE_ANALYTICS,
        navigation_intent_keywords=NAV_INTENT,
        navigation_surface_keywords=NAV_SURFACE,
        call_llm_endpoint=_invalid_llm,
        maybe_await=_maybe_await,
        logger=logger,
    )
    assert allowed is False
    assert reason.startswith("guard_invalid_reply:")


@pytest.mark.asyncio
async def test_check_dlp_endpoint_remaining_response_shapes_and_headers():
    logger = _Logger()
    seen = {}

    class _Response:
        def __init__(self, body):
            self._body = body

        def raise_for_status(self):
            return None

        def json(self):
            return self._body

    class _Client:
        async def post(self, url, **kwargs):
            seen["url"] = url
            seen["headers"] = kwargs["headers"]
            seen["json"] = kwargs["json"]
            return _Response({"pii_detected": True, "reason": "masked"})

    async def _get_client():
        return _Client()

    clean, detail = await _check_dlp_endpoint(
        "https://dlp.example.com/check",
        "secret",
        "token",
        get_async_http_client=_get_client,
        logger=logger,
    )
    assert clean is False
    assert detail == "masked"
    assert seen["headers"]["Authorization"] == "Bearer token"
    assert seen["json"] == {"text": "secret"}

    class _BlockedClient:
        async def post(self, *_args, **_kwargs):
            return _Response({"blocked": True})

    async def _get_blocked_client():
        return _BlockedClient()

    clean, detail = await _check_dlp_endpoint(
        "https://dlp.example.com/check",
        "secret",
        "",
        get_async_http_client=_get_blocked_client,
        logger=logger,
    )
    assert (clean, detail) == (False, "flagged")

    class _CleanClient:
        async def post(self, *_args, **_kwargs):
            return _Response({})

    async def _get_clean_client():
        return _CleanClient()

    clean, detail = await _check_dlp_endpoint(
        "https://dlp.example.com/check",
        "secret",
        "",
        get_async_http_client=_get_clean_client,
        logger=logger,
    )
    assert (clean, detail) == (True, "clean")
