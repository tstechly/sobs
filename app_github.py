"""GitHub integration helpers for SOBS.

This module contains GitHub issue creation, deduplication, Copilot assignment,
and related orchestration helpers extracted from ``app.py``.

The module is importable without starting the full Quart application, enabling
direct unit tests of individual helpers.  Heavy shared dependencies (HTTP client,
LLM endpoint, output masking) are resolved via lazy imports from ``app`` at
*call* time to avoid circular imports at module load time, while preserving the
``monkeypatch.setattr(sobs_app, ...)`` patching pattern used in the existing
test suite.
"""
from __future__ import annotations

import json
import logging
import re
import time
import urllib.parse
from datetime import datetime, timezone
from typing import Any, cast

import httpx

log = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

_AI_AGENT_MAX_ASSIGNMENTS_PER_HOUR_DEFAULT: int = 1
_AI_AGENT_MAX_ACTIVE_ASSIGNMENTS_DEFAULT: int = 1
_GITHUB_COPILOT_ASSIGNEE: str = "copilot-swe-agent[bot]"
_GITHUB_COPILOT_GRAPHQL_FEATURES: str = (
    "issues_copilot_assignment_api_support,coding_agent_model_selection"
)
_GITHUB_ISSUE_DEDUPE_CANDIDATE_LIMIT: int = 10
_GITHUB_WORK_ITEM_BACKFILL_INTERVAL_SEC: float = 300
_GITHUB_WORK_ITEM_BACKFILL_MAX_ITEMS: int = 25
_GITHUB_WORK_ITEM_BACKFILL_LAST_TS: float = 0.0
_GITHUB_WORK_ITEM_BACKFILL_RUNNING: bool = False
_GITHUB_TOKEN_EXPIRY_WARNING_DAYS: int = 14

# ---------------------------------------------------------------------------
# Private utilities (not exported)
# ---------------------------------------------------------------------------


def _json_loads(value: object, default: Any) -> Any:
    """JSON-safe loads with type-matched default fallback."""
    raw = str(value or "").strip()
    if not raw:
        return default
    try:
        parsed = json.loads(raw)
    except Exception:
        return default
    if isinstance(default, dict) and isinstance(parsed, dict):
        return cast(dict[str, Any], parsed)
    if isinstance(default, list) and isinstance(parsed, list):
        return cast(list[Any], parsed)
    return default


def _json_dumps(value: Any) -> str:
    """JSON-safe dumps."""
    if value is None:
        return "{}"
    if isinstance(value, str):
        stripped = value.strip()
        if not stripped:
            return "{}"
        try:
            parsed = json.loads(stripped)
            return json.dumps(parsed, ensure_ascii=False)
        except Exception:
            return "{}"
    if isinstance(value, (dict, list)):
        return json.dumps(value, ensure_ascii=False)
    return "{}"


async def _get_http_client() -> httpx.AsyncClient:
    """Return the shared async HTTP client from the app module.

    Falls back to a fresh client when the app module is not available
    (e.g. during standalone import tests).
    """
    try:
        import app as _sobs_app  # noqa: PLC0415  # lazy – avoids circular import

        return await _sobs_app._get_async_http_client()
    except Exception:
        return httpx.AsyncClient(follow_redirects=False, headers={"User-Agent": "SOBS/1.0"})


async def _call_llm(
    endpoint_url: str,
    model: str,
    api_key: str,
    messages: list[dict],
    *,
    thinking_level: str = "off",
    max_tokens: int = 1024,
    timeout: int = 30,
) -> tuple[str, dict]:
    """Delegate to the app-level LLM caller."""
    import app as _sobs_app  # noqa: PLC0415

    return await _sobs_app._call_llm_endpoint(
        endpoint_url,
        model,
        api_key,
        messages,
        thinking_level=thinking_level,
        max_tokens=max_tokens,
        timeout=timeout,
    )


def _mask_output(value: Any, mask_output_enabled: bool = True) -> str:
    """Apply output masking using the app-level masking function."""
    if not mask_output_enabled:
        return str(value or "")
    try:
        import app as _sobs_app  # noqa: PLC0415

        return _sobs_app._mask_string_for_output(value)
    except Exception:
        return str(value or "")


# ---------------------------------------------------------------------------
# Pure helper functions
# ---------------------------------------------------------------------------


