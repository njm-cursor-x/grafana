# Structured logging — QA regression checklist

Lane: **QA** (`chore/structured-logging-qa`)  
Draft PR: https://github.com/njm-cursor-x/grafana/pull/28  
Epic: `chore/structured-logging-epic`  
Tested against: epic + Backend #25, Frontend #26, Observability #23, Security #24, CI #27  
Date: 2026-09-18

This is the QA sign-off for the ticket Testing plan (Regression + log-parse).  
Other lanes own product code. Failures name the owning lane. Do not merge to `main`.

## How to run

```bash
# QA-owned (no Grafana process required)
node --test scripts/qa/structured-logging/*.test.mjs
make qa-structured-logging
make qa-structured-logging-test

# Frontend no-console (Frontend #26 / after that lane is merged)
yarn eslint public/app --quiet --max-warnings 0
make check-structured-logging

# Frontend logger unit tests (Frontend #26)
node node_modules/.bin/jest --watchAll=false --testPathPattern=public/app/core/logging/logger.test.ts

# Backend JSON + redaction (Backend/Security)
go test -count=1 ./pkg/infra/log/...
go test -count=1 ./pkg/infra/log/secretredact/

# e2e smoke (Grafana must already be up; set GRAFANA_URL to skip Playwright webServer)
GRAFANA_URL=http://localhost:3001 yarn e2e:pw --project smoke -- e2e-playwright/smoke-tests-suite/structured-logging-smoke.spec.ts
yarn e2e:pw --project unauthenticated -- e2e-playwright/unauthenticated/login.spec.ts
```

Manual JSON logging (when `make run` is available):

```ini
[log]
mode = console
[log.console]
format = json
```

Provoke a datasource error, then:

```bash
node scripts/qa/structured-logging/parse-json-logs.mjs --fail-on-secret --file /path/to/grafana.jsonl
```

Expected keys: `msg`, `logger`, plus `level` / `t`. Error lines should include `err`.

---

## 1) Regression

| Check | Result | Evidence / notes | Owner if fail |
| --- | --- | --- | --- |
| Full CI unit/integration green | **BLOCKED** | This environment cannot run Grafana CI. `go.work` requires Go **1.26.6**; toolchain download from `proxy.golang.org` fails (`EOF`). No Docker. Frontend #26 and CI #27 were `mergeable_state: unstable` at QA start. Re-run after PM integrates lanes. | CI to keep gates green; Backend/Frontend for package tests |
| e2e smoke: login → open dashboard → query panel → save | **BLOCKED** (spec added; not executed against a live UI) | Spec: `e2e-playwright/smoke-tests-suite/structured-logging-smoke.spec.ts` (testdata query + save toast). Login UI: existing `e2e-playwright/unauthenticated/login.spec.ts`. Playwright `--project smoke` attempted; webServer runs `yarn e2e:plugin:build` via Corepack, which could not fetch `repo.yarnpkg.com` (TLS ECONNRESET). `localhost:3000` / `3001` health = connection refused. `make run` needs Go 1.26.6. Closest substitute: QA node tests + Frontend logger Jest (16/16 PASS). Manual steps in §3. | QA owns the spec. Runtime: machine with `make run` + `yarn start`. Product regressions → Frontend/Backend |
| Manual: JSON logging, datasource error, UI unchanged, log parses | **PARTIAL** | Fixture parse **PASS**. Live Grafana JSON **BLOCKED**. Observability #23 (`54465905fa6`) sample JSONL **PASS** including `--require-err-on-error` (10/10; all `level=error` lines have `err`). | Backend for live JSON emit; Observability for live Loki/Drilldown |
| Frontend build / eslint `no-console` for `public/app` | **PASS** on Frontend #26; **absent** on epic-only | On Frontend #26: `eslint.config.js` has `grafana/no-console-public-app`; `eslint public/app --quiet --max-warnings 0` exit 0 (~89s). Critical-path files (logger, AppWrapper, app.ts, LoginCtrl, DashboardLoaderSrv, runRequest, QueryRunner, index.ts) also exit 0. Epic-only: 411 remaining `console.*` (rule not present). Frontend leftover: **331** `no-console` suppressions in `public/app` (201 files) — eslint is green via suppressions, not because production `console.*` is gone. | Frontend for remaining suppressed call sites; CI for baseline after Backend merge |

