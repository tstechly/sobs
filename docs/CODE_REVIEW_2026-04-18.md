# SOBS Code Review (2026-04-18)

## Scope

This review focused on core backend/auth/query-risk paths and test coverage in the current Python service.

Reviewed areas:
- `app.py` authentication and CSRF logic
- `app.py` user-supplied SQL filter validation path
- `mcp.py` map parsing normalization
- `tests/test_app.py` auth-focused coverage

Executed validation:
- `./.venv/bin/pytest -q tests/test_mcp.py tests/test_app.py -k "external_auth or session_cookie"`
- Result: `10 passed, 999 deselected`

## Findings

### 1) High: External-auth session cookie fallback ignores configured session cookie name

- Location:
  - `app.py:404` sets configurable cookie name via `SESSION_COOKIE_NAME`
  - `app.py:7710` reads hardcoded cookie key `session`
- Impact:
  - If `SOBS_SESSION_COOKIE_NAME` is changed from default, external-auth browser fallback silently fails and produces unauthorized responses.
  - This creates production auth regressions during hardening or platform-standard cookie name adoption.
- Recommendation:
  - Replace hardcoded `session` lookup with `app.config["SESSION_COOKIE_NAME"]` (with fallback to `session` only for backward compatibility if desired).
  - Add tests for non-default cookie names.

### 2) High: CSRF same-origin check trusts forwarded host/proto headers unconditionally

- Location:
  - `app.py:7578` to `app.py:7585`
- Impact:
  - `_same_origin_request` prioritizes `X-Forwarded-Host` and `X-Forwarded-Proto` without checking trusted proxy boundaries.
  - In direct deployments (or misconfigured proxies), a client can supply spoofed forwarded headers and satisfy the origin check unexpectedly.
- Recommendation:
  - Only trust forwarded headers when an explicit trusted-proxy mode is enabled.
  - Default to `request.host` / `request.scheme` for direct mode.
  - Add tests covering spoofed forwarded headers in non-proxy mode.

### 3) Medium: User SQL WHERE validator allows expensive subqueries and broad expressions

- Location:
  - `app.py:11365` to `app.py:11401`
  - `app.py:23355` (`SELECT 1 ... WHERE {safe_sql}` probe)
- Impact:
  - `_validate_user_sql_where` blocks write/DDL keywords but intentionally allows `SELECT` in WHERE fragments.
  - This enables deeply nested/expensive expressions in page filters, which can degrade responsiveness or trigger resource pressure in shared environments.
- Recommendation:
  - Add complexity guards for filter fragments (length, nested query depth, function allowlist for filter-bar paths).
  - Enforce lightweight execution timeout/resource guard for validation probe paths.
  - Keep full SQL freedom only in the dedicated NLQ/query surface with existing allowlist controls.

## Coverage Gaps

- No tests found for `_same_origin_request` forwarded-header trust behavior.
- Current external-auth cookie fallback tests only use `session` and do not verify behavior when `SESSION_COOKIE_NAME` is customized.

## Migration Follow-up (Single PR)

1. Implement auth reliability fixes in Go migration scope:
  - session cookie-name consistency + tests.
2. Implement CSRF trust-boundary behavior in Go migration scope:
  - trusted proxy switch + tests for spoofed forwarded headers.
3. Implement filter safety controls in Go migration scope:
  - complexity limits + timeout/resource guard for filter validation.
