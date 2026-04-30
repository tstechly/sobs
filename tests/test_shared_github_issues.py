from __future__ import annotations

import logging
from typing import Any

import httpx
import pytest

from shared.github_issues import (
    _classify_issue_dedupe_with_llm,
    _create_github_issue,
    _create_github_issue_record,
    _create_or_update_onboarding_issue,
    _extract_first_json_object,
    _fallback_issue_dedupe_decision,
    _fetch_open_github_issues,
    _github_api_headers,
    _github_get_issue_detail,
    _github_issue_is_new_state,
    _safe_json_loads,
    _search_open_pr_for_issue,
    _update_github_issue_record,
)


class _FakeResponse:
    def __init__(self, payload: Any, *, status_code: int = 200, content: bytes | None = None):
        self._payload = payload
        self.status_code = status_code
        self.content = b"{}" if content is None else content
        request = httpx.Request("GET", "https://github.com/test")
        if isinstance(payload, (dict, list)):
            self._response = httpx.Response(status_code, json=payload, request=request)
        elif isinstance(payload, str):
            self._response = httpx.Response(status_code, content=payload.encode(), request=request)
        else:
            self._response = httpx.Response(status_code, content=b"", request=request)

    def raise_for_status(self) -> None:
        if self.status_code >= 400:
            raise httpx.HTTPStatusError("boom", request=self._response.request, response=self._response)

    def json(self) -> Any:
        return self._payload


class _FakeClient:
    def __init__(
        self,
        *,
        get_responses: list[_FakeResponse] | None = None,
        post_responses: list[_FakeResponse] | None = None,
        patch_responses: list[_FakeResponse] | None = None,
        get_error: Exception | None = None,
        post_error: Exception | None = None,
        patch_error: Exception | None = None,
    ):
        self.get_responses = list(get_responses or [])
        self.post_responses = list(post_responses or [])
        self.patch_responses = list(patch_responses or [])
        self.get_error = get_error
        self.post_error = post_error
        self.patch_error = patch_error
        self.calls: list[tuple[str, str, dict[str, Any]]] = []

    async def get(self, url: str, **kwargs: Any) -> _FakeResponse:
        self.calls.append(("GET", url, kwargs))
        if self.get_error is not None:
            raise self.get_error
        return self.get_responses.pop(0)

    async def post(self, url: str, **kwargs: Any) -> _FakeResponse:
        self.calls.append(("POST", url, kwargs))
        if self.post_error is not None:
            raise self.post_error
        return self.post_responses.pop(0)

    async def patch(self, url: str, **kwargs: Any) -> _FakeResponse:
        self.calls.append(("PATCH", url, kwargs))
        if self.patch_error is not None:
            raise self.patch_error
        return self.patch_responses.pop(0)


def _mask(value: Any) -> str:
    return str(value).replace("ops@example.com", "****").replace("hunter2", "****")


async def _unused_client() -> _FakeClient:
    raise AssertionError("unexpected HTTP client request")


def test_github_api_headers_supports_optional_content_type_and_extra() -> None:
    headers = _github_api_headers("ghp_test", include_content_type=True, extra={"X-Test": "1"})

    assert headers["Authorization"] == "Bearer ghp_test"
    assert headers["Content-Type"] == "application/json"
    assert headers["X-Test"] == "1"


def test_safe_json_loads_handles_defaults_passthrough_and_type_mismatch() -> None:
    payload = {"classification": "same"}

    assert _safe_json_loads(None, {}) == {}
    assert _safe_json_loads(payload, {}) is payload
    assert _safe_json_loads('["a"]', []) == ["a"]
    assert _safe_json_loads('{"classification": "same"}', []) == []
    assert _safe_json_loads("not-json", {}) == {}


def test_extract_first_json_object_handles_fenced_and_embedded_json() -> None:
    assert _extract_first_json_object('```json\n{"classification": "same"}\n```') == {"classification": "same"}
    assert _extract_first_json_object('prefix {"classification": "related"} suffix') == {"classification": "related"}