## 2) Log parse checks

| Check | Result | Evidence | Owner if fail |
| --- | --- | --- | --- |
| QA fixture JSONL parses with `msg`, `logger`, `err` on errors | **PASS** | `make qa-structured-logging-test`: 13/13 after Observability re-check. `parse-json-logs.mjs --file scripts/qa/structured-logging/fixtures/valid.jsonl --require-err-on-error --fail-on-secret`: **6/6** | QA |
| Opaque string is rejected | **PASS** | Unit test + `invalid-mixed.jsonl` (1 opaque + 1 missing fields → 1/3 ok) | QA |
| Unredacted `Bearer test-secret` is flagged | **PASS** (parser); live emit not run | Parser unit test. Security `secretredact` isolated `go test` **PASS** (4 tests, stdlib-only, `GOTOOLCHAIN=local`). Backend `./pkg/infra/log` tests **BLOCKED** (Go 1.26.6). | Security (Faro/backend leakage tests); Backend (`pkg/infra/log` JSON encode) |
| Observability sample JSONL | **PASS** (re-check 2026-09-18, #23 `54465905fa6`) | Default and `--require-err-on-error --fail-on-secret`: **10/10**. Four error lines all have `err`, including `Alert rule evaluation failed` / `logger=ngalert.eval` → `err=failed to execute query: context deadline exceeded`. Snapshot: `scripts/qa/structured-logging/fixtures/observability-sample.jsonl`. | Observability |

### Sample redacted lines (expected shape)

```json
{"t":"2026-09-18T15:00:25.000000000Z","level":"error","msg":"Failed to query datasource","logger":"tsdb.loki","err":"context deadline exceeded","dashboardTitle":"ops / api latency"}
{"t":"2026-09-18T15:00:40.000000000Z","level":"error","msg":"Query data failed","logger":"query_data","err":"Authorization: [REDACTED]"}
```

Observability fixture re-check (#23 `54465905fa6`, redacted; live Loki not running):

```json
{"t":"2026-09-18T15:00:25.000000000Z","level":"error","msg":"Query data failed","logger":"query_data","err":"query backend unavailable"}
{"t":"2026-09-18T15:00:40.000000000Z","level":"error","msg":"Alert rule evaluation failed","logger":"ngalert.eval","err":"failed to execute query: context deadline exceeded","rule":"HighErrorRate"}
```

Dashboard title / query-like strings are **fields**, not format-string sinks.

## 3) Manual e2e steps (when Grafana is up)

1. `make run` with `[log.console] format = json` (do not commit secrets in `custom.ini`).
2. Open `http://localhost:3000`, login `admin` / `admin`, skip password change.
3. New dashboard → add panel → TestData **CSV Metric Values** → confirm `A-series` legend.
4. Save dashboard → toast **Dashboard saved**.
5. Break the panel query → UI still shows a panel error; JSON log line has `msg`, `logger`, `err`.
6. Optional: Observability `make devenv sources=structured-logging` then Drilldown → Logs / Explore.

Playwright equivalent once Grafana is already serving (avoid Corepack webServer):

```bash
GRAFANA_URL=http://localhost:3001 yarn e2e:pw --project smoke -- e2e-playwright/smoke-tests-suite/structured-logging-smoke.spec.ts
```

## 4) Commands actually run (no secrets)

```text
# QA lane
node --test scripts/qa/structured-logging/*.test.mjs          # 12/12 PASS
make qa-structured-logging-test                               # PASS
make qa-structured-logging                                    # PASS (epic-only tree)
node .yarn/releases/yarn-4.17.1.cjs install --immutable       # PASS (warnings); corepack yarn download failed

# Frontend #26 worktree
node scripts/qa/structured-logging/no-console-gate.mjs
  # eslint grafana/no-console-public-app present: true
  # check-structured-logging: PASS (245 files, 452 existing hits)
  # remaining non-allowlisted console.* (no eslint-disable line): 346
node node_modules/.bin/jest --watchAll=false --testPathPattern=public/app/core/logging/logger.test.ts
  # PASS 16 tests
node node_modules/.bin/eslint public/app --quiet --max-warnings 0
  # exit 0

# CI #27 worktree
node scripts/check-structured-logging.mjs                     # PASS (288 files, 517 hits)
node --test scripts/check-structured-logging.test.mjs         # 9/9 PASS

# Observability #23 fixture (re-check after 54465905fa6)
git fetch origin chore/structured-logging-observability
git show origin/chore/structured-logging-observability:devenv/docker/blocks/structured-logging/fixtures/sample-structured.jsonl \
  | node scripts/qa/structured-logging/parse-json-logs.mjs --stdin --require-err-on-error --fail-on-secret
  # 10/10 lines ok  opaque=0 failed=0 secretHits=0 warnings=0  EXIT 0
node scripts/qa/structured-logging/parse-json-logs.mjs \
  --file scripts/qa/structured-logging/fixtures/observability-sample.jsonl \
  --require-err-on-error --fail-on-secret
  # 10/10 lines ok  EXIT 0
  # error lines with err: Query data failed; HTTP server error; request failed; Alert rule evaluation failed

# Security #24 secretredact (copied to a stdlib-only module)
GOTOOLCHAIN=local go test -count=1 -v .                       # 4/4 PASS

# Backend #25
GOTOOLCHAIN=local go test ./pkg/infra/log/                    # BLOCKED: go.work requires go >= 1.26.6
go test                                                       # BLOCKED: proxy.golang.org toolchain EOF

# Playwright
playwright test --project smoke e2e-playwright/smoke-tests-suite/structured-logging-smoke.spec.ts
  # BLOCKED: webServer yarn/corepack TLS ECONNRESET; Grafana not running

# Trial merge (not pushed)
Backend + Frontend: clean
Backend + Frontend + Observability: clean
Backend + Frontend + CI: CONFLICT add/add
  scripts/check-structured-logging.mjs
  scripts/check-structured-logging.test.mjs
  scripts/structured-logging-baseline.json
Backend + Frontend + Security: CONFLICT add/add
  pkg/infra/log/redact.go
  pkg/infra/log/redact_test.go
After Backend+Frontend (Frontend baseline): check-structured-logging FAIL
  stale baseline for Backend print removals:
  pkg/api/http_server.go stdlib.log
  pkg/components/dashdiffs/formatter_json.go fmt.Print
  pkg/expr/sql/frame_table.go fmt.Print
  pkg/services/ldap/settings.go fmt.Print
```

## 5) Pass / fail summary

| Lane | QA verdict | Notes |
| --- | --- | --- |
| Backend #25 | **PARTIAL / blocked live tests** | Call sites look structured (`err` field). `go test ./pkg/infra/log` not runnable here. Conflicts with Security on `redact.go`. After merge with Frontend, CI baseline goes stale. |
| Frontend #26 | **PASS** for eslint gate + logger tests; **PARTIAL** vs “no console.* in public/app” | Faro wrapper Jest 16/16. `eslint public/app --quiet` 0. 331 `no-console` suppressions remain. |
| CI #27 | **PASS** on its own branch; **FAIL** after Backend+Frontend without baseline refresh | Checker unit tests 9/9. Conflicts with Frontend copies of the same scripts. **CI** should take Frontend’s refreshed baseline then re-run `--update-baseline` after Backend print removals. |
| Security #24 | **PASS** for isolated redaction tests; **conflict** with Backend | `secretredact` 4/4 PASS. Duplicate `pkg/infra/log/redact.go` vs Backend. PM/Security+Backend must pick one implementation. |
| Observability #23 | **PASS** (re-check) for JSON fixture `--require-err-on-error`; live Loki still **BLOCKED** (no Docker) | 10/10 after `54465905fa6`. `Alert rule evaluation failed` now has `err`. |
| QA | **this PR** | Checklist, parse script, smoke spec, evidence. e2e not executed live. |

**HITL:** do not merge this PR or the epic to `main`.
