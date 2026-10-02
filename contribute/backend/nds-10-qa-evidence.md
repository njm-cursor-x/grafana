# NDS-10 QA regression evidence

Lane: `chore/structured-logging-qa`  
Epic: `chore/structured-logging-epic` (`6126b3cc6c75e4e76b972cd07c627cf079aeb537`, same commit as `main`)  
Epic does not contain the Backend or Frontend commits. Evidence below is from those lane SHAs, not from an epic merge.

| Lane | Branch | SHA | PR |
| --- | --- | --- | --- |
| Epic / target base | `chore/structured-logging-epic` | `6126b3cc6c75e4e76b972cd07c627cf079aeb537` | — |
| Backend | `chore/structured-logging-backend` | `a46bb31a4589252af7c1e5413e8f0f7faa716351` | https://github.com/njm-cursor-x/grafana/pull/47 |
| Frontend | `chore/structured-logging-frontend` | `3ea4a758335923a748365b61c1e5c424691b8299` | https://github.com/njm-cursor-x/grafana/pull/46 |
| Security (spot-check) | `chore/structured-logging-security` | `dd9476ec7340c169f222eb8c5aab0cf17402c2a0` | https://github.com/njm-cursor-x/grafana/pull/45 |
| Observability (spot-check) | `chore/structured-logging-observability` | `61d479684249634bef1144be6e67854c5d9adc79` | https://github.com/njm-cursor-x/grafana/pull/43 |

## Evidence template

| Item | Result | Link / SHA / notes |
| --- | --- | --- |
| Target SHA(s) | Epic lacks BE+FE | Epic `6126b3cc6c75`. Backend `a46bb31a458`. Frontend `3ea4a758335`. |
| CI | FAIL | Not green. Jobs are pending or skipped on draft PRs into the epic. Backend: https://github.com/njm-cursor-x/grafana/pull/47 . Frontend: https://github.com/njm-cursor-x/grafana/pull/46 . |
| e2e smoke | FAIL | Not executed. `grafana-server` cannot be built here (`proxy.golang.org` TLS fails; the module graph is not on GitHub). Smoke spec now includes save; see below. |
| JSON log parse | PASS (logger unit) / UI not browser-checked | Sample from Backend `New("tsdb.prometheus").Error` through `newJSONLevelLogger`. |
| no-console | PASS (scoped rule) | ESLint exit 0 on the files in `grafana/structured-logging-no-console`. No added `console.*` vs `main`. Repo-wide ban is not configured (461 non-test `console.*` sites remain). |
| Redaction spot-check | PASS (recheck @ `55ce7ad35a7`) | `nonce` stays; value is `[REDACTED]`. `go test -mod=readonly -count=1 ./pkg/infra/log/secretredact/` passed. Prior FAIL was Backend `a46bb31a458` before fix `c7ade407925`. |
| Drilldown/Explore | FAIL | Not run. Promtail JSON stage is on the Observability branch. Loki was not started. |

## Backend unit

`go test ./pkg/infra/log/...` on `a46bb31a458` does **not** pass:

```text
package github.com/grafana/grafana/pkg/infra/log
	imports github.com/grafana/grafana/pkg/infra/log/slogadapter from slog_bridge_test.go
	imports github.com/grafana/grafana/pkg/infra/log from adapter.go: import cycle not allowed in test
FAIL	github.com/grafana/grafana/pkg/infra/log [setup failed]
```

`pkg/infra/log/slog_bridge_test.go` is `package log` and imports `slogadapter`, which imports `log`. That cycle stops the package test before `TestSlogHandler_ForwardsLevelsGroupsAndRedacts` runs.

With that file excluded, the rest of `./pkg/infra/log` and `./pkg/infra/log/secretredact` passed (Go 1.26.6). This was not an in-module `go test` of the Grafana `go.mod`: `proxy.golang.org` is unreachable. Sources were the lane files. `github.com/grafana/grafana-app-sdk/logging.NewSLogLogger` was a local stub because the real constructor imports `go.opentelemetry.io/otel` and `k8s.io/klog`, which could not be fetched. JSON bytes under test come from `json_level.go`, `ConcreteLogger.Log`, and `secretredact`, which do not read that stub back.

Their test `TestJSONLogger_RedactsSecretsAndEmitsStableLevel` passed in that setup. `Authorization: Bearer test-secret` did not appear in the JSON line.

## JSON sample (redacted)

Datasource error path, JSON format, fixed timestamp:

```json
{"Authorization":"[REDACTED]","dashboardTitle":"QA smoke","err":"datasource returned status 400: bad request","level":"error","logger":"tsdb.prometheus","msg":"query failed","t":"2026-10-02T21:00:00Z"}
```

Parsed keys present: `msg`, `logger`, `err`, plus `level` and `t`. The probe value `test-secret` is absent. Stock go-kit JSON without the wrapper is `{"lvl":"eror","msg":"x"}` (no stable `level`).

