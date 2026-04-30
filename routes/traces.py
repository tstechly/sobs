"""Web UI – Traces dashboard (`/traces`), raw span API, and incident view."""

from __future__ import annotations

import json
from datetime import datetime, timedelta, timezone
from typing import Any, cast

from quart import Blueprint, render_template, request

import telemetry as _telemetry

traces_bp = Blueprint("traces", __name__)


_RAW_SPAN_MAX_BYTES = 32 * 1024
_INCIDENT_MAX_RELATED_ERRORS = 50
_INCIDENT_MAX_RELATED_RUM_EVENTS = 20
_INCIDENT_WINDOW_DEFAULT_MINUTES = 30
_INCIDENT_WINDOW_MAX_MINUTES = 180


@traces_bp.route("/traces")
async def view_traces():
    from quart import current_app  # noqa: PLC0415

    from app import (  # noqa: PLC0415
        _TRACE_DETAIL_COLLAPSE_THRESHOLD,
        _TRACE_DETAIL_DEFAULT_LIMIT,
        _TRACE_DETAIL_HARD_CAP,
        _TRACE_DETAIL_MAX_LIMIT,
        ERROR_SOURCES_SQL,
        _active_part_rows,
        _append_regex_expression_clauses,
        _append_time_window_filter,
        _build_error_item,
        _build_span_tree,
        _build_trace_timeline_segments,
        _build_trace_window_overlay_segments,
        _coerce_positive_int,
        _compute_active_timeline_ms,
        _error_id_sql_expr,
        _fetch_trace_metric_context,
        _list_trace_overlapping_raw_windows,
        _load_work_item_links_for_ref_ids,
        _map_to_dict,
        _parse_limit,
        _parse_offset,
        _parse_sort,
        _parse_time_window_args,
        _prepare_re2_filter_patterns,
        _slice_span_tree_with_ancestors,
        _ts_str_to_epoch_ms,
        _where_clause,
        get_db,
        require_basic_auth,
    )

    @require_basic_auth
    @_telemetry.traced_view("sobs.dashboard.query", **{"dashboard.name": "traces", "route": "/traces"})
    async def _inner():
        db = get_db()
        error_id_sql = _error_id_sql_expr()
        selected_services = [svc.strip() for svc in request.args.getlist("service") if svc.strip()]
        service = selected_services[0] if selected_services else ""
        trace_id = request.args.get("trace_id", "").strip()
        from_ts, to_ts, time_error = _parse_time_window_args()
        limit = _parse_limit(100)
        offset = _parse_offset()
        trace_span_limit = _coerce_positive_int(
            request.args.get("trace_span_limit"),
            _TRACE_DETAIL_DEFAULT_LIMIT,
            1,
            _TRACE_DETAIL_MAX_LIMIT,
        )
        trace_span_offset = _coerce_positive_int(request.args.get("trace_span_offset"), 0, 0, _TRACE_DETAIL_HARD_CAP)
        sort_by, sort_col, sort_dir = _parse_sort(
            {
                "Timestamp": "Timestamp",
                "SpanName": "SpanName",
                "ServiceName": "ServiceName",
                "Duration": "Duration",
            },
            "Timestamp",
        )
        order_clause = f"ORDER BY {sort_col} {'ASC' if sort_dir == 'asc' else 'DESC'}"

        conditions = []
        params = []
        q = request.args.get("q", "").strip()
        q_error = ""
        include_patterns: list[str] = []
        exclude_patterns: list[str] = []
        if q:
            include_patterns, exclude_patterns, regex_error = _prepare_re2_filter_patterns(db, q)
            if regex_error:
                q_error = regex_error
        if selected_services:
            placeholders = ",".join(["?"] * len(selected_services))
            conditions.append(f"ServiceName IN ({placeholders})")
            params.extend(selected_services)
        if trace_id:
            conditions.append("TraceId=?")
            params.append(trace_id)
        _append_time_window_filter(conditions, params, "Timestamp", from_ts, to_ts)
        if q and not q_error:
            _append_regex_expression_clauses(
                conditions=conditions,
                params=params,
                column="SpanName",
                include_patterns=include_patterns,
                exclude_patterns=exclude_patterns,
            )
        where = _where_clause(conditions)
        if trace_id and not time_error:
            total = 0
            rows = []
        else:
            if not where:
                total = _active_part_rows(db, "otel_traces")
            else:
                total = db.execute(f"SELECT COUNT(*) FROM otel_traces {where}", params).fetchone()[0]
            rows = db.execute(
                (
                    "SELECT Timestamp, TraceId, SpanId, ParentSpanId, "
                    "SpanName, ServiceName, Duration, StatusCode, SpanAttributes "
                    f"FROM otel_traces {where} {order_clause} LIMIT ? OFFSET ?"
                ),
                params + [limit, offset],
            ).fetchall()

        spans = []
        for r in rows:
            attrs = _map_to_dict(r["SpanAttributes"])
            spans.append(
                {
                    "ts": str(r["Timestamp"]),
                    "trace_id": r["TraceId"],
                    "span_id": r["SpanId"],
                    "parent_span_id": r["ParentSpanId"],
                    "name": r["SpanName"],
                    "service": r["ServiceName"],
                    "duration_ms": round(float(r["Duration"]) / 1_000_000, 2),
                    "status": r["StatusCode"],
                    "http_method": attrs.get("http.method", attrs.get("http.request.method", "")),
                    "http_url": attrs.get("http.url", attrs.get("url.full", "")),
                    "http_status": attrs.get("http.status_code", attrs.get("http.response.status_code", "")),
                }
            )

        services = [
            row[0]
            for row in db.execute(
                "SELECT DISTINCT ServiceName FROM otel_traces WHERE ServiceName!='' ORDER BY ServiceName"
            ).fetchall()
        ]

        trace_detail: dict | None = None
        if trace_id and not time_error:
            trace_total_spans = int(
                db.execute("SELECT COUNT(*) FROM otel_traces WHERE TraceId=?", [trace_id]).fetchone()[0] or 0
            )
            detail_fetch_limit = min(trace_total_spans, _TRACE_DETAIL_HARD_CAP)
            detail_rows = db.execute(
                "SELECT Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, "
                "Duration, StatusCode, SpanAttributes "
                "FROM otel_traces WHERE TraceId=? ORDER BY Timestamp ASC, SpanId ASC LIMIT ?",
                [trace_id, detail_fetch_limit],
            ).fetchall()
            if detail_rows:
                all_trace_spans = []
                for r in detail_rows:
                    attrs = _map_to_dict(r["SpanAttributes"])
                    ts_str = str(r["Timestamp"])
                    start_ms = _ts_str_to_epoch_ms(ts_str)
                    dur_ms = round(float(r["Duration"]) / 1_000_000, 2)
                    all_trace_spans.append(
                        {
                            "ts": ts_str,
                            "trace_id": str(r["TraceId"]),
                            "span_id": str(r["SpanId"]),
                            "parent_span_id": str(r["ParentSpanId"]),
                            "name": str(r["SpanName"]),
                            "service": str(r["ServiceName"]),
                            "start_ms": start_ms,
                            "duration_ms": dur_ms,
                            "status": str(r["StatusCode"]),
                            "http_method": str(attrs.get("http.method", attrs.get("http.request.method", ""))),
                            "http_url": str(attrs.get("http.url", attrs.get("url.full", ""))),
                            "http_status": str(
                                attrs.get("http.status_code", attrs.get("http.response.status_code", ""))
                            ),
                            "namespace": str(attrs.get("k8s.namespace.name", attrs.get("namespace", ""))),
                            "pod": str(attrs.get("k8s.pod.name", attrs.get("pod", ""))),
                            "node": str(attrs.get("k8s.node.name", attrs.get("node", ""))),
                            "deployment": str(attrs.get("k8s.deployment.name", attrs.get("deployment", ""))),
                        }
                    )

                trace_start_ms = min(s["start_ms"] for s in all_trace_spans)
                trace_end_ms = max(s["start_ms"] + s["duration_ms"] for s in all_trace_spans)
                trace_total_ms = max(trace_end_ms - trace_start_ms, 1.0)
                trace_active_ms = _compute_active_timeline_ms(all_trace_spans)
                trace_coverage_pct = min(100.0, max(0.0, (trace_active_ms / trace_total_ms) * 100.0))
                trace_span_sum_ms = sum(max(0.0, float(s.get("duration_ms", 0.0) or 0.0)) for s in all_trace_spans)
                for span in all_trace_spans:
                    span["offset_pct"] = round((span["start_ms"] - trace_start_ms) / trace_total_ms * 100, 2)
                    span["width_pct"] = round(max(0.5, span["duration_ms"] / trace_total_ms * 100), 2)

                _TRACE_ERROR_LIMIT = 50
                trace_errors: list[dict] = []
                errors_truncated = False
                trace_activity_ts_ms: list[float] = []
                try:
                    err_rows = db.execute(
                        "SELECT Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes, ErrorId, "
                        "(ErrorId IN (SELECT ErrorId FROM sobs_error_resolutions GROUP BY ErrorId)) AS IsResolved "
                        "FROM ("
                        "SELECT Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes, "
                        f"{error_id_sql} AS ErrorId "
                        f"FROM ({ERROR_SOURCES_SQL}) WHERE TraceId=? LIMIT ?"
                        ")",
                        [trace_id, _TRACE_ERROR_LIMIT + 1],
                    ).fetchall()
                    if len(err_rows) > _TRACE_ERROR_LIMIT:
                        errors_truncated = True
                        err_rows = err_rows[:_TRACE_ERROR_LIMIT]
                    for row in err_rows:
                        item = _build_error_item(dict(row))
                        item["id"] = str(row["ErrorId"] or item["id"])
                        item["resolved"] = bool(row["IsResolved"])
                        trace_errors.append(item)
                        ts_raw = str(item.get("ts") or "")
                        if ts_raw:
                            trace_activity_ts_ms.append(_ts_str_to_epoch_ms(ts_raw))
                except Exception as exc:
                    current_app.logger.warning("view_traces: failed to fetch errors for trace %s: %s", trace_id, exc)

                error_span_ids = {e["span_id"] for e in trace_errors if e.get("span_id")}

                log_counts: dict[str, int] = {}
                try:
                    log_rows = db.execute(
                        "SELECT SpanId, count() AS cnt FROM otel_logs "
                        "WHERE TraceId=? AND SpanId!='' GROUP BY SpanId",
                        [trace_id],
                    ).fetchall()
                    for r in log_rows:
                        log_counts[str(r["SpanId"])] = int(r["cnt"])

                    log_ts_rows = db.execute(
                        "SELECT Timestamp FROM otel_logs WHERE TraceId=? LIMIT 2000",
                        [trace_id],
                    ).fetchall()
                    for r in log_ts_rows:
                        trace_activity_ts_ms.append(_ts_str_to_epoch_ms(str(r["Timestamp"])))
                except Exception as exc:
                    current_app.logger.warning(
                        "view_traces: failed to fetch log counts for trace %s: %s", trace_id, exc
                    )

                timeline_segments = _build_trace_timeline_segments(all_trace_spans, trace_activity_ts_ms)
                has_potential_gap = any(
                    seg.get("kind") == "gap" and bool(seg.get("potential")) for seg in timeline_segments
                )

                trace_anomaly_state: str | None = None
                try:
                    svc = all_trace_spans[0]["service"] if all_trace_spans else ""
                    if svc:
                        anomaly_row = db.execute(
                            "SELECT anomaly_state FROM v_derived_signals_anomaly "
                            "WHERE ServiceName=? AND SignalSource='traces' "
                            "AND time >= now() - INTERVAL 48 HOUR "
                            "ORDER BY time DESC LIMIT 1",
                            [svc],
                        ).fetchone()
                        if anomaly_row:
                            trace_anomaly_state = str(anomaly_row["anomaly_state"])
                except Exception as exc:
                    current_app.logger.warning(
                        "view_traces: failed to fetch anomaly state for trace %s: %s", trace_id, exc
                    )

                trace_windows: list[dict[str, object]] = []
                try:
                    trace_services = sorted(
                        {str(s.get("service") or "").strip() for s in all_trace_spans if s.get("service")}
                    )
                    trace_start_ts = datetime.fromtimestamp(trace_start_ms / 1000.0, tz=timezone.utc).isoformat()
                    trace_end_ts = datetime.fromtimestamp(trace_end_ms / 1000.0, tz=timezone.utc).isoformat()
                    trace_windows = _list_trace_overlapping_raw_windows(
                        db,
                        service_names=trace_services,
                        start_ts=trace_start_ts,
                        end_ts=trace_end_ts,
                    )
                except Exception as exc:
                    current_app.logger.warning(
                        "view_traces: failed to fetch raw windows for trace %s: %s", trace_id, exc
                    )

                trace_metrics_context: dict[str, object] = {
                    "source_mode": "none",
                    "total_points": 0,
                    "series": [],
                    "match_mode": "none",
                    "match_label": "no match",
                    "match_dimensions": [],
                }
                try:
                    trace_services = sorted(
                        {str(s.get("service") or "").strip() for s in all_trace_spans if s.get("service")}
                    )
                    trace_namespaces = sorted(
                        {str(s.get("namespace") or "").strip() for s in all_trace_spans if s.get("namespace")}
                    )
                    trace_pods = sorted({str(s.get("pod") or "").strip() for s in all_trace_spans if s.get("pod")})
                    trace_nodes = sorted({str(s.get("node") or "").strip() for s in all_trace_spans if s.get("node")})
                    trace_deployments = sorted(
                        {str(s.get("deployment") or "").strip() for s in all_trace_spans if s.get("deployment")}
                    )
                    _METRIC_PAD_MS = 5 * 60 * 1000
                    metric_ctx_start_ts = datetime.fromtimestamp(
                        (trace_start_ms - _METRIC_PAD_MS) / 1000.0, tz=timezone.utc
                    ).isoformat()
                    metric_ctx_end_ts = datetime.fromtimestamp(
                        (trace_end_ms + _METRIC_PAD_MS) / 1000.0, tz=timezone.utc
                    ).isoformat()
                    trace_metrics_context = _fetch_trace_metric_context(
                        db,
                        service_names=trace_services,
                        start_ts=metric_ctx_start_ts,
                        end_ts=metric_ctx_end_ts,
                        window_ids=[str(w.get("id") or "") for w in trace_windows if str(w.get("id") or "")],
                        namespace_values=trace_namespaces,
                        pod_values=trace_pods,
                        node_values=trace_nodes,
                        deployment_values=trace_deployments,
                    )
                except Exception as exc:
                    current_app.logger.warning(
                        "view_traces: failed to fetch metrics context for trace %s: %s", trace_id, exc
                    )

                trace_window_segments = _build_trace_window_overlay_segments(all_trace_spans, trace_windows)

                full_span_tree = _build_span_tree(all_trace_spans)
                capped_total_spans = len(full_span_tree)
                if trace_span_offset >= capped_total_spans and capped_total_spans > 0:
                    trace_span_offset = max(0, ((capped_total_spans - 1) // trace_span_limit) * trace_span_limit)
                trace_page_spans, trace_page_end, trace_context_rows = _slice_span_tree_with_ancestors(
                    full_span_tree,
                    trace_span_offset,
                    trace_span_limit,
                )
                detail_prev_offset = max(0, trace_span_offset - trace_span_limit)
                detail_next_offset = trace_span_offset + trace_span_limit
                detail_hard_capped = trace_total_spans > _TRACE_DETAIL_HARD_CAP
                default_collapsed = capped_total_spans > _TRACE_DETAIL_COLLAPSE_THRESHOLD

                total = trace_total_spans

                trace_detail = {
                    "span_tree": trace_page_spans,
                    "trace_start_ts": str(all_trace_spans[0]["ts"]),
                    "trace_end_ts": str(all_trace_spans[-1]["ts"]),
                    "trace_start_ms": round(trace_start_ms),
                    "trace_end_ms": round(trace_end_ms),
                    "errors": trace_errors,
                    "errors_truncated": errors_truncated,
                    "error_span_ids": error_span_ids,
                    "log_counts": log_counts,
                    "anomaly_state": trace_anomaly_state,
                    "total_ms": round(trace_total_ms, 2),
                    "active_ms": round(trace_active_ms, 2),
                    "coverage_pct": round(trace_coverage_pct, 2),
                    "span_sum_ms": round(trace_span_sum_ms, 2),
                    "timeline_segments": timeline_segments,
                    "has_potential_gap": has_potential_gap,
                    "raw_windows": trace_windows,
                    "raw_window_segments": trace_window_segments,
                    "metrics_context": trace_metrics_context,
                    "total_spans": trace_total_spans,
                    "capped_total_spans": capped_total_spans,
                    "hard_cap": _TRACE_DETAIL_HARD_CAP,
                    "hard_capped": detail_hard_capped,
                    "default_collapsed": default_collapsed,
                    "page_limit": trace_span_limit,
                    "page_offset": trace_span_offset,
                    "page_end": trace_page_end,
                    "context_rows": trace_context_rows,
                    "prev_offset": detail_prev_offset,
                    "next_offset": detail_next_offset,
                    "has_prev_page": trace_span_offset > 0,
                    "has_next_page": detail_next_offset < capped_total_spans,
                }

        trace_work_item_links: dict[str, dict] = {}
        if trace_detail:
            trace_errors_local = trace_detail.get("errors") or []
            ref_ids = list(
                {e["id"] for e in trace_errors_local if e.get("id")}
                | {e["trace_id"] for e in trace_errors_local if e.get("trace_id")}
            )
            if trace_id:
                ref_ids.append(trace_id)
            trace_work_item_links = _load_work_item_links_for_ref_ids(db, ref_ids)

        return await render_template(
            "traces.html",
            spans=spans,
            total=total,
            limit=limit,
            offset=offset,
            service=service,
            selected_services=selected_services,
            trace_id=trace_id,
            from_ts=from_ts,
            to_ts=to_ts,
            error_msg=q_error or time_error,
            q=q,
            services=services,
            sort_by=sort_by,
            sort_dir=sort_dir,
            trace_detail=trace_detail,
            work_item_links=trace_work_item_links,
        )

    return await _inner()


@traces_bp.route("/api/traces/span/<span_id>", methods=["GET"])
async def api_raw_span(span_id: str):
    from quart import jsonify  # noqa: PLC0415

    from app import (  # noqa: PLC0415
        _map_to_dict,
        _mask_value_for_output,
        get_db,
        masked_jsonify,
        require_basic_auth,
    )

    @require_basic_auth
    async def _inner():
        sid = span_id.strip()
        if not sid:
            return jsonify({"error": "span_id is required"}), 400

        trace_id = (request.args.get("trace_id") or "").strip()

        db = get_db()
        base_sql = (
            "SELECT Timestamp, TraceId, SpanId, ParentSpanId, TraceState, "
            "SpanName, SpanKind, ServiceName, ResourceAttributes, "
            "ScopeName, ScopeVersion, SpanAttributes, Duration, "
            "StatusCode, StatusMessage "
            "FROM otel_traces WHERE SpanId=?"
        )
        params: list[str] = [sid]
        if trace_id:
            base_sql += " AND TraceId=?"
            params.append(trace_id)
        base_sql += " ORDER BY Timestamp DESC LIMIT 1"
        row = db.execute(base_sql, params).fetchone()

        if row is None:
            return jsonify({"error": "span not found"}), 404

        span_attrs = dict(_map_to_dict(row["SpanAttributes"]))
        resource_attrs = dict(_map_to_dict(row["ResourceAttributes"]))

        payload: dict[str, object] = {
            "timestamp": str(row["Timestamp"]),
            "trace_id": str(row["TraceId"]),
            "span_id": str(row["SpanId"]),
            "parent_span_id": str(row["ParentSpanId"]),
            "trace_state": str(row["TraceState"]),
            "name": str(row["SpanName"]),
            "kind": str(row["SpanKind"]),
            "service": str(row["ServiceName"]),
            "scope_name": str(row["ScopeName"]),
            "scope_version": str(row["ScopeVersion"]),
            "duration_ns": int(row["Duration"]),
            "duration_ms": round(int(row["Duration"]) / 1_000_000, 3),
            "status_code": str(row["StatusCode"]),
            "status_message": str(row["StatusMessage"]),
            "attributes": span_attrs,
            "resource_attributes": resource_attrs,
        }

        masked_payload = cast(dict[str, object], _mask_value_for_output(payload))
        raw = json.dumps(masked_payload, ensure_ascii=False, indent=2)
        truncated = False
        if len(raw.encode()) > _RAW_SPAN_MAX_BYTES:
            truncated = True
            _ATTR_TRUNCATE = 512
            payload["attributes"] = {
                k: (v[:_ATTR_TRUNCATE] + "…" if isinstance(v, str) and len(v) > _ATTR_TRUNCATE else v)
                for k, v in span_attrs.items()
            }
            payload["resource_attributes"] = {
                k: (v[:_ATTR_TRUNCATE] + "…" if isinstance(v, str) and len(v) > _ATTR_TRUNCATE else v)
                for k, v in resource_attrs.items()
            }
            masked_payload = cast(dict[str, object], _mask_value_for_output(payload))
            raw = json.dumps(masked_payload, ensure_ascii=False, indent=2)

        return masked_jsonify({"span": masked_payload, "raw": raw, "truncated": truncated})

    return await _inner()


@traces_bp.route("/incident")
async def view_incident():
    from quart import current_app  # noqa: PLC0415

    from app import (  # noqa: PLC0415
        _RUM_SESSION_KEY_SQL,
        ERROR_SOURCES_SQL,
        _build_error_item,
        _build_rum_event_item,
        _fetch_trace_metric_context,
        _get_resolved_error_ids,
        _list_trace_overlapping_raw_windows,
        _load_work_item_links_for_ref_ids,
        _normalize_ch_timestamp,
        _parse_time_window_args,
        _time_window_conditions,
        _ts_str_to_epoch_ms,
        get_db,
        require_basic_auth,
    )

    @require_basic_auth
    async def _inner():
        db = get_db()
        trace_id = request.args.get("trace_id", "").strip()
        error_id = request.args.get("error_id", "").strip()
        rum_session = request.args.get("rum_session", "").strip()
        rum_ts = request.args.get("rum_ts", "").strip()
        from_ts, to_ts, time_error = _parse_time_window_args()

        try:
            _wm_raw = request.args.get("window_minutes", "").strip()
            _wm_int = int(_wm_raw) if _wm_raw else _INCIDENT_WINDOW_DEFAULT_MINUTES
            window_minutes = max(1, min(_INCIDENT_WINDOW_MAX_MINUTES, _wm_int))
        except (TypeError, ValueError):
            window_minutes = _INCIDENT_WINDOW_DEFAULT_MINUTES

        if not trace_id and not error_id and not rum_session:
            return await render_template(
                "incident.html",
                trace_id="",
                error_id="",
                rum_session="",
                rum_ts="",
                primary_error=None,
                primary_trace=None,
                primary_rum=None,
                service="",
                from_ts="",
                to_ts="",
                window_minutes=window_minutes,
                related_errors=[],
                related_log_count=0,
                related_span_count=0,
                related_rum_count=0,
                related_rum_sessions=0,
                related_rum_error_count=0,
                related_rum_events=[],
                raw_windows=[],
                metrics_context={
                    "source_mode": "none",
                    "total_points": 0,
                    "series": [],
                    "match_mode": "none",
                    "match_label": "no match",
                    "match_dimensions": [],
                },
                anomaly_state=None,
                work_item_links={},
                time_error="",
                error_msg="No incident reference provided. Specify trace_id, error_id, or rum_session.",
            )

        primary_error: dict | None = None
        if error_id:
            try:
                scan_limit = 5000
                err_rows = db.execute(
                    f"SELECT * FROM ({ERROR_SOURCES_SQL}) ORDER BY Timestamp DESC LIMIT ?",
                    [scan_limit],
                ).fetchall()
                resolved_ids = _get_resolved_error_ids(db)
                for row in err_rows:
                    candidate = _build_error_item(dict(row))
                    if candidate["id"] == error_id:
                        candidate["resolved"] = candidate["id"] in resolved_ids
                        primary_error = candidate
                        break
            except Exception as exc:
                current_app.logger.warning("view_incident: failed to look up error_id %s: %s", error_id, exc)

        primary_trace: dict | None = None
        if trace_id:
            try:
                span_rows = db.execute(
                    "SELECT Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, "
                    "Duration, StatusCode, SpanAttributes "
                    "FROM otel_traces WHERE TraceId=? ORDER BY Timestamp ASC",
                    [trace_id],
                ).fetchall()
                if span_rows:
                    services = sorted({str(r["ServiceName"]) for r in span_rows if r["ServiceName"]})
                    root = span_rows[0]
                    start_ms = _ts_str_to_epoch_ms(str(root["Timestamp"]))
                    end_ms = max(
                        _ts_str_to_epoch_ms(str(r["Timestamp"])) + round(float(r["Duration"]) / 1_000_000, 2)
                        for r in span_rows
                    )
                    primary_trace = {
                        "trace_id": trace_id,
                        "services": services,
                        "service": services[0] if services else "",
                        "span_count": len(span_rows),
                        "start_ts": str(root["Timestamp"]),
                        "start_ms": round(start_ms),
                        "end_ms": round(end_ms),
                        "total_ms": round(end_ms - start_ms, 2),
                        "root_name": str(root["SpanName"]),
                        "status": str(root["StatusCode"]),
                    }
            except Exception as exc:
                current_app.logger.warning("view_incident: failed to look up trace_id %s: %s", trace_id, exc)

        primary_rum: dict | None = None
        if rum_session:
            try:
                rum_where_parts = [f"{_RUM_SESSION_KEY_SQL}=?"]
                rum_where_params: list[str] = [rum_session]
                if rum_ts:
                    rum_where_parts.append("Timestamp <= parseDateTime64BestEffort(?, 9)")
                    rum_where_params.append(rum_ts)
                rum_where_sql = "WHERE " + " AND ".join(rum_where_parts)
                rum_row = db.execute(
                    "SELECT Timestamp, EventName, Body, LogAttributes, TraceId, SpanId, ServiceName "
                    f"FROM hyperdx_sessions {rum_where_sql} "
                    "ORDER BY Timestamp DESC LIMIT 1",
                    rum_where_params,
                ).fetchone()
                if rum_row:
                    primary_rum = _build_rum_event_item(rum_row)
            except Exception as exc:
                current_app.logger.warning("view_incident: failed to look up rum_session %s: %s", rum_session, exc)

        service = ""
        event_ts = ""
        if primary_error:
            service = primary_error.get("service", "")
            event_ts = primary_error.get("ts", "")
        elif primary_trace:
            service = primary_trace.get("service", "")
            event_ts = primary_trace.get("start_ts", "")
        elif primary_rum:
            service = str(primary_rum.get("service", "") or "")
            event_ts = str(primary_rum.get("ts", "") or "")

        if event_ts and not (from_ts and to_ts) and not time_error:
            try:
                dt = datetime.fromisoformat(event_ts.replace(" ", "T").rstrip("Z") + "+00:00")
                half = timedelta(minutes=window_minutes // 2)
                from_ts = _normalize_ch_timestamp(dt - half)
                to_ts = _normalize_ch_timestamp(dt + half)
            except (TypeError, ValueError):
                pass

        related_errors: list[dict] = []
        related_errors_truncated = False
        try:
            where_parts: list[str] = []
            where_params: list[str] = []
            if trace_id:
                where_parts.append("TraceId=?")
                where_params.append(trace_id)
            elif service:
                where_parts.append("ServiceName=?")
                where_params.append(service)
            tc, tp = _time_window_conditions("Timestamp", from_ts, to_ts)
            where_parts.extend(tc)
            where_params.extend(tp)
            where_sql = ("WHERE " + " AND ".join(where_parts)) if where_parts else ""
            err_rows = db.execute(
                f"SELECT * FROM ({ERROR_SOURCES_SQL}) {where_sql} " f"ORDER BY Timestamp DESC LIMIT ?",
                where_params + [_INCIDENT_MAX_RELATED_ERRORS + 1],
            ).fetchall()
            resolved_ids = _get_resolved_error_ids(db)
            primary_error_id = primary_error["id"] if primary_error else ""
            for row in err_rows[:_INCIDENT_MAX_RELATED_ERRORS]:
                item = _build_error_item(dict(row))
                item["resolved"] = item["id"] in resolved_ids
                if item["id"] != primary_error_id:
                    related_errors.append(item)
            related_errors_truncated = len(err_rows) > _INCIDENT_MAX_RELATED_ERRORS
        except Exception as exc:
            current_app.logger.warning("view_incident: failed to fetch related errors: %s", exc)

        related_log_count = 0
        try:
            log_where_parts: list[str] = []
            log_where_params: list[str] = []
            if trace_id:
                log_where_parts.append("TraceId=?")
                log_where_params.append(trace_id)
            elif service:
                log_where_parts.append("ServiceName=?")
                log_where_params.append(service)
            tc, tp = _time_window_conditions("Timestamp", from_ts, to_ts)
            log_where_parts.extend(tc)
            log_where_params.extend(tp)
            log_where_sql = ("WHERE " + " AND ".join(log_where_parts)) if log_where_parts else ""
            row_cnt = db.execute(
                f"SELECT count() AS cnt FROM otel_logs {log_where_sql}",
                log_where_params,
            ).fetchone()
            related_log_count = int(row_cnt["cnt"]) if row_cnt else 0
        except Exception as exc:
            current_app.logger.warning("view_incident: failed to count related logs: %s", exc)

        related_span_count = 0
        try:
            if service:
                span_where_parts: list[str] = ["ServiceName=?"]
                span_where_params: list[str] = [service]
                tc, tp = _time_window_conditions("Timestamp", from_ts, to_ts)
                span_where_parts.extend(tc)
                span_where_params.extend(tp)
                span_where_sql = "WHERE " + " AND ".join(span_where_parts)
                row_cnt = db.execute(
                    f"SELECT count() AS cnt FROM otel_traces {span_where_sql}",
                    span_where_params,
                ).fetchone()
                related_span_count = int(row_cnt["cnt"]) if row_cnt else 0
        except Exception as exc:
            current_app.logger.warning("view_incident: failed to count related spans: %s", exc)

        related_rum_count = 0
        related_rum_sessions = 0
        related_rum_error_count = 0
        related_rum_events: list[dict[str, Any]] = []
        try:
            rum_where_parts_i: list[str] = []
            rum_where_params_i: list[str] = []
            if trace_id:
                rum_where_parts_i.append("TraceId=?")
                rum_where_params_i.append(trace_id)
            elif service:
                rum_where_parts_i.append("(LogAttributes['service.name']=? OR LogAttributes['service']=?)")
                rum_where_params_i.extend([service, service])
            tc, tp = _time_window_conditions("Timestamp", from_ts, to_ts)
            rum_where_parts_i.extend(tc)
            rum_where_params_i.extend(tp)
            rum_where_sql = ("WHERE " + " AND ".join(rum_where_parts_i)) if rum_where_parts_i else ""

            rum_summary_row = db.execute(
                "SELECT "
                "count() AS ev_count, "
                f"uniq({_RUM_SESSION_KEY_SQL}) AS session_count, "
                "countIf(EventName IN ('error', 'unhandledrejection')) AS err_count "
                f"FROM hyperdx_sessions {rum_where_sql}",
                rum_where_params_i,
            ).fetchone()
            if rum_summary_row:
                related_rum_count = int(rum_summary_row["ev_count"])
                related_rum_sessions = int(rum_summary_row["session_count"])
                related_rum_error_count = int(rum_summary_row["err_count"])

            rum_rows = db.execute(
                "SELECT Timestamp, EventName, Body, LogAttributes, TraceId, SpanId, ServiceName "
                f"FROM hyperdx_sessions {rum_where_sql} "
                "ORDER BY Timestamp DESC LIMIT ?",
                rum_where_params_i + [_INCIDENT_MAX_RELATED_RUM_EVENTS],
            ).fetchall()
            related_rum_events = [_build_rum_event_item(row) for row in rum_rows]
        except Exception as exc:
            current_app.logger.warning("view_incident: failed to fetch related RUM evidence: %s", exc)

        raw_windows: list[dict[str, object]] = []
        metrics_context: dict[str, object] = {
            "source_mode": "none",
            "total_points": 0,
            "series": [],
            "match_mode": "none",
            "match_label": "no match",
            "match_dimensions": [],
        }
        try:
            if from_ts and to_ts:
                service_names = [service] if service else []
                raw_windows = _list_trace_overlapping_raw_windows(
                    db,
                    service_names=service_names,
                    start_ts=from_ts,
                    end_ts=to_ts,
                    limit=25,
                )
                metrics_context = _fetch_trace_metric_context(
                    db,
                    service_names=service_names,
                    start_ts=from_ts,
                    end_ts=to_ts,
                    window_ids=[str(w.get("id") or "") for w in raw_windows if str(w.get("id") or "")],
                    namespace_values=[],
                    pod_values=[],
                    node_values=[],
                    deployment_values=[],
                )
        except Exception as exc:
            current_app.logger.warning("view_incident: failed to fetch window/metrics context: %s", exc)

        anomaly_state: str | None = None
        try:
            if service:
                anomaly_row = db.execute(
                    "SELECT anomaly_state FROM v_derived_signals_anomaly "
                    "WHERE ServiceName=? AND SignalSource='traces' "
                    "AND time >= now() - INTERVAL 48 HOUR "
                    "ORDER BY time DESC LIMIT 1",
                    [service],
                ).fetchone()
                if anomaly_row:
                    anomaly_state = str(anomaly_row["anomaly_state"])
        except Exception as exc:
            current_app.logger.warning("view_incident: failed to fetch anomaly state for service %s: %s", service, exc)

        ref_ids: list[str] = []
        if primary_error:
            ref_ids.append(primary_error["id"])
        elif error_id:
            ref_ids.append(error_id)
        if trace_id:
            ref_ids.append(trace_id)
        if rum_session:
            ref_ids.append(rum_session)
        work_item_links = _load_work_item_links_for_ref_ids(db, ref_ids)

        existing_work_item: dict | None = None
        for ref in ref_ids:
            wi = work_item_links.get(ref)
            if wi and wi.get("issue_url"):
                existing_work_item = wi
                break

        return await render_template(
            "incident.html",
            trace_id=trace_id,
            error_id=error_id,
            rum_session=rum_session,
            rum_ts=rum_ts,
            primary_error=primary_error,
            primary_trace=primary_trace,
            primary_rum=primary_rum,
            service=service,
            from_ts=from_ts,
            to_ts=to_ts,
            window_minutes=window_minutes,
            related_errors=related_errors,
            related_errors_truncated=related_errors_truncated,
            related_log_count=related_log_count,
            related_span_count=related_span_count,
            related_rum_count=related_rum_count,
            related_rum_sessions=related_rum_sessions,
            related_rum_error_count=related_rum_error_count,
            related_rum_events=related_rum_events,
            raw_windows=raw_windows,
            metrics_context=metrics_context,
            anomaly_state=anomaly_state,
            work_item_links=work_item_links,
            existing_work_item=existing_work_item,
            time_error=time_error,
            error_msg=time_error or "",
        )

    return await _inner()
