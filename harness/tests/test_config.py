"""Middleware — harness configuration tests.

Copyright 2026 The Middleware Authors.
"""

import tempfile
import unittest
from pathlib import Path

from middleware_harness.config import ConfigError, load_settings

VALID_YAML = """
middleware:
  address: "127.0.0.1:9090"
daemon:
  id: "daemon-1"
  auth_token: "secret-token"
  max_concurrent_runs: 4
logging:
  level: "info"
  format: "json"
"""


class ConfigTest(unittest.TestCase):
    def write_config(self, content: str) -> Path:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        path = Path(directory.name) / "config.yaml"
        path.write_text(content, encoding="utf-8")
        return path

    def test_load_valid(self) -> None:
        settings = load_settings(self.write_config(VALID_YAML))
        self.assertEqual(settings.daemon.id, "daemon-1")
        self.assertEqual(settings.daemon.auth_token.get_secret_value(), "secret-token")
        self.assertEqual(settings.daemon.max_concurrent_runs, 4)
        self.assertNotIn("secret-token", str(settings.summary()))

    def test_env_overrides(self) -> None:
        settings = load_settings(
            self.write_config(VALID_YAML),
            environ={
                "HARNESS_MIDDLEWARE_ADDRESS": "mw:9443",
                "HARNESS_DAEMON_ID": "daemon-env",
                "HARNESS_DAEMON_MAX_CONCURRENT_RUNS": "8",
                "HARNESS_DAEMON_STRATEGIES": "react,echo",
                "HARNESS_DAEMON_LABELS": "pool=test,region=cn1",
                "HARNESS_LOGGING_LEVEL": "debug",
            },
        )
        self.assertEqual(settings.middleware.address, "mw:9443")
        self.assertEqual(settings.daemon.id, "daemon-env")
        self.assertEqual(settings.daemon.max_concurrent_runs, 8)
        self.assertEqual(settings.daemon.strategies, ["react", "echo"])
        self.assertEqual(settings.daemon.labels["region"], "cn1")
        self.assertEqual(settings.logging.level, "debug")

    def test_invalid_capacity(self) -> None:
        with self.assertRaises(ConfigError):
            load_settings(
                self.write_config(VALID_YAML.replace("max_concurrent_runs: 4", "max_concurrent_runs: 0"))
            )

    def test_invalid_yaml(self) -> None:
        with self.assertRaises(ConfigError):
            load_settings(self.write_config("daemon: ["))

    def test_invalid_env_bool(self) -> None:
        with self.assertRaises(ConfigError):
            load_settings(
                self.write_config(VALID_YAML),
                environ={"HARNESS_MIDDLEWARE_TLS": "sometimes"},
            )


if __name__ == "__main__":
    unittest.main()
