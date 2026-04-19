#!/usr/bin/env bash
# start_ollama_ai_test.sh — Start local SOBS against a local Ollama server.
#
# This script:
# 1) Validates local Ollama availability.
# 2) Exports SOBS AI/Guard env vars using Ollama's OpenAI-compatible /v1 endpoint.
# 3) Runs SOBS (or a custom command).
#
# Kubernetes is not used here. No kubectl setup is required.
#
# Usage:
#   ./scripts/start_ollama_ai_test.sh
#   ./scripts/start_ollama_ai_test.sh -- python app.py
#   SOBS_RUNTIME=go ./scripts/start_ollama_ai_test.sh
#   OLLAMA_BASE_URL=http://127.0.0.1:11434 ./scripts/start_ollama_ai_test.sh -- .venv/bin/python app.py
#
# Optional demo app controls:
#   START_EXAMPLE_APP=1 (default) launches a local Flask demo app for RUM/replay testing.
#   EXAMPLE_APP_PORT=5005 sets the demo app port.
#
# Optional demo OTEL auto-instrumentation controls:
#   EXAMPLE_APP_ENABLE_OTEL=1 (default) uses opentelemetry-instrument for the demo app.
#   EXAMPLE_APP_OTEL_AUTO_INSTALL=1 (default) auto-installs required OTEL Python packages.
#   EXAMPLE_APP_OTEL_SERVICE_NAME defaults to sobs-rum-replay-demo.
#   EXAMPLE_APP_OTEL_TRACES_EXPORTER defaults to console,otlp.
#   EXAMPLE_APP_OTEL_METRICS_EXPORTER defaults to console.
#   EXAMPLE_APP_OTEL_TRACES_ENDPOINT defaults to http://127.0.0.1:44317/v1/traces.
#   EXAMPLE_APP_OTEL_LOGS_ENDPOINT defaults to http://127.0.0.1:44317/v1/logs.
#   EXAMPLE_APP_OTEL_HEADERS can set OTLP headers (e.g. X-API-Key=...).

set -euo pipefail

choose_python() {
  if [[ -x .venv/bin/python ]]; then
    printf '%s' .venv/bin/python
    return 0
  fi
  if command -v python3 >/dev/null 2>&1; then
    printf '%s' python3
    return 0
  fi
  printf '%s' python
}

SOBS_PYTHON="${SOBS_PYTHON:-$(choose_python)}"

# Runtime selection for SOBS server startup.
# - python (default): starts app.py
# - go: starts cmd/sobs and sets compatible local defaults
SOBS_RUNTIME="${SOBS_RUNTIME:-python}"

OLLAMA_BASE_URL="${OLLAMA_BASE_URL:-http://127.0.0.1:11434}"
OLLAMA_TAGS_URL="${OLLAMA_BASE_URL%/}/api/tags"

# Default to practical local models; override as needed.
SOBS_AI_MODEL="${SOBS_AI_MODEL:-llama3.1:8b}"
SOBS_AI_GUARD_MODEL="${SOBS_AI_GUARD_MODEL:-llama-guard3:1b}"

# Optional auto-pull of models before launch.
OLLAMA_PULL_MODELS="${OLLAMA_PULL_MODELS:-0}"

