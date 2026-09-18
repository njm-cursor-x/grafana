#!/usr/bin/env python3
"""Push provisional Grafana-shaped JSON logs to Loki so the stack is queryable
without waiting on the Backend JSON-logging lane."""

from __future__ import annotations

import json
import os
import sys
import time
import urllib.error
import urllib.request
from datetime import datetime, timezone

LOKI_URL = os.environ.get("LOKI_URL", "http://loki:3100").rstrip("/")
READY_URL = f"{LOKI_URL}/ready"
PUSH_URL = f"{LOKI_URL}/loki/api/v1/push"

# Stream labels stay low-cardinality. Parsed JSON fields (level, msg, logger, …)
# are queried with `| json` — the same shape live grafana.log JSON will have.
STREAM = {
    "job": "grafana-structured",
    "source": "fixture",
    "service": "grafana",
}

# Provisional schema aligned with pkg/infra/log go-kit JSON (format = json):
#   t, level, msg, logger, plus arbitrary sibling fields.
# `source` is extra and marked provisional for the Backend lane to match or drop.
SAMPLES = [
    {
        "level": "info",
        "msg": "HTTP Server Listen",
        "logger": "http.server",
        "source": "backend",
        "address": "0.0.0.0:3000",
    },
    {
        "level": "info",
        "msg": "Request completed",
        "logger": "context",
        "source": "backend",
        "method": "GET",
        "path": "/api/dashboards/home",
        "status": 200,
    },
    {
        "level": "debug",
        "msg": "Plugin loaded",
        "logger": "plugins",
        "source": "backend",
        "pluginId": "loki",
    },
    {
        "level": "info",
        "msg": "User login succeeded",
        "logger": "login",
        "source": "backend",
        "userId": 1,
        "orgId": 1,
    },
    {
        "level": "warn",
        "msg": "Slow query",
        "logger": "tsdb.query",
        "source": "backend",
        "datasource": "gdev-loki",
        "duration": "2.1s",
    },
    {
        "level": "error",
        "msg": "Failed to query datasource",
        "logger": "tsdb.loki",
        "source": "backend",
        "err": "context deadline exceeded",
    },
    {
        "level": "info",
        "msg": "Dashboard saved",
        "logger": "dashboard",
        "source": "backend",
        "orgId": 1,
    },
    {
        "level": "error",
        "msg": "Alert rule evaluation failed",
        "logger": "ngalert.eval",
        "source": "backend",
        "rule": "HighErrorRate",
    },
    {
        "level": "info",
        "msg": "Frontend error reported",
        "logger": "frontend-faro",
        "source": "frontend",
        "app": "grafana",
    },
]


def wait_ready(timeout_s: int = 120) -> None:
    deadline = time.time() + timeout_s
    last_err = "not attempted"
    while time.time() < deadline:
        try:
            with urllib.request.urlopen(READY_URL, timeout=2) as resp:
                if 200 <= resp.status < 300:
                    print(f"loki ready at {READY_URL}", flush=True)
                    return
                last_err = f"status {resp.status}"
        except (urllib.error.URLError, TimeoutError, OSError) as err:
            last_err = str(err)
        time.sleep(2)
    raise SystemExit(f"loki not ready after {timeout_s}s: {last_err}")


def rfc3339_nano(ts: float) -> str:
    dt = datetime.fromtimestamp(ts, tz=timezone.utc)
    return dt.strftime("%Y-%m-%dT%H:%M:%S.") + f"{dt.microsecond:06d}000Z"


def push(values: list[list[str]]) -> None:
    payload = json.dumps({"streams": [{"stream": STREAM, "values": values}]}).encode()
    req = urllib.request.Request(
        PUSH_URL,
        data=payload,
        method="POST",
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=10) as resp:
        if resp.status not in (204, 200):
            raise SystemExit(f"loki push failed: HTTP {resp.status}")


def build_values(now: float | None = None) -> list[list[str]]:
    now = time.time() if now is None else now
    values: list[list[str]] = []
    for index, sample in enumerate(SAMPLES):
        ts = now - (len(SAMPLES) - index) * 5
        line = {"t": rfc3339_nano(ts), **sample}
        values.append([str(int(ts * 1_000_000_000)), json.dumps(line, separators=(",", ":"))])
    return values


def main() -> int:
    if "--dry-run" in sys.argv:
        payload = {"streams": [{"stream": STREAM, "values": build_values()}]}
        print(json.dumps(payload, indent=2))
        return 0

    wait_ready()
    values = build_values()
    push(values)
    print(f"pushed {len(values)} fixture lines to {PUSH_URL}", flush=True)
    print(f"stream labels: {STREAM}", flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
