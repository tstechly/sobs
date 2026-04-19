# Go Migration Parity Audit (2026-04-19)

## Scope

This audit compares Python Flask routes and behavior in app.py against Go net/http routes and handlers under internal/web.

User-reported failures included:
- Summary page shows zeros and no recent Logs/Errors.
- Settings page throws a template error.
- Dashboards do not render.
- Some pages return JSON instead of rendering templates.

This document is focused on endpoint parity and implementation parity without changing Jinja templates.

## Source Artifacts

Complete endpoint inventories generated during this audit:
- Python route map (path | methods | handler): docs/parity/python_routes_map.txt
- Go route map (path | handler): docs/parity/go_routes_map.txt
- Normalized Python-only paths currently missing in Go: docs/parity/missing_in_go.txt
- Normalized Python-only paths likely covered by Go subroute prefixes: docs/parity/missing_in_go_likely_subroutes.txt
- Normalized Python-only paths likely true coverage gaps after subroute classification: docs/parity/missing_in_go_probable_true.txt
- Go prefix/subroute coverage prefixes used for classification: docs/parity/go_subroute_prefixes.txt
- Normalized Go-only paths currently not present in Python: docs/parity/extra_in_go.txt
- Existing Python endpoint behavior inventory: ROUTE_INVENTORY.md

## High-Confidence Root Causes

### 1) Summary page parity gap (zeros by design in current Go)

Go handler currently hardcodes empty/zero values instead of querying data:
- internal/web/page_routes.go (summaryPage)

Current Go context hardcodes:
- stats.logs/errors/spans/rum/ai = 0
- recent_errors = []
- recent_logs = []
- rum_summary = []
- ai_summary = []

Python behavior (app.py summary) computes all of these from DB queries and cache.

### 2) Settings template error is reproducible in Go renderer

Reproduction done by rendering settings.html with Go renderer; it fails with:
- unknown function: unknown callable (at line 435)

Relevant template line uses Python-style method call:
- templates/settings.html:435
- expression: table_name.startswith('v_')

Go renderer compatibility shims currently do not support this call style:
- internal/templates/renderer.go

### 3) Dashboards page/detail flow mismatch (JSON vs HTML)

Go dashboards handlers return JSON for routes that must render templates in Python:
- internal/web/dashboards_management_api.go

Key mismatch:
- GET /dashboards/<dashboard_id>
  - Python: renders custom_dashboard_view.html
  - Go: returns JSON dashboard object

Also:
- GET /dashboards/new
  - Python: renders custom_dashboards.html with show_new_form=true
  - Go: returns JSON {form: "new-dashboard"}

### 4) JSON fallback path in many Go page handlers

Large number of page handlers currently return JSON when renderer is nil/error:
- internal/web/page_routes.go
- internal/web/page_handlers.go
- internal/web/table_explorer_pages.go
- internal/web/settings_* handlers

This creates behavior divergence from Python UI routes and explains "some pages return JSON" symptoms.

## Endpoint Coverage Snapshot

Raw counts from extraction:
- Python normalized paths: 168
- Go mux registrations: 203
- Go normalized paths: 203

After normalized path comparison:
- Python-only normalized paths before subroute classification: 39 (docs/parity/missing_in_go.txt)
- Python-only normalized paths likely covered by Go prefix/subroute handlers: 38 (docs/parity/missing_in_go_likely_subroutes.txt)
- Python-only normalized paths likely true gaps after subroute classification: 0 (docs/parity/missing_in_go_probable_true.txt)

Representative Python-only normalized paths that classify as likely covered by Go subroutes include:
- /dashboards/{}/charts
- /dashboards/{}/charts/{}/edit
- /dashboards/{}/charts/{}/clone
- /dashboards/{}/charts/{}/delete
- /dashboards/{}/delete
- /settings/repositories/{}/realtime-mode
- /settings/repositories/{}/ci-ingest-key/rotate
- /settings/repositories/{}/ci-ingest-key/revoke
- /settings/repositories/{}/releases
- /settings/repositories/{}/delete
- /settings/notifications/channels/{}/toggle
- /settings/notifications/rules/{}/toggle
- /api/notifications/channels/{}/test
- /v1/logs
- /v1/traces
- /v1/metrics
- /v1/errors
- /v1/rum
- /v1/ai
- /v1/apps/{}/releases
- /v1/releases/{}/artifacts
- /v1/releases/{}/artifacts/meta

Notes:
- Some items are path-shape equivalents handled by prefix subroutes in Go (for example /api/tags/ and /api/traces/span/), but behavioral parity still needs validation route-by-route.

## Methodical Fix Plan (No Jinja Template Changes)

### Phase 0: Lock and Verify Baseline

1. Keep templates unchanged.
2. Add/extend parity tests that assert route behavior type for page endpoints:
   - HTML content-type and key markers for page routes
   - JSON only for API routes
3. Add targeted failing tests for current user-reported issues before code changes.

### Phase 1: Fix Rendering Parity Foundation

1. Fix Go renderer compatibility for Python/Jinja string method usage used in existing templates (specifically startswith in settings template).
2. Remove JSON fallback behavior from page handlers in normal operation:
   - page routes should fail loudly with server error if renderer is unavailable, not silently change response shape.
3. Add template render regression tests for key pages:
   - /summary
   - /settings
   - /dashboards
   - /dashboards/<id>

### Phase 2: Summary Data Parity

1. Port summary query logic from Python to Go (including cache semantics):
   - counts for logs/errors/spans/rum/ai
   - recent_errors
   - recent_logs
   - rum_summary
   - ai_summary
   - signal_health
   - cve_overview aggregation
2. Match key names expected by templates exactly.
3. Validate with data fixture tests and manual page load.

### Phase 3: Dashboards Page Flow Parity

1. Change Go dashboard page endpoints to match Python response type and flow:
   - GET /dashboards => render custom_dashboards.html
   - GET /dashboards/new => render custom_dashboards.html with show_new_form=true
   - GET /dashboards/<dashboard_id> => render custom_dashboard_view.html
2. Ensure form POST routes return redirects where Python does (page flow), while API endpoints remain JSON.
3. Verify chart CRUD endpoints used by template actions map correctly.

### Phase 4: Settings + Feature Route Parity Sweep

1. Validate every settings subpage context against template usage:
   - /settings
   - /settings/ai
   - /settings/enrichment
   - /settings/repositories
   - /settings/notifications
   - /settings/masking
   - /settings/tags
   - /settings/agents
2. For each route, compare Python handler context keys with Go context keys and fill gaps.
3. Confirm action endpoints with dynamic IDs exist and match expected method and semantics (toggle/delete/revoke/rotate patterns).

### Phase 5: Ingest and API Contract Parity

1. Align v1 ingest endpoints and app/release artifact APIs with Python path + method + payload semantics.
2. Verify auth and security behavior parity for UI/API modes.
3. Add parity tests for representative endpoints per functional area.

## Execution Order (Highest user impact first)

1. Renderer/settings template error
2. Dashboard HTML rendering flow
3. Summary data population
4. JSON-vs-HTML response-type parity for all page routes
5. Remaining endpoint/path/method mismatches and contract parity

## Acceptance Criteria

1. No Jinja template edits required for parity.
2. All page routes render HTML (not JSON) under normal operation.
3. /summary shows non-zero real counts when data exists and includes recent logs/errors.
4. /settings renders without template exceptions.
5. /dashboards and /dashboards/<id> render templates and support chart workflows.
6. Normalized missing endpoint list reduced to only intentional, documented differences.
7. Parity test suite covers route type, status code, and key context/JSON contract checks.
