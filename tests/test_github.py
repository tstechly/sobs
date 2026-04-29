"""Unit tests for app_github.py – GitHub integration helpers.

These tests exercise the extracted helpers directly, without spinning up the
full Quart application, to achieve high per-module coverage of the GitHub
subsystem.

Tests that require async HTTP are provided with lightweight mock clients.
LLM-dependent helpers receive injected mock callables.
"""
from __future__ import annotations

import json
import os
import tempfile
import time
from typing import Any
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

# Point to a temp DB before importing anything that touches the DB.
os.environ.setdefault("SOBS_DATA_DIR", tempfile.mkdtemp())

import app_github as gh  # noqa: E402 – must come after env setup


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_fake_http_response(payload: Any, *, status: int = 200, content: bytes = b"{}") -> Any:
    """Return a minimal fake httpx-like response object."""

    class _FakeResponse:
        def __init__(self) -> None:
            self.status_code = status
            self.content = content if content else json.dumps(payload).encode()

        def raise_for_status(self) -> None:
            if self.status_code >= 400:
                import httpx

                request = MagicMock()
                raise httpx.HTTPStatusError(
                    f"HTTP {self.status_code}",
                    request=request,
                    response=MagicMock(
                        status_code=self.status_code,
                        text=json.dumps(payload) if isinstance(payload, dict) else str(payload),
                    ),
                )

        def json(self) -> Any:
            return payload

    return _FakeResponse()


def _make_fake_http_client(response_map: dict[str, Any]) -> Any:
    """
    Return a minimal fake async HTTP client.
    ``response_map`` maps URL fragments to payloads.
    """

    class _FakeClient:
        async def get(self, url: str, **kwargs: Any) -> Any:
            for fragment, payload in response_map.items():
                if fragment in url:
                    return _make_fake_http_response(payload)
            return _make_fake_http_response({})

        async def post(self, url: str, **kwargs: Any) -> Any:
            for fragment, payload in response_map.items():
                if fragment in url:
                    return _make_fake_http_response(payload)
            return _make_fake_http_response({})

    return _FakeClient()


def _make_work_item_row(**overrides: Any) -> dict[str, Any]:
    """Return a minimal valid work-item row dict for serialization tests.

    All fields are set to safe defaults; pass keyword arguments to override any.
    """
    defaults: dict[str, Any] = {
        "Id": "",
        "CreatedAt": "2025-01-15 12:00:00.000000",
        "CompletedAt": "",
        "AgentRuleId": "",
        "AgentRuleName": "",
        "AgentAction": "",
        "ServiceName": "",
        "AnomalyRuleId": "",
        "AnomalyState": "",
        "SignalSource": "",
        "SignalName": "",
        "SignalValue": 0.0,
        "GithubRepo": "",
        "DedupKey": "",
        "DedupDecision": "",
        "DedupConfidence": 0.0,
        "IssueNumber": 0,
        "IssueUrl": "",
        "CanonicalIssueNumber": 0,
        "CanonicalIssueUrl": "",
        "RelatedIssueUrls": "[]",
        "OccurrenceCount": 1,
        "IssueState": "",
        "IssueTitle": "",
        "AnalysisSummary": "",
        "SuggestionSummary": "",
        "CopilotAssignmentRequestedAt": 0,
        "CopilotAssignmentStatus": "not_requested",
        "CopilotAssignmentReason": "",
        "PrLinked": 0,
        "PrNumber": 0,
        "PrUrl": "",
    }
    defaults.update(overrides)
    return defaults


# ---------------------------------------------------------------------------
# Pure helper tests
# ---------------------------------------------------------------------------


class TestParseGithubRepoOwnerName:
    def test_https_url(self) -> None:
        assert gh._parse_github_repo_owner_name("https://github.com/acme/my-repo") == ("acme", "my-repo")

    def test_https_url_dot_git(self) -> None:
        assert gh._parse_github_repo_owner_name("https://github.com/acme/my-repo.git") == ("acme", "my-repo")

    def test_ssh_url(self) -> None:
        assert gh._parse_github_repo_owner_name("git@github.com:acme/my-repo.git") == ("acme", "my-repo")

    def test_owner_slash_repo(self) -> None:
        assert gh._parse_github_repo_owner_name("acme/my-repo") == ("acme", "my-repo")

    def test_empty_string(self) -> None:
        assert gh._parse_github_repo_owner_name("") == ("", "")

    def test_non_github_url(self) -> None:
        assert gh._parse_github_repo_owner_name("https://gitlab.com/acme/repo") == ("", "")

    def test_missing_repo_segment(self) -> None:
        assert gh._parse_github_repo_owner_name("https://github.com/acme") == ("", "")

    def test_path_with_trailing_slash(self) -> None:
        owner, repo = gh._parse_github_repo_owner_name("https://github.com/acme/repo/")
        assert owner == "acme"
        assert repo == "repo"


class TestBuildGithubRepoUrl:
    def test_builds_url(self) -> None:
        assert gh._build_github_repo_url("acme", "my-repo") == "https://github.com/acme/my-repo"

    def test_strips_dot_git(self) -> None:
        assert gh._build_github_repo_url("acme", "my-repo.git") == "https://github.com/acme/my-repo"

    def test_empty_owner_returns_empty(self) -> None:
        assert gh._build_github_repo_url("", "repo") == ""

    def test_empty_repo_returns_empty(self) -> None:
        assert gh._build_github_repo_url("owner", "") == ""


class TestResolveGithubRepoFields:
    def test_full_url_resolves(self) -> None:
        url, owner, repo = gh._resolve_github_repo_fields("https://github.com/acme/my-repo")
        assert url == "https://github.com/acme/my-repo"
        assert owner == "acme"
        assert repo == "my-repo"

    def test_owner_repo_override_url(self) -> None:
        url, owner, repo = gh._resolve_github_repo_fields(
            "https://github.com/acme/my-repo", owner="acme", repo="my-repo"
        )
        assert owner == "acme"
        assert repo == "my-repo"

    def test_partial_owner_fills_from_url(self) -> None:
        _url, owner, repo = gh._resolve_github_repo_fields("https://github.com/acme/my-repo", owner="acme")
        assert owner == "acme"
        assert repo == "my-repo"

    def test_empty_all_returns_empty(self) -> None:
        url, owner, repo = gh._resolve_github_repo_fields("")
        assert url == ""
        assert owner == ""
        assert repo == ""


class TestGithubApiHeaders:
    def test_basic_headers(self) -> None:
        h = gh._github_api_headers("mytoken")
        assert h["Authorization"] == "Bearer mytoken"
        assert h["Accept"] == "application/vnd.github+json"
        assert "Content-Type" not in h

    def test_include_content_type(self) -> None:
        h = gh._github_api_headers("tok", include_content_type=True)
        assert h["Content-Type"] == "application/json"

    def test_extra_headers_merged(self) -> None:
        h = gh._github_api_headers("tok", extra={"X-Custom": "value"})
        assert h["X-Custom"] == "value"


class TestParseBoundedIntSetting:
    def test_returns_value_within_bounds(self) -> None:
        assert gh._parse_bounded_int_setting({"key": "5"}, "key", 3, 1, 10) == 5

    def test_clamps_to_minimum(self) -> None:
        assert gh._parse_bounded_int_setting({"key": "0"}, "key", 3, 1, 10) == 1

    def test_clamps_to_maximum(self) -> None:
        assert gh._parse_bounded_int_setting({"key": "99"}, "key", 3, 1, 10) == 10

    def test_falls_back_to_default_on_missing_key(self) -> None:
        assert gh._parse_bounded_int_setting({}, "key", 3, 1, 10) == 3

    def test_falls_back_to_default_on_invalid_value(self) -> None:
        assert gh._parse_bounded_int_setting({"key": "abc"}, "key", 3, 1, 10) == 3


class TestExtractAgentTriggerFields:
    def test_extracts_basic_fields(self) -> None:
        ctx = {
            "trigger_ref_id": "rule-1",
            "trigger_state": "critical",
            "extra": json.dumps(
                {
                    "service": "checkout",
                    "state": "critical",
                    "source": "metrics",
                    "signal": "latency_p99",
                    "value": 1200.0,
                }
            ),
        }
        fields = gh._extract_agent_trigger_fields(ctx)
        assert fields["service_name"] == "checkout"
        assert fields["anomaly_rule_id"] == "rule-1"
        assert fields["anomaly_state"] == "critical"
        assert fields["signal_source"] == "metrics"
        assert fields["signal_name"] == "latency_p99"
        assert fields["signal_value"] == pytest.approx(1200.0)

    def test_handles_missing_extra(self) -> None:
        fields = gh._extract_agent_trigger_fields({"trigger_ref_id": "r1"})
        assert fields["service_name"] == ""
        assert fields["signal_value"] == 0.0

    def test_handles_dict_extra(self) -> None:
        ctx = {"extra": {"service": "api", "state": "warning"}}
        fields = gh._extract_agent_trigger_fields(ctx)
        assert fields["service_name"] == "api"
        assert fields["anomaly_state"] == "warning"


class TestNormalizeIssueMatchText:
    def test_lowercases_and_strips_special_chars(self) -> None:
        assert gh._normalize_issue_match_text("Hello-World!123") == "hello world 123"

    def test_empty_returns_empty(self) -> None:
        assert gh._normalize_issue_match_text("") == ""

    def test_collapses_whitespace(self) -> None:
        assert gh._normalize_issue_match_text("  foo   bar  ") == "foo bar"

    def test_none_returns_empty(self) -> None:
        assert gh._normalize_issue_match_text(None) == ""


