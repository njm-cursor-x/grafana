#!/usr/bin/env python3
"""Guard: every error-level seed sample includes Backend-shaped err."""

from __future__ import annotations

import json
import unittest
from pathlib import Path

from seed import SAMPLES, require_err_on_error

FIXTURE = Path(__file__).resolve().parent.parent / "fixtures" / "sample-structured.jsonl"
ERROR_LEVELS = frozenset({"error", "eror", "crit", "critical"})


class RequireErrOnErrorTests(unittest.TestCase):
    def test_seed_samples_include_err_on_every_error(self) -> None:
        require_err_on_error(SAMPLES)
        error_msgs = [s["msg"] for s in SAMPLES if str(s.get("level", "")).lower() in ERROR_LEVELS]
        self.assertIn("Alert rule evaluation failed", error_msgs)
        for sample in SAMPLES:
            if str(sample.get("level", "")).lower() in ERROR_LEVELS:
                self.assertIsInstance(sample.get("err"), str)
                self.assertTrue(sample["err"].strip())

    def test_fixture_jsonl_error_lines_include_err(self) -> None:
        lines = [json.loads(line) for line in FIXTURE.read_text().splitlines() if line.strip()]
        require_err_on_error(lines)
        alert = next(obj for obj in lines if obj.get("msg") == "Alert rule evaluation failed")
        self.assertEqual(alert["level"], "error")
        self.assertEqual(alert["logger"], "ngalert.eval")
        self.assertTrue(alert["err"])
        self.assertEqual(alert["rule"], "HighErrorRate")

    def test_missing_err_is_rejected(self) -> None:
        with self.assertRaises(ValueError) as ctx:
            require_err_on_error(
                [
                    {
                        "level": "error",
                        "msg": "Alert rule evaluation failed",
                        "logger": "ngalert.eval",
                    }
                ]
            )
        self.assertIn("err", str(ctx.exception))


if __name__ == "__main__":
    unittest.main()
