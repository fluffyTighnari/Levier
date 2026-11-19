from __future__ import annotations

import os
import stat
import tempfile
import unittest
from pathlib import Path

from levier_app.credentials import (
    Credentials,
    CredentialsError,
    Secret,
    load_credentials,
    save_credentials,
)


class CredentialsTest(unittest.TestCase):
    def test_round_trip_canonical_file(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "app.json"
            save_credentials(path, Credentials(
                app_id="app-1",
                app_name="demo",
                api_key=Secret("mw_secret"),
                server="http://127.0.0.1:8080/",
            ))
            loaded = load_credentials(path)
            if os.name != "nt":
                self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o600)
            self.assertEqual(loaded.app_id, "app-1")
            self.assertEqual(loaded.api_key.reveal(), "mw_secret")
            self.assertEqual(loaded.server, "http://127.0.0.1:8080")
            self.assertNotIn("mw_secret", repr(loaded))

    def test_legacy_file_still_loads(self) -> None:
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
            self.assertEqual(loaded.server, "")

    @unittest.skipIf(os.name == "nt", "POSIX mode bits are not enforced on Windows")
    def test_rejects_loose_permissions(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "app.json"
            path.write_text(
                '{"app_id":"a","app_name":"n","api_key":"mw_x"}',
                encoding="utf-8",
            )
            os.chmod(path, 0o644)
            with self.assertRaises(CredentialsError):
                load_credentials(path)


if __name__ == "__main__":
    unittest.main()
