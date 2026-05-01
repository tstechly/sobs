from __future__ import annotations

import json

from shared.reports import (
    REPORTS_EXPORT_VERSION,
    _build_report_record,
    _build_reports_export_payload,
    _get_report,
    _get_reports,
    _parse_report_filters,
    _plan_reports_import,
    _serialize_report_row,
    _validate_reports_import_payload,
)


class _FakeResult:
    def __init__(self, rows, one=None):
        self._rows = rows
        self._one = one

    def fetchall(self):
        return self._rows

    def fetchone(self):
        return self._one


class _FakeDb:
    def __init__(self, rows=None, one=None):
        self.rows = rows or []
        self.one = one
        self.calls: list[tuple[str, list[str] | None]] = []

    def execute(self, query, params=None):
        self.calls.append((query, params))
        return _FakeResult(self.rows, self.one)


def test_parse_report_filters_and_serialize_report_row_handle_invalid_and_valid_data():
    assert _parse_report_filters(None) == {}
    assert _parse_report_filters("{bad") == {}
    assert _parse_report_filters('["not-a-dict"]') == {}
    assert _parse_report_filters('{"service":"api"}') == {"service": "api"}

    row = _serialize_report_row(
        {
            "Id": 7,
            "Name": "Errors",
            "Description": "Recent errors",
            "PageType": "logs",
            "FiltersJson": '{"level":"ERROR"}',
        }
    )
    assert row == {
        "id": "7",
        "name": "Errors",
        "description": "Recent errors",
        "page_type": "logs",
        "filters": {"level": "ERROR"},
    }


def test_get_reports_and_get_report_issue_expected_queries():
    db = _FakeDb(
        rows=[
            {
                "Id": 1,
                "Name": "Errors",
                "Description": "Recent errors",
                "PageType": "logs",
                "FiltersJson": '{"level":"ERROR"}',
            }
        ]
    )
    assert _get_reports(db) == [
        {
            "id": "1",
            "name": "Errors",
            "description": "Recent errors",
            "page_type": "logs",
            "filters": {"level": "ERROR"},
        }
    ]
    assert "sobs_reports" in db.calls[0][0]

    filtered_db = _FakeDb(rows=[])
    _get_reports(filtered_db, "logs")
    assert filtered_db.calls[0][1] == ["logs"]

    one_db = _FakeDb(
        one={
            "Id": 2,
            "Name": "Trace Report",
            "Description": "Trace filters",
            "PageType": "traces",
            "FiltersJson": '{"service":"checkout"}',
        }
    )
    assert _get_report(one_db, "rep-2") == {
        "id": "2",
        "name": "Trace Report",
        "description": "Trace filters",
        "page_type": "traces",
        "filters": {"service": "checkout"},
    }
    assert one_db.calls[0][1] == ["rep-2"]

    missing_db = _FakeDb(one=None)
    assert _get_report(missing_db, "missing") is None


def test_build_report_record_and_export_payload_normalize_output():
    record = _build_report_record(
        "rep-1",
        "Error Spike",
        "Errors only",
        "logs",
        {"level": "ERROR"},
        version=123,
        is_deleted=1,
    )
    assert record == {
        "Id": "rep-1",
        "Name": "Error Spike",
        "Description": "Errors only",
        "PageType": "logs",
        "FiltersJson": json.dumps({"level": "ERROR"}, ensure_ascii=False),
        "IsDeleted": 1,
        "Version": 123,
    }

    payload = _build_reports_export_payload(
        [
            {
                "id": "rep-1",
                "name": "Error Spike",
                "description": "Errors only",
                "page_type": "logs",
                "filters": {"level": "ERROR"},
            }
        ],
        exported_at="2026-05-01T00:00:00Z",
        version=REPORTS_EXPORT_VERSION,
    )
    assert payload == {
        "sobs_reports_export": True,
        "version": REPORTS_EXPORT_VERSION,
        "exported_at": "2026-05-01T00:00:00Z",
        "reports": [
            {
                "id": "rep-1",
                "name": "Error Spike",
                "description": "Errors only",
                "page_type": "logs",
                "filters": {"level": "ERROR"},
            }
        ],
    }


def test_validate_reports_import_payload_rejects_invalid_envelopes_and_accepts_valid_input():
    incoming, error = _validate_reports_import_payload({}, "rename", max_reports=5)
    assert incoming is None
    assert error == "Not a valid SOBS reports export file"

    incoming, error = _validate_reports_import_payload(
        {"sobs_reports_export": True, "version": "99", "reports": []},
        "rename",
        max_reports=5,
    )
    assert incoming is None
    assert error == "Unsupported export version: '99'"

    incoming, error = _validate_reports_import_payload(
        {"sobs_reports_export": True, "version": REPORTS_EXPORT_VERSION, "reports": []},
        "other",
        max_reports=5,
    )
    assert incoming is None
    assert error == "on_conflict must be one of: rename, replace, skip"

    incoming, error = _validate_reports_import_payload(
        {"sobs_reports_export": True, "version": REPORTS_EXPORT_VERSION, "reports": {}},
        "rename",
        max_reports=5,
    )
    assert incoming is None
    assert error == "'reports' must be a list"

    incoming, error = _validate_reports_import_payload(
        {"sobs_reports_export": True, "version": REPORTS_EXPORT_VERSION, "reports": [{}, {}]},
        "rename",
        max_reports=1,
    )
    assert incoming is None
    assert error == "Too many reports (max 1)"

    body = {"sobs_reports_export": True, "version": REPORTS_EXPORT_VERSION, "reports": [{}]}
    incoming, error = _validate_reports_import_payload(body, "rename", max_reports=5)
    assert incoming == [{}]
    assert error is None


