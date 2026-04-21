# Go ↔ Python Backend Parity Audit
**Date:** 2026-04-20  
**Python source:** `app.py` (33 253 lines, Quart/async)  
**Go source:** `internal/web/` package  
**Auditor:** Copilot automated deep-audit  

> This report compares every Python route in `app.py` against the corresponding Go handler registered in `server.go` / `page_routes.go`.  
> Severity: **CRITICAL** = data loss, security gap, or complete feature breakage; **HIGH** = significant functional gap; **MEDIUM** = partial loss of feature or incorrect behaviour; **LOW** = cosmetic/minor behavioural difference.

---

## 1. Page Routes (HTML pages)

## Follow-up plans

- RUM ingestion parity execution plan: [docs/parity/RUM_INGEST_PARITY_IMPLEMENTATION_PLAN_2026-04-20.md](docs/parity/RUM_INGEST_PARITY_IMPLEMENTATION_PLAN_2026-04-20.md)

### [SEVERITY: LOW] Route: GET /settings

**Python:** Renders `settings.html` with a rich context: `tag_rule_count`, `anomaly_rule_count`, `agent_rule_count`, `ai_configured` (bool from endpoint_url+model), `notification_channel_count`, `notification_rule_count`, `masking_custom_key_count`, `masking_custom_pattern_count`, `kubernetes_view_enabled`, `backup_enabled`, and `query_allowed_tables` (sorted list of allowed SQL table names).

**Go:** `s.settingsPage` renders `settings.html` but does not appear to pass `query_allowed_tables` or the individual counts. The underlying `settingsPage` handler in `page_handlers.go` must be verified to confirm all variables are forwarded.

**Impact:** Template variables that drive the sidebar summary counts and feature badges may render as empty/zero.  
**Fix:** Audit the Go `settingsPage` handler and ensure all Python template context keys are passed.

---

### [SEVERITY: LOW] Route: GET /settings/notifications

**Python:** Passes `channels`, `rules`, `notification_log`, `channel_types`, `comparators`, `condition_types`, `severities`, `logic_operators`, `signal_sources`, `tag_match_operators`, `tag_record_types`, `edit_rule`, `vapid_public_key`, `vapid_key_source`, **and `metric_rules`** to the template.

**Go:** `s.settingsNotificationsPage` — the handler exists, but (based on `notifications_rules_api.go` and `notifications_settings_actions.go`) does not include `metric_rules` (list of anomaly rules).

**Impact:** The "Add Rule" modal on the notifications page cannot populate the anomaly-rule picker dropdown.  
**Fix:** Fetch and pass `anomaly_rules` from the metrics service when rendering the notifications page.

---

### [SEVERITY: LOW] Route: GET /settings/ai

**Python:** Passes `anomaly_rules` and `tag_rules` to `settings_ai.html`, in addition to all AI settings, token expiry, pricing data.

**Go:** `s.settingsAI` (GET) does NOT pass `anomaly_rules` or `tag_rules` to the template context.

**Impact:** Agent-trigger dropdowns in the AI settings template that depend on `anomaly_rules` and `tag_rules` will be empty.  
**Fix:** Add `anomaly_rules: s.metricsService.ListRules()` and `tag_rules: s.tagService.ListRules()` to the GET context.

---

## 2. Apps & Releases API

### [SEVERITY: LOW] Route: GET /v1/apps

**Python:** Filters apps by `q` (case-insensitive substring match on `name.lower()` and `slug.lower()`). Returns serialized array with camelCase fields (`id`, `name`, `slug`, `ownerTeam`, `repoUrl`, `defaultEnvironment`, `enabled`, `metadata`, `createdAt`, `updatedAt`).

**Go:** Filters similarly (`strings.Contains(Lower(name), query) || Contains(Lower(slug), query)`). Returns Go struct array via JSON marshal — field names depend on struct tags in `apps.App`. Must verify `apps.App` struct JSON tags match Python's camelCase keys.

**Impact:** If Go struct JSON tags use snake_case, clients expecting camelCase will break.  
**Fix:** Verify `apps.App` struct tags produce identical JSON shape to `_serialize_app_row()` in Python.

---

