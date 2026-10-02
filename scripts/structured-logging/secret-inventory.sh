#!/usr/bin/env bash
# Inventory secret-like patterns in log-adjacent code for the NDS-10 security lane.
# This is a report, not a merge gate: matches do not fail the command.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

if ! command -v rg >/dev/null 2>&1; then
  echo "rg is required to run the structured-logging secret inventory" >&2
  exit 1
fi

candidates=(
  pkg/infra/log
  pkg/middleware/loggermw
  pkg/api/frontendlogging
  pkg/api/frontend_logging.go
  pkg/services/pluginsintegration/clientmiddleware/logger_middleware.go
  packages/grafana-runtime/src/utils/logging.ts
  packages/grafana-runtime/src/services/logging
  public/app/core/services/echo
  public/app/core/logging
)

paths=()
missing=()
for p in "${candidates[@]}"; do
  if [[ -e "$p" ]]; then
    paths+=("$p")
  else
    missing+=("$p")
  fi
done

if [[ ${#paths[@]} -eq 0 ]]; then
  echo "no log-adjacent paths found" >&2
  exit 1
fi

patterns=(
  'Authorization'
  'Bearer'
  'api[_-]?key'
  'password'
  'grafana_session'
  'cookie'
)

echo "structured-logging secret inventory"
echo "root: $root"
echo "scanned:"
printf '  %s\n' "${paths[@]}"
if [[ ${#missing[@]} -gt 0 ]]; then
  echo "absent (skipped):"
  printf '  %s\n' "${missing[@]}"
fi
echo

total=0
for pat in "${patterns[@]}"; do
  echo "=== pattern: ${pat} ==="
  set +e
  matches="$(
    rg -n -i \
      --glob '!**/node_modules/**' \
      --glob '!*_test.go' \
      --glob '!*.test.ts' \
      --glob '!*.test.tsx' \
      -e "$pat" \
      "${paths[@]}"
  )"
  status=$?
  set -e
  if [[ $status -eq 0 ]]; then
    printf '%s\n' "$matches"
    count="$(printf '%s\n' "$matches" | wc -l | tr -d ' ')"
  elif [[ $status -eq 1 ]]; then
    echo "(no matches)"
    count=0
  else
    echo "rg failed for pattern ${pat}" >&2
    exit "$status"
  fi
  echo "count: ${count}"
  echo
  total=$((total + count))
done

echo "total matches: ${total}"
