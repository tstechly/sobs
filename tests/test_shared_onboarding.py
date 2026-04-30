import base64
import json
from typing import Any

from shared.onboarding import (
    _build_ci_metadata_issue_body,
    _build_otel_audit_issue_body,
    _create_onboarding_issue_result,
    _decode_github_contents_payload,
    _github_file_text,
    _github_import_repo_metadata,
    _github_list_directory,
    _github_list_repositories_for_owner,
    _inspect_repo_for_onboarding,
    _parse_gemfile_lock_dependencies,
    _parse_go_sum_dependencies,
    _parse_package_lock_dependencies,
    _parse_requirements_dependencies,
    _persist_onboarding_work_item,
)


class _FakeResponse:
    def __init__(self, status_code: int, payload: Any):
        self.status_code = status_code
        self._payload = payload
        self.content = b"" if payload is None else b"payload"

    def json(self):
        return self._payload


class _FakeClient:
    def __init__(self, responses: dict[str, _FakeResponse] | None = None, error_paths: set[str] | None = None):
        self.responses = responses or {}
        self.error_paths = error_paths or set()

    async def get(self, url: str, headers=None, timeout=0):
        path = url.split("/contents/", 1)[-1]
        if path in self.error_paths:
            raise RuntimeError(f"boom:{path}")
        return self.responses.get(path, _FakeResponse(404, {"message": "not found"}))


async def _get_fake_client(client: _FakeClient) -> _FakeClient:
    return client


class _FakeGithubResponse:
    def __init__(self, status_code: int, payload: Any):
        self.status_code = status_code
        self._payload = payload
        self.content = b"" if payload is None else b"payload"

    def json(self):
        return self._payload


class _FakeGithubLookupClient:
    def __init__(self, responses: dict[str, _FakeGithubResponse], errors: dict[str, Exception] | None = None):
        self.responses = responses
        self.errors = errors or {}

    async def get(self, url: str, headers=None, timeout=0):
        if url in self.errors:
            raise self.errors[url]
        return self.responses[url]


async def _get_fake_lookup_client(client: _FakeGithubLookupClient) -> _FakeGithubLookupClient:
    return client


def test_parse_requirements_dependencies_filters_comments_markers_and_duplicates():
    content = """
    # comment
    requests==2.32.0
    requests==2.32.0
    flask==3.1.0 ; python_version > '3.10'
    uvicorn>=0.30
    rich==
    """

    assert _parse_requirements_dependencies(content) == [
        {"package": "requests", "version": "2.32.0", "ecosystem": "PyPI"},
        {"package": "flask", "version": "3.1.0", "ecosystem": "PyPI"},
    ]


def test_parse_requirements_dependencies_strips_inline_comments():
    content = "httpx==0.28.1 # pinned for onboarding\n"

    assert _parse_requirements_dependencies(content) == [
        {"package": "httpx", "version": "0.28.1", "ecosystem": "PyPI"},
    ]


def test_parse_package_lock_dependencies_prefers_packages_section():
    content = json.dumps(
        {
            "packages": {
                "": {"name": "root"},
                "node_modules/react": {"version": "18.3.1"},
                "node_modules/@scope/pkg": {"version": "1.2.3"},
                "src/local": {"version": "0.0.1"},
            }
        }
    )

    assert _parse_package_lock_dependencies(content) == [
        {"package": "react", "version": "18.3.1", "ecosystem": "npm"},
        {"package": "@scope/pkg", "version": "1.2.3", "ecosystem": "npm"},
    ]


def test_parse_package_lock_dependencies_falls_back_to_legacy_dependencies():
    content = json.dumps(
        {
            "dependencies": {
                "vite": {"version": "5.4.0"},
                "typescript": {"version": "5.6.2"},
            }
        }
    )

    assert _parse_package_lock_dependencies(content) == [
        {"package": "vite", "version": "5.4.0", "ecosystem": "npm"},
        {"package": "typescript", "version": "5.6.2", "ecosystem": "npm"},
    ]


