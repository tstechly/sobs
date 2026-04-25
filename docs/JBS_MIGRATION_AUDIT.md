# jinja-bootstrap-spa Migration Audit

## Framework Reference

- Repository: `https://github.com/abartrim/jinja-bootstrap-spa`
- Branch: `main`
- Commit: `fc3d87e31358bd43bfd4cc88cf60d2e814d96718`
- Version: `0.1.0`
- Installed via: `pip install "git+https://github.com/abartrim/jinja-bootstrap-spa.git@main"`

## Installation Gate

Framework was successfully accessed, read, and installed:
- Package import: `from jinja_bootstrap_spa import conditional_fragment_response, parse_table_state, register_bootstrap_macros` ✓
- Static runtime JS: `jinja_bootstrap_spa/static/jinja-bootstrap-spa.js` present ✓
- Macro template: `jinja_bootstrap_spa/bootstrap_macros.html` registered via `register_bootstrap_macros(app.jinja_env)` ✓

Docs read before implementing:
- `AGENTS.md`
- `docs/llm_authoring_guide.md`
- `docs/api_reference.md`
- `docs/feature_coverage.md`
- `docs/framework_backlog.md`
- `docs/versioning_policy.md`

---

## Migration Audit — Sobs UI Surface Candidates

### 1. Work Items Queue (✅ IMPLEMENTED — first slice)

| Field | Value |
|---|---|
| **Page / component** | Work Items |
| **Template path** | `templates/work_items.html` → `templates/partials/work_items_table.html` |
| **Backend route** | `GET /work-items` (`view_work_items`) |
| **Fragment endpoint** | `GET /components/work-items` (`work_items_table_component`) |
| **Repeated UI pattern** | Paginated table with 8 columns, filter form, modal detail drawer |
| **JBS primitive used** | `ui.table(...)` from `jinja_bootstrap_spa/bootstrap_macros.html` |
| **Streaming mode** | `replace` (fragment replace on sort/page) |
| **State preserved** | `page`, `page_size`, `sort_by`, `sort_dir`, `service`, `rule_name`, `action_type`, `status`, `from_ts`, `to_ts` |
| **ETag / 304** | Yes — `conditional_fragment_response` on the fragment endpoint |
| **Backend changes** | Added `parse_table_state`, `_work_items_table_rows`, `_fetch_work_items_fragment_data`, `/components/work-items` endpoint |
| **Priority** | High |
| **Risk** | Low |

**Notes**: The existing filter accordion (Sobs `render_filter_accordion` macro) is preserved.  The JBS runtime handles sort and pagination via fragment replacement.  The existing detail modal is preserved unchanged.

---

### 2. Logs Page

| Field | Value |
|---|---|
| **Page / component** | Log Events |
| **Template path** | `templates/logs.html` |
| **Backend route** | `GET /logs` (`view_logs`) |
| **Repeated UI pattern** | Filterable accordion list with pagination, TZ timestamps, log level badges |
| **Recommended JBS primitive** | `ui.table(...)` or `ui.data_grid(...)` for the log entry rows |
| **Streaming mode** | `append` or `replace` — append would allow live-tail UX |
| **State to preserve** | `page`, `level`, `service`, `query`, `from_ts`, `to_ts`, `source` |
| **Backend changes** | Fragment endpoint `/components/logs`, `parse_table_state`, `conditional_fragment_response` |
| **Priority** | High |
| **Risk** | Medium (accordion expand/collapse state, TZ widget re-init) |

---

### 3. Error Events Page