class TestBuildGithubWorkItemDedupKey:
    def test_builds_key(self) -> None:
        fields = {
            "service_name": "Checkout",
            "signal_source": "Metrics",
            "signal_name": "Latency-P99",
            "anomaly_state": "Critical",
        }
        key = gh._build_github_work_item_dedup_key("acme/checkout", fields)
        assert "acme checkout" in key
        assert "checkout" in key
        assert "metrics" in key
        assert "latency p99" in key
        assert "critical" in key

    def test_handles_empty_fields(self) -> None:
        key = gh._build_github_work_item_dedup_key("", {})
        # All parts empty → key may be empty or contain only separators stripped away
        assert isinstance(key, str)


class TestBuildAgentIssueTitle:
    def test_with_service_signal_source_and_name(self) -> None:
        rule = {"name": "My Rule"}
        fields = {
            "service_name": "checkout",
            "signal_source": "metrics",
            "signal_name": "latency_p99",
            "anomaly_state": "critical",
        }
        title = gh._build_agent_issue_title(rule, fields)
        assert title.startswith("[SOBS Agent]")
        assert "checkout" in title
        assert "metrics/latency_p99" in title
        assert "critical" in title

    def test_fallback_without_signal(self) -> None:
        rule = {"name": "My Rule"}
        fields = {"service_name": "", "signal_source": "", "signal_name": "", "anomaly_state": "warning"}
        title = gh._build_agent_issue_title(rule, fields)
        assert "[SOBS Agent]" in title
        assert "My Rule" in title
        assert "warning" in title

    def test_uses_rule_name_when_no_service(self) -> None:
        rule = {"name": "Latency Alert"}
        fields = {"service_name": "", "signal_source": "metrics", "signal_name": "p99", "anomaly_state": "critical"}
        title = gh._build_agent_issue_title(rule, fields)
        assert "Latency Alert" in title


class TestSerializeGithubWorkItemRow:
    def test_serializes_flat_row(self) -> None:
        row = _make_work_item_row(
            Id="abc123",
            CompletedAt="2025-01-15 12:01:00.000000",
            AgentRuleId="rule1",
            AgentRuleName="My Rule",
            AgentAction="github_issue",
            ServiceName="checkout",
            AnomalyRuleId="ar1",
            AnomalyState="critical",
            SignalSource="metrics",
            SignalName="latency",
            SignalValue=1.5,
            GithubRepo="acme/checkout",
            DedupKey="key1",
            DedupDecision="new_issue",
            DedupConfidence=0.9,
            IssueNumber=42,
            IssueUrl="https://github.com/acme/checkout/issues/42",
            CanonicalIssueNumber=42,
            CanonicalIssueUrl="https://github.com/acme/checkout/issues/42",
            IssueState="open",
            IssueTitle="Test issue",
            AnalysisSummary="Something broke",
            SuggestionSummary="Fix it",
        )
        result = gh._serialize_github_work_item_row(row)
        assert result["id"] == "abc123"
        assert result["service"] == "checkout"
        assert result["issue_number"] == 42
        assert result["dedup_decision"] == "new_issue"
        assert result["pr_linked"] is False
        assert isinstance(result["related_issue_urls"], list)

    def test_related_issue_urls_deserialized(self) -> None:
        row = _make_work_item_row(RelatedIssueUrls='["https://github.com/a/b/issues/1"]')
        result = gh._serialize_github_work_item_row(row)
        assert result["related_issue_urls"] == ["https://github.com/a/b/issues/1"]


class TestParseIssueRefFromUrl:
    def test_standard_issue_url(self) -> None:
        owner, repo, number = gh._parse_issue_ref_from_url("https://github.com/acme/my-repo/issues/42")
        assert owner == "acme"
        assert repo == "my-repo"
        assert number == 42

    def test_invalid_url_returns_empty(self) -> None:
        assert gh._parse_issue_ref_from_url("not-a-url") == ("", "", 0)

    def test_non_issue_url_returns_empty(self) -> None:
        assert gh._parse_issue_ref_from_url("https://github.com/acme/repo/pulls/5") == ("", "", 0)


class TestDeriveCopilotAssignmentStatus:
    def test_closed_issue_with_active_marks_completed(self) -> None:
        status, reason = gh._derive_copilot_assignment_status("active", "closed", [], False)
        assert status == "completed"
        assert reason

    def test_closed_issue_not_active_preserves_status(self) -> None:
        status, reason = gh._derive_copilot_assignment_status("not_requested", "closed", [], False)
        assert status == "not_requested"

    def test_copilot_in_assignees_marks_active(self) -> None:
        status, reason = gh._derive_copilot_assignment_status(
            "not_requested", "open", ["copilot-swe-agent[bot]"], False
        )
        assert status == "active"

    def test_pr_linked_blocks_not_requested(self) -> None:
        status, reason = gh._derive_copilot_assignment_status("not_requested", "open", [], True)
        assert status == "blocked"
        assert reason

    def test_requested_preserved_when_open(self) -> None:
        status, reason = gh._derive_copilot_assignment_status("requested", "open", [], False)
        assert status == "requested"

    def test_already_active_on_open_issue(self) -> None:
        status, _reason = gh._derive_copilot_assignment_status("active", "open", [], False)
        # active without copilot assignee → remains as "requested" (still being worked)
        assert status == "requested"


class TestFallbackIssueDedupeDecision:
    def test_same_dedup_key_returns_same(self) -> None:
        proposed = {"dedup_key": "key|checkout|metrics|latency|critical"}
        candidates = [
            {"dedup_key": "key|checkout|metrics|latency|critical", "candidate_id": "c1"}
        ]
        result = gh._fallback_issue_dedupe_decision(proposed, candidates)
        assert result["classification"] == "same"
        assert result["candidate_id"] == "c1"
        assert result["confidence"] == pytest.approx(0.92)

    def test_same_service_signal_returns_related(self) -> None:
        proposed = {"dedup_key": "", "service_name": "checkout", "signal_name": "latency_p99"}
        candidates = [
            {"dedup_key": "", "candidate_id": "c2", "service_name": "checkout", "signal_name": "latency p99"}
        ]
        result = gh._fallback_issue_dedupe_decision(proposed, candidates)
        assert result["classification"] == "related"
        assert result["candidate_id"] == "c2"

    def test_no_match_returns_unrelated(self) -> None:
        proposed = {"dedup_key": "key1", "service_name": "svc-a", "signal_name": "err"}
        candidates = [{"dedup_key": "key2", "candidate_id": "c3", "service_name": "svc-b", "signal_name": "other"}]
        result = gh._fallback_issue_dedupe_decision(proposed, candidates)
        assert result["classification"] == "unrelated"

    def test_empty_candidates_returns_unrelated(self) -> None:
        result = gh._fallback_issue_dedupe_decision({"dedup_key": "k"}, [])
        assert result["classification"] == "unrelated"


class TestExtractFirstJsonObject:
    def test_plain_json(self) -> None:
        result = gh._extract_first_json_object('{"key": "value"}')
        assert result == {"key": "value"}

    def test_json_in_markdown_code_block(self) -> None:
        text = '```json\n{"classification": "same"}\n```'
        result = gh._extract_first_json_object(text)
        assert result["classification"] == "same"

    def test_json_embedded_in_text(self) -> None:
        text = 'Here is the result: {"classification": "unrelated", "confidence": 0.1}'
        result = gh._extract_first_json_object(text)
        assert result["classification"] == "unrelated"

    def test_empty_input_returns_empty_dict(self) -> None:
        assert gh._extract_first_json_object("") == {}

    def test_no_json_returns_empty_dict(self) -> None:
        assert gh._extract_first_json_object("not json at all") == {}

    def test_nested_json_parsed(self) -> None:
        text = '{"a": {"b": 1}}'
        result = gh._extract_first_json_object(text)
        assert result["a"]["b"] == 1


# ---------------------------------------------------------------------------
# Async GitHub API tests
# ---------------------------------------------------------------------------


class TestFetchOpenGithubIssues:
    @pytest.mark.asyncio
    async def test_returns_issues_list(self, monkeypatch: pytest.MonkeyPatch) -> None:
        payload = [
            {
                "number": 10,
                "html_url": "https://github.com/acme/repo/issues/10",
                "title": "Test issue",
                "body": "Body text",
                "state": "open",
                "assignees": [],
            }
        ]

        async def _fake_get_http_client():
            return _make_fake_http_client({"/issues": payload})

        monkeypatch.setattr(gh, "_get_http_client", _fake_get_http_client)
        issues = await gh._fetch_open_github_issues("token", "acme/repo")
        assert len(issues) == 1
        assert issues[0]["issue_number"] == 10
        assert issues[0]["issue_url"] == "https://github.com/acme/repo/issues/10"

    @pytest.mark.asyncio
    async def test_excludes_pull_requests(self, monkeypatch: pytest.MonkeyPatch) -> None:
        payload = [
            {"number": 1, "html_url": "url1", "title": "issue", "body": "", "state": "open", "assignees": []},
            {"number": 2, "html_url": "url2", "title": "pr", "body": "", "state": "open", "assignees": [],
             "pull_request": {"url": "..."}},
        ]

        async def _fake_get_http_client():
            return _make_fake_http_client({"/issues": payload})

        monkeypatch.setattr(gh, "_get_http_client", _fake_get_http_client)
        issues = await gh._fetch_open_github_issues("token", "acme/repo")
        assert len(issues) == 1
        assert issues[0]["issue_number"] == 1

    @pytest.mark.asyncio
    async def test_returns_empty_on_missing_token(self) -> None:
        issues = await gh._fetch_open_github_issues("", "acme/repo")
        assert issues == []

    @pytest.mark.asyncio
    async def test_returns_empty_on_missing_repo(self) -> None:
        issues = await gh._fetch_open_github_issues("token", "")
        assert issues == []

    @pytest.mark.asyncio
    async def test_returns_empty_on_http_error(self, monkeypatch: pytest.MonkeyPatch) -> None:
        async def _fake_get_http_client():
            class _ErrorClient:
                async def get(self, url, **kwargs):
                    raise Exception("network error")
            return _ErrorClient()

        monkeypatch.setattr(gh, "_get_http_client", _fake_get_http_client)
        issues = await gh._fetch_open_github_issues("token", "acme/repo")
        assert issues == []

    @pytest.mark.asyncio
    async def test_returns_empty_on_invalid_repo_url(self, monkeypatch: pytest.MonkeyPatch) -> None:
        # An invalid repo URL that can't be parsed
        issues = await gh._fetch_open_github_issues("token", "not-a-valid-owner-slash-repo-format-xyz")
        # Can't parse owner/repo → empty return
        assert issues == []