def test_extract_first_json_object_returns_empty_for_empty_or_invalid_text() -> None:
    assert _extract_first_json_object("") == {}
    assert _extract_first_json_object("plain text only") == {}
    assert _extract_first_json_object("prefix [1, 2, 3] suffix") == {}
    assert _extract_first_json_object("prefix {invalid json} suffix") == {}


def test_fallback_issue_dedupe_decision_prefers_key_then_service_signal() -> None:
    proposed = {"dedup_key": "repo|svc|src|sig|warn", "service_name": "svc", "signal_name": "latency"}
    assert (
        _fallback_issue_dedupe_decision(proposed, [{"candidate_id": "1", "dedup_key": "repo|svc|src|sig|warn"}])[
            "classification"
        ]
        == "same"
    )
    assert (
        _fallback_issue_dedupe_decision(
            proposed, [{"candidate_id": "2", "service_name": "svc", "signal_name": "latency"}]
        )["classification"]
        == "related"
    )


def test_fallback_issue_dedupe_decision_returns_unrelated_without_match() -> None:
    result = _fallback_issue_dedupe_decision(
        {"service_name": "checkout", "signal_name": "latency"},
        [{"candidate_id": "3", "service_name": "billing", "signal_name": "errors"}],
    )

    assert result == {
        "classification": "unrelated",
        "candidate_id": "",
        "confidence": 0.0,
        "reason": "no strong local match",
    }


@pytest.mark.asyncio
async def test_fetch_open_github_issues_filters_pull_requests() -> None:
    client = _FakeClient(
        get_responses=[
            _FakeResponse(
                [
                    {
                        "number": 1,
                        "html_url": "https://github.com/acme/demo/issues/1",
                        "title": "A",
                        "body": "B",
                        "state": "open",
                        "assignees": [{"login": "octo"}],
                    },
                    {"number": 2, "pull_request": {}, "html_url": "https://github.com/acme/demo/pull/2"},
                ]
            )
        ]
    )

    async def _get_client() -> _FakeClient:
        return client

    issues = await _fetch_open_github_issues("ghp", "acme/demo", get_async_http_client=_get_client)
    assert issues == [
        {
            "issue_number": 1,
            "issue_url": "https://github.com/acme/demo/issues/1",
            "issue_title": "A",
            "issue_body": "B",
            "issue_state": "open",
            "assignees": ["octo"],
        }
    ]


@pytest.mark.asyncio
async def test_fetch_open_github_issues_handles_invalid_inputs_and_non_list_payload() -> None:
    client = _FakeClient(get_responses=[_FakeResponse({"items": []})])

    async def _get_client() -> _FakeClient:
        return client

    assert await _fetch_open_github_issues("", "acme/demo", get_async_http_client=_unused_client) == []
    assert await _fetch_open_github_issues("ghp", "not-a-repo", get_async_http_client=_unused_client) == []
    assert await _fetch_open_github_issues("ghp", "acme/demo", get_async_http_client=_get_client) == []


@pytest.mark.asyncio
async def test_fetch_open_github_issues_logs_and_returns_empty_on_error(caplog: pytest.LogCaptureFixture) -> None:
    client = _FakeClient(get_error=RuntimeError("boom"))
    logger = logging.getLogger("tests.shared.github_issues.fetch")

    async def _get_client() -> _FakeClient:
        return client

    with caplog.at_level(logging.WARNING, logger=logger.name):
        issues = await _fetch_open_github_issues(
            "ghp",
            "acme/demo",
            get_async_http_client=_get_client,
            logger=logger,
        )

    assert issues == []
    assert "GitHub open issue fetch failed for acme/demo: boom" in caplog.text


@pytest.mark.asyncio
async def test_search_open_pr_for_issue_returns_first_match() -> None:
    client = _FakeClient(
        get_responses=[_FakeResponse({"items": [{"number": 4, "html_url": "https://github.com/acme/demo/pull/4"}]})]
    )

    async def _get_client() -> _FakeClient:
        return client

    result = await _search_open_pr_for_issue("ghp", "acme/demo", 12, get_async_http_client=_get_client)
    assert result == {"pr_number": 4, "pr_url": "https://github.com/acme/demo/pull/4"}


