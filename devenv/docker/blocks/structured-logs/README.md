# Structured logs (Loki + Alloy)

This block tails Grafana's log files and ships them to Loki so you can inspect JSON lines in **Drilldown → Logs** and **Explore**.

Grafana keeps running on the host. This block does not change application logging.

## Run

From the repository root:

```bash
mkdir -p data/log
make devenv sources=structured-logs
```

`make devenv` binds Loki to `http://localhost:3100`. That is the URL of the provisioned `gdev-loki` data source. Alloy's UI is on `http://localhost:12348`.

Stop the block with `make devenv-down`.

Do not run this block together with `loki`, `loki-promtail`, or `self-instrumentation`. Those blocks also publish port `3100`.

## JSON lines

File logs default to text. Set the file format to JSON in `conf/custom.ini`, then restart Grafana (`make run`):

```ini
[log]
mode = console file

[log.file]
format = json
```

Alloy reads `data/log/*.log` (Grafana's default `[paths] logs = data/log`). After the restart, new lines are JSON objects from `pkg/infra/log` (`t`, `level`, `logger`, `msg`, plus any key-value fields).

Alloy promotes only `level` and `logger` to labels, and it always sets `service_name=grafana` and `job=grafana`. Other fields stay in the line so a `| json` query can read them without a high-cardinality index.

## Inspect

Provision dev data sources once:

```bash
cd devenv && ./setup.sh
```

Restart Grafana, then:

- **Drilldown → Logs** (Logs Drilldown, `grafana-lokiexplore-app`). Pick the Loki data source if asked. The service label is `service_name`. Open **grafana**.
- **Explore**, data source `gdev-loki`:

```logql
{service_name="grafana"}
{service_name="grafana"} | json | level="error"
```

- Dashboard **Grafana structured logs** in the `gdev dashboards` folder (`devenv/dev-dashboards/datasource-loki/grafana_structured_logs.json`). Run `./setup.sh` so provisioning loads it.

Full steps, queries, and troubleshooting: [View structured logs in Loki](../../../docs/sources/developer-resources/view-structured-logs.md).

## Check the pipeline before Grafana emits JSON

Append one JSON line and query it. This does not replace real Grafana logs.

```bash
printf '%s\n' '{"t":"2026-10-02T00:00:00.000000000Z","level":"info","logger":"structured-logs.sample","msg":"json pipeline check"}' >> data/log/grafana.log
```

In Explore, within a minute:

```logql
{service_name="grafana", logger="structured-logs.sample"} | json
```

The details view shows `msg` as its own field. If the query is empty, read Alloy's UI at `http://localhost:12348` and confirm `data/log` is mounted.