def _parse_github_repo_owner_name(repo_url: str) -> tuple[str, str]:
    """Extract owner/repo from a GitHub repo URL.

    Supports HTTPS, SSH, and plain owner/repo styles.
    """
    cleaned = (repo_url or "").strip()
    if not cleaned:
        return "", ""

    direct_parts = [p for p in cleaned.split("/") if p]
    if len(direct_parts) == 2 and "://" not in cleaned and not cleaned.startswith("git@"):
        return direct_parts[0], direct_parts[1].removesuffix(".git")

    if cleaned.startswith("git@github.com:"):
        path = cleaned.split(":", 1)[1]
    else:
        parsed = urllib.parse.urlparse(cleaned)
        if parsed.netloc.lower() != "github.com":
            return "", ""
        path = parsed.path.lstrip("/")

    if path.endswith(".git"):
        path = path[:-4]
    parts = [p for p in path.split("/") if p]
    if len(parts) < 2:
        return "", ""
    return parts[0], parts[1]


def _build_github_repo_url(owner: str, repo: str) -> str:
    owner_clean = (owner or "").strip().strip("/")
    repo_clean = (repo or "").strip().strip("/").removesuffix(".git")
    if not owner_clean or not repo_clean:
        return ""
    return f"https://github.com/{owner_clean}/{repo_clean}"


def _resolve_github_repo_fields(repo_url: str, owner: str = "", repo: str = "") -> tuple[str, str, str]:
    repo_url_clean = str(repo_url or "").strip()
    owner_clean = str(owner or "").strip().strip("/")
    repo_clean = str(repo or "").strip().strip("/").removesuffix(".git")

    if (not owner_clean or not repo_clean) and repo_url_clean:
        parsed_owner, parsed_repo = _parse_github_repo_owner_name(repo_url_clean)
        if not owner_clean:
            owner_clean = parsed_owner
        if not repo_clean:
            repo_clean = parsed_repo

    canonical_repo_url = _build_github_repo_url(owner_clean, repo_clean)
    if canonical_repo_url:
        repo_url_clean = canonical_repo_url

    return repo_url_clean, owner_clean, repo_clean


def _github_api_headers(
    github_token: str, *, include_content_type: bool = False, extra: dict[str, str] | None = None
) -> dict[str, str]:
    headers = {
        "Authorization": f"Bearer {github_token}",
        "Accept": "application/vnd.github+json",
        "X-GitHub-Api-Version": "2022-11-28",
    }
    if include_content_type:
        headers["Content-Type"] = "application/json"
    if extra:
        headers.update(extra)
    return headers


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


def _extract_agent_trigger_fields(trigger_context: dict) -> dict[str, Any]:
    trigger_context_parsed = trigger_context.get("extra", {})
    if isinstance(trigger_context_parsed, str):
        trigger_context_parsed = _json_loads(trigger_context_parsed, {})
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