@pytest.mark.asyncio
async def test_search_open_pr_for_issue_handles_invalid_inputs_and_empty_results() -> None:
    client = _FakeClient(
        get_responses=[
            _FakeResponse({"items": []}),
            _FakeResponse([]),
            _FakeResponse({"items": ["not-a-dict"]}),
        ]
    )

    async def _get_client() -> _FakeClient:
        return client

    assert await _search_open_pr_for_issue("", "acme/demo", 12, get_async_http_client=_unused_client) is None
    assert await _search_open_pr_for_issue("ghp", "not-a-repo", 12, get_async_http_client=_unused_client) is None
    assert await _search_open_pr_for_issue("ghp", "acme/demo", 0, get_async_http_client=_unused_client) is None
    assert await _search_open_pr_for_issue("ghp", "acme/demo", 12, get_async_http_client=_get_client) is None
    assert await _search_open_pr_for_issue("ghp", "acme/demo", 12, get_async_http_client=_get_client) is None
    assert await _search_open_pr_for_issue("ghp", "acme/demo", 12, get_async_http_client=_get_client) is None


@pytest.mark.asyncio
async def test_search_open_pr_for_issue_returns_none_on_transport_error() -> None:
    client = _FakeClient(get_error=RuntimeError("boom"))

    async def _get_client() -> _FakeClient:
        return client

    assert await _search_open_pr_for_issue("ghp", "acme/demo", 12, get_async_http_client=_get_client) is None


@pytest.mark.asyncio
async def test_create_github_issue_record_masks_when_enabled() -> None:
    client = _FakeClient(
        post_responses=[
            _FakeResponse(
                {"html_url": "https://github.com/acme/demo/issues/11", "number": 11, "title": "masked", "state": "open"}
            )
        ]
    )

    async def _get_client() -> _FakeClient:
        return client

    await _create_github_issue_record(
        "ghp",
        "acme/demo",
        "Issue for ops@example.com",
        "password=hunter2",
        ["security"],
        get_async_http_client=_get_client,
        mask_string_for_output=_mask,
        mask_output_enabled=True,
    )
    payload = client.calls[-1][2]["json"]
    assert payload["title"] == "Issue for ****"
    assert payload["body"] == "password=****"


@pytest.mark.asyncio
async def test_create_github_issue_record_handles_invalid_inputs_and_default_labels() -> None:
    client = _FakeClient(
        post_responses=[
            _FakeResponse(
                {"html_url": "https://github.com/acme/demo/issues/9", "number": 9, "title": "fixture", "state": "open"}
            )
        ]
    )

    async def _get_client() -> _FakeClient:
        return client

    assert (
        await _create_github_issue_record(
            "",
            "acme/demo",
            "fixture",
            "body",
            get_async_http_client=_unused_client,
            mask_string_for_output=_mask,
        )
        == {}
    )
    assert (
        await _create_github_issue_record(
            "ghp",
            "broken",
            "fixture",
            "body",
            get_async_http_client=_unused_client,
            mask_string_for_output=_mask,
        )
        == {}
    )

    result = await _create_github_issue_record(
        "ghp",
        "prefix/acme/demo",
        "fixture",
        "body",
        get_async_http_client=_get_client,
        mask_string_for_output=_mask,
        mask_output_enabled=False,
    )

    assert result["issue_number"] == 9
    payload = client.calls[-1][2]["json"]
    assert payload["title"] == "fixture"
    assert payload["labels"] == ["sobs-agent", "automated"]
    assert client.calls[-1][1] == "https://api.github.com/repos/acme/demo/issues"


@pytest.mark.asyncio
async def test_create_github_issue_record_returns_error_on_http_failure() -> None:
    client = _FakeClient(post_responses=[_FakeResponse({"message": "bad credentials"}, status_code=401)])

    async def _get_client() -> _FakeClient:
        return client

    result = await _create_github_issue_record(
        "ghp",
        "acme/demo",
        "fixture",
        "body",
        get_async_http_client=_get_client,
        mask_string_for_output=_mask,
    )
    assert result == {"error": "GitHub issue creation failed: bad credentials"}