| Field | Value |
|---|---|
| **Page / component** | Errors (including grouped/dedup view) |
| **Template path** | `templates/errors.html`, `templates/_error_panels.html` |
| **Backend route** | `GET /errors` (`view_errors`) |
| **Repeated UI pattern** | Accordion error cards, dedup grouped view toggle, pagination, inline incident links |
| **Recommended JBS primitive** | `ui.table(...)` for the error list; `ui.status_region(...)` for toast notifications |
| **Streaming mode** | `replace` |
| **State to preserve** | `page`, `level`, `service`, `query`, `from_ts`, `to_ts`, `grouped` flag |
| **Backend changes** | Fragment endpoint `/components/errors`, state-aware grouped/ungrouped render |
| **Priority** | High |
| **Risk** | Medium (complex accordion structure, grouped mode toggle) |

---

### 4. Traces Page

| Field | Value |
|---|---|
| **Page / component** | Distributed Traces |
| **Template path** | `templates/traces.html` |
| **Backend route** | `GET /traces` (`view_traces`) |
| **Repeated UI pattern** | Table of spans with filter, sort, pagination |
| **Recommended JBS primitive** | `ui.table(...)` |
| **Streaming mode** | `replace` |
| **State to preserve** | `page`, `page_size`, `sort_by`, `sort_dir`, `service`, `query`, `from_ts`, `to_ts` |
| **Backend changes** | Fragment endpoint `/components/traces`, `parse_table_state` |
| **Priority** | High |
| **Risk** | Low |

---

### 5. AI Transparency Page

| Field | Value |
|---|---|
| **Page / component** | AI Events |
| **Template path** | `templates/ai.html` |
| **Backend route** | `GET /ai` (`view_ai`) |
| **Repeated UI pattern** | Table with AI call rows, filter, cost badges |
| **Recommended JBS primitive** | `ui.table(...)` |
| **Streaming mode** | `replace` |
| **State to preserve** | `page`, `model`, `service`, `query`, `from_ts`, `to_ts` |
| **Backend changes** | Fragment endpoint `/components/ai`, `parse_table_state` |
| **Priority** | Medium |
| **Risk** | Low |

---

### 6. Metrics Anomaly Page

| Field | Value |
|---|---|
| **Page / component** | Metrics Anomaly |
| **Template path** | `templates/metrics_anomaly.html` |
| **Backend route** | `GET /metrics/anomaly` (`view_metrics_anomaly`) |
| **Repeated UI pattern** | Table of anomaly events with severity badges, filter, sort |
| **Recommended JBS primitive** | `ui.table(...)` |
| **Streaming mode** | `replace` |
| **State to preserve** | `page`, `page_size`, `sort_by`, `sort_dir`, `service`, `state_filter`, `from_ts`, `to_ts` |
| **Backend changes** | Fragment endpoint `/components/metrics-anomaly`, `parse_table_state` |
| **Priority** | Medium |
| **Risk** | Low |

---

### 7. Summary Dashboard

| Field | Value |
|---|---|
| **Page / component** | Summary / Home |
| **Template path** | `templates/summary.html` |
| **Backend route** | `GET /` (`summary`) |
| **Repeated UI pattern** | Multiple stat cards, DB stats card, auto-refresh via polling |
| **Recommended JBS primitive** | Multiple `ui.data_grid(...)` components or custom `data-jbs-component` sections |
| **Streaming mode** | `replace` with SSE refresh event when backend data changes |
| **State to preserve** | None (read-only dashboard) |
| **Backend changes** | SSE endpoint or timed-refresh via `data-jbs-action="refresh"` |
| **Priority** | Medium |
| **Risk** | Medium (complex multi-card layout, existing JS polling) |

---

### 8. CVE Findings Page

| Field | Value |
|---|---|
| **Page / component** | CVE Findings |
| **Template path** | `templates/cve.html` |
| **Backend route** | `GET /cve` (`view_cve`) |
| **Repeated UI pattern** | Table of CVE findings with severity, service, CVSS filters |
| **Recommended JBS primitive** | `ui.table(...)` |
| **Streaming mode** | `replace` |
| **State to preserve** | `page`, `page_size`, `severity`, `service`, `query` |
| **Backend changes** | Fragment endpoint `/components/cve`, `parse_table_state` |
| **Priority** | Low |
| **Risk** | Low |

