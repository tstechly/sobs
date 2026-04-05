# Release Notes

## Unreleased

### Added

- Added Web Traffic page with browser context enrichment: delta-posting RUM optimization (87% payload reduction via session caching), 5 aggregation endpoints (browsers, OS, timezones, languages, devices), ECharts visualizations, and IP geolocation via local geoip2fast (no API keys, MIT license).
- Added CVE vulnerability scanning: OSV.dev integration with daily auto-scan (30s after startup, 24h interval), library inventory from release registry + OTEL ResourceAttributes + instrumentation scopes, and dedicated CVE findings page with severity filtering.
- Added Enrichment Settings page for configuring IP geolocation and CVE scanning behavior.
- Added browser context capture in RUM client: timezone, language, platform, browser/OS versions, device class, screen resolution with automatic delta posting to reduce network bandwidth and storage costs.

### Changed

- Query statistics now stay consistent with grep filtering by evaluating stats on the same query result set used for rendering.
- AI guard enforcement is now fail-closed: missing guard config, guard call failures, and invalid guard responses block execution.
- Agent action handling now respects configured actions (analysis only runs when `analyze` is enabled).
- Agent issue-rate controls now clamp max issues per hour server-side and treat latest run activity consistently for cooldown checks.
- SOBS session cookie now defaults to `sobs_session` (`SOBS_SESSION_COOKIE_NAME`) to avoid name/path collisions with same-origin management cookies named `session`.
- Dashboard chart cards now use a dedicated SQL modal and renamed the source button label to **Data Source**.
- Filter bars were unified across major pages with consistent responsive layout and shared report-save modal UX.
- Light-mode contrast/readability fixes were applied across Errors/Traces stack-trace surfaces, chart-help/code panels, metrics rules surfaces, and legacy dark-table usage.

### Testing
- Added 6 tests for web traffic aggregation endpoints (browsers, OS, timezones, languages, devices).
- Added 3 tests for RUM browser context delta posting: full context storage, delta cache retrieval, and backward compatibility.
- Added tests for CVE scanning endpoints, library version extraction from OTEL, and geoip2fast local lookup.

- Added UI tests for query-scoped statistics behavior and manual advanced-analysis rendering.
- Added backend tests for guard fail-closed behavior, settings encryption/decryption, action gating, and automated agent trigger execution.
- Added backend tests validating AI settings env/file override precedence over DB-stored values.
- Added regression coverage for report save/apply flows, report JSON hydration hardening, and report dropdown XSS-safe rendering.
- Added tests for query page runner/route behavior, SQL safety enforcement, and query add-to-dashboard flows.
- Added tests for notifications browser push service-worker routing/registration and VAPID key management paths.
