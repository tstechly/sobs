"""Shared GitHub work-item and agent-trigger helpers used by SOBS."""

from __future__ import annotations

import re
from datetime import datetime, timezone
from typing import Any


def _parse_bounded_int_setting(
    settings: dict[str, str],
    key: str,
    default: int,
    minimum: int,
    maximum: int,
) -> int:
    try:
        parsed = int(settings.get(key, "") or default)
    except (TypeError, ValueError):
        parsed = default
    return max(minimum, min(maximum, parsed))


def _extract_agent_trigger_fields(trigger_context: dict, *, safe_json_loads) -> dict[str, Any]:
    trigger_context_parsed = trigger_context.get("extra", {})
    if isinstance(trigger_context_parsed, str):
        trigger_context_parsed = safe_json_loads(trigger_context_parsed, {})
    if not isinstance(trigger_context_parsed, dict):
        trigger_context_parsed = {}

    service_name = str(trigger_context_parsed.get("service") or trigger_context.get("service") or "").strip()
    anomaly_rule_id = str(trigger_context.get("trigger_ref_id") or "").strip()
    anomaly_state = str(trigger_context_parsed.get("state") or trigger_context.get("trigger_state") or "").strip()
    signal_source = str(trigger_context_parsed.get("source") or "").strip()
    signal_name = str(trigger_context_parsed.get("signal") or "").strip()
    try:
        signal_value = float(trigger_context_parsed.get("value") or 0.0)
    except (TypeError, ValueError):
        signal_value = 0.0

    return {
        "service_name": service_name,
        "anomaly_rule_id": anomaly_rule_id,
        "anomaly_state": anomaly_state,
        "signal_source": signal_source,
        "signal_name": signal_name,
        "signal_value": signal_value,
        "extra": trigger_context_parsed,
    }


def _normalize_issue_match_text(value: Any) -> str:
    text = re.sub(r"[^a-z0-9]+", " ", str(value or "").lower())
    return " ".join(text.split())


def _build_github_work_item_dedup_key(github_repo: str, trigger_fields: dict[str, Any]) -> str:
    return "|".join(
        [
            _normalize_issue_match_text(github_repo),
            _normalize_issue_match_text(trigger_fields.get("service_name")),
            _normalize_issue_match_text(trigger_fields.get("signal_source")),
            _normalize_issue_match_text(trigger_fields.get("signal_name")),
            _normalize_issue_match_text(trigger_fields.get("anomaly_state")),
        ]
    ).strip("|")


def _build_agent_issue_title(rule: dict, trigger_fields: dict[str, Any]) -> str:
    service_name = str(trigger_fields.get("service_name") or "").strip()
    signal_name = str(trigger_fields.get("signal_name") or "").strip()
    signal_source = str(trigger_fields.get("signal_source") or "").strip()
    anomaly_state = str(trigger_fields.get("anomaly_state") or "detected").strip()
    focus = service_name or str(rule.get("name") or "Agent Rule")
    if signal_source and signal_name:
        return f"[SOBS Agent] {focus} — {signal_source}/{signal_name} {anomaly_state} anomaly"
    return f"[SOBS Agent] {focus} — {anomaly_state} state detected"


