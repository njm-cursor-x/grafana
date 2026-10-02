# NDS-10 logging security audit

Lane branch: `chore/structured-logging-security`  
Base: `chore/structured-logging-epic` at `6126b3cc` (same commit as `main` when this branch was cut).  
This file is the evidence record for the scan run on this branch. Regenerate the broad inventory with `make logging-secret-inventory` (report is gitignored).

## What was scanned

Production logging paths, not tests:

- `pkg/infra/log` emit path (`ConcreteLogger.Log`, `newConcreteLogger`, `WithPrefix` / `WithSuffix`) and `pkg/infra/log/slogadapter` (slog records go through `ConcreteLogger`).
- `pkg/middleware/loggermw` request completion logs.
- `pkg/**/*.go` excluding `*_test.go`: log calls (`.Debug` / `.Info` / `.Warn` / `.Error` / `.Log`) whose next few lines mention password, authorization, bearer, api key, secret, token, cookie, or session. That window scan returned **416** candidate sites. Most are messages that say "token" or "secret" without logging the value. The sites below are the ones that pass a credential, header, cookie, or identity claim as a field.
- `public/app/**/*.{ts,tsx}` `console.*` calls whose arguments mention password, token, secret, authorization, cookie, or apiKey. **No matches.**
- Faro `beforeSend` (`public/app/core/services/echo/backends/grafana-javascript-agent/beforeSendHandler.ts`).

Dependency CVE scans (`govulncheck`, `yarn npm audit`) were not re-run. They do not answer whether log lines leak secrets.

## Findings

Severity is the risk **before** emit-time redaction. "Mitigated" means `RedactFields` / Faro `redactSecretsDeep` now drop the value when the call goes through those sinks.

### High — credential in a log field

These pass the secret as the value of a key that contains `token`, `password`, `authorization`, or `cookie`. Emit-time redaction replaces the whole value with `[REDACTED]`.

| Severity | Site | Context |
| --- | --- | --- |
| High | `pkg/services/grpcserver/interceptors/auth.go:78` | `a.logger.Warn("request with invalid token", "error", err, "token", token)` logs the raw bearer token on auth failure. |
| High | `pkg/services/auth/authimpl/auth_token.go:153` | `"authToken", userAuthToken.AuthToken` plus `clientIP` and `userAgent`. Same pattern at lines 237, 249, 273, and 275. |
| High | `pkg/login/social/connectors/gitlab_oauth.go:296` | `"token", token` logs the `oauth2.Token` (access and refresh tokens). Also lines 302 and 320 (`fmt.Sprintf("%+v", idToken)`). |
| High | `pkg/login/social/connectors/google_oauth.go:265` | Same `oauth2.Token` / id token dump. Also lines 271 and 289. |
| High | `pkg/login/social/connectors/generic_oauth.go:441` | `"token", fmt.Sprintf("%+v", token)`. Also lines 447 and 466. |

`authToken` is not an exact key in the sensitive set. It is covered because the key contains the fragment `token`. The leakage test uses an opaque `glsa_…` value so a substring search for "secret" cannot pass the test by accident.

### High — still leaked after redaction

The field name is not sensitive, and the string does not match `Authorization`, `Bearer`, `password=`, or `id_token=` patterns.

| Severity | Site | Context |
| --- | --- | --- |
| High | `pkg/login/social/connectors/generic_oauth.go:502` | Debug logs `"raw_json", string(rawJSON)` and `"data", data.String()` for id token and access token claims. `UserInfoJson.String` includes email, login, and UPN. Line 496 logs the same JSON at Error when decoding fails. |
| High | `pkg/login/social/connectors/gitlab_oauth.go:325` | `logger.Debug("Received id_token", "raw_json", string(rawJSON))` is not behind the dev flag. Line 328 repeats it on decode failure. |
| Medium | `pkg/login/social/connectors/google_oauth.go:295` | Same `raw_json` dump, but only when `isDev(ctx)` is true. |
| Medium | `pkg/login/social/connectors/gitlab_oauth.go:284` | Dev-only `"data", fmt.Sprintf("%+v", idData)` includes email and the raw API body. |

Do not redact every email in the logger. That would hide legitimate identity fields. These call sites should stop logging the claim JSON.

### Medium

