# Structured logging lint

Grafana is moving application logs off ad-hoc `console.*` / `fmt.Print*` call sites.
This page documents the **CI lint gate** that fails on _new_ uses. It does not migrate
existing call sites; those are grandfathered until the logging migration lanes run.

Use structured loggers instead:

- Frontend: `@grafana/runtime` helpers such as `logInfo`, `logWarning`, `logError`, and `createMonitoringLogger`. For opt-in local debug, use `createDebugLog` from `public/app/core/utils/debugLog.ts`.
- Backend: [`pkg/infra/log`](../../pkg/infra/log/) as described in the [instrumentation guide](../backend/instrumentation.md).

## Frontend (`console.*`)

`no-console` is an ESLint **error** for `public/app/**` (tests, webpack configs, the Echo browser-console backend, and `createDebugLog` are allowlisted). The shared `@grafana/eslint-config` allows `console.log` / `warn` / `error` / `info`; the app override replaces that list (you may see a `grafanaStructuredLoggingSentinel` placeholder in the ESLint message — do not use it).

Existing violations live in [`eslint-suppressions.json`](../../eslint-suppressions.json). Adding a _new_ `console.*` in a suppressed file still fails because the suppression count increases.

Run the same check CI runs:

```bash
yarn lint:ts
```

`yarn lint` also runs this (it includes `lint:ts`). After removing a grandfathered `console.*`, prune stale suppressions:

```bash
yarn lint:prune
```

To allow a legitimate new use, prefer an allowlist entry in `eslint.config.js` (`grafana/no-console`) over a one-off `eslint-disable`.

## Backend (`fmt.Print*` / `log.Print*`)

`forbidigo` rejects `fmt.Print`, `fmt.Printf`, `fmt.Println`, and the standard-library `log.Print*` equivalents in production packages. Tests, `pkg/cmd/` (CLI output), `pkg/build/`, `devenv`, and `scripts` are excluded.

Existing call sites are grandfathered: the check reports only issues introduced after the merge base (`GIT_BASE`, default `remotes/origin/main`).

Run the same check CI runs:

```bash
make lint-go-print
```

Against a different base (for example the structured-logging epic):

```bash
GIT_BASE=origin/chore/structured-logging-epic make lint-go-print
```

To scan the whole tree instead of new-only (noisy until the migration finishes):

```bash
LINT_PRINT_NEW=0 make lint-go-print
```

This target is separate from `make lint-go` so the default backend linters stay unchanged.
