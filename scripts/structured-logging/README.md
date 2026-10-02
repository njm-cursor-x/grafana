# Structured logging lint

Pull requests fail when they add unstructured logging in scoped production code.
Existing call sites are recorded in `baseline.txt`, so the gate stays green until
a new call appears.

## What fails

| Language | Scope | Flagged | Not flagged |
| --- | --- | --- | --- |
| Go | `pkg/` | `fmt.Print`, `fmt.Printf`, `fmt.Println`; stdlib `log` print, fatal, panic, and `New` | `fmt.Sprint*`, `fmt.Errorf`, `fmt.Fprint*` |
| TypeScript | `public/app/` | `console.log` and the other logging methods, including `console?.log` | calls inside comments or string literals |

`allowlist.txt` lists the paths that are tests, stories, fixtures, generated files,
dev-only code, or copied libraries. Each entry says which kind of path it is.

## Run locally

```bash
make lint-structured-logging
```

The same command the workflow runs:

```bash
python3 scripts/structured-logging/check_unstructured_logs.py
python3 scripts/structured-logging/check_unstructured_logs.py --self-test
```

## Baseline

Do not add a baseline row to permit a new call. Go code should use `pkg/infra/log`.
Frontend code should use the structured logger for `public/app`.

Removing a call does not fail the check. After a migration removes calls, refresh
the baseline so those lines cannot come back unnoticed:

```bash
python3 scripts/structured-logging/check_unstructured_logs.py --update
```

GitHub Actions workflow: `.github/workflows/structured-logging-lint.yml`.

It runs on every pull request, and on pushes to `main`, `chore/structured-logging-epic`, and `chore/structured-logging-ci`. The job runs the self-test, then the tree check. A new `console.*` call under `public/app`, or a new `fmt.Print*` / stdlib `log` call under `pkg/`, fails the job.
