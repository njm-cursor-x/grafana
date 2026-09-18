# Logging-security audit notes

Lane: `chore/structured-logging-security`  
Regenerate inventory: `make logging-secret-inventory`  
Refresh this file after rescanning.

## How scans were run (this branch)

```bash
# Inventory (committed process; report is generated and gitignored)
make logging-secret-inventory
# -> scripts/logging-security/inventory-report.txt

# Frontend production tree (Yarn 4, bundled yarn to avoid corepack)
node .yarn/releases/yarn-4.17.1.cjs npm audit --recursive --environment production

# Backend (intended; blocked in this environment — see below)
govulncheck ./pkg/infra/log/...
```

## Inventory (2026-09-18)

| Metric | Count |
| --- | ---: |
| `console.*` / `fmt.Print` sites under `pkg` + `public/app` | 687 |
| secret-like identifier hits (`password\|authorization\|api_key\|secret\|bearer`) | 10351 |
| files containing **both** a log site and a secret-like identifier | 29 |

Full line listing: `make logging-secret-inventory`.

Overlap files (production-ish, excluding `*_test.go` / testdata) worth a later pass by Backend/Frontend lanes:

- `pkg/apimachinery/utils/meta.go`
- `pkg/components/satokengen/cmd/main.go`
- `pkg/services/ldap/settings.go`
- `pkg/services/sqlstore/sqlstore.go`
- `pkg/setting/setting.go`
- `pkg/util/xorm/dialect_postgres.go`
- `public/app/core/components/Login/LoginCtrl.tsx`
- `public/app/core/services/backend_srv.ts`
- `public/app/core/services/echo/init.ts`
- `public/app/features/admin/state/actions.ts`
- `public/app/features/alerting/unified/components/receivers/form/fields/OptionField.tsx`
- `public/app/features/auth-config/FieldRenderer.tsx`
- `public/app/features/profile/api.ts`
- `public/app/plugins/datasource/azuremonitor/azure_monitor/azure_monitor_datasource.ts`
- `public/app/plugins/datasource/azuremonitor/credentials.ts`
- `public/app/plugins/datasource/cloudwatch/components/ConfigEditor/ConfigEditor.tsx`
- `public/app/plugins/datasource/graphite/datasource.ts`

This lane does **not** migrate those call sites.

## `govulncheck`

Could not be executed in this Cloud Agent environment:

- Distro Go is 1.22; `go.mod` requires **1.26.6** (installed locally from `https://dl.google.com/go/go1.26.6.linux-amd64.tar.gz`).
- `proxy.golang.org` and other GOPROXY hosts fail TLS (`SSL_ERROR_SYSCALL` / EOF).
- `govulncheck` itself lives at `golang.org/x/vuln` (vanity import) and cannot be `go install`ed without that proxy.
- `GOPROXY=direct` works for `github.com/*` but not `golang.org/x/*` / `k8s.io/*` vanity paths.

Re-run when GOPROXY works:

```bash
export PATH="/usr/local/go/bin:$PATH"   # or any Go 1.26.6
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./pkg/infra/log/...
```

This change adds **no new Go module requirements**. The only new package is `pkg/infra/log/secretredact` (stdlib only). There is no new high from this lane's Go diff.

## `yarn npm audit`

Command:

```bash
node .yarn/releases/yarn-4.17.1.cjs npm audit --recursive --environment production
```

Result on this workspace (full Grafana tree, **not introduced by this lane**):

- **0 critical**
- **11 high** findings, all in existing dependencies this PR does not add or bump:
  - `@faker-js/faker` (alerting workspace)
  - `browserslist` (2)
  - `fast-uri` (4)
  - `js-yaml` (4)
- Remaining items are moderate/low/deprecation noise (`qs`, `react-router`, `rimraf`, `uuid`, babel, …)

This lane adds **no npm dependencies**. Frontend edits are two existing Faro files plus a new redaction helper with no imports beyond `@grafana/faro-core` (already used by `beforeSendHandler`).

## Tests run on this branch

```text
PASS  go test -count=1 ./pkg/infra/log/secretredact/
PASS  yarn jest --watchAll=false --testPathPattern='grafana-javascript-agent/(redactSecrets|beforeSendHandler)'
      41 tests (new leakage/reachability + existing bot-filter suite)
```

`go test ./pkg/infra/log/` (full logger package, including emit-through-`New()` tests in `redact_test.go`) could not be compiled here because `pkg/infra/log` imports `k8s.io/*` / `gopkg.in/ini.v1` via existing helpers. Those tests stay in-tree for CI / a working GOPROXY.

## Single redaction path

**`pkg/infra/log/secretredact` is the only implementation.** `pkg/infra/log/redact.go` is thin wrappers:

| Security name | Backend alias |
| --- | --- |
| `RedactSecrets` | `Redact` |
| `RedactLogKeyvals` | `RedactFields` |
| `RedactValue(key, val)` | same |
| `Redacted` | same |

`ConcreteLogger.Log`, `newConcreteLogger`, and `with()` all call package-level `RedactFields`.

**Backend should delete its inline `redact.go` on #25 and use `log.Redact` / `log.RedactFields` / `log.RedactValue`.** Do not add a second thin `redact.go` — Security owns that file.

## Acceptance mapping

| Criterion | Evidence |
| --- | --- |
| Fake `Authorization: Bearer test-secret` not in backend log output | `secretredact.TestBearerTokenDoesNotAppearInEncodedLogOutput`; `log.TestBearerTokenDoesNotAppearInJSONLogBytes` (CI) |
| Same token not in Faro payloads | `redactSecrets.test.ts` + `beforeSendHandler` reachability cases |
| `rg` inventory | `make logging-secret-inventory` |
| govulncheck / yarn audit, no new highs | this file |
| User-controlled title/query are fields, not format sinks | `TestUserControlledStringsAreFieldsNotFormatSinks` (Go + assertion that `%s` survives) |
| Crafted UI error → Faro excludes cookies/tokens | `Faro beforeSend reachability` in `redactSecrets.test.ts` |
