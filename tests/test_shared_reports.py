from __future__ import annotations

import json

from shared.reports import (
    REPORTS_EXPORT_VERSION,
    _build_report_record,
    _build_reports_export_payload,
    _get_report,
    _get_reports,
    _parse_report_filters,
    _serialize_report_row,
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
