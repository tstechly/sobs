from datetime import datetime, timezone

from shared.agent_work_items import (
    _build_agent_context_summary,
    _build_agent_issue_title,
    _build_github_work_item_dedup_key,
    _derive_copilot_assignment_status,
    _extract_agent_trigger_fields,
    _load_recent_work_item_candidates,
    _normalize_issue_match_text,
    _parse_bounded_int_setting,
    _parse_issue_ref_from_url,
    _persist_github_work_item,
    _serialize_github_work_item_row,
)


def _safe_json_loads(value, default):
    import json

    try:
        return json.loads(value)
    except Exception:
        return default


def test_agent_work_items_parse_bounded_int_setting_clamps_and_defaults():
    assert _parse_bounded_int_setting({"limit": "7"}, "limit", 5, 1, 10) == 7
    assert _parse_bounded_int_setting({"limit": "99"}, "limit", 5, 1, 10) == 10
    assert _parse_bounded_int_setting({"limit": "0"}, "limit", 5, 1, 10) == 1
    assert _parse_bounded_int_setting({"limit": "bad"}, "limit", 5, 1, 10) == 5
    assert _parse_bounded_int_setting({}, "limit", 5, 1, 10) == 5


def test_agent_work_items_extract_trigger_fields_covers_string_dict_and_invalid_values():
    fields = _extract_agent_trigger_fields(
        {
            "trigger_ref_id": "rule-1",
            "trigger_state": "open",
            "extra": '{"service":"checkout","state":"firing","source":"latency","signal":"p95","value":"12.5"}',
        },
        safe_json_loads=_safe_json_loads,
    )
    assert fields == {
        "service_name": "checkout",
        "anomaly_rule_id": "rule-1",
        "anomaly_state": "firing",
        "signal_source": "latency",
        "signal_name": "p95",
        "signal_value": 12.5,
        "extra": {"service": "checkout", "state": "firing", "source": "latency", "signal": "p95", "value": "12.5"},
    }

    fields = _extract_agent_trigger_fields(
        {
            "service": "fallback-service",
            "trigger_ref_id": "rule-2",
            "trigger_state": "degraded",
            "extra": {"value": "bad"},
        },
        safe_json_loads=_safe_json_loads,
    )
    assert fields["service_name"] == "fallback-service"
    assert fields["anomaly_state"] == "degraded"
    assert fields["signal_value"] == 0.0

    fields = _extract_agent_trigger_fields(
        {"extra": "not-json"},
        safe_json_loads=_safe_json_loads,
    )
    assert fields["extra"] == {}


def test_agent_work_items_text_normalization_and_titles_cover_expected_paths():
    assert _normalize_issue_match_text(" Repo/Checkout-Service ") == "repo checkout service"
    dedup_key = _build_github_work_item_dedup_key(
        "octo/repo",
        {
            "service_name": "Checkout Service",
            "signal_source": "Latency",
            "signal_name": "P95",
            "anomaly_state": "Firing",
        },
    )
    assert dedup_key == "octo repo|checkout service|latency|p95|firing"

    title = _build_agent_issue_title(
        {"name": "Rule Name"},
        {
            "service_name": "checkout",
            "signal_source": "latency",
            "signal_name": "p95",
            "anomaly_state": "firing",
        },
    )
    assert title == "[SOBS Agent] checkout — latency/p95 firing anomaly"

    fallback_title = _build_agent_issue_title(
        {"name": "Rule Name"},
        {"anomaly_state": "detected"},
    )
    assert fallback_title == "[SOBS Agent] Rule Name — detected state detected"


