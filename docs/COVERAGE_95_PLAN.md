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
- `shared/ai_sql.py` now measures `96.2%` line coverage after extracting the SQL planner/repair helpers in Milestone 3 phase 1.
- `shared/github.py` measures `97.06%` line coverage, which validates Milestone 1 as a successful high-confidence extraction.
- `shared/github_issues.py` now measures `96.4%` line coverage after the dedicated branch tests added in this phase.
- `shared/onboarding.py` now measures `98%` line coverage in its dedicated direct test run after extracting dependency parsers, repository inspection helpers, onboarding issue-body builders, onboarding work-item persistence helpers, the shared onboarding issue-result orchestration helper, GitHub repo import/list lookup helpers, and create-repo persistence helpers in Milestone 4.
- The latest sequential full-suite validation passed at `1359 passed, 4 skipped`.

## Working Rules

- Extract vertical subsystems, not just route handlers.
- Each extraction must leave `app.py` thinner and move logic into importable modules with direct tests.
- Preserve behaviour with thin compatibility wrappers in `app.py` only when needed for existing routes or tests.
- New subsystem modules should target `95%+` coverage before moving to the next slice.

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

Status: In progress, with SQL planner/repair helpers extracted in phase 1 and chart-spec helpers extracted in phase 2.

Measured result so far:

- `shared/ai_sql.py` now measures `96.2%` line coverage.
- `shared/ai_chart.py` now measures `97.52%` line coverage in the full-suite report and `98%` in its dedicated direct test run.
- This phase has extracted SQL generation, named-query planning, repair prompts, local SQL repair helpers, chart-spec normalization, chart JSON repair, and chart-spec generation while preserving app-level wrappers.

Scope:

- LLM request assembly.
- Guard and DLP decision handling.
- SQL generation and named-query helpers.
- Chart-spec normalization, JSON parsing/repair, and generation helpers.

Deliverables:

- Extracted AI service modules with deterministic unit tests around prompt assembly, response parsing, and chart-spec repair flows.
- Route tests reduced to integration checks.

### Milestone 4: Onboarding and repository inspection subsystem

Status: In progress, with dependency parsing, GitHub contents inspection, onboarding repository-readiness checks, onboarding issue-body builders, onboarding work-item persistence, shared onboarding issue-result orchestration, GitHub repo import/list lookup helpers, and create-repo persistence helpers extracted in this phase.

Measured result so far:

- `shared/onboarding.py` now measures `98%` line coverage in its dedicated direct test run.
- Existing onboarding app tests remained green alongside the new direct module tests.
- `app.py` now delegates the onboarding issue-body formatting, work-item persistence, repeated onboarding issue result handling, GitHub repo import/list lookup, and create-repo persistence paths to `shared/onboarding.py`, leaving a smaller route/orchestration surface behind.

Scope:

- Repository inspection.
- Dependency parsing.
- Onboarding issue generation.
- Seed/example content helpers.

Deliverables:

- Dedicated onboarding module with fixture-based tests.

### Milestone 5: Remaining high-branch business logic

Scope:

- Background jobs.
- Release artifact registration helpers.
- Notification/business-policy helpers.
- Remaining app-level orchestration that still mixes storage, HTTP, and formatting.

Deliverables:

- Final shrink pass on `app.py` until it is primarily composition, route registration, and compatibility glue.

## Definition Of Done Per Milestone

- Extracted module exists outside `app.py`.
- Direct tests cover success, validation, and failure branches.
- Existing route behaviour remains green.
- `app.py` loses logic, not just gains wrappers.
- Coverage improves in the extracted area without depending on brittle end-to-end-only tests.