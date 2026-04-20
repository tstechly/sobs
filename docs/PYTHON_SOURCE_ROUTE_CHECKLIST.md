# Python Source Route Checklist

This list is derived only from `app.py` and `mcp.py`.

Status legend:
- `TODO`
- `IN PROGRESS`
- `DONE`
- `BLOCKED`

## Root And Ingest

- `DONE | GET | / | app.py`
- `DONE | GET | /summary/help | app.py`
- `DONE | POST | /v1/logs | app.py`
- `DONE | POST | /v1/rum/assets | app.py`
- `DONE | GET | /v1/rum/assets/<asset_id> | app.py`
- `DONE | POST | /v1/rum/client-token | app.py`
- `DONE | POST | /v1/traces | app.py`
- `DONE | POST | /v1/metrics | app.py`
- `DONE | POST | /v1/rum | app.py`
- `DONE | POST | /v1/ai | app.py`
- `DONE | POST | /v1/errors | app.py`

## Apps And Releases

- `DONE | GET | /v1/apps | app.py`
- `DONE | POST | /v1/apps | app.py`
- `DONE | GET | /v1/apps/<app_id> | app.py`
- `DONE | PATCH | /v1/apps/<app_id> | app.py`
- `DONE | GET | /v1/apps/<app_id>/releases | app.py`
- `DONE | POST | /v1/apps/<app_id>/releases | app.py`
- `DONE | GET | /v1/releases/<release_id> | app.py`
- `DONE | GET | /v1/releases/<release_id>/artifacts | app.py`
- `DONE | POST | /v1/releases/<release_id>/artifacts/meta | app.py`

## Logs Errors Metrics Traces

- `DONE | GET | /logs | app.py`
- `DONE | GET | /logs/help | app.py`
- `DONE | GET | /metrics | app.py`
- `DONE | GET | /metrics/help | app.py`
- `DONE | GET | /metrics/rules | app.py`
- `DONE | GET | /metrics/help/rules | app.py`
- `DONE | GET | /metrics/help/rules/auto | app.py`
- `DONE | POST | /metrics/rules | app.py`
- `DONE | POST | /metrics/rules/auto | app.py`
- `DONE | POST | /metrics/rules/dashboard/auto | app.py`
- `DONE | POST | /metrics/rules/<rule_id>/delete | app.py`
- `DONE | GET | /metrics/anomaly | app.py`
- `DONE | GET | /metrics/help/anomaly | app.py`
- `DONE | GET | /errors | app.py`
- `DONE | GET | /errors/help | app.py`
- `DONE | POST | /errors/<string:error_id>/resolve | app.py`
- `DONE | GET | /traces | app.py`
- `DONE | GET | /traces/help | app.py`
- `DONE | GET | /api/traces/span/<span_id> | app.py`
- `DONE | GET | /incident | app.py`
- `DONE | GET | /incident/help | app.py`

## RUM And Web Traffic

- `DONE | GET | /rum | app.py`
- `DONE | GET | /rum/help | app.py`
- `DONE | GET | /web-traffic | app.py`
- `DONE | GET | /web-traffic/help | app.py`
- `DONE | GET | /api/web-traffic/geo | app.py`
- `DONE | GET | /api/web-traffic/browsers | app.py`
- `DONE | GET | /api/web-traffic/os | app.py`
- `DONE | GET | /api/web-traffic/timezones | app.py`
- `DONE | GET | /api/web-traffic/languages | app.py`
- `DONE | GET | /api/web-traffic/devices | app.py`

## Enrichment And Work Items

- `DONE | GET | /api/enrichment/libraries | app.py`
- `DONE | GET | /api/enrichment/github/repo-health | app.py`
- `DONE | GET | /enrichment/cve | app.py`
- `DONE | GET | /cve/help | app.py`
- `DONE | GET | /api/enrichment/cve/findings | app.py`
- `DONE | POST | /api/enrichment/cve/findings/<osv_id>/disposition | app.py`
- `DONE | POST | /api/enrichment/cve/scan | app.py`
- `DONE | GET | /work-items | app.py`
- `DONE | GET | /work-items/help | app.py`
- `DONE | GET | /api/work-items | app.py`

## AI And Dashboards

