import base64
import json
from typing import Any

from shared.onboarding import (
    _build_ci_metadata_issue_body,
    _build_otel_audit_issue_body,
    _decode_github_contents_payload,
    _github_file_text,
    _github_list_directory,
    _inspect_repo_for_onboarding,
    _parse_gemfile_lock_dependencies,
    _parse_go_sum_dependencies,
    _parse_package_lock_dependencies,
    _parse_requirements_dependencies,
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

    assert _parse_package_lock_dependencies("[]") == []
    assert _parse_package_lock_dependencies(packages_content) == [
        {"package": "react", "version": "18.3.1", "ecosystem": "npm"},
    ]
    assert _parse_package_lock_dependencies(json.dumps({"dependencies": []})) == []
    assert _parse_package_lock_dependencies(legacy_content) == [
        {"package": "vite", "version": "5.4.0", "ecosystem": "npm"},
    ]


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
