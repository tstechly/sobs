"""GitHub issue helpers shared across SOBS modules."""

from __future__ import annotations

import json
import logging
import re
from collections.abc import Awaitable, Callable
from typing import Any

import httpx

from shared.github import _parse_github_repo_owner_name


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


def _normalize_issue_match_text(value: Any) -> str:
    text = re.sub(r"[^a-z0-9]+", " ", str(value or "").lower())
    return " ".join(text.split())


def _safe_json_loads(value: object, default: Any) -> Any:
    if value is None:
        return default
    if isinstance(value, (dict, list)):
        return value
    try:
        parsed = json.loads(str(value))
    except (TypeError, ValueError, json.JSONDecodeError):
        return default
    return parsed if isinstance(parsed, type(default)) else default


def _extract_first_json_object(text: str) -> dict[str, Any]:
    raw = str(text or "").strip()
    if not raw:
        return {}
    if raw.startswith("```"):
        raw = re.sub(r"^```(?:json)?\s*|\s*```$", "", raw, flags=re.DOTALL).strip()
    try:
        parsed = json.loads(raw)
    except (TypeError, ValueError, json.JSONDecodeError):
        parsed = None
    if isinstance(parsed, dict):
        return parsed
    match = re.search(r"\{.*\}", raw, flags=re.DOTALL)
    if not match:
        return {}
    try:
        parsed = json.loads(match.group(0))
    except (TypeError, ValueError, json.JSONDecodeError):
        return {}
    return parsed if isinstance(parsed, dict) else {}


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


async def _classify_issue_dedupe_with_llm(
    settings: dict[str, str],
    proposed: dict[str, Any],
    candidates: list[dict[str, Any]],
    *,
    call_llm_endpoint: Callable[..., Awaitable[tuple[str, Any]]],
    candidate_limit: int = 10,
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
        for item in candidates[:candidate_limit]
    ]
    prompt = {
        "task": "Classify whether the proposed observability incident matches any existing GitHub issue.",
        "return_json_only": True,
        "required_keys": ["classification", "candidate_id", "confidence", "reason"],
        "allowed_classifications": ["same", "related", "unrelated"],
        "proposed_incident": proposed,
        "candidates": compact_candidates,
    }
    reply_text, _stats = await call_llm_endpoint(
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


async def _fetch_open_github_issues(
    github_token: str,
    github_repo: str,
    *,
    get_async_http_client: Callable[[], Awaitable[Any]],
    logger: logging.Logger | None = None,
    limit: int = 10,
) -> list[dict[str, Any]]:
    if not github_token or not github_repo:
        return []
    owner, repo = _parse_github_repo_owner_name(github_repo)
    if not owner or not repo:
        return []
    client = await get_async_http_client()
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
        if logger is not None:
            logger.warning("GitHub open issue fetch failed for %s/%s: %s", owner, repo, exc)
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
                    str(assignee.get("login") or "")
                    for assignee in (item.get("assignees") or [])
                    if isinstance(assignee, dict)
                ],
            }
        )
    return issues


async def _search_open_pr_for_issue(
    github_token: str,
    github_repo: str,
    issue_number: int,
    *,
    get_async_http_client: Callable[[], Awaitable[Any]],
) -> dict[str, Any] | None:
    if not github_token or not github_repo or issue_number <= 0:
        return None
    owner, repo = _parse_github_repo_owner_name(github_repo)
    if not owner or not repo:
        return None
    client = await get_async_http_client()
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


