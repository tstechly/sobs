from __future__ import annotations

import hashlib
from datetime import datetime, timedelta
from typing import Any


def _ensure_raw_metrics_retention(
    db,
    *,
    baseline_ttl_hours: int,
    pinned_ttl_days: int,
    logger: Any,
) -> None:
    statements = [
        f"ALTER TABLE otel_metrics_gauge MODIFY TTL TimeUnixMs + INTERVAL {baseline_ttl_hours} HOUR",
        f"ALTER TABLE otel_metrics_sum MODIFY TTL TimeUnixMs + INTERVAL {baseline_ttl_hours} HOUR",
        f"ALTER TABLE otel_metrics_histogram MODIFY TTL TimeUnixMs + INTERVAL {baseline_ttl_hours} HOUR",
        f"ALTER TABLE otel_metrics_gauge_pinned MODIFY TTL TimeUnixMs + INTERVAL {pinned_ttl_days} DAY",
        f"ALTER TABLE otel_metrics_sum_pinned MODIFY TTL TimeUnixMs + INTERVAL {pinned_ttl_days} DAY",
        f"ALTER TABLE otel_metrics_histogram_pinned MODIFY TTL TimeUnixMs + INTERVAL {pinned_ttl_days} DAY",
    ]
    for statement in statements:
        try:
            db.execute(statement)
        except Exception:
            logger.debug("raw metrics retention alter skipped: %s", statement, exc_info=True)


def _register_raw_window(
    db,
    *,
    signal_ts: datetime,
    signal_type: str,
    signal_ref: str,
    service_name: str = "",
    namespace: str = "",
    node_name: str = "",
    raw_metrics_window_minutes: int,
    insert_rows_json_each_row,
    now_ms: int,
) -> str:
    window_start = signal_ts - timedelta(minutes=raw_metrics_window_minutes)
    window_end = signal_ts + timedelta(minutes=raw_metrics_window_minutes)

    dedup_key = "|".join(
        [
            signal_ts.strftime("%Y-%m-%dT%H:%M"),
            signal_type[:64],
            signal_ref[:128],
            service_name[:64],
            namespace[:64],
            node_name[:64],
        ]
    )
    window_id = hashlib.sha256(dedup_key.encode()).hexdigest()[:32]

    ts_fmt = "%Y-%m-%d %H:%M:%S.%f"
    insert_rows_json_each_row(
        db,
        "sobs_raw_windows",
        [
            {
                "Id": window_id,
                "SignalTs": signal_ts.strftime(ts_fmt)[:-3],
                "WindowStart": window_start.strftime(ts_fmt)[:-3],
                "WindowEnd": window_end.strftime(ts_fmt)[:-3],
                "SignalType": signal_type[:64],
                "SignalRef": signal_ref[:256],
                "ServiceName": service_name[:128],
                "Namespace": namespace[:128],
                "NodeName": node_name[:128],
                "Version": now_ms,
            }
        ],
    )
    return window_id


def _window_copy_counts(db, window_ids: list[str]) -> dict[str, int]:
    if not window_ids:
        return {}
    placeholders = ",".join(["?"] * len(window_ids))
    rows = db.execute(
        "SELECT WindowId, countDistinct(SourceTable) AS c "
        "FROM sobs_raw_window_copy_state FINAL "
        f"WHERE WindowId IN ({placeholders}) "
        "GROUP BY WindowId",
        window_ids,
    ).fetchall()
    return {str(row["WindowId"]): int(row["c"] or 0) for row in rows}


def _list_trace_overlapping_raw_windows(
    db,
    service_names: list[str],
    start_ts: str,
    end_ts: str,
    limit: int = 25,
    *,
    raw_metric_tables: tuple[str, ...],
    window_copy_counts=_window_copy_counts,
) -> list[dict[str, object]]:
    where_parts = [
        "WindowEnd >= parseDateTime64BestEffort(?, 9)",
        "WindowStart <= parseDateTime64BestEffort(?, 9)",
    ]
    params: list[object] = [start_ts, end_ts]
    if service_names:
        placeholders = ",".join(["?"] * len(service_names))
        where_parts.append(f"(ServiceName = '' OR ServiceName IN ({placeholders}))")
        params.extend(service_names)
    where_sql = " AND ".join(where_parts)
    rows = db.execute(
        "SELECT Id, SignalType, SignalRef, ServiceName, Namespace, NodeName, WindowStart, WindowEnd "
        "FROM sobs_raw_windows FINAL "
        f"WHERE {where_sql} "
        "ORDER BY WindowStart DESC "
        "LIMIT ?",
        params + [max(1, min(limit, 100))],
    ).fetchall()
    if not rows:
        return []

    expected_count = len(raw_metric_tables)
    window_ids = [str(row["Id"]) for row in rows]
    copied_counts = window_copy_counts(db, window_ids)

    out: list[dict[str, object]] = []
    for row in rows:
        window_id = str(row["Id"])
        copied_count = copied_counts.get(window_id, 0)
        out.append(
            {
                "id": window_id,
                "signal_type": str(row["SignalType"]),
                "signal_ref": str(row["SignalRef"]),
                "service_name": str(row["ServiceName"]),
                "namespace": str(row["Namespace"]),
                "node_name": str(row["NodeName"]),
                "window_start": str(row["WindowStart"]),
                "window_end": str(row["WindowEnd"]),
                "copied_count": copied_count,
                "expected_count": expected_count,
                "copy_complete": copied_count >= expected_count,
            }
        )
    return out


