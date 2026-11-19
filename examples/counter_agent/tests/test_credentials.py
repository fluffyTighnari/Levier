"""Levier — App credential storage tests (P3.3).

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import os
import stat
import tempfile
import unittest
from pathlib import Path

from config import Secret
from credentials import AppCredentials, CredentialsError, load_credentials, save_credentials


class CredentialsTest(unittest.TestCase):
    def test_p3_3_round_trip_and_mode(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "app.json"
            credentials = AppCredentials(
                app_id="app-1",
                app_name="counter-app",
                api_key=Secret("mw_secret"),
            )
            save_credentials(path, credentials)
            if os.name != "nt":
                self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o600)
            loaded = load_credentials(path)
            self.assertEqual(loaded.app_id, "app-1")
            self.assertEqual(loaded.app_name, "counter-app")
            self.assertEqual(loaded.api_key.reveal(), "mw_secret")
            self.assertNotIn("mw_secret", repr(loaded))
            self.assertNotIn("mw_secret", str(loaded.summary()))
            self.assertEqual(loaded.credential_version, 1)
            self.assertEqual(loaded.server, "")
            self.assertTrue(loaded.created_at)
            self.assertTrue(loaded.updated_at)

    def test_legacy_credentials_still_load(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "app.json"
            path.write_text(
                '{"app_id":"a","app_name":"n","api_key":"mw_x"}',
                encoding="utf-8",
            )
            os.chmod(path, 0o600)
            loaded = load_credentials(path)
            self.assertEqual(loaded.app_id, "a")
            self.assertEqual(loaded.credential_version, 1)

    @unittest.skipIf(os.name == "nt", "POSIX file mode enforcement is not available on Windows")
    def test_p3_3_rejects_group_readable_file(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "app.json"
            path.write_text('{"app_id":"a","app_name":"n","api_key":"mw_x"}')
            os.chmod(path, 0o644)
            with self.assertRaises(CredentialsError):
                load_credentials(path)

    def test_server_metadata_round_trip(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "app.json"
            credentials = AppCredentials(
                app_id="a",
                app_name="n",
                api_key=Secret("mw_x"),
                server="http://127.0.0.1:8080/",
            )
            save_credentials(path, credentials)
            loaded = load_credentials(path)
            self.assertEqual(loaded.server, "http://127.0.0.1:8080")
            self.assertEqual(loaded.credential_version, 1)

    def test_p3_3_rejects_missing_fields(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "app.json"
            path.write_text('{"app_id":"a"}')
            os.chmod(path, 0o600)
            with self.assertRaises(CredentialsError):
                load_credentials(path)


if __name__ == "__main__":
    unittest.main()
