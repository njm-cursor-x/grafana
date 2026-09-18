# Loki (gdev)

Existing devenv Loki block. Start from the repo root:

```bash
make devenv sources=loki
```

That starts:

- **Loki** on `http://localhost:3100` (provisioned as **gdev-loki** after `./devenv/setup.sh`)
- **loki-data** — Node seeder that POSTs JSON and logfmt lines to `/loki/api/v1/push`

This is the Observability-lane path for structured JSON → Loki. Do not add a second Loki stack. Do not combine with `loki-promtail` or `self-instrumentation` (port 3100 clash).

## Streams

| Labels | Format | Notes |
| --- | --- | --- |
| `{source="structured-logging-sample", service_name="grafana"}` | JSON | SAMPLE Grafana-shaped lines (`msg`, `logger`, `level`, `err`). For Drilldown/Explore before Backend JSON exists. |
| `{place="moon", source="data"}` | JSON | Existing Loki datasource test data. |
| `{place="luna", source="data"}` | logfmt | Existing Loki datasource test data. |

Checked-in format examples (not tailed by this block): [sample-logs/grafana-backend.sample.jsonl](sample-logs/grafana-backend.sample.jsonl).

## Verify

```bash
curl -sfS http://localhost:3100/ready
```

Then follow [docs/structured-logging-observability.md](../../../../docs/structured-logging-observability.md) for Drilldown → Logs and Explore.
