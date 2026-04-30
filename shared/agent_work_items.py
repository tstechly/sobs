"""Shared GitHub work-item and agent-trigger helpers used by SOBS."""

from __future__ import annotations

import re
import time
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


def _load_recent_work_item_candidates(
    db,
    github_repo: str,
    limit: int,
    *,
    serialize_github_work_item_row,
) -> list[dict[str, Any]]:
    rows = db.execute(
        "SELECT * FROM sobs_github_work_items FINAL "
        "WHERE IsDeleted=0 AND GithubRepo=? AND IssueUrl != '' "
        "ORDER BY CreatedAt DESC LIMIT ?",
        [github_repo, max(1, int(limit))],
    ).fetchall()
    return [serialize_github_work_item_row(row) for row in rows]


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


def _persist_github_work_item(
    db,
    run_id: str,
    rule: dict,
    trigger_context: dict,
    github_issue_url: str,
    analysis: str,
    suggestion: str,
    agent_action: str,
    *,
    issue_title: str = "",
    issue_state: str = "",
    dedup_key: str = "",
    dedup_decision: str = "new_issue",
    dedup_confidence: float = 0.0,
    canonical_issue_url: str = "",
    canonical_issue_number: int = 0,
    related_issue_urls: list[str] | None = None,
    occurrence_count: int = 1,
    copilot_assignment_requested_at: int = 0,
    copilot_assignment_status: str = "not_requested",
    copilot_assignment_reason: str = "",
    pr_linked: bool = False,
    pr_number: int = 0,
    pr_url: str = "",
    normalize_ch_timestamp,
    extract_agent_trigger_fields,
    safe_json_dumps,
    insert_rows_json_each_row,
    invalidate_work_items_cache,
    logger,
    now=None,
) -> None:
    try:
        now_fn = now or (lambda: datetime.now(timezone.utc))
        now_ts = normalize_ch_timestamp(now_fn())
        issue_number = 0
        try:
            parts = github_issue_url.rstrip("/").split("/")
            if parts and parts[-1].isdigit():
                issue_number = int(parts[-1])
        except Exception:
            pass

        trigger_fields = extract_agent_trigger_fields(trigger_context)
        service_name = str(trigger_fields.get("service_name") or "")
        anomaly_rule_id = str(trigger_fields.get("anomaly_rule_id") or "")
        anomaly_state = str(trigger_fields.get("anomaly_state") or "")
        signal_source = str(trigger_fields.get("signal_source") or "")
        signal_name = str(trigger_fields.get("signal_name") or "")
        signal_value = float(trigger_fields.get("signal_value") or 0.0)

        github_repo = ""
        issue_source_url = canonical_issue_url or github_issue_url
        try:
            parts = issue_source_url.split("/")
            if len(parts) >= 4:
                github_repo = f"{parts[-4]}/{parts[-3]}"
        except Exception:
            pass

        canonical_number = int(canonical_issue_number or issue_number or 0)
        resolved_issue_url = github_issue_url or canonical_issue_url

        work_item = {
            "Id": run_id,
            "CreatedAt": now_ts,
            "CompletedAt": now_ts,
            "AgentRunId": run_id,
            "AgentRuleId": rule.get("id", ""),
            "AgentRuleName": rule.get("name", ""),
            "AgentAction": agent_action,
            "ServiceName": service_name,
            "AnomalyRuleId": anomaly_rule_id,
            "AnomalyState": anomaly_state,
            "SignalSource": signal_source,
            "SignalName": signal_name,
            "SignalValue": signal_value,
            "GithubRepo": github_repo,
            "DedupKey": dedup_key,
            "DedupDecision": dedup_decision,
            "DedupConfidence": float(dedup_confidence or 0.0),
            "IssueNumber": issue_number,
            "IssueUrl": resolved_issue_url,
            "CanonicalIssueNumber": canonical_number,
            "CanonicalIssueUrl": canonical_issue_url or resolved_issue_url,
            "RelatedIssueUrls": safe_json_dumps(related_issue_urls or []),
            "OccurrenceCount": max(1, int(occurrence_count or 1)),
            "IssueState": issue_state,
            "IssueTitle": issue_title,
            "AnalysisSummary": analysis[:500] if analysis else "",
            "SuggestionSummary": suggestion[:500] if suggestion else "",
            "CopilotAssignmentRequestedAt": int(copilot_assignment_requested_at or 0),
            "CopilotAssignmentStatus": copilot_assignment_status,
            "CopilotAssignmentReason": copilot_assignment_reason,
            "PrLinked": 1 if pr_linked else 0,
            "PrNumber": int(pr_number or 0),
            "PrUrl": pr_url,
            "IsDeleted": 0,
            "Version": int(time.time() * 1000),
        }

        insert_rows_json_each_row(db, "sobs_github_work_items", [work_item])
        invalidate_work_items_cache()
    except Exception as exc:
        logger.warning("Failed to persist work item: %s", exc)