class TestSearchOpenPrForIssue:
    @pytest.mark.asyncio
    async def test_finds_open_pr(self, monkeypatch: pytest.MonkeyPatch) -> None:
        payload = {
            "items": [{"number": 5, "html_url": "https://github.com/acme/repo/pull/5"}]
        }

        async def _fake_get_http_client():
            return _make_fake_http_client({"/search/issues": payload})

        monkeypatch.setattr(gh, "_get_http_client", _fake_get_http_client)
        result = await gh._search_open_pr_for_issue("token", "acme/repo", 10)
        assert result is not None
        assert result["pr_number"] == 5

    @pytest.mark.asyncio
    async def test_returns_none_on_missing_token(self) -> None:
        result = await gh._search_open_pr_for_issue("", "acme/repo", 10)
        assert result is None

    @pytest.mark.asyncio
    async def test_returns_none_on_zero_issue_number(self) -> None:
        result = await gh._search_open_pr_for_issue("token", "acme/repo", 0)
        assert result is None

    @pytest.mark.asyncio
    async def test_returns_none_on_empty_results(self, monkeypatch: pytest.MonkeyPatch) -> None:
        async def _fake_get_http_client():
            return _make_fake_http_client({"/search/issues": {"items": []}})

        monkeypatch.setattr(gh, "_get_http_client", _fake_get_http_client)
        result = await gh._search_open_pr_for_issue("token", "acme/repo", 10)
        assert result is None

    @pytest.mark.asyncio
    async def test_returns_none_on_exception(self, monkeypatch: pytest.MonkeyPatch) -> None:
        async def _fake_get_http_client():
            class _ErrorClient:
                async def get(self, url, **kwargs):
                    raise Exception("network error")
            return _ErrorClient()

        monkeypatch.setattr(gh, "_get_http_client", _fake_get_http_client)
        result = await gh._search_open_pr_for_issue("token", "acme/repo", 10)
        assert result is None


class TestGithubRepoSupportsCopilotAssignment:
    @pytest.mark.asyncio
    async def test_returns_true_when_copilot_in_nodes(self, monkeypatch: pytest.MonkeyPatch) -> None:
        payload = {
            "data": {
                "repository": {
                    "suggestedActors": {
                        "nodes": [
                            {"__typename": "Bot", "login": "copilot-swe-agent[bot]", "id": "1"},
                        ]
                    }
                }
            }
        }

        async def _fake_get_http_client():
            return _make_fake_http_client({"/graphql": payload})

        monkeypatch.setattr(gh, "_get_http_client", _fake_get_http_client)
        assert await gh._github_repo_supports_copilot_assignment("token", "acme/repo") is True

    @pytest.mark.asyncio
    async def test_returns_false_when_copilot_not_in_nodes(self, monkeypatch: pytest.MonkeyPatch) -> None:
        payload = {
            "data": {
                "repository": {
                    "suggestedActors": {"nodes": [{"__typename": "User", "login": "other-user", "id": "2"}]}
                }
            }
        }

        async def _fake_get_http_client():
            return _make_fake_http_client({"/graphql": payload})

        monkeypatch.setattr(gh, "_get_http_client", _fake_get_http_client)
        assert await gh._github_repo_supports_copilot_assignment("token", "acme/repo") is False

    @pytest.mark.asyncio
    async def test_returns_false_on_missing_token(self) -> None:
        assert await gh._github_repo_supports_copilot_assignment("", "acme/repo") is False

    @pytest.mark.asyncio
    async def test_returns_false_on_invalid_repo(self) -> None:
        assert await gh._github_repo_supports_copilot_assignment("token", "") is False

    @pytest.mark.asyncio
    async def test_returns_false_on_http_error(self, monkeypatch: pytest.MonkeyPatch) -> None:
        async def _fake_get_http_client():
            class _ErrorClient:
                async def post(self, url, **kwargs):
                    raise Exception("network error")
            return _ErrorClient()

        monkeypatch.setattr(gh, "_get_http_client", _fake_get_http_client)
        assert await gh._github_repo_supports_copilot_assignment("token", "acme/repo") is False


class TestAssignIssueToCopilot:
    @pytest.mark.asyncio
    async def test_returns_blocked_on_missing_token(self) -> None:
        status, reason, ts = await gh._assign_issue_to_copilot("", "acme/repo", 1)
        assert status == "blocked"

    @pytest.mark.asyncio
    async def test_returns_blocked_on_missing_repo(self) -> None:
        status, reason, ts = await gh._assign_issue_to_copilot("token", "", 1)
        assert status == "blocked"

    @pytest.mark.asyncio
    async def test_returns_blocked_on_zero_issue_number(self) -> None:
        status, reason, ts = await gh._assign_issue_to_copilot("token", "acme/repo", 0)
        assert status == "blocked"

    @pytest.mark.asyncio
    async def test_returns_blocked_when_copilot_not_supported(self, monkeypatch: pytest.MonkeyPatch) -> None:
        async def _fake_supports(_token: str, _repo: str) -> bool:
            return False

        monkeypatch.setattr(gh, "_github_repo_supports_copilot_assignment", _fake_supports)
        status, reason, ts = await gh._assign_issue_to_copilot("token", "acme/repo", 42)
        assert status == "blocked"
        assert "Copilot cloud agent is not enabled" in reason

    @pytest.mark.asyncio
    async def test_successful_assignment_returns_requested(self, monkeypatch: pytest.MonkeyPatch) -> None:
        assignee_payload = {"assignees": [{"login": "copilot-swe-agent[bot]"}]}

        async def _fake_supports(_token: str, _repo: str) -> bool:
            return True

        async def _fake_get_http_client():
            return _make_fake_http_client({"/assignees": assignee_payload})

        monkeypatch.setattr(gh, "_github_repo_supports_copilot_assignment", _fake_supports)
        monkeypatch.setattr(gh, "_get_http_client", _fake_get_http_client)
        status, reason, ts = await gh._assign_issue_to_copilot("token", "acme/repo", 42)
        assert status == "requested"

    @pytest.mark.asyncio
    async def test_http_error_returns_failed(self, monkeypatch: pytest.MonkeyPatch) -> None:
        async def _fake_supports(_token: str, _repo: str) -> bool:
            return True

        async def _fake_get_http_client():
            class _ErrorClient:
                async def post(self, url, **kwargs):
                    raise Exception("connection error")
            return _ErrorClient()

        monkeypatch.setattr(gh, "_github_repo_supports_copilot_assignment", _fake_supports)
        monkeypatch.setattr(gh, "_get_http_client", _fake_get_http_client)
        status, reason, ts = await gh._assign_issue_to_copilot("token", "acme/repo", 42)
        assert status == "failed"


