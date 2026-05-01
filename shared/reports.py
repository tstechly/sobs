from __future__ import annotations

import json
from collections.abc import Callable, Iterable, Mapping
from typing import Any

REPORT_PAGE_TYPES = {"logs", "traces", "errors", "metrics", "rum", "ai", "work_items", "web_traffic"}
REPORTS_EXPORT_VERSION = "1"


def _parse_report_filters(raw_filters_json: Any) -> dict[str, Any]:
    if not raw_filters_json:
        return {}
    try:
        parsed = json.loads(str(raw_filters_json))
    except (TypeError, ValueError):
        return {}
    return parsed if isinstance(parsed, dict) else {}


def _serialize_report_row(row) -> dict[str, Any]:
    return {
        "id": str(row["Id"]),
        "name": str(row["Name"]),
        "description": str(row["Description"]),
        "page_type": str(row["PageType"]),
        "filters": _parse_report_filters(row["FiltersJson"]),
    }


def _get_reports(db, page_type: str | None = None) -> list[dict[str, Any]]:
    if page_type:
        rows = db.execute(
            "SELECT Id, Name, Description, PageType, FiltersJson "
            "FROM sobs_reports FINAL WHERE IsDeleted = 0 AND PageType = ? ORDER BY Name",
            [page_type],
        ).fetchall()
    else:
        rows = db.execute(
            "SELECT Id, Name, Description, PageType, FiltersJson "
            "FROM sobs_reports FINAL WHERE IsDeleted = 0 ORDER BY PageType, Name"
        ).fetchall()
    return [_serialize_report_row(row) for row in rows]


def _get_report(db, report_id: str) -> dict[str, Any] | None:
    row = db.execute(
        "SELECT Id, Name, Description, PageType, FiltersJson " "FROM sobs_reports FINAL WHERE IsDeleted = 0 AND Id = ?",
        [report_id],
    ).fetchone()
    if not row:
        return None
    return _serialize_report_row(row)


def _build_report_record(
    report_id: str,
    name: str,
    description: str,
    page_type: str,
    filters: Mapping[str, object],
    *,
    version: int,
    is_deleted: int = 0,
) -> dict[str, object]:
    return {
        "Id": report_id,
        "Name": name,
        "Description": description,
        "PageType": page_type,
        "FiltersJson": json.dumps(filters, ensure_ascii=False),
        "IsDeleted": is_deleted,
        "Version": version,
    }


def _build_reports_export_payload(
    reports: list[Mapping[str, Any]],
    *,
    exported_at: str,
    version: str = REPORTS_EXPORT_VERSION,
) -> dict[str, object]:
    return {
        "sobs_reports_export": True,
        "version": version,
        "exported_at": exported_at,
        "reports": [
            {
                "id": str(report["id"]),
                "name": str(report["name"]),
                "description": str(report["description"]),
                "page_type": str(report["page_type"]),
                "filters": report["filters"],
            }
            for report in reports
        ],
    }


def _validate_reports_import_payload(
    body: Any,
    on_conflict: str,
    *,
    expected_version: str = REPORTS_EXPORT_VERSION,
    max_reports: int,
) -> tuple[list[Any] | None, str | None]:
    if not isinstance(body, dict) or not body.get("sobs_reports_export"):
        return None, "Not a valid SOBS reports export file"
    if str(body.get("version", "")) != expected_version:
        return None, f"Unsupported export version: {body.get('version')!r}"
    if on_conflict not in ("rename", "replace", "skip"):
        return None, "on_conflict must be one of: rename, replace, skip"

    incoming = body.get("reports")
    if not isinstance(incoming, list):
        return None, "'reports' must be a list"
    if len(incoming) > max_reports:
        return None, f"Too many reports (max {max_reports})"
    return incoming, None


def _build_reports_existing_index(
    reports: Iterable[Mapping[str, Any]],
) -> dict[tuple[str, str], dict[str, Any]]:
    return {(str(report["page_type"]), str(report["name"]).lower()): dict(report) for report in reports}


def _normalize_import_report_item(
    item: Any,
    *,
    report_page_types: set[str] = REPORT_PAGE_TYPES,
) -> tuple[str, str, str, dict[str, Any]] | None:
    if not isinstance(item, dict):
        return None

    name = str(item.get("name") or "").strip()
    description = str(item.get("description") or "").strip()
    page_type = str(item.get("page_type") or "").strip()
    filters = item.get("filters") or {}

    if not name or page_type not in report_page_types or not isinstance(filters, dict):
        return None
    return name, description, page_type, filters


def _next_imported_report_name(
    name: str,
    page_type: str,
    existing_index: Mapping[tuple[str, str], Mapping[str, Any]],
) -> str:
    candidate = f"{name} (imported)"
    suffix = 2
    while (page_type, candidate.lower()) in existing_index:
        candidate = f"{name} (imported {suffix})"
        suffix += 1
    return candidate


def _plan_reports_import(
    incoming: list[Any],
    existing_reports: Iterable[Mapping[str, Any]],
    *,
    on_conflict: str,
    version_base: int,
    uuid_factory: Callable[[], object],
) -> tuple[list[dict[str, object]], dict[str, int]]:
    existing_index = _build_reports_existing_index(existing_reports)
    rows_to_insert: list[dict[str, object]] = []
    n_imported = 0
    n_skipped = 0
    n_replaced = 0
    n_errors = 0

    for idx, item in enumerate(incoming):
        normalized = _normalize_import_report_item(item)
        if normalized is None:
            n_errors += 1
            continue

        name, description, page_type, filters = normalized
        conflict_key = (page_type, name.lower())
        conflict = existing_index.get(conflict_key)

        if conflict:
            if on_conflict == "skip":
                n_skipped += 1
                continue
            if on_conflict == "replace":
                conflict_filters_raw = conflict.get("filters")
                conflict_filters: dict[str, object] = (
                    conflict_filters_raw if isinstance(conflict_filters_raw, dict) else {}
                )
                rows_to_insert.append(
                    _build_report_record(
                        str(conflict["id"]),
                        str(conflict["name"]),
                        str(conflict.get("description") or ""),
                        str(conflict["page_type"]),
                        conflict_filters,
                        version=version_base + idx * 2,
                        is_deleted=1,
                    )
                )
                n_replaced += 1
                del existing_index[conflict_key]
            else:
                name = _next_imported_report_name(name, page_type, existing_index)

        new_id = str(uuid_factory())
        inserted_report = {
            "id": new_id,
            "name": name,
            "description": description,
            "page_type": page_type,
            "filters": filters,
        }
        rows_to_insert.append(
            _build_report_record(
                new_id,
                name,
                description,
                page_type,
                filters,
                version=version_base + idx * 2 + 1,
            )
        )
        existing_index[(page_type, name.lower())] = inserted_report

        if conflict and on_conflict == "replace":
            continue
        n_imported += 1

    return rows_to_insert, {
        "imported": n_imported,
        "skipped": n_skipped,
        "replaced": n_replaced,
        "errors": n_errors,
    }
