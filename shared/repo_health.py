from __future__ import annotations

from collections.abc import Callable, Iterable
from typing import Any


def _collect_release_versions_by_app(
    release_rows: Iterable[tuple[Any, Any]],
    *,
    max_versions_per_app: int = 5,
) -> dict[str, list[str]]:
    versions_by_app: dict[str, list[str]] = {}
    for row in release_rows:
        app_id = str(row[0] or "")
        release_version = str(row[1] or "").strip()
        if not app_id or not release_version:
            continue
        versions = versions_by_app.setdefault(app_id, [])
        if release_version not in versions and len(versions) < max_versions_per_app:
            versions.append(release_version)
    return versions_by_app


def _build_repo_health_targets(
    app_rows: Iterable[tuple[Any, Any, Any, Any]],
    versions_by_app: dict[str, list[str]],
    *,
    parse_github_repo_owner_name: Callable[[str], tuple[str, str]],
) -> list[dict[str, Any]]:
    targets: list[dict[str, Any]] = []
    for row in app_rows:
        app_id = str(row[0] or "")
        app_name = str(row[1] or row[2] or "")
        repo_url = str(row[3] or "")
        owner, repo = parse_github_repo_owner_name(repo_url)
        versions = versions_by_app.get(app_id, [])
        if not owner or not repo or not versions:
            continue
        targets.append(
            {
                "app_name": app_name,
                "owner": owner,
                "repo": repo,
                "versions": versions,
            }
        )
    return targets


def _collect_repo_health_version_tokens(
    versions: Iterable[str],
    *,
    github_version_tokens: Callable[[str], set[str]],
) -> set[str]:
    version_tokens: set[str] = set()
    for version in versions:
        cleaned = str(version).strip()
        if not cleaned:
            continue
        version_tokens.update(github_version_tokens(cleaned))
    return version_tokens


def _summarize_repo_health_items(
    items: Iterable[Any],
    *,
    version_tokens: set[str],
    text_mentions_version_tokens: Callable[[str, set[str]], bool],
    github_item_is_security_related: Callable[[dict[str, Any]], bool],
) -> tuple[int, int, int]:
    open_issues = 0
    open_prs = 0
    security_items = 0

    for item in items:
        if not isinstance(item, dict):
            continue
        text = f"{str(item.get('title') or '')}\n{str(item.get('body') or '')}"
        if not text_mentions_version_tokens(text, version_tokens):
            continue
        if isinstance(item.get("pull_request"), dict):
            open_prs += 1
        else:
            open_issues += 1
        if github_item_is_security_related(item):
            security_items += 1

    return open_issues, open_prs, security_items


def _build_repo_health_summary(
    repos_summary: list[dict[str, Any]],
    *,
    scanned_repos: int,
    total_repos_considered: int,
    last_synced_at: str,
) -> dict[str, Any]:
    repos = sorted(
        repos_summary,
        key=lambda row: (
            -(int(row.get("security_items", 0)) + int(row.get("open_issues", 0)) + int(row.get("open_prs", 0))),
            str(row.get("repo") or "").lower(),
        ),
    )
    return {
        "ok": True,
        "scanned_repos": scanned_repos,
        "total_repos_considered": total_repos_considered,
        "open_issues": sum(int(row.get("open_issues", 0)) for row in repos),
        "open_prs": sum(int(row.get("open_prs", 0)) for row in repos),
        "security_items": sum(int(row.get("security_items", 0)) for row in repos),
        "version_scoped": True,
        "last_synced_at": last_synced_at,
        "repos": repos,
    }