async def _create_github_issue_record(
    github_token: str,
    github_repo: str,
    title: str,
    body_md: str,
    labels: list[str] | None = None,
    *,
    get_async_http_client: Callable[[], Awaitable[Any]],
    mask_string_for_output: Callable[[Any], str],
    logger: logging.Logger | None = None,
    mask_output_enabled: bool = True,
) -> dict[str, Any]:
    if not github_token or not github_repo:
        return {}
    owner, repo = _parse_github_repo_owner_name(github_repo)
    if not owner or not repo:
        parts = [part for part in github_repo.strip("/").split("/") if part]
        if len(parts) >= 2:
            owner, repo = parts[-2], parts[-1]
    if not owner or not repo:
        return {}
    issue_title = mask_string_for_output(title) if mask_output_enabled else str(title or "")
    issue_body = mask_string_for_output(body_md) if mask_output_enabled else str(body_md or "")
    issue_payload: dict[str, Any] = {
        "title": issue_title,
        "body": issue_body,
        "labels": labels or ["sobs-agent", "automated"],
    }
    client = await get_async_http_client()
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
        if logger is not None:
            logger.warning("GitHub issue creation failed: %s", detail)
        return {"error": f"GitHub issue creation failed: {detail}"}
    except Exception as exc:
        if logger is not None:
            logger.warning("GitHub issue creation failed: %s", exc)
        return {"error": f"GitHub issue creation failed: {exc}"}


async def _create_github_issue(
    github_token: str,
    github_repo: str,
    title: str,
    body_md: str,
    labels: list[str] | None = None,
    *,
    get_async_http_client: Callable[[], Awaitable[Any]],
    mask_string_for_output: Callable[[Any], str],
    logger: logging.Logger | None = None,
    mask_output_enabled: bool = True,
) -> str:
    result = await _create_github_issue_record(
        github_token,
        github_repo,
        title,
        body_md,
        labels,
        get_async_http_client=get_async_http_client,
        mask_string_for_output=mask_string_for_output,
        logger=logger,
        mask_output_enabled=mask_output_enabled,
    )
    return str(result.get("issue_url", ""))


async def _github_get_issue_detail(
    github_token: str,
    github_repo: str,
    issue_number: int,
    *,
    get_async_http_client: Callable[[], Awaitable[Any]],
) -> dict[str, Any]:
    if not github_token or not github_repo or issue_number <= 0:
        return {}
    owner, repo = _parse_github_repo_owner_name(github_repo)
    if not owner or not repo:
        return {}
    client = await get_async_http_client()
    try:
        resp = await client.get(
            f"https://api.github.com/repos/{owner}/{repo}/issues/{issue_number}",
            headers=_github_api_headers(github_token),
            timeout=15,
        )
        resp.raise_for_status()
        payload = resp.json() if resp.content else {}
        return payload if isinstance(payload, dict) else {}
    except Exception:
        return {}


def _github_issue_is_new_state(issue_payload: dict[str, Any]) -> bool:
    if not isinstance(issue_payload, dict):
        return False
    state = str(issue_payload.get("state") or "").strip().lower()
    comments = int(issue_payload.get("comments") or 0)
    created_at = str(issue_payload.get("created_at") or "").strip()
    updated_at = str(issue_payload.get("updated_at") or "").strip()
    return state == "open" and comments == 0 and bool(created_at) and created_at == updated_at


