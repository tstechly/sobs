# Turbo-Flask SPA POC

## Overview

This branch is a **Proof of Concept** that converts SOBS from a traditional
multi-page Flask application into a more **SPA-like experience** using
[Hotwire Turbo](https://turbo.hotwired.dev/). The goal is to evaluate the
effort, trade-offs, and any challenges before committing to a full integration.

The server framework is **Quart** (async Flask). The `turbo-flask` PyPI package
targets synchronous Flask and is therefore **not used directly**; instead, a
small set of Quart-native helpers has been implemented in `app.py`.

---

## What Hotwire Turbo Provides

Turbo is composed of three complementary parts:

| Feature        | Description |
|----------------|-------------|
| **Turbo Drive** | Intercepts all link clicks and form submissions, fetches the new page via AJAX, and swaps the `<body>` without a full browser reload. Navigation becomes instant with no page-flash. |
| **Turbo Frames** | Marks a section of the page with `<turbo-frame id="…">`. Clicks or form submissions *inside* (or targeting) the frame cause only that frame's content to be replaced. |
| **Turbo Streams** | Allows the server to return a `text/vnd.turbo-stream.html` response containing one or more `<turbo-stream action="…">` tags that perform fine-grained DOM mutations (append, replace, remove, etc.) on the live page. |

---

## What Was Implemented in This POC

### 1. Turbo Drive – automatic SPA navigation

**File:** `templates/base.html`

`static/turbo.es2017-umd.js` (built from `@hotwired/turbo` v8.0.23) is loaded
in every page at the bottom of `<body>`. Turbo Drive is enabled by default for
all links and forms in the application the moment the script loads.

**Effect:** Every navigation between pages (Summary → Logs → Traces → …) is now
an AJAX body-swap. The page URL updates, the browser history works, and the
progress bar is shown during fetches.

### 2. Persistent sidebar via `data-turbo-permanent`

**File:** `templates/base.html`

The three navigation chrome elements are marked `data-turbo-permanent`:

```html
<div id="sbBackdrop"   … data-turbo-permanent>
<div id="sbTopbar"     … data-turbo-permanent>
<nav id="sbSidebar"    … data-turbo-permanent>
```

Turbo Drive preserves permanent elements across body swaps (matched by `id`).
This means:

- The sidebar never visually disappears and reappears during navigation.
- Sidebar event listeners are registered exactly once (`sidebar.dataset.turboSetup` guard).
- Sidebar compact/full state is never lost between page visits.

The active nav-link highlighting is updated via a `turbo:load` listener that
performs a URL-prefix match against each link's `href`.

### 3. Bootstrap modal cleanup

**File:** `templates/base.html`

A `turbo:before-visit` listener removes stale Bootstrap modal backdrops before
each navigation. Without this, a modal left open by the user could leave the
`<body>` in a `modal-open` state after Turbo replaces it.

### 4. Turbo Frame on the Logs results section

**File:** `templates/logs.html`

The filter form now carries `data-turbo-frame="logs-results"` and
`data-turbo-action="advance"`. The stats panel, results table, pagination, and
empty-state placeholder are wrapped in:

```html
<turbo-frame id="logs-results">
  …stats panel…
  …results table…
  …pagination…
  …empty state…
</turbo-frame>
```

**Effect:** Submitting the filter form replaces *only* the `logs-results` frame.
The filter accordion stays open, focus is maintained, and the URL is updated so
the back button works correctly.

### 5. Live-mode EventSource cleanup on Turbo navigation

**File:** `templates/logs.html`

When Turbo Drive navigates away from the Logs page the `turbo:before-visit`
listener calls `stopLive()` to close the `EventSource` connection, preventing a
dangling SSE connection in the background.

### 6. Server-side Turbo Stream helpers

**File:** `app.py`

The following Quart-native helper functions are available to any route handler:

```python
_turbo_can_stream()               # True if client accepts text/vnd.turbo-stream.html
_turbo_frame_id()                 # Value of Turbo-Frame request header
_turbo_replace(html, target)      # Build a <turbo-stream action="replace" …>
_turbo_update(html, target)       # Build a <turbo-stream action="update" …>
_turbo_append(html, target)       # Build a <turbo-stream action="append" …>
_turbo_prepend(html, target)      # Build a <turbo-stream action="prepend" …>
_turbo_remove(target)             # Build a <turbo-stream action="remove" …>
_turbo_stream_response(streams)   # Wrap in HTTP response with correct MIME type
```

Example – a route that returns a Turbo Stream if the client supports it, or a
full page otherwise:

```python
@app.route("/my-page")
@require_basic_auth
async def my_page():
    data = fetch_data()
    if _turbo_can_stream():
        html = await render_template("_my_partial.html", data=data)
        return _turbo_stream_response(_turbo_replace(html, target="my-section"))
    return await render_template("my_page.html", data=data)
```

A Jinja2 context variable `turbo_enabled = True` is injected into every
template via the `_inject_turbo_context` context processor, enabling conditional
Turbo markup in templates.

---

## Files Changed

| File | Change |
|------|--------|
| `static/turbo.es2017-umd.js` | Added – Hotwire Turbo UMD bundle (v8.0.23) |
| `requirements.txt` | Added `turbo-flask>=0.8.6` (reference; Quart helpers used instead) |
| `package.json` | Added `@hotwired/turbo` npm dependency |
| `app.py` | Added Turbo helper functions and `turbo_enabled` context processor |
| `templates/base.html` | Persistent sidebar, Turbo JS include, config, modal cleanup |
| `templates/logs.html` | Turbo Frame wrapping results; filter form target; live-mode cleanup |

---

## Known Challenges & Issues

### A. `turbo-flask` library incompatibility with Quart

`turbo-flask` imports from `flask` (synchronous). Quart provides a
Flask-compatible API but is an independent async framework. Using
`turbo_flask.Turbo` directly with a Quart app requires a compatibility shim or
causes import-time errors. For this POC the relevant behaviour is replicated
manually via the `_turbo_*` helpers in `app.py`.

**Mitigation path:** Either wait for an official `turbo-quart` package, or
maintain the hand-rolled helpers (they are small and well-understood).

### B. Inline JS re-execution on Drive navigation

Turbo Drive replaces the entire `<body>`. Inline `<script>` blocks in the new
body are re-executed. External scripts (same `src`) are deduplicated by Turbo
and not re-evaluated. This matches SOBS' current pattern (external libs once,
page JS inline) well. However, any code that registers listeners on `window` or
`document` and is not guarded will register duplicate listeners on each visit.

