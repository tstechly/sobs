from __future__ import annotations

from collections.abc import Mapping
from typing import Any


def _cve_finding_severity(vulnerability: Mapping[str, Any]) -> str:
    severity_entries = vulnerability.get("severity", [])
    if isinstance(severity_entries, list) and severity_entries:
        first_entry = severity_entries[0]
        if isinstance(first_entry, Mapping):
            severity = first_entry.get("score", "") or first_entry.get("type", "")
            if severity:
                return str(severity)

    db_specific = vulnerability.get("database_specific", {})
    if isinstance(db_specific, Mapping) and db_specific.get("severity"):
        return str(db_specific["severity"])

    return ""


def _build_osv_cve_findings(
    library: Mapping[str, Any],
    vulnerabilities: list[dict[str, Any]],
    *,
    scan_ts: str,
    max_vulns_per_pkg: int,
) -> list[dict[str, Any]]:
    findings: list[dict[str, Any]] = []
    package = str(library.get("package") or "")
    ecosystem = str(library.get("ecosystem") or "")
    version = str(library.get("version") or "")
    service_name = str(library.get("service") or "")

    for vulnerability in vulnerabilities[:max_vulns_per_pkg]:
        aliases = vulnerability.get("aliases", [])
        cve_ids = [alias for alias in aliases if isinstance(alias, str) and alias.startswith("CVE-")]
        findings.append(
            {
                "Package": package,
                "Ecosystem": ecosystem,
                "Version": version,
                "ServiceName": service_name,
                "OsvId": str(vulnerability.get("id", "")),
                "CveIds": ",".join(cve_ids),
                "Summary": str(vulnerability.get("summary", "") or "")[:500],
                "Severity": _cve_finding_severity(vulnerability),
                "Published": str(vulnerability.get("published", "") or "")[:10],
                "ScannedAt": scan_ts,
            }
        )

    return findings


def _build_cve_scan_summary(
    github_backfill: Mapping[str, Any],
    *,
    libraries_found: int,
    vulns_found: int,
    max_releases_default: int,
    scan_ts: str | None = None,
) -> dict[str, Any]:
    summary: dict[str, Any] = {
        "ok": True,
        "libraries_found": libraries_found,
        "vulns_found": vulns_found,
        "github_backfill_attempted": github_backfill.get("attempted", 0),
        "github_backfill_inserted": github_backfill.get("inserted", 0),
        "github_backfill_max_releases": github_backfill.get("max_releases", max_releases_default),
    }
    if scan_ts is not None:
        summary["scanned_at"] = scan_ts
    return summary