---

### 9. Query / Table Explorer Page

| Field | Value |
|---|---|
| **Page / component** | Query / NL→SQL |
| **Template path** | `templates/query.html`, `templates/table_explorer.html` |
| **Backend route** | `GET /query`, `GET /table-explorer` |
| **Repeated UI pattern** | Schema tables, query result table |
| **Recommended JBS primitive** | `ui.table(...)` for results; `ui.lazy_region(...)` for on-demand schema load |
| **Streaming mode** | `replace` |
| **State to preserve** | `query`, `table`, `limit` |
| **Backend changes** | Fragment endpoints per table |
| **Priority** | Low |
| **Risk** | Medium (streaming SQL results, schema metadata) |

---

## Implemented First Slice — Details

### What Changed

**`requirements.txt`**  
Added: `jinja-bootstrap-spa @ git+https://github.com/abartrim/jinja-bootstrap-spa.git@main`

**`app.py`**  
- Import: `from jinja_bootstrap_spa import conditional_fragment_response, parse_table_state, register_bootstrap_macros`
- `register_bootstrap_macros(app.jinja_env)` — registers the virtual macro template
- `GET /static/jinja-bootstrap-spa.js` — serves the packaged runtime JS from the installed package
- `GET /components/work-items` — JBS fragment endpoint returning the table HTML with ETag/304
- Helpers: `_work_items_table_rows`, `_fetch_work_items_fragment_data`, sort map constants

**`templates/base.html`**  
- Added `<script src="{{ url_for('jbs_static_js') }}"></script>` to load the JBS runtime on every page

**`templates/partials/work_items_table.html`** (new)  
- Uses `{% import "jinja_bootstrap_spa/bootstrap_macros.html" as ui %}`
- Renders `ui.table(...)` with 8 columns, state, pagination, sort
- `persist="header"` — state flows via `X-JBS-State` header, no URL pollution on sort/page

**`templates/work_items.html`**  
- Replaces 70+ lines of raw `<table>` markup with `{% include "partials/work_items_table.html" %}`
- Existing filter form, modals, and scripts unchanged

### No Local Shims

- The packaged `jinja-bootstrap-spa.js` is served directly from the installed package location
- No `data-jbs-*` attributes were invented; all attributes (`data-jbs-component`, `data-jbs-endpoint`, `data-jbs-target`, `data-jbs-state`, `data-jbs-persist`, `data-jbs-key`) come from the `ui.table(...)` macro
- `conditional_fragment_response`, `parse_table_state`, `register_bootstrap_macros` are all first-party framework imports

---

## Missing / Backlog Framework Primitives

During migration the following generic framework gaps were noted. None were workaround-implemented inside Sobs; they are documented as framework backlog items.

1. **Date-range filter primitive**: The Sobs filter form uses a custom `drp-combo` date-range picker. The framework has `ui.date_range_picker(...)` which covers this use case for future slices but wasn't used in this slice since the filter form is server-rendered Sobs markup.

2. **Async Quart integration helper**: The `conditional_fragment_response` helper is documented as returning `(str, int, dict)` which Quart accepts natively. No gap needed.

3. **`data-jbs-action="filter"` wiring with existing non-JBS filter forms**: When the existing filter form uses `method="get"` (full page reload), the data_grid state is re-seeded from GET params on the next render. This is the intended migration path — full interactivity can be added in a later slice by converting the filter form to a `data-jbs-form`.

---

## Migration Backlog — Recommended Next Slices

1. **Logs page** — high value, append streaming possible
2. **Traces page** — same table pattern, quick win
3. **Errors page** — complex accordion, medium risk
4. **AI page** — same as traces pattern
5. **Summary dashboard** — SSE-driven refresh for stat cards
6. **Filter form migration** — convert Sobs filter forms to `data-jbs-form` so filter changes trigger fragment replacement instead of full page reload
