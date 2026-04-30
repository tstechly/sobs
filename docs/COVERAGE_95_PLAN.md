# Coverage To 95 Plan

## Goal

Raise the codebase to sustainable `95%+` coverage by testing business logic directly, not by adding more route-only tests around a monolithic `app.py`.

## Current Baseline

- Overall line coverage is about `76.9%`.
- `app.py` remains the dominant risk and coverage bottleneck at roughly `30k` lines and about `76%` line coverage.
- Route blueprints are a foundation step, but they do not by themselves create the module seams needed for high-confidence unit coverage.

## Measured Current State

- Fresh sequential coverage run from `coverage-latest.xml` measured overall line coverage at `78.06%` versus the prior `76.91%` baseline.
- `app.py` now measures `76.29%` line coverage while the extracted business-logic modules continue to climb above the direct-test target.
- `shared/ai_chart.py` now measures `97.52%` line coverage in the full-suite report, and its dedicated direct test run measured `98%` coverage for the extracted chart-spec parsing/repair/generation helpers.
- `shared/ai_runtime.py` now measures `99%` line coverage in its dedicated direct test run after extracting the shared LLM request assembly, streaming parsing, guard prompt/parsing logic, thinking/token/timeout resolution, and DLP endpoint helpers from `app.py`.
- `shared/ai_sql.py` now measures `96.2%` line coverage after extracting the SQL planner/repair helpers in Milestone 3 phase 1.
- `shared/ai_memory.py` now measures `95%` line coverage in its dedicated direct test run after extracting the AI embedding, assistant-meta parsing, semantic-memory matching, memory consolidation, recent-turn loading, and tool-history helpers in Milestone 5.
- `shared/agent_work_items.py` now measures `96%` line coverage in its dedicated direct test run after extracting bounded integer parsing, agent-trigger field extraction, issue-match normalization, GitHub work-item dedup key/title helpers, work-item row serialization, issue URL parsing, and Copilot assignment status helpers from `app.py`.
- `shared/ai_pricing.py` now measures `100%` line coverage in its dedicated direct test run after extracting AI model-name normalization, pricing-entry coercion, saved/confirmed pricing loaders, observed-model pricing inference/merge helpers, sensitive-setting detection, and repo-scoped GitHub token helpers from `app.py`.
- `shared/ai_settings.py` now measures `100%` line coverage in its dedicated direct test run after extracting AI setting load/save/all-settings helpers from `app.py` while preserving the app-level compatibility wrappers.
- `shared/github.py` measures `97.06%` line coverage, which validates Milestone 1 as a successful high-confidence extraction.
- `shared/github_issues.py` now measures `96.4%` line coverage after the dedicated branch tests added in this phase.
- `shared/ci_push.py` now measures `100%` line coverage in its dedicated direct test run after extracting the managed CI push API-key TTL, hashing, status, validation, rotation, revocation, and realtime-flag helpers in Milestone 5.
- `shared/onboarding.py` now measures `99%` line coverage in its dedicated direct test run after extracting dependency parsers, repository inspection helpers, onboarding issue-body builders, onboarding work-item persistence helpers, the shared onboarding issue-result orchestration helper, GitHub repo import/list lookup helpers, create-repo persistence helpers, inspect-repo flow helpers, and create-issues request/realtime helpers in Milestone 4.
- `shared/write_queue.py` now measures `99%` line coverage in its dedicated direct test run after extracting the background write-batch, worker-loop, worker-start, queue-depth, enqueue, and worker-shutdown helpers from `app.py`.
- The latest sequential full-suite validation passed at `1359 passed, 4 skipped`.

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
- `shared/agent_work_items.py` now measures `96%` line coverage in its dedicated direct test run.
- `shared/ai_settings.py` now measures `100%` line coverage in its dedicated direct test run.
- `shared/ai_pricing.py` now measures `100%` line coverage in its dedicated direct test run.
- `shared/ci_push.py` now measures `100%` line coverage in its dedicated direct test run.
- `shared/write_queue.py` now measures `99%` line coverage in its dedicated direct test run.
- `app.py` now delegates the AI embedding, assistant-meta parsing, semantic-memory matching, memory consolidation, recent-turn loading, and tool-history helpers to `shared/ai_memory.py`.
- `app.py` now delegates bounded integer parsing, agent-trigger field extraction, GitHub work-item dedup/title helpers, work-item row serialization, issue URL parsing, and Copilot assignment status helpers to `shared/agent_work_items.py`.
- `app.py` now delegates AI setting load/save/all-settings helpers to `shared/ai_settings.py`.
- `app.py` now delegates AI model-name normalization, pricing-entry coercion, saved/confirmed pricing loading, observed-model pricing inference/merge logic, sensitive-setting detection, and repo-scoped GitHub token load/save helpers to `shared/ai_pricing.py`.
- `app.py` now delegates the managed CI push API-key TTL, expiry, hashing, status, validation, rotation, revocation, and realtime-flag helpers to `shared/ci_push.py`.
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