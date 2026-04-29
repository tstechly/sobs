"""Web UI – Logs dashboard (`/logs`)."""

from __future__ import annotations

import re

from quart import Blueprint, render_template, request

import telemetry as _telemetry

logs_bp = Blueprint("logs", __name__)


@logs_bp.route("/logs")
async def view_logs():
    from app import (  # noqa: PLC0415
        _active_part_rows,
        _append_regex_expression_clauses,
        _append_time_window_filter,
        _compute_advanced_log_analysis,
        _compute_log_stats,
        _parse_limit,
        _parse_offset,
        _parse_sort,
        _parse_time_window_args,
        _parse_trace_filter_values,
        _prepare_re2_filter_patterns,
        _public_dashboard_query_error,
        _record_id_for_log,
        _time_window_conditions,
        _validate_user_sql_where,
        _where_clause,
        get_db,
        require_basic_auth,
    )

    @require_basic_auth
    @_telemetry.traced_view("sobs.dashboard.query", **{"dashboard.name": "logs", "route": "/logs"})
    async def _inner():
        from datetime import datetime, timezone  # noqa: PLC0415

        db = get_db()
        q = request.args.get("q", "").strip()
        selected_levels = [
            level_val.strip().upper() for level_val in request.args.getlist("level") if level_val.strip()
        ]
        selected_services = [svc.strip() for svc in request.args.getlist("service") if svc.strip()]
        trace_id = request.args.get("trace_id", "").strip()
        trace_ids, trace_id = _parse_trace_filter_values(trace_id, request.args.getlist("trace_ids"))
        trace_ids_csv = ",".join(trace_ids)
        trace_ids_count = len(trace_ids)
        selected_event_names = [evt.strip() for evt in request.args.getlist("event_name") if evt.strip()]
        event_name = ""
        from_ts, to_ts, time_error = _parse_time_window_args()
        sql_where = request.args.get("sql", "").strip()
        run_advanced_analysis = request.args.get("analyze", "").strip() == "1"
        limit = _parse_limit(200)
        offset = _parse_offset()
        sort_by, sort_col, sort_dir = _parse_sort(
            {"Timestamp": "Timestamp", "SeverityText": "SeverityText", "ServiceName": "ServiceName"},
            "Timestamp",
        )
        order_clause = f"ORDER BY {sort_col} {'ASC' if sort_dir == 'asc' else 'DESC'}"

        rows = []
        log_rows = []
        total = 0
        error_msg = ""
        level_stats: dict = {}
        service_stats: dict = {}
        advanced_analysis = None
        stats_generated_at_iso = ""
        stats_generated_at_display = ""
        stats_generated_age_s = 0
        where = ""
        params: list = []
        include_patterns: list[str] = []
        exclude_patterns: list[str] = []

        if time_error:
            error_msg = time_error

        if q:
            include_patterns, exclude_patterns, regex_error = _prepare_re2_filter_patterns(db, q)
            if regex_error:
                error_msg = regex_error

        if error_msg:
            pass
        elif sql_where:
            try:
                _validate_user_sql_where(sql_where)
                safe_sql = sql_where.replace(";", "")
                safe_sql = re.sub(r"\blevel\b", "SeverityText", safe_sql, flags=re.IGNORECASE)
                safe_sql = re.sub(r"\bservice\b", "ServiceName", safe_sql, flags=re.IGNORECASE)
                safe_sql = re.sub(r"\btrace_id\b", "TraceId", safe_sql, flags=re.IGNORECASE)
                safe_sql = re.sub(r"\bspan_id\b", "SpanId", safe_sql, flags=re.IGNORECASE)
                safe_sql = re.sub(r"\bts\b", "Timestamp", safe_sql, flags=re.IGNORECASE)
                safe_sql = re.sub(r"\bbody\b", "Body", safe_sql, flags=re.IGNORECASE)

                def _translate_has_tag(m: re.Match) -> str:
                    tag_key = m.group(1).replace("''", "'").replace("'", "''")
                    tag_val = m.group(2).replace("''", "'").replace("'", "''")
                    return (
                        "MD5(concat(ServiceName,'|',toString(Timestamp),'|',TraceId,'|',SpanId)) IN ("
                        "SELECT RecordId FROM sobs_record_tags FINAL "
                        f"WHERE TagKey='{tag_key}' AND TagValue='{tag_val}' "
                        "AND IsDeleted=0 AND RecordType='log')"
                    )

                safe_sql = re.sub(
                    r"has_tag\s*\(\s*'((?:[^']|'')+)'\s*,\s*'((?:[^']|'')*)'\s*\)",
                    _translate_has_tag,
                    safe_sql,
                    flags=re.IGNORECASE,
                )
                where = f"WHERE {safe_sql}"
                time_conditions, time_params = _time_window_conditions("Timestamp", from_ts, to_ts)
                if time_conditions:
                    where = f"{where} AND " + " AND ".join(time_conditions)
                    params.extend(time_params)
            except Exception as exc:
                error_msg = f"SQL error: {_public_dashboard_query_error(exc)}"
        else:
            conditions = []
            params = []
            if selected_levels:
                placeholders = ",".join(["?"] * len(selected_levels))
                conditions.append(f"SeverityText IN ({placeholders})")
                params.extend(selected_levels)
            if selected_services:
                placeholders = ",".join(["?"] * len(selected_services))
                conditions.append(f"ServiceName IN ({placeholders})")
                params.extend(selected_services)
            if selected_event_names:
                placeholders = ",".join(["?"] * len(selected_event_names))
                conditions.append(f"EventName IN ({placeholders})")
                params.extend(selected_event_names)
            if trace_ids:
                placeholders = ",".join(["?"] * len(trace_ids))
                conditions.append(f"lower(TraceId) IN ({placeholders})")
                params.extend(trace_ids)
            elif trace_id:
                conditions.append("lower(TraceId)=?")
                params.append(trace_id.lower())
            _append_time_window_filter(conditions, params, "Timestamp", from_ts, to_ts)
            where = _where_clause(conditions)

        if not error_msg:
            try:
                query_where = where
                query_params = list(params)
                if q:
                    regex_conditions: list[str] = []
                    _append_regex_expression_clauses(
                        conditions=regex_conditions,
                        params=query_params,
                        column="Body",
                        include_patterns=include_patterns,
                        exclude_patterns=exclude_patterns,
                    )
                    if regex_conditions:
                        regex_sql = " AND ".join(regex_conditions)
                        query_where = f"{query_where} AND {regex_sql}" if query_where else f"WHERE {regex_sql}"

                select_base = (
                    "SELECT Timestamp, SeverityText, ServiceName, Body, TraceId, SpanId "
                    f"FROM otel_logs {query_where} "
                )

                if not query_where:
                    total = _active_part_rows(db, "otel_logs")
                else:
                    total = db.execute(f"SELECT COUNT(*) FROM otel_logs {query_where}", query_params).fetchone()[0]
                rows = db.execute(
                    f"{select_base}{order_clause} LIMIT ? OFFSET ?",
                    query_params + [limit, offset],
                ).fetchall()
                level_stats, service_stats = _compute_log_stats(db, query_where, query_params)
                if run_advanced_analysis:
                    analysis_rows = db.execute(
                        f"SELECT SeverityText, ServiceName, Body, LogAttributes FROM otel_logs {query_where}",
                        query_params,
                    ).fetchall()
                    advanced_analysis = _compute_advanced_log_analysis(analysis_rows, level_stats, service_stats)

                generated_at = datetime.now(timezone.utc)
                snapshot_raw = db.execute(
                    f"SELECT max(Timestamp) FROM otel_logs {query_where}", query_params
                ).fetchone()[0]
                snapshot_at = generated_at
                if snapshot_raw is not None:
                    if isinstance(snapshot_raw, datetime):
                        snapshot_at = snapshot_raw
                    else:
                        parsed = datetime.fromisoformat(str(snapshot_raw).replace("Z", "+00:00"))
                        snapshot_at = parsed
                    if snapshot_at.tzinfo is None:
                        snapshot_at = snapshot_at.replace(tzinfo=timezone.utc)
                    else:
                        snapshot_at = snapshot_at.astimezone(timezone.utc)

                stats_generated_at_iso = snapshot_at.isoformat()
                stats_generated_at_display = snapshot_at.strftime("%Y-%m-%d %H:%M:%S UTC")
                stats_generated_age_s = max(0, int((generated_at - snapshot_at).total_seconds()))
            except Exception as exc:
                if sql_where:
                    error_msg = f"SQL error: {_public_dashboard_query_error(exc)}"
                else:
                    error_msg = f"Query error: {exc}"
                rows = []
                total = 0
                level_stats = {}
                service_stats = {}
                advanced_analysis = None

        row_record_ids = [
            _record_id_for_log(str(r["Timestamp"]), str(r["ServiceName"]), str(r["TraceId"]), str(r["SpanId"]))
            for r in rows
        ]
        tags_by_record_id: dict[str, list[dict]] = {}
        tag_stats_count: dict[tuple[str, str], int] = {}
        if row_record_ids:
            try:
                placeholders = ",".join(["?"] * len(row_record_ids))
                tag_rows_raw = db.execute(
                    f"SELECT RecordId, TagKey, TagValue, IsAuto "
                    f"FROM sobs_record_tags FINAL "
                    f"WHERE RecordType='log' AND RecordId IN ({placeholders}) AND IsDeleted=0 "
                    f"ORDER BY RecordId, TagKey",
                    row_record_ids,
                ).fetchall()
                for tr in tag_rows_raw:
                    rid = str(tr["RecordId"])
                    entry = {"key": str(tr["TagKey"]), "value": str(tr["TagValue"]), "is_auto": bool(tr["IsAuto"])}
                    tags_by_record_id.setdefault(rid, []).append(entry)
                    tag_key = str(tr["TagKey"])
                    tag_value = str(tr["TagValue"])
                    stats_key = (tag_key, tag_value)
                    tag_stats_count[stats_key] = tag_stats_count.get(stats_key, 0) + 1
            except Exception:
                pass

        tag_stats = [
            {"key": k, "value": v, "count": cnt}
            for (k, v), cnt in sorted(tag_stats_count.items(), key=lambda item: (-item[1], item[0][0], item[0][1]))
        ]

        for r in rows:
            body = r["Body"]
            rid = _record_id_for_log(str(r["Timestamp"]), str(r["ServiceName"]), str(r["TraceId"]), str(r["SpanId"]))
            log_rows.append(
                {
                    "ts": str(r["Timestamp"]),
                    "level": r["SeverityText"],
                    "service": r["ServiceName"],
                    "body": body,
                    "trace_id": r["TraceId"],
                    "span_id": r["SpanId"],
                    "record_id": rid,
                    "tags": tags_by_record_id.get(rid, []),
                }
            )

        services = [
            row[0]
            for row in db.execute(
                "SELECT DISTINCT ServiceName FROM otel_logs WHERE ServiceName!='' ORDER BY ServiceName"
            ).fetchall()
        ]
        levels = [
            row[0] for row in db.execute("SELECT DISTINCT SeverityText FROM otel_logs ORDER BY SeverityText").fetchall()
        ]
        event_names = [
            row[0]
            for row in db.execute(
                "SELECT DISTINCT EventName FROM otel_logs WHERE EventName!='' ORDER BY EventName"
            ).fetchall()
        ]

        return await render_template(
            "logs.html",
            logs=log_rows,
            total=total,
            limit=limit,
            offset=offset,
            q=q,
            level="",
            selected_levels=selected_levels,
            service="",
            selected_services=selected_services,
            trace_id=trace_id,
            trace_ids_csv=trace_ids_csv,
            trace_ids_count=trace_ids_count,
            sql_where=sql_where,
            from_ts=from_ts,
            to_ts=to_ts,
            services=services,
            levels=levels,
            event_names=event_names,
            event_name=event_name,
            selected_event_names=selected_event_names,
            error_msg=error_msg,
            sort_by=sort_by,
            sort_dir=sort_dir,
            run_advanced_analysis=run_advanced_analysis,
            level_stats=level_stats,
            service_stats=service_stats,
            tag_stats=tag_stats,
            advanced_analysis=advanced_analysis,
            stats_generated_at_iso=stats_generated_at_iso,
            stats_generated_at_display=stats_generated_at_display,
            stats_generated_age_s=stats_generated_age_s,
        )

    return await _inner()
