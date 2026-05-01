# Coverage To 95 Plan

## Goal

Raise the codebase to sustainable `95%+` coverage by testing business logic directly, not by adding more route-only tests around a monolithic `app.py`.

## Current Baseline

- Overall line coverage is now `84%` from the latest sequential full-suite run.
- `app.py` remains the dominant risk and coverage bottleneck at `79%` line coverage.
- Route blueprints are a foundation step, but they do not by themselves create the module seams needed for high-confidence unit coverage.

## Measured Current State

- Fresh sequential coverage run from `coverage-latest.xml` measured overall line coverage at `84%` versus the prior `78.06%` checkpoint baseline.
- `app.py` now measures `79%` line coverage while the extracted business-logic modules continue to climb above the direct-test target.
- `shared/ai_chart.py` now measures `97.52%` line coverage in the full-suite report, and its dedicated direct test run measured `98%` coverage for the extracted chart-spec parsing/repair/generation helpers.
- `shared/chart_specs.py` now measures `96%` line coverage in its dedicated direct test run and `96%` in the latest full-suite report after extracting chart-query validation, SQL literal/coercion helpers, default/raw spec assembly, chart-spec normalization, builder SQL compilation, compiled-spec validation, template role-map resolution, boolean parsing, visual override helpers, column-type inference, public dashboard DB-error sanitization, deep placeholder substitution, binding extraction, drilldown timestamp normalization, drilldown metadata attachment, custom ECharts JSON parsing, custom binding resolution, custom drilldown payload assembly, series-point ordering, derived-signal row preparation, and generic chart-template rendering orchestration from `app.py`, including real missing-`sql.mode` and blank-error fallback bug fixes uncovered by the new direct tests.
- `shared/dashboards.py` now measures `100%` line coverage in its dedicated direct test run after extracting dashboard row serialization, chart row normalization, dashboard/chart row builders, dashboard template-list assembly, dashboard chart form parsing, and query-page add-to-dashboard payload normalization from `app.py`.
- `shared/ai_runtime.py` now measures `99%` line coverage in its dedicated direct test run after extracting the shared LLM request assembly, streaming parsing, guard prompt/parsing logic, thinking/token/timeout resolution, and DLP endpoint helpers from `app.py`.
- `shared/ai_sql.py` now measures `96.2%` line coverage after extracting the SQL planner/repair helpers in Milestone 3 phase 1.
- `shared/ai_memory.py` now measures `95%` line coverage in its dedicated direct test run after extracting the AI embedding, assistant-meta parsing, semantic-memory matching, memory consolidation, recent-turn loading, and tool-history helpers in Milestone 5.
- `shared/ai_actions.py` now measures `97%` line coverage in its dedicated direct test run after extracting AI helper action-token secret/encode/decode/issue helpers, generic client-action payload sanitization, generic UI action normalization, and the chart-to-dashboard pivot suggestion helper from `app.py`.
- `shared/log_attr_keys.py` now measures `98%` line coverage in its dedicated direct test run after extracting log attribute-key loading, cache priming, cached-key reads, discovered-key persistence, and attribute-map extraction helpers from `app.py`.
- `shared/output_masking.py` now measures `100%` line coverage in its dedicated direct test run after extracting masking settings cache reads/writes, output-masking flag resolution, JSON payload masking, value/string masking, SQL-output masking checks, and the optional SQL-output JSON response helper from `app.py`.
- `shared/otlp_security.py` now measures `99%` line coverage in its dedicated direct test run after extracting secure-context detection, OTLP/RUM origin allowlist matching, ingest-path CORS gating, allowed-method selection, Vary-header deduplication, and shared security/CORS header application from `app.py`.
- `shared/raw_metrics_window.py` now measures `100%` line coverage in its dedicated direct test run after extracting raw-metric retention TTL application, deterministic raw-window registration, copied-table counting, overlapping-window listing, and the raw-window copy worker core from `app.py`.
- `shared/rum_assets.py` now measures `100%` line coverage in its dedicated direct test run after extracting RUM asset name/type sanitization, extension inference, signature payload assembly, HMAC signature generation, and signed-upload verification helpers from `app.py`.
- `shared/agent_state.py` now measures `100%` line coverage in its dedicated direct test run after extracting agent rule loading, single-rule loading, agent run loading, agent run/counter helpers, trigger service-name extraction, and agent GitHub target resolution from `app.py`.
- `shared/agent_work_items.py` now measures `100%` line coverage in its dedicated direct test run after extracting bounded integer parsing, recent-candidate loading, agent-trigger field extraction, issue-match normalization, GitHub work-item dedup key/title helpers, work-item row serialization, work-item persistence, issue URL parsing, context-summary assembly, and Copilot assignment status helpers from `app.py`.
- `shared/ai_pricing.py` now measures `100%` line coverage in its dedicated direct test run after extracting AI model-name normalization, pricing-entry coercion, saved/confirmed pricing loaders, observed-model pricing inference/merge helpers, sensitive-setting detection, and repo-scoped GitHub token helpers from `app.py`.
- `shared/ai_settings.py` now measures `100%` line coverage in its dedicated direct test run after extracting AI setting load/save/all-settings helpers from `app.py` while preserving the app-level compatibility wrappers.
- `shared/app_settings.py` now measures `98%` line coverage in its dedicated direct test run after extracting generic app-setting load/save/delete helpers, monotonic app-setting timestamp generation, JSON string-list setting helpers, masking custom key/pattern loaders and savers, masking settings aggregation, and masking runtime-rule refresh handling from `app.py`.
- `shared/notifications.py` now measures `100%` line coverage in its dedicated direct test run after extracting notification channel loading, condition normalization/parsing, notification rule loading, notification log loading, channel config masking, and per-channel mask-output resolution from `app.py`.
- `shared/sql_where.py` now measures `100%` line coverage in its dedicated direct test run after extracting time-window WHERE fragment helpers, WHERE clause assembly, regex-expression clause assembly, SQL replacement outside quoted literals, AI SQL WHERE normalization, and the central user SQL WHERE validation helper from `app.py`.
- `shared/tag_rules.py` now measures `100%` line coverage in its dedicated direct test run after extracting stable record-id helpers, tag-rule condition JSON parsing, tag-rule loading with legacy fallback semantics, single-condition matching, composite tag-rule matching, and tag-rule attribute-key suggestion ranking from `app.py`.
- `shared/github.py` measures `97.06%` line coverage, which validates Milestone 1 as a successful high-confidence extraction.
- `shared/github_issues.py` now measures `96.4%` line coverage after the dedicated branch tests added in this phase.
- `shared/ci_push.py` now measures `100%` line coverage in its dedicated direct test run after extracting the managed CI push API-key TTL, hashing, status, validation, rotation, revocation, and realtime-flag helpers in Milestone 5.
- `shared/onboarding.py` now measures `99%` line coverage in its dedicated direct test run after extracting dependency parsers, repository inspection helpers, onboarding issue-body builders, onboarding work-item persistence helpers, the shared onboarding issue-result orchestration helper, GitHub repo import/list lookup helpers, create-repo persistence helpers, inspect-repo flow helpers, and create-issues request/realtime helpers in Milestone 4.
- `shared/write_queue.py` now measures `99%` line coverage in its dedicated direct test run after extracting the background write-batch, worker-loop, worker-start, queue-depth, enqueue, and worker-shutdown helpers from `app.py`.
- The latest sequential full-suite validation passed at `1568 passed, 4 skipped`.

