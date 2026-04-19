# SOBS Python Routes Inventory

**Generated**: April 19, 2026  
**Total routes**: 183 @app.route() decorators  
**Auth model**: @require_basic_auth (HTTP Basic or external bearer token) on all routes  
**Database**: ChDB (SQLite-compatible schema)

---

## Table of Contents

1. [Core Data Pages (HTML)](#core-data-pages)
2. [API Data Endpoints (JSON)](#api-data-endpoints)
3. [Configuration Pages & APIs](#configuration-pages)
4. [Dashboards & Reports](#dashboards--reports)
5. [Infrastructure & Correlation](#infrastructure--correlation)
6. [Query & Exploration](#query--exploration)
7. [Ingestion Endpoints (V1)](#ingestion-endpoints-v1)
8. [Static & Health](#static--health)
9. [Dependency Graph](#dependency-graph)

---

## Core Data Pages

These are main HTML pages served to users (GET only, returns rendered templates).

### `/` (Summary/Home)
- **HTTP Methods**: GET
- **Route**: `/`
- **Handler**: `summary()`
- **Criticality**: CRITICAL - landing page
- **Database Queries**:
  - `SELECT Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes FROM ({ERROR_SOURCES_SQL})` – last 48h unresolved errors (limit 5)
  - `SELECT count() FROM ({ERROR_SOURCES_SQL})` – total errors
  - `SELECT count() FROM ({ERROR_SOURCES_SQL}) WHERE {unresolved_condition}` – unresolved errors count
  - `SELECT COUNT(*) FROM otel_logs` – log row count
  - `SELECT COUNT(*) FROM otel_traces WHERE {_AI_SPAN_CONDITION}` – AI span count
  - `SELECT COUNT(*) FROM hyperdx_sessions` – RUM session count
  - `SELECT DISTINCT ServiceName FROM otel_logs/otel_traces/hyperdx_sessions` – service list
- **Context Variables**:
  - `stats`: {logs, spans, rum, ai, errors_total, errors, services}
  - `recent_errors`: [{id, ts, service, err_type, message}, ...]
  - `mobile_breakpoint_max`: 575.98px
- **Cached**: 60s TTL on stats (summary stats cache)
- **Related endpoints**: None (landing page)

---

### `/logs` (Logs Page)
- **HTTP Methods**: GET
- **Route**: `/logs`
- **Handler**: `view_logs()`
- **Criticality**: CRITICAL - primary observability page
- **Query Parameters**:
  - `q` – text search/regex filter on Body
  - `level` – SeverityText filter (array)
  - `service` – ServiceName filter (array)
  - `trace_id` / `trace_ids` – TraceId filter
  - `event_name` – EventName filter (array)
  - `from_ts`, `to_ts` – time window
  - `sql` – raw WHERE clause (validated)
  - `analyze` – advanced analysis flag
  - `limit` (default 200), `offset`
  - `sort_by` – Timestamp | SeverityText | ServiceName
- **Database Tables Queried**:
  - `otel_logs` – primary table
  - `sobs_record_tags` – if has_tag() used
- **Main Query**:
  ```sql
  SELECT Timestamp, SeverityText, ServiceName, Body, TraceId, SpanId 
  FROM otel_logs {WHERE clause with filters and regex}
  ORDER BY {sort_col} LIMIT {limit} OFFSET {offset}
  ```
- **Context Variables**:
  - `rows`: [{ts, level, service, body, trace_id, span_id}, ...]
  - `total`: record count
  - `level_stats`: {level: count, ...}
  - `service_stats`: {service: count, ...}
  - `advanced_analysis`: {patterns, top_keys, anomalies} (if requested)
  - `error_msg`: validation errors
  - Selected filters (levels, services, event_names, trace_ids)
  - Pagination (limit, offset)
- **Related endpoints**:
  - `/api/logs/field-hints` – autocomplete suggestions
  - `/api/logs/validate-filter` – filter validation
  - `/api/logs/validate-regex` – regex validation

---

### `/metrics` (Metrics/Signals Page)
- **HTTP Methods**: GET
- **Route**: `/metrics`
- **Handler**: `view_metrics()`
- **Criticality**: CRITICAL
- **Query Parameters**:
  - `service` – ServiceName filter (array)
  - `signal` – SignalName filter (array)
  - `source` – SignalSource filter (array)
  - `attr_fp` – AttrFingerprint filter
  - `q` – text search
  - `from_ts`, `to_ts` – time window
  - `hours` – recent hours if no time window (default 24, max 168)
  - `limit` (default 100), `offset`
  - `sort_by` – last_time | service | source | signal | last_value | last_anomaly_score | last_anomaly_state | last_sample_count | point_count
- **Database Tables Queried**:
  - `v_derived_signals_anomaly` – view with anomaly scoring
  - `sobs_anomaly_rules` – anomaly rule details
- **Main Query**:
  ```sql
  SELECT
    ServiceName, SignalSource, SignalName, AttrFingerprint,
    max(time), argMax(value, time), argMax(anomaly_score, time),
    argMax(anomaly_state, time), argMax(SampleCount, time), count()
  FROM v_derived_signals_anomaly {WHERE clause}
  GROUP BY ServiceName, SignalSource, SignalName, AttrFingerprint
  ORDER BY {sort_col}
  ```
- **Context Variables**:
  - `rows`: [{service, source, signal, attr_fp, last_time, last_value, last_anomaly_score, last_anomaly_state, last_sample_count, point_count, rule_name}, ...]
  - `total`: metric count
  - `services`, `signals`, `sources`: dimension lists for filters
  - Pagination info
- **Related endpoints**:
  - `/metrics/rules` – create/edit anomaly detection rules
  - `/metrics/anomaly` – anomaly detail page
  - `/api/metrics/anomaly` – anomaly data API
  - `/api/metrics/validate-regex`

---

### `/errors` (Errors Page)
- **HTTP Methods**: GET
- **Route**: `/errors`
- **Handler**: `view_errors()`
- **Criticality**: CRITICAL
- **Query Parameters**:
  - `service` – ServiceName filter (array)
  - `group_by` – group | message | fingerprint | signature (aggregation mode)
  - `grouped` – "1" for grouped mode
  - `from_ts`, `to_ts` – time window
  - `resolved` – "0" (unresolved), "1" (resolved), or other (all)
  - `q` – text search
  - `limit` (default 100), `offset`
  - `sort_by` – Timestamp | ServiceName | count | last_seen (in grouped mode)
- **Database Tables Queried**:
  - `ERROR_SOURCES_SQL` – union of otel_logs, otel_traces with error attributes
  - `sobs_error_resolutions` – resolved error tracking
- **Main Queries**:
  - **Grouped mode** (best-effort deduplication):
    ```sql
    SELECT Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes,
      substring(...ServiceName...), substring(...exception.type...), substring(...exception.message...)
    FROM ({ERROR_SOURCES_SQL})
    WHERE {grouped_where_sql}
    ORDER BY Timestamp DESC LIMIT ?
    ```
  - **Aggregation**:
    ```sql
    SELECT GroupService, GroupType, GroupMessage, count(), min(Timestamp), max(Timestamp), ...
    FROM ({grouped_probe_sql})
    GROUP BY GroupService, GroupType, GroupMessage
    ORDER BY {sort_col}
    ```
- **Context Variables**:
  - `rows`: [{ts, service, err_type, message, count, first_seen, last_seen, stack_preview, resolved}, ...] (grouped) OR [{ts, service, err_type, message, resolved}, ...] (flat)
  - `total`: error count
  - `grouped_mode`: bool
  - `resolved_flag`: "all" | "unresolved" | "resolved"
- **Related endpoints**:
  - `/errors/<error_id>/resolve` – POST to resolve error group
  - `/api/errors/validate-regex`
  - `/api/tags/<record_type>/<record_id>` – get/set tags on errors

---

### `/traces` (Traces Page)
- **HTTP Methods**: GET
- **Route**: `/traces`
- **Handler**: `view_traces()`
- **Criticality**: CRITICAL
- **Query Parameters**:
  - `service` – ServiceName filter (array)
  - `trace_id` – exact TraceId filter
  - `from_ts`, `to_ts` – time window
  - `q` – text search on SpanName
  - `limit` (default 100), `offset`
  - `trace_span_limit` – spans per trace (default 500, max 5000)
  - `trace_span_offset` – pagination within trace
  - `sort_by` – Timestamp | SpanName | ServiceName | Duration
- **Database Tables Queried**:
  - `otel_traces` – main spans table
- **Main Query**:
  ```sql
  SELECT Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration, StatusCode, SpanAttributes
  FROM otel_traces {WHERE clause}
  ORDER BY {sort_col}
  LIMIT {limit} OFFSET {offset}
  ```
- **Context Variables**:
  - `spans`: [{ts, trace_id, span_id, parent_span_id, name, service, duration_ms, status, http_method, http_url, http_status}, ...]
  - `total`: span count
  - `services`: distinct service list
  - Pagination info
- **Related endpoints**:
  - `/api/traces/span/<span_id>` – span detail API
  - `/api/traces/validate-regex`
  - `/api/tags/<record_type>/<record_id>` – get/set tags on spans

---

### `/metrics/rules` (Anomaly Rules Page)
- **HTTP Methods**: GET
- **Route**: `/metrics/rules`
- **Handler**: `view_metrics_rules()`
- **Criticality**: OPTIONAL - settings page
- **Database Tables Queried**:
  - `sobs_anomaly_rules` – anomaly detection rules
- **Context Variables**:
  - `rules`: [{id, source, signal, service, attr_fp, min_value, max_value, window_size, violation_count}, ...]
  - `services`, `signals`, `sources`: dimension lists
- **Related endpoints**:
  - `/metrics/rules` (POST) – create rule
  - `/metrics/rules/auto` (POST) – auto-generate rules
  - `/metrics/rules/dashboard/auto` (POST) – dashboard auto-gen
  - `/metrics/rules/<rule_id>/delete` (POST) – delete rule

---

### `/metrics/anomaly` (Anomaly Detail Page)
- **HTTP Methods**: GET
- **Route**: `/metrics/anomaly`
- **Handler**: `view_metrics_anomaly()`
- **Criticality**: OPTIONAL
- **Query Parameters**: source, signal, service, attr_fp, from_ts, to_ts
- **Database Queries**:
  - `SELECT time, value, anomaly_score, anomaly_state FROM v_derived_signals_anomaly WHERE {filters}`
- **Context Variables**:
  - Time series data with anomaly scores
  - Related rules

---

### `/errors/<error_id>/resolve` (Error Resolution - POST)
- **HTTP Methods**: POST
- **Route**: `/errors/<error_id>/resolve`
- **Handler**: `resolve_error(error_id)`
- **Criticality**: OPTIONAL - error lifecycle
- **Form Data**: resolution_mode (ignored), comment (optional)
- **Database Updates**:
  - Insert into `sobs_error_resolutions` (soft-delete pattern with Version timestamp)
- **Related endpoints**: `/errors` GET

---

### `/rum` (RUM/Sessions Page)
- **HTTP Methods**: GET
- **Route**: `/rum`
- **Handler**: `view_rum()`
- **Criticality**: CRITICAL
- **Query Parameters**:
  - `view` – "sessions" or "events"
  - `type` – EventName filter (e.g., "error", "unhandledrejection", "web-vital")
  - `error_source` – LogAttributes['errorSource'] filter
  - `from_ts`, `to_ts` – time window
  - `q` – text search on Body
  - `limit` (default 200), `offset`
  - `sort_by` – severity | last_seen | events | errors (sessions) OR Timestamp | EventName (events)
- **Database Tables Queried**:
  - `hyperdx_sessions` – RUM events (sessions table)
  - `sobs_record_tags` – tags on RUM events
- **Main Queries** (sessions mode):
  ```sql
  SELECT {session_key}, max(Timestamp), count(), countIf(EventName IN (...)), ...
  FROM hyperdx_sessions {WHERE clause}
  GROUP BY session_key
  ORDER BY {sort_col}
  ```
- **Context Variables**:
  - `session_groups`: [{session_key, session_id, last_ts, last_url, last_event_type, event_count, error_count, poor_vital_count, warn_vital_count, traced_count, severity}, ...]
  - `events`: [{ts, type, body, trace_id, session_key, url}, ...] (detail events per session)
  - `total`: session/event count
  - Pagination info
- **Related endpoints**:
  - `/api/rum/validate-regex`
  - `/api/tags/<record_type>/<record_id>` – get/set RUM tags

---

### `/incident` (Incident Correlation Page)
- **HTTP Methods**: GET
- **Route**: `/incident`
- **Handler**: `view_incident()`
- **Criticality**: OPTIONAL - correlation/drill-down page
- **Query Parameters**:
  - `trace_id` – trace reference
  - `error_id` – error reference
  - `rum_session` – RUM session reference
  - `rum_ts` – RUM timestamp
  - `from_ts`, `to_ts` – time window
  - `window_minutes` – correlation window (default 60, max 1440)
- **Database Queries**:
  - Resolve primary error from `ERROR_SOURCES_SQL`
  - Resolve primary trace from `otel_traces`
  - Resolve primary RUM from `hyperdx_sessions`
  - Find related errors in time window
  - Find related logs/spans in time window
  - Find raw_windows (time bucketed data) for metrics context
- **Context Variables**:
  - `primary_error`, `primary_trace`, `primary_rum`: detail objects
  - `related_errors`, `related_log_count`, `related_span_count`, `related_rum_count`, `related_rum_sessions`, `related_rum_error_count`, `related_rum_events`
  - `raw_windows`: time bucketed data for visualization
  - `metrics_context`: {source_mode, total_points, series, match_mode, match_label, match_dimensions}
  - `work_item_links`: GitHub issue links if configured
- **Related endpoints**: None direct (terminal drill-down page)

---

### `/web-traffic` (Web Traffic Page)
- **HTTP Methods**: GET
- **Route**: `/web-traffic`
- **Handler**: `view_web_traffic()`
- **Criticality**: OPTIONAL
- **Database Queries**:
  - Geo distribution: `SELECT LogAttributes['geo'], count() FROM hyperdx_sessions GROUP BY LogAttributes['geo']`
  - Browser: `SELECT LogAttributes['browser'], count() FROM hyperdx_sessions GROUP BY LogAttributes['browser']`
  - OS: `SELECT LogAttributes['os'], count() FROM hyperdx_sessions GROUP BY LogAttributes['os']`
  - Timezone: `SELECT LogAttributes['timezone'], count() FROM hyperdx_sessions GROUP BY LogAttributes['timezone']`
  - Language: `SELECT LogAttributes['language'], count() FROM hyperdx_sessions GROUP BY LogAttributes['language']`
  - Device: `SELECT LogAttributes['device'], count() FROM hyperdx_sessions GROUP BY LogAttributes['device']`
- **Context Variables**:
  - Dimension data (geo, browsers, os, timezones, languages, devices)
- **Related endpoints**:
  - `/api/web-traffic/geo` (GET)
  - `/api/web-traffic/browsers` (GET)
  - `/api/web-traffic/os` (GET)
  - `/api/web-traffic/timezones` (GET)
  - `/api/web-traffic/languages` (GET)
  - `/api/web-traffic/devices` (GET)

---

### `/work-items` (GitHub Work Items Page)
- **HTTP Methods**: GET
- **Route**: `/work-items`
- **Handler**: `view_work_items()`
- **Criticality**: OPTIONAL
- **Database Tables Queried**:
  - `sobs_gh_work_items` – GitHub issues/PRs linked to observability
  - `sobs_agent_runs` – CI agent runs (if applicable)
- **Context Variables**:
  - `work_items`: [{id, repo, issue_number, title, status, created_at, updated_at, copilot_assigned, ...}, ...]
  - Agent rule count, run count
- **Related endpoints**:
  - `/api/work-items` (GET) – data API
  - `/api/issues/raise` (POST) – create GitHub issue

---

### `/ai` (AI Observability Page)
- **HTTP Methods**: GET
- **Route**: `/ai`
- **Handler**: `view_ai()`
- **Criticality**: OPTIONAL - AI tracking page
- **Query Parameters**:
  - `service`, `model`, `operation`, `span_name`, `row_type` – filters
  - `sql` – raw WHERE clause for advanced filtering
  - `from_ts`, `to_ts` – time window
  - `view` – "flat" or "trace" (aggregation mode)
  - `limit`, `offset`, `sort_by`
- **Database Tables Queried**:
  - `otel_traces` where SpanAttributes['gen_ai.*'] is set
  - `sobs_ai_memories` – AI memory/context
- **Context Variables**:
  - `ai_items`: [{ts, service, model, operation, span_name, input_tokens, output_tokens, total_tokens, latency_ms, cost, trace_id, ...}, ...]
  - `total`: span count
  - `services`, `models`, `operations`, `span_names`: dimension lists
- **Related endpoints**:
  - `/api/ai/field-hints` (GET)
  - `/api/ai/validate-filter` (POST)
  - `/api/ai/span-attributes` (GET)
  - `/api/ai/conversation` (GET)
  - `/api/ai/export` (GET)

---

### `/enrichment/cve` (CVE/Vulnerability Page)
- **HTTP Methods**: GET
- **Route**: `/enrichment/cve`
- **Handler**: `view_cve_findings()`
- **Criticality**: OPTIONAL
- **Database Queries**:
  - `SELECT * FROM sobs_cve_findings WHERE IsDeleted=0`
- **Context Variables**:
  - CVE findings list
- **Related endpoints**:
  - `/api/enrichment/cve/findings` (GET)
  - `/api/enrichment/cve/findings/<osv_id>/disposition` (POST)
  - `/api/enrichment/cve/scan` (POST)

---

### `/dashboards` (Custom Dashboards List)
- **HTTP Methods**: GET
- **Route**: `/dashboards`
- **Handler**: `list_dashboards()`
- **Criticality**: OPTIONAL
- **Database Queries**:
  - `SELECT * FROM sobs_dashboards WHERE IsDeleted=0`
- **Context Variables**:
  - `dashboards`: [{id, name, description, created_at, updated_at}, ...]
- **Related endpoints**:
  - `/dashboards/new` (GET)
  - `/dashboards` (POST) – create
  - `/dashboards/<dashboard_id>` (GET) – view
  - `/dashboards/<dashboard_id>/delete` (POST)

---

### `/dashboards/<dashboard_id>` (Custom Dashboard View)
- **HTTP Methods**: GET
- **Route**: `/dashboards/<dashboard_id>`
- **Handler**: `view_custom_dashboard(dashboard_id)`
- **Criticality**: OPTIONAL
- **Database Queries**:
  - `SELECT * FROM sobs_dashboards WHERE Id=?`
  - `SELECT * FROM sobs_chart_configs WHERE DashboardId=? AND IsDeleted=0`
- **Context Variables**:
  - `dashboard`: {id, name, description}
  - `charts`: [{id, title, chart_type, query, options_json, position}, ...]
  - `templates`: [{id, name, description, icon, query_shape, default_spec}, ...]
- **Related endpoints**:
  - `/dashboards/<dashboard_id>/charts` (POST) – add chart
  - `/dashboards/<dashboard_id>/charts/<chart_id>/edit` (POST)
  - `/dashboards/<dashboard_id>/charts/<chart_id>/clone` (POST)
  - `/dashboards/<dashboard_id>/charts/<chart_id>/delete` (POST)
  - `/dashboards/<dashboard_id>/delete` (POST)

---

### `/reports` (Reports List)
- **HTTP Methods**: GET
- **Route**: `/reports`
- **Handler**: `list_reports()`
- **Criticality**: OPTIONAL
- **Database Queries**:
  - `SELECT * FROM sobs_reports WHERE IsDeleted=0`
- **Context Variables**:
  - `reports`: [{id, name, description, page_type, filters}, ...]
- **Related endpoints**:
  - `/api/reports` (GET/POST)
  - `/reports/<report_id>/delete` (POST)

---

### `/settings` (Settings Hub)
- **HTTP Methods**: GET
- **Route**: `/settings`
- **Handler**: `view_settings()`
- **Criticality**: OPTIONAL - config hub page
- **Database Queries**:
  - `SELECT * FROM sobs_tag_rules WHERE IsDeleted=0`
  - `SELECT * FROM sobs_anomaly_rules WHERE IsDeleted=0`
  - `SELECT * FROM sobs_agent_rules WHERE IsDeleted=0` (if GitHub enabled)
  - `SELECT * FROM sobs_app_settings` – key-value config
  - `SELECT * FROM sobs_notifications_channels WHERE IsDeleted=0`
  - `SELECT * FROM sobs_notifications_rules WHERE IsDeleted=0`
- **Context Variables**:
  - Rule counts (tags, anomalies, agents)
  - AI configured flag
  - Notification channel/rule counts
  - Masking settings summary
  - Kubernetes enabled flag
  - Backup enabled flag
- **Related endpoints**:
  - `/settings/masking` (GET/POST)
  - `/settings/tags` (GET/POST)
  - `/settings/notifications` (GET/POST)
  - `/settings/ai` (GET/POST)
  - `/settings/enrichment` (GET/POST)
  - `/settings/repositories` (GET/POST)
  - `/settings/agents` (GET/POST)
  - `/settings/kubernetes` (GET/POST)
  - `/settings/data-management` (GET/POST)

---

### `/settings/masking` (Masking Rules Settings)
- **HTTP Methods**: GET
- **Route**: `/settings/masking`
- **Handler**: `view_masking_settings()`
- **Criticality**: OPTIONAL
- **Database Queries**:
  - `SELECT * FROM sobs_app_settings WHERE Key LIKE 'masking.%'`
- **Context Variables**:
  - `custom_keys`, `default_keys`: sensitive key lists
  - `custom_patterns`, `default_patterns`: regex patterns
  - `effective_key_count`, `effective_pattern_count`
  - `output_masking_enabled`, `sql_output_masking_enabled`: flags
- **Related endpoints**:
  - `/settings/masking/keys` (POST) – add key
  - `/settings/masking/keys/delete` (POST)
  - `/settings/masking/patterns` (POST) – add pattern
  - `/settings/masking/patterns/delete` (POST)
  - `/settings/masking/output` (POST) – toggle output masking
  - `/settings/masking/sql-output` (POST) – toggle SQL masking
  - `/api/settings/masking/preview` (POST)
  - `/api/settings/masking/rules` (GET)

---

### `/settings/tags` (Tag Rules Settings)
- **HTTP Methods**: GET
- **Route**: `/settings/tags`
- **Handler**: `view_tag_rules()`
- **Criticality**: OPTIONAL
- **Database Queries**:
  - `SELECT * FROM sobs_tag_rules WHERE IsDeleted=0`
- **Context Variables**:
  - `rules`: [{id, name, condition, tags, enabled}, ...]
  - Available tag keys (from schema)
- **Related endpoints**:
  - `/settings/tags` (POST) – create/edit rule
  - `/settings/tags/auto` (POST) – auto-generate
  - `/settings/tags/<rule_id>/delete` (POST)
  - `/api/settings/tags/condition-suggestions` (GET)
  - `/api/tags/<record_type>/<record_id>` (GET/POST/DELETE)

---

### `/settings/ai` (AI Configuration)
- **HTTP Methods**: GET
- **Route**: `/settings/ai`
- **Handler**: `view_ai_settings()`
- **Criticality**: OPTIONAL - AI feature config
- **Database Queries**:
  - `SELECT * FROM sobs_app_settings WHERE Key LIKE 'ai.%'`
- **Context Variables**:
  - AI endpoint URL, model, guard model, API key (redacted)
  - LLM pricing (if available)
  - GitHub token status (expiry, repo-scoped tokens)
- **Related endpoints**:
  - `/settings/ai` (POST) – save AI config
  - `/api/ai/helper/*` – AI helper APIs

---

### `/settings/enrichment` (Data Enrichment Config)
- **HTTP Methods**: GET
- **Route**: `/settings/enrichment`
- **Handler**: `view_enrichment_settings()`
- **Criticality**: OPTIONAL
- **Database Queries**: Enrichment service configs (GitHub repo health, library detection, etc.)
- **Related endpoints**:
  - `/settings/enrichment` (POST)
  - `/api/enrichment/libraries` (GET)
  - `/api/enrichment/github/repo-health` (GET)

---

### `/settings/repositories` (Repository/App Config)
- **HTTP Methods**: GET
- **Route**: `/settings/repositories`
- **Handler**: `view_repository_settings()`
- **Criticality**: OPTIONAL
- **Database Queries**:
  - `SELECT * FROM sobs_app_settings WHERE Key LIKE 'app.%' OR Key LIKE 'ci_push.%'`
  - GitHub repos from sobs_gh_work_items
- **Context Variables**:
  - `repos`: [{app_id, name, github_repo, ci_push_enabled, ci_api_key_status}, ...]
  - Realtime mode flags
- **Related endpoints**:
  - `/settings/repositories` (POST)
  - `/settings/repositories/<app_id>/realtime-mode` (POST)
  - `/settings/repositories/<app_id>/ci-ingest-key/rotate` (POST)
  - `/settings/repositories/<app_id>/ci-ingest-key/revoke` (POST)
  - `/settings/repositories/<app_id>/releases` (POST)
  - `/settings/repositories/<app_id>/delete` (POST)
  - `/settings/repositories/github-token/validate` (POST)

---

### `/settings/agents` (Automated Agent Rules)
- **HTTP Methods**: GET
- **Route**: `/settings/agents`
- **Handler**: `view_agent_settings()`
- **Criticality**: OPTIONAL - GitHub Copilot integration config
- **Database Queries**:
  - `SELECT * FROM sobs_agent_rules WHERE IsDeleted=0`
  - `SELECT * FROM sobs_gh_work_items` (recent work items)
  - `SELECT COUNT(*) FROM sobs_gh_work_items WHERE CreatedAt > now() - INTERVAL 1 HOUR`
- **Context Variables**:
  - `rules`: [{id, name, trigger_condition, github_repo, enabled, last_run}, ...]
  - `recent_runs`: agent execution history
  - `stats`: {issues_last_hour, assignments_last_hour, active_assignments}
- **Related endpoints**:
  - `/settings/agents` (POST)
  - `/settings/agents/<rule_id>/delete` (POST)
  - `/api/agent/runs` (GET/POST)
  - `/api/agent/runs/<run_id>/dismiss` (POST)

---

### `/settings/notifications` (Notification Channels & Rules)
- **HTTP Methods**: GET
- **Route**: `/settings/notifications`
- **Handler**: `view_notification_settings()`
- **Criticality**: OPTIONAL
- **Database Queries**:
  - `SELECT * FROM sobs_notifications_channels WHERE IsDeleted=0`
  - `SELECT * FROM sobs_notifications_rules WHERE IsDeleted=0`
- **Context Variables**:
  - `channels`: [{id, name, type, config, enabled}, ...]
  - `rules`: [{id, name, condition, channel_ids, enabled}, ...]
- **Related endpoints**:
  - `/settings/notifications/channels` (POST)
  - `/settings/notifications/channels/<channel_id>/delete` (POST)
  - `/settings/notifications/channels/<channel_id>/toggle` (POST)
  - `/settings/notifications/rules` (POST)
  - `/settings/notifications/rules/<rule_id>/toggle` (POST)
  - `/settings/notifications/rules/<rule_id>/delete` (POST)
  - `/api/notifications/channels/<channel_id>/test` (POST)
  - `/api/notifications/rules/auto-generate` (POST)
  - `/api/notifications/check` (POST)

---

### `/settings/kubernetes` (Kubernetes Config)
- **HTTP Methods**: GET
- **Route**: `/settings/kubernetes`
- **Handler**: `view_kubernetes_settings()`
- **Criticality**: OPTIONAL
- **Database Queries**: Kubernetes endpoint config from sobs_app_settings
- **Related endpoints**:
  - `/settings/kubernetes` (POST)
  - `/kubernetes` (GET)
  - `/api/kubernetes/status` (GET)

---

### `/settings/data-management` (Data Retention & Backup)
- **HTTP Methods**: GET
- **Route**: `/settings/data-management`
- **Handler**: `view_data_management_settings()`
- **Criticality**: OPTIONAL
- **Database Queries**:
  - Data retention policies from sobs_app_settings
  - Backup list from backup directory
- **Related endpoints**:
  - `/settings/data-management` (POST)
  - `/api/data-management/backup/list` (GET)
  - `/api/data-management/backup/run` (POST)
  - `/api/data-management/restore` (POST)

---

### `/query` (NLQ Query Builder)
- **HTTP Methods**: GET
- **Route**: `/query`
- **Handler**: `view_query()`
- **Criticality**: OPTIONAL - AI-powered query builder
- **Template**: `query.html` (empty page, client-side loads from API)
- **Related endpoints**:
  - `/api/query/ask` (POST) – NLQ to SQL
  - `/api/query/run` (POST) – execute SQL
  - `/api/query/refine-chart` (POST)
  - `/api/query/schema` (GET)
  - `/api/query/add-to-dashboard` (POST)

---

### `/table-explorer` (Table Schema Explorer)
- **HTTP Methods**: GET
- **Route**: `/table-explorer`
- **Handler**: `view_table_explorer()`
- **Criticality**: OPTIONAL
- **Related endpoints**:
  - `/api/table-explorer/tables` (GET)
  - `/api/table-explorer/table/<name>` (GET)

---

### `/kubernetes` (Kubernetes Dashboard)
- **HTTP Methods**: GET
- **Route**: `/kubernetes`
- **Handler**: `view_kubernetes()`
- **Criticality**: OPTIONAL
- **Related endpoints**:
  - `/api/kubernetes/status` (GET)

---

### `/tail` (Live Log Stream)
- **HTTP Methods**: GET
- **Route**: `/tail`
- **Handler**: `view_tail()`
- **Criticality**: OPTIONAL
- **Database Queries**: Real-time tail of otel_logs, otel_traces, hyperdx_sessions
- **Notes**: SSE (Server-Sent Events) streaming

---

## Configuration Pages

### `/settings/masking/keys/delete`, `/settings/masking/patterns/delete`
### `/settings/tags/auto`, `/settings/tags/<rule_id>/delete`
### `/settings/notifications/channels/<channel_id>/toggle`
### `/metrics/rules/auto`, `/metrics/rules/<rule_id>/delete`

All of these POST endpoints modify configuration tables and redirect back to parent page.

---

## API Data Endpoints

Organized by functional category. All require HTTP Basic Auth or bearer token.

### Logs API

#### `/api/logs/field-hints` (GET)
- **Query Params**: `prefix` (autocomplete), `limit`
- **Database**: Queries field names from otel_logs schema and tag keys
- **Returns**: `[{label, value, kind}, ...]`

#### `/api/logs/validate-filter` (POST)
- **Body**: `{condition, sample_size}`
- **Database**: Validates filter syntax against otel_logs
- **Returns**: `{valid, error, matched_count}`

#### `/api/logs/validate-regex` (POST)
- **Body**: `{pattern}`
- **Database**: Validates RE2 regex
- **Returns**: `{valid, error}`

---

### Traces API

#### `/api/traces/span/<span_id>` (GET)
- **Database**: `SELECT * FROM otel_traces WHERE SpanId=?`
- **Returns**: Single span detail with full attributes

#### `/api/traces/validate-regex` (POST)
- **Body**: `{pattern}`
- **Returns**: `{valid, error}`

---

### Errors API

#### `/api/errors/validate-regex` (POST)
- **Body**: `{pattern}`
- **Returns**: `{valid, error}`

---

### Metrics API

#### `/api/metrics/validate-regex` (POST)
- **Returns**: `{valid, error}`

#### `/api/metrics/anomaly` (GET)
- **Query Params**: `service`, `signal`, `source`, `attr_fp`, `from_ts`, `to_ts`
- **Database**: Queries `v_derived_signals_anomaly` with time range
- **Returns**: Time series data with anomaly scores: `[{time, value, anomaly_score, anomaly_state}, ...]`

---

### RUM API

#### `/api/rum/validate-regex` (POST)
- **Returns**: `{valid, error}`

---

### AI API

#### `/api/ai/field-hints` (GET)
- **Query Params**: `prefix`, `limit`
- **Database**: otel_traces schema + gen_ai.* attribute keys
- **Returns**: `[{label, value, kind}, ...]`

#### `/api/ai/validate-filter` (POST)
- **Body**: `{condition}`
- **Database**: Validates filter syntax
- **Returns**: `{valid, error, matched_count}`

#### `/api/ai/span-attributes` (GET)
- **Database**: Queries distinct span attributes for gen_ai spans
- **Returns**: `{attributes: [{name, values: [...]}, ...]}`

#### `/api/ai/conversation` (GET/POST)
- **Database**: `sobs_ai_helper_chats` (chat history)
- **Returns**: Chat turns with context

#### `/api/ai/export` (GET)
- **Database**: Exports chat/conversation data
- **Returns**: JSON export

#### `/api/ai/helper` (POST) - Main AI Chat
- **Body**: `{question, chat_id, execute, ...}`
- **Database**: 
  - Queries schema from all observability tables
  - Stores chat turn in sobs_ai_helper_chats
  - May call `/api/query/ask` internally
- **Returns**: `{answer, trace_id, chart_spec, tool_calls, ...}`

#### `/api/ai/helper/capabilities` (GET)
- **Returns**: List of AI capabilities (chat, tools, etc.)

#### `/api/ai/helper/actions/manifest` (GET)
- **Database**: Loads AI action definitions for current page
- **Returns**: `[{id, name, description, params}, ...]`

#### `/api/ai/helper/chats` (GET)
- **Database**: `SELECT * FROM sobs_ai_helper_chats ORDER BY CreatedAt DESC`
- **Returns**: List of past chat sessions

#### `/api/ai/helper/chats/<chat_id>` (GET)
- **Database**: Single chat session with turns
- **Returns**: Chat detail

#### `/api/ai/helper/feedback` (POST)
- **Body**: `{chat_id, turn_id, feedback, rating}`
- **Database**: Stores feedback in sobs_ai_helper_chats
- **Returns**: `{ok: true}`

#### `/api/ai/helper/actions/execute` (POST)
- **Body**: `{action_id, action_token, params}`
- **Database**: Executes AI-selected action (create issue, add tag, etc.)
- **Returns**: Action result

---

### Tags API

#### `/api/tags/<record_type>/<record_id>` (GET)
- **Database**: `SELECT * FROM sobs_record_tags WHERE RecordId=? AND RecordType=? AND IsDeleted=0`
- **Returns**: `{record_id, tags: {key: value, ...}}`

#### `/api/tags/<record_type>/<record_id>` (POST)
- **Body**: `{tags: {key: value, ...}}`
- **Database**: Insert/upsert into sobs_record_tags
- **Returns**: Updated tags

#### `/api/tags/<record_type>/<record_id>/<tag_key>` (DELETE)
- **Database**: Mark tag as IsDeleted=1 in sobs_record_tags
- **Returns**: `{deleted: true}`

---

### Query/NLQ API

#### `/api/query/ask` (POST) - NLQ to SQL
- **Body**: `{question, execute, chart, thinking_level}`
- **Database**:
  - Queries schema from all tables
  - Validates against allowed_tables
  - Executes generated SQL if `execute=true`
  - Stores in sobs_ai_helper_chats
- **Returns**: `{ok, sql, columns, rows, chart_spec, error, trace_id}`

#### `/api/query/run` (POST) - Execute SQL
- **Body**: `{sql, limit, offset, chart_type}`
- **Database**: Executes user SQL (whitelist-validated)
- **Returns**: `{columns, rows, field_types, error}`

#### `/api/query/refine-chart` (POST)
- **Body**: `{question, current_chart_spec}`
- **Database**: Uses AI to refine chart based on feedback
- **Returns**: Updated chart_spec

#### `/api/query/schema` (GET)
- **Database**: Returns schema metadata for all allowed tables
- **Returns**: `{tables: [{name, columns: [{name, dtype}, ...]}, ...]}`

#### `/api/query/add-to-dashboard` (POST)
- **Body**: `{dashboard_id, chart_spec, title}`
- **Database**: Inserts into sobs_chart_configs
- **Returns**: `{chart_id}`

---

### Dashboard API

#### `/api/dashboards/list` (GET)
- **Database**: `SELECT * FROM sobs_dashboards WHERE IsDeleted=0`
- **Returns**: Dashboard list

#### `/api/dashboards/query` (POST)
- **Body**: `{chart_id, ...}`
- **Database**: Executes chart query
- **Returns**: Chart data

#### `/api/dashboards/spec/templates` (GET)
- **Returns**: Available chart template metadata

#### `/api/dashboards/spec/options` (GET)
- **Database**: Chart template options/config schema
- **Returns**: Options by template type

#### `/api/dashboards/spec/compile` (POST)
- **Body**: `{chart_spec}`
- **Database**: Validates and compiles chart spec
- **Returns**: Compiled spec

#### `/api/dashboards/spec/dry-run` (POST)
- **Body**: `{chart_spec, limit}`
- **Database**: Executes chart query without storing
- **Returns**: Dry-run results

#### `/api/dashboards/spec/validate` (POST)
- **Body**: `{chart_spec}`
- **Returns**: `{valid, errors}`

#### `/api/dashboards/spec/render` (POST)
- **Body**: `{chart_spec}`
- **Database**: Renders ECharts spec
- **Returns**: Rendered chart options

#### `/api/dashboards/render` (POST)
- **Body**: `{dashboard_id, chart_ids, ...}`
- **Database**: Renders multiple charts
- **Returns**: Chart data array

#### `/api/dashboards/spec/ai-build` (POST)
- **Body**: `{question, current_page, preferred_chart_type}`
- **Database**: Uses AI to build chart from description
- **Returns**: Chart spec

#### `/api/dashboards/<dashboard_id>/charts/<chart_id>/export` (GET)
- **Database**: Exports chart data
- **Returns**: CSV/JSON export

#### `/api/dashboards/<dashboard_id>/charts/import` (POST)
- **Body**: Uploaded file
- **Database**: Imports chart data
- **Returns**: Imported chart count

---

### Reports API

#### `/api/reports` (GET)
- **Query Params**: `page_type` (optional)
- **Database**: `SELECT * FROM sobs_reports WHERE IsDeleted=0 AND (PageType=? OR PageType IS NULL)`
- **Returns**: Report list

#### `/api/reports` (POST)
- **Body**: `{name, description, page_type, filters}`
- **Database**: Insert into sobs_reports
- **Returns**: New report

#### `/api/reports/<report_id>` (DELETE)
- **Database**: Soft-delete from sobs_reports
- **Returns**: `{deleted: true}`

#### `/api/reports/export` (GET)
- **Query Params**: `page_type`, `format` (csv/json)
- **Database**: Exports filtered data by report
- **Returns**: CSV/JSON download

#### `/api/reports/import` (POST)
- **Body**: Uploaded file
- **Database**: Imports report definitions
- **Returns**: Import summary

---

### Web Traffic API

#### `/api/web-traffic/geo` (GET)
- **Database**: `SELECT LogAttributes['geo'], COUNT(*) FROM hyperdx_sessions GROUP BY LogAttributes['geo']`
- **Returns**: Geo distribution

#### `/api/web-traffic/browsers` (GET)
- **Returns**: Browser distribution

#### `/api/web-traffic/os` (GET)
- **Returns**: OS distribution

#### `/api/web-traffic/timezones` (GET)
- **Returns**: Timezone distribution

#### `/api/web-traffic/languages` (GET)
- **Returns**: Language distribution

#### `/api/web-traffic/devices` (GET)
- **Returns**: Device distribution

---

### Enrichment API

#### `/api/enrichment/libraries` (GET)
- **Database**: Queries library metadata (if enrichment enabled)
- **Returns**: Library list with vulnerability counts

#### `/api/enrichment/github/repo-health` (GET)
- **Query Params**: `repo`, `org`
- **Database**: Queries GitHub API (if token configured)
- **Returns**: Repo health metrics (stars, open issues, etc.)

#### `/api/enrichment/cve/findings` (GET)
- **Database**: `SELECT * FROM sobs_cve_findings WHERE IsDeleted=0`
- **Returns**: CVE findings

#### `/api/enrichment/cve/findings/<osv_id>/disposition` (POST)
- **Body**: `{disposition, comment}`
- **Database**: Updates CVE finding disposition
- **Returns**: Updated finding

#### `/api/enrichment/cve/scan` (POST)
- **Body**: `{...}`
- **Database**: Triggers CVE scan
- **Returns**: Scan job status

---

### Work Items API

#### `/api/work-items` (GET)
- **Query Params**: `status`, `repo`, `limit`, `offset`
- **Database**: `SELECT * FROM sobs_gh_work_items WHERE IsDeleted=0`
- **Returns**: Work item list

---

### Notifications API

#### `/api/notifications/channels/<channel_id>/test` (POST)
- **Database**: Loads channel config
- **Returns**: Test result (success/failure)

#### `/api/notifications/rules/auto-generate` (POST)
- **Body**: `{page_type, condition}`
- **Database**: Generates rules for page
- **Returns**: Generated rules

#### `/api/notifications/check` (POST)
- **Body**: `{condition, ...}`
- **Database**: Validates notification rule
- **Returns**: Matching records count

#### `/api/notifications/vapid-public-key` (GET)
- **Database**: Loads WebPush VAPID key from sobs_app_settings
- **Returns**: Public key

#### `/api/notifications/subscribe` (POST)
- **Body**: `{subscription, ...}`
- **Database**: Stores subscription
- **Returns**: `{ok: true}`

#### `/api/notifications/vapid-keygen` (POST)
- **Database**: Generates new VAPID keypair, stores in sobs_app_settings
- **Returns**: Public key

#### `/api/notifications/vapid-keys` (DELETE)
- **Database**: Deletes VAPID keys
- **Returns**: `{deleted: true}`

---

### Issues & Agents API

#### `/api/issues/raise` (POST)
- **Body**: `{title, description, repo, assignee, ...}`
- **Database**: 
  - Creates GitHub issue via API
  - Stores in sobs_gh_work_items
- **Returns**: `{issue_number, issue_url, ...}`

#### `/api/agent/runs` (GET)
- **Database**: `SELECT * FROM sobs_agent_runs ORDER BY CreatedAt DESC LIMIT ?`
- **Returns**: Recent agent run history

#### `/api/agent/runs` (POST)
- **Body**: `{rule_id, ...}`
- **Database**: Triggers agent rule
- **Returns**: Run status

#### `/api/agent/runs/<run_id>/dismiss` (POST)
- **Database**: Mark run as dismissed
- **Returns**: `{dismissed: true}`

---

### Setup Wizard API

#### `/api/setup-wizard/steps` (GET)
- **Database**: Checks configuration completeness
- **Returns**: Step status array

#### `/api/onboarding/create-repo` (POST)
- **Body**: `{repo_name, ...}`
- **Database**: Creates app entry
- **Returns**: App config

#### `/api/onboarding/import-repo` (POST)
- **Body**: `{github_url, ...}`
- **Database**: Imports app config
- **Returns**: App config

#### `/api/onboarding/list-repos` (POST)
- **Body**: `{org, ...}`
- **Database**: Lists repos from GitHub
- **Returns**: Repo list

#### `/api/onboarding/inspect-repo` (GET)
- **Query Params**: `repo_url`
- **Database**: Inspects repo for instrumentation
- **Returns**: Inspection results

#### `/api/onboarding/create-issues` (POST)
- **Body**: `{repos: [...], issue_template}`
- **Database**: Creates setup issues
- **Returns**: Created issue URLs

---

### Chart Types API

#### `/api/chart-types` (GET)
- **Returns**: List of available chart template types with metadata

---

### Table Explorer API

#### `/api/table-explorer/tables` (GET)
- **Database**: Returns all tables in QUERY_ALLOWED_TABLES
- **Returns**: Table list

#### `/api/table-explorer/table/<name>` (GET)
- **Database**: `SELECT * FROM {table_name} LIMIT 100`
- **Returns**: Table schema + sample rows

---

### Kubernetes API

#### `/api/kubernetes/status` (GET)
- **Database**: Queries k8s endpoint config
- **Returns**: K8s cluster status (if enabled)

---

### Data Management API

#### `/api/data-management/backup/list` (GET)
- **Database**: Lists backup files from filesystem
- **Returns**: Backup list with timestamps, sizes

#### `/api/data-management/backup/run` (POST)
- **Database**: Triggers backup (async)
- **Returns**: Backup job ID

#### `/api/data-management/restore` (POST)
- **Body**: `{backup_id}`
- **Database**: Triggers restore (async)
- **Returns**: Restore job ID

---

## Ingestion Endpoints (V1)

All V1 endpoints require `X-API-Key` header (or external auth token via Authorization header).

### `/v1/logs` (POST)
- **Body**: OTLP LogRecord format (JSON)
- **Database**: Insert into otel_logs
- **Returns**: `{ok: true}` or error
- **Related**: Stores log attributes in LogAttributes (JSON)

### `/v1/traces` (POST)
- **Body**: OTLP Span format (JSON)
- **Database**: Insert into otel_traces
- **Returns**: `{ok: true}` or error

### `/v1/metrics` (POST)
- **Body**: OTLP Metric format (JSON)
- **Database**: Insert into derived_signals (aggregated metrics view)
- **Returns**: `{ok: true}` or error

### `/v1/errors` (POST)
- **Body**: Error event format
- **Database**: Insert into otel_logs with error attributes
- **Returns**: `{ok: true}` or error

### `/v1/rum` (POST)
- **Body**: RUM event format (JSON)
- **Database**: Insert into hyperdx_sessions
- **Returns**: `{ok: true}` or error

### `/v1/ai` (POST)
- **Body**: AI span format (JSON)
- **Database**: Insert into otel_traces with gen_ai.* attributes
- **Returns**: `{ok: true}` or error

### `/v1/rum/assets` (POST)
- **Headers**: X-API-Key
- **Body**: Binary or multipart RUM asset upload
- **Database**: Stores RUM SDK files in filesystem (data/rum_assets/)
- **Returns**: `{asset_id}`

### `/v1/rum/assets/<asset_id>` (GET)
- **Headers**: X-API-Key or unsigned
- **Database**: Retrieves RUM asset from filesystem
- **Returns**: Asset binary with CORS headers

### `/v1/rum/client-token` (POST)
- **Body**: `{app_id, ...}`
- **Database**: Queries sobs_app_settings for app config
- **Returns**: `{token, rum_endpoint, ...}`

### `/v1/apps` (GET)
- **Headers**: X-API-Key
- **Database**: `SELECT * FROM sobs_app_settings WHERE Key LIKE 'app.%'`
- **Returns**: App list

### `/v1/apps` (POST)
- **Body**: `{name, description, ...}`
- **Database**: Stores app config in sobs_app_settings
- **Returns**: New app

### `/v1/apps/<app_id>` (GET)
- **Database**: Queries app settings
- **Returns**: App config

### `/v1/apps/<app_id>` (PATCH)
- **Body**: Updated app config
- **Database**: Updates sobs_app_settings
- **Returns**: Updated app

### `/v1/apps/<app_id>/releases` (GET)
- **Database**: Lists releases for app
- **Returns**: Release list

### `/v1/apps/<app_id>/releases` (POST)
- **Body**: `{version, artifacts, ...}`
- **Database**: Stores release metadata
- **Returns**: Release ID

### `/v1/releases/<release_id>` (GET)
- **Database**: Queries release metadata
- **Returns**: Release detail

### `/v1/releases/<release_id>/artifacts` (GET)
- **Database**: Lists artifacts for release
- **Returns**: Artifact list

### `/v1/releases/<release_id>/artifacts/meta` (POST)
- **Body**: Artifact metadata
- **Database**: Stores artifact metadata
- **Returns**: Metadata stored

---

## Static & Health Routes

### `/static/rum.js`, `/static/rum.js.map`, `/static/rum.min.js`, `/static/rum.min.js.map`, `/static/rum.d.ts`
- **HTTP Methods**: GET
- **Handler**: Serve pre-built RUM SDK files from data/rum_assets/ or generated from TypeScript

### `/service-worker.js` (GET)
- **Handler**: Serve service worker for WebPush notifications

### `/health` (GET)
- **Handler**: Returns `{status: ok}`

### `/health/db` (GET)
- **Handler**: Checks database connectivity, returns `{status: ok|error, details}`

---

## Dependency Graph

### Critical Path (must implement first)

1. **Data Ingestion** (V1 endpoints)
   - `/v1/logs` (POST)
   - `/v1/traces` (POST)
   - `/v1/metrics` (POST)
   - `/v1/errors` (POST)
   - `/v1/rum` (POST)
   - `/v1/ai` (POST)

2. **Core Data Tables & Views**
   - `otel_logs`
   - `otel_traces`
   - `hyperdx_sessions` (RUM)
   - `v_derived_signals_anomaly` (metrics view)

3. **Data Page Routes** (HTML)
   - `/` (summary)
   - `/logs`
   - `/traces`
   - `/metrics`
   - `/errors`
   - `/rum`

4. **Basic Config** (Settings)
   - `/settings`
   - `/settings/masking`
   - `/settings/tags`
   - `sobs_app_settings` table

5. **API Data Endpoints** (JSON)
   - Log/trace/metric field hints
   - Filter/regex validation
   - Tags CRUD
   - Query APIs (`/api/query/*`)

### Secondary Path (can implement after critical path)

6. **Advanced Pages**
   - `/incident` (correlation)
   - `/ai` (AI observability)
   - `/web-traffic` (RUM analysis)
   - `/dashboards` (custom dashboards)
   - `/reports` (saved reports)

7. **Advanced Config**
   - `/settings/ai` (AI integration)
   - `/settings/notifications` (channels & rules)
   - `/settings/repositories` (GitHub integration)
   - `/settings/agents` (Copilot agents)
   - `/settings/kubernetes` (K8s integration)
   - `/settings/enrichment` (CVE/library enrichment)
   - `/settings/data-management` (backup/restore)

8. **Enrichment APIs**
   - `/api/enrichment/*` (CVE, libraries, etc.)
   - `/api/work-items` (GitHub work items)

### Optional/Tertiary (nice-to-have)

9. **NLQ/AI Features**
   - `/query` page
   - `/api/query/ask` (NLQ to SQL)
   - `/api/ai/helper` (chat)

10. **Advanced Infrastructure**
    - `/kubernetes` page
    - `/settings/kubernetes`
    - `/api/kubernetes/status`

---

## Critical vs Optional Routes

### CRITICAL (Required for MVP)
- ✅ `/` – summary page
- ✅ `/logs` – log viewer
- ✅ `/traces` – trace viewer
- ✅ `/metrics` – signal viewer
- ✅ `/errors` – error grouping/viewer
- ✅ `/rum` – RUM session viewer
- ✅ `/settings` – config hub
- ✅ `/settings/masking` – masking rules
- ✅ `/settings/tags` – tag rules
- ✅ All V1 ingest endpoints (`/v1/*`)
- ✅ All API field-hints, validate-*, tags CRUD
- ✅ `/health`, `/health/db` – health checks

### OPTIONAL (Can defer)
- ⭕ `/incident` – correlation dashboard
- ⭕ `/ai` – AI observability
- ⭕ `/web-traffic` – RUM analysis
- ⭕ `/dashboards` – custom dashboards
- ⭕ `/reports` – saved reports
- ⭕ `/query` – NLQ query builder
- ⭕ `/settings/notifications` – notification rules
- ⭕ `/settings/ai` – AI config (requires external LLM)
- ⭕ `/settings/agents` – GitHub Copilot integration
- ⭕ `/settings/repositories` – GitHub repo config
- ⭕ `/enrichment/cve` – CVE scanning
- ⭕ `/kubernetes` – K8s integration
- ⭕ `/tail` – live streaming
- ⭕ All help routes (`/**/help`)

---

## Notes for Porting

### Database Schema Expectations

**Core Tables**
- `otel_logs` – (Timestamp, SeverityText, ServiceName, Body, TraceId, SpanId, EventName, LogAttributes)
- `otel_traces` – (Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration, StatusCode, SpanAttributes)
- `hyperdx_sessions` – (Timestamp, EventName, Body, LogAttributes, TraceId, SpanId, ServiceName)
- `sobs_record_tags` – (RecordId, RecordType, TagKey, TagValue, IsDeleted, Version)
- `sobs_error_resolutions` – (ErrorId, ResolvedAt, Comment, Version)
- `sobs_dashboards` – (Id, Name, Description, IsDeleted, Version)
- `sobs_chart_configs` – (Id, DashboardId, Title, ChartType, Query, OptionsJson, Position, IsDeleted, Version)
- `sobs_reports` – (Id, Name, Description, PageType, FiltersJson, IsDeleted, Version)
- `sobs_anomaly_rules` – (Id, Source, Signal, Service, AttrFingerprint, Condition, Version, IsDeleted)
- `sobs_tag_rules` – (Id, Name, Condition, TagsJson, Enabled, Version, IsDeleted)
- `sobs_app_settings` – (Key, Value) – key-value store for all config

### Important Implementation Notes

1. **Time Windows**: All pages support `from_ts` / `to_ts` parameters. Use ISO 8601 format or Unix timestamps.
2. **Soft Deletes**: Config tables use `IsDeleted` flag + `Version` timestamp instead of hard deletes.
3. **Pagination**: Standard `limit` (default varies) and `offset` parameters.
4. **Sorting**: Each page defines sortable columns (e.g., `sort_by=Timestamp&sort_dir=DESC`).
5. **Caching**: Summary stats page uses TTL cache (60s). Attr key cache is primed on startup.
6. **Auth**: All routes require @require_basic_auth. V1 ingest routes require X-API-Key header.
7. **Error Handling**: Return HTTP 400/403/404/500 with JSON `{error: "message"}` or HTML error pages.
8. **Masking**: All output respects masking settings (stored in sobs_app_settings and cached).
9. **Tags**: Tags are stored in sobs_record_tags with RecordId = MD5(ServiceName|Timestamp|TraceId|SpanId).

### Regex & Filter Support

- Logs/errors/traces/RUM all support regex filtering via `q` parameter.
- Regex patterns are RE2 (Google's regex engine). Validated server-side.
- Raw SQL WHERE clauses supported on some pages (e.g., `/logs`, `/errors`, `/ai`) with input validation.

### Advanced Features (Low Priority for MVP)

- AI NLQ query builder (`/api/query/ask`) – requires external LLM integration
- GitHub Copilot agents (`/api/agent/runs`) – requires GitHub token + Copilot setup
- WebPush notifications (`/api/notifications/subscribe`) – requires VAPID keypair setup
- CVE enrichment (`/api/enrichment/cve/scan`) – requires external CVE database

---

## Summary Table: Route Counts by Category

| Category | GET (HTML) | POST (HTML) | GET (JSON) | POST (JSON) | DELETE | Total |
|----------|-----------|-----------|-----------|-----------|--------|-------|
| Core Data Pages | 6 | 0 | 0 | 0 | 0 | 6 |
| Settings Pages | 8 | 18 | 0 | 0 | 0 | 26 |
| Dashboard/Reports | 4 | 2 | 2 | 2 | 1 | 11 |
| Infrastructure | 2 | 0 | 6 | 1 | 0 | 9 |
| Query/Explorer | 2 | 0 | 4 | 3 | 0 | 9 |
| API Endpoints | 0 | 0 | 24 | 62 | 6 | 92 |
| V1 Ingestion | 0 | 6 | 2 | 1 | 0 | 9 |
| Static/Health | 0 | 0 | 2 | 0 | 0 | 2 |
| Help Routes | 0 | 0 | 0 | 0 | 0 | 28 |
| **TOTAL** | **24** | **20** | **40** | **69** | **7** | **183** |

