from __future__ import annotations

import re
from collections import Counter
from typing import Any


def _compute_log_stats(db: Any, where_clause: str, params: list[Any]) -> tuple[dict[str, Any], dict[str, Any]]:
    level_query = (
        "SELECT SeverityText, COUNT(*) AS cnt "
        f"FROM otel_logs {where_clause} "
        "GROUP BY SeverityText ORDER BY cnt DESC"
    )
    level_stats = {(row["SeverityText"] or "UNKNOWN"): row["cnt"] for row in db.execute(level_query, params).fetchall()}

    service_condition = "AND ServiceName!=''" if where_clause else "WHERE ServiceName!=''"
    service_query = (
        "SELECT ServiceName, COUNT(*) AS cnt "
        f"FROM otel_logs {where_clause} {service_condition} "
        "GROUP BY ServiceName ORDER BY cnt DESC LIMIT 10"
    )
    service_stats = {row["ServiceName"]: row["cnt"] for row in db.execute(service_query, params).fetchall()}
    return level_stats, service_stats


def _fingerprint_log_message(message: str) -> str:
    normalized = (message or "").strip().lower()
    if not normalized:
        return "(empty message)"

    patterns = [
        (r"\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b", "<uuid>"),
        (r"\b0x[0-9a-f]+\b", "<hex>"),
        (r"\b[0-9a-f]{16,}\b", "<hash>"),
        (r"\b\d{4,}\b", "<num>"),
        (r"\b\d+\b", "<n>"),
    ]
    for pattern, replacement in patterns:
        normalized = re.sub(pattern, replacement, normalized)

    normalized = re.sub(r"'[^']*'", "'<text>'", normalized)
    normalized = re.sub(r'"[^"]*"', '"<text>"', normalized)
    normalized = re.sub(r"\s+", " ", normalized).strip()
    return normalized[:160]


def _compute_advanced_log_analysis(
    rows: list[dict[str, Any]],
    level_stats: dict[str, Any],
    service_stats: dict[str, Any],
    *,
    map_to_dict: Any,
    fingerprint_log_message: Any = _fingerprint_log_message,
) -> dict[str, Any]:
    messages = [str(row["Body"] or "") for row in rows if row["Body"]]
    if not messages:
        return {
            "top_patterns": [],
            "top_keywords": [],
            "error_families": [],
            "hints": [],
        }

    fingerprint_counts: Counter[str] = Counter(fingerprint_log_message(message) for message in messages)
    most_common_patterns = fingerprint_counts.most_common(8)
    top_patterns = [{"pattern": pattern, "count": count} for pattern, count in most_common_patterns]

    family_regex = re.compile(r"\b([A-Za-z_][A-Za-z0-9_]*(?:Error|Exception|Timeout|Refused|Unavailable|Failure))\b")
    family_counts: Counter[str] = Counter()

    for row in rows:
        attrs = map_to_dict(row.get("LogAttributes"))
        exception_type = str(attrs.get("exception.type", "")).strip()
        if exception_type:
            family_counts[exception_type] += 1

    for message in messages:
        for family in set(family_regex.findall(message)):
            family_counts[family] += 1
    error_families = [{"family": family, "count": count} for family, count in family_counts.most_common(8)]

    stop_words = {
        "the",
        "and",
        "for",
        "with",
        "from",
        "into",
        "this",
        "that",
        "http",
        "https",
        "failed",
        "error",
        "warn",
        "info",
        "debug",
        "trace",
        "service",
    }
    keyword_counts: Counter[str] = Counter()
    for message in messages:
        for token in re.findall(r"[a-z][a-z0-9_\-]{2,}", message.lower()):
            if token not in stop_words:
                keyword_counts[token] += 1
    top_keywords = [{"keyword": keyword, "count": count} for keyword, count in keyword_counts.most_common(10)]

    hints = []
    total = max(len(rows), 1)
    severe = sum(
        int(count)
        for level, count in level_stats.items()
        if str(level).upper() in {"ERROR", "FATAL", "CRITICAL", "ALERT", "EMERGENCY"}
    )
    severe_ratio = severe / total
    if severe_ratio >= 0.25:
        hints.append(
            f"High severe-log ratio ({severe_ratio:.0%}); prioritize stabilizing error paths before scaling traffic."
        )

    if most_common_patterns and most_common_patterns[0][1] >= 3:
        top_count = most_common_patterns[0][1]
        hints.append(
            "Most frequent message pattern repeats "
            f"{top_count} times; consider deduplication/sampling and shared remediation guidance."
        )

    timeout_hits = keyword_counts.get("timeout", 0) + keyword_counts.get("timed", 0)
    if timeout_hits >= 3:
        hints.append("Timeout-related logs are common; review dependency latency, retry budgets, and circuit breakers.")

    if service_stats:
        top_service, top_service_count = next(iter(service_stats.items()))
        if int(top_service_count) / total >= 0.6:
            hints.append(
                f"Most events come from {top_service}; investigate service-level hotspots and noisy call paths."
            )

    return {
        "top_patterns": top_patterns,
        "top_keywords": top_keywords,
        "error_families": error_families,
        "hints": hints,
    }
