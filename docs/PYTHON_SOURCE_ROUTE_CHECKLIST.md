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
- `TODO | GET | /api/web-traffic/geo | app.py`
- `TODO | GET | /api/web-traffic/browsers | app.py`
- `TODO | GET | /api/web-traffic/os | app.py`
- `TODO | GET | /api/web-traffic/timezones | app.py`
- `TODO | GET | /api/web-traffic/languages | app.py`
- `TODO | GET | /api/web-traffic/devices | app.py`

## Enrichment And Work Items

- `TODO | GET | /api/enrichment/libraries | app.py`
- `TODO | GET | /api/enrichment/github/repo-health | app.py`
- `TODO | GET | /enrichment/cve | app.py`
- `TODO | GET | /cve/help | app.py`
- `TODO | GET | /api/enrichment/cve/findings | app.py`
- `TODO | POST | /api/enrichment/cve/findings/<osv_id>/disposition | app.py`
- `TODO | POST | /api/enrichment/cve/scan | app.py`
- `TODO | GET | /work-items | app.py`
- `TODO | GET | /work-items/help | app.py`
- `TODO | GET | /api/work-items | app.py`

## AI And Dashboards

- `TODO | GET | /ai | app.py`
- `TODO | GET | /ai/help | app.py`
- `TODO | GET | /api/ai/span-attributes | app.py`
- `TODO | GET | /api/ai/conversation | app.py`
- `TODO | GET | /api/ai/export | app.py`
- `TODO | GET | /api/dashboards/list | app.py`
- `TODO | POST | /api/query/add-to-dashboard | app.py`
- `TODO | GET | /dashboards | app.py`
- `TODO | GET | /dashboards/help/chart-editor | app.py`
- `TODO | GET | /dashboards/new | app.py`
- `TODO | POST | /dashboards | app.py`
- `TODO | GET | /dashboards/<dashboard_id> | app.py`
- `TODO | POST | /dashboards/<dashboard_id>/delete | app.py`
- `TODO | POST | /dashboards/<dashboard_id>/charts | app.py`
- `TODO | POST | /dashboards/<dashboard_id>/charts/<chart_id>/edit | app.py`
- `TODO | POST | /dashboards/<dashboard_id>/charts/<chart_id>/clone | app.py`
- `TODO | POST | /dashboards/<dashboard_id>/charts/<chart_id>/delete | app.py`
- `TODO | POST | /api/dashboards/query | app.py`
- `TODO | GET | /api/dashboards/spec/templates | app.py`
- `TODO | GET | /api/dashboards/spec/options | app.py`
- `TODO | POST | /api/dashboards/spec/compile | app.py`
- `TODO | POST | /api/dashboards/spec/dry-run | app.py`
- `TODO | POST | /api/dashboards/spec/validate | app.py`
- `TODO | POST | /api/dashboards/spec/render | app.py`
- `TODO | POST | /api/dashboards/render | app.py`
- `TODO | POST | /api/dashboards/spec/ai-build | app.py`
- `TODO | GET | /api/dashboards/<dashboard_id>/charts/<chart_id>/export | app.py`
- `TODO | POST | /api/dashboards/<dashboard_id>/charts/import | app.py`
- `DONE | GET | /api/metrics/anomaly | app.py`

## Reports

- `TODO | GET | /reports | app.py`
- `TODO | GET | /reports/help | app.py`
- `TODO | POST | /reports/<report_id>/delete | app.py`
- `TODO | GET | /api/reports | app.py`
- `TODO | POST | /api/reports | app.py`
- `TODO | DELETE | /api/reports/<report_id> | app.py`
- `TODO | GET | /api/reports/export | app.py`
- `TODO | POST | /api/reports/import | app.py`

## Static And Core Settings

