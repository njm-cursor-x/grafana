---
keywords:
  - grafana
  - documentation
  - developers
  - loki
  - logs
  - structured logging
  - alloy
labels:
  products:
    - oss
    - enterprise
description: Ship Grafana JSON file logs to Loki and inspect them in Logs Drilldown and Explore.
title: View structured logs in Loki
menuTitle: View structured logs
weight: 300
canonical: https://grafana.com/docs/grafana/latest/developer-resources/view-structured-logs/
---

# View structured logs in Loki

Ship Grafana's JSON file logs to a local Grafana Loki instance and inspect them in **Drilldown → Logs** and **Explore**.

The path uses the `structured-logs` development block: Grafana Alloy tails `data/log` and pushes lines to Loki. You keep running Grafana on the host with `make run`. This page doesn't change how the application writes logs.

## Before you begin

You need the following:

- **Docker:** Docker Compose V2 is installed. `make devenv` uses it.
- **Repository checkout:** You can run Grafana from this repository.
- **Free port 3100:** No other local Loki is bound to `3100`. The `loki`, `loki-promtail`, and `self-instrumentation` blocks also use that port.

## Start Loki and Alloy

Create the log directory before Compose starts. If the directory is missing, Docker creates it as root and the Grafana process on the host can't write `grafana.log`.

```sh
mkdir -p data/log
make devenv sources=structured-logs
```

The command starts two containers:

- **Loki:** `grafana/loki:3.5.3` on `http://localhost:3100`.
- **Alloy:** `grafana/alloy:v1.10.0`. It reads `/var/log/grafana/*.log` inside the container, which is the host directory `data/log`. Its UI is `http://localhost:12348`.

Stop the containers with `make devenv-down`.

## Write JSON log lines

Grafana file logs default to text (`[log.file] format = text`). Text lines still reach Loki, but they don't carry `level` or `logger` labels, and Logs Drilldown can't split them into fields.

Set the file handler to JSON in `conf/custom.ini`:

```ini
[log]
mode = console file

[log.file]
format = json
```

Restart Grafana so it reloads logging. New lines in `data/log/grafana.log` are JSON objects from `pkg/infra/log`. A line includes `t` (RFC3339Nano), `level`, `logger`, `msg`, and any extra key-value pairs passed at the call site.

`conf/custom.ini` is local configuration. Don't commit it.

{{< admonition type="note" >}}
Console output can stay on `format = console` for the terminal. Alloy reads the log file, so the file handler is the one that must use `json`.
{{< /admonition >}}

## Provision the Loki data source

`make devenv` doesn't provision data sources. From `devenv`, link the dev data sources and dashboards:

```sh
cd devenv
./setup.sh
```

Restart Grafana. **Connections > Data sources** includes `gdev-loki` with URL `http://localhost:3100` and UID `gdev-loki`.

The same setup loads the **Grafana structured logs** dashboard from `devenv/dev-dashboards/datasource-loki/grafana_structured_logs.json` into the `gdev dashboards` folder.

## Inspect logs in Drilldown

Open **Drilldown → Logs**. That screen is Logs Drilldown (`grafana-lokiexplore-app`).

Select the Loki data source when the app asks for one. The service label for this pipeline is `service_name`. Open the **grafana** service.

The service list shows volume for **grafana**. Select a line to open its details. A JSON line shows parsed fields such as `msg`, `logger`, and `level` instead of one opaque string. Turn on **Prettify JSON** in a logs visualization when you want the expanded object.

`level` and `logger` are also stream labels. Drilldown uses them as filters. `service_name` is what groups the service list.

## Query logs in Explore

Open **Explore** and select `gdev-loki`.

List recent Grafana lines:

```logql
{service_name="grafana"}
```

Parse the JSON body and keep error lines. `level` is already a label; `| json` exposes the remaining fields:

```logql
{service_name="grafana"} | json | level="error"
```

Filter one logger. The label value is the name passed to `log.New`, for example `http.server`:

```logql
{service_name="grafana", logger="http.server"}
```

The **Grafana structured logs** dashboard runs `{service_name="grafana"} | json` in a logs panel with details and prettified JSON enabled.

## Labels the pipeline sets

Alloy keeps the label set small on purpose. High-cardinality values stay in the JSON body.

| Label | Source | Use |
| --- | --- | --- |
| `service_name` | Always `grafana` | Logs Drilldown service grouping |
| `job` | Always `grafana` | Stream selector |
| `filename` | Log file path inside the container | Distinguish rotated files |
| `level` | JSON field `level`, when the line is JSON | `debug`, `info`, `warn`, `error` |
| `logger` | JSON field `logger`, when the line is JSON | Component name from `log.New` |

The timestamp `t` becomes the Loki timestamp when it parses as RFC3339Nano. A line that isn't JSON is still stored. It keeps `service_name` and `job`, and it doesn't get `level` or `logger`.

## Troubleshoot

- **Drilldown has no grafana service.** Confirm Loki is up at `http://localhost:3100/ready` and that Alloy's UI shows the `grafana` file target. Generate a line by using Grafana, or append a sample JSON line as described in the block README at `devenv/docker/blocks/structured-logs/README.md`.
- **Lines appear, but details are one string.** The file handler is still on `text`. Set `[log.file] format = json` and restart Grafana. Only lines written after the restart are JSON.
- **Port 3100 is already allocated.** Stop the other Loki block (`make devenv-down`) and start `structured-logs` on its own.
- **`data/log` stays empty.** The host directory must exist before `make devenv`, and Grafana must run from the repository root so `[paths] logs = data/log` matches the mount.
- **`self-instrumentation` is not this path.** That block also tails `data/log`, but its Alloy config doesn't promote JSON `level` and `logger`. Use `structured-logs` when you want Logs Drilldown labels.

## Next steps

- Refer to [Configure Grafana](../setup-grafana/configure-grafana/_index.md) for the `[log]` and `[log.file]` options.
- Refer to the [Loki data source](../datasources/loki/_index.md) for LogQL in Explore.
- The Compose file, Alloy config, and a one-line pipeline check are in `devenv/docker/blocks/structured-logs/`.
- Trace collection stays on the `self-instrumentation` block, described in `contribute/backend/instrumentation.md`. That block doesn't promote JSON `level` and `logger` labels.