def _parse_issue_ref_from_url(issue_url: str) -> tuple[str, str, int]:
    match = re.search(r"github\.com/([^/]+)/([^/]+)/issues/(\d+)", str(issue_url or ""))
    if not match:
        return "", "", 0
    return match.group(1), match.group(2), int(match.group(3))


def _build_agent_context_summary(db, trigger_context: dict, *, safe_json_loads) -> str:
    lines: list[str] = []
    lines.append("=== SOBS Observability Context ===")

    rule_name = trigger_context.get("rule_name", "unknown rule")
    trigger_state = trigger_context.get("trigger_state", "")
    lines.append(f"Triggered by: {rule_name} ({trigger_state})")

    extra = trigger_context.get("extra", "")
    extra_dict: dict[str, Any] = {}
    if isinstance(extra, dict):
        extra_dict = extra
    elif extra:
        extra_dict = safe_json_loads(str(extra), {})

    additional_context = str(extra_dict.get("additional_context") or "").strip()
    if additional_context:
        lines.append(f"\nUser-provided context: {additional_context}")

    service = str(extra_dict.get("service") or trigger_context.get("service") or "").strip()
    err_type = str(extra_dict.get("err_type") or "").strip()
    if service and err_type:
        try:
            freq_row = db.execute(
                "SELECT "
                "  countIf(Timestamp >= now() - INTERVAL 1 HOUR) AS c_1h, "
                "  count() AS c_24h "
                "FROM otel_logs "
                "WHERE Timestamp >= now() - INTERVAL 24 HOUR "
                "  AND SeverityText IN ('ERROR','FATAL') "
                "  AND ServiceName = ? "
                "  AND LogAttributes['exception.type'] = ?",
                [service, err_type],
            ).fetchone()
            count_1h = int(freq_row["c_1h"]) if freq_row else 0
            count_24h = int(freq_row["c_24h"]) if freq_row else 0
            lines.append(f"\nEvent frequency ({service} / {err_type}):")
            lines.append(f"  Last 1h:  {count_1h} occurrence(s)")
            lines.append(f"  Last 24h: {count_24h} occurrence(s)")
            if count_1h <= 1 and count_24h <= 2:
                lines.append("  Noise indicator: LOW recurrence — may be an isolated event")
            elif count_1h >= 10 or count_24h >= 50:
                lines.append("  Noise indicator: HIGH recurrence — persistent or systemic pattern")
            else:
                lines.append("  Noise indicator: MODERATE recurrence — monitor for escalation")
        except Exception:
            pass

    try:
        err_rows = db.execute(
            "SELECT ServiceName, ExceptionType, count() AS c "
            "FROM otel_logs FINAL "
            "WHERE Timestamp >= now() - INTERVAL 1 HOUR AND SeverityText IN ('ERROR','FATAL') "
            "GROUP BY ServiceName, ExceptionType ORDER BY c DESC LIMIT 5"
        ).fetchall()
        if err_rows:
            lines.append("\nRecent errors (last 1h, all services):")
            for row in err_rows:
                lines.append(f"  {row['ServiceName']} | {row['ExceptionType']} x{row['c']}")
    except Exception:
        pass

    try:
        anom_rows = db.execute(
            "SELECT ServiceName, Name AS Signal, anomaly_state "
            "FROM v_derived_signals_anomaly "
            "WHERE anomaly_state != 'normal' "
            "AND time >= now() - INTERVAL 2 HOUR "
            "LIMIT 5"
        ).fetchall()
        if anom_rows:
            lines.append("\nActive anomalies:")
            for row in anom_rows:
                lines.append(f"  {row['ServiceName']} | {row['Signal']} → {row['anomaly_state']}")
    except Exception:
        pass

    rendered_extra_keys = {"additional_context", "mask_output", "initiated_by"}
    if extra_dict:
        remaining = {key: value for key, value in extra_dict.items() if key not in rendered_extra_keys}
        if remaining:
            lines.append(f"\nTrigger details: {remaining}")
    elif extra:
        lines.append(f"\nAdditional context: {extra}")

    return "\n".join(lines)


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