def test_agent_work_items_serialize_row_covers_datetime_string_and_fallback_paths():
    row = {
        "Id": 1,
        "CreatedAt": "2026-04-01 12:30:00",
        "CompletedAt": datetime(2026, 4, 1, 12, 31, 2, 123000, tzinfo=timezone.utc),
        "AgentRuleId": "rule-1",
        "AgentRuleName": "Rule",
        "AgentAction": "github_issue",
        "ServiceName": "checkout",
        "AnomalyRuleId": "a-1",
        "AnomalyState": "firing",
        "SignalSource": "latency",
        "SignalName": "p95",
        "SignalValue": "12.5",
        "GithubRepo": "octo/repo",
        "DedupKey": "dedup",
        "DedupDecision": "new",
        "DedupConfidence": "0.7",
        "IssueNumber": "23",
        "IssueUrl": "https://github.com/octo/repo/issues/23",
        "CanonicalIssueNumber": "24",
        "CanonicalIssueUrl": "https://github.com/octo/repo/issues/24",
        "RelatedIssueUrls": '["https://github.com/octo/repo/issues/25"]',
        "OccurrenceCount": "3",
        "IssueState": "open",
        "IssueTitle": "title",
        "AnalysisSummary": "analysis",
        "SuggestionSummary": "suggestion",
        "CopilotAssignmentRequestedAt": "5",
        "CopilotAssignmentStatus": "requested",
        "CopilotAssignmentReason": "reason",
        "PrLinked": "1",
        "PrNumber": "77",
        "PrUrl": "https://github.com/octo/repo/pull/77",
    }
    serialized = _serialize_github_work_item_row(row, safe_json_loads=_safe_json_loads)
    assert serialized["id"] == "1"
    assert serialized["created_at"] == "2026-04-01T12:30:00.000Z"
    assert serialized["completed_at"] == "2026-04-01T12:31:02.123Z"
    assert serialized["signal_value"] == 12.5
    assert serialized["issue_number"] == 23
    assert serialized["canonical_issue_number"] == 24
    assert serialized["related_issue_urls"] == ["https://github.com/octo/repo/issues/25"]
    assert serialized["pr_linked"] is True

    serialized = _serialize_github_work_item_row(
        [
            ("CreatedAt", "not-a-date"),
            ("CompletedAt", datetime(2026, 4, 1, 12, 31, 2, 123000)),
            ("RelatedIssueUrls", "not-json"),
            ("SignalValue", None),
            ("DedupConfidence", None),
        ],
        safe_json_loads=_safe_json_loads,
    )
    assert serialized["created_at"] == "not-a-date"
    assert serialized["completed_at"] == "2026-04-01T12:31:02.123Z"
    assert serialized["related_issue_urls"] == []
    assert serialized["signal_value"] == 0.0
    assert serialized["dedup_confidence"] == 0.0


def test_agent_work_items_issue_ref_and_copilot_status_cover_branches():
    assert _parse_issue_ref_from_url("https://github.com/octo/repo/issues/23") == ("octo", "repo", 23)
    assert _parse_issue_ref_from_url("https://example.com") == ("", "", 0)

    assert _derive_copilot_assignment_status(
        "requested",
        "closed",
        [],
        False,
        github_copilot_assignee="copilot-swe-agent[bot]",
    ) == ("completed", "issue is closed")
    assert _derive_copilot_assignment_status(
        "blocked",
        "closed",
        [],
        False,
        github_copilot_assignee="copilot-swe-agent[bot]",
    ) == ("blocked", "")
    assert _derive_copilot_assignment_status(
        "not_requested",
        "open",
        [],
        True,
        github_copilot_assignee="copilot-swe-agent[bot]",
    ) == ("blocked", "linked pull request already exists")
    assert _derive_copilot_assignment_status(
        "not_requested",
        "open",
        ["copilot-swe-agent[bot]"],
        False,
        github_copilot_assignee="copilot-swe-agent[bot]",
    ) == ("active", "Copilot is assigned on the issue")
    assert _derive_copilot_assignment_status(
        "active",
        "open",
        [],
        False,
        github_copilot_assignee="copilot-swe-agent[bot]",
    ) == ("requested", "Copilot assignment requested")
    assert _derive_copilot_assignment_status(
        "not_requested",
        "open",
        [],
        False,
        github_copilot_assignee="copilot-swe-agent[bot]",
    ) == ("not_requested", "")


class _FakeResult:
    def __init__(self, row=None, rows=None):
        self._row = row
        self._rows = rows or []

    def fetchone(self):
        return self._row

    def fetchall(self):
        return self._rows


class _FakeDb:
    def __init__(self, rows_by_query=None):
        self.rows_by_query = rows_by_query or []
        self.calls = []

    def execute(self, query, params=None):
        self.calls.append((query, params))
        for matcher, result in self.rows_by_query:
            if matcher in query:
                if isinstance(result, Exception):
                    raise result
                row = result.get("row") if isinstance(result, dict) else None
                rows = result.get("rows") if isinstance(result, dict) else result
                return _FakeResult(row=row, rows=rows)
        return _FakeResult(row=None, rows=[])


