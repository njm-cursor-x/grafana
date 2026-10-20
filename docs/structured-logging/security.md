---
title: Structured logging security checks
---

# Structured logging security checks

This page describes the credential-leakage tests, the known redaction gaps, and the call-site inventory for the structured logging migration. It's part of Jira NDS-10. For how to write log calls and how backend redaction works, refer to [Backend structured logging](backend.md).

## Leakage tests

These tests fail if a credential reaches encoded backend log output or a Faro payload:

| Test                                                                      | What it checks                                                                                                                                                                                                                                       |
| ------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `pkg/infra/log/redact_security_test.go`                                   | Credentials logged through the production handler chain (`ReadLoggingConfig`, JSON and text formats) are masked. Covers wrapped errors, messages, mixed-case keys, `New(...)` and contextual attributes, `http.Header`, and the App SDK slog logger. |
| `pkg/middleware/loggermw/logger_secrets_test.go`                          | The `Request Completed` line never contains `Authorization`, `Cookie`, `X-Api-Key`, or `auth_token` values from the request, and masks `auth_token` in the referer.                                                                                  |
| `pkg/infra/log/text/text_logger_test.go`                                  | User-controlled values such as dashboard titles can't forge a second record or key, and format verbs are logged literally.                                                                                                                           |
| `public/app/core/logging/logger.security.test.ts`                         | A crafted request error and its nested request context don't send bearer tokens or session cookies to Faro through `createLogger`.                                                                                                                   |
| `packages/grafana-runtime/src/services/logging/registry.security.test.ts` | Whether `getLogger()` from `@grafana/runtime` forwards a bearer token to Faro.                                                                                                                                                                       |

Run the backend tests:

```sh
go test ./pkg/infra/log/... ./pkg/middleware/loggermw/... -count=1
```

Run the frontend tests:

```sh
yarn jest --no-watch public/app/core/logging packages/grafana-runtime/src/services/logging
```

## Known redaction gaps

Known gaps are encoded as tests that pass while the leak reproduces and fail once it's fixed. In Go they're cases with `knownGap` set, which skip with the reason. In Jest they're `it.failing` tests. When you fix a gap, remove the marker so the test guards the fix.

The backend redactor in `pkg/infra/log/redact.go` doesn't mask:

- **Short bearer tokens:** A free-text bearer token shorter than 16 characters or without a digit, such as `Authorization: Bearer test-secret` inside an error.
- **Maps and structs:** Credentials inside a `map[string]string` or a struct field. Only `http.Header` values are inspected.
- **Cookies in free text:** A `grafana_session` value inside an error message.
- **Query tokens in strings:** An `auth_token` query parameter in a URL passed as a string.

The frontend logger in `public/app/core/logging/logger.ts` doesn't mask:

- **Cookies in free text:** A `grafana_session` value that doesn't directly follow `Cookie:`.

An `auth_token` query parameter in a frontend error stack is now masked. The `FetchError` change (#52) made the value pattern match `token` after an underscore, so that test is now a normal passing test.

`getLogger()` in `@grafana/runtime` has no redaction. A bearer token in the message, the context, or an `Error` reaches `faro.api.pushLog` or `faro.api.pushError` unchanged. The Faro `beforeSend` hook only filters bots, so nothing downstream masks it either.

## Call-site inventory

`make logging-security-inventory` prints counts of unstructured log calls and log calls that mention credential keywords, for `pkg/` and `public/app/`. It's report-only and always exits 0. CI gating belongs to the CI lane.

```sh
make logging-security-inventory                         # counts
make logging-security-inventory VERBOSE=1               # every match
make logging-security-inventory OUTPUT_DIR=/tmp/loginv  # one file per section
```

Counts are candidates for review. Many credential-keyword matches are log messages about credentials, not credential values. For example, the single frontend match is the `Failed to change password` message in `LoginCtrl.tsx`, whose `FetchError` the logger now reduces to an allowlisted summary.

## Held call sites

These call sites appear in the inventory but are held for human approval because they sit on a security boundary. Don't modify them as part of this migration:

- `pkg/services/ldap/settings.go:226`
- `pkg/apimachinery/utils/meta.go:907`
