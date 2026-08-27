#!/usr/bin/env bash
# Reset this demo fork: close open PRs (no merge), delete all issues,
# delete non-main remote/local branches, hard-reset local main to origin/main.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: reset.sh [--dry-run|--yes]

  --dry-run   Inventory what would be closed/deleted/reset, then exit (default)
  --yes       Execute the reset
EOF
}

MODE="dry-run"
for arg in "$@"; do
  case "$arg" in
    --dry-run) MODE="dry-run" ;;
    --yes) MODE="yes" ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $arg" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if ! command -v gh >/dev/null 2>&1; then
  echo "error: gh is required" >&2
  exit 1
fi

if ! gh auth status >/dev/null 2>&1; then
  echo "error: gh is not authenticated" >&2
  exit 1
fi

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "error: not inside a git repository" >&2
  exit 1
fi

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

NAME_WITH_OWNER="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"
OWNER="$(gh repo view --json owner --jq .owner.login)"
DEFAULT_BRANCH="$(gh repo view --json defaultBranchRef --jq .defaultBranchRef.name)"

if [[ "$OWNER" == "grafana" || "$NAME_WITH_OWNER" == "grafana/grafana" ]]; then
  echo "error: refusing to run against ${NAME_WITH_OWNER}" >&2
  exit 1
fi

if [[ "$DEFAULT_BRANCH" != "main" ]]; then
  echo "error: default branch is '${DEFAULT_BRANCH}', expected main" >&2
  exit 1
fi

# Bash 3.2 (macOS /bin/bash) has no mapfile.
OPEN_PRS=()
ISSUES=()
REMOTE_BRANCHES=()
LOCAL_BRANCHES=()

collect_issues() {
  ISSUES=()
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -z "$line" ]] && continue
    ISSUES+=("$line")
  done < <(gh issue list --state all --limit 1000 --json number,title,state --jq '.[] | "\(.number)\t\(.state)\t\(.title)"')
}

while IFS= read -r line || [[ -n "$line" ]]; do
  [[ -z "$line" ]] && continue
  OPEN_PRS+=("$line")
done < <(gh pr list --state open --limit 1000 --json number,title --jq '.[] | "\(.number)\t\(.title)"')

collect_issues()

while IFS= read -r line || [[ -n "$line" ]]; do
  [[ -z "$line" ]] && continue
  REMOTE_BRANCHES+=("$line")
done < <(git ls-remote --heads origin | awk '{print $2}' | sed 's#^refs/heads/##' | grep -vx 'main' || true)

while IFS= read -r line || [[ -n "$line" ]]; do
  [[ -z "$line" ]] && continue
  LOCAL_BRANCHES+=("$line")
done < <(git for-each-ref --format='%(refname:short)' refs/heads | grep -vx 'main' || true)

print_section() {
  local title="$1"
  shift
  echo
  echo "${title} (${#})"
  if [[ $# -eq 0 ]]; then
    echo "  (none)"
    return
  fi
  local line
  for line in "$@"; do
    echo "  ${line}"
  done
}

echo "Demo reset — ${NAME_WITH_OWNER}"
echo "Mode: ${MODE}"
print_section "Open PRs to close (not merge)" "${OPEN_PRS[@]+"${OPEN_PRS[@]}"}"
print_section "Issues to close and delete (Issues tab must be empty)" "${ISSUES[@]+"${ISSUES[@]}"}"
print_section "Remote branches to delete" "${REMOTE_BRANCHES[@]+"${REMOTE_BRANCHES[@]}"}"
print_section "Local branches to delete" "${LOCAL_BRANCHES[@]+"${LOCAL_BRANCHES[@]}"}"
echo
echo "Local working tree: checkout main, reset --hard origin/main, git clean -fd"

if [[ "$MODE" != "yes" ]]; then
  echo
  echo "Dry run only. Re-run with --yes to execute."
  exit 0
fi

failed=0

warn() {
  echo "warn: $*" >&2
  failed=1
}

echo
echo "Closing open PRs..."
if [[ ${#OPEN_PRS[@]} -eq 0 ]]; then
  echo "  (none)"
else
  for line in "${OPEN_PRS[@]}"; do
    number="${line%%$'\t'*}"
    if ! gh pr close "$number" --comment "Closed by demo reset"; then
      warn "failed to close PR #${number}"
    fi
  done
fi

echo "Closing and deleting issues (Issues tab must be empty)..."
# Closing is not enough: GitHub still lists closed issues. Delete until none remain.
issue_pass=1
while true; do
  collect_issues
  if [[ ${#ISSUES[@]} -eq 0 ]]; then
    if [[ "$issue_pass" -eq 1 ]]; then
      echo "  (none)"
    else
      echo "  Issues tab is empty."
    fi
    break
  fi
  echo "  Pass ${issue_pass}: close+delete ${#ISSUES[@]} issue(s)..."
  for line in "${ISSUES[@]}"; do
    number="${line%%$'\t'*}"
    gh issue close "$number" >/dev/null 2>&1 || true
    if ! gh issue delete "$number" --yes; then
      warn "failed to delete issue #${number}"
    fi
  done
  if [[ "$issue_pass" -ge 3 ]]; then
    collect_issues
    if [[ ${#ISSUES[@]} -ne 0 ]]; then
      warn "issues still present; GitHub Issues tab is not empty"
      for line in "${ISSUES[@]}"; do
        echo "    remaining: ${line}" >&2
      done
    fi
    break
  fi
  issue_pass=$((issue_pass + 1))
done

echo "Deleting remote branches..."
if [[ ${#REMOTE_BRANCHES[@]} -eq 0 ]]; then
  echo "  (none)"
else
  for branch in "${REMOTE_BRANCHES[@]}"; do
    if ! git push origin --delete "$branch"; then
      warn "failed to delete remote branch ${branch}"
    fi
  done
fi

echo "Resetting local repo to origin/main..."
git fetch origin
git checkout -f main
git reset --hard origin/main
git clean -fd

echo "Deleting local branches..."
if [[ ${#LOCAL_BRANCHES[@]} -eq 0 ]]; then
  echo "  (none)"
else
  for branch in "${LOCAL_BRANCHES[@]}"; do
    if ! git branch -D "$branch"; then
      warn "failed to delete local branch ${branch}"
    fi
  done
fi

git remote prune origin

echo
if [[ "$failed" -ne 0 ]]; then
  echo "Demo reset finished with warnings."
  exit 1
fi
echo "Demo reset complete."
