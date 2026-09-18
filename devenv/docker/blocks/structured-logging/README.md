# Structured logging (JSON → Loki)

Developer-local path for Grafana structured JSON logs into Loki, so you can
exercise **Drilldown → Logs**, **Explore**, and a demo Logs panel without
waiting on the Backend JSON-logging lane.

This block is observability-only. It does not change Grafana's default log
format. Live `grafana.log` JSON depends on the Backend lane enabling
`[log.file] format = json`. Until then, fixtures seed Loki so the stack is
queryable immediately.

## Start

From the repo root:

```bash
make devenv sources=structured-logging
```

That starts:

- **Loki** on `http://localhost:3100` (matches the provisioned `gdev-loki` datasource)
- **Grafana Alloy** tailing `data/log/*.log` and this block's fixture JSONL
- **structured-logging-seed** pushing current-timestamp fixture lines to Loki

Do not combine this block with `loki`, `loki-promtail`, or
`self-instrumentation` — they share port `3100` and the `loki` service name.

Stop with `make devenv-down`.

### Where logs land

| Path | When it appears in Loki |
| --- | --- |
| Seeded HTTP push, `{service_name="grafana", source="fixture"}` | Immediately after the seed container succeeds |
| `devenv/docker/blocks/structured-logging/fixtures/*.jsonl` | After Alloy tails the file (`source="fixture-file"`) |
| Repo-root `data/log/grafana.log` (Grafana default `logs = data/log`) | After `make run` with JSON file format enabled |

Seeded streams also carry `level` and `logger` labels so Drilldown can group
without waiting on `| json`. The log line itself is still JSON (`t`, `level`,
`msg`, `logger`, …) so clicking a row shows parsed fields, not one opaque string.

On macOS, Docker Desktop may not create `/var/log/grafana` on the host. This
block bind-mounts repo `data/log` instead, so that Mac caveat from
`loki-promtail` does not apply here. Create `data/log` if Grafana has never
been started (`mkdir -p data/log`).

## Provisional JSON schema

**Provisional — Backend should match or propose a delta.** This is the shape
`pkg/infra/log` already emits when a mode is `format = json` (go-kit JSON
logger, also used by the slog adapter).

```json
{
  "t": "2026-09-18T15:00:00.123456000Z",
  "level": "info",
  "msg": "Request completed",
  "logger": "context",
  "source": "backend"
}
```

| Field | Required | Notes |
| --- | --- | --- |
| `t` | yes | RFC3339Nano timestamp from `pkg/infra/log` (`logTimeFormat`) |
| `level` | yes | `debug` \| `info` \| `warn` \| `error` (go-kit `level.Key()`) |
| `msg` | yes | Message string |
| `logger` | yes | Named logger (`log.New("http.server")`) |
| `source` | provisional | Suggested: `backend` \| `frontend` \| `fixture`. Not emitted by `pkg/infra/log` today — Backend may add it or drop it from this contract |
| extra pairs | optional | Logged as sibling JSON keys (`err`, `path`, `status`, `userId`, …) |

Checked-in examples: `fixtures/sample-structured.jsonl`.
The seed script (`seed/seed.py`) pushes the same shape with **current**
timestamps so Explore's default `now-1h` range finds them.

Until Backend emits JSON, keep Grafana on the default text/console format.
To opt into JSON file logs locally (after Backend lands, or to experiment):

```ini
[log.file]
format = json
```

Add that to `conf/custom.ini` and restart (`make run`). Alloy will then tail
`data/log/grafana.log` into `{job="grafana"}`.

## Provision Grafana datasources and the demo dashboard

```bash
./devenv/setup.sh
```

Restart Grafana if it is already running. This provisions:

- Datasource **gdev-loki** → `http://localhost:3100`
- Dashboard **Structured logging demo** under **gdev dashboards**
  (`devenv/dev-dashboards/structured-logging/structured-logs.json`)

Default login is `admin` / `admin` on `http://localhost:3000`.

## Drilldown → Logs

Live Drilldown against **real** Grafana process logs needs Backend JSON.
The fixture stream is enough to walk the UI today.

1. Start this block and provision datasources (`make devenv sources=structured-logging` then `./devenv/setup.sh`).
2. Run Grafana (`make run`). The first compile is slow; the UI is `http://localhost:3000`.
3. Open **Drilldown → Logs**, or go directly to
   `http://localhost:3000/a/grafana-lokiexplore-app`.
4. Select datasource **gdev-loki**.
5. Choose service **grafana** (`service_name=grafana`). Fixtures and live file
   tail share that label. Volume should appear for the last hour.
6. Click a line. The details view must show parsed JSON fields (`level`,
   `msg`, `logger`, `source`, …) — not one opaque string.
7. Filter / group by `level` and `logger` (stream labels on the fixture path).

If Drilldown shows no series, confirm Loki has data with `./verify.sh`
(below) and that the time picker covers the last hour.

## Explore

1. Open **Explore** (`http://localhost:3000/explore`).
2. Select **gdev-loki**.
3. Switch the query editor to **Code** and run LogQL.

Example queries:

```logql
{service_name="grafana"}
```

```logql
{service_name="grafana"} | json
```

```logql
{service_name="grafana", level="error"}
```

```logql
{service_name="grafana", logger="tsdb.loki"}
```

```logql
{service_name="grafana"} | json | source="backend"
```

```logql
{job=~"grafana.*"} | json | msg=~"(?i)failed"
```

```logql
sum by (level) (count_over_time({service_name="grafana"} [5m]))
```

After Backend JSON is on, the same labels apply to the live file stream:

```logql
{service_name="grafana", job="grafana"} | json | level="error"
```

## Verify without the Grafana UI

From the repo root, after the block is up:

```bash
./devenv/docker/blocks/structured-logging/verify.sh
./devenv/docker/blocks/structured-logging/verify.sh '{service_name="grafana", level="error"}'
```

Expected: Loki `/ready` returns `ready`, `/labels` includes `service_name` /
`level` / `logger` / `job`, and `query_range` returns the seeded JSON lines.

Re-seed (for example after `devenv-down`) by restarting the seed container, or
from the host if Loki is reachable:

```bash
LOKI_URL=http://localhost:3100 python3 devenv/docker/blocks/structured-logging/seed/seed.py
```

Print the push payload without contacting Loki:

```bash
python3 devenv/docker/blocks/structured-logging/seed/seed.py --dry-run
```

## Optional Logs panel

Dashboard UID `structured-logging-demo`, title **Structured logging demo**:

- Logs panel: `{service_name="grafana"} | json`
- Stat: error count over 5m

Open it from **Dashboards → gdev dashboards** after `./devenv/setup.sh`.

## Lane boundaries

- Observability owns this compose path, fixtures, runbook, and the demo panel.
- Backend owns switching Grafana `pkg/infra/log` output to JSON by default.
- Frontend owns Faro / `getLogger` wrapping.
- Do not treat this block as a CI gate.