- `TODO | GET | /static/rum.js | app.py`
- `TODO | GET | /static/rum.js.map | app.py`
- `TODO | GET | /static/rum.min.js | app.py`
- `TODO | GET | /static/rum.min.js.map | app.py`
- `TODO | GET | /static/rum.d.ts | app.py`
- `TODO | GET | /settings | app.py`
- `TODO | GET | /settings/help | app.py`
- `TODO | GET | /settings/masking | app.py`
- `TODO | GET | /settings/help/masking | app.py`
- `TODO | POST | /settings/masking/keys | app.py`
- `TODO | POST | /settings/masking/keys/delete | app.py`
- `TODO | POST | /settings/masking/patterns | app.py`
- `TODO | POST | /settings/masking/patterns/delete | app.py`
- `TODO | POST | /settings/masking/output | app.py`
- `TODO | POST | /settings/masking/sql-output | app.py`
- `TODO | POST | /api/settings/masking/preview | app.py`
- `TODO | GET | /api/settings/masking/rules | app.py`

## Tags And Validation Helpers

- `TODO | GET | /settings/tags | app.py`
- `TODO | GET | /settings/help/tags | app.py`
- `TODO | GET | /api/settings/tags/condition-suggestions | app.py`
- `TODO | POST | /settings/tags/auto | app.py`
- `TODO | POST | /settings/tags | app.py`
- `TODO | POST | /settings/tags/<rule_id>/delete | app.py`
- `TODO | GET | /api/tags/<record_type>/<record_id> | app.py`
- `TODO | POST | /api/tags/<record_type>/<record_id> | app.py`
- `TODO | DELETE | /api/tags/<record_type>/<record_id>/<tag_key> | app.py`
- `DONE | GET | /api/logs/field-hints | app.py`
- `DONE | POST | /api/logs/validate-filter | app.py`
- `DONE | POST | /api/logs/validate-regex | app.py`
- `TODO | POST | /api/errors/validate-regex | app.py`
- `TODO | POST | /api/traces/validate-regex | app.py`
- `TODO | POST | /api/metrics/validate-regex | app.py`
- `TODO | POST | /api/rum/validate-regex | app.py`
- `TODO | GET | /api/ai/field-hints | app.py`
- `TODO | POST | /api/ai/validate-filter | app.py`
- `DONE | GET | /tail | app.py`

## Notifications And Health

- `TODO | GET | /settings/notifications | app.py`
- `TODO | GET | /settings/help/notifications | app.py`
- `TODO | POST | /settings/notifications/channels | app.py`
- `TODO | POST | /settings/notifications/channels/<channel_id>/delete | app.py`
- `TODO | POST | /settings/notifications/channels/<channel_id>/toggle | app.py`
- `TODO | POST | /api/notifications/channels/<channel_id>/test | app.py`
- `TODO | POST | /settings/notifications/rules | app.py`
- `TODO | POST | /settings/notifications/rules/<rule_id>/toggle | app.py`
- `TODO | POST | /settings/notifications/rules/<rule_id>/delete | app.py`
- `TODO | POST | /api/notifications/rules/auto-generate | app.py`
- `TODO | POST | /api/notifications/check | app.py`
- `TODO | GET | /api/notifications/vapid-public-key | app.py`
- `TODO | GET | /service-worker.js | app.py`
- `TODO | POST | /api/notifications/subscribe | app.py`
- `TODO | POST | /api/notifications/vapid-keygen | app.py`
- `TODO | DELETE | /api/notifications/vapid-keys | app.py`
- `TODO | GET | /health | app.py`
- `TODO | GET | /health/db | app.py`

## AI Settings And Repositories

