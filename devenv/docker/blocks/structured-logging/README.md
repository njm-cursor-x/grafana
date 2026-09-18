# Structured logging (JSON → Loki)

Developer-local path for Grafana **backend JSON logs** into Loki, then
**Drilldown → Logs**, **Explore**, and an optional Logs panel.

Backend lane (`chore/structured-logging-backend`, PR #25) emits
`pkg/infra/log` JSON when `format = json`. This block does not merge that
lane. It **ingests** those lines (file and/or console tee) and documents how
to point Grafana at the Backend branch tip.

## Start (compose)

From the repo root, on this observability branch:

```bash
make devenv sources=structured-logging
```

That starts:

- **Loki** on `http://localhost:3100` (provisioned as `gdev-loki`)
- **Grafana Alloy** tailing `data/log/*.log` and `*.json.log` plus fixture JSONL
- **structured-logging-seed** pushing Backend-shaped JSON so Loki is never empty
- **grafana-json** on `http://localhost:3000` (admin/admin) with
  `GF_LOG_*_FORMAT=json`, writing `data/log/grafana.log` for Alloy to tail

Do not combine this block with `loki`, `loki-promtail`, `self-instrumentation`,
or the devenv `grafana` block (port `3100` / `3000` clashes). Stop with
`make devenv-down`.

### Where logs land

| Path | Loki labels |
| --- | --- |
| Seed HTTP push | `{service_name="grafana", source="fixture", job="grafana-structured"}` |
| Fixture JSONL (`fixtures/sample-structured.jsonl`) | `{service_name="grafana", source="fixture-file"}` |
| Compose `grafana-json` or host `data/log/grafana.log` | `{service_name="grafana", job="grafana", source="grafana-file"}` |
| Console tee `data/log/grafana-console.json.log` | `{service_name="grafana", source="grafana-console"}` |

Stream labels always include `service_name=grafana`, `level`, and `logger`
(Drilldown grouping). The line body is JSON (`t`, `level`, `msg`, `logger`,
optional `err` + fields) so a click shows **parsed fields**, not one opaque string.

Create `data/log` if needed (`mkdir -p data/log`). Alloy bind-mounts repo-root
`data/log` (the `loki-promtail` macOS `/var/log/grafana` caveat does not apply).

## Live Grafana JSON (Backend lane — do not merge)

Repo defaults stay `console`/`text`. Staging/prod (and this demo) set JSON
exactly as Backend documented:

```ini
[log]
mode = console file
level = info

[log.console]
format = json

[log.file]
format = json
```

Checked-in copy: `grafana-json-logging.ini`.

### Option A — compose Grafana (this block)

`grafana-json` already sets `GF_LOG_CONSOLE_FORMAT=json` and
`GF_LOG_FILE_FORMAT=json`. Alloy tails its file. Open
`http://localhost:3000` (anonymous Admin is on for local demo).

That image is stock OSS Grafana, not the Backend-lane binary. Lines are still
real `pkg/infra/log` JSON once you run a Backend-built process (option B).
Compose Grafana is enough to exercise Drilldown/Explore against seeded +
compose-file logs.

### Option B — Backend branch tip as the Grafana process

Do **not** merge `chore/structured-logging-backend` into this lane. Run it
from a worktree so file logs land where Alloy (or `ship_file.py`) can read them:

```bash
# 1) Loki/Alloy from this observability branch
make devenv sources=structured-logging
mkdir -p data/log

# 2) Grafana process from Backend tip
git fetch origin chore/structured-logging-backend
git worktree add /tmp/grafana-backend origin/chore/structured-logging-backend
LOGS="$(pwd)/data/log"
cat devenv/docker/blocks/structured-logging/grafana-json-logging.ini >> /tmp/grafana-backend/conf/custom.ini
printf '\n[paths]\nlogs = %s\n' "$LOGS" >> /tmp/grafana-backend/conf/custom.ini
# optional: also capture console JSON
# (cd /tmp/grafana-backend && make run) 2>&1 | tee -a "$LOGS/grafana-console.json.log"
cd /tmp/grafana-backend && make run
```

Then either wait for Alloy to tail `data/log/grafana.log`, or (no Docker):

```bash
LOKI_URL=http://localhost:3100 python3 devenv/docker/blocks/structured-logging/seed/ship_file.py data/log/grafana.log
```

### Option C — same-repo `make run` on observability + Backend ini only

If you only need JSON **format** (not Backend call-site migrations):

```bash
cat devenv/docker/blocks/structured-logging/grafana-json-logging.ini >> conf/custom.ini
make devenv sources=structured-logging
make run
```

Call-site strings like `Query data failed` / `HTTP server error` require
option B (Backend tip).

## Backend JSON schema (stable)

Aligned with `pkg/infra/log` go-kit JSON and Backend PR #25:

```json
{
  "t": "2026-09-18T15:00:25.000000000Z",
  "level": "error",
  "msg": "Query data failed",
  "logger": "query_data",
  "err": "query backend unavailable"
}
```

| Field | Required | Notes |
| --- | --- | --- |
| `t` | yes | RFC3339Nano (`logTimeFormat`) |
| `level` | yes | `debug` \| `info` \| `warn` \| `error` |
| `msg` | yes | Message string |
| `logger` | yes | `log.New("query_data")` / contextual name |
| `err` | when failing | Field, not interpolated into `msg` |
| extra pairs | optional | `path`, `status`, `orgId`, `header`, … |
| secrets | redacted | Backend replaces tokens / `Authorization` / `password` with `[REDACTED]` |

Fixtures (`fixtures/sample-structured.jsonl`, `seed/seed.py`) use these names,
including a redacted `Authorization` example from Backend’s leakage test.
Every `level=error` line includes `err` (QA `--require-err-on-error`), including
`Alert rule evaluation failed` (`logger=ngalert.eval`).

## Provision host Grafana (make run, not compose grafana-json)

```bash
./devenv/setup.sh
```

- Datasource **gdev-loki** → `http://localhost:3100`
- Dashboard **Structured logging demo**

Default login `admin` / `admin` on `http://localhost:3000`.

## Drilldown → Logs

1. Start the block (`make devenv sources=structured-logging`).
2. Open **Drilldown → Logs** or `http://localhost:3000/a/grafana-lokiexplore-app`.
3. Datasource **gdev-loki**. Service **grafana** (`service_name=grafana`).
4. Confirm **volume** for the last hour.
5. Click a line. Details must show parsed JSON fields (`level`, `msg`,
   `logger`, `err`, …) — not one opaque string.
6. Group / filter by `level` and `logger`.

Live process logs: `{job="grafana"}`. Seeded demo: `{job="grafana-structured"}`.
Both share `service_name=grafana`.

## Explore

1. **Explore** → **gdev-loki** → Code mode.

```logql
{service_name="grafana"}
{service_name="grafana"} | json
{service_name="grafana", level="error"}
{service_name="grafana", logger="query_data"}
{service_name="grafana"} | json | err!=""
{job="grafana"} | json | level="error"
sum by (level) (count_over_time({service_name="grafana"} [5m]))
```

## Verify without the UI

```bash
./devenv/docker/blocks/structured-logging/verify.sh
./devenv/docker/blocks/structured-logging/verify.sh '{service_name="grafana", level="error"}'
./devenv/docker/blocks/structured-logging/verify.sh '{job="grafana"}'
```

Expected: `/ready` → `ready`; labels include `service_name`, `level`, `logger`,
`job`; `query_range` returns JSON objects with `msg` / `logger` / `level`.

```bash
python3 devenv/docker/blocks/structured-logging/seed/seed.py --dry-run
LOKI_URL=http://localhost:3100 python3 devenv/docker/blocks/structured-logging/seed/seed.py
LOKI_URL=http://localhost:3100 python3 devenv/docker/blocks/structured-logging/seed/ship_file.py data/log/grafana.log
```

## Optional Logs panel

Dashboard UID `structured-logging-demo`: `{service_name="grafana"} | json`.
Compose Grafana loads it from this folder; host Grafana after `./devenv/setup.sh`.

## Lane boundaries

- Observability: compose, ingest, runbook, demo panel, verify.
- Backend: `pkg/infra/log` JSON emit, call-site migrations, redaction.
- Do not merge Backend into this lane. Do not treat this block as a CI gate.