# Demo app (browser RUM/replay test surface).
START_EXAMPLE_APP="${START_EXAMPLE_APP:-1}"
EXAMPLE_APP_PORT="${EXAMPLE_APP_PORT:-5005}"
EXAMPLE_APP_SOBS_BASE_URL="${EXAMPLE_APP_SOBS_BASE_URL:-}"
EXAMPLE_APP_SCRIPT="${EXAMPLE_APP_SCRIPT:-examples/python/rum_replay_test_app.py}"
EXAMPLE_APP_PYTHON="${EXAMPLE_APP_PYTHON:-$SOBS_PYTHON}"
EXAMPLE_APP_LOG="${EXAMPLE_APP_LOG:-/tmp/sobs-rum-replay-demo.log}"
EXAMPLE_APP_ENABLE_OTEL="${EXAMPLE_APP_ENABLE_OTEL:-1}"
EXAMPLE_APP_OTEL_AUTO_INSTALL="${EXAMPLE_APP_OTEL_AUTO_INSTALL:-1}"
EXAMPLE_APP_OTEL_SERVICE_NAME="${EXAMPLE_APP_OTEL_SERVICE_NAME:-sobs-rum-replay-demo}"
EXAMPLE_APP_OTEL_TRACES_EXPORTER="${EXAMPLE_APP_OTEL_TRACES_EXPORTER:-console,otlp}"
EXAMPLE_APP_OTEL_METRICS_EXPORTER="${EXAMPLE_APP_OTEL_METRICS_EXPORTER:-console}"
EXAMPLE_APP_OTEL_LOGS_EXPORTER="${EXAMPLE_APP_OTEL_LOGS_EXPORTER:-console,otlp}"
EXAMPLE_APP_OTEL_PROTOCOL="${EXAMPLE_APP_OTEL_PROTOCOL:-http/protobuf}"
EXAMPLE_APP_OTEL_TRACES_ENDPOINT="${EXAMPLE_APP_OTEL_TRACES_ENDPOINT:-}"
EXAMPLE_APP_OTEL_LOGS_ENDPOINT="${EXAMPLE_APP_OTEL_LOGS_ENDPOINT:-}"
EXAMPLE_APP_OTEL_HEADERS="${EXAMPLE_APP_OTEL_HEADERS:-}"
EXAMPLE_APP_PID=""

# Signed asset upload auth for /v1/rum/assets.
SOBS_RUM_ASSET_SIGNING_KEY="${SOBS_RUM_ASSET_SIGNING_KEY:-}"

if [[ "${1:-}" == "--" ]]; then
  shift
fi
RUN_CMD=("$@")
RUN_CMD_DEFAULT=0
if [[ ${#RUN_CMD[@]} -eq 0 ]]; then
  case "$SOBS_RUNTIME" in
    python|"")
      RUN_CMD=("$SOBS_PYTHON" app.py)
      ;;
    go)
      RUN_CMD=(go run ./cmd/sobs)
      ;;
    *)
      echo "[error] unsupported SOBS_RUNTIME: $SOBS_RUNTIME (expected: python or go)" >&2
      exit 1
      ;;
  esac
  RUN_CMD_DEFAULT=1
fi

if [[ "$SOBS_RUNTIME" == "go" ]]; then
  if ! command -v go >/dev/null 2>&1; then
    echo "[error] SOBS_RUNTIME=go requires Go toolchain in PATH" >&2
    exit 1
  fi
  # Keep local defaults aligned with prior Python workflow so demo app and
  # health checks keep working without extra flags.
  export SOBS_HTTP_ADDR="${SOBS_HTTP_ADDR:-127.0.0.1:44317}"
  export SOBS_GRPC_ADDR="${SOBS_GRPC_ADDR:-127.0.0.1:4317}"
  export SOBS_ENFORCE_API_AUTH="${SOBS_ENFORCE_API_AUTH:-0}"
fi

SOBS_BASE_URL_DEFAULT="http://127.0.0.1:44317"
if [[ -n "${SOBS_HTTP_ADDR:-}" ]]; then
  SOBS_BASE_URL_DEFAULT="http://${SOBS_HTTP_ADDR}"
fi
if [[ -z "$EXAMPLE_APP_SOBS_BASE_URL" ]]; then
  EXAMPLE_APP_SOBS_BASE_URL="$SOBS_BASE_URL_DEFAULT"
fi
if [[ -z "$EXAMPLE_APP_OTEL_TRACES_ENDPOINT" ]]; then
  EXAMPLE_APP_OTEL_TRACES_ENDPOINT="${EXAMPLE_APP_SOBS_BASE_URL%/}/v1/traces"
fi
if [[ -z "$EXAMPLE_APP_OTEL_LOGS_ENDPOINT" ]]; then
  EXAMPLE_APP_OTEL_LOGS_ENDPOINT="${EXAMPLE_APP_SOBS_BASE_URL%/}/v1/logs"
fi