def test_parse_package_lock_dependencies_handles_invalid_and_duplicate_entries():
    packages_content = json.dumps(
        {
            "packages": {
                "node_modules/react": {"version": "18.3.1"},
                "node_modules/React": {"version": "18.3.1"},
                "node_modules/react-copy": {"version": ""},
            }
        }
    )
    legacy_content = json.dumps(
        {
            "dependencies": {
                "vite": {"version": "5.4.0"},
                "VITE": {"version": "5.4.0"},
                "missing-version": {"version": ""},
                "broken": [],
            }
        }
    )

    assert _parse_package_lock_dependencies(packages_content) == [
        {"package": "react", "version": "18.3.1", "ecosystem": "npm"},
    ]
    assert _parse_package_lock_dependencies(json.dumps({"dependencies": []})) == []
    assert _parse_package_lock_dependencies(legacy_content) == [
        {"package": "vite", "version": "5.4.0", "ecosystem": "npm"},
    ]
    assert _parse_package_lock_dependencies("[]") == []


async def test_github_import_repo_metadata_success():
    url = "https://api.github.com/repos/octo/checkout-service"
    client = _FakeGithubLookupClient(
        {
            url: _FakeGithubResponse(
                200,
                {
                    "name": "checkout-service",
                    "full_name": "octo/checkout-service",
                    "html_url": "https://github.com/octo/checkout-service",
                    "default_branch": "main",
                    "visibility": "private",
                    "description": "Checkout API",
                },
            )
        }
    )

    status_code, payload = await _github_import_repo_metadata(
        "token",
        "octo",
        "checkout-service",
        get_async_http_client=lambda: _get_fake_lookup_client(client),
    )

    assert status_code == 200
    assert payload == {
        "ok": True,
        "owner": "octo",
        "repo": "checkout-service",
        "full_name": "octo/checkout-service",
        "repo_url": "https://github.com/octo/checkout-service",
        "name": "checkout-service",
        "slug": "checkout-service",
        "default_branch": "main",
        "visibility": "private",
        "description": "Checkout API",
    }


async def test_github_import_repo_metadata_handles_non_200_and_bad_payload():
    not_found_url = "https://api.github.com/repos/octo/missing"
    bad_payload_url = "https://api.github.com/repos/octo/weird"
    client = _FakeGithubLookupClient(
        {
            not_found_url: _FakeGithubResponse(404, {"message": "Not Found"}),
            bad_payload_url: _FakeGithubResponse(200, ["unexpected"]),
        }
    )

    not_found_status, not_found_payload = await _github_import_repo_metadata(
        "",
        "octo",
        "missing",
        get_async_http_client=lambda: _get_fake_lookup_client(client),
    )
    bad_payload_status, bad_payload_payload = await _github_import_repo_metadata(
        "",
        "octo",
        "weird",
        get_async_http_client=lambda: _get_fake_lookup_client(client),
    )

    assert not_found_status == 400
    assert not_found_payload == {"ok": False, "error": "Not Found"}
    assert bad_payload_status == 502
    assert bad_payload_payload == {"ok": False, "error": "Unexpected GitHub response payload"}


async def test_github_import_repo_metadata_handles_request_failure():
    url = "https://api.github.com/repos/octo/fail"
    client = _FakeGithubLookupClient({}, errors={url: RuntimeError("boom")})

    status_code, payload = await _github_import_repo_metadata(
        "",
        "octo",
        "fail",
        get_async_http_client=lambda: _get_fake_lookup_client(client),
    )

    assert status_code == 502
    assert payload == {"ok": False, "error": "GitHub lookup failed: boom"}


async def test_github_list_repositories_for_owner_sorts_results_and_falls_back_to_org_endpoint():
    user_url = "https://api.github.com/users/octo/repos?per_page=100&type=all&sort=full_name"
    org_url = "https://api.github.com/orgs/octo/repos?per_page=100&type=all&sort=full_name"
    client = _FakeGithubLookupClient(
        {
            user_url: _FakeGithubResponse(404, {"message": "Not Found"}),
            org_url: _FakeGithubResponse(
                200,
                [
                    {
                        "name": "z-service",
                        "full_name": "octo/z-service",
                        "private": True,
                        "owner": {"login": "octo"},
                    },
                    {
                        "name": "a-service",
                        "full_name": "octo/a-service",
                        "html_url": "https://github.com/octo/a-service",
                        "private": False,
                        "owner": {"login": "octo"},
                    },
                    {"not": "a repo"},
                ],
            ),
        }
    )

    status_code, payload = await _github_list_repositories_for_owner(
        "token",
        "octo",
        get_async_http_client=lambda: _get_fake_lookup_client(client),
    )

    assert status_code == 200
    assert payload == {
        "ok": True,
        "owner": "octo",
        "repos": [
            {
                "name": "a-service",
                "full_name": "octo/a-service",
                "repo_url": "https://github.com/octo/a-service",
                "private": False,
            },
            {
                "name": "z-service",
                "full_name": "octo/z-service",
                "repo_url": "https://github.com/octo/z-service",
                "private": True,
            },
        ],
        "token_used": True,
        "visibility_note": "",
    }


