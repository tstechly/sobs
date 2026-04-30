"""Shared notification loader and normalization helpers used by SOBS."""

from __future__ import annotations

import json
from typing import Any


def _load_notification_channels(db, *, decrypt_notification_config) -> list[dict[str, Any]]:
    rows = db.execute(
        "SELECT Id, Name, ChannelType, ConfigJson, Enabled "
        "FROM sobs_notification_channels FINAL WHERE IsDeleted = 0 ORDER BY Name"
    ).fetchall()
    return [
        {
            "id": str(row["Id"]),
            "name": str(row["Name"]),
            "channel_type": str(row["ChannelType"]),
            "config": decrypt_notification_config(json.loads(str(row["ConfigJson"]) or "{}")),
            "enabled": bool(int(row["Enabled"])),
        }
        for row in rows
    ]


def _normalize_notification_condition(
    raw: Any,
    *,
    comparators,
    tag_match_operators,
    tag_record_types,
) -> dict[str, Any] | None:
    if not isinstance(raw, dict):
        return None

    condition_type = str(raw.get("type") or "signal").strip().lower()
    if condition_type == "tag":
        record_type = str(raw.get("record_type") or "all").strip().lower()
        if record_type not in tag_record_types:
            record_type = "all"
        tag_match_operator = str(raw.get("tag_match_operator") or "eq").strip().lower()
        if tag_match_operator not in tag_match_operators:
            tag_match_operator = "eq"
        comparator = str(raw.get("comparator") or "gt").strip().lower()
        if comparator not in comparators:
            comparator = "gt"
        try:
            threshold = float(raw.get("threshold") or 0)
        except (TypeError, ValueError):
            threshold = 0.0
        try:
            window_minutes = max(1, min(60, int(raw.get("window_minutes") or 5)))
        except (TypeError, ValueError):
            window_minutes = 5
        return {
            "type": "tag",
            "record_type": record_type,
            "tag_key": str(raw.get("tag_key") or "").strip(),
            "tag_match_operator": tag_match_operator,
            "tag_value": str(raw.get("tag_value") or "").strip(),
            "comparator": comparator,
            "threshold": threshold,
            "window_minutes": window_minutes,
        }

    comparator = str(raw.get("comparator") or "gt").strip().lower()
    if comparator not in comparators:
        comparator = "gt"
    try:
        threshold = float(raw.get("threshold") or 0)
    except (TypeError, ValueError):
        threshold = 0.0
    try:
        window_minutes = max(1, min(60, int(raw.get("window_minutes") or 5)))
    except (TypeError, ValueError):
        window_minutes = 5
    return {
        "type": "signal",
        "source": str(raw.get("source") or "").strip(),
        "signal": str(raw.get("signal") or "").strip(),
        "service": str(raw.get("service") or "").strip(),
        "comparator": comparator,
        "threshold": threshold,
        "window_minutes": window_minutes,
    }


def _parse_notification_conditions_json(raw: Any, *, normalize_notification_condition) -> list[dict[str, Any]]:
    text = str(raw or "").strip()
    if not text:
        return []
    try:
        parsed = json.loads(text)
    except (TypeError, ValueError, json.JSONDecodeError):
        return []
    if not isinstance(parsed, list):
        return []

    normalized: list[dict[str, Any]] = []
    for item in parsed:
        condition = normalize_notification_condition(item)
        if condition is not None:
            normalized.append(condition)
    return normalized


def _load_notification_rules(db, *, parse_notification_conditions_json) -> list[dict[str, Any]]:
    rows = db.execute(
        "SELECT Id, Name, Enabled, LogicOperator, ConditionsJson, ChannelIds, "
        "Severity, CooldownSeconds, LastFiredAt "
        "FROM sobs_notification_rules FINAL WHERE IsDeleted = 0 ORDER BY Name"
    ).fetchall()
    return [
        {
            "id": str(row["Id"]),
            "name": str(row["Name"]),
            "enabled": bool(int(row["Enabled"])),
            "logic_operator": str(row["LogicOperator"] or "any"),
            "conditions": parse_notification_conditions_json(row["ConditionsJson"]),
            "channel_ids": [channel.strip() for channel in str(row["ChannelIds"]).split(",") if channel.strip()],
            "severity": str(row["Severity"] or "warning"),
            "cooldown_seconds": int(row["CooldownSeconds"]),
            "last_fired_at": str(row["LastFiredAt"]),
        }
        for row in rows
    ]


def _load_notification_log(db, limit: int = 50) -> list[dict[str, Any]]:
    rows = db.execute(
        "SELECT Id, RuleId, RuleName, ChannelId, ChannelName, FiredAt, Status, ErrorMessage, Summary "
        "FROM sobs_notification_log ORDER BY FiredAt DESC LIMIT ?",
        [limit],
    ).fetchall()
    return [
        {
            "id": str(row["Id"]),
            "rule_id": str(row["RuleId"]),
            "rule_name": str(row["RuleName"]),
            "channel_id": str(row["ChannelId"]),
            "channel_name": str(row["ChannelName"]),
            "fired_at": str(row["FiredAt"]),
            "status": str(row["Status"]),
            "error_message": str(row["ErrorMessage"]),
            "summary": str(row["Summary"]),
        }
        for row in rows
    ]


def _mask_channel_config(channel_type: str, config: dict[str, Any]) -> dict[str, Any]:
    masked = dict(config)
    sensitive_keys = {"smtp_password", "auth_token", "api_key"}
    for key in sensitive_keys:
        if key in masked and masked[key]:
            masked[key] = "••••••••"
    return masked


def _notification_channel_mask_output_enabled(channel: dict[str, Any], *, is_truthy_setting) -> bool:
    config = channel.get("config") if isinstance(channel, dict) else {}
    if not isinstance(config, dict):
        return True
    raw = config.get("mask_output_enabled")
    if raw is None or str(raw).strip() == "":
        return True
    return is_truthy_setting(str(raw), default=True)
