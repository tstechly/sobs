from __future__ import annotations

import json
from collections.abc import Callable, Iterable, Mapping
from typing import Any

from shared.repo_health import (
    _build_repo_health_summary,
    _build_repo_health_targets,
    _collect_release_versions_by_app,
    _collect_repo_health_version_tokens,
    _summarize_repo_health_items,
)


def _repo_health_compact_values(summary: Mapping[str, Any]) -> dict[str, int]:
    return {
        "scanned_repos": int(summary.get("scanned_repos", 0) or 0),
        "total_repos_considered": int(summary.get("total_repos_considered", 0) or 0),
        "open_issues": int(summary.get("open_issues", 0) or 0),
        "open_prs": int(summary.get("open_prs", 0) or 0),
        "security_items": int(summary.get("security_items", 0) or 0),
    }


def _build_repo_health_persist_payload(
    summary: Mapping[str, Any],
    previous_raw: str,
    *,
    safe_json_loads: Callable[[object, dict[str, Any]], dict[str, Any]],
) -> dict[str, Any]:
    compact_values = _repo_health_compact_values(summary)
    previous_values: dict[str, int] = {}
    if previous_raw:
        previous = safe_json_loads(previous_raw, {})
        previous_values = {
            "scanned_repos": int(previous.get("scanned_repos", 0) or 0),
            "total_repos_considered": int(previous.get("total_repos_considered", 0) or 0),
            "open_issues": int(previous.get("open_issues", 0) or 0),
            "open_prs": int(previous.get("open_prs", 0) or 0),
            "security_items": int(previous.get("security_items", 0) or 0),
        }

    compact = {
        **compact_values,
        "last_synced_at": str(summary.get("last_synced_at") or ""),
    }
    return {
        "should_persist": previous_values != compact_values,
        "last_synced_at": compact["last_synced_at"],
        "compact": compact,
        "compact_json": json.dumps(compact, separators=(",", ":")),
    }


async def _collect_github_repo_health_summary(
    app_rows: Iterable[Any],
    release_rows: Iterable[Any],
    *,
    default_github_token: str,
    client: Any,
    max_repos: int,
    max_items_per_repo: int,
    load_repo_scoped_github_token: Callable[[str, str], str],
    parse_github_repo_owner_name: Callable[[str], tuple[str, str]],
    github_version_tokens: Callable[[str], set[str]],
    text_mentions_version_tokens: Callable[[str, set[str]], bool],
    github_item_is_security_related: Callable[[dict[str, Any]], bool],
    github_api_headers: Callable[..., dict[str, str]],
    now_iso: Callable[[], str],
) -> dict[str, Any]:
    versions_by_app = _collect_release_versions_by_app(release_rows)
    repo_targets = _build_repo_health_targets(
        app_rows,
        versions_by_app,
        parse_github_repo_owner_name=parse_github_repo_owner_name,
    )[:max_repos]

    scanned_repos = 0
    repos_summary: list[dict[str, Any]] = []

    for target in repo_targets:
        owner = str(target["owner"])
        repo = str(target["repo"])
        github_token = load_repo_scoped_github_token(owner, repo) or default_github_token
        if not github_token:
            continue

        versions = [str(version) for version in target.get("versions", []) if str(version).strip()]
        version_tokens = _collect_repo_health_version_tokens(
            versions,
            github_version_tokens=github_version_tokens,
        )
        if not version_tokens:
            continue

        scanned_repos += 1
        try:
            response = await client.get(
                f"https://api.github.com/repos/{owner}/{repo}/issues",
                params={"state": "open", "per_page": str(max_items_per_repo)},
                headers=github_api_headers(github_token),
                timeout=15,
            )
            if response.status_code != 200:
                continue
            items = response.json() if response.content else []
            if not isinstance(items, list):
                continue
        except Exception:
            continue

        repo_issues, repo_prs, repo_security = _summarize_repo_health_items(
            items,
            version_tokens=version_tokens,
            text_mentions_version_tokens=text_mentions_version_tokens,
            github_item_is_security_related=github_item_is_security_related,
        )
        repos_summary.append(
            {
                "repo": f"{owner}/{repo}",
                "app_name": str(target.get("app_name") or ""),
                "versions": versions,
                "open_issues": repo_issues,
                "open_prs": repo_prs,
                "security_items": repo_security,
            }
        )

    return _build_repo_health_summary(
        repos_summary,
        scanned_repos=scanned_repos,
        total_repos_considered=len(repo_targets),
        last_synced_at=now_iso(),
    )
