"""Levier - React Runtime configuration tests.

Copyright 2026 The Levier Authors.
"""

import os
import tempfile
import unittest
from pathlib import Path

from levier_react_runtime.config import ConfigError, load_settings

VALID_YAML = """
middleware:
  address: "127.0.0.1:9090"
daemon:
  id: "daemon-1"
  runtime_name: "react1"
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
        self.assertEqual(settings.daemon.runtime_name, "react1")
        self.assertEqual(settings.daemon.max_concurrent_runs, 4)
        self.assertNotIn("secret-token", str(settings.summary()))

    def test_env_overrides(self) -> None:
        settings = load_settings(
            self.write_config(VALID_YAML),
            environ={
                "HARNESS_LEVIER_ADDRESS": "mw:9443",
                "HARNESS_DAEMON_ID": "daemon-env",
                "HARNESS_RUNTIME_NAME": "react2",
                "HARNESS_DAEMON_MAX_CONCURRENT_RUNS": "8",
                "HARNESS_DAEMON_LABELS": "pool=test,region=cn1",
                "HARNESS_LOGGING_LEVEL": "debug",
            },
        )
        self.assertEqual(settings.middleware.address, "mw:9443")
        self.assertEqual(settings.daemon.id, "daemon-env")
        self.assertEqual(settings.daemon.max_concurrent_runs, 8)
        self.assertEqual(settings.daemon.runtime_name, "react2")
        self.assertEqual(settings.daemon.labels["region"], "cn1")
        self.assertEqual(settings.logging.level, "debug")

    def test_new_env_names(self) -> None:
        settings = load_settings(
            self.write_config(VALID_YAML),
            environ={
                "LEVIER_REACT_RUNTIME_ADDRESS": "levier:9443",
                "LEVIER_REACT_RUNTIME_INSTANCE_ID": "daemon-new",
                "LEVIER_REACT_RUNTIME_NAME": "react2",
                "LEVIER_REACT_RUNTIME_MAX_CONCURRENT_RUNS": "6",
                "LEVIER_REACT_RUNTIME_LOG_LEVEL": "debug",
            },
        )
        self.assertEqual(settings.middleware.address, "levier:9443")
        self.assertEqual(settings.daemon.id, "daemon-new")
        self.assertEqual(settings.daemon.max_concurrent_runs, 6)
        self.assertEqual(settings.daemon.runtime_name, "react2")
        self.assertEqual(settings.logging.level, "debug")

    def test_auth_token_file(self) -> None:
        token_path = Path(self.write_config("token-file-placeholder")).with_name("daemon-token")
        token_path.write_text("token-from-file", encoding="utf-8")
        if os.name != "nt":
            token_path.chmod(0o600)
        config = VALID_YAML.replace('auth_token: "secret-token"', f'auth_token_file: "{token_path.as_posix()}"')
        settings = load_settings(self.write_config(config), environ={})
        self.assertEqual(settings.daemon.auth_token.get_secret_value(), "token-from-file")
        self.assertEqual(settings.daemon.auth_token_file, token_path.as_posix())

    def test_new_env_token_file(self) -> None:
        token_path = Path(self.write_config("token-file-placeholder")).with_name("daemon-token-env")
        token_path.write_text("token-from-env-file", encoding="utf-8")
        if os.name != "nt":
            token_path.chmod(0o600)
        config = VALID_YAML.replace('auth_token: "secret-token"', 'auth_token: ""')
        settings = load_settings(
            self.write_config(config),
            environ={"LEVIER_HARNESS_DAEMON_AUTH_TOKEN_FILE": str(token_path)},
        )
        self.assertEqual(settings.daemon.auth_token.get_secret_value(), "token-from-env-file")

    def test_legacy_env_names(self) -> None:
        settings = load_settings(
            self.write_config(VALID_YAML),
            environ={
                "HARNESS_MIDDLEWARE_ADDRESS": "legacy:9443",
                "HARNESS_MIDDLEWARE_TLS": "true",
            },
        )
        self.assertEqual(settings.middleware.address, "legacy:9443")
        self.assertTrue(settings.middleware.tls)

    def test_mcp_runtime_socket(self) -> None:
        settings = load_settings(self.write_config(
            VALID_YAML + "mcp_runtime:\n  socket: \"/tmp/test-mcp.sock\"\n"
        ))
        self.assertEqual(settings.mcp_runtime.socket, "/tmp/test-mcp.sock")
        self.assertEqual(settings.summary()["mcp_runtime_socket"], "/tmp/test-mcp.sock")

        overridden = load_settings(
            self.write_config(VALID_YAML),
            environ={"HARNESS_MCP_RUNTIME_SOCKET": "/run/mcp.sock"},
        )
        self.assertEqual(overridden.mcp_runtime.socket, "/run/mcp.sock")

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
                environ={"HARNESS_LEVIER_TLS": "sometimes"},
            )


if __name__ == "__main__":
    unittest.main()
