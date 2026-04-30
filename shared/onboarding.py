"""Onboarding and repository inspection helpers shared across SOBS modules."""

from __future__ import annotations

import base64
import re
import urllib.parse
from collections.abc import Awaitable, Callable
from typing import Any

from shared.github_issues import _github_api_headers, _safe_json_loads

_SOBS_CI_METADATA_INDICATORS: list[str] = [
    "sobs",
    "register release",
    "source map",
    "sourcemap",
    "artifactType",
    "/v1/apps/",
    "/v1/releases/",
]

_SOBS_CI_OTEL_INDICATORS: list[str] = [
    "opentelemetry",
    "otlp",
    "otel",
    "opentelemetry-sdk",
    "opentelemetry-api",
]


def _parse_requirements_dependencies(content: str) -> list[dict[str, str]]:
    deps: list[dict[str, str]] = []
    seen: set[tuple[str, str]] = set()
    for raw in (content or "").splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if " #" in line:
            line = line.split(" #", 1)[0].strip()
        line = line.split(";", 1)[0].strip()
        if "==" not in line:
            continue
        name, version = line.split("==", 1)
        pkg = name.strip()
        ver = version.strip()
        if not pkg or not ver:
            continue
        key = (pkg.lower(), ver)
        if key in seen:
            continue
        seen.add(key)
        deps.append({"package": pkg, "version": ver, "ecosystem": "PyPI"})
    return deps


def _parse_package_lock_dependencies(content: str) -> list[dict[str, str]]:
    deps: list[dict[str, str]] = []
    seen: set[tuple[str, str]] = set()
    body = _safe_json_loads(content, {})
    if not isinstance(body, dict):
        return deps

    packages = body.get("packages")
    if isinstance(packages, dict):
        for pkg_path, info in packages.items():
            if not isinstance(info, dict) or pkg_path in ("", "."):
                continue
            if not pkg_path.startswith("node_modules/"):
                continue
            name = pkg_path.split("node_modules/")[-1]
            version = str(info.get("version") or "").strip()
            if not name or not version:
                continue
            key = (name.lower(), version)
            if key in seen:
                continue
            seen.add(key)
            deps.append({"package": name, "version": version, "ecosystem": "npm"})

    if deps:
        return deps

    legacy = body.get("dependencies")
    if not isinstance(legacy, dict):
        return deps
    for name, info in legacy.items():
        if not isinstance(info, dict):
            continue
        version = str(info.get("version") or "").strip()
        if not name or not version:
            continue
        key = (str(name).lower(), version)
        if key in seen:
            continue
        seen.add(key)
        deps.append({"package": str(name), "version": version, "ecosystem": "npm"})
    return deps


def _parse_go_sum_dependencies(content: str) -> list[dict[str, str]]:
    deps: list[dict[str, str]] = []
    seen: set[tuple[str, str]] = set()
    for raw in (content or "").splitlines():
        line = raw.strip()
        if not line:
            continue
        parts = line.split()
        if len(parts) < 2:
            continue
        module_name = parts[0].strip()
        module_version = parts[1].strip()
        if module_version.endswith("/go.mod"):
            module_version = module_version[: -len("/go.mod")]
        if not module_name or not module_version:
            continue
        key = (module_name.lower(), module_version)
        if key in seen:
            continue
        seen.add(key)
        deps.append({"package": module_name, "version": module_version, "ecosystem": "Go"})
    return deps


def _parse_gemfile_lock_dependencies(content: str) -> list[dict[str, str]]:
    deps: list[dict[str, str]] = []
    seen: set[tuple[str, str]] = set()
    in_specs = False
    for raw in (content or "").splitlines():
        if raw.strip() == "specs:":
            in_specs = True
            continue
        if not in_specs:
            continue
        if raw and not raw.startswith(" "):
            break
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        match = re.match(r"^([A-Za-z0-9_\-\.]+)\s+\(([^)]+)\)", line)
        if not match:
            continue
        name = match.group(1).strip()
        version = match.group(2).split(",", 1)[0].strip()
        if not name or not version:
            continue
        key = (name.lower(), version)
        if key in seen:
            continue
        seen.add(key)
        deps.append({"package": name, "version": version, "ecosystem": "RubyGems"})
    return deps