def _serialize_github_work_item_row(row: dict | Any) -> dict[str, Any]:
    r = row if isinstance(row, dict) else dict(row)
    related_issue_urls_raw = _json_loads(r.get("RelatedIssueUrls", "[]"), [])
    related_issue_urls = cast(list[Any], related_issue_urls_raw) if isinstance(related_issue_urls_raw, list) else []

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
) -> tuple[str, str]:
    normalized_current = str(current_status or "").strip().lower() or "not_requested"
    normalized_state = str(issue_state or "").strip().lower()
    normalized_assignees = [str(item or "").strip().lower() for item in assignees]
    copilot_assigned = (
        _GITHUB_COPILOT_ASSIGNEE.lower() in normalized_assignees or "copilot-swe-agent" in normalized_assignees
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


def _fallback_issue_dedupe_decision(
    proposed: dict[str, Any],
    candidates: list[dict[str, Any]],
) -> dict[str, Any]:
    proposed_key = str(proposed.get("dedup_key") or "")
    proposed_service = _normalize_issue_match_text(proposed.get("service_name"))
    proposed_signal = _normalize_issue_match_text(proposed.get("signal_name"))
    for candidate in candidates:
        candidate_key = str(candidate.get("dedup_key") or "")
        if proposed_key and candidate_key and proposed_key == candidate_key:
            return {
                "classification": "same",
                "candidate_id": str(candidate.get("candidate_id") or ""),
                "confidence": 0.92,
                "reason": "deterministic dedupe key match",
            }
    for candidate in candidates:
        if (
            proposed_service
            and proposed_service == _normalize_issue_match_text(candidate.get("service_name"))
            and proposed_signal
            and proposed_signal == _normalize_issue_match_text(candidate.get("signal_name"))
        ):
            return {
                "classification": "related",
                "candidate_id": str(candidate.get("candidate_id") or ""),
                "confidence": 0.73,
                "reason": "same service and signal family",
            }
    return {
        "classification": "unrelated",
        "candidate_id": "",
        "confidence": 0.0,
        "reason": "no strong local match",
    }


def _extract_first_json_object(text: str) -> dict[str, Any]:
    raw = str(text or "").strip()
    if not raw:
        return {}
    if raw.startswith("```"):
        raw = re.sub(r"^```(?:json)?\s*|\s*```$", "", raw, flags=re.DOTALL).strip()
    parsed = _json_loads(raw, {})
    if isinstance(parsed, dict) and parsed:
        return parsed
    match = re.search(r"\{.*\}", raw, flags=re.DOTALL)
    if not match:
        return {}
    parsed = _json_loads(match.group(0), {})
    return parsed if isinstance(parsed, dict) else {}


# ---------------------------------------------------------------------------
# Database helpers
# ---------------------------------------------------------------------------


def _count_github_issues_last_hour(db: Any) -> int:
    """Count completed agent runs with a GitHub issue created in the last 60 minutes."""
    row = db.execute(
        "SELECT count() AS c FROM sobs_agent_runs FINAL "
        "WHERE IsDeleted=0 AND GithubIssueUrl != '' "
        "AND CreatedAt >= now() - INTERVAL 1 HOUR"
    ).fetchone()
    return int(row["c"]) if row else 0


def _count_copilot_assignments_last_hour(db: Any) -> int:
    cutoff_ms = max(0, int(time.time() * 1000) - 3600 * 1000)
    row = db.execute(
        "SELECT count() AS c FROM sobs_github_work_items FINAL "
        "WHERE IsDeleted=0 AND CopilotAssignmentRequestedAt >= ? AND CopilotAssignmentRequestedAt > 0",
        [cutoff_ms],
    ).fetchone()
    return int(row["c"]) if row else 0


def _count_active_copilot_assignments(db: Any) -> int:
    row = db.execute(
        "SELECT count() AS c FROM sobs_github_work_items FINAL "
        "WHERE IsDeleted=0 AND CopilotAssignmentStatus IN ('requested', 'active')"
    ).fetchone()
    return int(row["c"]) if row else 0


def _load_recent_work_item_candidates(
    db: Any,
    github_repo: str,
    limit: int = _GITHUB_ISSUE_DEDUPE_CANDIDATE_LIMIT,
) -> list[dict[str, Any]]:
    rows = db.execute(
        "SELECT * FROM sobs_github_work_items FINAL "
        "WHERE IsDeleted=0 AND GithubRepo=? AND IssueUrl != '' "
        "ORDER BY CreatedAt DESC LIMIT ?",
        [github_repo, max(1, int(limit))],
    ).fetchall()
    return [_serialize_github_work_item_row(r) for r in rows]


# ---------------------------------------------------------------------------
# Async GitHub API helpers
# ---------------------------------------------------------------------------


async def _fetch_open_github_issues(
    github_token: str,
    github_repo: str,
    limit: int = _GITHUB_ISSUE_DEDUPE_CANDIDATE_LIMIT,
) -> list[dict[str, Any]]:
    if not github_token or not github_repo:
        return []
    owner, repo = _parse_github_repo_owner_name(github_repo)
    if not owner or not repo:
        return []
    client = await _get_http_client()
    try:
        resp = await client.get(
            f"https://api.github.com/repos/{owner}/{repo}/issues",
            params={"state": "open", "per_page": str(max(1, min(100, limit)))},
            headers=_github_api_headers(github_token),
            timeout=15,
        )
        resp.raise_for_status()
        payload = resp.json() if resp.content else []
    except Exception as exc:
        log.warning("GitHub open issue fetch failed for %s/%s: %s", owner, repo, exc)
        return []
    if not isinstance(payload, list):
        return []

    issues: list[dict[str, Any]] = []
    for item in payload:
        if not isinstance(item, dict) or isinstance(item.get("pull_request"), dict):
            continue
        issues.append(
            {
                "issue_number": int(item.get("number", 0) or 0),
                "issue_url": str(item.get("html_url") or ""),
                "issue_title": str(item.get("title") or ""),
                "issue_body": str(item.get("body") or ""),
                "issue_state": str(item.get("state") or "open"),
                "assignees": [
                    str(a.get("login") or "") for a in (item.get("assignees") or []) if isinstance(a, dict)
                ],
            }
        )
    return issues


async def _search_open_pr_for_issue(
    github_token: str, github_repo: str, issue_number: int
) -> dict[str, Any] | None:
    if not github_token or not github_repo or issue_number <= 0:
        return None
    owner, repo = _parse_github_repo_owner_name(github_repo)
    if not owner or not repo:
        return None
    client = await _get_http_client()
    try:
        resp = await client.get(
            "https://api.github.com/search/issues",
            params={
                "q": f'repo:{owner}/{repo} is:pr is:open "#{issue_number}" in:body',
                "per_page": "1",
            },
            headers=_github_api_headers(github_token),
            timeout=15,
        )
        resp.raise_for_status()
        payload = resp.json() if resp.content else {}
    except Exception:
        return None
    items = payload.get("items") if isinstance(payload, dict) else []
    if not isinstance(items, list) or not items:
        return None
    item = items[0] if isinstance(items[0], dict) else {}
    if not item:
        return None
    return {
        "pr_number": int(item.get("number", 0) or 0),
        "pr_url": str(item.get("html_url") or ""),
    }


async def _github_repo_supports_copilot_assignment(github_token: str, github_repo: str) -> bool:
    owner, repo = _parse_github_repo_owner_name(github_repo)
    if not github_token or not owner or not repo:
        return False
    client = await _get_http_client()
    query = {
        "query": (
            "query($owner:String!, $name:String!) {"
            " repository(owner:$owner, name:$name) {"
            "  suggestedActors(capabilities:[CAN_BE_ASSIGNED], first:100) {"
            "   nodes {"
            "    __typename "
            "    login "
            "    ... on Bot { id } "
            "    ... on User { id }"
            "   }"
            "  }"
            " }"
            "}"
        ),
        "variables": {"owner": owner, "name": repo},
    }
    try:
        resp = await client.post(
            "https://api.github.com/graphql",
            json=query,
            headers=_github_api_headers(
                github_token,
                include_content_type=True,
                extra={"GraphQL-Features": _GITHUB_COPILOT_GRAPHQL_FEATURES},
            ),
            timeout=15,
        )
        resp.raise_for_status()
        payload = resp.json() if resp.content else {}
    except Exception as exc:
        log.warning("GitHub Copilot support probe failed for %s/%s: %s", owner, repo, exc)
        return False

    nodes = (((payload.get("data") or {}).get("repository") or {}).get("suggestedActors") or {}).get("nodes") or []
    for node in nodes:
        if not isinstance(node, dict):
            continue
        login = str(node.get("login") or "").strip().lower()
        if login in {"copilot-swe-agent", _GITHUB_COPILOT_ASSIGNEE.lower()}:
            return True
    return False


async def _assign_issue_to_copilot(
    github_token: str,
    github_repo: str,
    issue_number: int,
    *,
    base_branch: str = "",
    custom_instructions: str = "",
) -> tuple[str, str, int]:
    if not github_token or not github_repo or issue_number <= 0:
        return "blocked", "missing GitHub token, repo, or issue number", 0
    if not await _github_repo_supports_copilot_assignment(github_token, github_repo):
        return "blocked", "Copilot cloud agent is not enabled for the target repository", 0

    owner, repo = _parse_github_repo_owner_name(github_repo)
    if not owner or not repo:
        return "blocked", "invalid GitHub repository target", 0

    agent_assignment: dict[str, Any] = {"target_repo": f"{owner}/{repo}"}
    if base_branch:
        agent_assignment["base_branch"] = base_branch
    if custom_instructions:
        agent_assignment["custom_instructions"] = custom_instructions[:4000]

    payload = {
        "assignees": [_GITHUB_COPILOT_ASSIGNEE],
        "agent_assignment": agent_assignment,
    }
    client = await _get_http_client()
    requested_at = int(time.time() * 1000)
    try:
        resp = await client.post(
            f"https://api.github.com/repos/{owner}/{repo}/issues/{issue_number}/assignees",
            json=payload,
            headers=_github_api_headers(github_token, include_content_type=True),
            timeout=20,
        )
        resp.raise_for_status()
        body = resp.json() if resp.content else {}
        assignees = [
            str(item.get("login") or "").strip().lower()
            for item in (body.get("assignees") or [])
            if isinstance(item, dict)
        ]
        if _GITHUB_COPILOT_ASSIGNEE.lower() not in assignees and "copilot-swe-agent" not in assignees:
            return (
                "requested",
                "Copilot assignment request accepted; GitHub assignee visibility may lag briefly",
                requested_at,
            )
        return "requested", "Copilot assignment requested", requested_at
    except httpx.HTTPStatusError as exc:
        detail = exc.response.text[:500] if exc.response is not None else str(exc)
        log.warning("GitHub Copilot issue assignment failed: %s", detail)
        return "failed", detail or str(exc), requested_at
    except Exception as exc:
        log.warning("GitHub Copilot issue assignment failed: %s", exc)
        return "failed", str(exc), requested_at


async def _create_github_issue_record(
    github_token: str,
    github_repo: str,
    title: str,
    body_md: str,
    labels: list[str] | None = None,
    *,
    mask_output_enabled: bool = True,
) -> dict[str, Any]:
    if not github_token or not github_repo:
        return {}
    owner, repo = _parse_github_repo_owner_name(github_repo)
    if not owner or not repo:
        parts = [p for p in github_repo.strip("/").split("/") if p]
        if len(parts) >= 2:
            owner, repo = parts[-2], parts[-1]
    if not owner or not repo:
        return {}
    issue_title = _mask_output(title, mask_output_enabled)
    issue_body = _mask_output(body_md, mask_output_enabled)
    issue_payload: dict[str, Any] = {
        "title": issue_title,
        "body": issue_body,
        "labels": labels or ["sobs-agent", "automated"],
    }
    client = await _get_http_client()
    try:
        resp = await client.post(
            f"https://api.github.com/repos/{owner}/{repo}/issues",
            json=issue_payload,
            headers=_github_api_headers(github_token, include_content_type=True),
            timeout=15,
        )
        resp.raise_for_status()
        result = resp.json()
        return {
            "issue_url": str(result.get("html_url", "")),
            "issue_number": int(result.get("number", 0) or 0),
            "issue_title": str(result.get("title") or title),
            "issue_state": str(result.get("state") or "open"),
        }
    except httpx.HTTPStatusError as exc:
        detail = ""
        try:
            payload = exc.response.json()
            if isinstance(payload, dict):
                detail = str(payload.get("message") or "").strip()
        except Exception:
            detail = ""
        if not detail:
            detail = str(exc)
        log.warning("GitHub issue creation failed: %s", detail)
        return {"error": f"GitHub issue creation failed: {detail}"}
    except Exception as exc:
        log.warning("GitHub issue creation failed: %s", exc)
        return {"error": f"GitHub issue creation failed: {exc}"}


async def _create_github_issue(
    github_token: str,
    github_repo: str,
    title: str,
    body_md: str,
    labels: list[str] | None = None,
    *,
    mask_output_enabled: bool = True,
) -> str:
    """Create a GitHub issue and return the HTML URL."""
    result = await _create_github_issue_record(
        github_token,
        github_repo,
        title,
        body_md,
        labels,
        mask_output_enabled=mask_output_enabled,
    )
    return str(result.get("issue_url", ""))


# ---------------------------------------------------------------------------
# LLM-assisted deduplication
# ---------------------------------------------------------------------------


async def _classify_issue_dedupe_with_llm(
    settings: dict[str, str],
    proposed: dict[str, Any],
    candidates: list[dict[str, Any]],
) -> dict[str, Any]:
    endpoint_url = str(settings.get("ai.endpoint_url") or "").strip()
    model = str(settings.get("ai.model") or "").strip()
    api_key = str(settings.get("ai.api_key") or "").strip()
    thinking_level = str(settings.get("ai.thinking_level") or "off").strip() or "off"
    if not endpoint_url or not model or not candidates:
        return _fallback_issue_dedupe_decision(proposed, candidates)

    compact_candidates = [
        {
            "candidate_id": str(item.get("candidate_id") or ""),
            "issue_url": str(item.get("issue_url") or ""),
            "issue_title": str(item.get("issue_title") or ""),
            "service_name": str(item.get("service_name") or ""),
            "signal_source": str(item.get("signal_source") or ""),
            "signal_name": str(item.get("signal_name") or ""),
            "anomaly_state": str(item.get("anomaly_state") or ""),
            "dedup_key": str(item.get("dedup_key") or ""),
            "copilot_assignment_status": str(item.get("copilot_assignment_status") or ""),
            "has_open_pr": bool(item.get("pr_linked") or item.get("pr_url")),
            "assignees": list(item.get("assignees") or []),
        }
        for item in candidates[:_GITHUB_ISSUE_DEDUPE_CANDIDATE_LIMIT]
    ]
    prompt = {
        "task": "Classify whether the proposed observability incident matches any existing GitHub issue.",
        "return_json_only": True,
        "required_keys": ["classification", "candidate_id", "confidence", "reason"],
        "allowed_classifications": ["same", "related", "unrelated"],
        "proposed_incident": proposed,
        "candidates": compact_candidates,
    }
    reply_text, _stats = await _call_llm(
        endpoint_url,
        model,
        api_key,
        [
            {
                "role": "system",
                "content": (
                    "You classify observability incidents against existing GitHub issues. "
                    "Return a single JSON object only. Prefer 'same' only for clear duplicates, "
                    "'related' for likely same fault family but materially different work, otherwise 'unrelated'."
                ),
            },
            {"role": "user", "content": json.dumps(prompt, ensure_ascii=False)},
        ],
        thinking_level=thinking_level,
        max_tokens=400,
        timeout=25,
    )
    parsed = _extract_first_json_object(reply_text)
    classification = str(parsed.get("classification") or "").strip().lower()
    if classification not in {"same", "related", "unrelated"}:
        return _fallback_issue_dedupe_decision(proposed, candidates)
    candidate_id = str(parsed.get("candidate_id") or "").strip()
    try:
        confidence = float(parsed.get("confidence") or 0.0)
    except (TypeError, ValueError):
        confidence = 0.0
    return {
        "classification": classification,
        "candidate_id": candidate_id,
        "confidence": max(0.0, min(1.0, confidence)),
        "reason": str(parsed.get("reason") or "").strip(),
    }


# ---------------------------------------------------------------------------
# Orchestration: choose issue outcome
# ---------------------------------------------------------------------------


async def _choose_github_issue_outcome(
    db: Any,
    settings: dict[str, str],
    rule: dict,
    trigger_context: dict,
    *,
    github_repo: str,
    github_token: str,
    wants_copilot_assignment: bool,
    analysis: str,
    suggestion: str,
    issue_title: str,
    issue_body: str,
    allow_new_issue: bool,
    mask_output_enabled: bool = True,
) -> dict[str, Any]:
    trigger_fields = _extract_agent_trigger_fields(trigger_context)
    dedup_key = _build_github_work_item_dedup_key(github_repo, trigger_fields)
    local_candidates = _load_recent_work_item_candidates(db, github_repo)
    open_issues = await _fetch_open_github_issues(github_token, github_repo)
    open_issues_by_url = {str(item.get("issue_url") or ""): item for item in open_issues}

    candidates: list[dict[str, Any]] = []
    seen_urls: set[str] = set()
    for local_item in local_candidates:
        issue_url = str(local_item.get("issue_url") or "")
        if not issue_url or issue_url in seen_urls:
            continue
        open_item = open_issues_by_url.get(issue_url)
        if not open_item:
            continue
        candidate = {
            "candidate_id": issue_url,
            "issue_url": issue_url,
            "issue_number": int(open_item.get("issue_number") or local_item.get("issue_number") or 0),
            "issue_title": str(open_item.get("issue_title") or local_item.get("issue_title") or ""),
            "issue_body": str(open_item.get("issue_body") or ""),
            "issue_state": str(open_item.get("issue_state") or local_item.get("issue_state") or "open"),
            "service_name": str(local_item.get("service") or ""),
            "signal_source": str(local_item.get("signal_source") or ""),
            "signal_name": str(local_item.get("signal_name") or ""),
            "anomaly_state": str(local_item.get("anomaly_state") or ""),
            "dedup_key": str(local_item.get("dedup_key") or ""),
            "copilot_assignment_status": str(local_item.get("copilot_assignment_status") or ""),
            "pr_linked": bool(local_item.get("pr_linked")),
            "pr_url": str(local_item.get("pr_url") or ""),
            "assignees": list(open_item.get("assignees") or []),
        }
        candidates.append(candidate)
        seen_urls.add(issue_url)
    for open_item in open_issues:
        issue_url = str(open_item.get("issue_url") or "")
        if not issue_url or issue_url in seen_urls:
            continue
        candidates.append(
            {
                "candidate_id": issue_url,
                "issue_url": issue_url,
                "issue_number": int(open_item.get("issue_number") or 0),
                "issue_title": str(open_item.get("issue_title") or ""),
                "issue_body": str(open_item.get("issue_body") or ""),
                "issue_state": str(open_item.get("issue_state") or "open"),
                "service_name": "",
                "signal_source": "",
                "signal_name": "",
                "anomaly_state": "",
                "dedup_key": "",
                "copilot_assignment_status": "",
                "pr_linked": False,
                "pr_url": "",
                "assignees": list(open_item.get("assignees") or []),
            }
        )

    proposed = {
        "github_repo": github_repo,
        "service_name": str(trigger_fields.get("service_name") or ""),
        "signal_source": str(trigger_fields.get("signal_source") or ""),
        "signal_name": str(trigger_fields.get("signal_name") or ""),
        "anomaly_state": str(trigger_fields.get("anomaly_state") or ""),
        "dedup_key": dedup_key,
        "issue_title": issue_title,
        "analysis_summary": (analysis or "")[:300],
        "suggestion_summary": (suggestion or "")[:300],
    }
    classification = await _classify_issue_dedupe_with_llm(settings, proposed, candidates)
    classification_name = str(classification.get("classification") or "unrelated")
    candidate_id = str(classification.get("candidate_id") or "")
    matched = next((item for item in candidates if str(item.get("candidate_id") or "") == candidate_id), None)
    if classification_name in {"same", "related"} and matched:
        issue_url = str(matched.get("issue_url") or "")
        issue_number = int(matched.get("issue_number") or 0)
        pr_info = await _search_open_pr_for_issue(github_token, github_repo, issue_number)
        assignment_status = str(matched.get("copilot_assignment_status") or "not_requested")
        assignees = [str(item).lower() for item in (matched.get("assignees") or [])]
        if _GITHUB_COPILOT_ASSIGNEE.lower() in assignees or "copilot-swe-agent" in assignees:
            assignment_status = "active"
        occurrence_row = db.execute(
            "SELECT count() AS c FROM sobs_github_work_items FINAL WHERE IsDeleted=0 AND IssueUrl=?",
            [issue_url],
        ).fetchone()
        occurrence_count = int(occurrence_row["c"]) + 1 if occurrence_row else 1
        outcome: dict[str, Any] = {
            "issue_url": issue_url,
            "issue_number": issue_number,
            "issue_title": str(matched.get("issue_title") or issue_title),
            "issue_state": str(matched.get("issue_state") or "open"),
            "dedup_key": dedup_key,
            "dedup_decision": "reused_existing" if classification_name == "same" else "related_existing",
            "dedup_confidence": float(classification.get("confidence") or 0.0),
            "canonical_issue_url": issue_url,
            "canonical_issue_number": issue_number,
            "related_issue_urls": [issue_url],
            "occurrence_count": occurrence_count,
            "pr_linked": bool(pr_info and pr_info.get("pr_url")),
            "pr_number": int((pr_info or {}).get("pr_number", 0) or 0),
            "pr_url": str((pr_info or {}).get("pr_url", "") or ""),
            "copilot_assignment_status": assignment_status,
            "copilot_assignment_reason": str(classification.get("reason") or ""),
            "copilot_assignment_requested_at": 0,
            "created_new_issue": False,
        }
        if wants_copilot_assignment:
            max_assignments_per_hour = _parse_bounded_int_setting(
                settings,
                "ai.agent_max_assignments_per_hour",
                _AI_AGENT_MAX_ASSIGNMENTS_PER_HOUR_DEFAULT,
                1,
                20,
            )
            max_active_assignments = _parse_bounded_int_setting(
                settings,
                "ai.agent_max_active_assignments",
                _AI_AGENT_MAX_ACTIVE_ASSIGNMENTS_DEFAULT,
                1,
                10,
            )
            if outcome["pr_linked"]:
                outcome["copilot_assignment_status"] = "blocked"
                outcome["copilot_assignment_reason"] = "existing linked pull request already covers this issue"
            elif assignment_status in {"requested", "active"}:
                outcome["copilot_assignment_status"] = "blocked"
                outcome["copilot_assignment_reason"] = "issue is already being worked by Copilot"
            elif _count_copilot_assignments_last_hour(db) >= max_assignments_per_hour:
                outcome["copilot_assignment_status"] = "blocked"
                outcome["copilot_assignment_reason"] = "Copilot assignment hourly limit reached"
            elif _count_active_copilot_assignments(db) >= max_active_assignments:
                outcome["copilot_assignment_status"] = "blocked"
                outcome["copilot_assignment_reason"] = "active Copilot assignment limit reached"
            else:
                custom_instructions = str(settings.get("ai.github_copilot_custom_instructions") or "").strip()
                if suggestion:
                    custom_instructions = (
                        (custom_instructions + "\n\n") if custom_instructions else ""
                    ) + f"Use this suggested fix guidance when relevant:\n{suggestion[:1500]}"
                assign_status, assign_reason, requested_at = await _assign_issue_to_copilot(
                    github_token,
                    github_repo,
                    issue_number,
                    base_branch=str(settings.get("ai.github_copilot_base_branch") or "").strip(),
                    custom_instructions=custom_instructions,
                )
                outcome["copilot_assignment_status"] = assign_status
                outcome["copilot_assignment_reason"] = assign_reason
                outcome["copilot_assignment_requested_at"] = requested_at
        return outcome

    created: dict[str, Any] = {}
    if allow_new_issue:
        created = await _create_github_issue_record(
            github_token,
            github_repo,
            issue_title,
            issue_body,
            ["sobs-agent", "automated"],
            mask_output_enabled=mask_output_enabled,
        )

    creation_error = str(created.get("error") or "")
    if created.get("issue_url"):
        dedup_decision = "new_issue"
        dedup_confidence = 1.0
        assignment_reason = ""
    elif not allow_new_issue:
        dedup_decision = "suppressed_rate_limit"
        dedup_confidence = 0.0
        assignment_reason = "GitHub issue creation suppressed by hourly limit"
    else:
        dedup_decision = "create_failed"
        dedup_confidence = 0.0
        assignment_reason = creation_error or "GitHub issue creation failed"

    outcome = {
        "issue_url": str(created.get("issue_url") or ""),
        "issue_number": int(created.get("issue_number") or 0),
        "issue_title": str(created.get("issue_title") or issue_title),
        "issue_state": str(created.get("issue_state") or ("open" if created else "")),
        "dedup_key": dedup_key,
        "dedup_decision": dedup_decision,
        "dedup_confidence": dedup_confidence,
        "canonical_issue_url": str(created.get("issue_url") or ""),
        "canonical_issue_number": int(created.get("issue_number") or 0),
        "related_issue_urls": [],
        "occurrence_count": 1,
        "pr_linked": False,
        "pr_number": 0,
        "pr_url": "",
        "copilot_assignment_status": "not_requested",
        "copilot_assignment_reason": assignment_reason,
        "copilot_assignment_requested_at": 0,
        "created_new_issue": bool(created.get("issue_url")),
        "issue_error": creation_error,
    }
    if not created:
        outcome["copilot_assignment_status"] = "blocked" if wants_copilot_assignment else "not_requested"
        if dedup_decision == "create_failed":
            outcome["copilot_assignment_reason"] = assignment_reason
        return outcome

    if wants_copilot_assignment:
        max_assignments_per_hour = _parse_bounded_int_setting(
            settings,
            "ai.agent_max_assignments_per_hour",
            _AI_AGENT_MAX_ASSIGNMENTS_PER_HOUR_DEFAULT,
            1,
            20,
        )
        max_active_assignments = _parse_bounded_int_setting(
            settings,
            "ai.agent_max_active_assignments",
            _AI_AGENT_MAX_ACTIVE_ASSIGNMENTS_DEFAULT,
            1,
            10,
        )
        if _count_copilot_assignments_last_hour(db) >= max_assignments_per_hour:
            outcome["copilot_assignment_status"] = "blocked"
            outcome["copilot_assignment_reason"] = "Copilot assignment hourly limit reached"
            return outcome
        if _count_active_copilot_assignments(db) >= max_active_assignments:
            outcome["copilot_assignment_status"] = "blocked"
            outcome["copilot_assignment_reason"] = "active Copilot assignment limit reached"
            return outcome

        custom_instructions = str(settings.get("ai.github_copilot_custom_instructions") or "").strip()
        if suggestion:
            custom_instructions = (
                (custom_instructions + "\n\n") if custom_instructions else ""
            ) + f"Use this suggested fix guidance when relevant:\n{suggestion[:1500]}"
        assign_status, assign_reason, requested_at = await _assign_issue_to_copilot(
            github_token,
            github_repo,
            int(cast(Any, outcome.get("issue_number")) or 0),
            base_branch=str(settings.get("ai.github_copilot_base_branch") or "").strip(),
            custom_instructions=custom_instructions,
        )
        outcome["copilot_assignment_status"] = assign_status
        outcome["copilot_assignment_reason"] = assign_reason
        outcome["copilot_assignment_requested_at"] = requested_at
    return outcome


# ---------------------------------------------------------------------------
# Decision summary logging
# ---------------------------------------------------------------------------


def _emit_agent_issue_decision_summary(
    run_id: str,
    rule: dict[str, Any],
    trigger_context: dict[str, Any],
    issue_outcome: dict[str, Any],
    github_issue_url: str,
    wants_issue: bool,
    wants_copilot_assignment: bool,
    github_repo: str,
) -> None:
    if not wants_issue:
        return

    summary = {
        "run_id": str(run_id or ""),
        "rule_id": str(rule.get("id") or ""),
        "rule_name": str(rule.get("name") or ""),
        "trigger_type": str(trigger_context.get("trigger_type") or ""),
        "trigger_ref_id": str(trigger_context.get("trigger_ref_id") or ""),
        "github_repo": str(github_repo or ""),
        "issue_url": str(github_issue_url or issue_outcome.get("issue_url") or ""),
        "dedup_decision": str(issue_outcome.get("dedup_decision") or ""),
        "dedup_confidence": float(issue_outcome.get("dedup_confidence") or 0.0),
        "copilot_requested": bool(wants_copilot_assignment),
        "copilot_assignment_status": str(issue_outcome.get("copilot_assignment_status") or ""),
        "copilot_assignment_reason": str(issue_outcome.get("copilot_assignment_reason") or ""),
        "created_new_issue": bool(issue_outcome.get("created_new_issue")),
        "occurrence_count": int(issue_outcome.get("occurrence_count") or 0),
    }
    log.info("agent_issue_decision_summary %s", _json_dumps(summary))
