# NDS-10 structured logging integration audit

Integrate of the five lane drafts onto `chore/structured-logging-epic`.
This document is the audit trail for that integrate. Merging the epic into `main` is human-only. Do not merge this work to `main`.

QA regression is recorded below. Overall result: **FAIL (partial)**. The only product gate still open is the Backend/Security `nonce` mismatch. Backend owns that fix.

## Lane pull requests

| Lane | PR | Branch | Head integrated | What landed |
| --- | --- | --- | --- | --- |
| Observability | https://github.com/njm-cursor-x/grafana/pull/43 | `chore/structured-logging-observability` | `61d47968424` | Promtail and Alloy label `service_name=grafana`, `level`, and `logger`. Example ini for `[log.file] format = json`. Runbook in `contribute/backend/structured-logging-loki.md`. Pipeline YAML was not changed in the follow-up commit. |
| CI | https://github.com/njm-cursor-x/grafana/pull/44 | `chore/structured-logging-ci` | `9a271f84112` | `make lint-structured-logging`, `scripts/structured-logging/check_unstructured_logs.py`, baseline, allowlist, and `.github/workflows/structured-logging-lint.yml`. |
| Security | https://github.com/njm-cursor-x/grafana/pull/45 | `chore/structured-logging-security` | `dd9476ec734` | `pkg/infra/log/secretredact`, emit-time `RedactFields`, Faro `redactSecrets` in `beforeSend`, `nonce` / `authToken` coverage, `make logging-secret-inventory`. |
| Frontend | https://github.com/njm-cursor-x/grafana/pull/46 | `chore/structured-logging-frontend` | `3ea4a758335` | `createFaroLogger` and call sites in `app.ts`, `index.ts`, and `dashboardControls.ts`. `no-console` on those files. |
| Backend | https://github.com/njm-cursor-x/grafana/pull/47 | `chore/structured-logging-backend` | `a46bb31a458` | slog field unwrap, JSON `level` beside go-kit `lvl`, emit-time redaction, migrated production prints in query, HTTP server, dashdiffs, frame table, LDAP settings, and shutdown. |

Epic base at integrate start: `6126b3cc6c75e4e76b972cd07c627cf079aeb537` (same commit as `main` at that moment). The remote epic had no lane commits yet. Each lane was rebased onto that commit.

## How the lanes were combined

Integrate branch: `cursor/structured-logging-integrate-6d5e`.

Merge order: Backend, Security, CI, Frontend, Observability (`e1d36af8937`), then the Observability runbook commit (`61d47968424`).

`pkg/infra/log/log.go` auto-merged. The result keeps both behaviors:

- Security redacts in `newConcreteLogger`, `ConcreteLogger.Log`, and `with`.
- Backend JSON format uses `newJSONLevelLogger`, which keeps `lvl` and adds `level` (`eror` → `error`).

Add/add conflicts in `pkg/infra/log/secretredact`:

- Kept Security's `nonce` key and `TestRedactValue_AuthTokenAndNonceDoNotLeakOpaqueValues`.
- Kept Backend's `sameValue` helper. Comparing `any` maps with `!=` panics; `sameValue` compares those by pointer.

Makefile targets from CI (`lint-structured-logging`) and Security (`logging-secret-inventory`, `logging-security-audit`) both applied. No other path overlapped.

After the migrations, `check_unstructured_logs.py` reported 11 baselined calls that are gone. This integrate dropped those rows from `scripts/structured-logging/baseline.txt` so the same `fmt.Print` / `console.*` lines fail if they return. The check passed after that refresh (`408` baseline rows).

## Observability check against Backend #47

Offline pass, recorded by the Observability lane and folded in here. Promtail (`devenv/docker/blocks/loki-promtail/promtail-config.yaml`) and Alloy (`devenv/docker/blocks/self-instrumentation/config.alloy`) read `level`, then `lvl`, and map a lone `eror` to `error`. With both fields present the indexed label is `error`. Filter on `level`. `| json | lvl="eror"` still matches the raw go-kit field. `logger` is promoted. No pipeline edit was required for Backend head `a46bb31a458`.

