#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PARITY_DIR="$ROOT_DIR/docs/parity"

mkdir -p "$PARITY_DIR"

# Build Go route map from literal mux registrations.
rg --no-filename -N -o 'mux\.HandleFunc\("[^"]+",\s*s\.[a-zA-Z0-9_]+' \
  "$ROOT_DIR/internal/web" "$ROOT_DIR/internal/ingest/otlpreceiver" \
  | sed -E 's#mux\.HandleFunc\("([^"]+)",[[:space:]]*s\.([a-zA-Z0-9_]+).*#\1|\2#' \
  | sort -u > "$PARITY_DIR/go_routes_map.txt"

# Ensure normalized Python paths exist from current python route map.
if [[ -f "$PARITY_DIR/python_routes_map.txt" ]]; then
  cut -d'|' -f1 "$PARITY_DIR/python_routes_map.txt" \
    | sed -E 's#<[^>]+>#{}#g; s#\{[a-zA-Z_][a-zA-Z0-9_]*\}#{}#g; s#/$##' \
    | sort -u > "$PARITY_DIR/python_paths_norm.txt"
fi

sed -E 's#\{[a-zA-Z_][a-zA-Z0-9_]*\}#{}#g; s#/$##' "$PARITY_DIR/go_routes_map.txt" \
  | sort -u > "$PARITY_DIR/go_routes_norm.txt"

cut -d'|' -f1 "$PARITY_DIR/go_routes_norm.txt" | sort -u > "$PARITY_DIR/go_paths_norm.txt"

comm -23 "$PARITY_DIR/python_paths_norm.txt" "$PARITY_DIR/go_paths_norm.txt" > "$PARITY_DIR/missing_in_go.txt"
comm -13 "$PARITY_DIR/python_paths_norm.txt" "$PARITY_DIR/go_paths_norm.txt" > "$PARITY_DIR/extra_in_go.txt"

# Classify missing paths that are likely served by Go prefix handlers/subroute dispatchers.
awk -F'|' '$1 ~ /\/$/ || $2 ~ /Subroutes$/ {print $1}' "$PARITY_DIR/go_routes_map.txt" \
  | sed -E 's#/$##' | sort -u > "$PARITY_DIR/go_subroute_prefixes.txt"

: > "$PARITY_DIR/missing_in_go_likely_subroutes.txt"
: > "$PARITY_DIR/missing_in_go_probable_true.txt"

while IFS= read -r missing_path; do
  [[ -z "$missing_path" ]] && continue

  covered="false"
  while IFS= read -r prefix; do
    [[ -z "$prefix" ]] && continue
    if [[ "$missing_path" == "$prefix" || "$missing_path" == "$prefix"/* ]]; then
      covered="true"
      break
    fi
  done < "$PARITY_DIR/go_subroute_prefixes.txt"

  if [[ "$covered" == "true" ]]; then
    echo "$missing_path" >> "$PARITY_DIR/missing_in_go_likely_subroutes.txt"
  else
    echo "$missing_path" >> "$PARITY_DIR/missing_in_go_probable_true.txt"
  fi
done < "$PARITY_DIR/missing_in_go.txt"

printf 'python_paths=%s go_routes=%s go_paths=%s missing=%s likely_subroutes=%s probable_true=%s extra=%s\n' \
  "$(wc -l < "$PARITY_DIR/python_paths_norm.txt")" \
  "$(wc -l < "$PARITY_DIR/go_routes_map.txt")" \
  "$(wc -l < "$PARITY_DIR/go_paths_norm.txt")" \
  "$(wc -l < "$PARITY_DIR/missing_in_go.txt")" \
  "$(wc -l < "$PARITY_DIR/missing_in_go_likely_subroutes.txt")" \
  "$(wc -l < "$PARITY_DIR/missing_in_go_probable_true.txt")" \
  "$(wc -l < "$PARITY_DIR/extra_in_go.txt")"
