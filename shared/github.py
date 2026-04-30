"""GitHub repository and token helper utilities shared across SOBS modules."""

from __future__ import annotations

import re
import urllib.parse
from collections.abc import Awaitable, Callable
from datetime import datetime, timezone
from typing import Any


def _github_repo_token_key(owner: str, repo: str) -> str:
    return f"ai.github_token.repo.{owner.strip().lower()}/{repo.strip().lower()}"


def _parse_github_repo_owner_name(repo_url: str) -> tuple[str, str]:
    """Extract owner/repo from a GitHub repo URL.

    Supports HTTPS, SSH, and plain owner/repo styles.
    """
    cleaned = (repo_url or "").strip()
    if not cleaned:
        return "", ""

    direct_parts = [part for part in cleaned.split("/") if part]
    if len(direct_parts) == 2 and "://" not in cleaned and not cleaned.startswith("git@"):
        return direct_parts[0], direct_parts[1].removesuffix(".git")

    if cleaned.startswith("git@github.com:"):
        path = cleaned.split(":", 1)[1]
    else:
        parsed = urllib.parse.urlparse(cleaned)
        if parsed.netloc.lower() != "github.com":
            return "", ""
        path = parsed.path.lstrip("/")

    if path.endswith(".git"):
        path = path[:-4]
    parts = [part for part in path.split("/") if part]
    if len(parts) < 2:
        return "", ""
    return parts[0], parts[1]


def _build_github_repo_url(owner: str, repo: str) -> str:
    owner_clean = (owner or "").strip().strip("/")
    repo_clean = (repo or "").strip().strip("/").removesuffix(".git")
    if not owner_clean or not repo_clean:
        return ""
    return f"https://github.com/{owner_clean}/{repo_clean}"


def _resolve_github_repo_fields(repo_url: str, owner: str = "", repo: str = "") -> tuple[str, str, str]:
    repo_url_clean = str(repo_url or "").strip()
    owner_clean = str(owner or "").strip().strip("/")
    repo_clean = str(repo or "").strip().strip("/").removesuffix(".git")

    if (not owner_clean or not repo_clean) and repo_url_clean:
        parsed_owner, parsed_repo = _parse_github_repo_owner_name(repo_url_clean)
        if not owner_clean:
            owner_clean = parsed_owner
        if not repo_clean:
            repo_clean = parsed_repo

    canonical_repo_url = _build_github_repo_url(owner_clean, repo_clean)
    if canonical_repo_url:
        repo_url_clean = canonical_repo_url

    return repo_url_clean, owner_clean, repo_clean


def _parse_iso_datetime(value: str) -> datetime | None:
    raw = str(value or "").strip()
    if not raw:
        return None
    if raw.endswith("Z"):
        raw = raw[:-1] + "+00:00"
    try:
        parsed = datetime.fromisoformat(raw)
    except ValueError:
        return None
    if parsed.tzinfo is None:
        return parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(timezone.utc)


def _normalize_github_token_expiry_input(value: str) -> str:
    raw = str(value or "").strip()
    if not raw:
        return ""
    if re.fullmatch(r"\d{4}-\d{2}-\d{2}", raw):
        return f"{raw}T23:59:59+00:00"
    parsed = _parse_iso_datetime(raw)
    if not parsed:
        return ""
    return parsed.isoformat()


def _github_token_expiry_date_input_value(value: str) -> str:
    parsed = _parse_iso_datetime(value)
    return parsed.date().isoformat() if parsed else ""


def _github_token_expiry_status(expires_at: str, warning_days: int = 14) -> dict[str, Any]:
    parsed = _parse_iso_datetime(expires_at)
    if not parsed:
        return {
            "state": "unknown",
            "expires_at": "",
            "days_remaining": None,
            "message": "Token expiry date not set",
        }

    now_utc = datetime.now(timezone.utc)
    seconds_remaining = int((parsed - now_utc).total_seconds())
    days_remaining = seconds_remaining // 86400

    if seconds_remaining < 0:
        return {
            "state": "expired",
            "expires_at": parsed.isoformat(),
            "days_remaining": days_remaining,
            "message": f"Token expired on {parsed.date().isoformat()}",
        }
    if days_remaining <= warning_days:
        return {
            "state": "warning",
            "expires_at": parsed.isoformat(),
            "days_remaining": days_remaining,
            "message": f"Token expires in {days_remaining} day(s)",
        }
    return {
        "state": "healthy",
        "expires_at": parsed.isoformat(),
        "days_remaining": days_remaining,
        "message": f"Token healthy ({days_remaining} day(s) remaining)",
    }


async def _validate_github_token(
    github_token: str,
    get_async_http_client: Callable[[], Awaitable[Any]],
) -> tuple[str, str]:
    token = str(github_token or "").strip()
    if not token:
        return "missing", "No token configured"

    client = await get_async_http_client()
    try:
        response = await client.get(
            "https://api.github.com/rate_limit",
            headers={
                "Accept": "application/vnd.github+json",
                "Authorization": f"Bearer {token}",
                "X-GitHub-Api-Version": "2022-11-28",
            },
            timeout=15.0,
        )
    except Exception as exc:
        return "error", f"Validation request failed: {exc.__class__.__name__}"

    if response.status_code == 200:
        return "valid", "Token is valid"
    if response.status_code == 401:
        return "invalid", "Token rejected (401 Unauthorized)"
    if response.status_code == 403:
        return "error", "GitHub returned 403 (forbidden or rate-limited)"
    return "error", f"GitHub returned HTTP {response.status_code}"