Explore and Logs Drilldown were not run. This agent VM cannot pull Loki images (Docker / TLS to the image registries). `make devenv sources=loki-promtail` was not started. PR #43's pipeline files are unchanged; the runbook note is the only follow-up on that branch.

Residual: an image-capable host still has to open `{service_name="grafana", level="error"}` in Explore and service **grafana** in Drilldown → Logs. That is an environment residual.

## Cross-lane scan

`make lint-structured-logging` equivalent:

```text
python3 scripts/structured-logging/check_unstructured_logs.py --self-test  # 11 tests, ok
python3 scripts/structured-logging/check_unstructured_logs.py               # passed after baseline refresh
```

Migrated production files have no leftover `console.*`, `fmt.Print*`, or stdlib `log.Print` on the paths Backend and Frontend changed. `pkg/server/bootstrap/lifecycle.go` still has one pre-existing `fmt.Println` for elevated privileges. It remains in the baseline.

### Findings (triage)

| ID | Severity for this integrate | Finding | Disposition |
| --- | --- | --- | --- |
| F1 | Low | Frontend `redactSecrets.ts` exact keys omit `auth`, `x-api-key`, and `private_key`. Fragments `token` and `credential` cover `token`, `x-auth-token`, `credential`, and `credentials`. Backend `secretredact` redacts all of those exact keys. | Accept for integrate. Faro `beforeSend` and backend emit-time redaction are separate copies and can drift. |
| F2 | Low | `faroLogger.ts` uses a smaller regex (`password`, `secret`, `token`, `authorization`, `cookie`, `api key`, `bearer`). It does not treat `nonce`, `credential`, or `private_key` as sensitive. Nested attribute objects are `JSON.stringify`'d before `beforeSend`, so a nested `nonce` or `private_key` is no longer a key when `redactSecretsDeep` walks the payload. Flat keys still go through `beforeSend`. | Accept for integrate. Transport redaction still runs in `GrafanaJavascriptAgentBackend`. |
| F3 | Info | slog (`pkg/infra/log/slogadapter`) and Faro (`createFaroLogger`) are different APIs on purpose: key/value records into go-kit versus message plus string context. Both end in redaction, at different layers. Grouped slog keys become dotted names (`req.http.method`) and stay on the Loki line. | Compatible with the Observability runbook. |
| F4 | Info | The CI workflow fails new `fmt.Print*` / stdlib `log` under `pkg/` and new `console.*` under `public/app`. It does not flag a structured field such as `raw_json`. `make logging-secret-inventory` is not a required workflow. `eslint` `no-console` is limited to the Frontend migrated files; the Python gate covers the rest of `public/app`. Push events run for `main`, `chore/structured-logging-epic`, and `chore/structured-logging-ci`. Pull requests always run. | Gate matches its README. It will not catch the OAuth residual below. |
| F5 | Info | Tests exist for secret redaction, JSON `level`, the slog bridge, Faro logger, dashboard controls, and Faro `beforeSend`. Jest was not run here. Module `go test` was not run here. | See CI / test status. |

## Residuals (do not block this integrate)

### Security

OAuth connectors still log claim material on keys the redactor does not treat as secret:

- `pkg/login/social/connectors/generic_oauth.go` logs `raw_json` and `data` for user-info JSON.
- `pkg/login/social/connectors/gitlab_oauth.go` logs `raw_json` for the id token (including on decode failure).
- `pkg/login/social/connectors/google_oauth.go` logs `raw_json` when `isDev` is set.
- `pkg/login/social/connectors/okta_oauth.go` logs `raw_json` for the user-info response.

`fmt.Print*` / `console.*` of password, bearer, or token strings were not found on the migrated paths. `console.error(error)` elsewhere can still stringify an error that embeds headers, and those calls do not pass through Faro redaction unless the same error is also sent to Faro. Detail lives in `scripts/logging-security/AUDIT.md`.