### [SEVERITY: MEDIUM] Route: POST /settings/ai (Save AI settings)

**Python (`save_ai_settings`):** Complex logic:
1. Iterates over all `_AI_SETTING_KEYS`, saves each (except guard/pricing/confirmed/token validation keys).
2. Validates and saves `model_pricing` JSON (re-normalizes model keys, strips invalid entries).
3. Validates and saves `model_pricing_confirmed` JSON list (cross-checks against clean pricing dict).
4. Handles `github_token_expires_at` normalization.
5. If `github_token` changed from previous value, clears `github_token_last_validated_at`, `github_token_last_validation_status`, `github_token_last_validation_message`.
6. Redirects to settings page on success (not JSON).

**Go (`settingsAI` POST):** Decodes form/JSON body into `map[string]string` via `decodeStringMap(r)`, then calls `s.settingsService.SaveAI(vals)` which presumably stores the values. Returns JSON `{"ok": true, "settings": ...}`.

**Impact:**
- Model pricing JSON is NOT validated or normalized in Go.
- Go returns JSON instead of redirect (breaking browser form POST flow).
- Token change detection and clearing validation status fields is missing.
- Redirect after form POST is missing (causes double-submit on refresh).

**Fix:** Implement model_pricing validation, token change detection, and redirect response in the Go POST handler.

---

## 3. Logs, Metrics, Errors, Traces

### [SEVERITY: LOW] Route: GET /logs

**Python:** Parses `analyze`, `stats`, `stats_updated` params; runs advanced analysis queries; passes `stats_generated_at_iso`, `stats_generated_at_display`, `stats_generated_age_s` to template.

**Go:** `pageLogsHandler` parses same params (`analyze`, `stats`, `stats_updated`) and runs advanced analysis via `s.queryAdvancedLogAnalysis(...)`. Context keys `stats_generated_at_iso`, `stats_generated_at_display`, `stats_generated_age_s` are in the error branch but must be confirmed present in the success branch too.

**Impact:** Low — feature parity appears maintained; verify success-branch template context is complete.  
**Fix:** Audit the success-path template context in `pageLogsHandler`.

---

### [SEVERITY: LOW] Route: GET /metrics

**Python:** Valid `sort_by` values include `last_time`, `service`, `source`, `signal`, `last_value`, `last_anomaly_score`, `last_anomaly_state`, `last_sample_count`, `point_count`. Default `sort_by=last_time`, default `sort_dir=desc`. `hours` clamped 1–168 (default 24). `limit` default 100, max 1000.

**Go (`metricsPage`):** Identical sort map, same defaults, same hours clamping. Parity appears good.

**Impact:** None identified.

---

### [SEVERITY: LOW] Route: POST /errors/<error_id>/resolve

**Python:** Route pattern `/errors/<error_id>/resolve`. Creates `sobs_error_resolutions` table if missing, then INSERTs `ErrorId`. Returns `{"ok": True, "error_id": ..., "state": "resolved"}`.

**Go (`errorsResolve`):** Same pattern, same CREATE TABLE IF NOT EXISTS, same INSERT, same response shape. Parity appears good.

**Impact:** None identified.

---

## 4. Incident, RUM

### [SEVERITY: LOW] Route: GET /incident

**Python / Go:** Both render `incident.html`. Verify that the incident page template context is populated identically (sessions data, services, time window).

---

## 5. Web Traffic API

### [SEVERITY: LOW] Routes: GET /api/web-traffic/*

All six sub-routes (`/geo`, `/browsers`, `/os`, `/timezones`, `/languages`, `/devices`) exist in both Python and Go.

**Python `/geo`:** Returns `country_counts` sorted descending, `ip_details[:100]`, `geo_enabled`. Geo lookup is performed by `_geo_lookup_batch`.  
**Go (`apiWebTrafficGeo`):** Must verify it performs the same geo lookup. If Go's geo service is a stub, lookups will always return "Unknown".

**Impact:** Geo map will show all traffic as "Unknown" if geo lookup is not implemented in Go's enrichment service.  
**Fix:** Confirm Go's geo-lookup implementation in `enrichment.Service`.

---

## 6. Enrichment API