def test_plan_reports_import_handles_rename_invalid_and_same_batch_replace_conflicts():
    rows, summary = _plan_reports_import(
        [
            {"name": "Conflict", "description": "new", "page_type": "logs", "filters": {"level": "WARN"}},
            {"name": "Conflict", "description": "bad", "page_type": "bad", "filters": {}},
        ],
        [
            {
                "id": "existing-1",
                "name": "Conflict",
                "description": "old",
                "page_type": "logs",
                "filters": {"level": "ERROR"},
            }
        ],
        on_conflict="rename",
        version_base=100,
        uuid_factory=lambda: "new-1",
    )
    assert summary == {"imported": 1, "skipped": 0, "replaced": 0, "errors": 1}
    assert rows == [
        {
            "Id": "new-1",
            "Name": "Conflict (imported)",
            "Description": "new",
            "PageType": "logs",
            "FiltersJson": '{"level": "WARN"}',
            "IsDeleted": 0,
            "Version": 101,
        }
    ]

    rows, summary = _plan_reports_import(
        [{"name": "Conflict", "description": "skip", "page_type": "logs", "filters": {"level": "INFO"}}],
        [{"id": "existing-skip", "name": "Conflict", "description": "old", "page_type": "logs", "filters": {}}],
        on_conflict="skip",
        version_base=125,
        uuid_factory=lambda: "unused",
    )
    assert summary == {"imported": 0, "skipped": 1, "replaced": 0, "errors": 0}
    assert rows == []

    rows, summary = _plan_reports_import(
        [
            123,
            {"name": "Conflict", "description": "next", "page_type": "logs", "filters": {"level": "INFO"}},
        ],
        [
            {"id": "existing-a", "name": "Conflict", "description": "old", "page_type": "logs", "filters": {}},
            {
                "id": "existing-b",
                "name": "Conflict (imported)",
                "description": "older",
                "page_type": "logs",
                "filters": {},
            },
        ],
        on_conflict="rename",
        version_base=150,
        uuid_factory=lambda: "new-rename-2",
    )
    assert summary == {"imported": 1, "skipped": 0, "replaced": 0, "errors": 1}
    assert rows == [
        {
            "Id": "new-rename-2",
            "Name": "Conflict (imported 2)",
            "Description": "next",
            "PageType": "logs",
            "FiltersJson": '{"level": "INFO"}',
            "IsDeleted": 0,
            "Version": 153,
        }
    ]

    uuid_values = iter(["new-2", "new-3"])
    rows, summary = _plan_reports_import(
        [
            {"name": "Replace", "description": "first", "page_type": "rum", "filters": {"device": "desktop"}},
            {"name": "Replace", "description": "second", "page_type": "rum", "filters": {"device": "tablet"}},
        ],
        [
            {
                "id": "existing-2",
                "name": "Replace",
                "description": "old",
                "page_type": "rum",
                "filters": {"device": "mobile"},
            }
        ],
        on_conflict="replace",
        version_base=200,
        uuid_factory=lambda: next(uuid_values),
    )
    assert summary == {"imported": 0, "skipped": 0, "replaced": 2, "errors": 0}
    assert rows == [
        {
            "Id": "existing-2",
            "Name": "Replace",
            "Description": "old",
            "PageType": "rum",
            "FiltersJson": '{"device": "mobile"}',
            "IsDeleted": 1,
            "Version": 200,
        },
        {
            "Id": "new-2",
            "Name": "Replace",
            "Description": "first",
            "PageType": "rum",
            "FiltersJson": '{"device": "desktop"}',
            "IsDeleted": 0,
            "Version": 201,
        },
        {
            "Id": "new-2",
            "Name": "Replace",
            "Description": "first",
            "PageType": "rum",
            "FiltersJson": '{"device": "desktop"}',
            "IsDeleted": 1,
            "Version": 202,
        },
        {
            "Id": "new-3",
            "Name": "Replace",
            "Description": "second",
            "PageType": "rum",
            "FiltersJson": '{"device": "tablet"}',
            "IsDeleted": 0,
            "Version": 203,
        },
    ]

    rows, summary = _plan_reports_import(
        [{"name": "Replace Missing Filters", "description": "new", "page_type": "rum", "filters": {"ok": True}}],
        [{"id": "existing-3", "name": "Replace Missing Filters", "description": "old", "page_type": "rum"}],
        on_conflict="replace",
        version_base=300,
        uuid_factory=lambda: "new-4",
    )
    assert summary == {"imported": 0, "skipped": 0, "replaced": 1, "errors": 0}
    assert rows[0]["FiltersJson"] == "{}"
