#!/usr/bin/env bash
# Start the SOBS Go API server
# Ensures ClickHouse is running, builds the binary, and launches it.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Defaults (override via env)
export SOBS_CLICKHOUSE_DSN="${SOBS_CLICKHOUSE_DSN:-clickhouse://default:@127.0.0.1:9000/default}"
export SOBS_CLICKHOUSE_HTTP_URL="${SOBS_CLICKHOUSE_HTTP_URL:-http://localhost:8123}"
export SOBS_PORT="${SOBS_PORT:-44317}"

# Ensure ClickHouse container is running
if ! docker ps --format '{{.Names}}' | grep -q '^clickhouse$'; then
  echo "Starting ClickHouse container..."
  docker start clickhouse 2>/dev/null || \
    docker run -d --name clickhouse \
      -p 9000:9000 -p 8123:8123 \
      -e CLICKHOUSE_PASSWORD="" \
      clickhouse/clickhouse-server:latest
  sleep 3
fi

# Build
echo "Building sobs-api..."
cd "$GO_DIR"
go build -o sobs-api ./cmd/sobs-api/

# Run
echo "Starting sobs-api on port ${SOBS_PORT}..."
exec ./sobs-api
