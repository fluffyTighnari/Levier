"""Runtime SDK configuration tests."""

from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from levier_runtime.config import ConfigError, load_runtime_config

VALID = """
middleware:
  address: "127.0.0.1:9090"
  auth_token: "token"
runtime:
  name: "echo1"
  instance_id: "echo-local-1"
  type: "normal"
  max_concurrent_runs: 4
  labels:
    pool: "local"
logging:
  level: "debug"
"""


class ConfigTest(unittest.TestCase):
    def write(self, content: str) -> Path:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        path = Path(directory.name) / "runtime.yaml"
        path.write_text(content, encoding="utf-8")
        return path

    def test_load_valid(self) -> None:
        config = load_runtime_config(self.write(VALID))
        self.assertEqual(config.runtime_name, "echo1")
        self.assertEqual(config.instance_id, "echo-local-1")
        self.assertEqual(config.middleware.auth_token, "token")
        self.assertEqual(config.max_concurrent_runs, 4)
        self.assertEqual(config.labels["pool"], "local")
        self.assertEqual(config.log_level, "debug")

    def test_env_overrides(self) -> None:
        config = load_runtime_config(
            self.write(VALID),
            environ={
                "LEVIER_RUNTIME_NAME": "react1",
                "LEVIER_RUNTIME_INSTANCE_ID": "react-local-1",
                "LEVIER_RUNTIME_TOKEN": "env-token",
                "LEVIER_RUNTIME_LABELS": "pool=test,region=cn",
            },
        )
        self.assertEqual(config.runtime_name, "react1")
        self.assertEqual(config.instance_id, "react-local-1")
        self.assertEqual(config.middleware.auth_token, "env-token")
        self.assertEqual(config.labels["region"], "cn")

    def test_requires_instance_id(self) -> None:
        content = VALID.replace('  instance_id: "echo-local-1"\n', "")
        with self.assertRaises(ConfigError):
            load_runtime_config(self.write(content))

    def test_requires_token(self) -> None:
        content = VALID.replace('  auth_token: "token"\n', "")
        with self.assertRaises(ConfigError):
            load_runtime_config(self.write(content))

    def test_rejects_unknown_runtime_type(self) -> None:
        content = VALID.replace('type: "normal"', 'type: "vm"')
        with self.assertRaises(ConfigError):
            load_runtime_config(self.write(content))


if __name__ == "__main__":
    unittest.main()
