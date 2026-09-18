#!/usr/bin/env bash
# Regenerates an inventory of unstructured log calls and secret-like identifiers
# near those sites. Security lane evidence — not a CI gate (CI lane owns that).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

RG_GLOBS=(
  --glob '!**/node_modules/**'
  --glob '!**/vendor/**'
  --glob '!**/public/build/**'
  --glob '!**/public/lib/**'
  --glob '!**/devenv/**'
  --glob '!**/.git/**'
  --glob '!**/dist/**'
)

# Unstructured logs plus Faro / frontend-telemetry emit sites.
LOG_PAT='console\.(log|debug|info|warn|error)|fmt\.Print|pushLog|pushError|pushEvent|initializeFaro|beforeSend'
SECRET_PAT='password|authorization|api[_-]?key|secret|bearer'
# Reviewer-facing header / credential tokens requested by the security lane.
HEADER_PAT='Authorization|Bearer |["'\''`]token["'\''`]|password'

OUT_DIR="${LOG_INVENTORY_DIR:-scripts/logging-security}"
REPORT="${1:-${OUT_DIR}/inventory-report.txt}"
mkdir -p "$(dirname "$REPORT")"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

{
  echo "# Structured-logging security inventory"
  echo "# generated: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "# cwd: $ROOT"
  echo "# regenerate: make logging-secret-inventory"
  echo
} >"$REPORT"

echo "## 1) console.* / fmt.Print / Faro emit sites under pkg and public/app" | tee -a "$REPORT"
echo | tee -a "$REPORT"
rg -n --sort path "${RG_GLOBS[@]}" -e "$LOG_PAT" pkg public/app >"$tmp/log-sites.txt" || true
log_count="$(wc -l <"$tmp/log-sites.txt" | tr -d ' ')"
echo "count: $log_count" | tee -a "$REPORT"
echo | tee -a "$REPORT"
cat "$tmp/log-sites.txt" >>"$REPORT"
echo | tee -a "$REPORT"

echo "## 2) secret-like identifiers under pkg and public/app" | tee -a "$REPORT"
echo | tee -a "$REPORT"
rg -n --sort path -i "${RG_GLOBS[@]}" -e "$SECRET_PAT" pkg public/app >"$tmp/secret-sites.txt" || true
secret_count="$(wc -l <"$tmp/secret-sites.txt" | tr -d ' ')"
echo "count: $secret_count (full list omitted; overlap is the useful view)" | tee -a "$REPORT"
echo | tee -a "$REPORT"

echo "## 2b) Authorization / Bearer / token / password near log and Faro files" | tee -a "$REPORT"
echo | tee -a "$REPORT"
rg -n --sort path -i "${RG_GLOBS[@]}" -e "$HEADER_PAT" pkg public/app >"$tmp/header-sites.txt" || true
header_count="$(wc -l <"$tmp/header-sites.txt" | tr -d ' ')"
cut -d: -f1 "$tmp/log-sites.txt" | sort -u >"$tmp/log-files.txt"
cut -d: -f1 "$tmp/header-sites.txt" | sort -u >"$tmp/header-files.txt"
comm -12 "$tmp/log-files.txt" "$tmp/header-files.txt" >"$tmp/header-overlap-files.txt"
header_overlap_count="$(wc -l <"$tmp/header-overlap-files.txt" | tr -d ' ')"
echo "header_like_hits: $header_count" | tee -a "$REPORT"
echo "log_or_faro_files_also_mentioning_headers: $header_overlap_count" | tee -a "$REPORT"
echo | tee -a "$REPORT"
if [[ "$header_overlap_count" -gt 0 ]]; then
  cat "$tmp/header-overlap-files.txt" >>"$REPORT"
  echo | tee -a "$REPORT"
fi

echo "## 3) files that contain BOTH a log/Faro site and a secret-like identifier" | tee -a "$REPORT"
echo | tee -a "$REPORT"
cut -d: -f1 "$tmp/secret-sites.txt" | sort -u >"$tmp/secret-files.txt"
comm -12 "$tmp/log-files.txt" "$tmp/secret-files.txt" >"$tmp/overlap-files.txt"
overlap_count="$(wc -l <"$tmp/overlap-files.txt" | tr -d ' ')"
echo "file count: $overlap_count" | tee -a "$REPORT"
echo | tee -a "$REPORT"

if [[ "$overlap_count" -gt 0 ]]; then
  while IFS= read -r file; do
    echo "### $file" >>"$REPORT"
    rg -n -i -e "$LOG_PAT" -e "$SECRET_PAT" "$file" >>"$REPORT" || true
    echo >>"$REPORT"
  done <"$tmp/overlap-files.txt"
fi

echo "## 4) summary" | tee -a "$REPORT"
echo | tee -a "$REPORT"
{
  echo "log_or_faro_sites=$log_count"
  echo "secret_like_hits=$secret_count"
  echo "header_like_hits=$header_count"
  echo "header_overlap_files=$header_overlap_count"
  echo "overlap_files=$overlap_count"
  echo "report=$REPORT"
} | tee -a "$REPORT"

echo
echo "Wrote $REPORT"
