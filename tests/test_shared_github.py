from __future__ import annotations

from datetime import datetime, timedelta, timezone

import pytest

from shared.github import (
    _github_repo_token_key,
    _github_token_expiry_date_input_value,
    _github_token_expiry_status,
    _normalize_github_token_expiry_input,
    _parse_github_repo_owner_name,
    _resolve_github_repo_fields,
    _validate_github_token,
)


class _FakeResponse:
    def __init__(self, status_code: int):
        self.status_code = status_code


class _FakeClient:
    def __init__(self, status_code: int = 200, error: Exception | None = None):
        self.status_code = status_code
        self.error = error

    async def get(self, *_args, **_kwargs):
        if self.error is not None:
            raise self.error
        return _FakeResponse(self.status_code)


def test_parse_github_repo_owner_name_supports_plain_https_and_ssh() -> None:
    assert _parse_github_repo_owner_name("owner/repo") == ("owner", "repo")
    assert _parse_github_repo_owner_name("https://github.com/octo/widgets.git") == ("octo", "widgets")
    assert _parse_github_repo_owner_name("git@github.com:octo/widgets.git") == ("octo", "widgets")


def test_parse_github_repo_owner_name_rejects_non_github_hosts() -> None:
    assert _parse_github_repo_owner_name("https://example.com/octo/widgets") == ("", "")


def test_resolve_github_repo_fields_prefers_explicit_owner_and_repo() -> None:
    assert _resolve_github_repo_fields("", "Octo", "widgets.git") == (
        "https://github.com/Octo/widgets",
        "Octo",
        "widgets",
    )


def test_resolve_github_repo_fields_normalizes_existing_url() -> None:
    assert _resolve_github_repo_fields("git@github.com:octo/widgets.git") == (
        "https://github.com/octo/widgets",
        "octo",
        "widgets",
    )


def test_github_repo_token_key_normalizes_case() -> None:
    assert _github_repo_token_key("Octo", "Widgets") == "ai.github_token.repo.octo/widgets"


def test_normalize_github_token_expiry_input_supports_date_and_iso_values() -> None:
    assert _normalize_github_token_expiry_input("2025-02-03") == "2025-02-03T23:59:59+00:00"
    assert _normalize_github_token_expiry_input("2025-02-03T01:02:03Z") == "2025-02-03T01:02:03+00:00"


def test_normalize_github_token_expiry_input_rejects_invalid_values() -> None:
    assert _normalize_github_token_expiry_input("not-a-date") == ""


def test_github_token_expiry_date_input_value_returns_date_only() -> None:
    assert _github_token_expiry_date_input_value("2025-02-03T01:02:03+00:00") == "2025-02-03"


def test_github_token_expiry_status_handles_unknown_healthy_warning_and_expired() -> None:
    assert _github_token_expiry_status("")["state"] == "unknown"

    healthy = _github_token_expiry_status((datetime.now(timezone.utc) + timedelta(days=30)).isoformat())
    assert healthy["state"] == "healthy"

    warning = _github_token_expiry_status((datetime.now(timezone.utc) + timedelta(days=3)).isoformat())
    assert warning["state"] == "warning"

    expired = _github_token_expiry_status((datetime.now(timezone.utc) - timedelta(days=1)).isoformat())
    assert expired["state"] == "expired"


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("status_code", "expected_status", "expected_message"),
    [
        (200, "valid", "Token is valid"),
        (401, "invalid", "Token rejected (401 Unauthorized)"),
        (403, "error", "GitHub returned 403 (forbidden or rate-limited)"),
        (500, "error", "GitHub returned HTTP 500"),
    ],
)
async def test_validate_github_token_maps_http_status_codes(
    status_code: int,
    expected_status: str,
    expected_message: str,
) -> None:
    async def _get_client() -> _FakeClient:
        return _FakeClient(status_code=status_code)

    assert await _validate_github_token("ghp_test", _get_client) == (expected_status, expected_message)


@pytest.mark.asyncio
async def test_validate_github_token_handles_missing_token_and_transport_errors() -> None:
    async def _get_client() -> _FakeClient:
        return _FakeClient(error=RuntimeError("boom"))

    assert await _validate_github_token("", _get_client) == ("missing", "No token configured")
    assert await _validate_github_token("ghp_test", _get_client) == ("error", "Validation request failed: RuntimeError")