@pytest.mark.asyncio
async def test_create_github_issue_record_returns_error_on_transport_failure(caplog: pytest.LogCaptureFixture) -> None:
    client = _FakeClient(post_error=RuntimeError("boom"))
    logger = logging.getLogger("tests.shared.github_issues.create")

    async def _get_client() -> _FakeClient:
        return client

    with caplog.at_level(logging.WARNING, logger=logger.name):
        result = await _create_github_issue_record(
            "ghp",
            "acme/demo",
            "fixture",
            "body",
            get_async_http_client=_get_client,
            mask_string_for_output=_mask,
            logger=logger,
        )

    assert result == {"error": "GitHub issue creation failed: boom"}
    assert "GitHub issue creation failed: boom" in caplog.text


@pytest.mark.asyncio
async def test_create_github_issue_delegates_to_record() -> None:
    client = _FakeClient(
        post_responses=[
            _FakeResponse(
                {
                    "html_url": "https://github.com/acme/demo/issues/10",
                    "number": 10,
                    "title": "fixture",
                    "state": "open",
                }
            )
        ]
    )

    async def _get_client() -> _FakeClient:
        return client

    issue_url = await _create_github_issue(
        "ghp",
        "acme/demo",
        "fixture",
        "body",
        ["security"],
        get_async_http_client=_get_client,
        mask_string_for_output=_mask,
    )
    assert issue_url == "https://github.com/acme/demo/issues/10"


@pytest.mark.asyncio
async def test_create_github_issue_returns_empty_string_when_record_creation_fails() -> None:
    client = _FakeClient(post_error=RuntimeError("boom"))

    async def _get_client() -> _FakeClient:
        return client

    issue_url = await _create_github_issue(
        "ghp",
        "acme/demo",
        "fixture",
        "body",
        ["security"],
        get_async_http_client=_get_client,
        mask_string_for_output=_mask,
    )

    assert issue_url == ""


@pytest.mark.asyncio
async def test_github_get_issue_detail_and_new_state() -> None:
    client = _FakeClient(
        get_responses=[
            _FakeResponse(
                {
                    "state": "open",
                    "comments": 0,
                    "created_at": "2026-01-01T00:00:00Z",
                    "updated_at": "2026-01-01T00:00:00Z",
                }
            )
        ]
    )

    async def _get_client() -> _FakeClient:
        return client

    payload = await _github_get_issue_detail("ghp", "acme/demo", 5, get_async_http_client=_get_client)
    assert _github_issue_is_new_state(payload) is True


@pytest.mark.asyncio
async def test_github_get_issue_detail_handles_invalid_inputs_non_dict_payload_and_errors() -> None:
    payload_client = _FakeClient(get_responses=[_FakeResponse([], content=b"[]")])
    error_client = _FakeClient(get_error=RuntimeError("boom"))

    async def _payload_client() -> _FakeClient:
        return payload_client

    async def _error_client() -> _FakeClient:
        return error_client

    assert await _github_get_issue_detail("", "acme/demo", 5, get_async_http_client=_unused_client) == {}
    assert await _github_get_issue_detail("ghp", "broken", 5, get_async_http_client=_unused_client) == {}
    assert await _github_get_issue_detail("ghp", "acme/demo", 0, get_async_http_client=_unused_client) == {}
    assert await _github_get_issue_detail("ghp", "acme/demo", 5, get_async_http_client=_payload_client) == {}
    assert await _github_get_issue_detail("ghp", "acme/demo", 5, get_async_http_client=_error_client) == {}


def test_github_issue_is_new_state_rejects_non_new_payloads() -> None:
    assert _github_issue_is_new_state({"state": "closed", "comments": 0, "created_at": "a", "updated_at": "a"}) is False
    assert _github_issue_is_new_state({"state": "open", "comments": 1, "created_at": "a", "updated_at": "a"}) is False
    assert _github_issue_is_new_state({"state": "open", "comments": 0, "created_at": "", "updated_at": "a"}) is False


