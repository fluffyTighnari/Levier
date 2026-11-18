"""Levier — Counter App configuration tests (P2.1).

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import os
import tempfile
import unittest
from pathlib import Path

from config import ConfigError, Secret, from_env


class ConfigTest(unittest.TestCase):
    def test_p2_1_defaults(self) -> None:
        config = from_env({"LEVIER_ADMIN_TOKEN": "admin-secret"})
        self.assertEqual(config.mw_url, "http://127.0.0.1:8080")
        self.assertEqual(config.app_name, "retrofit-counter")
        self.assertEqual(config.admin_token.reveal(), "admin-secret")

    def test_p2_1_overrides_and_trailing_slash(self) -> None:
        config = from_env(
            {
                "LEVIER_URL": "https://mw.example.test/",
                "LEVIER_ADMIN_TOKEN": "admin-secret",
                "LEVIER_APP_NAME": "counter-app",
            }
        )
        self.assertEqual(config.mw_url, "https://mw.example.test")
        self.assertEqual(config.app_name, "counter-app")

    def test_p2_1_requires_admin_token(self) -> None:
        with self.assertRaises(ConfigError):
            from_env({})
        with self.assertRaises(ConfigError):
            from_env({"LEVIER_ADMIN_TOKEN": "   "})

    def test_p2_1_rejects_bad_url(self) -> None:
        for value in ("localhost:8080", "ftp://example.test", "http://"):
            with self.subTest(value=value):
                with self.assertRaises(ConfigError):
                    from_env({"LEVIER_URL": value, "LEVIER_ADMIN_TOKEN": "admin-secret"})

    def test_legacy_env_names_still_work(self) -> None:
        config = from_env(
            {
                "MW_URL": "https://legacy.example.test/",
                "MW_ADMIN_TOKEN": "legacy-secret",
                "MW_APP_NAME": "legacy-app",
            }
        )
        self.assertEqual(config.mw_url, "https://legacy.example.test")
        self.assertEqual(config.app_name, "legacy-app")
        self.assertEqual(config.admin_token.reveal(), "legacy-secret")

    def test_app_prefixed_env_names(self) -> None:
        config = from_env(
            {
                "LEVIER_APP_URL": "https://app.example.test/",
                "LEVIER_APP_ADMIN_TOKEN": "app-admin-secret",
                "LEVIER_APP_NAME": "app-prefixed",
            }
        )
        self.assertEqual(config.mw_url, "https://app.example.test")
        self.assertEqual(config.app_name, "app-prefixed")
        self.assertEqual(config.admin_token.reveal(), "app-admin-secret")

    def test_admin_token_file(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "admin-token"
            path.write_text("token-from-file", encoding="utf-8")
            if os.name != "nt":
                path.chmod(0o600)
            config = from_env({"LEVIER_APP_ADMIN_TOKEN_FILE": str(path)})
            self.assertEqual(config.admin_token.reveal(), "token-from-file")

    def test_p2_1_secret_not_printed(self) -> None:
        secret = Secret("super-secret")
        self.assertEqual(str(secret), "***")
        self.assertEqual(repr(secret), "Secret('***')")
        config = from_env({"LEVIER_ADMIN_TOKEN": "super-secret"})
        self.assertNotIn("super-secret", str(config.summary()))
        self.assertEqual(config.summary()["admin_token"], "***")


if __name__ == "__main__":
    unittest.main()