cleanup() {
  if [[ -n "$EXAMPLE_APP_PID" ]] && kill -0 "$EXAMPLE_APP_PID" >/dev/null 2>&1; then
    kill "$EXAMPLE_APP_PID" >/dev/null 2>&1 || true
    wait "$EXAMPLE_APP_PID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT INT TERM

check_rum_sdk_file() {
  local rum_file
  rum_file="static/rum.js"
  if [[ ! -f "$rum_file" ]]; then
    echo "[error] missing $rum_file (RUM SDK cannot be served)" >&2
    return 1
  fi
  if ! grep -q "var SOBS" "$rum_file"; then
    echo "[error] $rum_file does not contain expected SOBS SDK signature" >&2
    return 1
  fi
  echo "[ok] RUM SDK file check passed ($rum_file)"
}

check_otel_tooling() {
  if [[ "$EXAMPLE_APP_ENABLE_OTEL" != "1" ]]; then
    return 0
  fi

  local py_dir instrumenter
  py_dir="$(dirname "$EXAMPLE_APP_PYTHON")"
  instrumenter="$py_dir/opentelemetry-instrument"
  if [[ -x "$instrumenter" ]]; then
    echo "[ok] found OTEL instrumenter: $instrumenter"
  elif command -v opentelemetry-instrument >/dev/null 2>&1; then
    echo "[ok] found OTEL instrumenter on PATH: $(command -v opentelemetry-instrument)"
  else
    echo "[warn] opentelemetry-instrument not found yet; demo app will run without auto-instrumentation"
  fi

  if "$EXAMPLE_APP_PYTHON" - <<'PY' >/dev/null 2>&1
import importlib
mods = [
    "opentelemetry.distro",
    "opentelemetry.instrumentation.flask",
    "opentelemetry.instrumentation.logging",
    "opentelemetry.exporter.otlp.proto.http.trace_exporter",
]
for m in mods:
    importlib.import_module(m)
PY
  then
    echo "[ok] OTEL Python modules present in demo interpreter"
  else
    if [[ "$EXAMPLE_APP_OTEL_AUTO_INSTALL" == "1" ]]; then
      echo "[warn] OTEL Python modules missing now; auto-install is enabled and will run before startup"
    else
      echo "[warn] OTEL Python modules missing and auto-install disabled"
    fi
  fi
}

check_otlp_endpoint() {
  local label url code
  label="$1"
  url="$2"
  code="$(curl -sS -o /dev/null -w "%{http_code}" -X POST -H "Content-Type: application/json" --data '{}' "$url" || true)"

  case "$code" in
    200|400|401|403|405|415|500)
      echo "[ok] ${label} endpoint reachable: $url (HTTP $code)"
      ;;
    404)
      echo "[warn] ${label} endpoint not found: $url (HTTP 404)"
      ;;
    000)
      if [[ "$RUN_CMD_DEFAULT" == "1" ]]; then
        echo "[warn] ${label} endpoint not reachable yet: $url (SOBS will start after preflight)"
      else
        echo "[warn] ${label} endpoint not reachable: $url"
      fi
      ;;
    *)
      echo "[warn] ${label} endpoint unexpected response: $url (HTTP $code)"
      ;;
  esac
}

run_startup_self_checks() {
  echo "[info] running startup self-checks"
  check_rum_sdk_file
  check_otel_tooling
  if [[ "$RUN_CMD_DEFAULT" == "1" ]]; then
    echo "[info] skipping OTLP endpoint preflight (SOBS will be started by this script)"
  else
    check_otlp_endpoint "OTLP traces" "$EXAMPLE_APP_OTEL_TRACES_ENDPOINT"
    check_otlp_endpoint "OTLP logs" "$EXAMPLE_APP_OTEL_LOGS_ENDPOINT"
  fi
}

