#!/usr/bin/env bash
# Start the SOBS Go API server
# Builds the binary and launches it with embedded chDB.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Defaults (override via env)
export SOBS_PORT="${SOBS_PORT:-44317}"

# The Go binary now uses embedded chDB by default, so no external ClickHouse
# container is required here.

# Build
echo "Building sobs-api..."
cd "$GO_DIR"
go build -o sobs-api ./cmd/sobs-api/

# Run
echo "Starting sobs-api on port ${SOBS_PORT}..."
exec ./sobs-api
