# Python Route Parity Workflow

Source of truth:
- `app.py`
- `mcp.py`
- `masking.py`

Rules:
- Delete Go routes that do not exist in Python unless we explicitly approve a non-Python route in writing.
- Only treat `@app.route(...)`, `app.add_url_rule(...)`, `@mcp_app.route(...)`, and equivalent runtime registrations as real route sources.
- Do not infer routes from template maps, action manifests, renderer URL maps, or other metadata tables.
- Work one Python endpoint at a time.
- Do not mark an endpoint done because a Go test is green or because a page loads.
- Do not redesign request or response shapes.
- Do not redesign SQL if Python already has the correct query flow.
- Do not change templates, CSS, or JS as part of parity work.

For every endpoint, verify all of the following before moving on:
- Route path parity.
- HTTP method parity.
- Auth parity.
- Request parsing parity.
- Data model parity.
- SQL/query parity.
- Response status code parity.
- Response body parity.
- Template name and context parity for page routes.
- Error response parity.

Per-endpoint checklist:
1. Read the Python handler in `app.py` or `mcp.py`.
2. Identify the exact inputs.
3. Identify the exact DB reads and writes.
4. Identify the exact success payload or rendered template context.
5. Identify the exact error cases and error payloads.
6. Compare the Go route and handler.
7. Delete any extra Go route that is not part of the Python flow.
8. Fix Go until route, method, SQL, flow, and contract match Python.
9. Add or update focused tests for that endpoint only.
10. Move the checklist item from `TODO` to `DONE` only after source-to-source review.

Completion states:
- `TODO`: not yet audited.
- `IN PROGRESS`: currently being ported or reviewed.
- `DONE`: source parity verified.
- `BLOCKED`: cannot finish without a user decision.

Order of work:
- Start by removing extra Go-only routes.
- Then work through the Python route checklist in [docs/PYTHON_SOURCE_ROUTE_CHECKLIST.md](docs/PYTHON_SOURCE_ROUTE_CHECKLIST.md).
- Only after all routes are reviewed should we do a final sweep for shared data model and auth consistency.