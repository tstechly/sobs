from __future__ import annotations

import hashlib
import json
import urllib.parse
from collections.abc import Callable, Iterable
from typing import Any

GITHUB_CONTENTS_LOCKFILE_CANDIDATES: list[tuple[str, str, str]] = [
    ("requirements.txt", "text/plain", "requirements"),
    ("package-lock.json", "application/json", "package_lock"),
    ("go.sum", "text/plain", "go_sum"),
    ("Gemfile.lock", "text/plain", "gemfile_lock"),
]


def _build_github_backfill_targets(
    release_rows: Iterable[tuple[Any, Any, Any, Any]],
    app_rows: Iterable[tuple[Any, Any, Any]],
    existing_release_ids: set[str],
    *,
    parse_github_repo_owner_name: Callable[[str], tuple[str, str]],
) -> list[dict[str, str]]:
    apps_by_id: dict[str, dict[str, int | str]] = {
        str(row[0]): {
            "repo_url": str(row[1] or "").strip(),
            "enabled": int(row[2] or 0),
        }
        for row in app_rows
    }

    targets: list[dict[str, str]] = []
    for row in release_rows:
        release_id = str(row[0] or "")
        app_id = str(row[1] or "")
        release_version = str(row[2] or "").strip()
        commit_sha = str(row[3] or "").strip()
        if not release_id or not release_version or release_id in existing_release_ids:
            continue

        app_info = apps_by_id.get(app_id)
        if not app_info:
            continue

        repo_url = str(app_info["repo_url"] or "").strip()
        app_enabled = int(app_info["enabled"] or 0)
        if not app_enabled or not repo_url:
            continue

        owner, repo = parse_github_repo_owner_name(repo_url)
        if not owner or not repo:
            continue

        targets.append(
            {
                "release_id": release_id,
                "release_version": release_version,
                "commit_sha": commit_sha,
                "owner": owner,
                "repo": repo,
            }
        )

    return targets


def _build_github_contents_dependency_row(
    *,
    artifact_id: str,
    release_id: str,
    owner: str,
    repo: str,
    lockfile_path: str,
    content_type: str,
    ref: str,
    raw_bytes: bytes,
    dependencies: list[dict[str, str]],
    uploaded_at: str,
    version: int,
) -> dict[str, Any]:
    return {
        "Id": artifact_id,
        "ReleaseId": release_id,
        "ArtifactType": "dependencies-lockfile",
        "Name": lockfile_path,
        "ContentType": content_type,
        "Size": len(raw_bytes),
        "StorageRef": f"github://{owner}/{repo}/{lockfile_path}?ref={urllib.parse.quote(ref, safe='')}",
        "ChecksumSha256": hashlib.sha256(raw_bytes).hexdigest(),
        "Platform": "",
        "Architecture": "",
        "MetadataJson": json.dumps(
            {
                "source": "github_contents_api",
                "repo": f"{owner}/{repo}",
                "ref": ref,
                "path": lockfile_path,
                "dependencies": dependencies,
            },
            separators=(",", ":"),
        ),
        "UploadedAt": uploaded_at,
        "IsDeleted": 0,
        "Version": version,
    }