@pytest.mark.asyncio
async def test_update_github_issue_record_normalizes_result() -> None:
    client = _FakeClient(
        patch_responses=[
            _FakeResponse(
                {"html_url": "https://github.com/acme/demo/issues/5", "number": 5, "title": "Updated", "state": "open"}
            )
        ]
    )

    async def _get_client() -> _FakeClient:
        return client

    result = await _update_github_issue_record(
        "ghp",
        "acme/demo",
        5,
        "Updated",
        "body",
        ["triage"],
        get_async_http_client=_get_client,
        mask_string_for_output=_mask,
        mask_output_enabled=False,
    )
    assert result == {
        "issue_url": "https://github.com/acme/demo/issues/5",
        "issue_number": 5,
        "issue_title": "Updated",
        "issue_state": "open",
    }


@pytest.mark.asyncio
async def test_update_github_issue_record_handles_invalid_inputs_and_omits_labels_when_none() -> None:
    client = _FakeClient(patch_responses=[_FakeResponse({}, content=b"")])

    async def _get_client() -> _FakeClient:
        return client

    assert (
        await _update_github_issue_record(
            "",
            "acme/demo",
            5,
            "Updated",
            "body",
            get_async_http_client=_unused_client,
            mask_string_for_output=_mask,
        )
        == {}
    )
    assert (
        await _update_github_issue_record(
            "ghp",
            "broken",
            5,
            "Updated",
            "body",
            get_async_http_client=_unused_client,
            mask_string_for_output=_mask,
        )
        == {}
    )
    assert (
        await _update_github_issue_record(
            "ghp",
            "acme/demo",
            0,
            "Updated",
            "body",
            get_async_http_client=_unused_client,
            mask_string_for_output=_mask,
        )
        == {}
    )

    result = await _update_github_issue_record(
        "ghp",
        "acme/demo",
        5,
        "Updated",
        "body",
        get_async_http_client=_get_client,
        mask_string_for_output=_mask,
        mask_output_enabled=False,
    )

    assert result == {
        "issue_url": "",
        "issue_number": 5,
        "issue_title": "Updated",
        "issue_state": "open",
    }
    assert "labels" not in client.calls[-1][2]["json"]


@pytest.mark.asyncio
async def test_update_github_issue_record_returns_errors_for_http_and_transport_failures(
    caplog: pytest.LogCaptureFixture,
) -> None:
    http_client = _FakeClient(patch_responses=[_FakeResponse({"message": "denied"}, status_code=401)])
    transport_client = _FakeClient(patch_error=RuntimeError("boom"))
    logger = logging.getLogger("tests.shared.github_issues.update")

    async def _http_client() -> _FakeClient:
        return http_client

    async def _transport_client() -> _FakeClient:
        return transport_client

    http_result = await _update_github_issue_record(
        "ghp",
        "acme/demo",
        5,
        "Updated",
        "body",
        get_async_http_client=_http_client,
        mask_string_for_output=_mask,
    )

    with caplog.at_level(logging.WARNING, logger=logger.name):
        transport_result = await _update_github_issue_record(
            "ghp",
            "acme/demo",
            5,
            "Updated",
            "body",
            get_async_http_client=_transport_client,
            mask_string_for_output=_mask,
            logger=logger,
        )

    assert http_result == {"error": "GitHub issue update failed: denied"}
    assert transport_result == {"error": "GitHub issue update failed: boom"}
    assert "GitHub issue update failed: boom" in caplog.text


@pytest.mark.asyncio
async def test_create_or_update_onboarding_issue_creates_when_missing() -> None:
    client = _FakeClient(
        get_responses=[_FakeResponse([])],
        post_responses=[
            _FakeResponse(
                {"html_url": "https://github.com/acme/demo/issues/21", "number": 21, "title": "Setup", "state": "open"}
            )
        ],
    )

    async def _get_client() -> _FakeClient:
        return client

    result = await _create_or_update_onboarding_issue(
        "ghp",
        "acme/demo",
        "Setup",
        "body",
        ["automation"],
        get_async_http_client=_get_client,
        mask_string_for_output=_mask,
    )
    assert result["status"] == "created"