## Working Rules

- Extract vertical subsystems, not just route handlers.
- Each extraction must leave `app.py` thinner and move logic into importable modules with direct tests.
- Preserve behaviour with thin compatibility wrappers in `app.py` only when needed for existing routes or tests.
- New subsystem modules should target `95%+` coverage before moving to the next slice.
- Complete the current milestone before starting the next one.
- The only exception to milestone order is a shared-code extraction that directly supports the current milestone, prevents duplicate work, or creates a cleaner seam needed by the current milestone. If that exception is used, record the reason explicitly in this plan.

## Execution Policy

- The default workflow is sequential: finish Milestone 1 before Milestone 2, Milestone 2 before Milestone 3, and so on.
- Earlier out-of-order slices remain valid when they extracted genuinely shared helper code, but they do not change the default sequencing rule going forward.
- After any out-of-order shared extraction, the plan returns to the earliest incomplete milestone before taking additional work from later milestones.

## Execution Order

### Milestone 1: GitHub settings and repository helpers

Status: Implemented in this change.

Measured result:

- `shared/github.py` reached `97.06%` line coverage.

Scope:

- Repository URL parsing and normalization.
- GitHub token expiry parsing and status reporting.
- GitHub token validation request handling.

Deliverables:

- Dedicated helper module.
- Direct unit tests for parsing, normalization, expiry status, and token validation branches.
- Reduced direct dependency from settings routes to `app.py` for pure helper logic.

