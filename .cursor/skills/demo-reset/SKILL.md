---
name: demo-reset
description: Resets this Grafana demo fork to a clean main by closing open GitHub PRs without merging, deleting all issues, deleting non-main remote and local branches, and hard-resetting the working tree to origin/main. Use when the user asks to reset the demo, demo reset, or wipe the demo repo.
---

# Demo reset

Destructive reset for this demo fork only. **Execute the script. Do not improvise git or `gh` commands.**

## Workflow

1. Run a dry run from the repo root:

```bash
.cursor/skills/demo-reset/scripts/reset.sh --dry-run
```

2. Show the inventory to the user (open PRs, issues, remote branches, local branches).
3. Run execute **only** after the user confirms, or if this turn already contains an explicit go-ahead after a preview:

```bash
.cursor/skills/demo-reset/scripts/reset.sh --yes
```

The script needs `gh` auth and git push access. Use permissions that allow network plus git writes (typically `all`).

## What the script does (`--yes`)

- Aborts if the GitHub repo owner is `grafana` or the repo is `grafana/grafana`
- Aborts if the default branch is not `main`
- Closes every **open** PR without merging
- Deletes every issue (open and closed). Does not delete PRs.
- Deletes every remote branch except `main`
- Force-checkouts `main`, `git reset --hard origin/main`, `git clean -fd` (ignored files stay)
- Deletes every local branch except `main`
- Prunes remote-tracking refs

## Do not

- Merge PRs
- Rewrite `main` history
- Run this against `grafana/grafana`
- Include this skill directory in a PR back to upstream Grafana