| Severity | Site | Context |
| --- | --- | --- |
| Medium | `pkg/middleware/csp.go:40` | `logger.Debug("Successfully generated CSP nonce", "nonce", nonce)`. A logged nonce weakens CSP for that page. Key `nonce` is now redacted. |
| Medium | `pkg/services/serviceaccounts/secretscan/service.go:126` and `:139` | `"token", leakedToken.Name` is the token **name** (over-redacted by the `token` fragment). `"url", secretscanToken.URL` is not redacted and can point at the place the secret was found. |
| Medium | `pkg/middleware/loggermw/logger.go:105-117` | Request completion logs `remote_addr` (IP), `path`, and a sanitized `referer`. `r.URL.Path` is logged, not `RawQuery`. |
| Medium | `pkg/services/ngalert/image/service.go:150`, `:189`, `:200` | `"token", image.Token` is an image id, not a credential. The `token` fragment redacts it. False positive, kept so credential keys cannot opt out. |

### Low

| Severity | Site | Context |
| --- | --- | --- |
| Low | `pkg/services/frontend/index.go:259` and `:262` | Preview-asset cookie value logged as `folder`. Not a session secret, but it is cookie material. |
| Low | `pkg/middleware/recovery.go:134` | Panic value is logged as `error`. Strings and `error` values are scrubbed. A panicked struct or `*http.Request` is not walked. |

`fmt.Print*` of password / bearer / token in non-test `pkg/**/*.go` had no hits. Frontend `console.*` does not pass those words as arguments. `console.error(error)` can still stringify an HTTP error that embeds headers; those calls do not go through Faro redaction unless the error is also sent to Faro.

## Tests run on this branch

`go test` inside the Grafana module cannot download Go 1.26.6 here (`proxy.golang.org` returns EOF, and `go.dev` TLS fails). `pkg/infra/log/secretredact` is stdlib-only, so the same files were tested with the system Go 1.22 toolchain outside the module:

```text
ok  secretredact  0.002s
```

Faro helper checks (Node 22 type stripping, no Jest install in this environment): bearer, `id_token=`, and keys `token` / `authToken` / `nonce` did not leak. `pkg/infra/log/redact_test.go` still needs the full module graph in CI.

## What tests cover

Stdlib-only package (no module download):

- `pkg/infra/log/secretredact/redact_test.go`
  - `TestBearerTokenDoesNotAppearInEncodedLogOutput` — `Authorization: Bearer test-secret`, session cookie, error string, and `http.Header` must not survive JSON encoding.
  - `TestRedactSecrets_BearerAuthorization`, `TestRedactSecrets_BasicAuth`, `TestRedactSecrets_IDTokenAssignment`
  - `TestRedactValue_AuthTokenAndNonceDoNotLeakOpaqueValues` — `authToken`, a struct under `token`, and `nonce`, using values that do not contain the word "secret".
  - `TestUserControlledStringsAreFieldsNotFormatSinks` — dashboard title and query text stay intact.

Emit path (needs the Grafana module graph; runs in CI):

- `pkg/infra/log/redact_test.go`
  - `TestBearerTokenDoesNotAppearInBackendLogOutput`
  - `TestBearerTokenDoesNotAppearInJSONLogBytes`

Faro:

- `public/app/core/services/echo/backends/grafana-javascript-agent/redactSecrets.test.ts`
  - Bearer, `id_token=`, `grafana_session`, and object keys `token` / `authToken` / `nonce`.
  - `Faro beforeSend reachability` — crafted UI error must not contain the probe after `beforeSendHandler`.

## Residual risks

- OAuth `raw_json` / `data` fields above. Debug level, but debug is a normal setting while diagnosing login.
- IPs (`remote_addr`, `clientIP`) and user agents stay in logs. Redacting them would break incident response.
- `slog` values that are structs on a **non-sensitive** key are not walked. Sensitive keys replace the whole value.
- `console.*` and `fmt.Print*` bypass both redactors. The inventory script still lists those call sites.
- Image ids and token **names** under a key containing `token` become `[REDACTED]`.
- Request paths can still contain a secret if a handler puts one in the path.

## Follow-ups

- Stop logging `raw_json` and `data.String()` in `pkg/login/social/connectors/generic_oauth.go`, `gitlab_oauth.go`, and `google_oauth.go`. Log a stable error id instead.
- Review `secretscan` URLs before they are logged (`pkg/services/serviceaccounts/secretscan/service.go`).
- Keep new log fields off exact keys and off the `token` / `credential` fragments unless the value is safe to drop.