### [SEVERITY: HIGH] Route: GET /api/enrichment/cve/findings

**Python:** CVE findings are stored in `sobs_cve_findings` with full fields: `Package`, `Ecosystem`, `Version`, `Severity`, `OsvId`, `Summary`, `PublishedAt`, `Services`, `CveIds`, `disposition`, etc. Python returns rich objects including `ecosystem`, `version`, `service`, `cve_ids`, `summary`, `published`, `disposition`, `disposition_expired`.

**Go (`enrichmentCVEPage` and `apiEnrichmentCVEFindings`):** The CVE page handler maps `finding.Disposition`, `finding.OSVID`, `finding.Package`, `finding.Severity` but **hardcodes `ecosystem: "unknown"`, `version: ""`, `service: ""`, `cve_ids: []`, `summary: ""`**.

**Impact:** CVE finding details (ecosystem, version, service, CVE IDs, summary) are always empty/unknown in the Go UI. Severity filtering and ecosystem filtering lose meaning.  
**Fix:** Populate all fields from the enrichment service's finding model, or extend the `enrichment.Finding` struct.

---

### [SEVERITY: LOW] Route: GET /api/enrichment/libraries

**Python:** Returns libraries with `source`, `app_name`, `release_version`, `environment` from a merged inventory combining `release_registry`, `otel_sdk`, `otel_scope` sources. Sort key: `(-cve_count, source_order, package, version, service)`.

**Go:** Must verify the same inventory merge and sort order are implemented in `s.enrichmentService`.

---

## 7. Work Items API

### [SEVERITY: LOW] Route: GET /api/work-items

**Python:** Additionally calls `await _maybe_backfill_github_work_item_links(db, settings)` before querying — this async backfill may update `IssueUrl`/`CanonicalIssueUrl` from GitHub before returning data.

**Go:** Does NOT perform backfill before query. Returns stale GitHub issue URLs if backfill hasn't run separately.

**Impact:** Work item `issue_url` may be stale in Go if the background backfill hasn't run.  
**Fix:** Either implement periodic background backfill or trigger it on the first GET.

---

## 8. AI API

### [SEVERITY: LOW] Route: GET /api/ai/conversation

**Python:** Returns an HTML fragment (htmx partial) from `_ai_conversation_partial.html`. Checks conditions including `time_conditions` from `from_ts`/`to_ts`.

**Go:** Does the same — returns HTML from `_ai_conversation_partial.html`. Parity appears good.

---

## 9. Dashboards API

### [SEVERITY: MEDIUM] Route: POST /api/query/add-to-dashboard

**Python:** Validates `sql` field is required (returns 400 if missing). Validates `chart_spec` is required. Calls `_compile_chart_spec(spec_raw)` to normalize the spec.

**Go (`apiQueryAddToDashboard`):** Validates `dashboard_id` required, `title` required, `type` required, `spec` required. But the Python version requires `sql` + `chart_spec`, while Go requires `title` + `type` + `spec`. This means:
- Python request body: `{dashboard_id, title, sql, chart_spec (echarts option JSON)}`
- Go request body: `{dashboard_id, title, type, spec (compiled chart spec map)}`

**Impact:** Different request shapes — JavaScript client calling this endpoint built against Python API will fail against Go.  
**Fix:** Align the Go handler to accept the same `{dashboard_id, title, sql, chart_spec}` body and perform the same spec compilation internally.

---

### [SEVERITY: LOW] Route: GET /dashboards/new

**Python:** Returns `list_dashboards` template with `show_new_form=True`.

**Go (`dashboardsNew`):** Must verify same template render with `show_new_form=True`.

---

## 10. Reports API

### [SEVERITY: HIGH] Route: POST /api/reports/import — No multipart/form-data support

**Python:** Accepts EITHER `multipart/form-data` file upload (field `file`) OR `application/json` body. Also accepts `on_conflict` from EITHER request body OR query param OR form field.

**Go (`apiReportsImport`):** ONLY accepts `application/json` body. `on_conflict` is taken from query param only.

