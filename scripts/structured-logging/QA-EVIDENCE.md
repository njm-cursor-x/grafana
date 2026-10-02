# NDS-10 QA regression — structured logging

Overall: **PARTIAL**

| | |
| --- | --- |
| Repo | `njm-cursor-x/grafana` |
| Lane branch | `chore/structured-logging-qa` |
| Epic base | `chore/structured-logging-epic` |
| SHA tested | `75e6dd8aa74ac070b3cc59c62b56fe23babbdfc0` |
| Tip contains | Frontend Faro/`no-console`, backend `pkg/infra/log` JSON + redaction, security leakage tests, observability `structured-logs` block |
| Not done | Merge to `main`. Final PR to `main`. |

`75e6dd8` is the epic tip this run reset onto. It is ahead of `f9afab43` and still contains that backend merge.

## Result

| Check | Result | Evidence |
| --- | --- | --- |
| B. Backend `go test -count=1 ./pkg/infra/log/...` | **Blocked in-tree / pass on the same sources** | `proxy.golang.org` TLS EOF while resolving `cloud.google.com/go/aiplatform@v1.125.0`. Same `pkg/infra/log` sources compiled with git-cloned direct deps: `ok`, 22 top-level tests, 0 failures. |
| B. Frontend Jest (Faro / logging) | **Pass** | 3 suites, 16 tests. |
| C. Structured-logging CI checker | **Pass** | Self-test 11/11 and `check_unstructured_logs.py` passed. |
| C. Full unit + integration CI | **Blocked** | Same module-proxy failure. Grafana's full `make test-go-unit` / integration stack was not started. |
| C. Playwright smoke (login → dashboard → panel → save) | **Blocked** | Spec exists at `e2e-playwright/smoke-tests-suite/smoketests.spec.ts`. No Grafana server, no Playwright browser run. |
| C. JSON file log (`[log.file] format = json`) | **Pass** | Line below parses as JSON. `msg`, `logger`, and `err` are present. `test-secret` is absent. |
| C. eslint `no-console` on `public/app` | **Pass** | `yarn eslint public/app -f stylish` exit 0, no `no-console` findings. |
| D. Bearer not in backend/Faro payloads | **Pass** | Backend redact tests and Faro Jest tests passed. Sample line redacts `Authorization: Bearer test-secret`. |
| D. Secret inventory | **Pass (report)** | `make structured-logging-secret-inventory` exit 0. 31 production matches, almost all redaction rules. |
| D. `govulncheck` | **Blocked** | `proxy.golang.org` and `vuln.go.dev` TLS EOF. No CVE claimed. |
| D. `yarn npm audit --severity high` | **Ran, not a clean bill** | 39 high advisories in the existing tree. None named the logging packages. Not diffed against `main`. |
| E. Observability path documented | **Pass** | `make devenv sources=structured-logs` and `[log.file] format = json` are documented. |
| E. Live Drilldown / Explore | **Deferred** | Docker is not installed on this VM. Left to the Observability lane. |

## B. Unit

### Backend

In-tree command, Go 1.26.6 (downloaded from `dl.google.com` because `/usr/bin/go` is 1.22.2 and `/usr/local/go` is absent):

```text
GOPROXY=https://proxy.golang.org,direct go test -count=1 ./pkg/infra/log/...
pkg/infra/log/log.go:20:2: cloud.google.com/go/aiplatform@v1.125.0: Get "https://proxy.golang.org/cloud.google.com/go/aiplatform/@v/v1.125.0.mod": EOF
FAIL github.com/grafana/grafana/pkg/infra/log [setup failed]
```

`github.com` git and `dl.google.com` work. `proxy.golang.org`, `sum.golang.org`, `goproxy.io`, and `vuln.go.dev` fail the TLS handshake.

Substitute: copy `pkg/infra/log` from this SHA into a module whose `go.mod` replaces only the logger's direct libraries (go-kit, ini, testify, and their small deps) with git clones. `pkg/util.SplitString` is the function from `pkg/util/strings.go`. The app-sdk logger assignment is a compile stub; redaction and JSON tests do not call it. Command: `go test -count=1 ./pkg/infra/log/...` from that module.

```text
ok  github.com/grafana/grafana/pkg/infra/log  0.005s
```

Top-level passes include:

- `TestLogger_redactsSensitiveFields`
- `TestLogger_redactsBearerInsideMessage`
- `TestJSONOutput_redactsSecrets`
- `TestWithPrefix_redactsContext`
- `TestLogToStderr_redactsSecrets`
- `TestIsSensitiveKey`
- `TestJSONAndTextLogsDoNotInventRequestSecrets` (`json` and `text` subtests)
- `TestUserControlledLogFieldsAreNotFormatStringSinks`
- `TestQAEvidenceJSONLine` (harness-only; drives `ReadLoggingConfig` with `[log.file] format = json`)

`TestLogger_redactsSensitiveFields` asserts the encoded record does not contain `test-secret`, `hunter2`, `sk-live-secret`, or `tok-123`, and that `err` is `Authorization: [REDACTED]`.

### Frontend

```text
yarn jest --watchAll=false --ci \
  public/app/core/logging/logger.test.ts \
  packages/grafana-runtime/src/utils/logging.secretLeakage.test.ts \
  public/app/core/services/echo/backends/grafana-javascript-agent/secretLeakage.test.ts
```

