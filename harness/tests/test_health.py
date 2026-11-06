"""Middleware — harness health tests.

Copyright 2026 The Middleware Authors.
"""

import unittest

from middleware_harness.health import HealthState


class HealthTest(unittest.TestCase):
    def test_snapshot(self) -> None:
        health = HealthState(daemon_id="daemon-1", version="0.1.0")
        health.set_check("config", True)
        health.set_ready(True)
        snapshot = health.snapshot()
        self.assertTrue(snapshot["ready"])
        self.assertEqual(snapshot["daemon_id"], "daemon-1")
        self.assertTrue(snapshot["checks"]["config"])


if __name__ == "__main__":
    unittest.main()
