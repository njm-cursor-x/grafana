---
title: Backend structured logging
---

# Backend structured logging

This page describes how Grafana backend code logs with `pkg/infra/log`, how log output is redacted, and the recommended JSON log configuration for staging and production. It's part of Jira NDS-10.

This is a recommendation only. No deployed configuration changes as part of this work.

## Write structured log calls

Use a named `pkg/infra/log` logger with a constant message and key-value fields. Don't use `fmt.Print*` or the standard library `log` package in server code.

```go
var logger = log.New("plugin.loader")

logger.Error("failed to load plugin", "pluginId", id, "err", err)
```

Follow these rules:

- **Constant messages:** Keep the message the same for every occurrence, so you can search and aggregate on it. Put variable data in fields.
- **Errors as fields:** Pass errors with the `err` key, for example `"err", err`. Don't use `error` as the key, and don't interpolate errors into the message.
- **No secrets:** Don't log credentials, even though redaction is in place. Redaction is a safety net, not a license.
- **No zap:** Grafana logs through `pkg/infra/log`. Don't add or migrate to other logging libraries.

## Redaction

Every handler configured by `[log] mode` (console, file, and syslog) wraps its output in a redacting logger in `pkg/infra/log/redact.go`. Redaction runs after level filtering, so it only costs time for lines that are actually written. It covers fields bound with `New(...)` and `WithContextualAttributes` as well as fields passed on the call.

The redactor masks values with `[REDACTED]` in these cases:

- **Sensitive keys:** The key, lowercased with punctuation removed, ends in `password`, `passwd`, `secret`, `token`, `apikey`, `authorization`, `cookie`, `credentials`, or `privatekey`. For example, `password`, `accessToken`, `api_key`, and `X-Api-Key` all match.
- **HTTP headers:** For `http.Header` values, any header whose name matches a sensitive key, such as `Authorization`, `Cookie`, `Set-Cookie`, or `X-Api-Key`.
- **Credentials in text:** In string and `error` values, including the message, it masks `Bearer` tokens, `Basic` credentials that decode to `user:password`, Grafana service account tokens (`glsa_`), Grafana Cloud tokens (`glc_`), legacy API keys, and passwords in URLs.
- **URLs:** `url.URL` and `*url.URL` values that contain a password.

The redactor doesn't look inside structs, maps (other than `http.Header`), or `fmt.Stringer` values. Don't rely on it for those.

## Recommended configuration for staging and production

Use JSON output for staging and production so that log pipelines, such as Grafana Loki with the `json` parser, can index fields without regular expressions. Keep the `console` format for local development.

Add the following to the Grafana configuration file (`custom.ini` or `grafana.ini`):

```ini
[log]
mode = console
level = info

[log.console]
format = json

[log.file]
format = json

[log.syslog]
format = json
```

Each JSON line contains `t`, `level`, `logger`, `msg`, and the call's fields. The following example shows a typical error line:

```json
{"err":"open /var/lib/grafana/plugins/x/plugin.json: no such file or directory","level":"error","logger":"plugin.loader","msg":"failed to load plugin","pluginId":"x","t":"2026-10-20T21:00:00.000000000Z"}
```

In containers, prefer `mode = console` and let the platform collect stdout. Only enable `file` mode if something reads the files, and leave `log_rotate` on.

You can set the same options with environment variables, which is often easier in Kubernetes or Docker:

```sh
GF_LOG_MODE=console
GF_LOG_LEVEL=info
GF_LOG_CONSOLE_FORMAT=json
```

## Related resources

These pages have more detail:

- [Backend logging call-site inventory](backend-inventory.md)
- [Configure Grafana: log](https://grafana.com/docs/grafana/latest/setup-grafana/configure-grafana/#log)