async def test_github_list_repositories_for_owner_handles_error_and_request_failure():
    public_user_url = "https://api.github.com/users/octo/repos?per_page=100&type=public&sort=full_name"
    public_org_url = "https://api.github.com/orgs/octo/repos?per_page=100&type=public&sort=full_name"
    error_client = _FakeGithubLookupClient(
        {
            public_user_url: _FakeGithubResponse(403, {"message": "Forbidden"}),
            public_org_url: _FakeGithubResponse(403, {"message": "Forbidden"}),
        }
    )
    failure_client = _FakeGithubLookupClient({}, errors={public_user_url: RuntimeError("boom")})

    error_status, error_payload = await _github_list_repositories_for_owner(
        "",
        "octo",
        get_async_http_client=lambda: _get_fake_lookup_client(error_client),
    )
    failure_status, failure_payload = await _github_list_repositories_for_owner(
        "",
        "octo",
        get_async_http_client=lambda: _get_fake_lookup_client(failure_client),
    )

    assert error_status == 400
    assert error_payload == {"ok": False, "error": "Forbidden"}
    assert failure_status == 502
    assert failure_payload == {"ok": False, "error": "GitHub lookup failed: boom"}


def test_parse_go_sum_dependencies_dedupes_and_strips_go_mod_suffix():
    content = """
    github.com/example/mod v1.2.3 h1:abc
    github.com/example/mod v1.2.3/go.mod h1:def
    github.com/other/pkg v0.9.0 h1:ghi
    """

    assert _parse_go_sum_dependencies(content) == [
        {"package": "github.com/example/mod", "version": "v1.2.3", "ecosystem": "Go"},
        {"package": "github.com/other/pkg", "version": "v0.9.0", "ecosystem": "Go"},
    ]


def test_parse_go_sum_dependencies_ignores_short_and_empty_entries():
    content = """
    incomplete-entry
    github.com/example/empty /go.mod h1:abc
    github.com/example/valid v1.0.0 h1:def
    """

    assert _parse_go_sum_dependencies(content) == [
        {"package": "github.com/example/valid", "version": "v1.0.0", "ecosystem": "Go"},
    ]


def test_parse_gemfile_lock_dependencies_reads_specs_only():
    content = """
    GEM
      specs:
        activesupport (7.2.0)
        bootsnap (1.18.4)

    DEPENDENCIES
      rails
    """

    assert _parse_gemfile_lock_dependencies(content) == [
        {"package": "activesupport", "version": "7.2.0", "ecosystem": "RubyGems"},
        {"package": "bootsnap", "version": "1.18.4", "ecosystem": "RubyGems"},
    ]


def test_parse_gemfile_lock_dependencies_skips_invalid_or_duplicate_specs():
    content = """
        GEM
            specs:
                valid-gem (1.2.3)
                invalid-line
                empty-version (, jruby)
                valid-gem (1.2.3)
        DEPENDENCIES
            rails
        """

    assert _parse_gemfile_lock_dependencies(content) == [
        {"package": "valid-gem", "version": "1.2.3", "ecosystem": "RubyGems"},
    ]


def test_decode_github_contents_payload_decodes_base64_and_handles_invalid_payloads():
    encoded = base64.b64encode(b"hello world").decode("ascii")

    assert _decode_github_contents_payload({"encoding": "base64", "content": encoded}) == b"hello world"
    assert _decode_github_contents_payload({"encoding": "utf-8", "content": encoded}) == b""
    assert _decode_github_contents_payload({"encoding": "base64", "content": object()}) == b""
    assert _decode_github_contents_payload({"encoding": "base64", "content": "%%%"}) == b""


