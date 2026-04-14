# SOBS Beta 2 (v0.1.0-beta.2)

Status: Pre-release
Date: 2026-04-14

SOBS v0.1.0-beta.2 focuses on hardening release operations, extending MCP/AI capabilities, and improving runtime reliability.

## Highlights

- Added MCP server support for Copilot-backed workflows and expanded MCP settings UX fixes.
- Added AI model pricing auto-discovery and inferred-state management.
- Expanded privacy protections with a masking layer for UI output, notifications, and GitHub issue payloads.
- Improved query performance on Summary, RUM, Traces, and Errors via SQL pushdown and strategy optimizations.
- Improved release and runtime reliability with container packaging/startup fixes and safer GHCR retention behavior.
- Added broader in-app help coverage and fixed Work Items UTC-to-local rendering.
- Increased CI quality checks by adding `djlint` enforcement.

## Beta Scope

- This remains a beta pre-release intended for staging and controlled production validation.
- APIs and UI behavior may continue to evolve before stable v0.1.0.
- Feedback on MCP workflows, release artifact handling, and query behavior is encouraged.

## Getting Started

- Deploy artifacts tagged v0.1.0-beta.2.
- Validate MCP settings, API key expiry behavior, and visibility/contrast in both light and dark themes.
- Run smoke checks for query performance paths (Summary, Errors, Traces, RUM) and Work Items timestamp rendering.

## Known Gaps

- Beta-only capabilities still do not carry a long-term compatibility guarantee.
- Additional release-automation hardening is expected before stable v0.1.0.

## Included Change Set (Recent)

- #227 Fix container startup by copying `mcp.py` module into image.
- #225 Disable overly aggressive GHCR cleanup job.
- #224 Fix GHCR cleanup behavior that removed latest SHA-tagged images.
- #222 Add `djlint` to CI and pre-commit checks.
- #220 Fix Work Items UTC date rendering pipeline.
- #213 Add onboarding wizard automated CI metadata setup and Copilot assignment.
- #207 Add AI model pricing auto-discovery and inferred-state management.
- #205 Prevent `UnicodeDecodeError` on `/errors` for invalid UTF-8 payloads.
- #203 Optimize Summary, RUM, and Traces SQL pushdown.
- #202 Optimize Errors page query strategy.
- #199 Add MCP server for Copilot access to telemetry tables.
- #190 Include `masking.py` module in Docker image.
- #188 Add third-party dependency license audit (`NOTICES`).
- #185 Add help pages for major user-facing pages.
- #177 Add PII/secret masking across OTEL UI, notifications, and GitHub issues.