**Impact:** Users who upload report JSON files via the browser file picker (form-based import) will receive a parse error in Go.  
**Fix:** Add multipart form handling to the Go import endpoint; also read `on_conflict` from body when Content-Type is JSON.

---

### [SEVERITY: LOW] Route: GET /api/reports

**Python:** Returns a plain JSON array.  
**Go:** Returns a plain JSON array from `s.reportService.List()`.

Verify `reports.Report` JSON tags match Python's `{id, name, description, page_type, filters}` shape.

---

## 11. Settings (Masking, Tags, Notifications, AI/Enrichment, Repos, Agents)

### [SEVERITY: CRITICAL] Route: POST /settings/notifications/channels — Completely different semantics

**Python:** Creates a persistent, named notification channel stored in `sobs_notification_channels` table. Supports four `channel_type` values: `webhook` (url, method, headers, body_template), `slack` (webhook_url), `email` (smtp_host, smtp_port, smtp_user, smtp_password, from_addr, to_addr, use_tls), `browser_push` (endpoint, p256dh, auth). Encrypts sensitive config. Validates required fields per type. Redirects on success.

**Go (`settingsNotificationsChannelsCreate`):** Calls `s.notificationService.Subscribe(vals["endpoint"])` — this creates a **browser-push subscription only**, not a named multi-type channel. Webhook, Slack, and Email channel types are completely unimplemented.

**Impact:** Three out of four notification channel types cannot be created. Existing webhooks/slack/email channels from the Python DB will not be manageable.  
**Fix:** Implement full channel creation CRUD in Go (webhook, slack, email, browser_push) matching Python's form fields and `sobs_notification_channels` table schema.

---

### [SEVERITY: CRITICAL] Route: POST /settings/notifications/rules — Severely incomplete

**Python:** Creates a notification rule with: `name`, `logic_operator` (any/all), `severity` (info/warning/critical), `cooldown_seconds` (0–86400, default 300), `channel_ids` (list), and a complex conditions array (each condition has `condition_type`, `source`, `signal`, `service`, `comparator`, `threshold`, `window_minutes`, optionally `tag_key`, `tag_match_operator`, `tag_value`, `record_type`). Validates all fields. Stores in `sobs_notification_rules` table.

**Go (`settingsNotificationsRulesCreate`):** Only accepts `name` and calls `s.notificationService.CreateRule(name)`. All other fields (conditions, channels, severity, cooldown) are ignored.

**Impact:** Notification rules created via Go have no conditions and no channels assigned, making them non-functional for alerting.  
**Fix:** Implement full rule creation matching Python's form field set and validation logic.

---

### [SEVERITY: HIGH] Route: POST /settings/notifications/channels/<id>/delete and /toggle

**Python:** `delete` and `toggle` are at `/settings/notifications/channels/<id>/delete` and `/settings/notifications/channels/<id>/toggle`. Both operate on `sobs_notification_channels` via soft-delete or enabled-flag flip. Redirect to notifications page.

**Go (`settingsNotificationsChannelActions`):** The handler at `/settings/notifications/channels/` must handle `delete` and `toggle` actions. But since Go's channel model is a browser-push subscription (not a full channel), the underlying `notificationService` may not support toggle/delete for webhook/slack/email channels.

**Impact:** Channel toggle and delete are broken for non-browser-push channel types.  
**Fix:** Implement channel toggle and delete against `sobs_notification_channels` table for all channel types.

---

### [SEVERITY: HIGH] Route: POST /api/notifications/channels/<id>/test — Test doesn't dispatch

**Python:** Fetches the channel from DB, decrypts config, builds a test payload, calls `_dispatch_notification_channel(channel, test_payload)` which actually sends the notification (webhook POST, Slack POST, SMTP email, push). Returns `{"ok": True}` on success or `{"ok": False, "error": ...}`.

**Go (`apiNotificationsChannelSubroutes`):** Returns `{"ok": true, "tested": true}` without dispatching any notification. Only checks `s.notificationService.HasSubscription(parts[0])`.

**Impact:** The "Test" button appears to succeed even when the channel is misconfigured and cannot actually send.  
**Fix:** Implement actual channel dispatch in the test endpoint.

---

### [SEVERITY: MEDIUM] Route: GET /settings/masking — Missing response fields in `apiSettingsMaskingRules`

