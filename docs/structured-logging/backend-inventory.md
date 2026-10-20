---
title: Backend logging call-site inventory
---

# Backend logging call-site inventory

This inventory lists `fmt.Print*` and ad-hoc standard library `log.` calls in production Go code under `pkg/`, and records which ones moved to `pkg/infra/log`. It's part of Jira NDS-10, the structured logging backend lane.

## How the inventory is generated

The inventory uses `rg` and excludes tests, test data, generated code, and comment lines. `pkg/ruleguard.rules.go` is also excluded because it contains linter match patterns as strings, not calls.

```sh
rg -n --type go \
  -g '!*_test.go' -g '!**/testdata/**' -g '!*_gen.go' -g '!*.gen.go' \
  -g '!**/codegen/**' -g '!*.pb.go' -g '!zz_generated*' -g '!ruleguard.rules.go' \
  '\bfmt\.Print(f|ln)?\(|\blog\.(Print|Printf|Println|Fatal|Fatalf|Fatalln|Panic|Panicf|Panicln)\(' pkg/ \
  | grep -vE '^[^:]+:[0-9]+:\s*//'
```

Without the comment filter, the same search returns 130 lines on `main`. 23 of those lines are commented-out code or doc examples.

## Summary

The search returns 107 call sites on `main` and 92 after this change. All 15 migrated call sites are runtime server code. The 92 that remain are CLI output, code-generation tools, test helpers, output that runs before logging starts, or call sites held back by the security safeguard.

| Category | Before | After |
| --- | --- | --- |
| Runtime server code, migrated | 15 | 0 |
| Runtime server code, held by security safeguard | 2 | 2 |
| Runs before logging is initialized | 5 | 5 |
| Logger's own fallback output | 2 | 2 |
| CLI tools and user-facing CLI output | 70 | 70 |
| Code generators (`go:build ignore` or `main` generators) | 3 | 3 |
| Test-only helpers in non-test files | 7 | 7 |
| Dead code (`if false`) | 1 | 1 |
| Tutorial code | 2 | 2 |
| Comment-free total | 107 | 92 |

## Counts per package

This table shows the count for each package on `main` and after the change.

| Package | Before | After | Category |
| --- | --- | --- | --- |
| `pkg/build/wire/cmd/wire` | 27 | 27 | CLI tool |
| `pkg/cmd/grafana-cli/logger` | 18 | 18 | CLI tool output |
| `pkg/services/ngalert/api/tooling/cmd/clean-swagger` | 13 | 13 | CLI tool |
| `pkg/components/simplejson` | 9 | 0 | Migrated |
| `pkg/server/bootstrap` | 7 | 7 | Runs before logging starts (3), `--version` output (4) |
| `pkg/components/satokengen/cmd` | 6 | 6 | CLI tool |
| `pkg/services/live/pipeline` | 4 | 0 | Migrated |
| `pkg/services/sqlstore/sqlutil` | 3 | 3 | Test-only helper |
| `pkg/services/sqlstore` | 3 | 3 | Test-only helper (`SetupTestDB`, `CleanupTestDB`) |
| `pkg/setting` | 2 | 2 | Runs before logging starts |
| `pkg/services/publicdashboards/internal/commands/generate_datasources` | 2 | 2 | Code generator |
| `pkg/infra/log` | 2 | 2 | Logger's own syslog fallback |
| `pkg/cmd/grafana` | 2 | 2 | CLI output |
| `pkg/build/wire/_tutorial` | 2 | 2 | Tutorial code |
| `pkg/storage/unified/resource` | 1 | 1 | Test-only snapshot helper |
| `pkg/services/preference` | 1 | 1 | Code generator (`go:build ignore`) |
| `pkg/services/ldap` | 1 | 1 | Held by security safeguard |
| `pkg/services/apiserver/builder` | 1 | 1 | Dead code |
| `pkg/expr/sql` | 1 | 0 | Migrated |
| `pkg/components/dashdiffs` | 1 | 0 | Migrated |
| `pkg/apimachinery/utils` | 1 | 1 | Held by security safeguard |

## Migrated call sites

These call sites now use `pkg/infra/log` with constant messages and key-value fields, or no longer log at all.

- **`pkg/components/simplejson/simplejson.go`:** Nine `log.Panicf` calls in the `Must*` accessors are now `panic(fmt.Sprintf(...))`. The recovered panic value is the same string. The unstructured line that `log.Panicf` wrote to stderr is gone.
- **`pkg/services/live/pipeline/devdata.go`:** `log.Println(payload)` is now `logger.Debug("posting live pipeline dev data", "payload", ...)`. Three `log.Fatal(err)` calls are now `logger.Error("failed to post live pipeline dev data", "url", ..., "err", err)` followed by a return. Before, any HTTP error in this goroutine killed the whole Grafana process. This code only runs when `GF_LIVE_PIPELINE_DEV` is set.
- **`pkg/expr/sql/frame_table.go`:** `fmt.Printf("------- Unsupported field type: ...")` is now `Warn("unsupported field type, falling back to JSON", "fieldType", ...)`.
- **`pkg/components/dashdiffs/formatter_json.go`:** `fmt.Printf("%v\n", err)` is now `Error("failed to render JSON diff template", "err", err)`.

## Call sites intentionally left in place

These call sites stay as they are, for the reasons listed.

- **Runs before logging starts:** `pkg/server/bootstrap/diagnostics.go` (pprof and trace enabled notices), `pkg/server/bootstrap/lifecycle.go` (elevated privileges warning), and `pkg/setting/setting.go` (missing or unparsable `defaults.ini`) all run before `loadConfig` calls `log.ReadLoggingConfig`. Until then, the root logger writes to `io.Discard`, so moving these messages to the logger would silently drop them.
- **CLI and `--version` output:** `pkg/server/bootstrap/servers.go` `printVersion`, `pkg/cmd/grafana/main.go`, `pkg/cmd/grafana-cli/logger`, and the `cmd/` tools write output that a user reads directly. They aren't server logs.
- **Logger's own fallback:** `pkg/infra/log/syslog.go` reports that the syslog handler failed. It can't log through the logger that just failed.
- **Test-only helpers:** `pkg/services/sqlstore/sqlutil/sqlutil.go`, `pkg/services/sqlstore/sqlstore.go` (`SetupTestDB`, `CleanupTestDB`), and `pkg/storage/unified/resource/table.go` (`AssertTableSnapshot`) only run from tests.
- **Dead code:** `pkg/services/apiserver/builder/helper.go` is inside an `if false` audit block.

## Held by the security safeguard

These call sites need human approval before anyone changes them, because they sit on a security boundary.

- **`pkg/services/ldap/settings.go:226`:** `fmt.Println("unknown")` in `assertNotEmptyCfg`. The LDAP package is part of authentication.
- **`pkg/apimachinery/utils/meta.go:907`:** `fmt.Printf("ERR: %v", r)` in the panic recovery of `SetSecureValues`, which handles secure values (secrets). `pkg/apimachinery` is also a separate Go module that can't import `pkg/infra/log`.
