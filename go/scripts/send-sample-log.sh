#!/usr/bin/env bash
# Send 1 random OTLP JSON log to POST /v1/logs
set -euo pipefail

SOBS_URL="${SOBS_URL:-http://localhost:44317}"
TS=$(date -u +%s)000000000

SERVICES=("web-api" "auth-service" "payment-service" "notification-worker" "data-pipeline")
LEVELS=("INFO" "WARN" "ERROR" "DEBUG")
BODIES=(
  "Request processed successfully"
  "Connection pool exhausted, retrying"
  "Failed to parse configuration file"
  "Cache miss for key user:session:abc"
  "Health check passed"
  "Timeout waiting for downstream response"
  "Database migration applied"
  "Rate limit exceeded for client 10.0.0.5"
)

SVC=${SERVICES[$((RANDOM % ${#SERVICES[@]}))]}
LVL=${LEVELS[$((RANDOM % ${#LEVELS[@]}))]}
BODY=${BODIES[$((RANDOM % ${#BODIES[@]}))]}
TRACE_ID=$(printf '%032x' $((RANDOM * RANDOM)))
SPAN_ID=$(printf '%016x' $((RANDOM * RANDOM)))

PAYLOAD=$(cat <<EOF
{
  "resourceLogs": [{
    "resource": {
      "attributes": [
        {"key": "service.name", "value": {"stringValue": "${SVC}"}}
      ]
    },
    "scopeLogs": [{
      "scope": {"name": "sample-script"},
      "logRecords": [{
        "timeUnixNano": "${TS}",
        "severityText": "${LVL}",
        "body": {"stringValue": "${BODY}"},
        "traceId": "${TRACE_ID}",
        "spanId": "${SPAN_ID}",
        "attributes": [
          {"key": "env", "value": {"stringValue": "dev"}},
          {"key": "host", "value": {"stringValue": "node-$(( RANDOM % 5 + 1 ))"}}
        ]
      }]
    }]
  }]
}
EOF
)

echo "Sending log: service=${SVC} level=${LVL} body=\"${BODY}\""
curl -s -X POST "${SOBS_URL}/v1/logs" \
  -H "Content-Type: application/json" \
  -d "${PAYLOAD}"
echo ""
