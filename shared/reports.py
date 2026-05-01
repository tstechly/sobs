from __future__ import annotations

import json
from collections.abc import Mapping
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
