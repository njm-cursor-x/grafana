#!/usr/bin/env bash
#
# Report-only inventory of logging call sites that are unstructured or may leak
# credentials. Always exits 0 unless ripgrep is missing; CI gating lives elsewhere.
#
# Usage: scripts/logging-security-inventory.sh [-v] [-o OUTPUT_DIR]
#   -v             print every match, not only counts
#   -o OUTPUT_DIR  also write each section's matches to OUTPUT_DIR/<section>.txt

set -euo pipefail

VERBOSE=0
OUTPUT_DIR=""
while getopts "vo:" opt; do
	case "$opt" in
	v) VERBOSE=1 ;;
	o) OUTPUT_DIR="$OPTARG" ;;
	*) echo "usage: $0 [-v] [-o OUTPUT_DIR]" >&2; exit 2 ;;
	esac
done

if ! command -v rg >/dev/null 2>&1; then
	echo "ripgrep (rg) is required" >&2
	exit 1
fi

cd "$(git rev-parse --show-toplevel)"

[ -n "$OUTPUT_DIR" ] && mkdir -p "$OUTPUT_DIR"

GO_EXCLUDES=(--glob '!**/*_test.go' --glob '!**/testdata/**' --glob '!**/*_gen.go' --glob '!**/*.pb.go')
TS_EXCLUDES=(--glob '!**/*.test.*' --glob '!**/*.spec.*' --glob '!**/__mocks__/**' --glob '!**/mocks/**' --glob '!**/*.fixture.*' --glob '!**/*.md')
LOG_CALL='(\.(Info|Warn|Error|Debug|Log)(Ctx)?\(|\bconsole\.(log|debug|info|warn|error)\(|pushLog\(|\bfmt\.(Print|Fprint)|\blog\.(Print|Fatal|Panic))'
SENSITIVE='(password|passwd|authorization|api[_-]?key|secret|bearer|cookie|access[_-]?token|refresh[_-]?token)'

section() {
	local name="$1" title="$2"
	shift 2
	local matches count
	matches="$(rg --no-heading -n "$@" 2>/dev/null || true)"
	count=0
	[ -n "$matches" ] && count="$(printf '%s\n' "$matches" | wc -l | tr -d ' ')"

	printf '%-48s %6s\n' "$title" "$count"
	if [ "$VERBOSE" = 1 ] && [ -n "$matches" ]; then
		printf '%s\n\n' "$matches"
	fi
	if [ -n "$OUTPUT_DIR" ]; then
		printf '%s\n' "$matches" >"$OUTPUT_DIR/$name.txt"
	fi
}

echo "Logging security inventory ($(git rev-parse --short HEAD))"
echo

section go-fmt-print "Go fmt.Print*/Fprint* (pkg/)" \
	"${GO_EXCLUDES[@]}" -e '\bfmt\.(Print|Println|Printf)\(' -e '\bfmt\.Fprint(ln|f)?\(os\.(Stdout|Stderr)' pkg
section go-stdlib-log "Go stdlib log.* (pkg/)" \
	"${GO_EXCLUDES[@]}" -e '^\s*"log"$' pkg
section go-sprintf-msg "Go logger msg built with Sprintf (pkg/)" \
	"${GO_EXCLUDES[@]}" -e '\.(Info|Warn|Error|Debug)(Ctx)?\((ctx, )?fmt\.Sprintf\(' pkg
section ts-console "TS console.* (public/app/)" \
	"${TS_EXCLUDES[@]}" -e '\bconsole\.(log|debug|info|warn|error)\(' public/app
section go-sensitive "Go log call + sensitive keyword (pkg/)" \
	"${GO_EXCLUDES[@]}" -i --pcre2 -e "${LOG_CALL}.*${SENSITIVE}" pkg
section ts-sensitive "TS log call + sensitive keyword (public/app/)" \
	"${TS_EXCLUDES[@]}" -i --pcre2 -e "${LOG_CALL}.*${SENSITIVE}" public/app

echo
echo "Counts are candidates for review, not confirmed findings."
