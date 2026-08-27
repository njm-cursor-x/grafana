---
name: branch-regression
description: >-
  Runs a frozen demo-smoke cloud-agent regression (eslint + at most one Jest
  file + at most one Go package, no Playwright) then opens a PR on pass or a
  GitHub issue on fail. Use when the user creates a new branch and asks to
  start a cloud agent, start regression, launch a cloud agent, or regression
  test this branch.
---

# Branch regression

Frozen demo-smoke cloud-agent regression for this demo fork. **Paste [prompt.md](prompt.md). Do not invent a suite.**

This is a live demo of Shift+K-style frontend work. Prefer speed over coverage.

Abort if the GitHub owner is `grafana` or the repo is `grafana/grafana`.

A skill cannot fire on `git checkout -b` by itself. Invoke when the user creates a new branch and asks to start a cloud agent / regression, or says start regression, launch a cloud agent, or regression test this branch.

## Local agent

This conversation. Do not re-run tests locally. Do not open the PR or issue locally.

1. Identify the branch (current, or the one just created).
2. If it is not on origin, `git push -u origin HEAD` (cloud agents cannot see unpushed branches).
3. Write a 1–3 sentence feature context from `git diff origin/main...HEAD` (what changed and why it exists).
4. Read [prompt.md](prompt.md). Fill `{{BRANCH}}`, `{{REPO}}` (`njm-cursor-x/grafana`), and `{{FEATURE_CONTEXT}}`. Launch **exactly one** `Task`:
   - `environment`: `"cloud"`
   - `cloud_base_branch`: that branch
   - `subagent_type`: `generalPurpose`
   - `run_in_background`: `true` unless the user asked to wait
   - `prompt`: the filled prompt.md text — do not rewrite the test list
5. Tell the user the cloud agent is running and link it (`[Name](id)`).
6. When the cloud agent returns, report the PR or issue URL. Do not duplicate its git/`gh` work.

## Cloud agent

The procedure is frozen in [prompt.md](prompt.md). The local agent pastes that file as the Task prompt. The cloud agent must not add suites, skip steps, write a canvas, or plant DEMO-VULN / XSS / credentials.

Frozen suite every time (no other tests):

- eslint on changed JS/TS files vs `origin/main` only
- at most one related Jest file
- if Go changed: `go test -count=1` in **one** affected package
- **No Playwright**
- If no matching Jest file, skip unit and treat as pass after lint

## Do not

- Recreate demo-branch-cloud-gate
- Plant XSS, credentials, or DEMO-VULN
- Write a canvas
- Run Playwright (`yarn e2e:playwright`)
- Run full `make test-go-unit` or unfiltered `yarn test`
- Rewrite the test list in prompt.md
- Open a PR or issue from this local conversation
- Run this against `grafana/grafana`
