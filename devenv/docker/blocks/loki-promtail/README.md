# Loki + Promtail (Grafana JSON logs)

This block tails Grafana’s file log into Loki so **Explore** and **Drilldown → Logs** can query it after the backend writes JSON.

It does not work properly on macOS. Docker reads `grafana.log` once at container start and then stops seeing new lines.

## Start

From the repo root, with nothing else bound to port `3100` (do not combine with `sources=loki` or `sources=self-instrumentation`):

```bash
make devenv sources=loki-promtail
```

Promtail mounts `data/log` at `/var/log/grafana` and pushes to Loki on `http://localhost:3100`.

Enable JSON on the Grafana process you want to scrape. Copy [grafana-json-logging.ini.example](grafana-json-logging.ini.example) into `conf/custom.ini`, then:

```bash
cd devenv && ./setup.sh
make run
```

`./devenv/setup.sh` provisions datasource **gdev-loki** at `http://localhost:3100`. Default login is `admin` / `admin`.

Until `[log.file] format = json` is set, lines still arrive as `{job="grafana", service_name="grafana"}` but `| json` has no fields.

## Labels

| Label | Source |
| --- | --- |
| `job` | Static `grafana` |
| `service_name` | Static `grafana` (Logs Drilldown service) |
| `level` | JSON `level`, or go-kit `lvl` with `eror` mapped to `error` |
| `logger` | JSON `logger` |

`msg`, `t`, `error`, and any other keys stay on the log line. Do not promote user-controlled values to labels.

## Verify the pipeline

```bash
curl -sfS http://localhost:3100/ready
```

```bash
curl -sfSG 'http://localhost:3100/loki/api/v1/query_range' \
  --data-urlencode 'query={service_name="grafana"}' \
  --data-urlencode "start=$(($(date +%s) - 3600))000000000" \
  --data-urlencode "end=$(date +%s)000000000" \
  --data-urlencode 'limit=5'
```

A JSON line from `pkg/infra/log` with `format = json` looks like:

```json
{"level":"error","logger":"http.server","msg":"Request Completed","t":"2026-10-02T21:00:00.000000000Z","error":"boom"}
```

`t` is RFC3339Nano. `logger` is the name passed to `log.New`. The error field in current call sites is `error` (see `contribute/backend/instrumentation.md`).

## Explore

1. Open `http://localhost:3000/explore`.
2. Select **gdev-loki**.
3. Run:

```logql
{service_name="grafana"} | json
{job="grafana", service_name="grafana"} | json | level="error"
{service_name="grafana", logger="http.server"} | json
sum by (level) (count_over_time({service_name="grafana"}[5m]))
```

Open a line. Log details should list `msg`, `logger`, `level`, and `t` as fields, not one opaque string.

## Drilldown → Logs

Logs Drilldown is the preinstalled app `grafana-lokiexplore-app`.

1. Open **Drilldown → Logs**, or `http://localhost:3000/a/grafana-lokiexplore-app`.
2. Select **gdev-loki**.
3. Open service **grafana** (`service_name="grafana"`).
4. Confirm a volume graph for the last hour.
5. Open a line and confirm the same parsed fields as Explore.
6. Filter `level=error` and `logger`.

Full steps: [contribute/backend/structured-logging-loki.md](../../../../contribute/backend/structured-logging-loki.md).

## Docker logging driver

To ship another compose service’s stdout (not Grafana’s file log):

```yaml
logging:
  driver: loki
  options:
    loki-url: "http://localhost:3100/loki/api/v1/push"
```

Install the driver first: https://grafana.com/docs/loki/latest/send-data/docker-driver/
