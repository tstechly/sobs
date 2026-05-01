from __future__ import annotations

from collections.abc import Iterable, Mapping
from typing import Any

_SOURCE_ORDER = {"release_registry": 0, "otel_sdk": 1, "otel_scope": 2}


def _effective_cve_disposition(
    raw_disposition: str,
    package: str,
    ecosystem: str,
    version: str,
    versions_by_package: Mapping[str, set[str]],
) -> tuple[str, bool]:
    disposition = str(raw_disposition or "open")
    if disposition != "fixed":
        return disposition, False
    current_versions = versions_by_package.get(f"{ecosystem}::{package}", set())
    if any(current_version != version for current_version in current_versions):
        return "open", True
    return disposition, False


def _build_library_api_payload(
    inventory: Iterable[Mapping[str, Any]],
    cve_rows: Iterable[Any],
    *,
    scanned_at: str,
) -> dict[str, Any]:
    cve_count_by_key = {f"{str(row[0])}::{str(row[1])}::{str(row[2])}": int(row[3]) for row in cve_rows}

    libraries: list[dict[str, Any]] = []
    for item in inventory:
        package = str(item.get("package") or "")
        ecosystem = str(item.get("ecosystem") or "")
        version = str(item.get("version") or "")
        service = str(item.get("service") or item.get("app_name") or "")
        source = str(item.get("source") or "")
        cve_count = cve_count_by_key.get(f"{package}::{ecosystem}::{version}", 0)
        if not ecosystem:
            status = "unknown_ecosystem"
        elif cve_count > 0:
            status = "vulnerable"
        else:
            status = "clean"
        libraries.append(
            {
                "package": package,
                "ecosystem": ecosystem,
                "version": version,
                "service": service,
                "source": source,
                "app_name": str(item.get("app_name") or ""),
                "release_version": str(item.get("release_version") or ""),
                "environment": str(item.get("environment") or ""),
                "cve_count": cve_count,
                "status": status,
            }
        )

    libraries.sort(
        key=lambda item: (
            -int(item.get("cve_count", 0)),
            _SOURCE_ORDER.get(str(item.get("source") or ""), 99),
            str(item.get("package") or "").lower(),
            str(item.get("version") or "").lower(),
            str(item.get("service") or "").lower(),
        )
    )

    return {"ok": True, "libraries": libraries, "scanned_at": scanned_at}


def _build_dispositions_by_key(disposition_rows: Iterable[Any]) -> dict[str, dict[str, str]]:
    return {
        f"{str(row[0])}::{str(row[1])}::{str(row[2])}::{str(row[3])}": {
            "disposition": str(row[4] or "open"),
            "note": str(row[5] or ""),
        }
        for row in disposition_rows
    }


def _serialize_cve_findings(
    rows: Iterable[Any],
    *,
    dispositions_by_key: Mapping[str, Mapping[str, str]],
    versions_by_package: Mapping[str, set[str]],
    show_all: bool,
) -> list[dict[str, Any]]:
    findings: list[dict[str, Any]] = []
    for row in rows:
        finding_key = f"{str(row[4])}::{str(row[0])}::{str(row[1])}::{str(row[2])}"
        disposition_data = dispositions_by_key.get(finding_key, {})
        raw_disposition = str(disposition_data.get("disposition", "open") or "open")
        disposition, disposition_expired = _effective_cve_disposition(
            raw_disposition,
            str(row[0]),
            str(row[1]),
            str(row[2]),
            versions_by_package,
        )
        if (not show_all) and disposition in ("accepted", "false_positive", "fixed"):
            continue
        findings.append(
            {
                "package": str(row[0]),
                "ecosystem": str(row[1]),
                "version": str(row[2]),
                "service": str(row[3]),
                "osv_id": str(row[4]),
                "cve_ids": [cve for cve in str(row[5]).split(",") if cve],
                "summary": str(row[6]),
                "severity": str(row[7]),
                "published": str(row[8]),
                "disposition": disposition,
                "raw_disposition": raw_disposition,
                "disposition_expired": disposition_expired,
                "disposition_note": str(disposition_data.get("note", "") or ""),
            }
        )
    return findings


def _filter_cve_findings(
    findings: list[dict[str, Any]],
    *,
    selected_severities: list[str],
    selected_ecosystems: list[str],
    package_filter: str,
    show_all: bool,
) -> tuple[list[dict[str, Any]], list[str], list[str]]:
    ecosystems = sorted({str(finding["ecosystem"]) for finding in findings if finding.get("ecosystem")})
    severities = sorted({str(finding["severity"]) for finding in findings if finding.get("severity")})

    filtered = list(findings)
    if selected_severities:
        selected_severity_set = set(selected_severities)
        filtered = [finding for finding in filtered if finding["severity"] in selected_severity_set]
    if selected_ecosystems:
        selected_ecosystem_set = set(selected_ecosystems)
        filtered = [finding for finding in filtered if finding["ecosystem"] in selected_ecosystem_set]
    if package_filter:
        package_filter_lower = package_filter.lower()
        filtered = [finding for finding in filtered if package_filter_lower in finding["package"].lower()]
    if not show_all:
        filtered = [
            finding
            for finding in filtered
            if finding.get("disposition", "open") not in ("accepted", "false_positive", "fixed")
        ]

    return filtered, ecosystems, severities