- `DONE | GET | /ai | app.py`
- `DONE | GET | /ai/help | app.py`
- `DONE | GET | /api/ai/span-attributes | app.py`
- `DONE | GET | /api/ai/conversation | app.py`
- `DONE | GET | /api/ai/export | app.py`
- `DONE | GET | /api/dashboards/list | app.py`
- `DONE | POST | /api/query/add-to-dashboard | app.py`
- `DONE | GET | /dashboards | app.py`
- `DONE | GET | /dashboards/help/chart-editor | app.py`
- `DONE | GET | /dashboards/new | app.py`
- `DONE | POST | /dashboards | app.py`
- `DONE | GET | /dashboards/<dashboard_id> | app.py`
- `DONE | POST | /dashboards/<dashboard_id>/delete | app.py`
- `DONE | POST | /dashboards/<dashboard_id>/charts | app.py`
- `DONE | POST | /dashboards/<dashboard_id>/charts/<chart_id>/edit | app.py`
- `DONE | POST | /dashboards/<dashboard_id>/charts/<chart_id>/clone | app.py`
- `DONE | POST | /dashboards/<dashboard_id>/charts/<chart_id>/delete | app.py`
- `DONE | POST | /api/dashboards/query | app.py`
- `DONE | GET | /api/dashboards/spec/templates | app.py`
- `DONE | GET | /api/dashboards/spec/options | app.py`
- `DONE | POST | /api/dashboards/spec/compile | app.py`
- `DONE | POST | /api/dashboards/spec/dry-run | app.py`
- `DONE | POST | /api/dashboards/spec/validate | app.py`
- `DONE | POST | /api/dashboards/spec/render | app.py`
- `DONE | POST | /api/dashboards/render | app.py`
- `DONE | POST | /api/dashboards/spec/ai-build | app.py`
- `DONE | GET | /api/dashboards/<dashboard_id>/charts/<chart_id>/export | app.py`
- `DONE | POST | /api/dashboards/<dashboard_id>/charts/import | app.py`
- `DONE | GET | /api/metrics/anomaly | app.py`

## Reports

- `DONE | GET | /reports | app.py`
- `DONE | GET | /reports/help | app.py`
- `DONE | POST | /reports/<report_id>/delete | app.py`
- `DONE | GET | /api/reports | app.py`
- `DONE | POST | /api/reports | app.py`
- `DONE | DELETE | /api/reports/<report_id> | app.py`
- `DONE | GET | /api/reports/export | app.py`
- `DONE | POST | /api/reports/import | app.py`

## Static And Core Settings

- `DONE | GET | /static/rum.js | app.py`
- `DONE | GET | /static/rum.js.map | app.py`
- `DONE | GET | /static/rum.min.js | app.py`
- `DONE | GET | /static/rum.min.js.map | app.py`
- `DONE | GET | /static/rum.d.ts | app.py`
- `DONE | GET | /settings | app.py`
- `DONE | GET | /settings/help | app.py`
- `DONE | GET | /settings/masking | app.py`
- `DONE | GET | /settings/help/masking | app.py`
- `DONE | POST | /settings/masking/keys | app.py`
- `DONE | POST | /settings/masking/keys/delete | app.py`
- `DONE | POST | /settings/masking/patterns | app.py`
- `DONE | POST | /settings/masking/patterns/delete | app.py`
- `DONE | POST | /settings/masking/output | app.py`
- `DONE | POST | /settings/masking/sql-output | app.py`
- `DONE | POST | /api/settings/masking/preview | app.py`
- `DONE | GET | /api/settings/masking/rules | app.py`

## Tags And Validation Helpers

- `DONE | GET | /settings/tags | app.py`
- `DONE | GET | /settings/help/tags | app.py`
- `DONE | GET | /api/settings/tags/condition-suggestions | app.py`
- `DONE | POST | /settings/tags/auto | app.py`
- `DONE | POST | /settings/tags | app.py`
- `DONE | POST | /settings/tags/<rule_id>/delete | app.py`
- `DONE | GET | /api/tags/<record_type>/<record_id> | app.py`
- `DONE | POST | /api/tags/<record_type>/<record_id> | app.py`
- `DONE | DELETE | /api/tags/<record_type>/<record_id>/<tag_key> | app.py`
- `DONE | GET | /api/logs/field-hints | app.py`
- `DONE | POST | /api/logs/validate-filter | app.py`
- `DONE | POST | /api/logs/validate-regex | app.py`
- `DONE | POST | /api/errors/validate-regex | app.py`
- `DONE | POST | /api/traces/validate-regex | app.py`
- `DONE | POST | /api/metrics/validate-regex | app.py`
- `DONE | POST | /api/rum/validate-regex | app.py`
- `DONE | GET | /api/ai/field-hints | app.py`
- `DONE | POST | /api/ai/validate-filter | app.py`
- `DONE | GET | /tail | app.py`

## Notifications And Health

- `DONE | GET | /settings/notifications | app.py`
- `DONE | GET | /settings/help/notifications | app.py`
- `DONE | POST | /settings/notifications/channels | app.py`
- `DONE | POST | /settings/notifications/channels/<channel_id>/delete | app.py`
- `DONE | POST | /settings/notifications/channels/<channel_id>/toggle | app.py`
- `DONE | POST | /api/notifications/channels/<channel_id>/test | app.py`
- `DONE | POST | /settings/notifications/rules | app.py`
- `DONE | POST | /settings/notifications/rules/<rule_id>/toggle | app.py`
- `DONE | POST | /settings/notifications/rules/<rule_id>/delete | app.py`
- `DONE | POST | /api/notifications/rules/auto-generate | app.py`
- `DONE | POST | /api/notifications/check | app.py`
- `DONE | GET | /api/notifications/vapid-public-key | app.py`
- `DONE | GET | /service-worker.js | app.py`
- `DONE | POST | /api/notifications/subscribe | app.py`
- `DONE | POST | /api/notifications/vapid-keygen | app.py`
- `DONE | DELETE | /api/notifications/vapid-keys | app.py`
- `DONE | GET | /health | app.py`
- `DONE | GET | /health/db | app.py`