async def test_github_list_directory_returns_entries_and_errors():
    client = _FakeClient(
        responses={
            ".github/workflows": _FakeResponse(200, [{"name": "build.yml"}]),
        }
    )

    entries, error = await _github_list_directory(
        "token",
        "owner",
        "repo",
        ".github/workflows",
        get_async_http_client=lambda: _get_fake_client(client),
    )
    assert entries == [{"name": "build.yml"}]
    assert error == ""

    missing_entries, missing_error = await _github_list_directory(
        "token",
        "owner",
        "repo",
        "missing",
        get_async_http_client=lambda: _get_fake_client(client),
    )
    assert missing_entries == []
    assert missing_error == "GitHub API returned 404 for missing"


async def test_github_list_directory_reports_request_failures():
    client = _FakeClient(error_paths={".github/workflows"})

    entries, error = await _github_list_directory(
        "token",
        "owner",
        "repo",
        ".github/workflows",
        get_async_http_client=lambda: _get_fake_client(client),
    )

    assert entries == []
    assert "GitHub API request failed for .github/workflows" in error


async def test_github_file_text_decodes_base64_contents_and_reports_bad_payloads():
    encoded = base64.b64encode(b"otel\n").decode("ascii")
    client = _FakeClient(
        responses={
            "requirements.txt": _FakeResponse(200, {"encoding": "base64", "content": encoded}),
            "bad.json": _FakeResponse(200, []),
        }
    )

    content, error = await _github_file_text(
        "token",
        "owner",
        "repo",
        "requirements.txt",
        get_async_http_client=lambda: _get_fake_client(client),
    )
    assert content == "otel\n"
    assert error == ""

    bad_content, bad_error = await _github_file_text(
        "token",
        "owner",
        "repo",
        "bad.json",
        get_async_http_client=lambda: _get_fake_client(client),
    )
    assert bad_content == ""
    assert bad_error == "Unexpected GitHub API response for bad.json"


async def test_github_file_text_reports_non_200_and_request_failures():
    client = _FakeClient(
        responses={"missing.txt": _FakeResponse(500, {"message": "boom"})},
        error_paths={"error.txt"},
    )

    missing_content, missing_error = await _github_file_text(
        "token",
        "owner",
        "repo",
        "missing.txt",
        get_async_http_client=lambda: _get_fake_client(client),
    )
    assert missing_content == ""
    assert missing_error == "GitHub API returned 500 for missing.txt"

    error_content, error_text = await _github_file_text(
        "token",
        "owner",
        "repo",
        "error.txt",
        get_async_http_client=lambda: _get_fake_client(client),
    )
    assert error_content == ""
    assert "GitHub API request failed for error.txt" in error_text


async def test_inspect_repo_for_onboarding_handles_missing_configuration():
    result = await _inspect_repo_for_onboarding(
        "",
        "owner",
        "repo",
        get_async_http_client=lambda: _get_fake_client(_FakeClient()),
        github_repo_supports_copilot_assignment=lambda *_args: _return_bool(False),
    )

    assert result["has_github_actions"] is False
    assert result["error"] == "GitHub token or repository not configured"


async def test_inspect_repo_for_onboarding_detects_workflow_indicators():
    workflow_yaml = """
    steps:
      - run: curl -X POST https://sobs.internal/v1/apps/123/releases
      - run: python -m opentelemetry.instrumentation.auto_instrumentation
    """
    client = _FakeClient(
        responses={
            ".github/workflows": _FakeResponse(200, [{"name": "build.yml"}, {"name": "notes.txt"}]),
            ".github/workflows/build.yml": _FakeResponse(
                200,
                {
                    "encoding": "base64",
                    "content": base64.b64encode(workflow_yaml.encode("utf-8")).decode("ascii"),
                },
            ),
        }
    )

    result = await _inspect_repo_for_onboarding(
        "token",
        "owner",
        "repo",
        get_async_http_client=lambda: _get_fake_client(client),
        github_repo_supports_copilot_assignment=lambda *_args: _return_bool(True),
    )

    assert result == {
        "has_github_actions": True,
        "sobs_ci_found": True,
        "sobs_otel_found": True,
        "copilot_available": True,
        "workflow_files": ["build.yml"],
        "error": "",
    }