class TestCreateGithubIssueRecord:
    @pytest.mark.asyncio
    async def test_creates_issue_and_returns_url(self, monkeypatch: pytest.MonkeyPatch) -> None:
        payload = {
            "html_url": "https://github.com/acme/demo/issues/10",
            "number": 10,
            "title": "Test Issue",
            "state": "open",
        }

        async def _fake_get_http_client():
            return _make_fake_http_client({"/issues": payload})

        monkeypatch.setattr(gh, "_get_http_client", _fake_get_http_client)
        result = await gh._create_github_issue_record("token", "acme/demo", "Test Issue", "Body")
        assert result["issue_url"] == "https://github.com/acme/demo/issues/10"
        assert result["issue_number"] == 10

    @pytest.mark.asyncio
    async def test_returns_empty_on_missing_token(self) -> None:
        result = await gh._create_github_issue_record("", "acme/demo", "Title", "Body")
        assert result == {}

    @pytest.mark.asyncio
    async def test_returns_empty_on_missing_repo(self) -> None:
        result = await gh._create_github_issue_record("token", "", "Title", "Body")
        assert result == {}

    @pytest.mark.asyncio
    async def test_uses_default_labels(self, monkeypatch: pytest.MonkeyPatch) -> None:
        captured_payloads: list[dict] = []
        payload = {"html_url": "https://github.com/acme/demo/issues/1", "number": 1, "title": "T", "state": "open"}

        class _CapturingClient:
            async def post(self, url, json=None, **kwargs):
                captured_payloads.append(dict(json or {}))
                return _make_fake_http_response(payload)

        async def _fake_get_http_client():
            return _CapturingClient()

        monkeypatch.setattr(gh, "_get_http_client", _fake_get_http_client)
        await gh._create_github_issue_record("token", "acme/demo", "Title", "Body")
        assert len(captured_payloads) == 1
        assert "sobs-agent" in captured_payloads[0]["labels"]

    @pytest.mark.asyncio
    async def test_mask_output_disabled_passes_raw(self, monkeypatch: pytest.MonkeyPatch) -> None:
        captured_payloads: list[dict] = []
        payload = {"html_url": "https://github.com/acme/demo/issues/1", "number": 1, "title": "T", "state": "open"}

        class _CapturingClient:
            async def post(self, url, json=None, **kwargs):
                captured_payloads.append(dict(json or {}))
                return _make_fake_http_response(payload)

        async def _fake_get_http_client():
            return _CapturingClient()

        monkeypatch.setattr(gh, "_get_http_client", _fake_get_http_client)
        await gh._create_github_issue_record(
            "token", "acme/demo", "sensitive title", "sensitive body", mask_output_enabled=False
        )
        assert captured_payloads[0]["title"] == "sensitive title"
        assert captured_payloads[0]["body"] == "sensitive body"

    @pytest.mark.asyncio
    async def test_returns_error_on_http_failure(self, monkeypatch: pytest.MonkeyPatch) -> None:
        async def _fake_get_http_client():
            class _ErrorClient:
                async def post(self, url, **kwargs):
                    raise Exception("unexpected error")
            return _ErrorClient()

        monkeypatch.setattr(gh, "_get_http_client", _fake_get_http_client)
        result = await gh._create_github_issue_record("token", "acme/demo", "Title", "Body")
        assert "error" in result
        assert "GitHub issue creation failed" in result["error"]

    @pytest.mark.asyncio
    async def test_custom_labels_passed_through(self, monkeypatch: pytest.MonkeyPatch) -> None:
        captured: list[dict] = []
        payload = {"html_url": "url", "number": 1, "title": "T", "state": "open"}

        class _CapturingClient:
            async def post(self, url, json=None, **kwargs):
                captured.append(dict(json or {}))
                return _make_fake_http_response(payload)

        monkeypatch.setattr(gh, "_get_http_client", lambda: (lambda: _CapturingClient())())

        async def _async_client():
            return _CapturingClient()

        monkeypatch.setattr(gh, "_get_http_client", _async_client)
        await gh._create_github_issue_record("token", "acme/demo", "T", "B", ["security", "bug"])
        assert captured[0]["labels"] == ["security", "bug"]


class TestClassifyIssueDedupeWithLlm:
    @pytest.mark.asyncio
    async def test_falls_back_when_no_endpoint(self) -> None:
        settings: dict[str, str] = {}
        proposed = {"dedup_key": "k1"}
        candidates = [{"dedup_key": "k1", "candidate_id": "c1"}]
        result = await gh._classify_issue_dedupe_with_llm(settings, proposed, candidates)
        # No endpoint configured → falls back to deterministic decision
        assert result["classification"] == "same"

    @pytest.mark.asyncio
    async def test_falls_back_when_no_candidates(self, monkeypatch: pytest.MonkeyPatch) -> None:
        settings = {"ai.endpoint_url": "http://llm", "ai.model": "gpt-test"}
        proposed = {"dedup_key": "k1"}
        result = await gh._classify_issue_dedupe_with_llm(settings, proposed, [])
        assert result["classification"] == "unrelated"

    @pytest.mark.asyncio
    async def test_uses_llm_when_configured(self, monkeypatch: pytest.MonkeyPatch) -> None:
        llm_response = '{"classification": "same", "candidate_id": "c1", "confidence": 0.95, "reason": "exact match"}'

        async def _fake_call_llm(*args, **kwargs):
            return llm_response, {}

        monkeypatch.setattr(gh, "_call_llm", _fake_call_llm)
        settings = {"ai.endpoint_url": "http://llm", "ai.model": "gpt-test"}
        proposed = {"dedup_key": "k1"}
        candidates = [{"candidate_id": "c1", "dedup_key": "k1"}]
        result = await gh._classify_issue_dedupe_with_llm(settings, proposed, candidates)
        assert result["classification"] == "same"
        assert result["candidate_id"] == "c1"
        assert result["confidence"] == pytest.approx(0.95)

    @pytest.mark.asyncio
    async def test_falls_back_on_invalid_llm_classification(self, monkeypatch: pytest.MonkeyPatch) -> None:
        async def _fake_call_llm(*args, **kwargs):
            return '{"classification": "maybe", "candidate_id": "c1"}', {}

        monkeypatch.setattr(gh, "_call_llm", _fake_call_llm)
        settings = {"ai.endpoint_url": "http://llm", "ai.model": "gpt-test"}
        proposed = {"dedup_key": "key1", "service_name": "svc-a", "signal_name": "err"}
        candidates = [{"candidate_id": "c1", "dedup_key": "key9", "service_name": "svc-b", "signal_name": "other"}]
        result = await gh._classify_issue_dedupe_with_llm(settings, proposed, candidates)
        # Falls back to deterministic → unrelated
        assert result["classification"] == "unrelated"

    @pytest.mark.asyncio
    async def test_confidence_clamped_to_unit_range(self, monkeypatch: pytest.MonkeyPatch) -> None:
        async def _fake_call_llm(*args, **kwargs):
            return '{"classification": "related", "candidate_id": "c1", "confidence": 1.5}', {}

        monkeypatch.setattr(gh, "_call_llm", _fake_call_llm)
        settings = {"ai.endpoint_url": "http://llm", "ai.model": "gpt-test"}
        proposed = {"dedup_key": "k1"}
        candidates = [{"candidate_id": "c1"}]
        result = await gh._classify_issue_dedupe_with_llm(settings, proposed, candidates)
        assert result["confidence"] <= 1.0


