# Structured logging — QA regression checklist

Lane: `chore/structured-logging-qa`  
Epic: `chore/structured-logging-epic`  
Integration tip: Backend `#31` (`7203465e54f`) + Frontend `#34` (`47540a919d1`) merged locally into this branch. Merge was clean (no conflicts). Security `#30`, Observability `#32`, CI `#33` were **not** merged (awareness only).

QA owns this file, `scripts/qa/structured-logging/*`, and `e2e-playwright/smoke-tests-suite/structured-logging.spec.ts`. Product logging code belongs to peer lanes — failures are attributed, not “fixed” here.

Status values: **PASS** / **FAIL** / **BLOCKED**.  
`FAIL` = check ran and the product behavior is wrong (owning lane listed).  
`BLOCKED` = environment or missing toolchain; not a product fail.

## How to run

```bash
# Parse checks (no extra deps)
node --test scripts/qa/structured-logging/parse-checks.test.mjs
node scripts/qa/structured-logging/parse-log-line.mjs --file scripts/qa/structured-logging/fixtures/valid-backend-error.jsonl

# Frontend Faro wrapper (needs yarn install)
yarn jest --watchAll=false public/app/core/logging/faro.test.ts

# Frontend no-console (needs yarn install)
yarn eslint public/app --suppress-rule no-console

# Backend secretredact (needs Go matching go.mod)
go test -count=1 ./pkg/infra/log/secretredact/

# e2e smoke (needs live Grafana)
yarn e2e:pw --project smoke e2e-playwright/smoke-tests-suite/structured-logging.spec.ts
```

## Integration merge

| Step | Result |
| --- | --- |
| Branch `chore/structured-logging-qa` from epic `6126b3cc6c7` | PASS |
| Merge `origin/chore/structured-logging-backend` (`7203465e54f`) | PASS — fast-forward, 18 files |
| Merge `origin/chore/structured-logging-frontend` (`47540a919d1`) | PASS — ort merge, 17 files, no conflicts |

## Evidence table

| # | Check | Command | Result | Owning lane if not PASS | Evidence |
| --- | --- | --- | --- | --- | --- |
| 1 | Log parse checks (Node) | `node --test scripts/qa/structured-logging/parse-checks.test.mjs` | PENDING | QA | First run after this file lands. |
| 2 | Parse CLI on valid fixture | `node scripts/qa/structured-logging/parse-log-line.mjs --file scripts/qa/structured-logging/fixtures/valid-backend-error.jsonl` | PENDING | QA | |
| 3 | Frontend Faro wrapper unit | `yarn jest --watchAll=false public/app/core/logging/faro.test.ts` | PENDING | Frontend / env | |
| 4 | Frontend `no-console` | `yarn eslint public/app --suppress-rule no-console` | PENDING | Frontend / CI / env | FE #34 reports clean after suppressions. |
| 5 | Backend `secretredact` | `go test -count=1 ./pkg/infra/log/secretredact/` | PENDING | Backend / env | BE #31 + Security #30 report PASS with GOPROXY=off when Go matches go.mod. |
| 6 | Backend `pkg/infra/log` | `go test -count=1 ./pkg/infra/log/` | PENDING | Backend / env | |
| 7 | e2e smoke login → dashboard → query → save | `yarn e2e:pw --project smoke e2e-playwright/smoke-tests-suite/structured-logging.spec.ts` | PENDING | env | Requires live Grafana. |
| 8 | Full CI unit/integration | GitHub Actions on this PR | PENDING | CI | Not claimed green from this VM. |
| 9 | Manual JSON log from live Grafana | provoke datasource error; parse line | PENDING | Observability / env | Needs Grafana + JSON `format = json`. |

## Environment probe (this VM)

Recorded before the first test pass. Update the table above with actual results.

| Probe | Result |
| --- | --- |
| `node -v` (non-login) | v22.14.0 |
| `bash -lc 'node -v && yarn -v'` | v22.14.0 / 4.17.1 |
| Distro `go version` + `GOTOOLCHAIN=local` | go1.22.2 — `go.work` requires **go >= 1.26.6** |
| `go` toolchain download | **blocked** — `proxy.golang.org` TLS EOF (`golang.org/toolchain@v0.0.1-go1.26.6.linux-amd64`) |
| Docker | **not installed** (`docker: command not found`) |
| `localhost:3000` / `:3001` | **refused** (no Grafana process) |
| `/workspace/node_modules` | **missing** at probe time |
| `/workspace/bin/grafana` | **missing** |

## Failures by owning lane

Filled after commands run. Empty until then.

- **Backend:** —
- **Frontend:** —
- **Security:** —
- **CI:** —
- **Observability:** —
- **env:** —

## Hard stops

No production deploy, no authz/secret-store changes, no billing. Parse fixtures use the placeholder `test-secret` only.
