# Logging-security audit notes

Lane: `chore/structured-logging-security`  
Regenerate inventory: `make logging-secret-inventory`  
Refresh this file after rescanning. Record only what the tools actually printed.

## How scans were run (this branch)

```bash
# Inventory (report is generated and gitignored)
make logging-secret-inventory
# -> scripts/logging-security/inventory-report.txt

# Frontend production tree
yarn npm audit --recursive --environment production

# Backend (stdlib-only leakage package compiles here)
export PATH="/tmp/go-install/go/bin:$PATH"   # Go 1.26.6 tarball from dl.google.com
export GOTOOLCHAIN=local
go test -count=1 ./pkg/infra/log/secretredact/

# govulncheck was built from the cached golang.org/x/vuln@v1.1.4 sources
# (proxy.golang.org / vuln.go.dev TLS handshake fails in this environment)
/tmp/govulncheck ./pkg/infra/log/secretredact/
/tmp/govulncheck ./pkg/infra/log/...
```

## Inventory (2026-09-18, this run)

| Metric | Count |
| --- | ---: |
| `console.*` / `fmt.Print` / Faro emit sites under `pkg` + `public/app` | 723 |
| secret-like identifier hits (`password\|authorization\|api_key\|secret\|bearer`) | 10394 |
| Authorization / Bearer / token / password hits | 3682 |
| log/Faro files also mentioning those header tokens | 22 |
| files containing **both** a log/Faro site and a secret-like identifier | 33 |

Full line listing: `make logging-secret-inventory`.

Header-overlap files (reviewer starting list; tests included):

- `pkg/services/ldap/settings.go`
- `pkg/services/sqlstore/sqlstore.go`
- `pkg/setting/setting.go`
- `pkg/util/xorm/dialect_postgres.go`
- `public/app/core/components/Login/LoginCtrl.tsx`
- `public/app/core/services/backend_srv.ts`
- `public/app/core/services/echo/backends/grafana-javascript-agent/redactSecrets.ts`
- `public/app/features/admin/state/actions.ts`
- `public/app/features/profile/api.ts`
- `public/app/plugins/datasource/graphite/datasource.ts`

This lane does **not** migrate those call sites. It only inventories them and redacts at emit time for `pkg/infra/log` and Faro `beforeSend`.

## `govulncheck`

Binary: `/tmp/govulncheck` built from cached `golang.org/x/vuln@v1.1.4` (vanity `golang.org/x/*` fetch is blocked here).

### `./pkg/infra/log/secretredact/` (stdlib-only)

Exact tool output:

```text
govulncheck: fetching vulnerabilities: Get "https://vuln.go.dev/index/modules.json.gz": EOF
```

`vuln.go.dev` and `storage.googleapis.com` fail TLS (`OpenSSL SSL_connect: SSL_ERROR_SYSCALL`). No CVE list was produced. This lane adds **no new Go module requirements**.

### `./pkg/infra/log/...`

Package load failed before a vuln scan. Representative errors:

```text
govulncheck: loading packages:
cloud.google.com/go/aiplatform@v1.125.0: Get "https://proxy.golang.org/cloud.google.com/go/aiplatform/@v/v1.125.0.mod": EOF
could not import github.com/go-kit/log (invalid package name: "")
could not import gopkg.in/ini.v1 (invalid package name: "")
```

Same GOPROXY TLS failure. Re-run when the module proxy and vuln DB are reachable:

```bash
export PATH="/usr/local/go/bin:$PATH"   # or any Go 1.26.6
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./pkg/infra/log/secretredact/
govulncheck ./pkg/infra/log/...
```

Existing CI: `.github/workflows/govulncheck.yml` runs `govulncheck` on every pull_request that touches Go files.

## `yarn npm audit`

Command:

```bash
yarn npm audit --recursive --environment production
```

Result on this workspace (full Grafana tree, **not introduced by this lane**):

| Severity | Findings |
| --- | ---: |
| critical | 0 |
| high | 11 |
| moderate | 24 |
| low | 6 |

High findings (existing dependencies this PR does not add or bump):

- `@faker-js/faker` — GHSA-qxc2-j82w-r537 (alerting workspace)
- `browserslist` — GHSA-c83g-rgw3-j3cx, GHSA-73wf-gq98-2v4g
- `fast-uri` — GHSA-5jgf-p345-68v8, GHSA-f65p-4m7j-42xc, GHSA-fph4-wmhf-6fwf, GHSA-jqff-g426-hqxp
- `js-yaml` — GHSA-5p4m-2wfm-xmqj, GHSA-2883-xcg3-v3hh

Remaining items are moderate/low/deprecation noise (`qs`, `react-router`, `rimraf`, `uuid`, babel, …).

This lane adds **no npm dependencies**. Frontend edits are two existing Faro files plus a new redaction helper with no imports beyond `@grafana/faro-core` (already used by `beforeSendHandler`).

## Tests run on this branch

```text
PASS  go test -count=1 ./pkg/infra/log/secretredact/
PASS  yarn jest --watchAll=false --testPathPattern='grafana-javascript-agent/(redactSecrets|beforeSendHandler)'
      42 tests (leakage/reachability + existing bot-filter suite)
```

`go test ./pkg/infra/log/` (emit-through-`New()` tests in `redact_test.go`) could not be compiled here because `pkg/infra/log` imports `github.com/go-kit/log`, `gopkg.in/ini.v1`, and other modules that `proxy.golang.org` cannot fetch (`SSL_ERROR_SYSCALL` / EOF). Those tests stay in-tree for CI / a working GOPROXY.

## Single redaction path

**`pkg/infra/log/secretredact` is the only implementation.** `pkg/infra/log/redact.go` is thin wrappers:

| Security name | Backend alias |
| --- | --- |
| `RedactSecrets` | `Redact` |
| `RedactLogKeyvals` | `RedactFields` |
| `RedactValue(key, val)` | same |
| `Redacted` | same |

`ConcreteLogger.Log`, `newConcreteLogger`, and `with()` all call package-level `RedactFields`.

**Backend lane should use `log.Redact` / `log.RedactFields` / `log.RedactValue` rather than adding a second `redact.go`.**

## Acceptance mapping

| Criterion | Evidence |
| --- | --- |
| Fake `Authorization: Bearer test-secret` not in backend log output | `secretredact.TestBearerTokenDoesNotAppearInEncodedLogOutput`; `log.TestBearerTokenDoesNotAppearInJSONLogBytes` (CI) |
| Same token not in Faro payloads | `redactSecrets.test.ts` + `beforeSendHandler` reachability cases |
| `id_token` redacted when the value does not contain `secret` | `TestRedactSecrets_IDTokenAssignment` / matching Jest case |
| `rg` inventory | `make logging-secret-inventory` |
| govulncheck / yarn audit, no new highs from this lane | this file |
| User-controlled title/query are fields, not format sinks | `TestUserControlledStringsAreFieldsNotFormatSinks` |
| Crafted UI error → Faro excludes cookies/tokens | `Faro beforeSend reachability` in `redactSecrets.test.ts` |