async def test_inspect_repo_for_onboarding_falls_back_to_manifest_for_otel():
    workflow_yaml = "name: ci\nsteps:\n  - run: echo source map\n"
    client = _FakeClient(
        responses={
            ".github/workflows": _FakeResponse(200, [{"name": "build.yml"}]),
            ".github/workflows/build.yml": _FakeResponse(
                200,
                {
                    "encoding": "base64",
                    "content": base64.b64encode(workflow_yaml.encode("utf-8")).decode("ascii"),
                },
            ),
            "requirements.txt": _FakeResponse(
                200,
                {
                    "encoding": "base64",
                    "content": base64.b64encode(b"opentelemetry-sdk==1.28.0\n").decode("ascii"),
                },
            ),
        }
    )

    result = await _inspect_repo_for_onboarding(
        "token",
        "owner",
        "repo",
        get_async_http_client=lambda: _get_fake_client(client),
        github_repo_supports_copilot_assignment=lambda *_args: _return_bool(False),
    )

    assert result["has_github_actions"] is True
    assert result["sobs_ci_found"] is True
    assert result["sobs_otel_found"] is True
    assert result["copilot_available"] is False


async def test_inspect_repo_for_onboarding_returns_directory_error():
    client = _FakeClient(responses={".github/workflows": _FakeResponse(500, {"message": "boom"})})

    result = await _inspect_repo_for_onboarding(
        "token",
        "owner",
        "repo",
        get_async_http_client=lambda: _get_fake_client(client),
        github_repo_supports_copilot_assignment=lambda *_args: _return_bool(True),
    )

    assert result["error"] == "GitHub API returned 500 for .github/workflows"
    assert result["workflow_files"] == []


async def test_inspect_repo_for_onboarding_preserves_workflow_read_error_and_manifest_error():
    client = _FakeClient(
        responses={
            ".github/workflows": _FakeResponse(200, [{"name": "build.yml"}]),
            "requirements.txt": _FakeResponse(500, {"message": "boom"}),
        },
        error_paths={".github/workflows/build.yml"},
    )

    result = await _inspect_repo_for_onboarding(
        "token",
        "owner",
        "repo",
        get_async_http_client=lambda: _get_fake_client(client),
        github_repo_supports_copilot_assignment=lambda *_args: _return_bool(False),
    )

    assert result["has_github_actions"] is True
    assert result["workflow_files"] == ["build.yml"]
    assert "GitHub API request failed for .github/workflows/build.yml" in result["error"]


async def test_inspect_repo_for_onboarding_records_manifest_error_without_workflows():
    client = _FakeClient(
        responses={
            ".github/workflows": _FakeResponse(404, {"message": "missing"}),
            "requirements.txt": _FakeResponse(500, {"message": "boom"}),
            "package.json": _FakeResponse(404, {"message": "missing"}),
            "go.mod": _FakeResponse(404, {"message": "missing"}),
            "pom.xml": _FakeResponse(404, {"message": "missing"}),
            "build.gradle": _FakeResponse(404, {"message": "missing"}),
        }
    )

    result = await _inspect_repo_for_onboarding(
        "token",
        "owner",
        "repo",
        get_async_http_client=lambda: _get_fake_client(client),
        github_repo_supports_copilot_assignment=lambda *_args: _return_bool(False),
    )

    assert result["has_github_actions"] is False
    assert result["workflow_files"] == []
    assert result["error"] == "GitHub API returned 500 for requirements.txt"


def test_build_ci_metadata_issue_body_contains_required_sections():
    body = _build_ci_metadata_issue_body("myorg", "myrepo", has_github_actions=True)

    assert "Register a release" in body
    assert "Upload dependency lockfile" in body
    assert "actions/upload-artifact" in body
    assert "include-hidden-files: true" in body
    assert "Trigger a CVE scan" in body
    assert "myorg/myrepo" in body


def test_build_otel_audit_issue_body_contains_expected_sections():
    body = _build_otel_audit_issue_body("myorg", "myrepo")

    assert "RUM" in body
    assert "gen_ai" in body
    assert "Infrastructure" in body
    assert "myorg/myrepo" in body


async def _return_bool(value: bool) -> bool:
    return value


class _FakeLogger:
    def __init__(self):
        self.messages: list[str] = []

    def warning(self, message: str, *args: Any):
        self.messages.append(message % args if args else message)