@pytest.mark.asyncio
async def test_create_or_update_onboarding_issue_propagates_create_error_and_passes_limit() -> None:
    seen: dict[str, Any] = {}

    async def _fetch_open_issues(_token: str, _repo: str, *, limit: int) -> list[dict[str, Any]]:
        seen["limit"] = limit
        return []

    async def _create_issue(*_args: Any, **kwargs: Any) -> dict[str, Any]:
        seen["create_mask_output_enabled"] = kwargs["mask_output_enabled"]
        return {"error": "create failed"}

    result = await _create_or_update_onboarding_issue(
        "ghp",
        "acme/demo",
        "Setup",
        "body",
        ["automation"],
        get_async_http_client=_unused_client,
        mask_string_for_output=_mask,
        open_issue_limit=7,
        fetch_open_github_issues=_fetch_open_issues,
        create_github_issue_record=_create_issue,
    )

    assert result == {"error": "create failed"}
    assert seen == {"limit": 7, "create_mask_output_enabled": False}


@pytest.mark.asyncio
async def test_create_or_update_onboarding_issue_updates_when_existing_issue_is_new() -> None:
    client = _FakeClient(
        get_responses=[
            _FakeResponse(
                [
                    {
                        "number": 21,
                        "html_url": "https://github.com/acme/demo/issues/21",
                        "title": "Setup",
                        "state": "open",
                        "body": "body",
                        "assignees": [],
                    }
                ]
            ),
            _FakeResponse(
                {
                    "html_url": "https://github.com/acme/demo/issues/21",
                    "title": "Setup",
                    "state": "open",
                    "comments": 0,
                    "created_at": "2026-01-01T00:00:00Z",
                    "updated_at": "2026-01-01T00:00:00Z",
                }
            ),
        ],
        patch_responses=[
            _FakeResponse(
                {"html_url": "https://github.com/acme/demo/issues/21", "number": 21, "title": "Setup", "state": "open"}
            )
        ],
    )

    async def _get_client() -> _FakeClient:
        return client

    result = await _create_or_update_onboarding_issue(
        "ghp",
        "acme/demo",
        "Setup",
        "body",
        ["automation"],
        get_async_http_client=_get_client,
        mask_string_for_output=_mask,
    )
    assert result["status"] == "updated"


@pytest.mark.asyncio
async def test_create_or_update_onboarding_issue_propagates_update_error_with_injected_helpers() -> None:
    async def _fetch_open_issues(_token: str, _repo: str, *, limit: int) -> list[dict[str, Any]]:
        assert limit == 10
        return [
            {
                "issue_number": 21,
                "issue_url": "https://github.com/acme/demo/issues/21",
                "issue_title": "Setup",
                "issue_state": "open",
            }
        ]

    async def _get_issue_detail(_token: str, _repo: str, issue_number: int) -> dict[str, Any]:
        assert issue_number == 21
        return {"html_url": "https://github.com/acme/demo/issues/21", "title": "Setup", "state": "open"}

    async def _update_issue(*_args: Any, **kwargs: Any) -> dict[str, Any]:
        assert kwargs["mask_output_enabled"] is False
        return {"error": "update failed"}

    result = await _create_or_update_onboarding_issue(
        "ghp",
        "acme/demo",
        "Setup",
        "body",
        ["automation"],
        get_async_http_client=_unused_client,
        mask_string_for_output=_mask,
        fetch_open_github_issues=_fetch_open_issues,
        github_get_issue_detail=_get_issue_detail,
        update_github_issue_record=_update_issue,
        github_issue_is_new_state=lambda _payload: True,
    )

    assert result == {"error": "update failed"}


@pytest.mark.asyncio
async def test_create_or_update_onboarding_issue_reuses_when_existing_issue_not_new() -> None:
    client = _FakeClient(
        get_responses=[
            _FakeResponse(
                [
                    {
                        "number": 21,
                        "html_url": "https://github.com/acme/demo/issues/21",
                        "title": "Setup",
                        "state": "open",
                        "body": "body",
                        "assignees": [],
                    }
                ]
            ),
            _FakeResponse(
                {
                    "html_url": "https://github.com/acme/demo/issues/21",
                    "title": "Setup",
                    "state": "open",
                    "comments": 2,
                    "created_at": "2026-01-01T00:00:00Z",
                    "updated_at": "2026-01-02T00:00:00Z",
                }
            ),
        ],
    )

    async def _get_client() -> _FakeClient:
        return client

    result = await _create_or_update_onboarding_issue(
        "ghp",
        "acme/demo",
        "Setup",
        "body",
        ["automation"],
        get_async_http_client=_get_client,
        mask_string_for_output=_mask,
    )
    assert result["status"] == "reused"