```text
Test Suites: 3 passed, 3 total
Tests:       16 passed, 16 total
```

`public/app/core/logging/logger.test.ts` covers Faro `pushLog` level mapping, dropping `Bearer` material from the payload, and not calling `console` when Faro is missing or when `browserLogger.info` runs. The other two suites are the security-lane Faro leakage tests (`document.cookie` / bearer canaries are not copied into `pushLog` or `beforeSend`).

## C. Regression

Structured-logging CI job equivalent:

```text
python3 scripts/structured-logging/check_unstructured_logs.py --self-test
python3 scripts/structured-logging/check_unstructured_logs.py
```

Self-test: 11 tests, OK. Checker: `unstructured logging check passed`.

eslint, after `yarn install --immutable` (exit 0, warnings only):

```text
yarn eslint public/app -f stylish
ESLINT:0
```

No `no-console` findings. The rule is `grafana/no-console-app` in `eslint.config.js`: error on `public/app/**/*.{ts,tsx,js,jsx}`, empty allow-list of console methods. Ignores are tests, webpack configs, and three documented files: `BrowseConsoleBackend.ts`, `debugLog.ts`, `performanceUtils.ts`. Existing violations are in `eslint-suppressions.json` (242 production files in a static cross-check). A static scan found no production `console.*` call outside those suppressions, the documented allow-list, inline `eslint-disable` comments, or commented-out lines. `public/app/core/logging/logger.ts` and `redact.ts` do not reference `console`.

Playwright: `e2e-playwright/smoke-tests-suite/smoketests.spec.ts` is the login → test-data source → dashboard → panel → legend scenario. It was not executed. This VM has no Docker and did not start `make run` or a browser.

### JSON line

Produced by the real `ReadLoggingConfig` + `getLogFormat("json")` + `New("tsdb.prometheus").Error(...)` path, with the error value `Authorization: Bearer test-secret`. File: `scripts/structured-logging/qa-json-sample.log`.

```json
{"dashboardUid":"abc","err":"Authorization: [REDACTED]","level":"error","logger":"tsdb.prometheus","msg":"datasource query failed","t":"2026-10-02T19:05:20.204062379Z"}
```

`json.loads` accepts it. Fields `msg`, `logger`, `err`, `level`, and `t` are present. The string `test-secret` is not.

## D. Vulnerability spot-check

Backend and Faro tests above are the bearer check. The sample line is the encoded backend payload.

`bash scripts/structured-logging/secret-inventory.sh` (also `make structured-logging-secret-inventory`):

| Pattern | Production matches | Where |
| --- | --- | --- |
| Authorization | 6 | `pkg/infra/log/redact.go`, `public/app/core/logging/redact.ts` (rules, not emitted secrets) |
| Bearer | 9 | same redaction sources |
| api key | 8 | redaction rules, plus Faro collector `apiKey` in `echo/init.ts`, `GrafanaJavascriptAgentBackend.ts`, `types.ts` |
| password | 5 | redaction rules |
| grafana_session | 0 | |
| cookie | 3 | redaction key lists |
| **total** | **31** | |

The security-lane note `scripts/structured-logging/AUDIT.md` is stale on this SHA. It says `public/app/core/logging` is absent and that caller-supplied bearers are still copied. This tip has that package, `pkg/infra/log/redact.go`, and tests that reject `Authorization: Bearer test-secret` in the encoded line. Refreshing that note belongs to Security. QA did not change redaction code.

Broader counts (not a failure; the CI checker gates new call sites against `baseline.txt`, 413 lines): about 416 `console.*` hits under `public/app` outside `*.test`/`*.spec`, and about 168 `fmt.Print` hits under `pkg` outside `*_test.go`.

`govulncheck` was not run. `yarn npm audit --severity high --recursive` exited 1 with 39 high advisories (axios, joi, nodemailer, node-forge, js-yaml, svgo, webpack-dev-middleware, and others). No high advisory name was a logging package added on this epic. This is not a comparison to `main`, and it is not a clean bill of health.

## E. Observability

Documented on this SHA:

- `devenv/docker/blocks/structured-logs/README.md`
- `docs/sources/developer-resources/view-structured-logs.md`
- `devenv/README.md`

The path is `mkdir -p data/log && make devenv sources=structured-logs`, then:

```ini
[log]
mode = console file

[log.file]
format = json
```

Live Drilldown was not run. `docker` is not installed. Block left to Observability.

## Owning-lane follow-ups

- **Security:** `AUDIT.md` still describes the pre-merge tree. The integrated tip redacts bearer material in `pkg/infra/log` and the Faro wrapper.
- **Observability:** Live Loki / Drilldown / Explore evidence still needs Docker (`make devenv sources=structured-logs`).
- **CI / environment:** In-tree `go test` and `govulncheck` need `proxy.golang.org` (and `vuln.go.dev`). Both TLS endpoints failed on this VM.
- **QA:** Playwright smoke was not executed. Re-run `e2e-playwright/smoke-tests-suite/smoketests.spec.ts` where Grafana and a browser can start.

## Trade-off

The backend pass is the epic-tip `pkg/infra/log` sources, not `go test` inside the Grafana module graph. A future run with a working module proxy should repeat the in-tree command before anyone treats the backend lane as CI-green.