def test_persist_onboarding_work_item_returns_early_without_issue_url():
    inserted_rows: list[dict[str, Any]] = []

    _persist_onboarding_work_item(
        db=object(),
        github_repo="owner/repo",
        issue_url="",
        issue_number=1,
        issue_title="ignored",
        issue_state="open",
        dedup_decision="created",
        note="ignored",
        copilot_assignment_status="not_requested",
        copilot_assignment_reason="",
        copilot_assignment_requested_at=0,
        issue_type="ci",
        normalize_ch_timestamp=lambda _dt: "2026-04-30 12:00:00.000000",
        parse_github_repo_owner_name=lambda _repo: ("owner", "repo"),
        parse_issue_ref_from_url=lambda _url: ("owner", "repo", 1),
        insert_rows_json_each_row=lambda _db, _table, rows: inserted_rows.extend(rows),
        invalidate_work_items_cache=lambda: inserted_rows.append({"cache": True}),
    )

    assert inserted_rows == []


def test_persist_onboarding_work_item_builds_expected_row_and_falls_back_to_issue_url_repo():
    inserted_rows: list[dict[str, Any]] = []
    cache_invalidations = {"count": 0}

    _persist_onboarding_work_item(
        db=object(),
        github_repo="",
        issue_url="https://github.com/acme/widget/issues/777",
        issue_number=777,
        issue_title="[Sobs] Set up CI metadata scripts for widget",
        issue_state="",
        dedup_decision="reused",
        note="Reused existing onboarding issue.",
        copilot_assignment_status="requested",
        copilot_assignment_reason="copilot available",
        copilot_assignment_requested_at=42,
        issue_type="ci",
        normalize_ch_timestamp=lambda _dt: "2026-04-30 12:00:00.000000",
        parse_github_repo_owner_name=lambda _repo: ("", ""),
        parse_issue_ref_from_url=lambda _url: ("acme", "widget", 777),
        insert_rows_json_each_row=lambda _db, _table, rows: inserted_rows.extend(rows),
        invalidate_work_items_cache=lambda: cache_invalidations.__setitem__("count", 1),
    )

    assert cache_invalidations["count"] == 1
    assert len(inserted_rows) == 1
    row = inserted_rows[0]
    assert row["GithubRepo"] == "acme/widget"
    assert row["ServiceName"] == "widget"
    assert row["AgentAction"] == "onboarding_ci"
    assert row["DedupDecision"] == "reused"
    assert row["DedupConfidence"] == 1.0
    assert row["IssueState"] == "open"
    assert row["SuggestionSummary"] == "Reused existing onboarding issue."
    assert row["CopilotAssignmentStatus"] == "requested"
    assert row["CopilotAssignmentReason"] == "copilot available"
    assert row["CopilotAssignmentRequestedAt"] == 42
    assert row["CreatedAt"] == "2026-04-30 12:00:00.000000"
    assert row["CompletedAt"] == "2026-04-30 12:00:00.000000"


def test_persist_onboarding_work_item_logs_and_swallows_insert_failures():
    logger = _FakeLogger()

    def _raise_insert(_db: Any, _table_name: str, _rows: list[dict[str, Any]]):
        raise RuntimeError("boom")

    _persist_onboarding_work_item(
        db=object(),
        github_repo="owner/repo",
        issue_url="https://github.com/owner/repo/issues/1",
        issue_number=1,
        issue_title="Issue",
        issue_state="open",
        dedup_decision="created",
        note="Created",
        copilot_assignment_status="not_requested",
        copilot_assignment_reason="",
        copilot_assignment_requested_at=0,
        issue_type="observability",
        normalize_ch_timestamp=lambda _dt: "2026-04-30 12:00:00.000000",
        parse_github_repo_owner_name=lambda _repo: ("owner", "repo"),
        parse_issue_ref_from_url=lambda _url: ("owner", "repo", 1),
        insert_rows_json_each_row=_raise_insert,
        invalidate_work_items_cache=lambda: None,
        logger=logger,
    )

    assert logger.messages == ["Failed to persist onboarding work item: boom"]


