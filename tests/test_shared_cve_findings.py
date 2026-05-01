from shared.cve_findings import (
    _build_dispositions_by_key,
    _build_library_api_payload,
    _effective_cve_disposition,
    _filter_cve_findings,
    _serialize_cve_findings,
)


def test_effective_cve_disposition_returns_non_fixed_as_is() -> None:
    disposition, expired = _effective_cve_disposition(
        "accepted",
        "requests",
        "PyPI",
        "2.32.3",
        {"PyPI::requests": {"2.32.3"}},
    )

    assert disposition == "accepted"
    assert expired is False


def test_effective_cve_disposition_expires_fixed_when_new_version_present() -> None:
    disposition, expired = _effective_cve_disposition(
        "fixed",
        "requests",
        "PyPI",
        "2.31.0",
        {"PyPI::requests": {"2.31.0", "2.32.3"}},
    )

    assert disposition == "open"
    assert expired is True


def test_effective_cve_disposition_keeps_fixed_when_only_same_version_present() -> None:
    disposition, expired = _effective_cve_disposition(
        "fixed",
        "requests",
        "PyPI",
        "2.31.0",
        {"PyPI::requests": {"2.31.0"}},
    )

    assert disposition == "fixed"
    assert expired is False


def test_build_library_api_payload_assigns_statuses_and_sorts() -> None:
    inventory = [
        {
            "package": "boto3",
            "ecosystem": "PyPI",
            "version": "1.35.0",
            "service": "svc-b",
            "source": "otel_sdk",
        },
        {
            "package": "requests",
            "ecosystem": "PyPI",
            "version": "2.32.3",
            "service": "svc-a",
            "source": "release_registry",
            "app_name": "svc-a",
            "release_version": "2026.04.07",
            "environment": "prod",
        },
        {
            "package": "mystery",
            "ecosystem": "",
            "version": "0.1.0",
            "app_name": "svc-c",
            "source": "otel_scope",
        },
    ]
    cve_rows = [("requests", "PyPI", "2.32.3", 2)]

    payload = _build_library_api_payload(inventory, cve_rows, scanned_at="2026-04-07T12:00:00Z")

    assert payload["ok"] is True
    assert payload["scanned_at"] == "2026-04-07T12:00:00Z"
    assert [item["package"] for item in payload["libraries"]] == ["requests", "boto3", "mystery"]
    assert payload["libraries"][0]["status"] == "vulnerable"
    assert payload["libraries"][1]["status"] == "clean"
    assert payload["libraries"][2]["status"] == "unknown_ecosystem"
    assert payload["libraries"][2]["service"] == "svc-c"


def test_build_dispositions_by_key_maps_rows() -> None:
    dispositions = _build_dispositions_by_key([("OSV-1", "requests", "PyPI", "2.32.3", "accepted", "known issue")])

    assert dispositions == {"OSV-1::requests::PyPI::2.32.3": {"disposition": "accepted", "note": "known issue"}}


def test_serialize_cve_findings_skips_triaged_when_show_all_disabled() -> None:
    rows = [
        (
            "requests",
            "PyPI",
            "2.32.3",
            "svc-a",
            "OSV-1",
            "CVE-1,CVE-2",
            "summary one",
            "HIGH",
            "2026-04-01",
        ),
        (
            "urllib3",
            "PyPI",
            "2.2.2",
            "svc-b",
            "OSV-2",
            "",
            "summary two",
            "MEDIUM",
            "2026-04-02",
        ),
    ]
    dispositions_by_key = {
        "OSV-1::requests::PyPI::2.32.3": {"disposition": "accepted", "note": "tracked"},
        "OSV-2::urllib3::PyPI::2.2.2": {"disposition": "fixed", "note": "upgraded"},
    }

    findings = _serialize_cve_findings(
        rows,
        dispositions_by_key=dispositions_by_key,
        versions_by_package={"PyPI::urllib3": {"2.2.2", "2.3.0"}},
        show_all=False,
    )

    assert findings == [
        {
            "package": "urllib3",
            "ecosystem": "PyPI",
            "version": "2.2.2",
            "service": "svc-b",
            "osv_id": "OSV-2",
            "cve_ids": [],
            "summary": "summary two",
            "severity": "MEDIUM",
            "published": "2026-04-02",
            "disposition": "open",
            "raw_disposition": "fixed",
            "disposition_expired": True,
            "disposition_note": "upgraded",
        }
    ]


def test_serialize_cve_findings_keeps_triaged_when_show_all_enabled() -> None:
    rows = [
        (
            "requests",
            "PyPI",
            "2.32.3",
            "svc-a",
            "OSV-1",
            "CVE-1,CVE-2",
            "summary one",
            "HIGH",
            "2026-04-01",
        )
    ]
    findings = _serialize_cve_findings(
        rows,
        dispositions_by_key={"OSV-1::requests::PyPI::2.32.3": {"disposition": "accepted", "note": "tracked"}},
        versions_by_package={},
        show_all=True,
    )

    assert findings[0]["cve_ids"] == ["CVE-1", "CVE-2"]
    assert findings[0]["disposition"] == "accepted"
    assert findings[0]["disposition_expired"] is False


def test_filter_cve_findings_applies_filters_and_show_all() -> None:
    findings = [
        {
            "package": "requests",
            "ecosystem": "PyPI",
            "severity": "HIGH",
            "disposition": "open",
        },
        {
            "package": "urllib3",
            "ecosystem": "PyPI",
            "severity": "LOW",
            "disposition": "accepted",
        },
        {
            "package": "leftpad",
            "ecosystem": "npm",
            "severity": "CRITICAL",
            "disposition": "open",
        },
    ]

    filtered, ecosystems, severities = _filter_cve_findings(
        findings,
        selected_severities=["HIGH", "LOW"],
        selected_ecosystems=["PyPI"],
        package_filter="req",
        show_all=False,
    )

    assert ecosystems == ["PyPI", "npm"]
    assert severities == ["CRITICAL", "HIGH", "LOW"]
    assert filtered == [
        {
            "package": "requests",
            "ecosystem": "PyPI",
            "severity": "HIGH",
            "disposition": "open",
        }
    ]
