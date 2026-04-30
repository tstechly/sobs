"""Shared tag-rule helpers used by SOBS."""

from __future__ import annotations

import hashlib
import json
import re
from typing import Any


def _record_id_for_log(ts: str, service: str, trace_id: str, span_id: str) -> str:
    key = f"{service}|{ts}|{trace_id}|{span_id}"
    return hashlib.md5(key.encode()).hexdigest()


def _record_id_for_span(trace_id: str, span_id: str) -> str:
    key = f"{trace_id}|{span_id}"
    return hashlib.md5(key.encode()).hexdigest()


def _parse_tag_rule_conditions_json(raw: Any) -> list[dict[str, str]]:
    text = str(raw or "").strip()
    if not text:
        return []
    try:
        parsed = json.loads(text)
    except (TypeError, ValueError, json.JSONDecodeError):
        return []
    if not isinstance(parsed, list):
        return []

    normalized: list[dict[str, str]] = []
    for item in parsed:
        if not isinstance(item, dict):
            continue
        normalized.append(
            {
                "match_field": str(item.get("match_field", "") or ""),
                "match_operator": str(item.get("match_operator", "") or ""),
                "match_value": str(item.get("match_value", "") or ""),
                "match_attr_key": str(item.get("match_attr_key", "") or ""),
            }
        )
    return normalized


def _load_tag_rules(db, *, parse_tag_rule_conditions_json) -> list[dict[str, Any]]:
    rows = db.execute(
        "SELECT Id, Name, RecordTypes, MatchField, MatchOperator, MatchValue, "
        "MatchAttrKey, TagKey, TagValue, ConditionsJson "
        "FROM sobs_tag_rules FINAL WHERE IsDeleted = 0 ORDER BY Name"
    ).fetchall()
    loaded: list[dict[str, Any]] = []
    for row in rows:
        conditions = parse_tag_rule_conditions_json(row["ConditionsJson"])
        if not conditions and str(row["MatchField"] or "").strip():
            conditions = [
                {
                    "match_field": str(row["MatchField"] or ""),
                    "match_operator": str(row["MatchOperator"] or "eq"),
                    "match_value": str(row["MatchValue"] or ""),
                    "match_attr_key": str(row["MatchAttrKey"] or ""),
                }
            ]

        loaded.append(
            {
                "id": str(row["Id"]),
                "name": str(row["Name"]),
                "record_types": [token.strip() for token in str(row["RecordTypes"]).split(",") if token.strip()],
                "match_field": str(row["MatchField"]),
                "match_operator": str(row["MatchOperator"]),
                "match_value": str(row["MatchValue"]),
                "match_attr_key": str(row["MatchAttrKey"]),
                "tag_key": str(row["TagKey"]),
                "tag_value": str(row["TagValue"]),
                "conditions": conditions,
            }
        )
    return loaded


def _match_single_condition(
    cond: dict[str, Any],
    service: str,
    severity: str,
    body: str,
    attrs: dict[str, Any],
    span_name: str = "",
    event_type: str = "",
) -> bool:
    field = cond.get("match_field", "")
    if field == "service_name":
        value = service
    elif field == "severity":
        value = severity
    elif field == "body":
        value = body
    elif field == "span_name":
        value = span_name
    elif field == "event_type":
        value = event_type
    elif field == "attribute":
        value = str(attrs.get(cond.get("match_attr_key", ""), "")) if isinstance(attrs, dict) else ""
    else:
        value = ""

    operator = cond.get("match_operator", "")
    match_value = cond.get("match_value", "")
    if operator == "eq":
        return value == match_value
    if operator == "contains":
        return str(match_value).lower() in value.lower()
    if operator == "regex":
        try:
            return bool(re.search(str(match_value), value))
        except re.error:
            return False
    return False


def _match_tag_rule(
    rule: dict[str, Any],
    record_type: str,
    service: str,
    severity: str,
    body: str,
    attrs: dict[str, Any],
    span_name: str = "",
    event_type: str = "",
    *,
    match_single_condition,
) -> bool:
    rule_types = rule["record_types"]
    if rule_types and "all" not in rule_types and record_type not in rule_types:
        return False

    conditions: list[dict[str, Any]] = rule.get("conditions") or []
    if conditions:
        return all(
            match_single_condition(cond, service, severity, body, attrs, span_name, event_type) for cond in conditions
        )

    return match_single_condition(
        {
            "match_field": rule["match_field"],
            "match_operator": rule["match_operator"],
            "match_value": rule["match_value"],
            "match_attr_key": rule["match_attr_key"],
        },
        service,
        severity,
        body,
        attrs,
        span_name,
        event_type,
    )


def _tag_rule_attribute_key_suggestions(
    db, query_text: str, limit: int, *, attr_key_record_types, get_cached_attr_keys
):
    keys: set[str] = set()
    for record_type in attr_key_record_types:
        keys.update(get_cached_attr_keys(db, record_type))

    query = query_text.strip().lower()
    ranked = sorted(
        (key for key in keys if key),
        key=lambda key: (
            0 if query and key.lower().startswith(query) else 1,
            0 if query and query in key.lower() else 1,
            key.lower(),
        ),
    )
    if query:
        ranked = [key for key in ranked if query in key.lower()]
    return ranked[:limit]
