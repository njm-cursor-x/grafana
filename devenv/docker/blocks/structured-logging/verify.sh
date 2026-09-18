#!/usr/bin/env bash
# Query the local Loki for seeded structured JSON. Used as compose smoke-check.
set -euo pipefail

LOKI_URL="${LOKI_URL:-http://localhost:3100}"
QUERY="${1:-"{job=\"grafana-structured\"}"}"

echo "== Loki ready =="
curl -sfS "${LOKI_URL}/ready"
echo

echo "== Labels =="
curl -sfS "${LOKI_URL}/loki/api/v1/labels"
echo
echo

echo "== Query: ${QUERY} =="
# Instant-query the last hour so fixture timestamps (seeded at "now") match.
END_NS="$(($(date +%s) * 1000000000))"
START_NS="$((END_NS - 3600 * 1000000000))"
curl -sfSG "${LOKI_URL}/loki/api/v1/query_range" \
  --data-urlencode "query=${QUERY}" \
  --data-urlencode "start=${START_NS}" \
  --data-urlencode "end=${END_NS}" \
  --data-urlencode "limit=20"
echo