## AI Settings And Repositories

- `DONE | GET | /settings/ai | app.py`
- `DONE | GET | /settings/help/ai | app.py`
- `DONE | POST | /settings/ai | app.py`
- `DONE | GET | /settings/enrichment | app.py`
- `DONE | GET | /settings/help/enrichment | app.py`
- `DONE | POST | /settings/enrichment | app.py`
- `DONE | GET | /settings/repositories | app.py`
- `DONE | GET | /settings/help/repositories | app.py`
- `DONE | POST | /settings/repositories | app.py`
- `DONE | POST | /settings/repositories/github-token/validate | app.py`
- `DONE | POST | /settings/repositories/<app_id>/realtime-mode | app.py`
- `DONE | POST | /settings/repositories/<app_id>/ci-ingest-key/rotate | app.py`
- `DONE | POST | /settings/repositories/<app_id>/ci-ingest-key/revoke | app.py`
- `DONE | POST | /settings/repositories/<app_id> | app.py`
- `DONE | POST | /settings/repositories/<app_id>/releases | app.py`
- `DONE | POST | /settings/repositories/<app_id>/delete | app.py`

## Agents And AI Helper

- `DONE | GET | /settings/agents | app.py`
- `DONE | GET | /settings/help/agents | app.py`
- `DONE | POST | /settings/agents | app.py`
- `DONE | POST | /settings/agents/<rule_id>/delete | app.py`
- `DONE | GET | /api/ai/helper/capabilities | app.py`
- `DONE | GET | /api/ai/helper/actions/manifest | app.py`
- `DONE | GET | /api/ai/helper/chats | app.py`
- `DONE | GET | /api/ai/helper/chats/<chat_id> | app.py`
- `DONE | POST | /api/ai/helper/feedback | app.py`
- `DONE | POST | /api/ai/helper | app.py`
- `DONE | POST | /api/ai/helper/actions/execute | app.py`
- `DONE | POST | /api/issues/raise | app.py`
- `DONE | GET | /api/agent/runs | app.py`
- `DONE | POST | /api/agent/runs | app.py`
- `DONE | POST | /api/agent/runs/<run_id>/dismiss | app.py`

## Query Explorer Kubernetes And Data Management

- `DONE | GET | /query | app.py`
- `DONE | GET | /query/help | app.py`
- `DONE | POST | /api/query/ask | app.py`
- `DONE | POST | /api/query/run | app.py`
- `DONE | POST | /api/query/refine-chart | app.py`
- `DONE | GET | /api/query/schema | app.py`
- `DONE | GET | /table-explorer | app.py`
- `DONE | GET | /table-explorer/help | app.py`
- `DONE | GET | /api/table-explorer/tables | app.py`
- `DONE | GET | /api/table-explorer/table/<name> | app.py`
- `DONE | GET | /api/chart-types | app.py`
- `DONE | GET | /settings/kubernetes | app.py`
- `DONE | GET | /kubernetes/help | app.py`
- `DONE | GET | /settings/help/kubernetes | app.py`
- `DONE | POST | /settings/kubernetes | app.py`
- `DONE | GET | /kubernetes | app.py`
- `DONE | GET | /api/kubernetes/status | app.py`
- `DONE | GET | /settings/data-management | app.py`
- `DONE | GET | /settings/help/data-management | app.py`
- `DONE | POST | /settings/data-management | app.py`
- `DONE | GET | /api/data-management/backup/list | app.py`
- `DONE | POST | /api/data-management/backup/run | app.py`
- `DONE | POST | /api/data-management/restore | app.py`
- `DONE | GET | /api/setup-wizard/steps | app.py`
- `DONE | GET | /setup/help/playbooks | app.py`
- `DONE | POST | /api/onboarding/create-repo | app.py`
- `DONE | POST | /api/onboarding/import-repo | app.py`
- `DONE | POST | /api/onboarding/list-repos | app.py`
- `DONE | GET | /api/onboarding/inspect-repo | app.py`
- `TODO | POST | /api/onboarding/create-issues | app.py`

## MCP Routes

- `TODO | GET | /mcp/tools | mcp.py`
- `TODO | POST | /mcp | mcp.py`
- `TODO | GET | /api/mcp/keys | mcp.py`
- `TODO | POST | /api/mcp/keys | mcp.py`
- `TODO | DELETE | /api/mcp/keys/<key_id> | mcp.py`
- `TODO | POST | /api/mcp/enabled | mcp.py`
- `TODO | GET | /settings/mcp | mcp.py`