start_example_app() {
  if [[ "$START_EXAMPLE_APP" != "1" ]]; then
    return 0
  fi

  if [[ ! -f "$EXAMPLE_APP_SCRIPT" ]]; then
    echo "[warn] demo app script not found: $EXAMPLE_APP_SCRIPT (continuing without demo app)"
    return 0
  fi

  local -a demo_cmd
  demo_cmd=("$EXAMPLE_APP_PYTHON" "$EXAMPLE_APP_SCRIPT")

  ensure_example_otel_deps() {
    local -a required_modules
    required_modules=(
      opentelemetry.distro
      opentelemetry.instrumentation.flask
      opentelemetry.instrumentation.logging
      opentelemetry.exporter.otlp.proto.http.trace_exporter
    )
    local -a missing_modules
    local mod
    for mod in "${required_modules[@]}"; do
      if ! "$EXAMPLE_APP_PYTHON" - <<PY >/dev/null 2>&1
import importlib
importlib.import_module("$mod")
PY
      then
        missing_modules+=("$mod")
      fi
    done

    if [[ ${#missing_modules[@]} -eq 0 ]]; then
      return 0
    fi

    if [[ "$EXAMPLE_APP_OTEL_AUTO_INSTALL" != "1" ]]; then
      echo "[warn] missing OTEL modules in demo app interpreter: ${missing_modules[*]}"
      echo "[warn] set EXAMPLE_APP_OTEL_AUTO_INSTALL=1 or install OTEL packages manually"
      return 0
    fi

    echo "[info] installing missing OTEL packages for demo app instrumentation"
    "$EXAMPLE_APP_PYTHON" -m pip install -q \
      opentelemetry-distro \
      opentelemetry-instrumentation \
      opentelemetry-instrumentation-flask \
      opentelemetry-instrumentation-logging \
      opentelemetry-exporter-otlp-proto-http >/dev/null
  }

  if [[ "$EXAMPLE_APP_ENABLE_OTEL" == "1" ]]; then
    ensure_example_otel_deps

    local py_dir instrumenter
    py_dir="$(dirname "$EXAMPLE_APP_PYTHON")"
    instrumenter="$py_dir/opentelemetry-instrument"
    if [[ -x "$instrumenter" ]]; then
      demo_cmd=("$instrumenter" "$EXAMPLE_APP_PYTHON" "$EXAMPLE_APP_SCRIPT")
    elif command -v opentelemetry-instrument >/dev/null 2>&1; then
      demo_cmd=(opentelemetry-instrument "$EXAMPLE_APP_PYTHON" "$EXAMPLE_APP_SCRIPT")
    else
      echo "[warn] EXAMPLE_APP_ENABLE_OTEL=1 but opentelemetry-instrument not found; starting without OTEL auto-instrumentation"
    fi

    if ! "$EXAMPLE_APP_PYTHON" - <<'PY' >/dev/null 2>&1
import importlib
importlib.import_module("opentelemetry.instrumentation.logging")
PY
    then
      echo "[warn] opentelemetry-instrumentation-logging not installed; OTEL logs signal may be limited"
    fi
  fi

  local otel_headers
  otel_headers="$EXAMPLE_APP_OTEL_HEADERS"
  if [[ -z "$otel_headers" && -n "${SOBS_API_KEY:-}" ]]; then
    # OTLP ingest routes are API-key protected when SOBS_API_KEY is enabled.
    otel_headers="X-API-Key=${SOBS_API_KEY}"
  fi

  echo "[info] starting RUM replay demo app on http://127.0.0.1:${EXAMPLE_APP_PORT}"
  SOBS_BASE_URL="$EXAMPLE_APP_SOBS_BASE_URL" \
    EXAMPLE_APP_PORT="$EXAMPLE_APP_PORT" \
    OTEL_SERVICE_NAME="$EXAMPLE_APP_OTEL_SERVICE_NAME" \
    OTEL_TRACES_SAMPLER="always_on" \
    OTEL_TRACES_EXPORTER="$EXAMPLE_APP_OTEL_TRACES_EXPORTER" \
    OTEL_METRICS_EXPORTER="$EXAMPLE_APP_OTEL_METRICS_EXPORTER" \
    OTEL_LOGS_EXPORTER="$EXAMPLE_APP_OTEL_LOGS_EXPORTER" \
    OTEL_PYTHON_LOGGING_AUTO_INSTRUMENTATION_ENABLED="true" \
    OTEL_EXPORTER_OTLP_PROTOCOL="$EXAMPLE_APP_OTEL_PROTOCOL" \
    OTEL_EXPORTER_OTLP_TRACES_PROTOCOL="$EXAMPLE_APP_OTEL_PROTOCOL" \
    OTEL_EXPORTER_OTLP_LOGS_PROTOCOL="$EXAMPLE_APP_OTEL_PROTOCOL" \
    OTEL_EXPORTER_OTLP_HEADERS="$otel_headers" \
    OTEL_EXPORTER_OTLP_TRACES_HEADERS="$otel_headers" \
    OTEL_EXPORTER_OTLP_LOGS_HEADERS="$otel_headers" \
    OTEL_EXPORTER_OTLP_TRACES_ENDPOINT="$EXAMPLE_APP_OTEL_TRACES_ENDPOINT" \
    OTEL_EXPORTER_OTLP_LOGS_ENDPOINT="$EXAMPLE_APP_OTEL_LOGS_ENDPOINT" \
    "${demo_cmd[@]}" >"$EXAMPLE_APP_LOG" 2>&1 &
  EXAMPLE_APP_PID=$!

  local i
  for i in $(seq 1 40); do
    if nc -z 127.0.0.1 "$EXAMPLE_APP_PORT" >/dev/null 2>&1; then
      echo "[ok] demo app available at http://127.0.0.1:${EXAMPLE_APP_PORT}"
      echo "[info] demo app log: $EXAMPLE_APP_LOG"
      return 0
    fi
    if ! kill -0 "$EXAMPLE_APP_PID" >/dev/null 2>&1; then
      echo "[warn] demo app exited early; continuing without it. Log: $EXAMPLE_APP_LOG"
      tail -n 40 "$EXAMPLE_APP_LOG" || true
      EXAMPLE_APP_PID=""
      return 0
    fi
    sleep 0.2
  done

  echo "[warn] demo app startup timed out; continuing without it. Log: $EXAMPLE_APP_LOG"
  EXAMPLE_APP_PID=""
  return 0
}

if ! curl -fsS "$OLLAMA_TAGS_URL" >/dev/null 2>&1; then
  echo "[error] cannot reach Ollama at $OLLAMA_BASE_URL" >&2
  echo "Start Ollama first (example: 'ollama serve') or set OLLAMA_BASE_URL." >&2
  exit 1
fi

if [[ "$RUN_CMD_DEFAULT" == "1" ]] && curl -fsS "${EXAMPLE_APP_SOBS_BASE_URL%/}/health" >/dev/null 2>&1; then
  echo "[error] SOBS already appears to be running on ${EXAMPLE_APP_SOBS_BASE_URL%/}" >&2
  echo "Stop the existing instance before running this script with default app startup." >&2
  echo "Hint: if you only want the demo app, run with '-- echo demo-only' and START_EXAMPLE_APP=1." >&2
  exit 1
fi

if [[ "$OLLAMA_PULL_MODELS" == "1" ]]; then
  if ! command -v ollama >/dev/null 2>&1; then
    echo "[error] OLLAMA_PULL_MODELS=1 requires 'ollama' CLI in PATH" >&2
    exit 1
  fi
  echo "[info] pulling model: $SOBS_AI_MODEL"
  ollama pull "$SOBS_AI_MODEL"
  if [[ "$SOBS_AI_GUARD_MODEL" != "$SOBS_AI_MODEL" ]]; then
    echo "[info] pulling guard model: $SOBS_AI_GUARD_MODEL"
    ollama pull "$SOBS_AI_GUARD_MODEL"
  fi
fi

export SOBS_AI_ENDPOINT_URL="${SOBS_AI_ENDPOINT_URL:-${OLLAMA_BASE_URL%/}/v1}"
export SOBS_AI_GUARD_ENDPOINT_URL="${SOBS_AI_GUARD_ENDPOINT_URL:-${OLLAMA_BASE_URL%/}/v1}"
export SOBS_AI_MODEL
export SOBS_AI_GUARD_MODEL

# DLP is optional for local Ollama workflow. Only export if provided externally.
if [[ -n "${SOBS_AI_DLP_ENDPOINT_URL:-}" ]]; then
  export SOBS_AI_DLP_ENDPOINT_URL
fi
if [[ -n "${SOBS_AI_API_KEY:-}" ]]; then
  export SOBS_AI_API_KEY
fi

if [[ -z "$SOBS_RUM_ASSET_SIGNING_KEY" ]]; then
  if [[ -x "$SOBS_PYTHON" ]] || command -v "$SOBS_PYTHON" >/dev/null 2>&1; then
    SOBS_RUM_ASSET_SIGNING_KEY="$("$SOBS_PYTHON" - <<'PY'
import secrets
print(secrets.token_hex(32))
PY
)"
  else
    SOBS_RUM_ASSET_SIGNING_KEY="$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
  fi
fi
export SOBS_RUM_ASSET_SIGNING_KEY

run_startup_self_checks
start_example_app

echo
printf 'Configured AI settings for local Ollama:\n'
printf '  kubernetes_integration=disabled (local only)\n'
printf '  sobs_runtime=%s\n' "$SOBS_RUNTIME"
printf '  sobs_base_url=%s\n' "${EXAMPLE_APP_SOBS_BASE_URL%/}"
if [[ -n "${SOBS_HTTP_ADDR:-}" ]]; then
  printf '  SOBS_HTTP_ADDR=%s\n' "$SOBS_HTTP_ADDR"
fi
if [[ -n "${SOBS_GRPC_ADDR:-}" ]]; then
  printf '  SOBS_GRPC_ADDR=%s\n' "$SOBS_GRPC_ADDR"
fi
if [[ -n "${SOBS_ENFORCE_API_AUTH:-}" ]]; then
  printf '  SOBS_ENFORCE_API_AUTH=%s\n' "$SOBS_ENFORCE_API_AUTH"
fi
printf '  SOBS_AI_ENDPOINT_URL=%s\n' "$SOBS_AI_ENDPOINT_URL"
printf '  SOBS_AI_GUARD_ENDPOINT_URL=%s\n' "$SOBS_AI_GUARD_ENDPOINT_URL"
printf '  SOBS_AI_MODEL=%s\n' "$SOBS_AI_MODEL"
printf '  SOBS_AI_GUARD_MODEL=%s\n' "$SOBS_AI_GUARD_MODEL"
if [[ -n "${SOBS_AI_DLP_ENDPOINT_URL:-}" ]]; then
  printf '  SOBS_AI_DLP_ENDPOINT_URL=%s\n' "$SOBS_AI_DLP_ENDPOINT_URL"
else
  printf '  SOBS_AI_DLP_ENDPOINT_URL=<empty>\n'
fi
if [[ -n "${SOBS_AI_API_KEY:-}" ]]; then
  printf '  SOBS_AI_API_KEY=<set>\n'
else
  printf '  SOBS_AI_API_KEY=<empty>\n'
fi
printf '  SOBS_RUM_ASSET_SIGNING_KEY=<set>\n'
if [[ "$START_EXAMPLE_APP" == "1" ]]; then
  printf '  demo_app_url=http://127.0.0.1:%s\n' "$EXAMPLE_APP_PORT"
  printf '  demo_app_script=%s\n' "$EXAMPLE_APP_SCRIPT"
  printf '  demo_app_otel_enabled=%s\n' "$EXAMPLE_APP_ENABLE_OTEL"
  if [[ "$EXAMPLE_APP_ENABLE_OTEL" == "1" ]]; then
    printf '  EXAMPLE_APP_OTEL_AUTO_INSTALL=%s\n' "$EXAMPLE_APP_OTEL_AUTO_INSTALL"
    printf '  EXAMPLE_APP_PYTHON=%s\n' "$EXAMPLE_APP_PYTHON"
    printf '  OTEL_SERVICE_NAME=%s\n' "$EXAMPLE_APP_OTEL_SERVICE_NAME"
    printf '  OTEL_TRACES_SAMPLER=always_on\n'
    printf '  OTEL_TRACES_EXPORTER=%s\n' "$EXAMPLE_APP_OTEL_TRACES_EXPORTER"
    printf '  OTEL_METRICS_EXPORTER=%s\n' "$EXAMPLE_APP_OTEL_METRICS_EXPORTER"
    printf '  OTEL_LOGS_EXPORTER=%s\n' "$EXAMPLE_APP_OTEL_LOGS_EXPORTER"
    printf '  OTEL_EXPORTER_OTLP_PROTOCOL=%s\n' "$EXAMPLE_APP_OTEL_PROTOCOL"
    printf '  OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=%s\n' "$EXAMPLE_APP_OTEL_TRACES_ENDPOINT"
    printf '  OTEL_EXPORTER_OTLP_LOGS_ENDPOINT=%s\n' "$EXAMPLE_APP_OTEL_LOGS_ENDPOINT"
    if [[ -n "$EXAMPLE_APP_OTEL_HEADERS" || -n "${SOBS_API_KEY:-}" ]]; then
      printf '  OTEL_EXPORTER_OTLP_HEADERS=<set>\n'
    else
      printf '  OTEL_EXPORTER_OTLP_HEADERS=<empty>\n'
    fi
  fi
else
  printf '  demo_app=<disabled>\n'
fi
echo
printf 'Running: %s\n' "${RUN_CMD[*]}"

"${RUN_CMD[@]}"