def _decode_github_contents_payload(payload: dict[str, Any]) -> bytes:
    content = payload.get("content")
    encoding = str(payload.get("encoding") or "").lower()
    if not isinstance(content, str) or encoding != "base64":
        return b""
    try:
        return base64.b64decode(content, validate=False)
    except Exception:
        return b""


async def _github_list_directory(
    github_token: str,
    owner: str,
    repo: str,
    path: str,
    *,
    get_async_http_client: Callable[[], Awaitable[Any]],
) -> tuple[list[dict[str, Any]], str]:
    client = await get_async_http_client()
    encoded = urllib.parse.quote(path, safe="/")
    try:
        resp = await client.get(
            f"https://api.github.com/repos/{owner}/{repo}/contents/{encoded}",
            headers=_github_api_headers(github_token),
            timeout=12,
        )
        if resp.status_code != 200:
            return [], f"GitHub API returned {resp.status_code} for {path}"
        data = resp.json() if resp.content else []
        return (data if isinstance(data, list) else []), ""
    except Exception as exc:
        return [], f"GitHub API request failed for {path}: {exc}"


async def _github_file_text(
    github_token: str,
    owner: str,
    repo: str,
    path: str,
    *,
    get_async_http_client: Callable[[], Awaitable[Any]],
) -> tuple[str, str]:
    client = await get_async_http_client()
    encoded = urllib.parse.quote(path, safe="/")
    try:
        resp = await client.get(
            f"https://api.github.com/repos/{owner}/{repo}/contents/{encoded}",
            headers=_github_api_headers(github_token),
            timeout=12,
        )
        if resp.status_code != 200:
            return "", f"GitHub API returned {resp.status_code} for {path}"
        data = resp.json() if resp.content else {}
        if not isinstance(data, dict):
            return "", f"Unexpected GitHub API response for {path}"
        raw = _decode_github_contents_payload(data)
        return (raw.decode("utf-8", errors="replace") if raw else ""), ""
    except Exception as exc:
        return "", f"GitHub API request failed for {path}: {exc}"


async def _inspect_repo_for_onboarding(
    github_token: str,
    owner: str,
    repo: str,
    *,
    get_async_http_client: Callable[[], Awaitable[Any]],
    github_repo_supports_copilot_assignment: Callable[[str, str], Awaitable[bool]],
) -> dict[str, Any]:
    if not github_token or not owner or not repo:
        return {
            "has_github_actions": False,
            "sobs_ci_found": False,
            "sobs_otel_found": False,
            "copilot_available": False,
            "workflow_files": [],
            "error": "GitHub token or repository not configured",
        }

    workflow_entries, workflow_error = await _github_list_directory(
        github_token,
        owner,
        repo,
        ".github/workflows",
        get_async_http_client=get_async_http_client,
    )
    if workflow_error and " 404 " not in f" {workflow_error} ":
        return {
            "has_github_actions": False,
            "sobs_ci_found": False,
            "sobs_otel_found": False,
            "copilot_available": False,
            "workflow_files": [],
            "error": workflow_error,
        }
    workflow_files = [
        entry["name"]
        for entry in workflow_entries
        if isinstance(entry, dict) and str(entry.get("name", "")).endswith((".yml", ".yaml"))
    ]
    has_github_actions = bool(workflow_files)

    sobs_ci_found = False
    sobs_otel_found = False
    inspect_error = ""
    for filename in workflow_files[:10]:
        content, content_error = await _github_file_text(
            github_token,
            owner,
            repo,
            f".github/workflows/{filename}",
            get_async_http_client=get_async_http_client,
        )
        if content_error and not inspect_error:
            inspect_error = content_error
            continue
        lower = content.lower()
        if not sobs_ci_found and any(ind in lower for ind in _SOBS_CI_METADATA_INDICATORS):
            sobs_ci_found = True
        if not sobs_otel_found and any(ind in lower for ind in _SOBS_CI_OTEL_INDICATORS):
            sobs_otel_found = True
        if sobs_ci_found and sobs_otel_found:
            break

    if not sobs_otel_found:
        for check_path in ("requirements.txt", "package.json", "go.mod", "pom.xml", "build.gradle"):
            content, content_error = await _github_file_text(
                github_token,
                owner,
                repo,
                check_path,
                get_async_http_client=get_async_http_client,
            )
            if content_error and " 404 " not in f" {content_error} " and not inspect_error:
                inspect_error = content_error
            if content and any(ind in content.lower() for ind in _SOBS_CI_OTEL_INDICATORS):
                sobs_otel_found = True
                break

    copilot_available = await github_repo_supports_copilot_assignment(github_token, f"{owner}/{repo}")

    return {
        "has_github_actions": has_github_actions,
        "sobs_ci_found": sobs_ci_found,
        "sobs_otel_found": sobs_otel_found,
        "copilot_available": copilot_available,
        "workflow_files": workflow_files,
        "error": inspect_error,
    }