### Milestone 2: GitHub issue orchestration subsystem

Status: Implemented and now above the target coverage bar.

Measured result:

- `shared/github_issues.py` now measures `96.4%` line coverage.
- Direct tests now cover error paths, fallback branches, and injected-IO variants, so this milestone can be treated as complete and stable.

Scope:

- Issue creation.
- Open-issue fetch.
- Dedupe classification fallback logic.
- Work-item serialization and persistence.
- Copilot assignment decision flow.

Deliverables:

- Dedicated module with injected IO boundaries for GitHub HTTP, LLM calls, and persistence.
- Direct tests for dedupe, assignment limits, create-vs-reuse decisions, and failure handling.

### Milestone 3: AI query, SQL generation, and chart-spec subsystem

Status: Implemented and now above the target coverage bar.

Measured result so far:

- `shared/ai_sql.py` now measures `96.2%` line coverage.
- `shared/ai_chart.py` now measures `97.52%` line coverage in the full-suite report and `98%` in its dedicated direct test run.
- `shared/ai_runtime.py` now measures `99%` line coverage in its dedicated direct test run, and the targeted app regressions for `_call_llm_endpoint`, `_stream_llm_endpoint`, `_check_guard_model`, and query-route monkeypatch compatibility all passed after the wrapper conversion.
- This milestone has now extracted SQL generation, named-query planning, repair prompts, local SQL repair helpers, chart-spec normalization, chart JSON repair, chart-spec generation, shared LLM request assembly, streaming/tool-call parsing, guard decision handling, and DLP endpoint handling while preserving app-level wrappers.

Scope:

- LLM request assembly.
- Guard and DLP decision handling.
- SQL generation and named-query helpers.
- Chart-spec normalization, JSON parsing/repair, and generation helpers.

Deliverables:

- Extracted AI service modules with deterministic unit tests around prompt assembly, response parsing, and chart-spec repair flows.
- Route tests reduced to integration checks.

Remaining work before moving forward:

- Milestone 3 extraction work is complete. Resume the next earliest incomplete milestone, which is Milestone 5.

### Milestone 4: Onboarding and repository inspection subsystem

Status: Implemented and now above the target coverage bar.

Measured result so far:

- `shared/onboarding.py` now measures `99%` line coverage in its dedicated direct test run.
- Existing onboarding app tests remained green alongside the new direct module tests.
- `app.py` now delegates the onboarding issue-body formatting, work-item persistence, repeated onboarding issue result handling, GitHub repo import/list lookup, create-repo persistence, inspect-repo flow, and create-issues request/realtime setup paths to `shared/onboarding.py`, leaving the onboarding routes as thin request/response wrappers.

Scope:

- Repository inspection.
- Dependency parsing.
- Onboarding issue generation.
- Seed/example content helpers.

Deliverables:

- Dedicated onboarding module with fixture-based tests.

### Milestone 5: Remaining high-branch business logic

Status: In progress.

Measured result so far:

