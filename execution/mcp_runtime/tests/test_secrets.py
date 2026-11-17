"""Levier — MCP runtime secret resolution tests (P11)."""

from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from levier_mcp_runtime.secrets import (
    ChainResolver,
    EnvSecretResolver,
    FileSecretResolver,
    SecretNotFound,
    resolve_binding,
    resolve_string,
)


class ResolveStringTest(unittest.TestCase):
    def test_replaces_all_placeholders(self) -> None:
        resolver = FileSecretResolver({"token": "s3", "other": "x"})
        self.assertEqual(
            resolve_string("a-${secret:token}-${secret:other}", resolver),
            "a-s3-x",
        )

    def test_missing_secret_raises(self) -> None:
        with self.assertRaises(SecretNotFound):
            resolve_string("${secret:missing}", FileSecretResolver({}))

    def test_plain_string_untouched(self) -> None:
        self.assertEqual(resolve_string("plain", FileSecretResolver({})), "plain")

    def test_env_resolver_reads_prefixed_variable(self) -> None:
        import os

        os.environ["LEVIER_SECRET_TEST_TOKEN"] = "env-value"
        try:
            self.assertEqual(EnvSecretResolver().resolve("test-token"), "env-value")
        finally:
            del os.environ["LEVIER_SECRET_TEST_TOKEN"]

    def test_env_resolver_legacy_prefix(self) -> None:
        import os

        os.environ["MIDDLEWARE_SECRET_TEST_TOKEN"] = "legacy-value"
        try:
            self.assertEqual(EnvSecretResolver().resolve("test-token"), "legacy-value")
        finally:
            del os.environ["MIDDLEWARE_SECRET_TEST_TOKEN"]

    def test_chain_falls_back(self) -> None:
        resolver = ChainResolver([FileSecretResolver({}), FileSecretResolver({"x": "1"})])
        self.assertEqual(resolver.resolve("x"), "1")

    def test_file_resolver_load(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "secrets.json"
            path.write_text('{"token":"file-value"}', encoding="utf-8")
            resolver = FileSecretResolver.load(str(path))
            self.assertEqual(resolver.resolve("token"), "file-value")


class ResolveBindingTest(unittest.TestCase):
    def test_resolves_command_args_env_url_headers(self) -> None:
        binding = {
            "server_name": "srv",
            "transport": "stdio",
            "command": "${secret:cmd}",
            "args": ["--token", "${secret:token}"],
            "env": {"TOKEN": "Bearer ${secret:token}"},
            "url": "https://x/${secret:token}",
            "headers": {"Authorization": "Bearer ${secret:token}"},
            "allowed_tools": ["add"],
            "namespace": "mcp__srv__",
        }
        resolved = resolve_binding(binding, FileSecretResolver({"cmd": "python", "token": "t"}))
        self.assertEqual(resolved["command"], "python")
        self.assertEqual(resolved["args"], ["--token", "t"])
        self.assertEqual(resolved["env"]["TOKEN"], "Bearer t")
        self.assertEqual(resolved["url"], "https://x/t")
        self.assertEqual(resolved["headers"]["Authorization"], "Bearer t")
        self.assertEqual(binding["command"], "${secret:cmd}")


if __name__ == "__main__":
    unittest.main()