def _serialize_github_work_item_row(row: dict | Any, *, safe_json_loads) -> dict[str, Any]:
    r = row if isinstance(row, dict) else dict(row)
    related_issue_urls_raw = safe_json_loads(r.get("RelatedIssueUrls", "[]"), [])
    related_issue_urls = related_issue_urls_raw if isinstance(related_issue_urls_raw, list) else []

    def _to_utc_iso(ts_value: Any) -> str:
        raw = str(ts_value or "").strip()
        if not raw:
            return ""
        if isinstance(ts_value, datetime):
            dt = ts_value
        else:
            normalized = raw.replace(" ", "T")
            if normalized.endswith("Z"):
                normalized = normalized[:-1] + "+00:00"
            if not re.search(r"[zZ]|[+\-]\d\d:?\d\d$", normalized):
                normalized += "+00:00"
            try:
                dt = datetime.fromisoformat(normalized)
            except ValueError:
                return raw
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=timezone.utc)
        else:
            dt = dt.astimezone(timezone.utc)
        return dt.isoformat(timespec="milliseconds").replace("+00:00", "Z")

    return {
        "id": str(r.get("Id", "")),
        "created_at": _to_utc_iso(r.get("CreatedAt", "")),
        "completed_at": _to_utc_iso(r.get("CompletedAt", "")),
        "agent_rule_id": str(r.get("AgentRuleId", "")),
        "agent_rule_name": str(r.get("AgentRuleName", "")),
        "agent_action": str(r.get("AgentAction", "")),
        "service": str(r.get("ServiceName", "")),
        "anomaly_rule_id": str(r.get("AnomalyRuleId", "")),
        "anomaly_state": str(r.get("AnomalyState", "")),
        "signal_source": str(r.get("SignalSource", "")),
        "signal_name": str(r.get("SignalName", "")),
        "signal_value": float(r.get("SignalValue", 0.0) or 0.0),
        "github_repo": str(r.get("GithubRepo", "")),
        "dedup_key": str(r.get("DedupKey", "")),
        "dedup_decision": str(r.get("DedupDecision", "")),
        "dedup_confidence": float(r.get("DedupConfidence", 0.0) or 0.0),
        "issue_number": int(r.get("IssueNumber", 0) or 0),
        "issue_url": str(r.get("IssueUrl", "")),
        "canonical_issue_number": int(r.get("CanonicalIssueNumber", 0) or 0),
        "canonical_issue_url": str(r.get("CanonicalIssueUrl", "")),
        "related_issue_urls": related_issue_urls,
        "occurrence_count": int(r.get("OccurrenceCount", 1) or 1),
        "issue_state": str(r.get("IssueState", "")),
        "issue_title": str(r.get("IssueTitle", "")),
        "analysis_summary": str(r.get("AnalysisSummary", "")),
        "suggestion_summary": str(r.get("SuggestionSummary", "")),
        "copilot_assignment_requested_at": int(r.get("CopilotAssignmentRequestedAt", 0) or 0),
        "copilot_assignment_status": str(r.get("CopilotAssignmentStatus", "not_requested")),
        "copilot_assignment_reason": str(r.get("CopilotAssignmentReason", "")),
        "pr_linked": bool(int(r.get("PrLinked", 0) or 0)),
        "pr_number": int(r.get("PrNumber", 0) or 0),
        "pr_url": str(r.get("PrUrl", "")),
    }


def _parse_issue_ref_from_url(issue_url: str) -> tuple[str, str, int]:
    match = re.search(r"github\.com/([^/]+)/([^/]+)/issues/(\d+)", str(issue_url or ""))
    if not match:
        return "", "", 0
    return match.group(1), match.group(2), int(match.group(3))


def _derive_copilot_assignment_status(
    current_status: str,
    issue_state: str,
    assignees: list[str],
    pr_linked: bool,
    *,
    github_copilot_assignee: str,
) -> tuple[str, str]:
    normalized_current = str(current_status or "").strip().lower() or "not_requested"
    normalized_state = str(issue_state or "").strip().lower()
    normalized_assignees = [str(item or "").strip().lower() for item in assignees]
    copilot_assigned = (
        github_copilot_assignee.lower() in normalized_assignees or "copilot-swe-agent" in normalized_assignees
    )

    if normalized_state == "closed":
        if normalized_current in {"requested", "active"}:
            return "completed", "issue is closed"
        return normalized_current, ""
    if pr_linked and normalized_current in {"not_requested", "blocked"}:
        return "blocked", "linked pull request already exists"
    if copilot_assigned:
        return "active", "Copilot is assigned on the issue"
    if normalized_current in {"requested", "active"}:
        return "requested", "Copilot assignment requested"
    return normalized_current, ""