def _build_ci_metadata_issue_body(owner: str, repo: str, has_github_actions: bool) -> str:
    ci_section = (
        """
## CI Provider

This repository uses **GitHub Actions**. Use polling mode first, then optionally add
realtime push once security approval for outbound CI calls is in place.
"""
        if has_github_actions
        else """
## CI Provider

No GitHub Actions workflows were detected. The steps below are provider-agnostic and can
be adapted for Jenkins, CircleCI, GitLab CI, Buildkite, or other CI systems.
"""
    )

    return f"""# Sobs CI Metadata Setup

This issue defines how `{owner}/{repo}` should integrate with Sobs CI metadata.

Sobs supports two modes:

1. **Polling mode (default)**
     - No CI workflow edits required.
    - Sobs reads GitHub run/check state and uses conditional requests
      (`ETag`/`If-None-Match`) to keep polling efficient.
     - Best starting point when CI outbound calls require security approval.

2. **Realtime push mode (optional)**
     - CI posts release metadata directly to Sobs with a Sobs API key.
     - Faster and deterministic release visibility.
     - Optional GitHub webhook can be added for faster refresh triggers.

> Keep polling mode available as fallback even if realtime push is enabled.

{ci_section}

---

## Step 1 - Baseline repository setup in Sobs

- Verify repository URL in **Settings -> Repositories**
- Verify GitHub token is valid for read operations
- Verify token expiry tracking is configured

---

## Step 2 - Polling mode (no CI changes)

No workflow updates are required for this step.

- Confirm Sobs can read workflow/check state for this repo
- Confirm Sobs conditional polling is enabled and stable
- Confirm CVE/release views continue to populate

---

## Step 3 - Register a release (optional realtime push mode)

If CI outbound integration is approved, add these CI secrets:

| Secret | Description |
|--------|-------------|
| `SOBS_URL` | Base URL of your Sobs instance (for example `https://sobs.internal`) |
| `SOBS_INGEST_API_KEY` | Sobs ingest API key from Settings -> Repositories |
| `SOBS_APP_ID` | Application ID from Settings -> Repositories |

Use this push call in CI:

```bash
curl -sS -X POST "${{SOBS_URL}}/v1/apps/${{SOBS_APP_ID}}/releases" \\
        -H "X-API-Key: ${{SOBS_INGEST_API_KEY}}" \\
        -H "Content-Type: application/json" \\
        -d '{{
                "version":    "${{VERSION}}",
                "commitSha":  "${{COMMIT_SHA}}",
                "buildId":    "${{BUILD_ID}}",
                "environment": "production"
        }}'
```

Best practice requirements for release identity:

- Use a release `version` that exactly matches deployed runtime identity (for example image tag or Git tag).
- Keep `commitSha` and `buildId` immutable per published release.
- Propagate the same release identifier into OTEL `service.version` so Sobs can
    correlate CVEs to observed runtime activity.
- For containerized workloads, include image digest/tag in release metadata where available.

---

## Step 4 - Upload dependency lockfile metadata

Lockfile metadata improves release-scoped CVE enrichment. Best practice is to
extract resolved dependency snapshots from the built container image for each
target architecture (for example linux/amd64 and linux/arm64), then register
each snapshot with provenance fields (size/checksum/storageRef/platform/architecture):

For GitHub Actions, prefer a visible artifact directory/path for dependency
snapshots (for example `sobs-release/pip-freeze-linux-amd64.txt`). Hidden
directories such as `.sobs-release/` are excluded by `actions/upload-artifact`
unless `include-hidden-files: true` is set explicitly.

```bash
curl -sS -X POST "${{SOBS_URL}}/v1/releases/${{RELEASE_ID}}/artifacts/meta" \\
        -H "X-API-Key: ${{SOBS_INGEST_API_KEY}}" \\
        -H "Content-Type: application/json" \\
        -d '{{
                "artifactType": "dependencies-lockfile",
                                "name": "pip-freeze-linux-amd64",
                                "contentType": "application/json",
                                "size": ${{LOCKFILE_SIZE}},
                                "storageRef": "ci://artifacts/pip-freeze-linux-amd64.txt",
                                "checksumSha256": "${{LOCKFILE_SHA256}}",
                                "platform": "linux",
                                "architecture": "amd64",
                                "metadata": {{
                                    "dependencies": ${{RESOLVED_DEPS_JSON}}
                                }}
        }}'
```

Repeat per architecture (for example `pip-freeze-linux-arm64`) to ensure CVE
tracking reflects what is actually shipped for each target platform.

Dependency capture requirements:

- Derive snapshots from the built/published container image, not from a host-only
    resolver run.
- Track per-arch snapshots independently for multi-arch releases.
- Fail CI early if any expected dependency snapshot file is missing or empty
    before artifact upload and metadata registration.
- Verify the dependency snapshot artifact upload succeeds before release/artifact
    registration continues.
- Include provenance fields (`storageRef`, `checksumSha256`, `size`, `platform`,
  `architecture`) on every dependency artifact.

---

## Step 5 - Upload JS source maps (web front-end only)

Source maps let Sobs resolve minified stack traces to original source locations:

```bash
curl -sS -X POST "${{SOBS_URL}}/v1/releases/${{RELEASE_ID}}/artifacts/meta" \\
    -H "X-API-Key: ${{SOBS_INGEST_API_KEY}}" \\
    -H "Content-Type: application/json" \\
    -d '{{
        "artifactType": "js_sourcemap",
        "name": "app.min.js.map",
        "contentType": "application/json",
        "size": ${{SOURCEMAP_SIZE}},
        "checksumSha256": "${{SOURCEMAP_SHA256}}",
        "storageRef": "ci://artifacts/app.min.js.map"
    }}'
```

Source map capture requirements:

- Register maps from the same build outputs that were deployed.
- Include `size` and `checksumSha256` for provenance and troubleshooting.

---

## Step 6 - Optional webhook acceleration

If repository admins approve webhook setup, add a GitHub webhook to Sobs for push/workflow events.

- This is optional and should not block onboarding.
- Admin/webhook-write permissions are usually required.
- Keep polling mode enabled as fallback.

---

## Step 7 - Trigger a CVE scan (optional)

```bash
curl -sS -X POST "${{SOBS_URL}}/api/enrichment/cve/scan" \\
        -H "X-API-Key: ${{SOBS_INGEST_API_KEY}}" \\
        -H "Content-Type: application/json" \\
        -d '{{}}'
```

---

## Step 8 - OTEL-linked CVE impact triage

Use CVE results together with OTEL/log evidence to separate:

- **Confirmed impact candidates**: vulnerable package/version appears in release
    metadata and related services show active OTEL/log usage for that runtime.
- **Latent exposure**: vulnerable package/version exists in release metadata but no
    current OTEL/log evidence of active usage.

This lets teams prioritize "must patch now" findings while still tracking latent risk.

Recommended correlation keys:

- `service.name`
- `service.version` (must match the registered release version)
- `deployment.environment`
- release metadata (`version`, `commitSha`, `buildId`, image tag/digest)

---

## Manual verification checklist

- Confirm first pushed release appears in Sobs
- Confirm lockfile artifact metadata is visible for each architecture
- Confirm dependency snapshot artifacts upload successfully from non-hidden CI paths
- Confirm dependency artifacts include provenance fields (size/checksum/storageRef/platform/architecture)
- Confirm release version matches OTEL `service.version`
- Confirm CVE findings reflect the container-derived dependency snapshots
- Confirm CVE review distinguishes confirmed impact candidates vs latent exposure
- Confirm polling-only fallback works if CI push or webhook path is blocked

---

*This issue was created automatically by the Sobs Onboarding Wizard for repository \
`{owner}/{repo}`.*
"""


