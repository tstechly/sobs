"""Shared agent rule, run, and target-resolution helpers used by SOBS."""

from __future__ import annotations

import time
from typing import Any


def _load_agent_rules(db) -> list[dict[str, Any]]:
    rows = db.execute(
        "SELECT Id, Name, Description, TriggerType, TriggerRefId, TriggerState, "
        "Actions, RateLimitMinutes, IsEnabled "
        "FROM sobs_agent_rules FINAL WHERE IsDeleted=0 ORDER BY Name"
    ).fetchall()
    return [_agent_rule_row_to_dict(row) for row in rows]


def _load_agent_rule(db, rule_id: str) -> dict[str, Any] | None:
    row = db.execute(
        "SELECT Id, Name, Description, TriggerType, TriggerRefId, TriggerState, "
        "Actions, RateLimitMinutes, IsEnabled "
        "FROM sobs_agent_rules FINAL WHERE IsDeleted=0 AND Id=? LIMIT 1",
        [rule_id],
    ).fetchone()
    if not row:
        return None
    return _agent_rule_row_to_dict(row)


def _agent_rule_row_to_dict(row) -> dict[str, Any]:
    return {
        "id": str(row["Id"]),
        "name": str(row["Name"]),
        "description": str(row["Description"]),
        "trigger_type": str(row["TriggerType"]),
        "trigger_ref_id": str(row["TriggerRefId"]),
        "trigger_state": str(row["TriggerState"]),
        "actions": [action.strip() for action in str(row["Actions"]).split(",") if action.strip()],
        "rate_limit_minutes": int(row["RateLimitMinutes"]),
        "is_enabled": bool(int(row["IsEnabled"])),
    }


def _load_agent_runs(db, limit: int = 50) -> list[dict[str, Any]]:
    rows = db.execute(
        "SELECT Id, RuleId, RuleName, TriggerContext, Status, GuardDecision, DlpResult, "
        "Analysis, Suggestion, GithubIssueUrl, ErrorMessage, CreatedAt, CompletedAt, IsDismissed "
        "FROM sobs_agent_runs FINAL WHERE IsDeleted=0 ORDER BY CreatedAt DESC "
        f"LIMIT {int(limit)}"
    ).fetchall()
    return [
        {
            "id": str(row["Id"]),
            "rule_id": str(row["RuleId"]),
            "rule_name": str(row["RuleName"]),
            "trigger_context": str(row["TriggerContext"]),
            "status": str(row["Status"]),
            "guard_decision": str(row["GuardDecision"]),
            "dlp_result": str(row["DlpResult"]),
            "analysis": str(row["Analysis"]),
            "suggestion": str(row["Suggestion"]),
            "github_issue_url": str(row["GithubIssueUrl"]),
            "error_message": str(row["ErrorMessage"]),
            "created_at": str(row["CreatedAt"]),
            "completed_at": str(row["CompletedAt"]),
            "is_dismissed": bool(int(row["IsDismissed"])),
        }
        for row in rows
    ]


def _agent_rule_last_run_ts(db, rule_id: str) -> float:
    row = db.execute(
        "SELECT max(toUnixTimestamp64Milli(CreatedAt)) AS t "
        "FROM sobs_agent_runs FINAL WHERE IsDeleted=0 AND RuleId=?",
        [rule_id],
    ).fetchone()
    return float(row["t"]) / 1000.0 if row and row["t"] else 0.0


def _count_github_issues_last_hour(db) -> int:
    row = db.execute(
        "SELECT count() AS c FROM sobs_agent_runs FINAL "
        "WHERE IsDeleted=0 AND GithubIssueUrl != '' "
        "AND CreatedAt >= now() - INTERVAL 1 HOUR"
    ).fetchone()
    return int(row["c"]) if row else 0


def _count_copilot_assignments_last_hour(db, *, now=time.time) -> int:
    cutoff_ms = max(0, int(now() * 1000) - 3600 * 1000)
    row = db.execute(
        "SELECT count() AS c FROM sobs_github_work_items FINAL "
        "WHERE IsDeleted=0 AND CopilotAssignmentRequestedAt >= ? AND CopilotAssignmentRequestedAt > 0",
        [cutoff_ms],
    ).fetchone()
    return int(row["c"]) if row else 0


def _count_active_copilot_assignments(db) -> int:
    row = db.execute(
        "SELECT count() AS c FROM sobs_github_work_items FINAL "
        "WHERE IsDeleted=0 AND CopilotAssignmentStatus IN ('requested', 'active')"
    ).fetchone()
    return int(row["c"]) if row else 0


def _extract_trigger_service_name(trigger_context: dict[str, Any], *, safe_json_loads) -> str:
    service = str(trigger_context.get("service") or "").strip()
    if service:
        return service

    extra_raw = trigger_context.get("extra")
    if isinstance(extra_raw, dict):
        extra = extra_raw
    else:
        extra = safe_json_loads(str(extra_raw or ""), {})

    if isinstance(extra, dict):
        for key in ("service", "service_name", "ServiceName"):
            value = str(extra.get(key) or "").strip()
            if value:
                return value
    return ""


def _resolve_agent_github_target(
    db,
    settings: dict[str, str],
    trigger_context: dict[str, Any],
    *,
    extract_trigger_service_name,
    parse_github_repo_owner_name,
    load_repo_scoped_github_token,
) -> tuple[str, str]:
    default_repo = str(settings.get("ai.github_repo", "")).strip()
    default_token = str(settings.get("ai.github_token", "")).strip()

    service_name = extract_trigger_service_name(trigger_context)
    if service_name:
        row = db.execute(
            "SELECT RepoUrl FROM sobs_apps FINAL "
            "WHERE IsDeleted=0 AND Enabled=1 AND RepoUrl != '' "
            "AND (lower(Name)=lower(?) OR lower(Slug)=lower(?)) "
            "ORDER BY UpdatedAt DESC LIMIT 1",
            [service_name, service_name],
        ).fetchone()
        if row:
            owner, repo = parse_github_repo_owner_name(str(row["RepoUrl"] or ""))
            if owner and repo:
                scoped_token = load_repo_scoped_github_token(db, owner, repo)
                return f"{owner}/{repo}", (scoped_token or default_token)

    if default_repo:
        owner, repo = parse_github_repo_owner_name(default_repo)
        if not owner or not repo:
            parts = [part for part in default_repo.strip("/").split("/") if part]
            if len(parts) >= 2:
                owner, repo = parts[-2], parts[-1]
        if owner and repo:
            scoped_token = load_repo_scoped_github_token(db, owner, repo)
            return f"{owner}/{repo}", (scoped_token or default_token)
        return default_repo, default_token

    return "", default_token
