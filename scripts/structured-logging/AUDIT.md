# NDS-10 security lane — leakage inventory and vuln audit

Scope is secret-leakage tests, the inventory target, and this note. No production auth, crypto, or secret-handling code was changed.

Epic base: `chore/structured-logging-epic` at `6126b3cc6c75e4e76b972cd07c627cf079aeb537`. On that SHA, `public/app/core/logging` does not exist. Tests target the logging and Faro paths that are already in the tree.

## What the tests lock

Canaries are fake fixtures (`nds10-bearer-canary`, `nds10-session-canary`, and the query/referer/userinfo variants). They are not credentials.

| Test | Path | Assertion |
| --- | --- | --- |
| `TestJSONAndTextLogsDoNotInventRequestSecrets` | `pkg/infra/log` JSON and text loggers | A completed-request line does not gain a bearer, session cookie, or userinfo the logger was not given. |
| `TestUserControlledLogFieldsAreNotFormatStringSinks` | `pkg/infra/log` JSON logger | A dashboard title containing `%s` is stored as a field. |
| `TestPrepareLogParamsOmitsAuthorizationAndSession` | `pkg/middleware/loggermw` | `Authorization`, `Cookie`, URL userinfo, and the `api_key` query are absent from request-log params. `auth_token` on `Referer` is rewritten to `hidden` by `pkg/util.SanitizeURI`. The error string stays a field (`upstream status %s`). |
| `TestFrontendLoggingOmitsRequestAuthorizationAndSession` | `POST /log-grafana-javascript-agent` | The handler log record for a Faro log plus exception does not include the request `Authorization` or `Cookie`. The page URL, message, dashboard uid, and stack function are present. |
| Faro logging secret leakage | `packages/grafana-runtime/src/utils/logging.ts` | `logInfo` / `logWarning` / `logDebug` / `logError` / `createMonitoringLogger` do not copy `document.cookie` into `faro.api.pushLog`, `pushError`, `pushMeasurement`, or console output. |
| Faro beforeSend secret leakage | `beforeSendHandler` | With the bot filter on or off, the transport item is not given a session cookie or bearer that exists only on `document.cookie`. |

## Blocker — caller-supplied secrets are still copied

These tests do not prove that a bearer token is stripped once a caller has already placed it in a log message, a Faro context field, or an exception value. On this SHA:

- `pkg/infra/log` writes the message and key/value arguments it is given.
- `pkg/api/frontend_logging.go` copies Faro log context, exception value/stack, and `page_url` into the frontend logger.
- `packages/grafana-runtime/src/utils/logging.ts` passes context through to `faro.api.pushLog` / `pushError`.
- `beforeSendHandler` only drops bot user agents. It does not redact payload fields.
- `SanitizeURI` masks `auth_token`, `X-Amz-Signature`, `X-Goog-Signature`, and Azure `sig` when `sv` is present. It does not mask `password`, `api_key`, or URL userinfo on `Referer`.

Redacting those would change secret handling in the logger. This lane did not ship that change. Backend and frontend lanes still need a redaction step before these tests can be extended to reject secrets that arrive inside the event body.

## Inventory

```bash
make structured-logging-secret-inventory
```

Script: `scripts/structured-logging/secret-inventory.sh`.

Ran on this branch. Production files only (unit tests excluded). Result: **3 matches, all `apiKey` on the Faro collector transport**, not log lines:

- `public/app/core/services/echo/init.ts`
- `public/app/core/services/echo/backends/grafana-javascript-agent/GrafanaJavascriptAgentBackend.ts`
- `public/app/core/services/echo/backends/grafana-javascript-agent/types.ts`

No `Authorization`, `Bearer`, `password`, `grafana_session`, or `cookie` hits in the scanned production paths. `public/app/core/logging` is absent and was skipped.

The script exits 0 whether or not it finds matches. It is a report, not a CI gate.

## govulncheck

Not completed. No CVEs are claimed.

What was attempted:

- `proxy.golang.org` failed the TLS handshake (`SSL_ERROR_SYSCALL` on connect). `go tool -modfile=.citools/src/govulncheck/go.mod govulncheck` could not download `golang.org/x/vuln v1.7.0` from that proxy.
- A local `govulncheck` v1.7.0 binary was compiled from a git clone of `github.com/golang/vuln` at tag `v1.7.0`. `govulncheck -version` printed `Scanner: govulncheck@v1.7.0` and `DB: https://vuln.go.dev`. That is a version check of the binary, not a scan of this repo.
- A source scan of `./pkg/infra/log` was not finished. `go test` module resolution for the Grafana graph stalled while fetching `cloud.google.com/go/aiplatform` (`github.com/googleapis/google-cloud-go`). The vulnerability database at `vuln.go.dev` was not queried.

Do not treat this note as a clean bill of health. Re-run when the module proxy and vuln DB are reachable:

```bash
# from repo root, using the tool pin in .citools/src/govulncheck/go.mod
GOVULNCHECK=$(GOWORK=off go tool -n -modfile="$PWD/.citools/src/govulncheck/go.mod" golang.org/x/vuln/cmd/govulncheck)
"$GOVULNCHECK" ./pkg/infra/log/ ./pkg/middleware/loggermw/ ./pkg/api/frontendlogging/
```

Compare that output with `main` before calling any finding new.

## Commands

```bash
make structured-logging-secret-inventory

go test -count=1 ./pkg/infra/log/ -run 'TestJSONAndTextLogsDoNotInventRequestSecrets|TestUserControlledLogFieldsAreNotFormatStringSinks'
go test -count=1 ./pkg/middleware/loggermw/ -run TestPrepareLogParamsOmitsAuthorizationAndSession
go test -count=1 ./pkg/api/ -run TestFrontendLoggingOmitsRequestAuthorizationAndSession

# login shell so Node matches .nvmrc
bash -lc 'yarn jest --watchAll=false packages/grafana-runtime/src/utils/logging.secretLeakage.test.ts public/app/core/services/echo/backends/grafana-javascript-agent/secretLeakage.test.ts'
```

## Evidence on this run

| Check | Result |
| --- | --- |
| `make structured-logging-secret-inventory` | Passed. 3 production `apiKey` hits, listed above. |
| Go leakage tests | Not executed. Module fetch did not finish (`google-cloud-go` / `aiplatform`). |
| Jest leakage tests | Not executed. `node_modules` is not installed in this environment, and install was not started after the Go fetch was stopped. |
| `govulncheck` scan | Not executed. No findings. No CVEs invented. |
