# Structured logging observability (Loki + Drilldown)

This is the **Observability lane** runbook for the structured-logging epic. It uses the **existing** Grafana devenv Loki path. It does not add a second compose stack and does not change production deploy configs.

Live Drilldown against a real `grafana-server` JSON file waits on the **Backend** lane. Until that lands, use the sample JSON this path already seeds.

## Prerequisites

Before you begin, ensure you have the following:

- Docker and the repo `make devenv` flow (see [devenv/README.md](../devenv/README.md)).
- A Grafana instance with the gdev datasources (run `./devenv/setup.sh` from `devenv/`, then `make run`). Default login is `admin` / `admin` at `http://localhost:3000`.
- No other devenv block already bound to port `3100` (`loki`, `loki-promtail`, `self-instrumentation`).

## How JSON logs get into Loki

Pick the existing in-repo path that matches what you want to see.

### 1. Sample JSON (use this now)

`make devenv sources=loki` starts Loki on `http://localhost:3100` and the `loki-data` seeder (`devenv/docker/blocks/loki/data/data.js`). That seeder already POSTs JSON to Loki’s push API (`/loki/api/v1/push`).

This lane adds **Grafana-shaped SAMPLE** lines on top of the existing fake `place=moon` JSON / `place=luna` logfmt streams:

| Stream | What it is |
| --- | --- |
| `{source="structured-logging-sample", service_name="grafana"}` | SAMPLE backend-shaped JSON (`msg`, `logger`, `level`, `err` on errors). Labeled `sample: true`. |
| `{place="moon", source="data"}` | Existing generic JSON used by Loki datasource tests. |
| `{place="luna", source="data"}` | Existing logfmt test data. |

The sample file [devenv/docker/blocks/loki/sample-logs/grafana-backend.sample.jsonl](../devenv/docker/blocks/loki/sample-logs/grafana-backend.sample.jsonl) is the same shape, checked in so you can read it without starting Docker.

### 2. File scrape of Grafana’s log (after Backend JSON)

Two existing blocks tail `data/log` (Grafana’s file log when you `make run`):

- **`loki-promtail`** — Promtail scrapes `../data/log` → Loki. Simplest file path.
- **`self-instrumentation`** — Grafana Alloy scrapes the same directory, and already documents `[log.file] format = json`. Heavier (Tempo + Prometheus + Pyroscope too).

Repo defaults stay `console` / `text`. To emit JSON from a local Grafana process (does not require the Backend lane for *format*, only for call-site migrations):

```ini
[log.file]
format = json
```

Put that in `conf/custom.ini`. Do not change `conf/defaults.ini` or any production config in this lane.

Until Backend writes production-path JSON, `grafana.log` is still mostly text/logfmt. Promtail/Alloy will ingest those lines as raw text. Parsed JSON fields in Drilldown need either the sample stream above or Backend JSON.

### 3. Provision the Loki datasource

`make devenv` only starts containers. From `devenv/`:

```bash
./setup.sh
```

That symlinks `devenv/datasources.yaml`, including **gdev-loki** → `http://localhost:3100`.

## Start the demo path

From the repo root:

```bash
make devenv sources=loki
cd devenv && ./setup.sh
make run
```

Wait until Loki is ready:

```bash
curl -sfS http://localhost:3100/ready
```

Optional: confirm SAMPLE labels exist (after `loki-data` has pushed):

```bash
curl -sfS http://localhost:3100/loki/api/v1/label/source/values
curl -sfSG 'http://localhost:3100/loki/api/v1/query_range' \
  --data-urlencode 'query={source="structured-logging-sample"}' \
  --data-urlencode "start=$(($(date +%s) - 3600))000000000" \
  --data-urlencode "end=$(date +%s)000000000" \
  --data-urlencode 'limit=5'
```

## Drilldown → Logs

1. Open Grafana at `http://localhost:3000`.
2. Go to **Drilldown → Logs**, or open `http://localhost:3000/a/grafana-lokiexplore-app`.
3. Select datasource **gdev-loki**.
4. Pick service **grafana** (`service_name=grafana`) or filter `source=structured-logging-sample`.
5. Confirm a **volume** graph for the last hour (sample lines are timestamped “now”).
6. Click a line. Details should show parsed fields (`level`, `msg`, `logger`, and `err` on errors), not one opaque string.
7. Group or filter by `level` and `logger`.

Live process logs, once Backend JSON exists and you use `loki-promtail` or `self-instrumentation` with `[log.file] format = json`, show up as `{job="grafana"}` / `{filename="/var/log/grafana/grafana.log"}`.

## Explore

1. Open **Explore**.
2. Select **gdev-loki**.
3. Switch to **Code** mode and run:

```logql
{source="structured-logging-sample"}
{source="structured-logging-sample"} | json
{service_name="grafana"} | json
{service_name="grafana", level="error"} | json
{service_name="grafana", logger="query_data"} | json
{source="structured-logging-sample"} | json | err!=""
sum by (level) (count_over_time({service_name="grafana"}[5m]))
```

Existing generic JSON (not Grafana-shaped):

```logql
{place="moon"} | json
{place="moon"} | json | level="error"
```

After Backend JSON + file scrape:

```logql
{job="grafana"} | json
{filename="/var/log/grafana/grafana.log"} | json
{filename="/var/log/grafana/grafana.log"} | logfmt
```

(`logfmt` is what stock file logs look like today. Switch to `| json` when `[log.file] format = json` is on.)

## Optional Logs panel

After `./devenv/setup.sh`, open the gdev dashboard **Structured logging sample (Loki)** (`uid=structured-logging-sample`) in folder **gdev dashboards**. It queries `{source="structured-logging-sample"} | json`.

## Sample JSON shape

These lines are **samples**. They match the Backend lane contract (`msg`, `logger`, `level`, `err` on failures). Stock `pkg/infra/log` `format = json` today also emits go-kit `lvl` (error is `eror`) and `t`; Backend may stabilize on `level`.

```json
{"t":"2026-09-18T15:00:25.000000000Z","level":"error","msg":"Query data failed","logger":"query_data","err":"query backend unavailable","sample":true}
```

Every `level=error` sample includes `err`. Secrets are not present in fixtures (use `[REDACTED]` if you add any).

## Lane boundaries

- **This lane:** existing Loki compose, sample JSON, Drilldown/Explore docs, optional Logs panel.
- **Not this lane:** Go backend log migration, Faro wrapper, CI lint, security/vuln work.
- **Blocked:** live Drilldown against real Backend JSON until that lane emits it.

## Related

- [devenv/docker/blocks/loki/README.md](../devenv/docker/blocks/loki/README.md)
- [devenv/docker/blocks/loki-promtail/README.md](../devenv/docker/blocks/loki-promtail/README.md)
- [devenv/docker/blocks/self-instrumentation/readme.md](../devenv/docker/blocks/self-instrumentation/readme.md)
- [contribute/backend/instrumentation.md](../contribute/backend/instrumentation.md)
