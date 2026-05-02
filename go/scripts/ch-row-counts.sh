#!/usr/bin/env bash
# Show row counts for all SOBS tables in ClickHouse
set -euo pipefail

TABLES=(otel_logs otel_traces hyperdx_sessions otel_metrics_gauge otel_metrics_sum otel_metrics_histogram)

printf "%-30s %s\n" "TABLE" "ROWS"
printf "%-30s %s\n" "-----" "----"
for t in "${TABLES[@]}"; do
  count=$(docker exec clickhouse clickhouse-client -q "SELECT count() FROM $t" 2>/dev/null || echo "N/A")
  printf "%-30s %s\n" "$t" "$count"
done