async def _update_github_issue_record(
    github_token: str,
    github_repo: str,
    issue_number: int,
    title: str,
    body_md: str,
    labels: list[str] | None = None,
    *,
    get_async_http_client: Callable[[], Awaitable[Any]],
    mask_string_for_output: Callable[[Any], str],
    logger: logging.Logger | None = None,
    mask_output_enabled: bool = True,
) -> dict[str, Any]:
    if not github_token or not github_repo or issue_number <= 0:
        return {}
    owner, repo = _parse_github_repo_owner_name(github_repo)
    if not owner or not repo:
        return {}
    issue_title = mask_string_for_output(title) if mask_output_enabled else str(title or "")
    issue_body = mask_string_for_output(body_md) if mask_output_enabled else str(body_md or "")
    issue_payload: dict[str, Any] = {"title": issue_title, "body": issue_body}
    if labels is not None:
        issue_payload["labels"] = labels

    client = await get_async_http_client()
    try:
        resp = await client.patch(
            f"https://api.github.com/repos/{owner}/{repo}/issues/{issue_number}",
            json=issue_payload,
            headers=_github_api_headers(github_token, include_content_type=True),
            timeout=15,
        )
        resp.raise_for_status()
        result = resp.json() if resp.content else {}
        return {
            "issue_url": str(result.get("html_url", "")),
            "issue_number": int(result.get("number", 0) or issue_number),
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
        if logger is not None:
            logger.warning("GitHub issue update failed: %s", detail)
        return {"error": f"GitHub issue update failed: {detail}"}
    except Exception as exc:
        if logger is not None:
            logger.warning("GitHub issue update failed: %s", exc)
        return {"error": f"GitHub issue update failed: {exc}"}


async def _create_or_update_onboarding_issue(
    github_token: str,
    github_repo: str,
    title: str,
    body_md: str,
    labels: list[str],
    *,
    get_async_http_client: Callable[[], Awaitable[Any]],
    mask_string_for_output: Callable[[Any], str],
    logger: logging.Logger | None = None,
    open_issue_limit: int = 10,
    fetch_open_github_issues: Callable[..., Awaitable[list[dict[str, Any]]]] | None = None,
    create_github_issue_record: Callable[..., Awaitable[dict[str, Any]]] | None = None,
    github_get_issue_detail: Callable[..., Awaitable[dict[str, Any]]] | None = None,
    update_github_issue_record: Callable[..., Awaitable[dict[str, Any]]] | None = None,
    github_issue_is_new_state: Callable[[dict[str, Any]], bool] | None = None,
) -> dict[str, Any]:
    if fetch_open_github_issues is None:
        open_issues = await _fetch_open_github_issues(
            github_token,
            github_repo,
            get_async_http_client=get_async_http_client,
            logger=logger,
            limit=open_issue_limit,
        )
    else:
        open_issues = await fetch_open_github_issues(github_token, github_repo, limit=open_issue_limit)
    title_norm = str(title or "").strip()
    existing = next((item for item in open_issues if str(item.get("issue_title") or "").strip() == title_norm), None)

    if not existing:
        if create_github_issue_record is None:
            created = await _create_github_issue_record(
                github_token,
                github_repo,
                title,
                body_md,
                labels=labels,
                get_async_http_client=get_async_http_client,
                mask_string_for_output=mask_string_for_output,
                logger=logger,
                mask_output_enabled=False,
            )
        else:
            created = await create_github_issue_record(
                github_token,
                github_repo,
                title,
                body_md,
                labels=labels,
                mask_output_enabled=False,
            )
        if "error" in created:
            return created
        created["status"] = "created"
        created["note"] = "Created a new onboarding issue."
        return created

    issue_number = int(existing.get("issue_number", 0) or 0)
    issue_url = str(existing.get("issue_url", ""))
    if github_get_issue_detail is None:
        detail = await _github_get_issue_detail(
            github_token,
            github_repo,
            issue_number,
            get_async_http_client=get_async_http_client,
        )
    else:
        detail = await github_get_issue_detail(github_token, github_repo, issue_number)

    issue_is_new = github_issue_is_new_state or _github_issue_is_new_state
    if detail and issue_is_new(detail):
        if update_github_issue_record is None:
            updated = await _update_github_issue_record(
                github_token,
                github_repo,
                issue_number,
                title,
                body_md,
                labels=labels,
                get_async_http_client=get_async_http_client,
                mask_string_for_output=mask_string_for_output,
                logger=logger,
                mask_output_enabled=False,
            )
        else:
            updated = await update_github_issue_record(
                github_token,
                github_repo,
                issue_number,
                title,
                body_md,
                labels=labels,
                mask_output_enabled=False,
            )
        if "error" in updated:
            return updated
        updated["status"] = "updated"
        updated["note"] = "Updated the existing onboarding issue because it was still new."
        return updated

    existing_state = str((detail or {}).get("state") or existing.get("issue_state") or "open")
    return {
        "issue_url": str((detail or {}).get("html_url") or issue_url),
        "issue_number": issue_number,
        "issue_title": str((detail or {}).get("title") or existing.get("issue_title") or title),
        "issue_state": existing_state,
        "status": "reused",
        "note": "Existing onboarding issue is not in new state; left unchanged.",
    }