- `shared/ai_memory.py` now measures `95%` line coverage in its dedicated direct test run.
- `shared/ai_actions.py` now measures `97%` line coverage in its dedicated direct test run.
- `shared/log_attr_keys.py` now measures `98%` line coverage in its dedicated direct test run.
- `shared/output_masking.py` now measures `100%` line coverage in its dedicated direct test run.
- `shared/chart_specs.py` now measures `96%` line coverage in its dedicated direct test run and `96%` in the latest full-suite report.
- `shared/otlp_security.py` now measures `99%` line coverage in its dedicated direct test run.
- `shared/raw_metrics_window.py` now measures `100%` line coverage in its dedicated direct test run.
- `shared/rum_assets.py` now measures `100%` line coverage in its dedicated direct test run.
- `shared/agent_state.py` now measures `100%` line coverage in its dedicated direct test run.
- `shared/agent_work_items.py` now measures `100%` line coverage in its dedicated direct test run.
- `shared/ai_settings.py` now measures `100%` line coverage in its dedicated direct test run.
- `shared/ai_pricing.py` now measures `100%` line coverage in its dedicated direct test run.
- `shared/app_settings.py` now measures `98%` line coverage in its dedicated direct test run.
- `shared/ci_push.py` now measures `100%` line coverage in its dedicated direct test run.
- `shared/notifications.py` now measures `100%` line coverage in its dedicated direct test run.
- `shared/sql_where.py` now measures `100%` line coverage in its dedicated direct test run.
- `shared/tag_rules.py` now measures `100%` line coverage in its dedicated direct test run.
- `shared/write_queue.py` now measures `99%` line coverage in its dedicated direct test run.
- `shared/dashboards.py` now measures `100%` line coverage in its dedicated direct test run.
- `app.py` now delegates the AI embedding, assistant-meta parsing, semantic-memory matching, memory consolidation, recent-turn loading, and tool-history helpers to `shared/ai_memory.py`.
- `app.py` now delegates AI helper action-token secret/encode/decode/issue helpers, generic client-action payload sanitization, generic UI action normalization, and chart-to-dashboard pivot suggestion logic to `shared/ai_actions.py` while preserving the app-level wrappers exercised by AI helper execution and normalization tests.
- `app.py` now delegates log attribute-key loading, cache priming, cached-key reads, discovered-key persistence, and attribute-map extraction to `shared/log_attr_keys.py` while preserving the app-level cache globals and wrappers exercised by log field-hint and trace/resource attribute-key persistence tests.
- `app.py` now delegates masking settings cache writes/reads, output-masking flag resolution, JSON payload masking, value/string masking, SQL-output masking checks, and the optional SQL-output JSON response helper to `shared/output_masking.py` while preserving the app-level cache state and wrappers exercised by masking preview and toggle regressions.
- `app.py` now delegates chart-query validation, SQL literal/coercion helpers, default/raw chart-spec assembly, chart-spec normalization, builder SQL compilation, compiled-spec validation, template role-map resolution, boolean parsing, chart visual override handling, column-type inference, public dashboard DB-error sanitization, deep placeholder substitution, binding extraction, drilldown timestamp normalization, drilldown metadata attachment, custom ECharts JSON parsing, custom binding resolution, custom drilldown assembly, custom series-point ordering, derived-signal row preparation, and generic chart-template rendering orchestration to `shared/chart_specs.py` while preserving the app-level wrappers exercised by dashboard chart compile, validate, render, named-query, custom ECharts, derived-signal overlay, drilldown metadata, and non-SELECT rejection regressions.
- `app.py` now delegates dashboard row serialization, chart row normalization, dashboard/chart row builders, dashboard template-list assembly, chart form parsing, and query-page add-to-dashboard payload normalization to `shared/dashboards.py` while preserving the app-level wrappers exercised by dashboard listing, chart add/edit/clone/delete, dashboard delete, and query-page save-to-dashboard regressions.
- `app.py` now delegates secure-context detection, OTLP/RUM origin allowlist matching, ingest-path CORS gating, allowed-method selection, Vary-header deduplication, and shared security/CORS header application to `shared/otlp_security.py` while preserving the app-level helpers and after-request hook exercised by the OTLP CORS regression tests.
- `app.py` now delegates raw-metric retention TTL application, deterministic raw-window registration, copied-table counting, overlapping-window listing, and the raw-window copy worker core to `shared/raw_metrics_window.py` while preserving the app-level wrappers and scheduler loop exercised by raw-window, trace-detail, and incident regressions.
- `app.py` now delegates RUM asset name/type sanitization, extension inference, signature payload assembly, HMAC signature generation, and signed-upload verification to `shared/rum_assets.py` while preserving the app-level wrappers exercised by RUM asset upload and download route tests.
- `app.py` now delegates agent rule loading, single-rule loading, agent run loading, agent-run counter helpers, trigger service-name extraction, and agent GitHub target resolution to `shared/agent_state.py`.
- `app.py` now delegates bounded integer parsing, recent work-item candidate loading, agent-trigger field extraction, GitHub work-item dedup/title helpers, work-item row serialization, work-item persistence, issue URL parsing, context-summary assembly, and Copilot assignment status helpers to `shared/agent_work_items.py`.
- `app.py` now delegates generic app-setting load/save/delete helpers, monotonic app-setting timestamp generation, JSON string-list setting helpers, masking custom key/pattern loaders and savers, masking settings aggregation, and masking runtime-rule refresh handling to `shared/app_settings.py` while preserving the app-level globals and wrapper signatures used by the existing tests.
- `app.py` now delegates AI setting load/save/all-settings helpers to `shared/ai_settings.py`.
- `app.py` now delegates AI model-name normalization, pricing-entry coercion, saved/confirmed pricing loading, observed-model pricing inference/merge logic, sensitive-setting detection, and repo-scoped GitHub token load/save helpers to `shared/ai_pricing.py`.
- `app.py` now delegates the managed CI push API-key TTL, expiry, hashing, status, validation, rotation, revocation, and realtime-flag helpers to `shared/ci_push.py`.
- `app.py` now delegates notification channel loading, condition normalization/parsing, notification rule loading, notification log loading, channel config masking, and per-channel mask-output resolution to `shared/notifications.py`.
- `app.py` now delegates time-window WHERE fragment helpers, WHERE clause assembly, regex-expression clause assembly, SQL replacement outside quoted literals, AI SQL WHERE normalization, and central user SQL WHERE validation to `shared/sql_where.py` while preserving the app-level wrappers used by filter validation and query-route tests.
- `app.py` now delegates stable record-id helpers, tag-rule condition parsing, tag-rule loading with legacy fallback semantics, single-condition matching, composite tag-rule matching, and attribute-key suggestion ranking to `shared/tag_rules.py` while preserving the app-level helpers used by tag settings, ingest auto-tagging, and existing tests.
- `app.py` now delegates the write-batch runner, worker loop, worker startup, enqueue, queue-depth, and worker-shutdown helpers to `shared/write_queue.py` while preserving app-level queue APIs for route tests.

Why these slices were taken early:

- They were shared helper extractions with narrow blast radius and high direct-test value.
- They reduced `app.py` without creating a second implementation path that would later need to be merged back into Milestone 3 work.
- They fit the shared-code exception above, but they should be treated as exceptions rather than the ongoing execution order.

Scope:

- Background jobs.
- Release artifact registration helpers.
- Notification/business-policy helpers.
- Remaining app-level orchestration that still mixes storage, HTTP, and formatting.

Deliverables:

- Final shrink pass on `app.py` until it is primarily composition, route registration, and compatibility glue.

Sequencing note:

- Resume this milestone only after Milestone 3 is complete, unless another later slice is clearly shared code required to finish the current active milestone.

## Definition Of Done Per Milestone

- Extracted module exists outside `app.py`.
- Direct tests cover success, validation, and failure branches.
- Existing route behaviour remains green.
- `app.py` loses logic, not just gains wrappers.
- Coverage improves in the extracted area without depending on brittle end-to-end-only tests.