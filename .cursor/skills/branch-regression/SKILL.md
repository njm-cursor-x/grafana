---
name: branch-regression
description: >-
  Runs a frozen demo-smoke cloud-agent regression (eslint + at most one Jest
  file + at most one Go package, no Playwright) then opens a ready (non-draft)
  PR on pass with a pass comment, or files one Jira Bug per failure in
  njm-demo-space (NDS). Use when the user creates a new branch and asks to
  start a cloud agent, start regression, launch a cloud agent, or regression
  test this branch.
---

# Branch regression

Frozen demo-smoke cloud-agent regression for this demo fork. **Paste [prompt.md](prompt.md). Do not invent a suite.**

This is a live demo of Shift+K-style frontend work. Prefer speed over coverage.

Abort if the GitHub owner is `grafana` or the repo is `grafana/grafana`.

A skill cannot fire on `git checkout -b` by itself. Invoke when the user creates a new branch and asks to start a cloud agent / regression, or says start regression, launch a cloud agent, or regression test this branch.

## Local agent

This conversation. Do not re-run tests locally. Do not open the PR locally. Do not create GitHub issues.

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
6. When the cloud agent returns:
   - **Pass:** report the PR URL. Do not duplicate its git/`gh` work except: if it opened a **draft** PR, run `gh pr ready`; if the PR has no pass comment, post one with `gh pr comment` (body: `All regression tests passed.`).
   - **Fail:** do not open a GitHub issue or a PR. File **one Jira Bug per failed entry** in the returned `tests` array (`status` not passed). Then report the Jira issue URLs.

### Jira (fail only)

Cloud agents do not have Atlassian MCP. Create bugs from this conversation:

- Site: `https://fe-anysphere-demo.atlassian.net`
- `cloudId`: `564eb250-21c1-45d7-81f9-527d6bf705ad`
- Project: `njm-demo-space` (`projectKey`: `NDS`)
- `issueTypeName`: `Bug`
- Tool: Atlassian MCP `createJiraIssue`
- Summary: `Regression failed on {{BRANCH}}: {{test name}}`
- Description (markdown): branch, commit, command, trimmed `detail`
- Do not mention this skill in the Jira body

## Cloud agent

The procedure is frozen in [prompt.md](prompt.md). The local agent pastes that file as the Task prompt. The cloud agent must not add suites, skip steps, write a canvas, or plant DEMO-VULN / XSS / credentials.

Frozen suite every time (no other tests):

- eslint on changed JS/TS files vs `origin/main` only
- at most one related Jest file
- if Go changed: `go test -count=1` in **one** affected package
- **No Playwright**
- If no matching Jest file, skip unit and treat as pass after lint

On pass: open a ready PR against `main`, then comment `All regression tests passed.`
On fail: no PR, no GitHub issue; return structured failures so the local agent can file Jira bugs.

## Do not

- Recreate demo-branch-cloud-gate
- Plant XSS, credentials, or DEMO-VULN
- Write a canvas
- Run Playwright (`yarn e2e:playwright`)
- Run full `make test-go-unit` or unfiltered `yarn test`
- Rewrite the test list in prompt.md
- Open a PR or GitHub issue from this local conversation
- File Jira bugs on a passing run
- Run this against `grafana/grafana`
