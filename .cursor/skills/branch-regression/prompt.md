# Cloud agent: branch regression (demo smoke)

You are a cloud agent on **{{REPO}}**, branch **{{BRANCH}}**.

Follow this procedure in this exact order. Do not add extra suites. Do not skip steps. Do not write a canvas. Do not plant DEMO-VULN, XSS, or credentials. Do not mention the branch-regression skill in PRs or issues.

This is a live demo of Shift+K-style frontend work. Prefer speed over coverage. The suite is **fixed demo smoke**, not change-scoped coverage.

Feature context:

{{FEATURE_CONTEXT}}

Abort immediately if the GitHub owner is `grafana` or the repo is `grafana/grafana`. If the current branch is not `{{BRANCH}}`, stop. Do not switch to `main`.

---

## 1. Confirm checkout

```bash
git branch --show-current
git status -sb
git rev-parse --short HEAD
```

`git branch --show-current` must equal `{{BRANCH}}`. If it does not, stop.

## 2. Discover change set

```bash
git fetch origin main
git diff --name-only origin/main...HEAD
```

Use this list only to pick lint paths, at most one Jest file, and at most one Go package.

## 3. Run this smoke suite and no other

A fail is any non-zero test or lint exit. Do not run Playwright. Do not run `yarn e2e:playwright`. Never bare `yarn test`. Never `make test-go-unit`. Never typecheck unless the user explicitly asked in the launch prompt / feature context.

### Lint

Eslint on changed JS/TS files vs `origin/main` only (`yarn eslint` / project equivalent on those paths). If no JS/TS files, skip lint.

### Jest (at most one file)

If a related co-located `*.test.ts` / `*.test.tsx` exists, run **exactly one** file:

```bash
yarn jest --no-watch --watchAll=false <one *.test.ts(x)>
```

If no matching Jest file, skip unit and treat as pass after lint.

### Go (at most one package)

If Go changed: `go test -count=1` in **one** affected package. Do not test every affected package.

### Playwright

**No Playwright.** Do not run `yarn e2e:playwright`.

## 4. Pass → PR against main

All green → `gh pr create` against `main`. Title like Grafana PRs: `Area: user-facing change`. Body: what changed, why, commands run, result pass. Do not mention this skill.

## 5. Fail → issue, no PR

Any failure → `gh issue create` titled `Regression failed on branch {{BRANCH}}`. Body: commands, trimmed failure output, `No pull request was opened.` Do not open a PR.

## 6. Final report

Return exactly these fields:

- `branch`
- `commit`
- `tests`: array of `{ name, command, status, durationSec, detail }` (`detail` is failure output if failed)
- `overallStatus`: `passed` | `failed`
- `outcome`: `pr_opened` | `issue_filed` | `failed_no_issue`
- `prUrl` **or** `issueUrl` (mutually exclusive)
- `summary`
- `changedFiles`
