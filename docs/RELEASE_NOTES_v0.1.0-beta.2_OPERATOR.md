# SOBS v0.1.0-beta.2 Operator Notes

Status: Pre-release
Date: 2026-04-14
Target: Platform operators, SRE, and deployment owners

This document provides operational guidance for validating and rolling out SOBS v0.1.0-beta.2.

## What Changed

### MCP and AI Operations

- MCP server integration enables Copilot-oriented workflows over telemetry data.
- MCP settings UX and key lifecycle behavior were improved (including key expiry handling and visual contrast fixes).
- AI model pricing metadata is now auto-discovered and inferred-state aware.

### Runtime and Container Reliability

- Docker image packaging now explicitly includes `mcp.py` and `masking.py` to prevent startup/runtime failures.
- Error decoding paths were hardened to avoid `/errors` failures on invalid UTF-8 bytes.

### Query and Page Performance

- SQL pushdown and query path optimizations improve responsiveness for Summary, RUM, Traces, and Errors views.
- Work Items timestamp rendering now correctly preserves UTC storage and local display conversion.

### Release and CI Pipeline

- CI now includes `djlint` checks.
- GHCR cleanup behavior was corrected to avoid deleting the newest/latest images.

## Pre-Deployment Checklist

- Confirm image/artifact tag is v0.1.0-beta.2 in every environment.
- Validate MCP settings panel behavior and API key expiry flows.
- Confirm error views continue rendering for malformed/legacy payloads.
- Review CI policy changes where local pipelines mirror project checks.
- Verify registry retention/cleanup behavior aligns with your artifact policy.

## Post-Deployment Smoke Tests

- Ingestion: verify traces, metrics, logs, and RUM events continue arriving.
- MCP: verify MCP server requests, auth, and telemetry query access.
- Query: verify Summary, Errors, Traces, and RUM response times and result correctness.
- Errors page: verify malformed byte payloads do not break render paths.
- Work Items: verify timestamps display correctly in local timezone.

## Rollout Strategy

- Start in staging.
- Promote to a limited production slice.
- Monitor ingestion lag, query latency, error rates, and container start/restart health.
- Proceed to wider rollout only after at least one full business cycle of stable behavior.

## Risk and Compatibility Notes

- This is a beta release and may include behavior changes before stable v0.1.0.
- Compatibility expectations for beta-only capabilities remain best-effort.
- Keep rollback artifacts and previous deployment manifests available.

## Rollback Guidance

- Revert to the previous known-good release tag (v0.1.0-beta.1).
- Restore prior deployment manifests/configuration values if modified for beta.2.
- Validate ingestion continuity, MCP access behavior, and query correctness after rollback.

## Source Changes Referenced

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
- #185 Add help pages for major user-facing pages.
- #177 Add PII/secret masking across OTEL UI, notifications, and GitHub issues.