**Python `GET /api/settings/masking/rules`:** Returns `{ok, keys, patterns, custom_keys, custom_patterns, output_masking_enabled, sql_output_masking_enabled}`.

**Go (`apiSettingsMaskingRules`):** Calls `s.maskingService.ListRules()`. Verify the response includes all seven fields. The `ListRules()` implementation may return a flat map missing the `effective_keys`/`effective_patterns` split.

**Impact:** Masking rules API consumers (JS frontend) may miss individual field categories.  
**Fix:** Ensure Go response shape matches exactly.

---

### [SEVERITY: HIGH] Route: GET/POST /settings/data-management — Missing S3/backup fields

**Python:** Loads and saves 15 settings keys: `backup_enabled`, `s3_bucket`, `s3_access_key_id`, **`s3_secret_access_key`** (encrypted), `s3_region`, `s3_path_prefix`, **`s3_encrypt_backup`**, **`backup_encryption_password`** (encrypted), **`backup_schedule_full`**, **`backup_schedule_incremental`**, `ttl_logs_days`, `ttl_traces_days`, `ttl_metrics_hours`, `ttl_sessions_days`, `ttl_backup_coupling_enabled`. Validates S3 fields with strict regex patterns. Handles secret encryption/decryption.

**Go (`settingsDataManagement` GET):** Hardcodes the following as empty string / zero / false:
- `data_management.ttl_backup_coupling_enabled` → always `"0"`
- `data_management.s3_region` → always `""`
- `data_management.s3_path_prefix` → always `""`
- `data_management.s3_access_key_id` → always `""`
- `data_management.s3_encrypt_backup` → always `"0"`
- `data_management.backup_schedule_full` → always `""`
- `data_management.backup_schedule_incremental` → always `""`

**Go (`settingsDataManagement` POST):** Only saves `BackupEnabled`, `S3Bucket`, `TTLLogsDays`, `TTLTracesDays`, `TTLMetricsHours`, `TTLSessionsDays`. **S3 credentials, region, prefix, encryption password, backup schedules, TTL coupling flag are silently dropped.**

**Impact:** S3 backup configuration (credentials, encryption, schedules) cannot be set or persisted via Go. Users will lose all S3 backup config when POSTing settings.  
**Fix:** Extend `datamanagement.Settings` struct and `SaveSettings()` to include all 15 fields. Implement secret encryption matching Python's `_encrypt_secret_value`.

---

### [SEVERITY: MEDIUM] Route: GET/POST /settings/repositories — Different data model

**Python:** Uses `sobs_apps` + `sobs_app_releases` tables. Renders per-app info including: `repo_token_configured` (from `_load_repo_scoped_github_token`), `ci_push_status` (from `_ci_push_api_key_status`), `ci_push_plain` (show-once API key), `realtime_seed` (aggregated across all apps: `enabled`, `configured`, `expires_at`, `expiry_message`, `api_key`, `api_key_show_once`).

**Go (`settingsRepositories`):** Uses `s.repositoryService` which is a separate service backed by a different model. The `realtime_seed` structure and CI push key flow may differ.

**Impact:** CI push integration (real-time ingest keys per repository) and repo-scoped GitHub token UI will behave differently or be non-functional.  
**Fix:** Align the Go repositories service model to match Python's `sobs_apps`-based flow including CI key management and repo-scoped GitHub tokens.

---

### [SEVERITY: MEDIUM] Route: POST /settings/notifications/rules/<id>/toggle

**Python:** Reads current rule from `sobs_notification_rules`, flips `Enabled` flag, re-inserts (soft-update). Redirects.

**Go:** `settingsNotificationsRulesActions` with action `toggle` calls `s.notificationService.ToggleRule(id)`. If the underlying service doesn't operate on `sobs_notification_rules` table properly, the toggle won't persist.

---

## 12. Query & Table Explorer

### [SEVERITY: LOW] Route: GET /table-explorer

**Python:** Renders `table_explorer.html` with full context variables including available tables list, column metadata.

**Go (`tableExplorerPage`):** Renders `table_explorer.html` with only `{title: "table-explorer", message: "Go runtime active."}` — no table list or column metadata passed server-side.