@pytest.mark.asyncio
async def test_create_or_update_onboarding_issue_reuses_existing_metadata_when_detail_missing() -> None:
    async def _fetch_open_issues(_token: str, _repo: str, *, limit: int) -> list[dict[str, Any]]:
        assert limit == 10
        return [
            {
                "issue_number": 21,
                "issue_url": "https://github.com/acme/demo/issues/21",
                "issue_title": "Setup",
                "issue_state": "closed",
            }
        ]

    async def _get_issue_detail(_token: str, _repo: str, _issue_number: int) -> dict[str, Any]:
        return {}

    result = await _create_or_update_onboarding_issue(
        "ghp",
        "acme/demo",
        "Setup",
        "body",
        ["automation"],
        get_async_http_client=_unused_client,
        mask_string_for_output=_mask,
        fetch_open_github_issues=_fetch_open_issues,
        github_get_issue_detail=_get_issue_detail,
    )

    assert result == {
        "issue_url": "https://github.com/acme/demo/issues/21",
        "issue_number": 21,
        "issue_title": "Setup",
        "issue_state": "closed",
        "status": "reused",
        "note": "Existing onboarding issue is not in new state; left unchanged.",
    }


@pytest.mark.asyncio
async def test_classify_issue_dedupe_with_llm_uses_fallback_without_model() -> None:
    async def _unused_llm(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        return "", {}

    result = await _classify_issue_dedupe_with_llm(
        {},
        {"dedup_key": "same"},
        [{"candidate_id": "1", "dedup_key": "same"}],
        call_llm_endpoint=_unused_llm,
    )
    assert result["classification"] == "same"


@pytest.mark.asyncio
async def test_classify_issue_dedupe_with_llm_falls_back_on_invalid_classification() -> None:
    async def _fake_llm(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        return '{"classification": "bogus", "candidate_id": "2", "confidence": 0.6, "reason": "match"}', {}

    result = await _classify_issue_dedupe_with_llm(
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt", "ai.api_key": "key"},
        {"dedup_key": "same"},
        [{"candidate_id": "2", "dedup_key": "same"}],
        call_llm_endpoint=_fake_llm,
    )

    assert result["classification"] == "same"


@pytest.mark.asyncio
async def test_classify_issue_dedupe_with_llm_limits_candidates_and_handles_bad_confidence() -> None:
    seen: dict[str, Any] = {}

    async def _fake_llm(
        _endpoint: str,
        _model: str,
        _api_key: str,
        messages: list[dict[str, Any]],
        **_kwargs: Any,
    ) -> tuple[str, dict[str, Any]]:
        seen["prompt"] = messages[1]["content"]
        return '{"classification": "related", "candidate_id": "2", "confidence": "oops", "reason": "match"}', {}

    result = await _classify_issue_dedupe_with_llm(
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt", "ai.api_key": "key"},
        {"dedup_key": "x"},
        [{"candidate_id": "2"}, {"candidate_id": "3"}],
        call_llm_endpoint=_fake_llm,
        candidate_limit=1,
    )

    assert '"candidate_id": "2"' in seen["prompt"]
    assert '"candidate_id": "3"' not in seen["prompt"]
    assert result == {"classification": "related", "candidate_id": "2", "confidence": 0.0, "reason": "match"}


@pytest.mark.asyncio
async def test_classify_issue_dedupe_with_llm_parses_json_reply() -> None:
    async def _fake_llm(*_args: Any, **_kwargs: Any) -> tuple[str, dict[str, Any]]:
        return (
            '```json\n{"classification": "related", "candidate_id": "2", "confidence": 0.6, "reason": "match"}\n```',
            {},
        )

    result = await _classify_issue_dedupe_with_llm(
        {"ai.endpoint_url": "https://llm.example", "ai.model": "gpt", "ai.api_key": "key"},
        {"dedup_key": "x"},
        [{"candidate_id": "2"}],
        call_llm_endpoint=_fake_llm,
    )
    assert result == {"classification": "related", "candidate_id": "2", "confidence": 0.6, "reason": "match"}