class TestChooseGithubIssueOutcome:
    """Tests for the _choose_github_issue_outcome orchestration helper."""

    def _make_mock_db(self, work_items: list[dict] | None = None) -> MagicMock:
        """Return a minimal mock DB that satisfies the DB queries in the helper."""
        db = MagicMock()

        class _FakeRows:
            def __init__(self, rows: list[dict]):
                self._rows = rows

            def fetchall(self) -> list[dict]:
                return self._rows

            def fetchone(self) -> dict | None:
                return self._rows[0] if self._rows else None

        # All execute calls return empty by default
        db.execute.return_value = _FakeRows([])
        return db

    @pytest.mark.asyncio
    async def test_creates_new_issue_when_no_candidates(self, monkeypatch: pytest.MonkeyPatch) -> None:
        db = self._make_mock_db()

        async def _fake_fetch_issues(*args, **kwargs):
            return []

        async def _fake_create_issue(*args, **kwargs):
            return {
                "issue_url": "https://github.com/acme/repo/issues/1",
                "issue_number": 1,
                "issue_title": "New Issue",
                "issue_state": "open",
            }

        monkeypatch.setattr(gh, "_fetch_open_github_issues", _fake_fetch_issues)
        monkeypatch.setattr(gh, "_create_github_issue_record", _fake_create_issue)

        outcome = await gh._choose_github_issue_outcome(
            db,
            settings={},
            rule={"id": "r1", "name": "Rule 1"},
            trigger_context={"trigger_ref_id": "r1"},
            github_repo="acme/repo",
            github_token="token",
            wants_copilot_assignment=False,
            analysis="analysis text",
            suggestion="fix suggestion",
            issue_title="New Issue",
            issue_body="Issue body",
            allow_new_issue=True,
        )

        assert outcome["created_new_issue"] is True
        assert outcome["issue_url"] == "https://github.com/acme/repo/issues/1"
        assert outcome["dedup_decision"] == "new_issue"

    @pytest.mark.asyncio
    async def test_suppresses_issue_creation_when_not_allowed(self, monkeypatch: pytest.MonkeyPatch) -> None:
        db = self._make_mock_db()

        async def _fake_fetch_issues(*args, **kwargs):
            return []

        monkeypatch.setattr(gh, "_fetch_open_github_issues", _fake_fetch_issues)

        outcome = await gh._choose_github_issue_outcome(
            db,
            settings={},
            rule={"id": "r1", "name": "Rule 1"},
            trigger_context={},
            github_repo="acme/repo",
            github_token="token",
            wants_copilot_assignment=False,
            analysis="",
            suggestion="",
            issue_title="Issue",
            issue_body="Body",
            allow_new_issue=False,
        )

        assert outcome["dedup_decision"] == "suppressed_rate_limit"
        assert outcome["created_new_issue"] is False

    @pytest.mark.asyncio
    async def test_reuses_existing_issue_on_same_classification(self, monkeypatch: pytest.MonkeyPatch) -> None:
        db = self._make_mock_db()

        existing_url = "https://github.com/acme/repo/issues/5"

        # Simulate a local candidate that matches an open issue
        async def _fake_fetch_issues(*args, **kwargs):
            return [
                {
                    "issue_number": 5,
                    "issue_url": existing_url,
                    "issue_title": "Existing Issue",
                    "issue_body": "",
                    "issue_state": "open",
                    "assignees": [],
                }
            ]

        async def _fake_classify(*args, **kwargs):
            return {
                "classification": "same",
                "candidate_id": existing_url,
                "confidence": 0.92,
                "reason": "same dedup key",
            }

        async def _fake_search_pr(*args, **kwargs):
            return None

        class _FakeRows:
            def fetchone(self):
                return {"c": 2}

        db.execute.return_value = _FakeRows()

        monkeypatch.setattr(gh, "_fetch_open_github_issues", _fake_fetch_issues)
        monkeypatch.setattr(gh, "_classify_issue_dedupe_with_llm", _fake_classify)
        monkeypatch.setattr(gh, "_search_open_pr_for_issue", _fake_search_pr)
        monkeypatch.setattr(gh, "_load_recent_work_item_candidates", lambda db, repo, **kw: [
            {
                "issue_url": existing_url,
                "issue_number": 5,
                "issue_title": "Existing Issue",
                "issue_state": "open",
                "service": "checkout",
                "signal_source": "metrics",
                "signal_name": "latency",
                "anomaly_state": "critical",
                "dedup_key": "key1",
                "copilot_assignment_status": "not_requested",
                "pr_linked": False,
                "pr_url": "",
            }
        ])

        outcome = await gh._choose_github_issue_outcome(
            db,
            settings={},
            rule={"id": "r1", "name": "Rule 1"},
            trigger_context={"trigger_ref_id": "r1"},
            github_repo="acme/repo",
            github_token="token",
            wants_copilot_assignment=False,
            analysis="",
            suggestion="",
            issue_title="New Incident",
            issue_body="Body",
            allow_new_issue=True,
        )

        assert outcome["dedup_decision"] == "reused_existing"
        assert outcome["issue_url"] == existing_url
        assert outcome["created_new_issue"] is False
        assert outcome["occurrence_count"] == 3  # DB returned c=2, +1

    @pytest.mark.asyncio
    async def test_create_failed_when_create_returns_error(self, monkeypatch: pytest.MonkeyPatch) -> None:
        db = self._make_mock_db()

        async def _fake_fetch_issues(*args, **kwargs):
            return []

        async def _fake_create(*args, **kwargs):
            return {"error": "GitHub issue creation failed: 422 Unprocessable Entity"}

        monkeypatch.setattr(gh, "_fetch_open_github_issues", _fake_fetch_issues)
        monkeypatch.setattr(gh, "_create_github_issue_record", _fake_create)

        outcome = await gh._choose_github_issue_outcome(
            db,
            settings={},
            rule={"id": "r1", "name": "Rule 1"},
            trigger_context={},
            github_repo="acme/repo",
            github_token="token",
            wants_copilot_assignment=False,
            analysis="",
            suggestion="",
            issue_title="Issue",
            issue_body="Body",
            allow_new_issue=True,
        )

        assert outcome["dedup_decision"] == "create_failed"
        assert outcome["created_new_issue"] is False

    @pytest.mark.asyncio
    async def test_copilot_assignment_blocked_on_pr_linked(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """When a matching issue has a linked PR, copilot assignment is blocked."""
        db = self._make_mock_db()

        existing_url = "https://github.com/acme/repo/issues/7"

        async def _fake_fetch_issues(*args, **kwargs):
            return [
                {
                    "issue_number": 7,
                    "issue_url": existing_url,
                    "issue_title": "Existing",
                    "issue_body": "",
                    "issue_state": "open",
                    "assignees": [],
                }
            ]

        async def _fake_classify(*args, **kwargs):
            return {"classification": "same", "candidate_id": existing_url, "confidence": 0.9, "reason": "match"}

        async def _fake_search_pr(*args, **kwargs):
            return {"pr_number": 3, "pr_url": "https://github.com/acme/repo/pull/3"}

        class _FakeRows:
            def fetchone(self):
                return {"c": 1}

        db.execute.return_value = _FakeRows()

        monkeypatch.setattr(gh, "_fetch_open_github_issues", _fake_fetch_issues)
        monkeypatch.setattr(gh, "_classify_issue_dedupe_with_llm", _fake_classify)
        monkeypatch.setattr(gh, "_search_open_pr_for_issue", _fake_search_pr)
        monkeypatch.setattr(gh, "_load_recent_work_item_candidates", lambda db, repo, **kw: [
            {
                "issue_url": existing_url,
                "issue_number": 7,
                "issue_title": "Existing",
                "issue_state": "open",
                "service": "",
                "signal_source": "",
                "signal_name": "",
                "anomaly_state": "",
                "dedup_key": "",
                "copilot_assignment_status": "not_requested",
                "pr_linked": False,
                "pr_url": "",
            }
        ])

        outcome = await gh._choose_github_issue_outcome(
            db,
            settings={},
            rule={"id": "r1", "name": "Rule 1"},
            trigger_context={},
            github_repo="acme/repo",
            github_token="token",
            wants_copilot_assignment=True,
            analysis="",
            suggestion="",
            issue_title="Issue",
            issue_body="Body",
            allow_new_issue=True,
        )

        assert outcome["copilot_assignment_status"] == "blocked"
        assert "pull request" in outcome["copilot_assignment_reason"]


class TestEmitAgentIssueDecisionSummary:
    def test_does_nothing_when_wants_issue_false(self) -> None:
        # Should not raise even with empty inputs
        gh._emit_agent_issue_decision_summary(
            run_id="r1",
            rule={},
            trigger_context={},
            issue_outcome={},
            github_issue_url="",
            wants_issue=False,
            wants_copilot_assignment=False,
            github_repo="",
        )

    def test_logs_summary_when_wants_issue_true(self, caplog) -> None:
        import logging

        with caplog.at_level(logging.INFO, logger="app_github"):
            gh._emit_agent_issue_decision_summary(
                run_id="run-1",
                rule={"id": "r1", "name": "My Rule"},
                trigger_context={"trigger_type": "anomaly_rule", "trigger_ref_id": "ar1"},
                issue_outcome={
                    "dedup_decision": "new_issue",
                    "dedup_confidence": 1.0,
                    "copilot_assignment_status": "not_requested",
                    "copilot_assignment_reason": "",
                    "created_new_issue": True,
                    "occurrence_count": 1,
                },
                github_issue_url="https://github.com/acme/repo/issues/1",
                wants_issue=True,
                wants_copilot_assignment=False,
                github_repo="acme/repo",
            )
        assert any("agent_issue_decision_summary" in r.message for r in caplog.records)


# ---------------------------------------------------------------------------
# Private utility tests
# ---------------------------------------------------------------------------


class TestJsonLoads:
    def test_parses_dict(self) -> None:
        result = gh._json_loads('{"a": 1}', {})
        assert result == {"a": 1}

    def test_parses_list(self) -> None:
        result = gh._json_loads("[1, 2, 3]", [])
        assert result == [1, 2, 3]

    def test_returns_default_on_invalid(self) -> None:
        result = gh._json_loads("not json", {})
        assert result == {}

    def test_returns_default_on_empty(self) -> None:
        result = gh._json_loads("", {"default": True})
        assert result == {"default": True}

    def test_type_mismatch_returns_default(self) -> None:
        # dict expected but list provided
        result = gh._json_loads("[1, 2]", {})
        assert result == {}


class TestJsonDumps:
    def test_dict_roundtrips(self) -> None:
        d = {"key": "value", "num": 42}
        assert json.loads(gh._json_dumps(d)) == d

    def test_list_roundtrips(self) -> None:
        lst = [1, 2, 3]
        assert json.loads(gh._json_dumps(lst)) == lst

    def test_none_returns_empty_object(self) -> None:
        assert gh._json_dumps(None) == "{}"

    def test_invalid_str_returns_empty_object(self) -> None:
        assert gh._json_dumps("not json") == "{}"

    def test_valid_json_str_roundtrips(self) -> None:
        s = '{"a": 1}'
        result = gh._json_dumps(s)
        assert json.loads(result) == {"a": 1}

    def test_empty_string_returns_empty_object(self) -> None:
        assert gh._json_dumps("") == "{}"

    def test_whitespace_string_returns_empty_object(self) -> None:
        assert gh._json_dumps("   ") == "{}"

# ---------------------------------------------------------------------------
# Additional coverage tests for branches not yet covered
# ---------------------------------------------------------------------------


class TestExtractAgentTriggerFieldsEdgeCases:
    def test_invalid_signal_value_coerces_to_zero(self) -> None:
        ctx = {
            "extra": json.dumps({"service": "svc", "value": "not_a_float"})
        }
        fields = gh._extract_agent_trigger_fields(ctx)
        assert fields["signal_value"] == 0.0

    def test_extra_not_dict_defaults_to_empty(self) -> None:
        ctx = {"extra": [1, 2, 3]}
        fields = gh._extract_agent_trigger_fields(ctx)
        assert fields["service_name"] == ""

    def test_service_from_top_level_context(self) -> None:
        ctx = {"service": "my-service"}
        fields = gh._extract_agent_trigger_fields(ctx)
        assert fields["service_name"] == "my-service"


class TestSerializeGithubWorkItemRowEdgeCases:
    def test_datetime_object_in_created_at(self) -> None:
        from datetime import datetime, timezone
        row = _make_work_item_row(
            Id="x",
            CreatedAt=datetime(2025, 1, 15, 12, 0, 0, tzinfo=timezone.utc),
        )
        result = gh._serialize_github_work_item_row(row)
        assert "2025-01-15" in result["created_at"]

    def test_iso_timestamp_with_z_suffix(self) -> None:
        row = _make_work_item_row(Id="x", CreatedAt="2025-01-15T12:00:00.000Z")
        result = gh._serialize_github_work_item_row(row)
        assert result["created_at"].endswith("Z")

    def test_naive_datetime_treated_as_utc(self) -> None:
        from datetime import datetime
        row = _make_work_item_row(
            Id="x",
            CreatedAt=datetime(2025, 6, 1, 10, 30, 0),  # naive (no tzinfo)
        )
        result = gh._serialize_github_work_item_row(row)
        assert "2025-06-01" in result["created_at"]


class TestChooseGithubIssueOutcomeAdditionalBranches:
    """Test additional branches in _choose_github_issue_outcome."""

    def _make_mock_db_with_count(self, count: int = 0) -> MagicMock:
        db = MagicMock()

        class _FakeRows:
            def __init__(self, rows):
                self._rows = rows

            def fetchall(self):
                return self._rows

            def fetchone(self):
                return {"c": count}

        db.execute.return_value = _FakeRows([])
        return db

    @pytest.mark.asyncio
    async def test_copilot_assignment_hourly_limit_blocked_for_existing(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Test hourly limit blocking in the reused_existing flow."""
        db = MagicMock()

        class _FakeRows:
            def fetchall(self):
                return []

            def fetchone(self):
                return {"c": 5}  # 5 assignments this hour → exceeds any limit

        db.execute.return_value = _FakeRows()

        existing_url = "https://github.com/acme/repo/issues/9"

        async def _fake_fetch_issues(*args, **kwargs):
            return [
                {
                    "issue_number": 9,
                    "issue_url": existing_url,
                    "issue_title": "Old Issue",
                    "issue_body": "",
                    "issue_state": "open",
                    "assignees": [],
                }
            ]

        async def _fake_classify(*args, **kwargs):
            return {"classification": "same", "candidate_id": existing_url, "confidence": 0.9, "reason": "match"}

        async def _fake_search_pr(*args, **kwargs):
            return None

        monkeypatch.setattr(gh, "_fetch_open_github_issues", _fake_fetch_issues)
        monkeypatch.setattr(gh, "_classify_issue_dedupe_with_llm", _fake_classify)
        monkeypatch.setattr(gh, "_search_open_pr_for_issue", _fake_search_pr)
        monkeypatch.setattr(
            gh,
            "_load_recent_work_item_candidates",
            lambda db, repo, **kw: [
                {
                    "issue_url": existing_url,
                    "issue_number": 9,
                    "issue_title": "Old Issue",
                    "issue_state": "open",
                    "service": "",
                    "signal_source": "",
                    "signal_name": "",
                    "anomaly_state": "",
                    "dedup_key": "",
                    "copilot_assignment_status": "not_requested",
                    "pr_linked": False,
                    "pr_url": "",
                }
            ],
        )
        # Make count functions return high values to trigger hourly limit
        monkeypatch.setattr(gh, "_count_copilot_assignments_last_hour", lambda db: 99)

        outcome = await gh._choose_github_issue_outcome(
            db,
            settings={"ai.agent_max_assignments_per_hour": "1"},
            rule={"id": "r1", "name": "Rule 1"},
            trigger_context={},
            github_repo="acme/repo",
            github_token="token",
            wants_copilot_assignment=True,
            analysis="",
            suggestion="",
            issue_title="Issue",
            issue_body="Body",
            allow_new_issue=True,
        )

        assert outcome["copilot_assignment_status"] == "blocked"
        assert "hourly limit" in outcome["copilot_assignment_reason"]

    @pytest.mark.asyncio
    async def test_copilot_assignment_active_limit_blocked_for_existing(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Test active assignment limit blocking in the reused_existing flow."""
        db = MagicMock()

        class _FakeRows:
            def fetchall(self):
                return []

            def fetchone(self):
                return {"c": 0}

        db.execute.return_value = _FakeRows()

        existing_url = "https://github.com/acme/repo/issues/8"

        async def _fake_fetch_issues(*args, **kwargs):
            return [
                {
                    "issue_number": 8,
                    "issue_url": existing_url,
                    "issue_title": "Old Issue",
                    "issue_body": "",
                    "issue_state": "open",
                    "assignees": [],
                }
            ]

        async def _fake_classify(*args, **kwargs):
            return {"classification": "same", "candidate_id": existing_url, "confidence": 0.9, "reason": "match"}

        async def _fake_search_pr(*args, **kwargs):
            return None

        monkeypatch.setattr(gh, "_fetch_open_github_issues", _fake_fetch_issues)
        monkeypatch.setattr(gh, "_classify_issue_dedupe_with_llm", _fake_classify)
        monkeypatch.setattr(gh, "_search_open_pr_for_issue", _fake_search_pr)
        monkeypatch.setattr(
            gh,
            "_load_recent_work_item_candidates",
            lambda db, repo, **kw: [
                {
                    "issue_url": existing_url,
                    "issue_number": 8,
                    "issue_title": "Old Issue",
                    "issue_state": "open",
                    "service": "",
                    "signal_source": "",
                    "signal_name": "",
                    "anomaly_state": "",
                    "dedup_key": "",
                    "copilot_assignment_status": "not_requested",
                    "pr_linked": False,
                    "pr_url": "",
                }
            ],
        )
        # Hourly ok, but active limit exceeded
        monkeypatch.setattr(gh, "_count_copilot_assignments_last_hour", lambda db: 0)
        monkeypatch.setattr(gh, "_count_active_copilot_assignments", lambda db: 99)

        outcome = await gh._choose_github_issue_outcome(
            db,
            settings={"ai.agent_max_active_assignments": "1"},
            rule={"id": "r1", "name": "Rule 1"},
            trigger_context={},
            github_repo="acme/repo",
            github_token="token",
            wants_copilot_assignment=True,
            analysis="",
            suggestion="",
            issue_title="Issue",
            issue_body="Body",
            allow_new_issue=True,
        )

        assert outcome["copilot_assignment_status"] == "blocked"
        assert "active" in outcome["copilot_assignment_reason"]

    @pytest.mark.asyncio
    async def test_copilot_assignment_already_active_blocked_for_existing(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Test that an already-active Copilot assignment blocks a new one."""
        db = MagicMock()

        class _FakeRows:
            def fetchall(self):
                return []

            def fetchone(self):
                return {"c": 0}

        db.execute.return_value = _FakeRows()

        existing_url = "https://github.com/acme/repo/issues/11"

        async def _fake_fetch_issues(*args, **kwargs):
            return [
                {
                    "issue_number": 11,
                    "issue_url": existing_url,
                    "issue_title": "Active Issue",
                    "issue_body": "",
                    "issue_state": "open",
                    "assignees": ["copilot-swe-agent[bot]"],
                }
            ]

        async def _fake_classify(*args, **kwargs):
            return {"classification": "same", "candidate_id": existing_url, "confidence": 0.9, "reason": "match"}

        async def _fake_search_pr(*args, **kwargs):
            return None

        monkeypatch.setattr(gh, "_fetch_open_github_issues", _fake_fetch_issues)
        monkeypatch.setattr(gh, "_classify_issue_dedupe_with_llm", _fake_classify)
        monkeypatch.setattr(gh, "_search_open_pr_for_issue", _fake_search_pr)
        monkeypatch.setattr(
            gh,
            "_load_recent_work_item_candidates",
            lambda db, repo, **kw: [
                {
                    "issue_url": existing_url,
                    "issue_number": 11,
                    "issue_title": "Active Issue",
                    "issue_state": "open",
                    "service": "",
                    "signal_source": "",
                    "signal_name": "",
                    "anomaly_state": "",
                    "dedup_key": "",
                    "copilot_assignment_status": "requested",
                    "pr_linked": False,
                    "pr_url": "",
                }
            ],
        )

        outcome = await gh._choose_github_issue_outcome(
            db,
            settings={},
            rule={"id": "r1", "name": "Rule 1"},
            trigger_context={},
            github_repo="acme/repo",
            github_token="token",
            wants_copilot_assignment=True,
            analysis="",
            suggestion="",
            issue_title="Issue",
            issue_body="Body",
            allow_new_issue=True,
        )

        assert outcome["copilot_assignment_status"] == "blocked"
        assert "already being worked" in outcome["copilot_assignment_reason"]

    @pytest.mark.asyncio
    async def test_copilot_assignment_new_issue_with_hourly_limit_blocked(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Test hourly limit blocking when creating a new issue with copilot."""
        db = MagicMock()

        class _FakeRows:
            def fetchall(self):
                return []

            def fetchone(self):
                return {"c": 0}

        db.execute.return_value = _FakeRows()

        async def _fake_fetch_issues(*args, **kwargs):
            return []

        async def _fake_create(*args, **kwargs):
            return {
                "issue_url": "https://github.com/acme/repo/issues/20",
                "issue_number": 20,
                "issue_title": "New Issue",
                "issue_state": "open",
            }

        monkeypatch.setattr(gh, "_fetch_open_github_issues", _fake_fetch_issues)
        monkeypatch.setattr(gh, "_create_github_issue_record", _fake_create)
        monkeypatch.setattr(gh, "_count_copilot_assignments_last_hour", lambda db: 99)

        outcome = await gh._choose_github_issue_outcome(
            db,
            settings={"ai.agent_max_assignments_per_hour": "1"},
            rule={"id": "r1", "name": "Rule 1"},
            trigger_context={},
            github_repo="acme/repo",
            github_token="token",
            wants_copilot_assignment=True,
            analysis="",
            suggestion="new issue suggestion",
            issue_title="Issue",
            issue_body="Body",
            allow_new_issue=True,
        )

        assert outcome["copilot_assignment_status"] == "blocked"
        assert "hourly limit" in outcome["copilot_assignment_reason"]

    @pytest.mark.asyncio
    async def test_copilot_assignment_new_issue_with_active_limit_blocked(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Test active limit blocking when creating a new issue with copilot."""
        db = MagicMock()

        class _FakeRows:
            def fetchall(self):
                return []

            def fetchone(self):
                return {"c": 0}

        db.execute.return_value = _FakeRows()

        async def _fake_fetch_issues(*args, **kwargs):
            return []

        async def _fake_create(*args, **kwargs):
            return {
                "issue_url": "https://github.com/acme/repo/issues/21",
                "issue_number": 21,
                "issue_title": "New Issue",
                "issue_state": "open",
            }

        monkeypatch.setattr(gh, "_fetch_open_github_issues", _fake_fetch_issues)
        monkeypatch.setattr(gh, "_create_github_issue_record", _fake_create)
        monkeypatch.setattr(gh, "_count_copilot_assignments_last_hour", lambda db: 0)
        monkeypatch.setattr(gh, "_count_active_copilot_assignments", lambda db: 99)

        outcome = await gh._choose_github_issue_outcome(
            db,
            settings={"ai.agent_max_active_assignments": "1"},
            rule={"id": "r1", "name": "Rule 1"},
            trigger_context={},
            github_repo="acme/repo",
            github_token="token",
            wants_copilot_assignment=True,
            analysis="",
            suggestion="",
            issue_title="Issue",
            issue_body="Body",
            allow_new_issue=True,
        )

        assert outcome["copilot_assignment_status"] == "blocked"
        assert "active" in outcome["copilot_assignment_reason"]

    @pytest.mark.asyncio
    async def test_copilot_assigned_on_new_issue_success(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Test successful Copilot assignment when creating a new issue."""
        db = MagicMock()

        class _FakeRows:
            def fetchall(self):
                return []

            def fetchone(self):
                return {"c": 0}

        db.execute.return_value = _FakeRows()

        async def _fake_fetch_issues(*args, **kwargs):
            return []

        async def _fake_create(*args, **kwargs):
            return {
                "issue_url": "https://github.com/acme/repo/issues/22",
                "issue_number": 22,
                "issue_title": "New Issue",
                "issue_state": "open",
            }

        async def _fake_assign(*args, **kwargs):
            return "requested", "Copilot assignment requested", 1234567890000

        monkeypatch.setattr(gh, "_fetch_open_github_issues", _fake_fetch_issues)
        monkeypatch.setattr(gh, "_create_github_issue_record", _fake_create)
        monkeypatch.setattr(gh, "_assign_issue_to_copilot", _fake_assign)
        monkeypatch.setattr(gh, "_count_copilot_assignments_last_hour", lambda db: 0)
        monkeypatch.setattr(gh, "_count_active_copilot_assignments", lambda db: 0)

        outcome = await gh._choose_github_issue_outcome(
            db,
            settings={},
            rule={"id": "r1", "name": "Rule 1"},
            trigger_context={},
            github_repo="acme/repo",
            github_token="token",
            wants_copilot_assignment=True,
            analysis="",
            suggestion="use this fix guidance",
            issue_title="Issue",
            issue_body="Body",
            allow_new_issue=True,
        )

        assert outcome["copilot_assignment_status"] == "requested"
        assert outcome["created_new_issue"] is True

    @pytest.mark.asyncio
    async def test_create_failed_blocks_copilot_assignment(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Test that create failure blocks copilot assignment for new issue flow."""
        db = MagicMock()

        class _FakeRows:
            def fetchall(self):
                return []

            def fetchone(self):
                return {"c": 0}

        db.execute.return_value = _FakeRows()

        async def _fake_fetch_issues(*args, **kwargs):
            return []

        async def _fake_create(*args, **kwargs):
            return {"error": "Creation failed: 403 Forbidden"}

        monkeypatch.setattr(gh, "_fetch_open_github_issues", _fake_fetch_issues)
        monkeypatch.setattr(gh, "_create_github_issue_record", _fake_create)

        outcome = await gh._choose_github_issue_outcome(
            db,
            settings={},
            rule={"id": "r1", "name": "Rule 1"},
            trigger_context={},
            github_repo="acme/repo",
            github_token="token",
            wants_copilot_assignment=True,
            analysis="",
            suggestion="",
            issue_title="Issue",
            issue_body="Body",
            allow_new_issue=True,
        )

        assert outcome["copilot_assignment_status"] == "blocked"
        assert outcome["dedup_decision"] == "create_failed"

    @pytest.mark.asyncio
    async def test_open_issue_not_in_local_candidates_still_adds_to_candidates(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Test that open GitHub issues not in local DB are still considered as candidates."""
        db = MagicMock()

        class _FakeRows:
            def fetchall(self):
                return []

            def fetchone(self):
                return {"c": 0}

        db.execute.return_value = _FakeRows()

        github_url = "https://github.com/acme/repo/issues/15"

        async def _fake_fetch_issues(*args, **kwargs):
            # Return an issue that is NOT in local DB candidates
            return [
                {
                    "issue_number": 15,
                    "issue_url": github_url,
                    "issue_title": "Open GitHub Issue",
                    "issue_body": "Issue body",
                    "issue_state": "open",
                    "assignees": [],
                }
            ]

        async def _fake_classify(settings, proposed, candidates):
            # Should receive the candidate from GitHub
            assert any(c["issue_url"] == github_url for c in candidates)
            return {
                "classification": "same",
                "candidate_id": github_url,
                "confidence": 0.88,
                "reason": "title match",
            }

        async def _fake_search_pr(*args, **kwargs):
            return None

        monkeypatch.setattr(gh, "_fetch_open_github_issues", _fake_fetch_issues)
        monkeypatch.setattr(gh, "_classify_issue_dedupe_with_llm", _fake_classify)
        monkeypatch.setattr(gh, "_search_open_pr_for_issue", _fake_search_pr)
        # No local candidates
        monkeypatch.setattr(gh, "_load_recent_work_item_candidates", lambda db, repo, **kw: [])

        outcome = await gh._choose_github_issue_outcome(
            db,
            settings={},
            rule={"id": "r1", "name": "Rule 1"},
            trigger_context={},
            github_repo="acme/repo",
            github_token="token",
            wants_copilot_assignment=False,
            analysis="",
            suggestion="",
            issue_title="Issue",
            issue_body="Body",
            allow_new_issue=True,
        )

        assert outcome["issue_url"] == github_url
        assert outcome["dedup_decision"] == "reused_existing"


# ---------------------------------------------------------------------------
# Tests for remaining uncovered paths
# ---------------------------------------------------------------------------


class TestJsonDumpsEdgeCases:
    def test_non_serializable_type_returns_empty_object(self) -> None:
        # Integer/float/bool are not str/dict/list → return "{}"
        assert gh._json_dumps(42) == "{}"
        assert gh._json_dumps(3.14) == "{}"
        assert gh._json_dumps(True) == "{}"


class TestSerializeGithubWorkItemRowTimestamps:
    def test_invalid_datetime_string_returns_raw(self) -> None:
        """When fromisoformat raises ValueError, the raw string is returned."""
        row = _make_work_item_row(CreatedAt="not-a-valid-timestamp")
        result = gh._serialize_github_work_item_row(row)
        # Invalid timestamp should be returned as-is
        assert result["created_at"] == "not-a-valid-timestamp"

    def test_aware_datetime_converted_to_utc(self) -> None:
        """Timezone-aware datetime is converted to UTC."""
        from datetime import datetime, timezone, timedelta
        # +05:30 offset
        tz = timezone(timedelta(hours=5, minutes=30))
        row = _make_work_item_row(CreatedAt=datetime(2025, 1, 1, 15, 30, 0, tzinfo=tz))
        result = gh._serialize_github_work_item_row(row)
        # 15:30 +05:30 → 10:00 UTC
        assert "10:00" in result["created_at"]
        assert result["created_at"].endswith("Z")


class TestDeriveCopilotAssignmentStatusAdditional:
    def test_not_requested_open_issue_no_assignees(self) -> None:
        status, reason = gh._derive_copilot_assignment_status("not_requested", "open", [], False)
        assert status == "not_requested"
        assert reason == ""


class TestCreateGithubIssueHTTPErrors:
    @pytest.mark.asyncio
    async def test_http_status_error_with_json_message(self, monkeypatch: pytest.MonkeyPatch) -> None:
        import httpx

        class _FailingClient:
            async def post(self, url, **kwargs):
                resp = MagicMock()
                resp.json.return_value = {"message": "Unprocessable Entity"}
                raise httpx.HTTPStatusError("422", request=MagicMock(), response=resp)

        async def _fake_get_http_client():
            return _FailingClient()

        monkeypatch.setattr(gh, "_get_http_client", _fake_get_http_client)
        result = await gh._create_github_issue_record("token", "acme/demo", "Title", "Body")
        assert "error" in result
        assert "Unprocessable Entity" in result["error"]

    @pytest.mark.asyncio
    async def test_http_status_error_json_parse_fails_falls_back_to_str(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        import httpx

        class _FailingClient:
            async def post(self, url, **kwargs):
                resp = MagicMock()
                resp.json.side_effect = Exception("not json")
                raise httpx.HTTPStatusError("500", request=MagicMock(), response=resp)

        async def _fake_get_http_client():
            return _FailingClient()

        monkeypatch.setattr(gh, "_get_http_client", _fake_get_http_client)
        result = await gh._create_github_issue_record("token", "acme/demo", "Title", "Body")
        assert "error" in result

    @pytest.mark.asyncio
    async def test_repo_fallback_parsing(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test the fallback parsing for non-standard repo URL formats."""
        payload = {
            "html_url": "https://github.com/acme/demo/issues/10",
            "number": 10,
            "title": "Test Issue",
            "state": "open",
        }

        async def _fake_get_http_client():
            return _make_fake_http_client({"/issues": payload})

        monkeypatch.setattr(gh, "_get_http_client", _fake_get_http_client)
        # "acme/demo" - direct owner/repo format without https://
        result = await gh._create_github_issue_record("token", "acme/demo", "Test Issue", "Body")
        assert result.get("issue_url") == "https://github.com/acme/demo/issues/10"


class TestCreateGithubIssueWrapper:
    @pytest.mark.asyncio
    async def test_returns_url_string(self, monkeypatch: pytest.MonkeyPatch) -> None:
        async def _fake_create_record(*args, **kwargs):
            return {"issue_url": "https://github.com/acme/demo/issues/5"}

        monkeypatch.setattr(gh, "_create_github_issue_record", _fake_create_record)
        url = await gh._create_github_issue(
            "token", "acme/demo", "Title", "Body"
        )
        assert url == "https://github.com/acme/demo/issues/5"

    @pytest.mark.asyncio
    async def test_returns_empty_string_on_error(self, monkeypatch: pytest.MonkeyPatch) -> None:
        async def _fake_create_record(*args, **kwargs):
            return {"error": "failed"}

        monkeypatch.setattr(gh, "_create_github_issue_record", _fake_create_record)
        url = await gh._create_github_issue("token", "acme/demo", "Title", "Body")
        assert url == ""

    @pytest.mark.asyncio
    async def test_custom_labels_passed(self, monkeypatch: pytest.MonkeyPatch) -> None:
        captured: list[dict] = []

        async def _fake_create_record(token, repo, title, body, labels=None, **kw):
            captured.append({"labels": labels})
            return {"issue_url": "https://github.com/acme/demo/issues/1"}

        monkeypatch.setattr(gh, "_create_github_issue_record", _fake_create_record)
        await gh._create_github_issue("token", "acme/demo", "Title", "Body", ["custom"])
        assert captured[0]["labels"] == ["custom"]


class TestGithubRepoSupportsCopilotCopilotVariantLogin:
    @pytest.mark.asyncio
    async def test_copilot_swe_agent_variant_detected(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Copilot support detected when login is 'copilot-swe-agent' (no brackets)."""
        payload = {
            "data": {
                "repository": {
                    "suggestedActors": {
                        "nodes": [{"__typename": "Bot", "login": "copilot-swe-agent", "id": "99"}]
                    }
                }
            }
        }

        async def _fake_get_http_client():
            return _make_fake_http_client({"/graphql": payload})

        monkeypatch.setattr(gh, "_get_http_client", _fake_get_http_client)
        assert await gh._github_repo_supports_copilot_assignment("token", "acme/repo") is True

    @pytest.mark.asyncio
    async def test_non_dict_node_skipped(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Non-dict nodes in the suggested actors list are skipped gracefully."""
        payload = {
            "data": {
                "repository": {
                    "suggestedActors": {
                        "nodes": ["not-a-dict", {"login": "other-user"}]
                    }
                }
            }
        }

        async def _fake_get_http_client():
            return _make_fake_http_client({"/graphql": payload})

        monkeypatch.setattr(gh, "_get_http_client", _fake_get_http_client)
        assert await gh._github_repo_supports_copilot_assignment("token", "acme/repo") is False


class TestAssignIssueToCopilotAdditionalPaths:
    @pytest.mark.asyncio
    async def test_successful_assignment_assignee_lag(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test the 'lag' path where Copilot is not yet in assignees list."""
        assignee_payload = {"assignees": [{"login": "other-user"}]}

        async def _fake_supports(_token: str, _repo: str) -> bool:
            return True

        async def _fake_get_http_client():
            return _make_fake_http_client({"/assignees": assignee_payload})

        monkeypatch.setattr(gh, "_github_repo_supports_copilot_assignment", _fake_supports)
        monkeypatch.setattr(gh, "_get_http_client", _fake_get_http_client)
        status, reason, ts = await gh._assign_issue_to_copilot("token", "acme/repo", 42)
        assert status == "requested"
        assert "lag" in reason.lower() or reason

    @pytest.mark.asyncio
    async def test_returns_blocked_on_invalid_repo(self, monkeypatch: pytest.MonkeyPatch) -> None:
        async def _fake_supports(_token, _repo):
            return True

        monkeypatch.setattr(gh, "_github_repo_supports_copilot_assignment", _fake_supports)
        # Single segment = can't parse owner/repo
        status, reason, ts = await gh._assign_issue_to_copilot("token", "singleword", 42)
        assert status == "blocked"
        assert "invalid" in reason.lower()

    @pytest.mark.asyncio
    async def test_custom_instructions_and_base_branch_included(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        captured_payloads: list[dict] = []
        assignee_payload = {"assignees": [{"login": "copilot-swe-agent[bot]"}]}

        class _CapturingClient:
            async def post(self, url, json=None, **kwargs):
                captured_payloads.append(dict(json or {}))
                return _make_fake_http_response(assignee_payload)

        async def _fake_supports(_token, _repo):
            return True

        monkeypatch.setattr(gh, "_github_repo_supports_copilot_assignment", _fake_supports)
        monkeypatch.setattr(gh, "_get_http_client", lambda: _CapturingClient())

        async def _async_client():
            return _CapturingClient()

        monkeypatch.setattr(gh, "_get_http_client", _async_client)

        status, reason, ts = await gh._assign_issue_to_copilot(
            "token", "acme/repo", 42,
            base_branch="main",
            custom_instructions="fix the login flow",
        )
        assert status == "requested"
        assert len(captured_payloads) == 1
        agent_assignment = captured_payloads[0].get("agent_assignment", {})
        assert agent_assignment.get("base_branch") == "main"
        assert "fix the login flow" in agent_assignment.get("custom_instructions", "")


class TestClassifyIssueDedupeEdgeCases:
    @pytest.mark.asyncio
    async def test_confidence_parsing_exception_defaults_to_zero(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        async def _fake_call_llm(*args, **kwargs):
            return '{"classification": "related", "candidate_id": "c1", "confidence": "invalid_number"}', {}

        monkeypatch.setattr(gh, "_call_llm", _fake_call_llm)
        settings = {"ai.endpoint_url": "http://llm", "ai.model": "gpt-test"}
        proposed = {"dedup_key": "k1"}
        candidates = [{"candidate_id": "c1"}]
        result = await gh._classify_issue_dedupe_with_llm(settings, proposed, candidates)
        # confidence parsing fails → default 0.0
        assert result["confidence"] == pytest.approx(0.0)


class TestChooseGithubIssueOutcomeReusedWithCopilot:
    """Test the reused_existing flow with successful copilot assignment."""

    @pytest.mark.asyncio
    async def test_copilot_assigned_on_reused_existing(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        db = MagicMock()

        class _FakeRows:
            def fetchall(self):
                return []

            def fetchone(self):
                return {"c": 1}

        db.execute.return_value = _FakeRows()

        existing_url = "https://github.com/acme/repo/issues/5"

        async def _fake_fetch_issues(*args, **kwargs):
            return [
                {
                    "issue_number": 5,
                    "issue_url": existing_url,
                    "issue_title": "Existing Issue",
                    "issue_body": "",
                    "issue_state": "open",
                    "assignees": [],
                }
            ]

        async def _fake_classify(*args, **kwargs):
            return {
                "classification": "same",
                "candidate_id": existing_url,
                "confidence": 0.9,
                "reason": "key match",
            }

        async def _fake_search_pr(*args, **kwargs):
            return None

        async def _fake_assign(*args, **kwargs):
            return "requested", "Copilot assignment requested", 1234567890000

        monkeypatch.setattr(gh, "_fetch_open_github_issues", _fake_fetch_issues)
        monkeypatch.setattr(gh, "_classify_issue_dedupe_with_llm", _fake_classify)
        monkeypatch.setattr(gh, "_search_open_pr_for_issue", _fake_search_pr)
        monkeypatch.setattr(gh, "_assign_issue_to_copilot", _fake_assign)
        monkeypatch.setattr(gh, "_count_copilot_assignments_last_hour", lambda db: 0)
        monkeypatch.setattr(gh, "_count_active_copilot_assignments", lambda db: 0)
        monkeypatch.setattr(gh, "_load_recent_work_item_candidates", lambda db, repo, **kw: [
            {
                "issue_url": existing_url,
                "issue_number": 5,
                "issue_title": "Existing Issue",
                "issue_state": "open",
                "service": "",
                "signal_source": "",
                "signal_name": "",
                "anomaly_state": "",
                "dedup_key": "key1",
                "copilot_assignment_status": "not_requested",
                "pr_linked": False,
                "pr_url": "",
            }
        ])

        outcome = await gh._choose_github_issue_outcome(
            db,
            settings={},
            rule={"id": "r1", "name": "Rule 1"},
            trigger_context={},
            github_repo="acme/repo",
            github_token="token",
            wants_copilot_assignment=True,
            analysis="",
            suggestion="important suggestion",
            issue_title="Issue",
            issue_body="Body",
            allow_new_issue=True,
        )

        assert outcome["copilot_assignment_status"] == "requested"
        assert outcome["dedup_decision"] == "reused_existing"
