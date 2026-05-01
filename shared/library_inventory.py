from __future__ import annotations

import hashlib
import json
import os
import re
from collections.abc import Callable, Iterable, Mapping
from typing import Any

from shared.github_issues import _safe_json_loads

_SOURCE_PRIORITY = {"release_registry": 0, "otel_sdk": 1, "otel_scope": 2}


def _github_actions_snapshot_name(filename: str) -> tuple[str, str, str] | None:
    base = os.path.basename(str(filename or "").strip())
    if not base:
        return None
    match = re.match(r"^pip-freeze-([a-z0-9_-]+)-([a-z0-9_-]+)\.txt$", base, re.IGNORECASE)
    if not match:
        return None
    platform = match.group(1).lower()
    architecture = match.group(2).lower()
    return f"pip-freeze-{platform}-{architecture}", platform, architecture


def _build_github_actions_dependency_row(
    *,
    record_id: str,
    release_id: str,
    owner: str,
    repo: str,
    run_id: str,
    run_head_sha: str,
    artifact_id: str,
    artifact_name: str,
    filename: str,
    release_version: str,
    platform: str,
    architecture: str,
    raw_bytes: bytes,
    dependencies: list[dict[str, str]],
    uploaded_at: str,
    version: int,
) -> dict[str, Any]:
    return {
        "Id": record_id,
        "ReleaseId": release_id,
        "ArtifactType": "dependencies-lockfile",
        "Name": f"pip-freeze-{platform}-{architecture}",
        "ContentType": "text/plain",
        "Size": len(raw_bytes),
        "StorageRef": (
            f"github-actions://{owner}/{repo}/runs/{run_id}" f"/artifacts/{artifact_id}/{os.path.basename(filename)}"
        ),
        "ChecksumSha256": hashlib.sha256(raw_bytes).hexdigest(),
        "Platform": platform,
        "Architecture": architecture,
        "MetadataJson": json.dumps(
            {
                "source": "github_actions_artifact",
                "repo": f"{owner}/{repo}",
                "run_id": run_id,
                "run_head_sha": run_head_sha,
                "release_version": release_version,
                "artifact_name": artifact_name,
                "dependencies": dependencies,
            },
            separators=(",", ":"),
        ),
        "UploadedAt": uploaded_at,
        "IsDeleted": 0,
        "Version": version,
    }


def _build_release_registry_inventory_items(
    artifact_rows: Iterable[Mapping[str, Any]],
    release_rows: Iterable[Mapping[str, Any]],
    app_rows: Iterable[Mapping[str, Any]],
) -> list[dict[str, str]]:
    releases_by_id = {
        str(row["Id"]): {
            "app_id": str(row["AppId"] or ""),
            "release_version": str(row["ReleaseVersion"] or ""),
            "environment": str(row["Environment"] or ""),
        }
        for row in release_rows
    }
    apps_by_id = {str(row["Id"]): {"name": str(row["Name"] or ""), "slug": str(row["Slug"] or "")} for row in app_rows}

    items: list[dict[str, str]] = []
    for row in artifact_rows:
        release_info = releases_by_id.get(str(row.get("ReleaseId") or ""), {})
        app_info = apps_by_id.get(str(release_info.get("app_id") or ""), {})
        metadata = _safe_json_loads(row.get("MetadataJson", ""), {})
        dependencies = metadata.get("dependencies", []) if isinstance(metadata, dict) else []
        if not isinstance(dependencies, list):
            continue
        for dependency in dependencies:
            if not isinstance(dependency, dict):
                continue
            app_name = str(app_info.get("name") or app_info.get("slug") or "")
            items.append(
                {
                    "package": str(dependency.get("package", dependency.get("name", "")) or ""),
                    "version": str(dependency.get("version", "") or ""),
                    "ecosystem": str(dependency.get("ecosystem", "") or ""),
                    "service": app_name,
                    "source": "release_registry",
                    "app_name": app_name,
                    "release_version": str(release_info.get("release_version") or ""),
                    "environment": str(release_info.get("environment") or ""),
                }
            )
    return items


def _build_sdk_inventory_items(
    rows: Iterable[Any],
    *,
    lang_to_osv_ecosystem: Callable[[str], str],
    source: str = "otel_sdk",
) -> list[dict[str, str]]:
    return [
        {
            "package": str(row[0] or ""),
            "version": str(row[1] or ""),
            "ecosystem": lang_to_osv_ecosystem(str(row[2] or "")),
            "service": str(row[3] or ""),
            "source": source,
        }
        for row in rows
    ]


def _build_scope_inventory_items(
    rows: Iterable[Any],
    *,
    inventory_scope_ecosystem: Callable[[str], str],
    source: str = "otel_scope",
) -> list[dict[str, str]]:
    items: list[dict[str, str]] = []
    for row in rows:
        scope_name = str(row[0] or "")
        items.append(
            {
                "package": scope_name,
                "version": str(row[1] or ""),
                "ecosystem": inventory_scope_ecosystem(scope_name),
                "service": str(row[2] or ""),
                "source": source,
            }
        )
    return items


def _merge_library_inventory(items: Iterable[Mapping[str, str]]) -> list[dict[str, str]]:
    inventory: dict[str, dict[str, str]] = {}

    for item in items:
        package = str(item.get("package") or "").strip()
        version = str(item.get("version") or "").strip()
        if not package or not version:
            continue
        normalized = {
            "package": package,
            "version": version,
            "ecosystem": str(item.get("ecosystem") or "").strip(),
            "service": str(item.get("service") or "").strip(),
            "source": str(item.get("source") or "").strip(),
            "app_name": str(item.get("app_name") or "").strip(),
            "release_version": str(item.get("release_version") or "").strip(),
            "environment": str(item.get("environment") or "").strip(),
        }
        service_label = normalized["service"] or normalized["app_name"]
        item_key = "::".join([normalized["ecosystem"], normalized["package"], normalized["version"], service_label])
        current = inventory.get(item_key)
        if not current:
            inventory[item_key] = normalized
            continue
        if _SOURCE_PRIORITY.get(normalized["source"], 99) < _SOURCE_PRIORITY.get(current.get("source", ""), 99):
            inventory[item_key] = normalized

    return list(inventory.values())


def _extract_library_versions_from_inventory(inventory: Iterable[Mapping[str, str]]) -> list[dict[str, str]]:
    return [
        {
            "package": str(item.get("package") or ""),
            "version": str(item.get("version") or ""),
            "ecosystem": str(item.get("ecosystem") or ""),
            "service": str(item.get("service") or item.get("app_name") or ""),
        }
        for item in inventory
    ]


def _inventory_versions_by_package_from_inventory(
    inventory: Iterable[Mapping[str, str]],
) -> dict[str, set[str]]:
    versions_by_package: dict[str, set[str]] = {}
    for item in inventory:
        package = str(item.get("package") or "").strip()
        ecosystem = str(item.get("ecosystem") or "").strip()
        version = str(item.get("version") or "").strip()
        if not package or not ecosystem or not version:
            continue
        versions_by_package.setdefault(f"{ecosystem}::{package}", set()).add(version)
    return versions_by_package
