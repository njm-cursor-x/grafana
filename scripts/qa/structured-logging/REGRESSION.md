# Structured logging — QA regression checklist

Lane: `chore/structured-logging-qa`  
Draft PR: https://github.com/njm-cursor-x/grafana/pull/35  
Epic: `chore/structured-logging-epic` (`6126b3cc6c7`)  
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

# e2e smoke (needs live Grafana + Playwright browsers)
NODE_OPTIONS='-C @grafana-app/source' yarn playwright test --project=smoke --grep @structured-logging --reporter=list
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
| 1 | Log parse checks (Node) | `node --test scripts/qa/structured-logging/parse-checks.test.mjs` | **PASS** | — | 16/16 tests. JSON objects require `msg` + `logger`; error/`eror` lines require `err`; opaque blobs and unredacted Bearer/password/cookie rejected; `[REDACTED]` allowed. |
| 2 | Parse CLI on valid fixture | `node scripts/qa/structured-logging/parse-log-line.mjs --file scripts/qa/structured-logging/fixtures/valid-backend-error.jsonl` | **PASS** | — | `PASS line 1 msg="Query data failed"` / `summary 1/1 passed`. |
| 3 | Parse CLI rejects secrets + opaque blob | same CLI on `reject-secret-leak.jsonl` and `reject-opaque-blob.txt` | **PASS** | — | Secrets: 0/3, reasons `bearer`, `password`, `cookie,grafana_session`. Opaque: `not-json`. Exit 1 expected. |
| 4 | Frontend Faro wrapper unit | `yarn jest --watchAll=false public/app/core/logging/faro.test.ts` | **PASS** | — | 14/14 passed (6.5s). Level mapping, attribute pass-through, redaction, `console.*` unused. |
| 5 | Frontend critical-path units | `yarn jest --watchAll=false public/app/core/services/context_srv.test.ts public/app/features/query/state/runRequest.test.ts` | **PASS** | — | 23 passed, 1 snapshot. |
| 6 | Frontend `no-console` | `yarn eslint public/app --suppress-rule no-console` | **PASS** | — | Exit 0. Only `@stylistic/eslint-plugin-ts` deprecation warning. Residual production `console.*` are frozen in `eslint-suppressions.json` (FE #34). |
| 7 | Backend `secretredact` | `go test -count=1 ./pkg/infra/log/secretredact/` | **BLOCKED** | env | Distro Go 1.22.2; `go.work` requires 1.26.6. `GOTOOLCHAIN=local`: `go.work requires go >= 1.26.6`. Default: `proxy.golang.org` TLS EOF downloading `go1.26.6`. `GOPROXY=direct`: `toolchain not available`. |
| 8 | Backend `pkg/infra/log` | `go test -count=1 ./pkg/infra/log/` | **BLOCKED** | env | Same Go toolchain / GOPROXY block as #7. Not invented. |
| 9 | e2e smoke login → dashboard → query → save | `yarn playwright test --project=smoke --grep @structured-logging` | **BLOCKED** | env | See e2e section. Spec exists; live run did not start Grafana or Chromium. |
| 10 | Full CI unit/integration | GitHub Actions on this PR | **BLOCKED** | CI / env | Not claimed green from this VM. |
| 11 | Manual JSON log from live Grafana | provoke datasource error; parse line | **BLOCKED** | env / Observability | No Grafana process. `curl 127.0.0.1:3000` and `:3001` → connection refused. |

## e2e attempt (exact reasons)

1. **No Grafana binary / cannot `make run`.** `make run` repeatedly hits `Get "https://proxy.golang.org/golang.org/toolchain/@v/v0.0.1-go1.26.6.linux-amd64.zip": EOF`, then `air` fails (`Permission denied` on empty binary path).
2. **Default Playwright `webServer` failed** (`yarn e2e:plugin:build && ./e2e-playwright/start-server`): nx plugin webpack `ERR_UNKNOWN_FILE_EXTENSION` for `packages/grafana-plugin-configs/webpack.config.ts`. Exit: `Process from config.webServer was not able to start. Exit code: 1`.
3. **With `GRAFANA_URL=http://127.0.0.1:3000`** (skip webServer): authenticate failed `browserType.launch: Executable doesn't exist at /home/ubuntu/.cache/ms-playwright/chromium_headless_shell-1194/chrome-linux/headless_shell`. Playwright browsers were never installed (`.yarnrc.yml` `enableScripts: false`).
4. **No Docker** (`docker: command not found`) — cannot use devenv Loki/Grafana images (Observability #32 same class of block).

Do **not** treat this as a Frontend or Backend product FAIL.

## Environment probe

| Probe | Result |
| --- | --- |
| `node -v` (non-login) | v22.14.0 |
| `bash -lc 'node -v && yarn -v'` | v22.14.0 / 4.17.1 |
| Distro `go version` + `GOTOOLCHAIN=local` | go1.22.2 — `go.work` requires **go >= 1.26.6** |
| `go` toolchain download | **blocked** — `proxy.golang.org` TLS EOF |
| Docker | **not installed** |
| `127.0.0.1:3000` / `:3001` | **connection refused** |
| `yarn install --immutable` | Done (warnings only); used for Jest + eslint |
| `/workspace/bin/grafana` | missing |
| Playwright browsers | missing (`~/.cache/ms-playwright` absent) |

## Inventory (awareness, not a lane FAIL)

`rg` on the integration tip (this branch):

| Pattern | Count |
| --- | --- |
| `console.(log\|debug\|info\|warn\|error)` under `public/app` | 673 lines / 408 files (includes tests) |
| same, excluding tests/mocks | 414 lines / 252 files |
| `fmt.Print*` under `pkg` excluding `*_test.go` | 168 lines / 99 files |

Frontend #34 grandfathered remaining production `console.*` in `eslint-suppressions.json`. Backend #31 left tests, `pkg/cmd/`, `pkg/build/`, setting init, xorm, syslog out of scope. Residual counts are follow-up for those lanes / CI, not a QA product FAIL from an executed assertion.

## Failures by owning lane

Executed product checks did **not** FAIL. Remaining gaps are environment or out-of-scope residuals.

- **Backend:** no executed FAIL. `go test` BLOCKED (env). Residual `fmt.Print*` sites remain (Backend out-of-scope list).
- **Frontend:** no executed FAIL. Faro unit + `no-console` eslint PASS on this tip. Residual suppressed `console.*` remain by design of #34.
- **Security:** not re-run here (not merged). Their #30 notes the same GOPROXY block for `pkg/infra/log` emit tests; `secretredact` was PASS for them when Go matched.
- **CI:** full Actions suite not executed on this VM (**BLOCKED** / env). Lint gate lives on #33.
- **Observability:** live Drilldown/Explore + Loki not re-run (**BLOCKED** — no Docker / no Grafana). Fixture JSON matches their documented Backend-shaped line (`lvl` + `level` + `msg` + `logger` + `err`).
- **env:** Go 1.26.6 toolchain download, no Docker, ports 3000/3001 refused, no Playwright browsers, plugin webpack `.ts` load failure for e2e webServer.

## Hard stops

No production deploy, no authz/secret-store changes, no billing. Parse fixtures use the placeholder `test-secret` only.