async def test_create_onboarding_issue_result_returns_error_without_assignment_or_persistence():
    assignment_calls = {"count": 0}
    persisted: list[dict[str, Any]] = []

    async def _fake_upsert(_token: str, _repo: str, _title: str, _body_md: str, _labels: list[str]) -> dict[str, Any]:
        return {"error": "GitHub failed"}

    async def _fake_assign(_token: str, _repo: str, _issue_number: int) -> tuple[str, str, int]:
        assignment_calls["count"] += 1
        return "requested", "should not run", 1

    result = await _create_onboarding_issue_result(
        github_token="token",
        github_repo="owner/repo",
        title="Issue title",
        body_md="Issue body",
        labels=["sobs-onboarding"],
        assign_copilot=True,
        issue_type="ci",
        issue_title_fallback="fallback",
        create_or_update_onboarding_issue=_fake_upsert,
        assign_issue_to_copilot=_fake_assign,
        persist_onboarding_work_item=lambda **kwargs: persisted.append(kwargs),
    )

    assert result == {"error": "GitHub failed"}
    assert assignment_calls["count"] == 0
    assert persisted == []


async def test_create_onboarding_issue_result_assigns_and_persists_created_issue():
    persisted: list[dict[str, Any]] = []

    async def _fake_upsert(_token: str, _repo: str, _title: str, _body_md: str, _labels: list[str]) -> dict[str, Any]:
        return {
            "issue_url": "https://github.com/owner/repo/issues/77",
            "issue_number": 77,
            "status": "created",
            "note": "Created a new onboarding issue.",
            "issue_state": "open",
        }

    async def _fake_assign(_token: str, _repo: str, _issue_number: int) -> tuple[str, str, int]:
        return "requested", "copilot available", 12345

    result = await _create_onboarding_issue_result(
        github_token="token",
        github_repo="owner/repo",
        title="Issue title",
        body_md="Issue body",
        labels=["sobs-onboarding"],
        assign_copilot=True,
        issue_type="observability",
        issue_title_fallback="Fallback title",
        create_or_update_onboarding_issue=_fake_upsert,
        assign_issue_to_copilot=_fake_assign,
        persist_onboarding_work_item=lambda **kwargs: persisted.append(kwargs),
    )

    assert result == {
        "url": "https://github.com/owner/repo/issues/77",
        "number": 77,
        "status": "created",
        "note": "Created a new onboarding issue.",
        "copilot_status": "requested",
        "copilot_assignment_status": "requested",
        "copilot_assignment_reason": "copilot available",
        "copilot_assignment_requested_at": 12345,
    }
    assert persisted == [
        {
            "github_repo": "owner/repo",
            "issue_url": "https://github.com/owner/repo/issues/77",
            "issue_number": 77,
            "issue_title": "Fallback title",
            "issue_state": "open",
            "dedup_decision": "created",
            "note": "Created a new onboarding issue.",
            "copilot_assignment_status": "requested",
            "copilot_assignment_reason": "copilot available",
            "copilot_assignment_requested_at": 12345,
            "issue_type": "observability",
        }
    ]


async def test_create_onboarding_issue_result_skips_persistence_for_reused_issue():
    assignment_calls = {"count": 0}
    persisted: list[dict[str, Any]] = []

    async def _fake_upsert(_token: str, _repo: str, _title: str, _body_md: str, _labels: list[str]) -> dict[str, Any]:
        return {
            "issue_url": "https://github.com/owner/repo/issues/88",
            "issue_number": 88,
            "status": "reused",
            "note": "Existing issue reused.",
            "issue_title": "Existing title",
        }

    async def _fake_assign(_token: str, _repo: str, _issue_number: int) -> tuple[str, str, int]:
        assignment_calls["count"] += 1
        return "requested", "unused", 1

    result = await _create_onboarding_issue_result(
        github_token="token",
        github_repo="owner/repo",
        title="Issue title",
        body_md="Issue body",
        labels=["sobs-onboarding"],
        assign_copilot=False,
        issue_type="ci",
        issue_title_fallback="Fallback title",
        create_or_update_onboarding_issue=_fake_upsert,
        assign_issue_to_copilot=_fake_assign,
        persist_onboarding_work_item=lambda **kwargs: persisted.append(kwargs),
    )

    assert result["status"] == "reused"
    assert result["copilot_assignment_status"] == "not_requested"
    assert assignment_calls["count"] == 0
    assert persisted == []
