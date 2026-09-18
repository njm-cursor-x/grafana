#!/usr/bin/env python3
"""Push existing Grafana JSON log lines (file or console tee) to Loki.

Used on hosts without Docker/Alloy, and for live verify when a Grafana
process on the Backend lane is writing `[log.file] format = json`.
"""

from __future__ import annotations

import json
import os
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

LOKI_URL = os.environ.get("LOKI_URL", "http://localhost:3100").rstrip("/")
READY_URL = f"{LOKI_URL}/ready"
PUSH_URL = f"{LOKI_URL}/loki/api/v1/push"
DEFAULT_PATH = Path(os.environ.get("GRAFANA_LOG", "data/log/grafana.log"))


def wait_ready(timeout_s: int = 120) -> None:
    deadline = time.time() + timeout_s
    last_err = "not attempted"
    while time.time() < deadline:
        try:
            with urllib.request.urlopen(READY_URL, timeout=2) as resp:
                if 200 <= resp.status < 300:
                    return
                last_err = f"status {resp.status}"
        except (urllib.error.URLError, TimeoutError, OSError) as err:
            last_err = str(err)
        time.sleep(2)
    raise SystemExit(f"loki not ready after {timeout_s}s: {last_err}")


def parse_line(raw: str) -> tuple[dict, dict] | None:
    raw = raw.strip()
    if not raw:
        return None
    try:
        obj = json.loads(raw)
    except json.JSONDecodeError:
        obj = {"msg": raw, "level": "info", "logger": "unknown"}
    if not isinstance(obj, dict):
        obj = {"msg": raw, "level": "info", "logger": "unknown"}
    labels = {
        "job": "grafana",
        "source": "grafana-file",
        "service_name": "grafana",
        "level": str(obj.get("level") or "info"),
        "logger": str(obj.get("logger") or "unknown"),
    }
    return labels, obj


def push(streams: list[dict]) -> None:
    req = urllib.request.Request(
        PUSH_URL,
        data=json.dumps({"streams": streams}).encode(),
        method="POST",
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=15) as resp:
        if resp.status not in (204, 200):
            raise SystemExit(f"loki push failed: HTTP {resp.status}")


def main() -> int:
    path = Path(sys.argv[1]) if len(sys.argv) > 1 else DEFAULT_PATH
    if not path.is_file():
        raise SystemExit(f"log file not found: {path}")
    wait_ready()
    now = time.time()
    streams: list[dict] = []
    for index, raw in enumerate(path.read_text().splitlines()):
        parsed = parse_line(raw)
        if parsed is None:
            continue
        labels, obj = parsed
        # Keep timestamps inside Loki/Explore default now-1h (and behind "now"
        # so a query_range end=now includes the line).
        ts = now - 5 - index * 0.2
        streams.append(
            {
                "stream": labels,
                "values": [[str(int(ts * 1_000_000_000)), json.dumps(obj, separators=(",", ":"))]],
            }
        )
    if not streams:
        raise SystemExit(f"no log lines in {path}")
    # Loki accepts large batches; keep one request.
    push(streams)
    print(f"shipped {len(streams)} lines from {path} to {PUSH_URL}", flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
