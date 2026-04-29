"""Web UI – RUM (Real User Monitoring) dashboard (`/rum`)."""

from __future__ import annotations

from typing import Any

from quart import Blueprint, render_template, request

rum_bp = Blueprint("rum", __name__)


@rum_bp.route("/rum")
async def view_rum():
    from quart import current_app  # noqa: PLC0415

    from app import (  # noqa: PLC0415
        _RUM_SESSION_KEY_SQL,
        RUM_SESSION_DETAIL_EVENT_CAP,
        _active_part_rows,
        _append_regex_expression_clauses,
        _append_time_window_filter,
        _build_rum_event_item,
        _parse_limit,
        _parse_offset,
        _parse_sort,
        _parse_time_window_args,
        _prepare_re2_filter_patterns,
        _where_clause,
        get_db,
        require_basic_auth,
    )

    @require_basic_auth
    async def _inner():
        db = get_db()
        view_mode = request.args.get("view", "sessions").strip().lower()
        if view_mode not in ("sessions", "events"):
            view_mode = "sessions"
        event_type = request.args.get("type", "").strip()
        error_source = request.args.get("error_source", "").strip()
        limit = _parse_limit(200)
        offset = _parse_offset()
        if view_mode == "sessions":
            sort_by, sort_col, sort_dir = _parse_sort(
                {
                    "severity": "severity_rank",
                    "last_seen": "last_ts",
                    "events": "event_count",
                    "errors": "error_count",
                },
                "severity",
            )
        else:
            sort_by, sort_col, sort_dir = _parse_sort(
                {"Timestamp": "Timestamp", "EventName": "EventName"},
                "Timestamp",
            )
        order_clause = f"ORDER BY {sort_col} {'ASC' if sort_dir == 'asc' else 'DESC'}"
        from_ts, to_ts, time_error = _parse_time_window_args()

        q = request.args.get("q", "").strip()
        q_error = ""
        include_patterns: list[str] = []
        exclude_patterns: list[str] = []
        if q:
            include_patterns, exclude_patterns, regex_error = _prepare_re2_filter_patterns(db, q)
            if regex_error:
                q_error = regex_error

        conditions = []
        params = []
        if event_type:
            conditions.append("EventName=?")
            params.append(event_type)
        if error_source:
            conditions.append("LogAttributes['errorSource']=?")
            params.append(error_source)
        _append_time_window_filter(conditions, params, "Timestamp", from_ts, to_ts)
        if q and not q_error:
            _append_regex_expression_clauses(
                conditions=conditions,
                params=params,
                column="Body",
                include_patterns=include_patterns,
                exclude_patterns=exclude_patterns,
            )
        where = _where_clause(conditions)
        total = 0
        events: list[dict[str, Any]] = []
        session_groups: list[dict[str, Any]] = []
        if view_mode == "sessions":
            total = db.execute(
                "SELECT count() FROM ("
                f"SELECT {_RUM_SESSION_KEY_SQL} AS session_key "
                f"FROM hyperdx_sessions {where} GROUP BY session_key)",
                params,
            ).fetchone()[0]
            summary_rows = db.execute(
                "SELECT "
                f"  {_RUM_SESSION_KEY_SQL} AS session_key,"
                "  max(Timestamp) AS last_ts,"
                "  count() AS event_count,"
                "  countIf(EventName IN ('error', 'unhandledrejection')) AS error_count,"
                "  countIf(EventName = 'web-vital' "
                "AND JSONExtractString(Body, 'rating') = 'poor') AS poor_vital_count,"
                "  countIf(EventName = 'web-vital' "
                "AND JSONExtractString(Body, 'rating') = 'needs-improvement') AS warn_vital_count,"
                "  countIf(TraceId != '') AS traced_count,"
                "  multiIf("
                "    countIf(EventName IN ('error', 'unhandledrejection')) > 0, 3,"
                "    countIf(EventName = 'web-vital' "
                "AND JSONExtractString(Body, 'rating') = 'poor') > 0, 2,"
                "    countIf(EventName = 'web-vital' "
                "AND JSONExtractString(Body, 'rating') = 'needs-improvement') > 0, 1,"
                "    0"
                "  ) AS severity_rank,"
                "  argMax(if(LogAttributes['url'] != '', LogAttributes['url'], "
                "LogAttributes['url.full']), Timestamp) AS last_url,"
                "  argMax(EventName, Timestamp) AS last_event_type"
                f" FROM hyperdx_sessions {where}"
                " GROUP BY session_key "
                f" ORDER BY {sort_col} {'ASC' if sort_dir == 'asc' else 'DESC'}, last_ts DESC LIMIT ? OFFSET ?",
                params + [limit, offset],
            ).fetchall()

            if summary_rows:
                session_keys = [str(row["session_key"]) for row in summary_rows]
                placeholders = ",".join(["?"] * len(session_keys))
                detail_conditions = list(conditions)
                detail_conditions.append(f"{_RUM_SESSION_KEY_SQL} IN ({placeholders})")
                detail_where = "WHERE " + " AND ".join(detail_conditions)
                detail_rows = db.execute(
                    "SELECT Timestamp, EventName, Body, LogAttributes, TraceId, SpanId "
                    "FROM ("
                    "SELECT Timestamp, EventName, Body, LogAttributes, TraceId, SpanId, "
                    f"{_RUM_SESSION_KEY_SQL} AS session_key, "
                    f"row_number() OVER (PARTITION BY {_RUM_SESSION_KEY_SQL} ORDER BY Timestamp DESC) AS row_rank "
                    f"FROM hyperdx_sessions {detail_where}"
                    ") "
                    "WHERE row_rank <= ? "
                    "ORDER BY session_key ASC, Timestamp DESC",
                    params + session_keys + [RUM_SESSION_DETAIL_EVENT_CAP],
                ).fetchall()
                events_by_session: dict[str, list[dict[str, Any]]] = {}
                for row in detail_rows:
                    item = _build_rum_event_item(row)
                    events_by_session.setdefault(str(item["session_key"]), []).append(item)

                for row in summary_rows:
                    session_key = str(row["session_key"])
                    session_events = events_by_session.get(session_key, [])
                    session_trace_id = next(
                        (str(ev.get("trace_id", "")) for ev in session_events if ev.get("trace_id")), ""
                    )
                    session_groups.append(
                        {
                            "session_key": session_key,
                            "session_id": session_key[:8],
                            "last_ts": str(row["last_ts"]),
                            "last_url": str(row["last_url"] or ""),
                            "last_event_type": str(row["last_event_type"] or ""),
                            "event_count": int(row["event_count"]),
                            "error_count": int(row["error_count"]),
                            "poor_vital_count": int(row["poor_vital_count"]),
                            "warn_vital_count": int(row["warn_vital_count"]),
                            "severity_rank": int(row["severity_rank"]),
                            "traced_count": int(row["traced_count"]),
                            "trace_id": session_trace_id,
                            "has_replay": any(bool(ev.get("has_replay")) for ev in session_events),
                            "has_artifact": any(bool(ev.get("has_artifact")) for ev in session_events),
                            "events": session_events,
                        }
                    )
        else:
            if not where:
                total = _active_part_rows(db, "hyperdx_sessions")
            else:
                total = db.execute(f"SELECT COUNT(*) FROM hyperdx_sessions {where}", params).fetchone()[0]
            rows = db.execute(
                f"SELECT Timestamp, EventName, Body, LogAttributes, TraceId, SpanId FROM hyperdx_sessions {where} "
                f"{order_clause} LIMIT ? OFFSET ?",
                params + [limit, offset],
            ).fetchall()
            events = [_build_rum_event_item(row) for row in rows]

        event_types = [
            row[0]
            for row in db.execute("SELECT DISTINCT EventName FROM hyperdx_sessions ORDER BY EventName").fetchall()
        ]
        error_sources = [
            row[0]
            for row in db.execute(
                "SELECT DISTINCT LogAttributes['errorSource'] FROM hyperdx_sessions "
                "WHERE LogAttributes['errorSource']!='' ORDER BY LogAttributes['errorSource']"
            ).fetchall()
        ]

        vitals_summary: dict[str, dict[str, object]] = {}
        vitals_sparklines: dict[str, list[dict[str, object]]] = {}
        vitals_hotspot: dict[str, list[dict[str, object]]] = {}
        try:
            anom_rows = db.execute(
                "SELECT SignalName,"
                " argMax(value, time) AS latest_value,"
                " argMax(anomaly_state, time) AS latest_state,"
                " toUInt64(argMax(SampleCount, time)) AS latest_count"
                " FROM v_derived_signals_anomaly"
                " WHERE SignalSource = 'rum_vitals'"
                "   AND time >= now() - INTERVAL 60 MINUTE"
                " GROUP BY SignalName"
            ).fetchall()
            for row in anom_rows:
                nm = str(row["SignalName"])
                val = float(row["latest_value"])
                state = str(row["latest_state"])
                cnt = int(row["latest_count"])
                vitals_summary[nm] = {
                    "p75": round(val, 3) if nm == "CLS" else round(val, 0),
                    "count": cnt,
                    "anomaly_state": state,
                }
            spark_rows = db.execute(
                "SELECT SignalName, MinuteBucket, Value, SampleCount"
                " FROM v_derived_signals_1m"
                " WHERE SignalSource = 'rum_vitals'"
                "   AND MinuteBucket >= now() - INTERVAL 60 MINUTE"
                " ORDER BY SignalName, MinuteBucket"
            ).fetchall()
            for row in spark_rows:
                nm = str(row["SignalName"])
                vitals_sparklines.setdefault(nm, []).append(
                    {
                        "t": str(row["MinuteBucket"]),
                        "v": round(float(row["Value"]), 3) if nm == "CLS" else round(float(row["Value"]), 1),
                    }
                )
            hotspot_rows = db.execute(
                "SELECT"
                "  JSONExtractString(Body, 'name') AS metric,"
                "  LogAttributes['url'] AS url,"
                "  count() AS total,"
                "  countIf(JSONExtractString(Body, 'rating') = 'poor') AS poor_count,"
                "  round(toFloat64(poor_count) / toFloat64(total), 3) AS poor_rate,"
                "  round(quantileExact(0.75)(JSONExtractFloat(Body, 'value')), 1) AS p75"
                " FROM hyperdx_sessions"
                " WHERE EventName = 'web-vital'"
                "   AND Timestamp >= now() - INTERVAL 24 HOUR"
                " GROUP BY metric, url"
                " HAVING total >= 3"
                " ORDER BY metric ASC, poor_rate DESC, total DESC"
                " LIMIT 60"
            ).fetchall()
            for row in hotspot_rows:
                metric = str(row["metric"])
                if not metric:
                    continue
                vitals_hotspot.setdefault(metric, []).append(
                    {
                        "url": str(row["url"]),
                        "total": int(row["total"]),
                        "poor_count": int(row["poor_count"]),
                        "poor_rate": float(row["poor_rate"]),
                        "p75": float(row["p75"]),
                    }
                )
            for metric in vitals_hotspot:
                vitals_hotspot[metric] = vitals_hotspot[metric][:5]
        except Exception:
            current_app.logger.exception("vitals derived-signal query failed")

        error_stats: dict[str, Any] = {
            "total": 0,
            "by_type": {},
            "trend": "stable",
            "recent": 0,
            "prior": 0,
            "sparkline": [],
            "top_messages": [],
            "top_urls": [],
        }
        try:
            trend_row = db.execute(
                "SELECT"
                " countIf(Timestamp >= now() - INTERVAL 30 MINUTE) AS recent,"
                " countIf("
                "   Timestamp >= now() - INTERVAL 60 MINUTE"
                "   AND Timestamp < now() - INTERVAL 30 MINUTE"
                " ) AS prior"
                " FROM hyperdx_sessions"
                " WHERE EventName IN ('error', 'unhandledrejection')"
                "   AND Timestamp >= now() - INTERVAL 60 MINUTE"
            ).fetchone()
            if trend_row:
                recent_cnt = int(trend_row["recent"])
                prior_cnt = int(trend_row["prior"])
                error_stats["recent"] = recent_cnt
                error_stats["prior"] = prior_cnt
                if prior_cnt == 0:
                    err_trend = "stable" if recent_cnt == 0 else "up"
                elif recent_cnt > prior_cnt * 1.25:
                    err_trend = "up"
                elif recent_cnt < prior_cnt * 0.75:
                    err_trend = "down"
                else:
                    err_trend = "stable"
                error_stats["trend"] = err_trend
            type_rows = db.execute(
                "SELECT EventName, count() AS cnt"
                " FROM hyperdx_sessions"
                " WHERE EventName IN ('error', 'unhandledrejection')"
                "   AND Timestamp >= now() - INTERVAL 24 HOUR"
                " GROUP BY EventName"
            ).fetchall()
            total_24h = 0
            by_type: dict[str, int] = {}
            for row in type_rows:
                cnt = int(row["cnt"])
                total_24h += cnt
                by_type[str(row["EventName"])] = cnt
            error_stats["total"] = total_24h
            error_stats["by_type"] = by_type
            spark_rows = db.execute(
                "SELECT mb, cnt"
                " FROM ("
                "   SELECT toStartOfMinute(Timestamp) AS mb, count() AS cnt"
                "   FROM hyperdx_sessions"
                "   WHERE EventName IN ('error', 'unhandledrejection')"
                "     AND Timestamp >= now() - INTERVAL 180 MINUTE"
                "   GROUP BY mb"
                " )"
                " ORDER BY mb"
                " WITH FILL"
                " FROM toStartOfMinute(now() - INTERVAL 180 MINUTE)"
                " TO toStartOfMinute(now())"
                " STEP toIntervalMinute(1)"
            ).fetchall()
            error_stats["sparkline"] = [{"t": str(row["mb"]), "v": int(row["cnt"])} for row in spark_rows]
            msg_rows = db.execute(
                "SELECT JSONExtractString(Body, 'message') AS message, count() AS cnt"
                " FROM hyperdx_sessions"
                " WHERE EventName IN ('error', 'unhandledrejection')"
                "   AND Timestamp >= now() - INTERVAL 24 HOUR"
                "   AND JSONExtractString(Body, 'message') != ''"
                " GROUP BY message ORDER BY cnt DESC LIMIT 8"
            ).fetchall()
            error_stats["top_messages"] = [
                {"message": str(row["message"]), "count": int(row["cnt"])} for row in msg_rows
            ]
            url_rows = db.execute(
                "SELECT LogAttributes['url'] AS url, count() AS cnt"
                " FROM hyperdx_sessions"
                " WHERE EventName IN ('error', 'unhandledrejection')"
                "   AND Timestamp >= now() - INTERVAL 24 HOUR"
                "   AND LogAttributes['url'] != ''"
                " GROUP BY url ORDER BY cnt DESC LIMIT 5"
            ).fetchall()
            error_stats["top_urls"] = [{"url": str(row["url"]), "count": int(row["cnt"])} for row in url_rows]
        except Exception:
            current_app.logger.exception("error stats query failed")

        return await render_template(
            "rum.html",
            events=events,
            session_groups=session_groups,
            total=total,
            limit=limit,
            offset=offset,
            view_mode=view_mode,
            event_type=event_type,
            event_types=event_types,
            error_source=error_source,
            error_sources=error_sources,
            vitals_summary=vitals_summary,
            vitals_sparklines=vitals_sparklines,
            vitals_hotspot=vitals_hotspot,
            error_stats=error_stats,
            sort_by=sort_by,
            sort_dir=sort_dir,
            from_ts=from_ts,
            to_ts=to_ts,
            q=q,
            error_msg=q_error or time_error,
        )

    return await _inner()