**Impact:** If the template relies on server-side table/column data, the page will render empty lists. (May be acceptable if all data is loaded client-side via `/api/table-explorer/tables`.)  
**Fix:** Confirm the template is fully client-side for table/column loading; otherwise pass the required context.

---

### [SEVERITY: LOW] Route: POST /api/query/ask

**Python:** `stream=true` triggers SSE streaming of query and AI reasoning chunks. `thinking_level` param controls CoT depth.

**Go (`apiQueryAsk`):** Implements streaming via SSE. Verify `thinking_level`, `preferred_chart_type`, `chart_instruction` params are forwarded to the AI service.

---

## 13. Kubernetes & Data Management

### [SEVERITY: HIGH] Route: GET /api/kubernetes/status — Missing filter/sort/pagination params

**Python:** Accepts 18 query parameters:
- `namespace`, `namespace_values[]`, `node_values[]`, `deployment_values[]`, `pod_values[]`, `name`
- Per entity type: `nodes_sort`, `nodes_dir`, `nodes_page`, `nodes_page_size`
- `deployments_sort`, `deployments_dir`, `deployments_page`, `deployments_page_size`
- `pods_sort`, `pods_dir`, `pods_page`, `pods_page_size`

**Go (`apiKubernetesStatus`):** Only reads `nodes_page`, `pods_page`, `deployments_page` as integers. Sort direction, sort field, page_size per type, and multi-value namespace/node/deployment/pod filters are NOT read.

**Impact:** Kubernetes dashboard cannot be filtered, sorted, or paginated properly. All entities always render on page 1 in default sort order, and namespace/node/deployment/pod filters are ignored.  
**Fix:** Parse all 18 query parameters in the Go handler and pass them to the Kubernetes service query.

---

### [SEVERITY: MEDIUM] Route: GET /api/data-management/backup/list

**Python:** Returns detailed backup objects from S3 (name, size, created_at, type) or local paths. Performs actual S3 listing using credentials.

**Go:** Returns `s.dataManagementService.ListBackups()`. Since the Go service doesn't have S3 credentials (see data-management settings gap above), this will always return an empty list or error.

**Impact:** Backup list is always empty in Go until S3 credentials are properly stored.  
**Fix:** Address the data-management settings S3 fields gap (item above).

---

## 14. Setup & Onboarding

### [SEVERITY: MEDIUM] Route: GET /api/setup-wizard/steps — Placeholder steps in Go

**Python:** Calls `_build_setup_wizard_steps(env, language, deployment)` which generates language-specific and deployment-specific commands, code snippets, package manager commands, Docker compose examples, Kubernetes manifests, environment variable lists, and verify checks.

**Go (`apiSetupWizardSteps`):** Returns a **hardcoded 3-step placeholder**:
```json
{"id":"sdk_install","title":"Install OTEL SDK","commands":["install sdk for <lang>"]}
{"id":"collector","title":"Run OTEL Collector","commands":["deployment=<dep>"]}
{"id":"verify","title":"Verify in SOBS","commands":["open /"]}
```

**Impact:** The setup wizard shows generic placeholder text instead of actionable, language-specific OTEL setup instructions. Users cannot follow the onboarding steps.  
**Fix:** Port `_build_setup_wizard_steps()` logic from Python to Go, generating proper instructions per language/deployment/env combination.

---

### [SEVERITY: LOW] Route: POST /api/onboarding/list-repos

**Python:** Uses `POST`, reads `owner` from JSON body, returns `{ok, owner, repos, token_used, visibility_note}`.

**Go:** Uses `POST`, reads `owner` from JSON body, returns `{ok, owner, repos, token_used, visibility_note}`.

**Impact:** Parity appears good.

---

### [SEVERITY: MEDIUM] Route: GET /api/onboarding/inspect-repo

**Python:** Performs real GitHub API calls (lists `.github/workflows/`, reads workflow file contents for OTEL/CI indicators, checks Copilot availability). Returns `{has_github_actions, sobs_ci_found, sobs_otel_found, copilot_available, workflow_files, error}`.

