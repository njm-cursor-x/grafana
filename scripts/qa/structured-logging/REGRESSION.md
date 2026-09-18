# Structured logging — QA regression checklist

Lane: **QA** (`chore/structured-logging-qa`)  
Epic: `chore/structured-logging-epic`  
Tested against: epic + draft PRs Backend #25, Frontend #26, Observability #23, Security #24, CI #27  
Date: 2026-09-18

This file is the QA sign-off for the ticket Testing plan (Regression + log-parse).  
Other lanes own product code. Failures name the owning lane.

## How to run (local / CI)

```bash
# QA-owned (no Grafana process required)
node --test scripts/qa/structured-logging/*.test.mjs
node scripts/qa/structured-logging/run.mjs
make qa-structured-logging
make qa-structured-logging-test

# Frontend no-console (after Frontend lane is merged, or on that worktree)
yarn eslint public/app --rule 'no-console: error' --quiet   # if grafana/no-console-public-app is in eslint.config.js
make check-structured-logging                               # CI lane script

# Frontend logger unit tests (Frontend lane)
yarn jest --watchAll=false --testPathPattern=public/app/core/logging/logger.test.ts

# Backend JSON + redaction (Backend/Security lanes)
go test -count=1 ./pkg/infra/log/...

# e2e smoke (needs Grafana on GRAFANA_URL, default http://localhost:3001 for Playwright)
yarn e2e:pw --project smoke -- e2e-playwright/smoke-tests-suite/structured-logging-smoke.spec.ts
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

Expected keys on a structured line: `msg`, `logger`, plus `level` / `t`. Error lines should include `err`.

---

## 1) Regression

| Check | Result | Evidence / notes | Owner if fail |
| --- | --- | --- | --- |
| Full CI unit/integration green | **BLOCKED** | This environment cannot run Grafana CI. Go 1.26.6 toolchain download from `proxy.golang.org` fails (EOF). No Docker. Lane PRs are draft; GitHub `mergeable_state` was `unstable` for Frontend #26 and CI #27 at QA start. Re-run on a CI-capable runner after PM integrates lanes. | CI (to keep gates green); Backend/Frontend for their package tests |
| e2e smoke: login → open dashboard → query panel → save | **BLOCKED** (spec added; not executed against a live UI) | Playwright spec: `e2e-playwright/smoke-tests-suite/structured-logging-smoke.spec.ts` (dashboard + testdata query + save). Login UI: existing `e2e-playwright/unauthenticated/login.spec.ts`. Blockers: Grafana backend not running (`localhost:3000/3001` health = 000), no `node_modules` at branch start, no Playwright browsers, `make run` needs Go 1.26.6. Closest substitute: QA node tests + Frontend logger Jest on the frontend worktree. Manual steps below. | QA owns the spec. Runtime belongs to whoever can `make run` + `yarn start`. Product regressions → Frontend/Backend |
| Manual: JSON logging, datasource error, UI unchanged, log parses | **PARTIAL** (fixture parse **PASS**; live Grafana **BLOCKED**) | Bundled fixture parse: 6/6 JSON lines with `msg`+`logger`; error line has `err`; no opaque strings; secrets fixture uses `[REDACTED]`. Live process logs not captured. Observability compose also needs Docker. | Backend for live JSON emit; Observability for Loki/Drilldown |
| Frontend build / eslint `no-console` for `public/app` | *filled after running against Frontend #26* | Inventory script: `node scripts/qa/structured-logging/no-console-gate.mjs`. eslint rule `grafana/no-console-public-app` lives on Frontend #26, not on epic-only. | Frontend (rule + remaining call sites); CI (baseline gate) |

## 2) Log parse checks

| Check | Result | Evidence | Owner if fail |
| --- | --- | --- | --- |
| QA fixture JSONL parses with `msg`, `logger`, `err` on errors | *filled by test run* | `node --test scripts/qa/structured-logging/*.test.mjs` and `parse-json-logs.mjs --file scripts/qa/structured-logging/fixtures/valid.jsonl --require-err-on-error --fail-on-secret` | QA |
| Opaque string is rejected | *filled by test run* | `invalid-mixed.jsonl` + unit test `rejects an opaque non-JSON string` | QA |
| Unredacted `Bearer test-secret` is flagged | *filled by test run* | Parser unit test; live redaction is Backend/Security | Security (emit), Backend (pkg/infra/log) |
| Observability sample JSONL (if present) | *run if Observability merged* | `devenv/docker/blocks/structured-logging/fixtures/sample-structured.jsonl` | Observability if fixture is opaque; note one error line in that fixture lacks `err` (`Alert rule evaluation failed`) — warn-only unless `--require-err-on-error` |

### Sample redacted lines (expected shape)

```json
{"t":"2026-09-18T15:00:25.000000000Z","level":"error","msg":"Failed to query datasource","logger":"tsdb.loki","err":"context deadline exceeded","dashboardTitle":"ops / api latency"}
{"t":"2026-09-18T15:00:40.000000000Z","level":"error","msg":"Query data failed","logger":"query_data","err":"Authorization: [REDACTED]"}
```

Dashboard title and query-like strings are **fields**, not `fmt` format-string sinks.

## 3) Manual e2e steps (when Grafana is up)

1. `make run` with `[log.console] format = json` (do not commit local `custom.ini` secrets).
2. Open `http://localhost:3000`, login `admin` / `admin`, skip password change.
3. New dashboard → add panel → TestData **CSV Metric Values** → confirm `A-series` legend.
4. Save dashboard → toast **Dashboard saved**.
5. Break the panel query (invalid datasource / timeout) → UI still shows a panel error; JSON log line has `msg`, `logger`, `err`.
6. Optional: Observability `make devenv sources=structured-logging` then Drilldown → Logs / Explore (Observability lane).

## 4) Commands actually run in this environment

_Filled in after the QA agent run. Do not paste tokens._

```
(pending)
```

## 5) Pass / fail summary

| Lane | QA verdict | Notes |
| --- | --- | --- |
| Backend #25 | *pending live/go tests* | Structured `err` field + JSON recommendation in PR body. Go tests blocked here. |
| Frontend #26 | *pending jest/eslint* | Faro wrapper + `no-console` on that branch. |
| CI #27 | *pending checker* | Incremental baseline gate. |
| Security #24 | *pending* | Leakage tests are Security-owned; QA parser flags unredacted Bearer. |
| Observability #23 | *pending* | Fixture JSON is structured; live Loki blocked (no Docker). |
| QA | *this PR* | Checklist, parse script, smoke spec, evidence. |

**HITL:** do not merge this PR or the epic to `main`.