UI unchanged was **not** checked in a browser. `dashboardControls.test.ts` still expects the datasource failure to emit the same variable event and to call Faro instead of `console.warn`.

## Frontend unit and no-console

On `3ea4a758335`, after `yarn install --immutable` (Yarn 4.17.1 from `.yarn/releases`):

- `yarn jest --watchAll=false --ci --testPathPattern=logging` — 2 suites, 32 tests, pass (`faroLogger.test.ts`, `packages/grafana-runtime/src/services/logging/registry.test.ts`).
- `faroLogger.test.ts` + `dashboardControls.test.ts` — 2 suites, 23 tests, pass. Faro `pushLog` level/context asserted; `console.*` spies not called. Bearer probe redacted to `[REDACTED]`.

Faro-shaped payload asserted by `faroLogger.test.ts` (not a live browser capture):

```json
{"args":["query failed"],"level":"error","context":{"source":"grafana.dashboard.datasource","error":"rejected Bearer [REDACTED]"}}
```

ESLint on `public/app/core/logging`, `public/app/index.ts`, `public/app/app.ts`, and `public/app/features/dashboard-scene/utils/dashboardControls.ts` exited 0. The `no-console` error applies only to those files (tests ignored). Diff vs `main` adds no `console.*` calls. About 461 non-test `console.*` call sites remain elsewhere in `public/app`. The Frontend PR leaves a repo-wide ban to the CI lane.

## e2e smoke

Existing `e2e-playwright/smoke-tests-suite/smoketests.spec.ts` logged in (auth project), created a testdata datasource, opened a dashboard, and queried a panel. It did not save. This lane adds the save-toolbar and save-drawer clicks and expects the `Dashboard saved` status.

Playwright was not started. `e2e-playwright/start-server` needs a built `grafana-server`, and that build cannot download the Go module graph here.

## Security spot-check (#45, `dd9476ec734`)

- `pkg/infra/log/secretredact` tests, including `TestBearerTokenDoesNotAppearInEncodedLogOutput` and `TestRedactValue_AuthTokenAndNonceDoNotLeakOpaqueValues`: pass. Encoded output does not contain `test-secret`.
- `redactSecrets.test.ts`: 8 tests, pass. Covers bearer, `id_token=`, session cookie, and keys `token` / `authToken` / `nonce` on the Faro `beforeSend` path.
- `scripts/logging-security/AUDIT.md` still lists high residual leakage: `raw_json` / claim JSON in `pkg/login/social/connectors/generic_oauth.go` and `gitlab_oauth.go` under a non-sensitive key. Emit-time redaction does not strip those.
- `govulncheck` / `yarn npm audit` were not run. `proxy.golang.org` is blocked, and Security's own note says those scans were not re-run.

Backend vs Security: Backend `secretredact` removed the exact key `nonce`. Re-running Security's nonce test against Backend `redact.go` fails:

```text
opaque credential leaked: {"authToken":"[REDACTED]","msg":"session created","nonce":"n0nce-value-998877","token":"[REDACTED]","userID":7}
```

`authToken` and `token` are still redacted via the `token` fragment. `nonce` is not.

## Observability

`devenv/docker/blocks/loki-promtail/promtail-config.yaml` on `61d47968424` sets `service_name: grafana` and a JSON stage that promotes `level` (or go-kit `lvl`, with `eror` mapped to `error`) and `logger`. Explore / Drilldown were not opened. No Loki process was started.

## Redaction recheck @ epic `55ce7ad35a7`

PASS. QA branch merged epic tip `55ce7ad35a778c16481b28f08454d574d266bffd` (fix `c7ade40792562a1b551861d4c81465c40c54454c`).

`pkg/infra/log/secretredact/redact.go` lists `"nonce"` in `sensitiveKeys`. `RedactValue` replaces the value with `[REDACTED]` and keeps the key. `TestRedactValue_AuthTokenAndNonceDoNotLeakOpaqueValues` asserts `"nonce":"[REDACTED]"` and fails if the key is dropped.

```text
go test -mod=readonly -count=1 ./pkg/infra/log/secretredact/
ok  	github.com/grafana/grafana/pkg/infra/log/secretredact	0.003s

go test -mod=readonly -count=1 -v ./pkg/infra/log/secretredact/ -run 'TestRedactValue_AuthTokenAndNonce|TestBearerTokenDoesNotAppear'
--- PASS: TestBearerTokenDoesNotAppearInEncodedLogOutput (0.00s)
--- PASS: TestRedactValue_AuthTokenAndNonceDoNotLeakOpaqueValues (0.00s)
ok  	github.com/grafana/grafana/pkg/infra/log/secretredact	0.003s
```

Go 1.26.6. `-mod=readonly` did not need a module download; this package is stdlib-only.

## Not done

- Slack `#njm-demo-channel` was not posted. This run was instructed not to message the human.
- No merge to `main` or to the epic.
- No production deploy, no change to auth or other security boundaries, no billing or spend path.
