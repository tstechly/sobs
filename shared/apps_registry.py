from __future__ import annotations

import json
import re
from typing import Any, cast, overload

from shared.github import _parse_github_repo_owner_name


def _safe_json_dumps(value: Any) -> str:
    if value is None:
        return "{}"
    if isinstance(value, str):
        stripped = value.strip()
        if not stripped:
            return "{}"
        try:
            parsed = json.loads(stripped)
            return json.dumps(parsed, ensure_ascii=False)
        except Exception:
            return "{}"
    if isinstance(value, (dict, list)):
        return json.dumps(value, ensure_ascii=False)
    return "{}"


@overload
def _safe_json_loads(value: object, default: dict[str, Any]) -> dict[str, Any]: ...


@overload
def _safe_json_loads(value: object, default: list[Any]) -> list[Any]: ...


def _safe_json_loads(value: object, default: Any) -> Any:
    raw = str(value or "").strip()
    if not raw:
        return default
    try:
        parsed = json.loads(raw)
    except Exception:
        return default
    if isinstance(default, dict) and isinstance(parsed, dict):
        return cast(dict[str, Any], parsed)
    if isinstance(default, list) and isinstance(parsed, list):
        return cast(list[Any], parsed)
    return default


def _app_slug(value: str, fallback: str = "app") -> str:
    slug = re.sub(r"[^a-z0-9]+", "-", str(value or "").strip().lower()).strip("-")
    return (slug or fallback)[:80]


def _find_app_by_id(db: Any, app_id: str) -> dict[str, Any] | None:
    row = db.execute(
        "SELECT * FROM sobs_apps FINAL WHERE Id=? AND IsDeleted=0 LIMIT 1",
        [app_id],
    ).fetchone()
    return dict(row) if row else None


def _find_app_id_by_repo_url(db: Any, repo_url: str) -> str:
    normalized_input = str(repo_url or "").strip()
    if not normalized_input:
        return ""
    input_owner, input_repo = _parse_github_repo_owner_name(normalized_input)
    if not input_owner or not input_repo:
        return ""

    rows = db.execute("SELECT Id, RepoUrl FROM sobs_apps FINAL WHERE IsDeleted=0").fetchall()
    for row in rows:
        owner, repo = _parse_github_repo_owner_name(str(row["RepoUrl"] or ""))
        if owner.lower() == input_owner.lower() and repo.lower() == input_repo.lower():
            return str(row["Id"] or "")
    return ""


def _find_release_by_id(db: Any, release_id: str) -> dict[str, Any] | None:
    row = db.execute(
        "SELECT * FROM sobs_app_releases FINAL WHERE Id=? AND IsDeleted=0 LIMIT 1",
        [release_id],
    ).fetchone()
    return dict(row) if row else None


def _serialize_app_row(row: dict[str, Any]) -> dict[str, Any]:
    return {
        "id": str(row.get("Id", "")),
        "name": str(row.get("Name", "")),
        "slug": str(row.get("Slug", "")),
        "ownerTeam": str(row.get("OwnerTeam", "")),
        "repoUrl": str(row.get("RepoUrl", "")),
        "defaultEnvironment": str(row.get("DefaultEnvironment", "")),
        "enabled": bool(int(row.get("Enabled", 1) or 0)),
        "metadata": _safe_json_loads(row.get("MetadataJson", ""), {}),
        "createdAt": str(row.get("CreatedAt", "")),
        "updatedAt": str(row.get("UpdatedAt", "")),
    }
