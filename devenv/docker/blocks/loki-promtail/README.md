NB: This will not work properly on MacOS. The loglines of grafana.log are ingested at the start of this devenv and you won't get any more logs are the Docker service is started.

By default this block is setup to scrape logs from Grafana (`../data/log` → `/var/log/grafana`). If you need to log some service from the docker-compse you can add:

```
    # For this to work you need to install the logging driver see https://github.com/grafana/loki/tree/master/cmd/docker-driver#plugin-installation
    logging:
      driver: loki
      options:
        loki-url: "http://loki:3100/loki/api/v1/push"
```

## Structured JSON (Observability lane)

This is the existing **file-scrape** path for when a local Grafana process writes `data/log/grafana.log`.

Repo defaults are text/logfmt. To emit JSON (does not change production configs):

```ini
[log.file]
format = json
```

Promtail then best-effort parses JSON and promotes `level` / `logger` (or go-kit `lvl`) to labels. Non-JSON lines still ingest as raw text.

**Live Drilldown against real Backend JSON waits on the Backend lane.** Until then, use the existing `loki` block sample stream (`make devenv sources=loki`) — do not run this block at the same time (port 3100 clash).

Full Drilldown / Explore steps: [docs/structured-logging-observability.md](../../../../docs/structured-logging-observability.md).