def _build_otel_audit_issue_body(owner: str, repo: str) -> str:
    return f"""# OTEL & RUM Telemetry Audit

This issue requests a comprehensive audit of the `{owner}/{repo}` repository to identify
gaps in observability coverage and add best-practice OpenTelemetry (OTEL) instrumentation,
Real User Monitoring (RUM), and AI telemetry.

---

## Audit Checklist

### 1. Core OTEL SDK Setup

- [ ] Install and configure the OTEL SDK for the primary language(s) used in this repository
- [ ] Set up a `TracerProvider` with OTLP export pointing to Sobs (`<SOBS_URL>:4317`)
- [ ] Set up a `LoggerProvider` (or bridge) so structured application logs flow through OTEL
- [ ] Set up a `MeterProvider` for custom metrics (request counts, error rates, latency histograms)
- [ ] Ensure `service.name`, `service.version`, and `deployment.environment` resource attributes
      are set

**Example (Python):**
```python
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter

provider = TracerProvider(
    resource=Resource({{"service.name": "my-service", "service.version": "1.0.0"}})
)
provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter(endpoint="http://sobs:4317")))
trace.set_tracer_provider(provider)
```

---

### 2. Web Front-End — RUM Snippet (if applicable)

If this repository contains a web front-end (HTML, React, Vue, Angular, etc.):

- [ ] Add the Sobs RUM snippet to the `<head>` of every page (or the root layout component)
- [ ] Configure RUM to capture **console logs**, **JavaScript stack traces**, **navigation
      breadcrumbs**, **Web Vitals** (LCP, CLS, INP, TTFB, FCP), **screenshots** (on error),
      and **session replays**
- [ ] Set `service`, `environment`, and `release` attributes in the RUM config

**Sobs RUM snippet:**
```html
<script>
  window.SobsRumConfig = {{
    endpoint: '<SOBS_URL>/rum',
    service:  'my-frontend',
    env:      'production',
    release:  '{{{{ APP_VERSION }}}}',
    captureConsole: true,
    captureErrors:  true,
    captureReplays: true,
    captureScreenshots: true
  }};
</script>
<script src="<SOBS_URL>/static/rum.min.js"></script>
```

---

### 3. AI / LLM Workloads (if applicable)

If this repository makes LLM API calls (OpenAI, Anthropic, Azure OpenAI, etc.):

- [ ] Use `opentelemetry-instrumentation-openai` (or equivalent) to auto-instrument LLM calls
- [ ] Emit OTEL `gen_ai.*` semantic-convention attributes on every LLM span:
      `gen_ai.system`, `gen_ai.request.model`, `gen_ai.usage.input_tokens`,
      `gen_ai.usage.output_tokens`
- [ ] Propagate trace context into LLM calls so the Sobs AI page can correlate prompts with
      application traces
- [ ] Record prompt templates and response hashes (not full content) as span attributes for
      traceability
- [ ] Ensure no PII / secrets are emitted in span attributes

---

### 4. Infrastructure & Web Logs (if applicable)

For infrastructure services (proxies, gateways, databases, queues):

- [ ] Add OTEL log bridge or structured JSON logging shipped via OTLP to Sobs
- [ ] Include `http.method`, `http.route`, `http.status_code`, `net.peer.ip` attributes
      for HTTP services
- [ ] For databases: include `db.system`, `db.statement` (redacted), `db.name` span attributes
- [ ] For message queues: include `messaging.system`, `messaging.destination` span attributes

---

### 5. Error & Exception Capture

- [ ] Call `span.record_exception(exc)` and `span.set_status(StatusCode.ERROR)` in all
      exception handlers
- [ ] Ensure unhandled exceptions are captured and forwarded to the Sobs errors endpoint
- [ ] Add a global uncaught-exception handler that emits a final error span before process exit

---

### 6. Telemetry Verification

After implementing the above:

- [ ] Verify traces appear on the Sobs **Traces** page
- [ ] Verify logs appear on the Sobs **Logs** page
- [ ] Verify metrics appear on the Sobs **Metrics** page
- [ ] Verify RUM events appear on the Sobs **RUM** page (if web front-end added)
- [ ] Verify AI calls appear on the Sobs **AI** page (if LLM workload added)
- [ ] Run the CVE scan and verify findings appear on the Sobs **CVE** page

---

## What remains manual

- Reviewing each checklist item and confirming it applies to this repository's technology stack
- Testing that telemetry flows correctly end-to-end
- Removing any accidentally captured PII or secrets from span attributes

---

*This issue was created automatically by the Sobs Onboarding Wizard for repository \
`{owner}/{repo}`.*
"""