- `TODO | GET | /settings/ai | app.py`
- `TODO | GET | /settings/help/ai | app.py`
- `TODO | POST | /settings/ai | app.py`
- `TODO | GET | /settings/enrichment | app.py`
- `TODO | GET | /settings/help/enrichment | app.py`
- `TODO | POST | /settings/enrichment | app.py`
- `TODO | GET | /settings/repositories | app.py`
- `TODO | GET | /settings/help/repositories | app.py`
- `TODO | POST | /settings/repositories | app.py`
- `TODO | POST | /settings/repositories/github-token/validate | app.py`
- `TODO | POST | /settings/repositories/<app_id>/realtime-mode | app.py`
- `TODO | POST | /settings/repositories/<app_id>/ci-ingest-key/rotate | app.py`
- `TODO | POST | /settings/repositories/<app_id>/ci-ingest-key/revoke | app.py`
- `TODO | POST | /settings/repositories/<app_id> | app.py`
- `TODO | POST | /settings/repositories/<app_id>/releases | app.py`
- `TODO | POST | /settings/repositories/<app_id>/delete | app.py`

## Agents And AI Helper

- `TODO | GET | /settings/agents | app.py`
- `TODO | GET | /settings/help/agents | app.py`
- `TODO | POST | /settings/agents | app.py`
- `TODO | POST | /settings/agents/<rule_id>/delete | app.py`
- `TODO | GET | /api/ai/helper/capabilities | app.py`
- `TODO | GET | /api/ai/helper/actions/manifest | app.py`
- `TODO | GET | /api/ai/helper/chats | app.py`
- `TODO | GET | /api/ai/helper/chats/<chat_id> | app.py`
- `TODO | POST | /api/ai/helper/feedback | app.py`
- `TODO | POST | /api/ai/helper | app.py`
- `TODO | POST | /api/ai/helper/actions/execute | app.py`
- `TODO | POST | /api/issues/raise | app.py`
- `TODO | GET | /api/agent/runs | app.py`
- `TODO | POST | /api/agent/runs | app.py`
- `TODO | POST | /api/agent/runs/<run_id>/dismiss | app.py`

## Query Explorer Kubernetes And Data Management

- `TODO | GET | /query | app.py`
- `TODO | GET | /query/help | app.py`
- `TODO | POST | /api/query/ask | app.py`
- `TODO | POST | /api/query/run | app.py`
- `TODO | POST | /api/query/refine-chart | app.py`
- `TODO | GET | /api/query/schema | app.py`
- `TODO | GET | /table-explorer | app.py`
- `TODO | GET | /table-explorer/help | app.py`
- `TODO | GET | /api/table-explorer/tables | app.py`
- `TODO | GET | /api/table-explorer/table/<name> | app.py`
- `TODO | GET | /api/chart-types | app.py`
- `TODO | GET | /settings/kubernetes | app.py`
- `TODO | GET | /kubernetes/help | app.py`
- `TODO | GET | /settings/help/kubernetes | app.py`
- `TODO | POST | /settings/kubernetes | app.py`
- `TODO | GET | /kubernetes | app.py`
- `TODO | GET | /api/kubernetes/status | app.py`
- `TODO | GET | /settings/data-management | app.py`
- `TODO | GET | /settings/help/data-management | app.py`
- `TODO | POST | /settings/data-management | app.py`
- `TODO | GET | /api/data-management/backup/list | app.py`
- `TODO | POST | /api/data-management/backup/run | app.py`
- `TODO | POST | /api/data-management/restore | app.py`
- `TODO | GET | /api/setup-wizard/steps | app.py`
- `TODO | GET | /setup/help/playbooks | app.py`
- `TODO | POST | /api/onboarding/create-repo | app.py`
- `TODO | POST | /api/onboarding/import-repo | app.py`
- `TODO | POST | /api/onboarding/list-repos | app.py`
- `TODO | GET | /api/onboarding/inspect-repo | app.py`
- `TODO | POST | /api/onboarding/create-issues | app.py`

## MCP Routes

- `TODO | GET | /mcp/tools | mcp.py`
- `TODO | POST | /mcp | mcp.py`
- `TODO | GET | /api/mcp/keys | mcp.py`
- `TODO | POST | /api/mcp/keys | mcp.py`
- `TODO | DELETE | /api/mcp/keys/<key_id> | mcp.py`
- `TODO | POST | /api/mcp/enabled | mcp.py`
- `TODO | GET | /settings/mcp | mcp.py`