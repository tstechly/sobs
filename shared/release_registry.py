from __future__ import annotations

import json
from collections.abc import Callable
from typing import Any

from shared.github_issues import _safe_json_loads


def _serialize_release_row(row: dict[str, Any]) -> dict[str, Any]:
    return {
        "id": str(row.get("Id", "")),
        "appId": str(row.get("AppId", "")),
        "version": str(row.get("ReleaseVersion", "")),
        "commitSha": str(row.get("CommitSha", "")),
        "buildId": str(row.get("BuildId", "")),
        "environment": str(row.get("Environment", "")),
        "releasedAt": str(row.get("ReleasedAt", "")),
        "metadata": _safe_json_loads(row.get("MetadataJson", ""), {}),
    }


def _serialize_artifact_row(row: dict[str, Any]) -> dict[str, Any]:
    return {
        "id": str(row.get("Id", "")),
        "releaseId": str(row.get("ReleaseId", "")),
        "artifactType": str(row.get("ArtifactType", "")),
        "name": str(row.get("Name", "")),
        "contentType": str(row.get("ContentType", "")),
        "size": int(row.get("Size", 0) or 0),
        "storageRef": str(row.get("StorageRef", "")),
        "checksumSha256": str(row.get("ChecksumSha256", "")),
        "platform": str(row.get("Platform", "")),
        "architecture": str(row.get("Architecture", "")),
        "metadata": _safe_json_loads(row.get("MetadataJson", ""), {}),
        "uploadedAt": str(row.get("UploadedAt", "")),
    }


def _parse_app_registry_seed(seed_raw: str) -> tuple[list[Any], str | None]:
    try:
        parsed = json.loads(seed_raw)
    except Exception as exc:
        return [], f"Failed to parse app registry seed JSON: {exc}"

    if isinstance(parsed, dict):
        apps = parsed.get("apps", [])
    elif isinstance(parsed, list):
        apps = parsed
    else:
        return [], "Ignoring app registry seed: expected object with 'apps' or an array"

    if not isinstance(apps, list):
        return [], "Ignoring app registry seed: 'apps' must be an array"
    return apps, None


def _build_seed_registry_rows(
    apps: list[Any],
    *,
    find_existing_app_id: Callable[[str], str],
    find_existing_release_id: Callable[[str, str, str, str], str],
    app_slug: Callable[[str], str],
    parse_bool: Callable[[Any, bool], bool],
    safe_json_dumps: Callable[[Any], str],
    now_iso: Callable[[], str],
    now_version: int,
    generate_id: Callable[[], str],
) -> tuple[list[dict[str, Any]], list[dict[str, Any]], list[dict[str, Any]]]:
    app_rows: list[dict[str, Any]] = []
    release_rows: list[dict[str, Any]] = []
    artifact_rows: list[dict[str, Any]] = []

    for app_item in apps:
        if not isinstance(app_item, dict):
            continue
        name = str(app_item.get("name", "")).strip()
        if not name:
            continue

        slug = app_slug(str(app_item.get("slug", "")).strip() or name)
        existing_app_id = find_existing_app_id(slug)
        app_id = str(app_item.get("id", "")).strip() or existing_app_id or generate_id()

        app_rows.append(
            {
                "Id": app_id,
                "Name": name,
                "Slug": slug,
                "OwnerTeam": str(app_item.get("ownerTeam", "")).strip(),
                "RepoUrl": str(app_item.get("repoUrl", "")).strip(),
                "DefaultEnvironment": str(app_item.get("defaultEnvironment", "")).strip(),
                "Enabled": 1 if parse_bool(app_item.get("enabled", True), True) else 0,
                "MetadataJson": safe_json_dumps(app_item.get("metadata", {})),
                "IsDeleted": 0,
                "Version": now_version,
                "CreatedAt": now_iso(),
                "UpdatedAt": now_iso(),
            }
        )

        releases = app_item.get("releases", [])
        if not isinstance(releases, list):
            continue
        for rel in releases:
            if not isinstance(rel, dict):
                continue
            rel_version = str(rel.get("version", "")).strip()
            if not rel_version:
                continue

            commit_sha = str(rel.get("commitSha", "")).strip()
            environment = str(rel.get("environment", "")).strip()
            existing_rel_id = find_existing_release_id(app_id, rel_version, commit_sha, environment)
            rel_id = str(rel.get("id", "")).strip() or existing_rel_id or generate_id()

            release_rows.append(
                {
                    "Id": rel_id,
                    "AppId": app_id,
                    "ReleaseVersion": rel_version,
                    "CommitSha": commit_sha,
                    "BuildId": str(rel.get("buildId", "")).strip(),
                    "Environment": environment,
                    "ReleasedAt": str(rel.get("releasedAt", "")).strip() or now_iso(),
                    "MetadataJson": safe_json_dumps(rel.get("metadata", {})),
                    "IsDeleted": 0,
                    "Version": now_version,
                }
            )

            artifacts = rel.get("artifacts", [])
            if not isinstance(artifacts, list):
                continue
            for art in artifacts:
                if not isinstance(art, dict):
                    continue
                artifact_type = str(art.get("artifactType", "")).strip()
                artifact_name = str(art.get("name", "")).strip()
                if not artifact_type or not artifact_name:
                    continue

                artifact_rows.append(
                    {
                        "Id": str(art.get("id", "")).strip() or generate_id(),
                        "ReleaseId": rel_id,
                        "ArtifactType": artifact_type,
                        "Name": artifact_name,
                        "ContentType": str(art.get("contentType", "")).strip(),
                        "Size": int(art.get("size", 0) or 0),
                        "StorageRef": str(art.get("storageRef", "")).strip(),
                        "ChecksumSha256": str(art.get("checksumSha256", "")).strip(),
                        "Platform": str(art.get("platform", "")).strip(),
                        "Architecture": str(art.get("architecture", "")).strip(),
                        "MetadataJson": safe_json_dumps(art.get("metadata", {})),
                        "UploadedAt": str(art.get("uploadedAt", "")).strip() or now_iso(),
                        "IsDeleted": 0,
                        "Version": now_version,
                    }
                )

    return app_rows, release_rows, artifact_rows