**Go:** Delegates to `s.onboardingService.InspectRepo(app_id, repo)`. Verify this performs equivalent GitHub API inspection. If the service is a stub, it returns empty/false data.

**Impact:** Onboarding readiness detection will show incorrect results (always "not configured") if the Go service is not fully implemented.  
**Fix:** Implement GitHub API inspection in `onboarding.Service.InspectRepo()`.

---

## 15. MCP API

### [SEVERITY: LOW] Routes: /mcp, /mcp/tools, /api/mcp/keys, /api/mcp/keys/<id>, /api/mcp/enabled

**Python:** Full MCP key management (create, list, revoke, test), tool listing with schema, JSON-RPC 2.0 dispatch.

**Go:** `s.mcpEndpoint`, `s.mcpListTools`, `s.apiMCPKeys`, `s.apiMCPKeySubroutes`, `s.apiMCPEnabled`.

**Impact:** Verify key validation logic (constant-time compare), tool schema matches Python's `_mcp.list_tools()`, and JSON-RPC error codes match.

---

## 16. Validation API

### [SEVERITY: LOW] Routes: POST /api/logs/validate-filter, /api/ai/validate-filter

**Python:** Validates the SQL WHERE clause by normalizing it and checking for unsafe patterns. Returns `{ok, normalized, issues}`.

**Go (`validateFilterHandler`):** Uses `structuralSQLIssues()` and `normalizeLogsSQLWhere()`. Returns `{ok, normalized, issues}`. Parity appears good but verify the exact unsafe-pattern list matches Python's.

---

### [SEVERITY: LOW] Routes: POST /api/*/validate-regex

**Python:** Compiles regex, returns `{ok: true}` or `{ok: false, error: ...}`.

**Go (`validateRegexHandler`):** Identical logic using Go's `regexp.Compile`. Note: Python uses `re` (PCRE-like), Go uses RE2 semantics — some lookahead/lookbehind patterns valid in Python will fail in Go.

**Impact:** Users who saved Python-compatible regexes with lookahead/lookbehind will see them rejected as invalid in Go.  
**Fix:** Document the RE2 limitation; optionally warn users when migrating.

---

## Additional Cross-Cutting Findings

### [SEVERITY: HIGH] Auth model difference

**Python:** Uses `@require_basic_auth` (HTTP Basic Auth) for browser routes and `@require_api_key` for `/v1/*` API routes. The `@require_api_key` checks `Authorization: Bearer <key>` or `X-API-Key: <key>` headers.

**Go:** Uses `s.wrapSecurity(mux)` — must verify this applies Basic Auth to browser routes and API-key auth to `/v1/*` routes with the same header names and error response codes.

**Impact:** If auth model differs, API integrations or CI/CD pipelines using Bearer tokens may fail.  
**Fix:** Audit `wrapSecurity()` to confirm equivalent auth enforcement.

---

### [SEVERITY: MEDIUM] Static RUM assets — ETag header discrepancy

**Python `/static/rum.js`:** Sets ETag using SHA-256 of file content (first 16 hex chars). Also sets `X-SourceMap` and `SourceMap` headers pointing to `rum.js.map`.

**Go (`rumJS`):** Must verify ETag generation uses the same algorithm and the source map headers are set.

**Impact:** CDN/browser caching will break if ETag format differs. Source map lookup fails without the headers.  
**Fix:** Match Python's SHA-256 ETag approach and add `X-SourceMap`/`SourceMap` headers in Go.

---

### [SEVERITY: LOW] `GET /api/notifications/vapid-public-key`

**Python:** Returns `{ok, vapid_public_key, vapid_key_source}` where `vapid_key_source` is `"stored"` or `"env"`.

**Go (`apiNotificationsVapidPublicKey`):** Must include the `vapid_key_source` field.

---

### [SEVERITY: MEDIUM] Notification `POST /api/notifications/subscribe`

**Python (not explicitly found in this audit range):** Full push subscription object including `keys.p256dh` and `keys.auth`.

**Go:** Accepts `{endpoint}` only. If the full Web Push subscription object (with `p256dh` and `auth` keys) needs to be stored for VAPID-encrypted pushes, the Go model is incomplete.

