# DONE Endpoint Parity Re-Review (Python vs Go)

Audit date: 2026-04-20

Scope: All DONE endpoints listed in docs/PYTHON_SOURCE_ROUTE_CHECKLIST.md (212 total).

Approach: One-by-one static parity review against app.py decorators/help-route registrations and Go route registrations/handlers in internal/web, with Go web test validation after parity-fix commit 382dc20.

## Resolution Update

- Previous high-confidence differences identified in the earlier report have been addressed in commit 382dc20.
- Re-review result: no remaining high-confidence functional differences for DONE endpoints.

- Total DONE endpoints reviewed: 212
- Endpoints with noted functional differences: 0

## Full Endpoint-By-Endpoint Matrix

### Root And Ingest

| Method | Endpoint | Python Reference | Go Reference | Result | Note |
|---|---|---|---|---|---|
| GET | / | app.py:10592 | internal/web/server.go:124 (s.root) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /summary/help | app.py:21016 | internal/web/page_routes.go:70 (s.summaryHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /v1/logs | app.py:9432 | internal/web/server.go:123 (s.root -> ingest switch) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /v1/rum/assets | app.py:9462 | internal/web/server.go:218 (s.v1RUMAssets) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /v1/rum/assets/<asset_id> | app.py:9524 | internal/web/server.go:219 (s.v1RUMAssetByID, route /v1/rum/assets/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /v1/rum/client-token | app.py:9554 | internal/web/server.go:220 (s.v1RUMClientToken) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /v1/traces | app.py:9597 | internal/web/server.go:123 (s.root -> ingest switch) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /v1/metrics | app.py:9655 | internal/web/server.go:123 (s.root -> ingest switch) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /v1/rum | app.py:9765 | internal/web/server.go:123 (s.root -> ingest switch) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /v1/ai | app.py:9909 | internal/web/server.go:123 (s.root -> ingest switch) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /v1/errors | app.py:10001 | internal/web/server.go:123 (s.root -> ingest switch) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
### Apps And Releases

| Method | Endpoint | Python Reference | Go Reference | Result | Note |
|---|---|---|---|---|---|
| GET | /v1/apps | app.py:10054 | internal/web/server.go:129 (s.v1Apps) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /v1/apps | app.py:10066 | internal/web/server.go:129 (s.v1Apps) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /v1/apps/<app_id> | app.py:10103 | internal/web/server.go:130 (s.v1AppsSubroutes, route /v1/apps/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| PATCH | /v1/apps/<app_id> | app.py:10113 | internal/web/server.go:130 (s.v1AppsSubroutes, route /v1/apps/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /v1/apps/<app_id>/releases | app.py:10155 | internal/web/server.go:130 (s.v1AppsSubroutes, route /v1/apps/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /v1/apps/<app_id>/releases | app.py:10172 | internal/web/server.go:130 (s.v1AppsSubroutes, route /v1/apps/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /v1/releases/<release_id> | app.py:10202 | internal/web/server.go:131 (s.v1ReleasesSubroutes, route /v1/releases/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /v1/releases/<release_id>/artifacts | app.py:10221 | internal/web/server.go:131 (s.v1ReleasesSubroutes, route /v1/releases/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /v1/releases/<release_id>/artifacts/meta | app.py:10238 | internal/web/server.go:130 (s.v1ReleasesSubroutes) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
### Logs Errors Metrics Traces

| Method | Endpoint | Python Reference | Go Reference | Result | Note |
|---|---|---|---|---|---|
| GET | /logs | app.py:10989 | internal/web/page_routes.go:67 (s.pageLogsHandler) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /logs/help | app.py:21007 | internal/web/page_routes.go:71 (s.logsHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /metrics | app.py:13175 | internal/web/server.go:221 (s.metricsPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /metrics/help | app.py:21012 | internal/web/page_routes.go:100 (s.metricsHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /metrics/rules | app.py:13348 | internal/web/server.go:203 (s.metricsRules) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /metrics/help/rules | app.py:20990 | internal/web/page_routes.go:101 (s.metricsRulesHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /metrics/help/rules/auto | app.py:20991 | internal/web/page_routes.go:102 (s.metricsRulesAutoHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /metrics/rules | app.py:13348 | internal/web/server.go:203 (s.metricsRules) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /metrics/rules/auto | app.py:13466 | internal/web/server.go:204 (s.metricsRulesAuto) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /metrics/rules/dashboard/auto | app.py:13593 | internal/web/server.go:205 (s.metricsRulesDashboardAuto) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /metrics/rules/<rule_id>/delete | app.py:13710 | internal/web/server.go:206 (s.metricsRulesSubroutes, route /metrics/rules/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /metrics/anomaly | app.py:13755 | internal/web/server.go:207 (s.metricsAnomalyPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /metrics/help/anomaly | app.py:21013 | internal/web/page_routes.go:103 (s.metricsAnomalyHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /errors | app.py:13954 | internal/web/page_routes.go:68 (s.pageErrorsHandler) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /errors/help | app.py:21004 | internal/web/page_routes.go:72 (s.errorsHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /errors/<string:error_id>/resolve | app.py:14332 | internal/web/server.go:222 (s.errorsResolve, route /errors/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /traces | app.py:15055 | internal/web/page_routes.go:69 (s.pageTracesHandler) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /traces/help | app.py:21008 | internal/web/page_routes.go:73 (s.tracesHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/traces/span/<span_id> | app.py:15438 | internal/web/server.go:223 (s.apiTraceSpan, route /api/traces/span/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /incident | app.py:15528 | internal/web/page_routes.go:74 (s.incidentPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /incident/help | app.py:21018 | internal/web/page_routes.go:75 (s.incidentHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
### RUM And Web Traffic

| Method | Endpoint | Python Reference | Go Reference | Result | Note |
|---|---|---|---|---|---|
| GET | /rum | app.py:16818 | internal/web/page_routes.go:76 (s.rumPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /rum/help | app.py:21009 | internal/web/page_routes.go:77 (s.rumHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /web-traffic | app.py:17174 | internal/web/page_routes.go:78 (s.webTrafficPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /web-traffic/help | app.py:21003 | internal/web/page_routes.go:79 (s.webTrafficHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/web-traffic/geo | app.py:17220 | internal/web/server.go:178 (s.apiWebTrafficGeo) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/web-traffic/browsers | app.py:17277 | internal/web/server.go:179 (s.apiWebTrafficBrowsers) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/web-traffic/os | app.py:17304 | internal/web/server.go:180 (s.apiWebTrafficOS) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/web-traffic/timezones | app.py:17331 | internal/web/server.go:181 (s.apiWebTrafficTimezones) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/web-traffic/languages | app.py:17351 | internal/web/server.go:182 (s.apiWebTrafficLanguages) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/web-traffic/devices | app.py:17371 | internal/web/server.go:183 (s.apiWebTrafficDevices) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
### Enrichment And Work Items

| Method | Endpoint | Python Reference | Go Reference | Result | Note |
|---|---|---|---|---|---|
| GET | /api/enrichment/libraries | app.py:17391 | internal/web/server.go:184 (s.apiEnrichmentLibraries) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/enrichment/github/repo-health | app.py:17455 | internal/web/server.go:185 (s.apiEnrichmentGitHubRepoHealth) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /enrichment/cve | app.py:17607 | internal/web/server.go:189 (s.enrichmentCVEPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /cve/help | app.py:21011 | internal/web/page_routes.go:107 (s.cveHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/enrichment/cve/findings | app.py:17723 | internal/web/server.go:186 (s.apiEnrichmentCVEFindings) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/enrichment/cve/findings/<osv_id>/disposition | app.py:17786 | internal/web/server.go:187 (s.apiEnrichmentCVEFindingsSubroutes, route /api/enrichment/cve/findings/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/enrichment/cve/scan | app.py:17845 | internal/web/server.go:188 (s.apiEnrichmentCVEScan) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /work-items | app.py:17864 | internal/web/page_routes.go:80 (s.workItemsPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /work-items/help | app.py:21017 | internal/web/page_routes.go:81 (s.workItemsHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/work-items | app.py:17996 | internal/web/server.go:155 (s.apiWorkItems) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
### AI And Dashboards

| Method | Endpoint | Python Reference | Go Reference | Result | Note |
|---|---|---|---|---|---|
| GET | /ai | app.py:18134 | internal/web/page_routes.go:82 (s.aiPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /ai/help | app.py:21010 | internal/web/page_routes.go:83 (s.aiHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/ai/span-attributes | app.py:18506 | internal/web/server.go:157 (s.apiAISpanAttributes) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/ai/conversation | app.py:18551 | internal/web/server.go:156 (s.apiAIConversation) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/ai/export | app.py:18626 | internal/web/server.go:158 (s.apiAIExport) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/dashboards/list | app.py:20825 | internal/web/server.go:136 (s.apiDashboardsList) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/query/add-to-dashboard | app.py:20834 | internal/web/server.go:150 (s.apiQueryAddToDashboard) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /dashboards | app.py:20915 | internal/web/server.go:147 (s.dashboardsRoot) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /dashboards/help/chart-editor | app.py:20989 | internal/web/page_routes.go:105 (s.chartEditorHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /dashboards/new | app.py:20923 | internal/web/server.go:148 (s.dashboardsNew) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /dashboards | app.py:20915 | internal/web/server.go:147 (s.dashboardsRoot) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /dashboards/<dashboard_id> | app.py:20949 | internal/web/server.go:149 (s.dashboardsSubroutes, route /dashboards/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /dashboards/<dashboard_id>/delete | app.py:21052 | internal/web/server.go:149 (s.dashboardsSubroutes, route /dashboards/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /dashboards/<dashboard_id>/charts | app.py:21097 | internal/web/server.go:149 (s.dashboardsSubroutes, route /dashboards/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /dashboards/<dashboard_id>/charts/<chart_id>/edit | app.py:21154 | internal/web/server.go:148 (s.dashboardsSubroutes) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /dashboards/<dashboard_id>/charts/<chart_id>/clone | app.py:21197 | internal/web/server.go:148 (s.dashboardsSubroutes) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /dashboards/<dashboard_id>/charts/<chart_id>/delete | app.py:21241 | internal/web/server.go:148 (s.dashboardsSubroutes) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/dashboards/query | app.py:21275 | internal/web/server.go:137 (s.apiDashboardsQuery) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/dashboards/spec/templates | app.py:21299 | internal/web/server.go:138 (s.apiDashboardsSpecTemplates) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/dashboards/spec/options | app.py:21319 | internal/web/server.go:139 (s.apiDashboardsSpecOptions) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/dashboards/spec/compile | app.py:21395 | internal/web/server.go:140 (s.apiDashboardsSpecCompile) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/dashboards/spec/dry-run | app.py:21410 | internal/web/server.go:141 (s.apiDashboardsSpecDryRun) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/dashboards/spec/validate | app.py:21454 | internal/web/server.go:142 (s.apiDashboardsSpecValidate) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/dashboards/spec/render | app.py:21489 | internal/web/server.go:143 (s.apiDashboardsSpecRender) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/dashboards/render | app.py:21539 | internal/web/server.go:144 (s.apiDashboardsRender) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/dashboards/spec/ai-build | app.py:21766 | internal/web/server.go:145 (s.apiDashboardsSpecAIBuild) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/dashboards/<dashboard_id>/charts/<chart_id>/export | app.py:21939 | internal/web/server.go:145 (s.apiDashboardsChartSubroutes) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/dashboards/<dashboard_id>/charts/import | app.py:21970 | internal/web/server.go:145 (s.apiDashboardsChartSubroutes) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/metrics/anomaly | app.py:22043 | internal/web/server.go:208 (s.apiMetricsAnomaly) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
### Reports

| Method | Endpoint | Python Reference | Go Reference | Result | Note |
|---|---|---|---|---|---|
| GET | /reports | app.py:22194 | internal/web/page_routes.go:84 (s.reportsPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /reports/help | app.py:21015 | internal/web/page_routes.go:85 (s.reportsHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /reports/<report_id>/delete | app.py:22202 | internal/web/server.go:154 (s.reportsPageDelete, route /reports/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/reports | app.py:22230 | internal/web/server.go:132 (s.apiReports) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/reports | app.py:22239 | internal/web/server.go:132 (s.apiReports) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| DELETE | /api/reports/<report_id> | app.py:22277 | internal/web/server.go:133 (s.apiReportsSubroutes, route /api/reports/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/reports/export | app.py:22313 | internal/web/server.go:134 (s.apiReportsExport) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/reports/import | app.py:22360 | internal/web/server.go:135 (s.apiReportsImport) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
### Static And Core Settings

| Method | Endpoint | Python Reference | Go Reference | Result | Note |
|---|---|---|---|---|---|
| GET | /static/rum.js | app.py:22537 | internal/web/server.go:259 (s.rumJS) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /static/rum.js.map | app.py:22548 | internal/web/server.go:260 (s.rumJSMap) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /static/rum.min.js | app.py:22557 | internal/web/server.go:261 (s.rumMinJS) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /static/rum.min.js.map | app.py:22566 | internal/web/server.go:262 (s.rumMinJSMap) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /static/rum.d.ts | app.py:22572 | internal/web/server.go:263 (s.rumDTS) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /settings | app.py:22581 | internal/web/page_routes.go:86 (s.settingsPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /settings/help | app.py:20994 | internal/web/page_routes.go:87 (s.settingsHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /settings/masking | app.py:22611 | internal/web/server.go:238 (s.settingsMaskingPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /settings/help/masking | app.py:20995 | internal/web/page_routes.go:93 (s.settingsMaskingHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/masking/keys | app.py:22629 | internal/web/server.go:239 (s.settingsMaskingKeysCreate) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/masking/keys/delete | app.py:22649 | internal/web/server.go:240 (s.settingsMaskingKeysDelete) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/masking/patterns | app.py:22666 | internal/web/server.go:241 (s.settingsMaskingPatternsCreate) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/masking/patterns/delete | app.py:22689 | internal/web/server.go:242 (s.settingsMaskingPatternsDelete) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/masking/output | app.py:22712 | internal/web/server.go:243 (s.settingsMaskingOutput) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/masking/sql-output | app.py:22731 | internal/web/server.go:244 (s.settingsMaskingSQLOutput) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/settings/masking/preview | app.py:22752 | internal/web/server.go:245 (s.apiSettingsMaskingPreview) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/settings/masking/rules | app.py:22761 | internal/web/server.go:246 (s.apiSettingsMaskingRules) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
### Tags And Validation Helpers

| Method | Endpoint | Python Reference | Go Reference | Result | Note |
|---|---|---|---|---|---|
| GET | /settings/tags | app.py:22781 | internal/web/server.go:247 (s.settingsTags) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /settings/help/tags | app.py:20999 | internal/web/page_routes.go:96 (s.settingsTagsHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/settings/tags/condition-suggestions | app.py:22810 | internal/web/server.go:251 (s.apiSettingsTagsConditionSuggestions) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/tags/auto | app.py:22856 | internal/web/server.go:248 (s.settingsTagsAuto) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/tags | app.py:22781 | internal/web/server.go:247 (s.settingsTags) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/tags/<rule_id>/delete | app.py:23092 | internal/web/server.go:249 (s.settingsTagsSubroutes, route /settings/tags/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/tags/<record_type>/<record_id> | app.py:23127 | internal/web/server.go:252 (s.apiTagsRecord, route /api/tags/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/tags/<record_type>/<record_id> | app.py:23135 | internal/web/server.go:252 (s.apiTagsRecord, route /api/tags/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| DELETE | /api/tags/<record_type>/<record_id>/<tag_key> | app.py:23163 | internal/web/server.go:252 (s.apiTagsRecord, route /api/tags/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/logs/field-hints | app.py:23210 | internal/web/server.go:209 (s.apiLogsFieldHints) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/logs/validate-filter | app.py:23287 | internal/web/server.go:211 (s.apiLogsValidateFilter) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/logs/validate-regex | app.py:23457 | internal/web/server.go:213 (s.apiLogsValidateRegex) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/errors/validate-regex | app.py:23516 | internal/web/server.go:214 (s.apiErrorsValidateRegex) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/traces/validate-regex | app.py:23565 | internal/web/server.go:215 (s.apiTracesValidateRegex) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/metrics/validate-regex | app.py:23618 | internal/web/server.go:216 (s.apiMetricsValidateRegex) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/rum/validate-regex | app.py:23679 | internal/web/server.go:217 (s.apiRUMValidateRegex) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/ai/field-hints | app.py:23732 | internal/web/server.go:210 (s.apiAIFieldHints) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/ai/validate-filter | app.py:23910 | internal/web/server.go:212 (s.apiAIValidateFilter) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /tail | app.py:23965 | internal/web/server.go:257 (s.tail) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
### Notifications And Health

| Method | Endpoint | Python Reference | Go Reference | Result | Note |
|---|---|---|---|---|---|
| GET | /settings/notifications | app.py:25219 | internal/web/page_routes.go:97 (s.settingsNotificationsPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /settings/help/notifications | app.py:20998 | internal/web/page_routes.go:94 (s.settingsNotificationsHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/notifications/channels | app.py:25251 | internal/web/server.go:253 (s.settingsNotificationsChannelsCreate) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/notifications/channels/<channel_id>/delete | app.py:25327 | internal/web/server.go:254 (s.settingsNotificationsChannelActions, route /settings/notifications/channels/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/notifications/channels/<channel_id>/toggle | app.py:25357 | internal/web/server.go:254 (s.settingsNotificationsChannelActions, route /settings/notifications/channels/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/notifications/channels/<channel_id>/test | app.py:25391 | internal/web/server.go:229 (s.apiNotificationsChannelSubroutes, route /api/notifications/channels/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/notifications/rules | app.py:25427 | internal/web/server.go:255 (s.settingsNotificationsRulesCreate) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/notifications/rules/<rule_id>/toggle | app.py:25603 | internal/web/server.go:256 (s.settingsNotificationsRulesActions, route /settings/notifications/rules/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/notifications/rules/<rule_id>/delete | app.py:25642 | internal/web/server.go:256 (s.settingsNotificationsRulesActions, route /settings/notifications/rules/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/notifications/rules/auto-generate | app.py:25767 | internal/web/server.go:230 (s.apiNotificationsRulesAutoGenerate) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/notifications/check | app.py:25850 | internal/web/server.go:224 (s.notificationsCheck) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/notifications/vapid-public-key | app.py:25961 | internal/web/server.go:225 (s.apiNotificationsVapidPublicKey) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /service-worker.js | app.py:25971 | internal/web/server.go:258 (s.serviceWorker) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/notifications/subscribe | app.py:26006 | internal/web/server.go:226 (s.apiNotificationsSubscribe) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/notifications/vapid-keygen | app.py:26049 | internal/web/server.go:227 (s.apiNotificationsVAPIDKeygen) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| DELETE | /api/notifications/vapid-keys | app.py:26085 | internal/web/server.go:228 (s.apiNotificationsVAPIDKeysDelete) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /health | app.py:26114 | internal/web/server.go:125 (s.healthz) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /health/db | app.py:26119 | internal/web/server.go:126 (s.readyz) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
### AI Settings And Repositories

| Method | Endpoint | Python Reference | Go Reference | Result | Note |
|---|---|---|---|---|---|
| GET | /settings/ai | app.py:26155 | internal/web/server.go:234 (s.settingsAI) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /settings/help/ai | app.py:20996 | internal/web/page_routes.go:88 (s.settingsAIHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/ai | app.py:26186 | internal/web/server.go:234 (s.settingsAI) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /settings/enrichment | app.py:26268 | internal/web/server.go:235 (s.settingsEnrichment) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /settings/help/enrichment | app.py:21000 | internal/web/page_routes.go:91 (s.settingsEnrichmentHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/enrichment | app.py:26287 | internal/web/server.go:235 (s.settingsEnrichment) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /settings/repositories | app.py:26314 | internal/web/server.go:231 (s.settingsRepositories) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /settings/help/repositories | app.py:21001 | internal/web/page_routes.go:95 (s.settingsRepositoriesHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/repositories | app.py:26398 | internal/web/server.go:231 (s.settingsRepositories) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/repositories/github-token/validate | app.py:26465 | internal/web/server.go:232 (s.settingsRepositoriesValidateToken) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/repositories/<app_id>/realtime-mode | app.py:26484 | internal/web/server.go:233 (s.settingsRepositoriesSubroutes, route /settings/repositories/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/repositories/<app_id>/ci-ingest-key/rotate | app.py:26503 | internal/web/server.go:233 (s.settingsRepositoriesSubroutes, route /settings/repositories/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/repositories/<app_id>/ci-ingest-key/revoke | app.py:26531 | internal/web/server.go:233 (s.settingsRepositoriesSubroutes, route /settings/repositories/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/repositories/<app_id> | app.py:26544 | internal/web/server.go:233 (s.settingsRepositoriesSubroutes, route /settings/repositories/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/repositories/<app_id>/releases | app.py:26591 | internal/web/server.go:233 (s.settingsRepositoriesSubroutes, route /settings/repositories/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/repositories/<app_id>/delete | app.py:26626 | internal/web/server.go:233 (s.settingsRepositoriesSubroutes, route /settings/repositories/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
### Agents And AI Helper

| Method | Endpoint | Python Reference | Go Reference | Result | Note |
|---|---|---|---|---|---|
| GET | /settings/agents | app.py:26683 | internal/web/server.go:236 (s.settingsAgents) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /settings/help/agents | app.py:20997 | internal/web/page_routes.go:89 (s.settingsAgentsHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/agents | app.py:26703 | internal/web/server.go:236 (s.settingsAgents) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/agents/<rule_id>/delete | app.py:26756 | internal/web/server.go:237 (s.settingsAgentsSubroutes, route /settings/agents/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/ai/helper/capabilities | app.py:26921 | internal/web/server.go:159 (s.apiAIHelperCapabilities) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/ai/helper/actions/manifest | app.py:26944 | internal/web/server.go:160 (s.apiAIHelperActionsManifest) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/ai/helper/chats | app.py:26957 | internal/web/server.go:161 (s.apiAIHelperChats) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/ai/helper/chats/<chat_id> | app.py:27016 | internal/web/server.go:162 (s.apiAIHelperChatByID, route /api/ai/helper/chats/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/ai/helper/feedback | app.py:27088 | internal/web/server.go:163 (s.apiAIHelperFeedback) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/ai/helper | app.py:27116 | internal/web/server.go:164 (s.apiAIHelper) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/ai/helper/actions/execute | app.py:27909 | internal/web/server.go:165 (s.apiAIHelperActionsExecute) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/issues/raise | app.py:28055 | internal/web/server.go:166 (s.apiIssuesRaise) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/agent/runs | app.py:28167 | internal/web/server.go:167 (s.apiAgentRuns) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/agent/runs | app.py:28179 | internal/web/server.go:167 (s.apiAgentRuns) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/agent/runs/<run_id>/dismiss | app.py:28240 | internal/web/server.go:168 (s.apiAgentRunsSubroutes, route /api/agent/runs/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
### Query Explorer Kubernetes And Data Management

| Method | Endpoint | Python Reference | Go Reference | Result | Note |
|---|---|---|---|---|---|
| GET | /query | app.py:29700 | internal/web/page_routes.go:98 (s.queryPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /query/help | app.py:21014 | internal/web/page_routes.go:99 (s.queryHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/query/ask | app.py:29711 | internal/web/server.go:169 (s.apiQueryAsk) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/query/run | app.py:30019 | internal/web/server.go:170 (s.apiQueryRun) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/query/refine-chart | app.py:30278 | internal/web/server.go:171 (s.apiQueryRefineChart) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/query/schema | app.py:30382 | internal/web/server.go:172 (s.apiQuerySchema) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /table-explorer | app.py:30402 | internal/web/server.go:173 (s.tableExplorerPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /table-explorer/help | app.py:21005 | internal/web/server.go:174 (s.tableExplorerHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/table-explorer/tables | app.py:30414 | internal/web/server.go:175 (s.apiTableExplorerTables) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/table-explorer/table/<name> | app.py:30456 | internal/web/server.go:176 (s.apiTableExplorerTable, route /api/table-explorer/table/) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/chart-types | app.py:30498 | internal/web/server.go:177 (s.apiChartTypes) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /settings/kubernetes | app.py:31210 | internal/web/server.go:190 (s.settingsKubernetes) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /kubernetes/help | app.py:20992 | internal/web/page_routes.go:106 (s.kubernetesHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /settings/help/kubernetes | app.py:21002 | internal/web/page_routes.go:92 (s.settingsKubernetesHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/kubernetes | app.py:31226 | internal/web/server.go:190 (s.settingsKubernetes) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /kubernetes | app.py:31242 | internal/web/server.go:191 (s.kubernetesPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/kubernetes/status | app.py:31254 | internal/web/server.go:192 (s.apiKubernetesStatus) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /settings/data-management | app.py:31582 | internal/web/server.go:193 (s.settingsDataManagement) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /settings/help/data-management | app.py:20993 | internal/web/page_routes.go:90 (s.settingsDataManagementHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /settings/data-management | app.py:31606 | internal/web/server.go:193 (s.settingsDataManagement) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/data-management/backup/list | app.py:31644 | internal/web/server.go:194 (s.apiDataManagementBackupList) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/data-management/backup/run | app.py:31654 | internal/web/server.go:195 (s.apiDataManagementBackupRun) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/data-management/restore | app.py:31670 | internal/web/server.go:196 (s.apiDataManagementRestore) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/setup-wizard/steps | app.py:32145 | internal/web/server.go:197 (s.apiSetupWizardSteps) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /setup/help/playbooks | app.py:21006 | internal/web/page_routes.go:104 (s.setupPlaybooksHelpPage) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/onboarding/create-repo | app.py:32758 | internal/web/server.go:198 (s.apiOnboardingCreateRepo) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/onboarding/import-repo | app.py:32832 | internal/web/server.go:199 (s.apiOnboardingImportRepo) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| POST | /api/onboarding/list-repos | app.py:32897 | internal/web/server.go:200 (s.apiOnboardingListRepos) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
| GET | /api/onboarding/inspect-repo | app.py:32977 | internal/web/server.go:201 (s.apiOnboardingInspectRepo) | MATCH | No functional difference identified in this re-review (path/method mapping and updated parity tests align). |