**Current guard:** The sidebar JS block is guarded with
`if (sidebar.dataset.turboSetup) return;`. The AI assistant widget and other
large JS blocks inside `base.html` may need similar guards if observed to
misbehave.

### C. `DOMContentLoaded` does not re-fire on Turbo navigation

Turbo Drive does not dispatch `DOMContentLoaded` after a body swap. Any page
code that relies exclusively on `DOMContentLoaded` to initialise DOM references
must also listen to `turbo:load`. SOBS templates currently use immediately-
invoked function expressions (IIFEs) that run synchronously as the new body is
parsed, which avoids this issue.

### D. Live mode + Turbo Frame filter change race condition

If the user has live mode active and then changes filters via the Turbo Frame
form, the frame navigation loads a new set of DOM elements (`logsTableBody` etc.)
but the live-mode JavaScript already holds references to the *old* elements. The
new rows would be inserted into the detached old table, not the visible one.

**Mitigation path:** Listen for `turbo:frame-render` on the `logs-results` frame
and re-acquire DOM references (or restart live mode) after each frame update.
This requires refactoring the live-mode JS to support re-initialisation.

### E. Active nav link state for multi-endpoint routes

The server uses Jinja2 `request.endpoint` to set the `active` class on nav
links. With the sidebar `data-turbo-permanent`, the `active` class is managed
client-side via URL prefix matching after `turbo:load`. Settings sub-pages (e.g.
`/settings/tags`, `/settings/ai`) are covered by the prefix match on
`/settings`. Routes that do not share a URL prefix with their nav link (e.g.
custom dashboard at `/dashboards/<id>`) may need explicit mapping.

### F. Browser back-button and Turbo cache

Turbo caches a snapshot of the page before navigation and shows it immediately
on back-button press (preview). If the snapshot contains stale data (e.g. a
results table that has since been updated), the user sees outdated content
briefly before the live copy loads. This is a trade-off inherent to Turbo Drive.
Setting `<meta name="turbo-cache-control" content="no-cache">` in specific
templates opts individual pages out of caching.

### G. ECharts and other canvas-based charts

Apache ECharts renders into a `<canvas>` element. When Turbo replaces the
`<body>`, the new canvas element does not inherit the previous instance's
context. The chart initialisation code re-runs from the inline script block,
which is correct. However, if ECharts is initialised before its container has a
stable size (e.g. inside a collapsible accordion that Turbo renders closed), the
chart may render at zero width. The existing Bootstrap accordion-show listener
pattern used in SOBS pages already handles this correctly.

---

## Testing the POC

1. **Start the server** as usual.
2. Navigate between pages via the sidebar – observe that the URL updates and the
   sidebar does not flicker.
3. Open the **Logs** page, apply a filter in the filter form and click **Apply** –
   observe that only the results section updates while the filter accordion stays
   open.
4. Enable **Live mode** on the Logs page, then click a different sidebar link –
   observe that the SSE connection is cleanly closed.
5. Use the **browser back button** – navigation should behave as expected with
   correct URL history.
6. Check the **browser Network tab** – page navigations should show `fetch()`
   requests rather than full document loads (except the very first visit).

---

## Potential Next Steps

- Add `data-turbo-permanent` to the AI assistant panel so its chat history
  persists across navigations.
- Implement Turbo Stream responses for the Summary page stats refresh
  (replacing the current JavaScript `setInterval` polling approach).
- Extend Turbo Frame support to the Errors, Traces, and Metrics filter forms.
- Add `<meta name="turbo-cache-control" content="no-cache">` to pages with
  rapidly-changing data (Summary dashboard).
- Investigate `data-turbo-refresh-method="morph"` for same-URL page refreshes
  to enable smooth auto-refresh without full body replacement.
- Fix live-mode + Turbo Frame race (Challenge D above) by refactoring the
  live-mode JS module.
- Evaluate whether an official Quart-compatible Turbo library is preferable to
  the current hand-rolled helpers.