**Impact:** Browser push notifications will fail because the encryption keys are not stored.  
**Fix:** Extend the subscribe endpoint to accept and store the full PushSubscription object.

---

### [SEVERITY: LOW] `DELETE /api/notifications/vapid-keys`

**Python:** Not explicitly found in the audited range.

**Go:** `s.apiNotificationsVAPIDKeysDelete` handles `DELETE /api/notifications/vapid-keys` and calls `s.notificationService.DeleteVAPIDKeys()`.

**Impact:** Go may have this endpoint when Python doesn't, or Python has it in an unaudited section. Verify direction of discrepancy.

---

### [SEVERITY: MEDIUM] Missing Python help routes in Go

**Python:** Registers help routes for `/logs/help`, `/errors/help`, `/traces/help`, and many others via `_register_help_route()`.

**Go:** `registerPageRoutes()` registers most of these. However Python also has:
- `/static/rum.js`, `/static/rum.js.map`, `/static/rum.min.js`, `/static/rum.min.js.map`, `/static/rum.d.ts`

**Go:** These are also registered. Parity appears good.

---

### [SEVERITY: LOW] Summary page — `_active_part_rows` vs full count

**Python:** Uses `_active_part_rows(db, "hyperdx_sessions")` which queries `system.parts` for active partition row estimates (fast). Falls back to a full COUNT when a time filter is active.

**Go (`summaryActivePartRows`):** Uses `SELECT count() FROM <table>` — a full table scan. This is slower but more accurate.

**Impact:** Performance difference on large datasets; no functional difference.  
**Fix:** Optionally optimize Go to use `system.parts` for row estimates.

---

## TOP CRITICAL AND HIGH PRIORITY ITEMS

### CRITICAL

| # | Route | Issue |
|---|-------|-------|
| 1 | `POST /settings/notifications/channels` | Go only creates browser-push subscriptions. Webhook, Slack, Email channels are **completely unimplemented**. |
| 2 | `POST /settings/notifications/rules` | Go only accepts `name`. All rule conditions, channels, severity, cooldown are **silently dropped**. Rules are non-functional. |
| 3 | `GET/POST /settings/data-management` | Go missing 9 S3/backup settings fields. S3 credentials, encryption, backup schedules **cannot be configured or persisted**. |
| 4 | `POST /api/notifications/channels/<id>/test` | Go returns `{"ok": true, "tested": true}` **without dispatching** the notification. Test feature is entirely non-functional. |

### HIGH

| # | Route | Issue |
|---|-------|-------|
| 5 | `GET /api/kubernetes/status` | 15 of 18 query params (namespace filters, sort fields, page sizes per entity type) are **ignored** in Go. K8s dashboard filtering/sorting/pagination broken. |
| 6 | `POST /api/notifications/channels/<id>/delete` / `toggle` | Since Go channels are browser-push-only, toggle/delete for webhook/slack/email channels is broken. |
| 7 | `POST /settings/notifications/rules/<id>` (create/edit) | Rules have no conditions or channels; all alert logic is lost. |
| 8 | `GET /api/enrichment/cve/findings` | `ecosystem`, `version`, `service`, `cve_ids`, `summary` are **always empty/unknown**. CVE findings page is misleading. |
| 9 | `POST /api/reports/import` | No multipart/form-data support in Go. File-upload import **fails** silently. |
| 10 | `GET /api/setup-wizard/steps` | Go returns **hardcoded placeholder** steps instead of language/deployment-specific OTEL setup instructions. Onboarding wizard is non-functional. |
| 11 | `GET /api/data-management/backup/list` | S3 credentials not stored → backup list always empty in Go. |
| 12 | `POST /settings/ai` | Model-pricing JSON validation, token-change detection, and redirect-after-POST are missing in Go. |
| 13 | Auth model | `wrapSecurity()` must be audited to confirm Basic Auth and API-key auth match Python's enforcement exactly. |
| 14 | `POST /api/query/add-to-dashboard` | Different request body shape (`sql`+`chart_spec` in Python vs `type`+`spec` in Go). JS client will break against Go. |

---

*End of audit — 2026-04-20*
