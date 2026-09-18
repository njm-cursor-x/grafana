#!/usr/bin/env python3
"""Push Backend-shaped Grafana JSON logs to Loki (pkg/infra/log format=json).

Matches the Backend lane contract: msg, logger, level, key/value fields,
err as a field, secrets already redacted. Used when compose comes up and
when a live Grafana process is not yet writing grafana.log."""

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

# Stream labels stay low-cardinality. Drilldown groups on service_name, then
# level / logger. JSON fields stay on the line so a click shows parsed keys
# (not one opaque string).
STREAM_BASE = {
    "job": "grafana-structured",
    "source": "fixture",
    "service_name": "grafana",
}

ERROR_LEVELS = frozenset({"error", "eror", "crit", "critical"})

# Backend lane (PR #25) + pkg/infra/log go-kit JSON (format = json):
#   t, level, msg, logger, plus sibling fields. err is a field. Secrets redacted.
# QA parse --require-err-on-error: every error-level object must include err.
SAMPLES = [
    {
        "level": "info",
        "msg": "HTTP Server Listen",
        "logger": "http.server",
        "address": "0.0.0.0:3000",
    },
    {
        "level": "info",
        "msg": "Request completed",
        "logger": "context",
        "method": "GET",
        "path": "/api/dashboards/home",
        "status": 200,
    },
    {
        "level": "debug",
        "msg": "QueryMetricsV2: request received",
        "logger": "query_data",
        "time_in_query": False,
    },
    {
        "level": "info",
        "msg": "User login succeeded",
        "logger": "login",
        "userId": 1,
        "orgId": 1,
    },
    {
        "level": "warn",
        "msg": "Slow query",
        "logger": "tsdb.query",
        "datasource": "gdev-loki",
        "duration": "2.1s",
    },
    {
        "level": "error",
        "msg": "Query data failed",
        "logger": "query_data",
        "err": "query backend unavailable",
    },
    {
        "level": "error",
        "msg": "HTTP server error",
        "logger": "http.server",
        "err": "http: accept error: connection reset by peer",
    },
    {
        "level": "error",
        "msg": "request failed",
        "logger": "http",
        "Authorization": "[REDACTED]",
        "password": "[REDACTED]",
        "err": "Authorization: [REDACTED]",
        "orgId": 1,
    },
    {
        "level": "error",
        "msg": "Alert rule evaluation failed",
        "logger": "ngalert.eval",
        "err": "failed to execute query: context deadline exceeded",
        "rule": "HighErrorRate",
    },
    {
        "level": "warn",
        "msg": "skipped duplicate response header",
        "logger": "query",
        "header": "Set-Cookie",
    },
]


def require_err_on_error(samples: list[dict]) -> None:
    """QA --require-err-on-error: every level=error object must include err."""
    for sample in samples:
        level = str(sample.get("level", "")).lower()
        if level not in ERROR_LEVELS:
            continue
        err = sample.get("err")
        if not isinstance(err, str) or not err.strip():
            raise ValueError(
                f"error-level sample {sample.get('msg')!r} must include a non-empty err field"
            )


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


def push(streams: list[dict]) -> None:
    payload = json.dumps({"streams": streams}).encode()
    req = urllib.request.Request(
        PUSH_URL,
        data=payload,
        method="POST",
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=10) as resp:
        if resp.status not in (204, 200):
            raise SystemExit(f"loki push failed: HTTP {resp.status}")


def build_streams(now: float | None = None) -> list[dict]:
    require_err_on_error(SAMPLES)
    now = time.time() if now is None else now
    streams: list[dict] = []
    for index, sample in enumerate(SAMPLES):
        ts = now - (len(SAMPLES) - index) * 5
        line = {"t": rfc3339_nano(ts), **sample}
        labels = {
            **STREAM_BASE,
            "level": sample["level"],
            "logger": sample["logger"],
        }
        streams.append(
            {
                "stream": labels,
                "values": [[str(int(ts * 1_000_000_000)), json.dumps(line, separators=(",", ":"))]],
            }
        )
    return streams


def main() -> int:
    streams = build_streams()
    if "--dry-run" in sys.argv:
        print(json.dumps({"streams": streams}, indent=2))
        return 0

    wait_ready()
    push(streams)
    print(f"pushed {len(streams)} fixture lines to {PUSH_URL}", flush=True)
    print(f"stream labels: {STREAM_BASE} + per-line level, logger", flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
