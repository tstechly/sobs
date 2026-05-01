from __future__ import annotations

from shared.cve_scan import _build_cve_scan_summary, _build_osv_cve_findings, _cve_finding_severity


def test_cve_finding_severity_prefers_score_then_type_then_database_specific():
    assert _cve_finding_severity({"severity": [{"score": "9.8", "type": "CVSS_V3"}]}) == "9.8"
    assert _cve_finding_severity({"severity": [{"type": "CVSS_V4"}]}) == "CVSS_V4"
    assert _cve_finding_severity({"database_specific": {"severity": "HIGH"}}) == "HIGH"
    assert _cve_finding_severity({"severity": ["bad-shape"], "database_specific": {}}) == ""


def test_build_osv_cve_findings_limits_rows_and_extracts_cve_ids_and_fields():
    findings = _build_osv_cve_findings(
        {
            "package": "requests",
            "ecosystem": "PyPI",
            "version": "2.32.3",
            "service": "checkout",
        },
        [
            {
                "id": "OSV-1",
                "aliases": ["CVE-2026-0001", "GHSA-123"],
                "summary": "A" * 600,
                "severity": [{"score": "7.5"}],
                "published": "2026-05-01T12:34:56Z",
            },
            {
                "id": "OSV-2",
                "aliases": ["CVE-2026-0002"],
                "database_specific": {"severity": "MEDIUM"},
                "published": "2026-05-02",
            },
        ],
        scan_ts="2026-05-03T00:00:00Z",
        max_vulns_per_pkg=1,
    )

    assert findings == [
        {
            "Package": "requests",
            "Ecosystem": "PyPI",
            "Version": "2.32.3",
            "ServiceName": "checkout",
            "OsvId": "OSV-1",
            "CveIds": "CVE-2026-0001",
            "Summary": "A" * 500,
            "Severity": "7.5",
            "Published": "2026-05-01",
            "ScannedAt": "2026-05-03T00:00:00Z",
        }
    ]


def test_build_cve_scan_summary_includes_optional_scan_timestamp():
    summary = _build_cve_scan_summary(
        {"attempted": 4, "inserted": 2},
        libraries_found=3,
        vulns_found=5,
        max_releases_default=25,
    )
    assert summary == {
        "ok": True,
        "libraries_found": 3,
        "vulns_found": 5,
        "github_backfill_attempted": 4,
        "github_backfill_inserted": 2,
        "github_backfill_max_releases": 25,
    }

    scanned = _build_cve_scan_summary(
        {"attempted": 1, "inserted": 1, "max_releases": 77},
        libraries_found=2,
        vulns_found=1,
        max_releases_default=25,
        scan_ts="2026-05-03T00:00:00Z",
    )
    assert scanned["scanned_at"] == "2026-05-03T00:00:00Z"
    assert scanned["github_backfill_max_releases"] == 77
