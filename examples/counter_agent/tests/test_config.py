"""Middleware — Counter App configuration tests (P2.1).

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import unittest

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

    def test_p2_1_secret_not_printed(self) -> None:
        secret = Secret("super-secret")
        self.assertEqual(str(secret), "***")
        self.assertEqual(repr(secret), "Secret('***')")
        config = from_env({"LEVIER_ADMIN_TOKEN": "super-secret"})
        self.assertNotIn("super-secret", str(config.summary()))
        self.assertEqual(config.summary()["admin_token"], "***")


if __name__ == "__main__":
    unittest.main()