def _run_raw_window_copy_worker(
    db,
    *,
    raw_window_copy_max_per_run: int,
    raw_metric_tables: tuple[str, ...],
    pinned_metric_tables: tuple[str, ...],
    insert_rows_json_each_row,
    now_ms: int,
    logger: Any,
) -> dict[str, int]:
    stats: dict[str, int] = {"windows_attempted": 0, "copies_ok": 0, "copies_error": 0}

    try:
        windows = db.execute(
            "SELECT Id, WindowStart, WindowEnd, ServiceName, Namespace, NodeName "
            "FROM sobs_raw_windows FINAL "
            "ORDER BY WindowStart DESC "
            f"LIMIT {raw_window_copy_max_per_run * 20}"
        ).fetchall()
    except Exception:
        logger.debug("raw window copy: failed to fetch windows", exc_info=True)
        return stats

    if not windows:
        return stats

    copies_attempted = 0
    for window_row in windows:
        if copies_attempted >= raw_window_copy_max_per_run:
            break

        window_id = str(window_row["Id"])
        window_start = str(window_row["WindowStart"])
        window_end = str(window_row["WindowEnd"])
        service_name = str(window_row.get("ServiceName") or "")
        namespace = str(window_row.get("Namespace") or "")
        node_name = str(window_row.get("NodeName") or "")

        for raw_table, pinned_table in zip(raw_metric_tables, pinned_metric_tables):
            if copies_attempted >= raw_window_copy_max_per_run:
                break

            try:
                already_copied = db.execute(
                    "SELECT 1 FROM sobs_raw_window_copy_state FINAL WHERE WindowId=? AND SourceTable=? LIMIT 1",
                    [window_id, raw_table],
                ).fetchone()
            except Exception:
                logger.debug(
                    "raw window copy: failed to check copy state for window=%s table=%s",
                    window_id,
                    raw_table,
                    exc_info=True,
                )
                continue

            if already_copied is not None:
                continue

            stats["windows_attempted"] += 1

            where_clauses = [
                "TimeUnix >= parseDateTime64BestEffort(?, 9)",
                "TimeUnix <= parseDateTime64BestEffort(?, 9)",
            ]
            params: list[object] = [window_start, window_end]

            if service_name:
                where_clauses.append("ServiceName = ?")
                params.append(service_name)
            if namespace:
                where_clauses.append("Attributes['k8s.namespace.name'] = ?")
                params.append(namespace)
            if node_name:
                where_clauses.append("Attributes['k8s.node.name'] = ?")
                params.append(node_name)

            where_sql = " AND ".join(where_clauses)

            if raw_table == "otel_metrics_histogram":
                select_cols = (
                    "TimeUnix, TimeUnixMs, ServiceName, MetricName, MetricDescription, "
                    "MetricUnit, Attributes, Count, Sum, BucketCounts, ExplicitBounds, "
                    "Flags, AggregationTemporality, AttrFingerprint"
                )
            elif raw_table == "otel_metrics_sum":
                select_cols = (
                    "TimeUnix, TimeUnixMs, ServiceName, MetricName, MetricDescription, "
                    "MetricUnit, Attributes, Value, Flags, IsMonotonic, "
                    "AggregationTemporality, AttrFingerprint"
                )
            else:
                select_cols = (
                    "TimeUnix, TimeUnixMs, ServiceName, MetricName, MetricDescription, "
                    "MetricUnit, Attributes, Value, Flags, AttrFingerprint"
                )

            try:
                count_row = db.execute(f"SELECT count() AS cnt FROM {raw_table} WHERE {where_sql}", params).fetchone()
                matched_rows = int((count_row or {}).get("cnt", 0))
                if matched_rows <= 0:
                    continue

                missing_row = db.execute(
                    f"SELECT count() AS cnt FROM {raw_table} WHERE {where_sql} "
                    f"AND (ServiceName, MetricName, AttrFingerprint, TimeUnix) NOT IN ("
                    f"SELECT ServiceName, MetricName, AttrFingerprint, TimeUnix "
                    f"FROM {pinned_table} WHERE {where_sql})",
                    params * 2,
                ).fetchone()
                missing_rows = int((missing_row or {}).get("cnt", 0))
                if missing_rows <= 0:
                    insert_rows_json_each_row(
                        db,
                        "sobs_raw_window_copy_state",
                        [{"WindowId": window_id, "SourceTable": raw_table, "Version": now_ms}],
                    )
                    copies_attempted += 1
                    stats["copies_ok"] += 1
                    continue

                db.execute(
                    f"INSERT INTO {pinned_table} ({select_cols}) "
                    f"SELECT {select_cols} FROM {raw_table} WHERE {where_sql} "
                    f"AND (ServiceName, MetricName, AttrFingerprint, TimeUnix) NOT IN ("
                    f"SELECT ServiceName, MetricName, AttrFingerprint, TimeUnix "
                    f"FROM {pinned_table} WHERE {where_sql})",
                    params * 2,
                )
                insert_rows_json_each_row(
                    db,
                    "sobs_raw_window_copy_state",
                    [{"WindowId": window_id, "SourceTable": raw_table, "Version": now_ms}],
                )
                copies_attempted += 1
                stats["copies_ok"] += 1
            except Exception:
                copies_attempted += 1
                logger.debug("raw window copy error: window=%s table=%s", window_id, raw_table, exc_info=True)
                stats["copies_error"] += 1

    return stats