class _BrokenUrl:
    def rstrip(self, _suffix):
        raise RuntimeError("bad-rstrip")


class _BrokenSplitUrl:
    def split(self, _separator):
        raise RuntimeError("bad-split")


def test_agent_work_items_load_recent_candidates_and_persist_row_cover_paths():
    db = _FakeDb(
        rows_by_query=[
            (
                "FROM sobs_github_work_items FINAL",
                {"rows": [{"IssueNumber": 1}, {"IssueNumber": 2}]},
            )
        ]
    )
    candidates = _load_recent_work_item_candidates(
        db,
        "octo/repo",
        0,
        serialize_github_work_item_row=lambda row: {"issue_number": row["IssueNumber"]},
    )
    assert candidates == [{"issue_number": 1}, {"issue_number": 2}]
    assert db.calls[0][1] == ["octo/repo", 1]

    inserted = []
    invalidated = []
    warnings = []
    _persist_github_work_item(
        object(),
        "run-1",
        {"id": "rule-1", "name": "Rule 1"},
        {"extra": {"service": "checkout", "state": "critical", "source": "logs", "signal": "rate", "value": 3}},
        "https://github.com/octo/repo/issues/42",
        "a" * 510,
        "b" * 510,
        "github_issue",
        issue_title="Issue title",
        issue_state="open",
        dedup_key="dedup",
        canonical_issue_url="https://github.com/octo/repo/issues/43",
        canonical_issue_number=43,
        related_issue_urls=["https://github.com/octo/repo/issues/40"],
        occurrence_count=0,
        copilot_assignment_requested_at=7,
        copilot_assignment_status="requested",
        copilot_assignment_reason="Requested",
        pr_linked=True,
        pr_number=9,
        pr_url="https://github.com/octo/repo/pull/9",
        normalize_ch_timestamp=lambda dt: dt.isoformat(),
        extract_agent_trigger_fields=lambda trigger_context: {
            "service_name": "checkout",
            "anomaly_rule_id": "anomaly-1",
            "anomaly_state": "critical",
            "signal_source": "logs",
            "signal_name": "rate",
            "signal_value": 3.0,
        },
        safe_json_dumps=lambda value: str(value),
        insert_rows_json_each_row=lambda _db, table, rows: inserted.append((table, rows)),
        invalidate_work_items_cache=lambda: invalidated.append(True),
        logger=type("Logger", (), {"warning": lambda self, msg, exc: warnings.append((msg, str(exc)))})(),
        now=lambda: datetime(2026, 4, 1, 12, 0, tzinfo=timezone.utc),
    )
    assert invalidated == [True]
    table, rows = inserted[0]
    assert table == "sobs_github_work_items"
    row = rows[0]
    assert row["IssueNumber"] == 42
    assert row["CanonicalIssueNumber"] == 43
    assert row["GithubRepo"] == "octo/repo"
    assert row["OccurrenceCount"] == 1
    assert len(row["AnalysisSummary"]) == 500
    assert len(row["SuggestionSummary"]) == 500
    assert row["PrLinked"] == 1
    assert warnings == []

    _persist_github_work_item(
        object(),
        "run-2",
        {},
        {},
        "",
        "",
        "",
        "github_issue",
        normalize_ch_timestamp=lambda dt: dt.isoformat(),
        extract_agent_trigger_fields=lambda _trigger_context: {},
        safe_json_dumps=lambda value: str(value),
        insert_rows_json_each_row=lambda *_args, **_kwargs: (_ for _ in ()).throw(RuntimeError("boom")),
        invalidate_work_items_cache=lambda: None,
        logger=type("Logger", (), {"warning": lambda self, msg, exc: warnings.append((msg, str(exc)))})(),
        now=lambda: datetime(2026, 4, 1, 12, 0, tzinfo=timezone.utc),
    )
    assert warnings[-1] == ("Failed to persist work item: %s", "boom")


