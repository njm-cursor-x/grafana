# Loki JSON logs

Ship the structured (JSON) logs of a locally running Grafana server to Loki and browse them in **Explore** and **Drilldown → Logs** (Logs Drilldown).

```
make run ──writes──▶ data/log/grafana.log ──tailed by──▶ Alloy ──push──▶ Loki :3100 ◀── gdev-loki datasource
```

| Service | Image | Port (localhost only) |
| ------- | ----- | --------------------- |
| Loki    | `grafana/loki:3.5.1`   | `3100`  |
| Alloy   | `grafana/alloy:v1.9.1` | `12345` (Alloy UI / pipeline graph) |

This block binds port `3100`, so don't combine it with the `loki`, `loki-promtail`, or `jaeger` blocks.

## Setup

1. Make the file logger emit JSON. Add this to `conf/custom.ini`:

   ```ini
   [log]
   mode = console file

   [log.file]
   format = json
   ```

   Or export `GF_LOG_FILE_FORMAT=json` before starting Grafana.

2. Start Loki and Alloy:

   ```bash
   make devenv sources=loki-json-logs
   ```

3. Start Grafana:

   ```bash
   make run
   ```

4. Make sure Grafana has a Loki datasource pointing at `http://localhost:3100`. The `gdev-loki` datasource from `devenv/datasources.yaml` (installed by `devenv/setup.sh`) already does. Otherwise, add one under **Connections → Data sources → Loki**.

Alloy reads the log file from the beginning, so existing lines are ingested as well. It polls the file, which means it also works on macOS bind mounts.

## Labels and fields

Alloy parses each JSON line and promotes only low-cardinality keys to stream labels. Every other key stays in the line and appears as a detected field.

| Key | Where | Example |
| --- | ----- | ------- |
| `service_name` | label (static) | `grafana` |
| `job` | label (static) | `grafana` |
| `level` | label | `info`, `warn`, `error`, `debug` |
| `logger` | label | `context`, `plugin.loki`, `settings` |
| `t` | log timestamp | `2026-10-20T20:56:37.397060462Z` |
| `msg`, `error`, `orgId`, `uname`, `status`, … | detected fields | |

Errors logged through `pkg/infra/log` are encoded under the `error` key (not `err`). `| json` doesn't extract array values, so `http.Header` fields (logged as `{"Name":["value"]}`) are only visible in the raw line.

## View logs

### Logs Drilldown

1. Open **Drilldown → Logs** (`/a/grafana-lokiexplore-app/explore`) and pick the Loki datasource.
2. The **grafana** service shows up with its log volume. Click **Show logs**.
3. Use the **Labels** tab to break down by `level` or `logger`, and the **Fields** tab to filter by `msg`, `error`, `status`, and other fields. The **Patterns** tab groups similar messages.
4. Expand a log line to see its parsed fields rather than one opaque string.

Logs Drilldown is a preinstalled app that Grafana downloads from grafana.com on startup. If you're offline, install `grafana-lokiexplore-app` manually into `data/plugins`.

### Explore

Open **Explore**, select the Loki datasource, and switch to **Code** mode:

```logql
# Everything from the local server
{service_name="grafana"}

# Errors, with JSON fields extracted
{service_name="grafana", level="error"} | json

# Failed datasource queries
{service_name="grafana", logger=~"plugin.*"} | json | error != ""

# Slow HTTP requests
{service_name="grafana", logger="context"} | json | time_ms > 500

# Error rate by logger
sum by (logger) (count_over_time({service_name="grafana", level="error"}[5m]))
```

Because `level` and `logger` are already labels, `| json` also emits `level_extracted` and `logger_extracted`. Filter on the labels instead.

### Logs panel (optional)

Any of the Explore queries above can back a **Logs** visualization on a dashboard. Use **Add to dashboard** from Explore.

## Verification checklist

Run this checklist after changing logging code or this block.

- [ ] `make devenv sources=loki-json-logs` starts `loki` (healthy) and `alloy`.
- [ ] `head -1 data/log/grafana.log` is a JSON object containing `t`, `level`, `logger`, and `msg`.
- [ ] `curl -s localhost:3100/loki/api/v1/labels` lists `service_name`, `level`, and `logger`.
- [ ] `curl -s localhost:3100/loki/api/v1/label/level/values` lists the levels Grafana has emitted.
- [ ] Alloy UI at `http://localhost:12345` shows the `loki.source.file.grafana` → `loki.process.grafana` → `loki.write.local` pipeline as healthy.
- [ ] **Drilldown → Logs** lists the `grafana` service with volume, and the `level` and `logger` breakdowns are populated.
- [ ] Expanding a line in Drilldown or Explore shows parsed fields (`msg`, `logger`, `error`, …).
- [ ] Provoke a datasource error, for example with the Loki query `{service_name=` in Explore. The UI shows the error as before, and `{service_name="grafana", logger="plugin.loki"} | json | error != ""` returns a line with `msg`, `logger`, and `error`.
- [ ] Send a request with a fake `Authorization: Bearer test-secret` header. `{service_name="grafana"} |= "test-secret"` returns nothing.
- [ ] Log a line with fake credentials, for example `password`, `api_key`, `accessToken`, an `http.Header` with `Authorization`, and a URL with a password. In Loki, `{service_name="grafana"} |= "<fake value>"` returns nothing for each value, and the line shows `[REDACTED]` (URL passwords show as `xxxxx`).

## Staging and production

This block is for local development only: auth is disabled, storage is ephemeral, and ports bind to localhost. For staging and production, use `format = json` for the `console` and/or `file` log modes and ship them with your existing agent (Alloy, Promtail, Fluent Bit) using the same `service_name`, `level`, and `logger` labels.

## Troubleshooting

- **No `grafana` stream in Loki.** Check that `data/log/grafana.log` exists and contains JSON, then look at `docker compose logs alloy` from `devenv/`.
- **Alloy logs `error sending batch ... context deadline exceeded`.** That's expected for a few seconds while Loki's ingester joins the ring. If it doesn't go away, the containers can't reach each other on the Compose network. Check that host firewall rules (for example an `iptables` `FORWARD` policy of `DROP`) aren't blocking Docker bridge traffic.
- **Text lines instead of JSON.** `[log.file] format` is still `text`. Restart Grafana after changing it. Older text lines stay in the file and are ingested unparsed.
