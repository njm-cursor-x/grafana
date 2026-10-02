# View backend JSON logs in Loki

Use this when you need to confirm Grafana backend JSON logs in **Explore** and **Logs Drilldown**. The dev path is the existing `loki-promtail` block. It does not change production logging defaults.

## Before you begin

- Docker, for `make devenv`.
- Port `3100` free. Do not run `sources=loki` or `sources=self-instrumentation` at the same time as `loki-promtail`.
- Grafana running with developer datasources (`./devenv/setup.sh`, then `make run`).

## Send JSON logs to Loki

1. Turn on JSON for the local server. Copy `devenv/docker/blocks/loki-promtail/grafana-json-logging.ini.example` into `conf/custom.ini`. Leave `conf/defaults.ini` alone.

   File output is `data/log/grafana.log`. That is the file Promtail tails. Console output uses the same JSON shape.

2. Start Loki and Promtail from the repo root:

   ```bash
   make devenv sources=loki-promtail
   ```

   Promtail adds `job=grafana` and `service_name=grafana`. When a line is JSON, it also adds `level` and `logger`. `eror` on go-kit `lvl` is stored as `level=error`. Every other field stays in the line.

3. Restart Grafana (`make run`) so new lines are JSON.

The heavier `self-instrumentation` block (Alloy) scrapes the same directory and applies the same labels. Use that stack only when you also want metrics, traces, and profiles. Its config snippet already sets `[log.file] format = json`.

## Check Loki

```bash
curl -sfS http://localhost:3100/ready
```

```bash
curl -sfSG 'http://localhost:3100/loki/api/v1/query_range' \
  --data-urlencode 'query={service_name="grafana"} | json' \
  --data-urlencode "start=$(($(date +%s) - 3600))000000000" \
  --data-urlencode "end=$(date +%s)000000000" \
  --data-urlencode 'limit=5'
```

Expect one JSON object per line, for example:

```json
{"level":"info","logger":"http.server","msg":"Request Completed","t":"2026-10-02T21:00:00.000000000Z"}
```

| Field | Meaning |
| --- | --- |
| `t` | RFC3339Nano timestamp |
| `level` | `debug`, `info`, `warn`, or `error` |
| `logger` | Name from `log.New` |
| `msg` | Log message |
| `error` | Go error, when the call site passes `"error"` |

If the body is still logfmt or console text, `[log.file] format` is not `json` yet. Labels `job` and `service_name` can still be present.

macOS: this Promtail mount does not follow the file after the container starts. New lines never show up. Use Linux, or the Alloy block in `self-instrumentation`.

## Explore

1. Open `http://localhost:3000` (`admin` / `admin`) and go to **Explore**.
2. Choose datasource **gdev-loki** (`http://localhost:3100`).
3. Run:

```logql
{service_name="grafana"} | json
{service_name="grafana", level="error"} | json
{service_name="grafana", logger="http.server"} | json
{job="grafana"} | json | level="error"
sum by (level) (count_over_time({service_name="grafana"}[5m]))
```

4. Select a row. **Log details** lists `level`, `logger`, `msg`, and `t` as separate fields.

## Drilldown → Logs

Grafana preinstalls Logs Drilldown (`grafana-lokiexplore-app`) under **Drilldown → Logs** (`/a/grafana-lokiexplore-app`).

1. Open **Drilldown → Logs** and select **gdev-loki**.
2. Open the **grafana** service. That row is the `service_name=grafana` stream.
3. Check that the volume panel has data for the selected range.
4. Open a line. Fields match Explore (`level`, `logger`, `msg`, `t`, plus `error` on failures).
5. Filter on `level` and `logger`.

An optional dashboard Logs panel can use `{service_name="grafana"} | json`. This path does not add that panel.

## Related

- [Instrumenting Grafana](/contribute/backend/instrumentation.md)
- [devenv/docker/blocks/loki-promtail/README.md](/devenv/docker/blocks/loki-promtail/README.md)
