# NDS-10 baseline and integration checks

Run: 2026-10-20 17:26 EDT on `chore/structured-logging-epic`.
Findings referenced as F-NNN live in [findings-log.md](findings-log.md).

## Trees checked

| Tree     | Commit        | Contents                                                                                                    |
| -------- | ------------- | ----------------------------------------------------------------------------------------------------------- |
| Baseline | `6126b3cc6c7` | Epic before any lane integration; identical to `main` at that point                                         |
| Epic     | `96662f13347` | Baseline plus merge of `chore/structured-logging-backend` up to `759c7cf7bf5` (`error` -> `err` key rename) |

The backend merge applied cleanly with no conflicts.

## Environment

- Go 1.26.6 (matches `go.mod`), Node 22.23.3, Yarn 4.17.1 from `.yarn/releases`, golangci-lint v2.12.2 (release binary, checksum verified, matches `.citools`).
- Module proxies are unreachable from the sandbox (F-001). Go dependencies were fetched directly from GitHub and verified against `go.sum`. A handful of `go.mod`-only entries from blocked hosts that are never compiled were stubbed in throwaway worktrees outside the repo; `-mod=readonly` would have failed if this changed any selected version.

## Results

| Check                | Command                                                                                                                                                   | Baseline                                                         | Epic                                                             |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------- | ---------------------------------------------------------------- |
| Go build             | `go build ./pkg/services/live/pipeline/...`                                                                                                               | blocked (F-002)                                                  | blocked (F-002)                                                  |
| Go vet               | `go vet` per package, see scope below                                                                                                                     | pass (6 of 6 loadable packages); root `pipeline` blocked (F-002) | pass (6 of 6 loadable packages); root `pipeline` blocked (F-002) |
| Go lint              | `golangci-lint run --config .golangci.yml <pkg>/...`                                                                                                      | 0 issues in 4 packages; `pipeline/...` typecheck blocked         | 0 issues in 4 packages; `pipeline/...` typecheck blocked         |
| Go unit tests        | `go test -count=1 ./pkg/infra/log/... ./pkg/services/live/pipeline/... ./pkg/components/simplejson/... ./pkg/components/dashdiffs/... ./pkg/expr/sql/...` | 143 passed, 0 failed                                             | 171 passed, 0 failed                                             |
| TypeScript           | `yarn typecheck`                                                                                                                                          | pass                                                             | n/a (no frontend changes)                                        |
| ESLint and stylelint | `yarn lint`                                                                                                                                               | pass, 0 warnings                                                 | n/a (no frontend changes)                                        |
| Jest                 | `yarn jest --no-watch --ci public/app/core`                                                                                                               | 156 of 156 suites; 1513 passed, 1 skipped, 1 todo                | n/a (no frontend changes)                                        |

Go test counts per package (baseline -> epic): `infra/log` 23 -> 39, `components/simplejson` 17 -> 27, `expr/sql` 78 -> 80, `components/dashdiffs` 3 -> 3, `live/pipeline/tree` 19 -> 19, `live/pipeline/pattern` 3 -> 3. The `infra/log` subpackages `logtest`, `slogadapter`, `term`, and `text` have no tests. The root `pkg/services/live/pipeline` package fails at build setup on both trees for environmental reasons (F-002), so its tests have not run yet.

## Static inventory

Counts exclude `_test.go` files, `testdata`, and `pkg/infra/log`.

| Inventory                                         | Baseline         | Epic             |
| ------------------------------------------------- | ---------------- | ---------------- |
| `fmt.Print*` calls in `pkg/`                      | 72 in 27 files   | 70 in 25 files   |
| stdlib `log` imports in `pkg/`                    | 9 files          | 7 files          |
| stdlib `log.Print*/Fatal*/Panic*` calls in `pkg/` | 47               | 32               |
| `console.*` calls in `public/app` (non-test)      | 422 in 259 files | 422 in 259 files |
| `"error", err` pairs in `pkg/`                    | 1284             | 1284             |
| `"err", err` pairs in `pkg/`                      | 630              | 634              |