### Backend tests

`go test` inside the Grafana module cannot download the Go 1.26.6 toolchain: `proxy.golang.org` returns EOF. First real module test run is CI.

`pkg/infra/log/secretredact` is stdlib-only. Copied out of the module and tested with local Go 1.22.2 (`GOTOOLCHAIN=local`): `ok secretredact 0.003s`. That run does not cover `pkg/infra/log` (go-kit) or the HTTP server tests.

### Observability UI

Needs a host that can pull Loki / Grafana images and run Explore plus Drilldown. Infra residual.

## QA regression

Overall: **FAIL (partial)**.

| | |
| --- | --- |
| Draft PR | https://github.com/njm-cursor-x/grafana/pull/49 (`chore/structured-logging-qa` → epic) |
| Agent | https://cursor.com/agents/bc-b9c30798-220a-5c7d-ae06-634e3d5bd3b0 |
| Evidence | `contribute/backend/nds-10-qa-evidence.md` on that branch (`cc3e43d7c85`) |
| Tree QA executed | Epic `6126b3cc6c75e4e76b972cd07c627cf079aeb537` |

That run is against the pre-integrate epic tip. It is not a result for combined tip `79663a7c92f874b292b06bd3434546835a3c82e0`. CI on the combined tip still needs a recheck.

### Pass

- JSON log parse unit: `msg`, `logger`, `err`, and stable `level` present on a Backend JSON line. Bearer probe redacted.
- Scoped `no-console` ESLint: exit 0 on `public/app/core/logging`, `public/app/index.ts`, `public/app/app.ts`, and `dashboardControls.ts`.

### Product gate still open

Redaction mismatch. Backend head `a46bb31a458` (#47) drops the exact `nonce` key. Security's `TestRedactValue_AuthTokenAndNonceDoNotLeakOpaqueValues` on #45 (`dd9476ec734`) fails against that file: `authToken` and `token` redact via the `token` fragment, and `nonce` leaks (`n0nce-value-998877`). Filed on #47 and #45.

This is the only product gate still open. Backend owns the fix and is pushing a follow-up on `chore/structured-logging-backend`. At the time of this note that branch was still `a46bb31a458`. This audit update does not change product logging code and does not take that follow-up.

The integrate conflict resolution copied Security's `nonce` key into `pkg/infra/log/secretredact/redact.go` on the epic. That is not Backend's follow-up, and QA did not execute it. The gate stays open until the follow-up lands and the combined tip is rechecked.

### Not run (infra, non-blocking)

- CI green: not confirmed. Draft PRs into the epic were pending or skipped.
- e2e smoke: not executed. `grafana-server` cannot be built while `proxy.golang.org` is blocked. The smoke spec on #49 includes a dashboard save; it was not started.
- Drilldown / Explore UI: not run (Docker / TLS; Loki images not pulled).

## CI / test status

| Check | Result |
| --- | --- |
| `check_unstructured_logs.py --self-test` | Pass (11) on the integrate pass |
| `check_unstructured_logs.py` on the integrated tree | Pass after dropping 11 migrated baseline rows |
| `go test` for `./pkg/infra/log/...` | Not run here. Toolchain download EOF |
| `secretredact` tests on Go 1.22.2 outside the module | Pass on the integrate pass |
| Frontend Jest (`faroLogger`, `dashboardControls`, `redactSecrets`) | Not run in the integrate pass. QA reports Jest pass on Frontend `3ea4a758335` |
| Explore / Drilldown | Not run. No Loki images |
| QA regression | FAIL (partial). See the QA section. Combined tip not rechecked |

## Human merge gate

`chore/structured-logging-epic` is the integration branch. A later merge of that epic into `main` is human-only (HITL). This integrate does not merge to `main` and does not open a pull request whose base is `main`.
