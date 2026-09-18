# Structured logging observability (Loki + Drilldown)

This is the **Observability lane** runbook for the structured-logging epic. It uses the **existing** Grafana devenv Loki path. It does not add a second compose stack and does not change production deploy configs.

Backend draft PR [#31](https://github.com/njm-cursor-x/grafana/pull/31) (`chore/structured-logging-backend`) now emits stable JSON `level` (maps go-kit `eror` → `error`) **alongside** `lvl`. Samples in this lane match that shape. Live file scrape of a Backend-built `grafana-server` is optional once that binary can be compiled.

## Prerequisites

Before you begin, ensure you have the following:

- Docker and `make devenv` (see [devenv/README.md](../devenv/README.md)), **or** a Loki binary on `:3100` plus the seeder (same push API).
- A Grafana instance with **gdev-loki** (`./devenv/setup.sh` then `make run`, or an equivalent OSS Grafana with that datasource). Default login is `admin` / `admin` at `http://localhost:3000`.
- No other process already bound to port `3100`.

## How JSON logs get into Loki

### 1. Sample JSON (works without Backend binary)

`make devenv sources=loki` starts Loki on `http://localhost:3100` and the `loki-data` seeder (`devenv/docker/blocks/loki/data/data.js`). That seeder POSTs JSON to `/loki/api/v1/push`.

Without Docker, run the same seeder against a local Loki:

```bash
node devenv/docker/blocks/loki/data/data.js http://127.0.0.1:3100
```

| Stream | What it is |
| --- | --- |
| `{source="structured-logging-sample", service_name="grafana"}` | SAMPLE Backend-shaped JSON: `t`, `lvl`, `level`, `msg`, `logger`, `err` on errors. |
| `{place="moon", source="data"}` | Existing generic JSON used by Loki datasource tests. |
| `{place="luna", source="data"}` | Existing logfmt test data. |

Checked-in format examples: [devenv/docker/blocks/loki/sample-logs/grafana-backend.sample.jsonl](../devenv/docker/blocks/loki/sample-logs/grafana-backend.sample.jsonl).

### 2. File scrape of Grafana’s log

Existing blocks tail `data/log` when you `make run`:

- **`loki-promtail`** — Promtail scrapes `../data/log` → Loki. Extracts `level` (or go-kit `lvl`) and `logger`.
- **`self-instrumentation`** — Alloy scrapes the same directory. Heavier stack.

Backend-recommended local JSON (do not change `conf/defaults.ini` or production configs):

```ini
[log]
mode = console file
level = info

[log.console]
format = json

[log.file]
format = json
```

Put that in `conf/custom.ini`. On Backend #31, each JSON line includes both `lvl` and `level`.

Do not run `loki-promtail` or `self-instrumentation` together with `sources=loki` (port 3100 clash).

### 3. Provision the Loki datasource

From `devenv/`:

```bash
./setup.sh
```

That symlinks **gdev-loki** → `http://localhost:3100`.

## Start the demo path

Preferred (Docker):

```bash
make devenv sources=loki
cd devenv && ./setup.sh
make run
```

Equivalent without Docker: Loki binary on `:3100`, then `node devenv/docker/blocks/loki/data/data.js http://127.0.0.1:3100`, then Grafana with gdev-loki.

Wait until Loki is ready:

```bash
curl -sfS http://localhost:3100/ready
```

```bash
curl -sfS http://localhost:3100/loki/api/v1/label/source/values
curl -sfSG 'http://localhost:3100/loki/api/v1/query_range' \
  --data-urlencode 'query={source="structured-logging-sample"}' \
  --data-urlencode "start=$(($(date +%s) - 3600))000000000" \
  --data-urlencode "end=$(date +%s)000000000" \
  --data-urlencode 'limit=5'
```

To inspect a Backend worktree without merging it into this lane:

```bash
git fetch origin chore/structured-logging-backend
git worktree add /tmp/grafana-backend origin/chore/structured-logging-backend
# JSON mapping lives in pkg/infra/log/json_level.go (lvl + level).
```

## Drilldown → Logs

1. Open Grafana at `http://localhost:3000`.
2. Go to **Drilldown → Logs**, or open `http://localhost:3000/a/grafana-lokiexplore-app`.
3. Select datasource **gdev-loki**.
4. Pick service **grafana** (`service_name=grafana`) or filter `source=structured-logging-sample`.
5. Confirm a **volume** graph for the last hour.
6. Click a line. Details must show parsed fields (`level`, `lvl`, `msg`, `logger`, and `err` on errors), not one opaque string.
7. Group or filter by `level` and `logger`. Prefer `level="error"` (not `lvl="eror"`).

## Explore

1. Open **Explore**.
2. Select **gdev-loki**.
3. Switch to **Code** mode and run:

```logql
{source="structured-logging-sample"} | json
{service_name="grafana"} | json
{service_name="grafana"} | json | level="error"
{service_name="grafana", level="error"} | json
{service_name="grafana", logger="query_data"} | json
{source="structured-logging-sample"} | json | err!=""
{source="structured-logging-sample"} | json | lvl="eror"
sum by (level) (count_over_time({service_name="grafana"}[5m]))
```

After file scrape of a JSON `grafana.log`:

```logql
{job="grafana"} | json
{job="grafana"} | json | level="error"
{filename="/var/log/grafana/grafana.log"} | json
```

## Optional Logs panel

gdev dashboard **Structured logging sample (Loki)** (`uid=structured-logging-sample`) queries `{source="structured-logging-sample"} | json`.

## Sample JSON shape (matches Backend #31)

```json
{"t":"2026-09-18T15:00:25.000000000Z","lvl":"eror","level":"error","msg":"Query data failed","logger":"query_data","err":"query backend unavailable","sample":true}
```

| Field | Notes |
| --- | --- |
| `t` | RFC3339Nano |
| `lvl` | go-kit key; error is `eror` |
| `level` | Stable Drilldown field; `eror` → `error` |
| `msg` | Message string |
| `logger` | Logger name |
| `err` | Required on `level=error` samples |

## Live verify notes

On a machine with Docker, `make devenv sources=loki` plus Grafana is the product path (Drilldown chrome at `/a/grafana-lokiexplore-app`).

This VM: Docker **daemon** can be started, but **image pull is blocked** (`auth.docker.io` egress). Grafana/Loki release binaries are also blocked (`release-assets.githubusercontent.com`, `dl.grafana.com`). `make run` needs Go 1.26.6 from `proxy.golang.org` (blocked). Backend #31 is in a worktree at `/tmp/grafana-backend` (`pkg/infra/log/json_level.go`) and was **not** merged into this lane.

What was verified here: Backend-shaped samples pushed to a Loki-compatible API on `:3100`. Queries `{service_name="grafana"}` and `{level="error"}` return JSON objects with parsed `lvl` + `level` + `msg` + `logger`, and `err` on every error. That is the data Drilldown/Explore would parse. Product chrome screenshots need a host that can run Grafana + Loki images.

## Lane boundaries

- **This lane:** existing Loki path, sample JSON, Drilldown/Explore docs, optional Logs panel, live verify against Loki.
- **Not this lane:** Go backend log migration, Faro wrapper, CI lint, security/vuln work.
- **Do not merge** Backend into this branch or this PR into `main`.

## Related

- [devenv/docker/blocks/loki/README.md](../devenv/docker/blocks/loki/README.md)
- [devenv/docker/blocks/loki-promtail/README.md](../devenv/docker/blocks/loki-promtail/README.md)
- [devenv/docker/blocks/self-instrumentation/readme.md](../devenv/docker/blocks/self-instrumentation/readme.md)
- Backend PR: https://github.com/njm-cursor-x/grafana/pull/31