def test_agent_work_items_build_context_summary_covers_sections_and_fallbacks():
    db = _FakeDb(
        rows_by_query=[
            ("countIf(Timestamp >= now() - INTERVAL 1 HOUR)", {"row": {"c_1h": 12, "c_24h": 60}}),
            (
                "FROM otel_logs FINAL",
                {"rows": [{"ServiceName": "checkout", "ExceptionType": "RuntimeError", "c": 5}]},
            ),
            (
                "FROM v_derived_signals_anomaly",
                {"rows": [{"ServiceName": "checkout", "Signal": "latency", "anomaly_state": "critical"}]},
            ),
        ]
    )
    summary = _build_agent_context_summary(
        db,
        {
            "rule_name": "Rule 1",
            "trigger_state": "critical",
            "extra": (
                '{"service":"checkout","err_type":"RuntimeError",'
                '"additional_context":"started after deploy","mask_output":true,"source":"logs"}'
            ),
        },
        safe_json_loads=_safe_json_loads,
    )
    assert "User-provided context: started after deploy" in summary
    assert "HIGH recurrence" in summary
    assert "Recent errors (last 1h, all services):" in summary
    assert "Active anomalies:" in summary
    assert "Trigger details: {'service': 'checkout', 'err_type': 'RuntimeError', 'source': 'logs'}" in summary

    fallback_summary = _build_agent_context_summary(
        _FakeDb(),
        {"rule_name": "Rule 2", "trigger_state": "warning", "extra": "not-json"},
        safe_json_loads=_safe_json_loads,
    )
    assert "Additional context: not-json" in fallback_summary


def test_agent_work_items_cover_remaining_branch_paths():
    fields = _extract_agent_trigger_fields(
        {"extra": "[]", "service": "svc"},
        safe_json_loads=lambda _value, _default: [],
    )
    assert fields["extra"] == {}

    serialized = _serialize_github_work_item_row(
        {"CreatedAt": "", "CompletedAt": "2026-04-01T12:31:02Z"},
        safe_json_loads=_safe_json_loads,
    )
    assert serialized["created_at"] == ""
    assert serialized["completed_at"] == "2026-04-01T12:31:02.000Z"

    warnings = []
    _persist_github_work_item(
        object(),
        "run-3",
        {},
        {},
        _BrokenUrl(),
        "",
        "",
        "github_issue",
        canonical_issue_url=_BrokenSplitUrl(),
        normalize_ch_timestamp=lambda dt: dt.isoformat(),
        extract_agent_trigger_fields=lambda _trigger_context: {},
        safe_json_dumps=lambda value: str(value),
        insert_rows_json_each_row=lambda *_args, **_kwargs: None,
        invalidate_work_items_cache=lambda: None,
        logger=type("Logger", (), {"warning": lambda self, msg, exc: warnings.append((msg, str(exc)))})(),
        now=lambda: datetime(2026, 4, 1, 12, 0, tzinfo=timezone.utc),
    )
    assert warnings == []

    low_summary = _build_agent_context_summary(
        _FakeDb(rows_by_query=[("countIf(Timestamp >= now() - INTERVAL 1 HOUR)", {"row": {"c_1h": 0, "c_24h": 1}})]),
        {
            "rule_name": "Rule low",
            "trigger_state": "warning",
            "extra": {"service": "checkout", "err_type": "RuntimeError"},
        },
        safe_json_loads=_safe_json_loads,
    )
    assert "LOW recurrence" in low_summary

    moderate_summary = _build_agent_context_summary(
        _FakeDb(rows_by_query=[("countIf(Timestamp >= now() - INTERVAL 1 HOUR)", {"row": {"c_1h": 2, "c_24h": 10}})]),
        {
            "rule_name": "Rule moderate",
            "trigger_state": "warning",
            "extra": {"service": "checkout", "err_type": "RuntimeError"},
        },
        safe_json_loads=_safe_json_loads,
    )
    assert "MODERATE recurrence" in moderate_summary

    exception_summary = _build_agent_context_summary(
        _FakeDb(
            rows_by_query=[
                ("countIf(Timestamp >= now() - INTERVAL 1 HOUR)", RuntimeError("freq boom")),
                ("FROM otel_logs FINAL", RuntimeError("err boom")),
                ("FROM v_derived_signals_anomaly", RuntimeError("anom boom")),
            ]
        ),
        {
            "rule_name": "Rule exception",
            "trigger_state": "warning",
            "extra": {"service": "checkout", "err_type": "RuntimeError", "initiated_by": "user"},
        },
        safe_json_loads=_safe_json_loads,
    )
    assert "Triggered by: Rule exception (warning)" in exception_summary
