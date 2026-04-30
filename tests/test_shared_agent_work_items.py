from datetime import datetime, timezone

from shared.agent_work_items import (
    _build_agent_issue_title,
    _build_github_work_item_dedup_key,
    _derive_copilot_assignment_status,
    _extract_agent_trigger_fields,
    _normalize_issue_match_text,
    _parse_bounded_int_setting,
    _parse_issue_ref_from_url,
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
