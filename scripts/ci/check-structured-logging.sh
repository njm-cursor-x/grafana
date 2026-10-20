#!/usr/bin/env bash
#
# Fails when changes add unstructured logging to production code:
#   - pkg/**/*.go:        fmt.Print*, stdlib log.Print*/Fatal*/Panic*
#   - public/app/**/*.ts: console.log/debug/info/warn/error/trace
#
# Only lines added relative to the merge base are checked, so existing call
# sites do not fail the build while the structured logging migration is in
# progress. Uncommitted changes in the working tree are included.
#
# Usage: scripts/ci/check-structured-logging.sh [base-ref]   (default: origin/main)
# Opt out a single line with a trailing `logging-lint: allow <reason>` comment.

set -euo pipefail

base_ref="${1:-${GIT_BASE:-origin/main}}"

if ! merge_base="$(git merge-base "$base_ref" HEAD 2>/dev/null)"; then
  echo "error: cannot find merge base between '$base_ref' and HEAD (fetch the base branch first)" >&2
  exit 2
fi

pathspecs=(
  ':(glob)pkg/**/*.go'
  ':(glob)public/app/**/*.ts'
  ':(glob)public/app/**/*.tsx'
  ':(glob)public/app/**/*.js'
  ':(glob)public/app/**/*.jsx'
  # Tests, fixtures and stories may print freely.
  ':(exclude,glob)**/*_test.go'
  ':(exclude,glob)**/testdata/**'
  ':(exclude,glob)**/*.test.*'
  ':(exclude,glob)**/*.spec.*'
  ':(exclude,glob)**/*.story.*'
  ':(exclude,glob)**/__mocks__/**'
  ':(exclude,glob)**/test/**'
  ':(exclude,glob)**/mocks/**'
  # The logger itself, CLIs and build/codegen tooling write to stdout by design.
  ':(exclude,glob)pkg/infra/log/**'
  ':(exclude,glob)pkg/cmd/**'
  ':(exclude,glob)pkg/build/**'
  ':(exclude,glob)**/codegen/**'
  ':(exclude)pkg/ruleguard.rules.go'
)

git diff -U0 --no-color --no-ext-diff --diff-filter=AMR "$merge_base" -- "${pathspecs[@]}" |
  awk '
    /^\+\+\+ / { file = substr($0, 7); next }
    /^@@ / {
      split($3, hunk, ",")
      line = substr(hunk[1], 2) + 0
      next
    }
    /^\+/ {
      text = substr($0, 2)
      current = line++
      trimmed = text
      sub(/^[ \t]+/, "", trimmed)
      if (trimmed ~ /^(\/\/|\/\*|\*)/ || text ~ /logging-lint: allow/) next

      if (file ~ /\.go$/) {
        if (text ~ /(^|[^A-Za-z0-9_.])(fmt\.Print(f|ln)?|log\.(Print|Fatal|Panic)(f|ln)?)\(/) {
          printf "%s:%d: use pkg/infra/log instead of fmt/log printing: %s\n", file, current, trimmed
          found++
        }
      } else if (text ~ /(^|[^A-Za-z0-9_.]|window\.|globalThis\.)console[?]?\.(log|debug|info|warn|error|trace)([^A-Za-z0-9_]|$)/) {
        printf "%s:%d: use the structured frontend logger instead of console.*: %s\n", file, current, trimmed
        found++
      }
    }
    END {
      if (found) {
        printf "\n%d unstructured logging call(s) added. See contribute/backend/instrumentation.md.\n", found
        exit 1
      }
      print "No new unstructured logging calls."
    }
  '
