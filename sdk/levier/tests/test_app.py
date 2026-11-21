"""Tests for the one-import App wrapper."""

from __future__ import annotations

import json
import os
import tempfile
import unittest
from pathlib import Path

from levier import App


class AppWrapperTest(unittest.TestCase):
    def credentials(self) -> Path:
        directory = Path(tempfile.mkdtemp())
        path = directory / "app.json"
        path.write_text(
            json.dumps(
                {
                    "credential_version": 1,
                    "app_id": "app-1",
                    "app_name": "demo",
                    "api_key": "mw_test",
                    "server": "http://127.0.0.1:8080",
                }
            ),
            encoding="utf-8",
        )
        if os.name != "nt":
            path.chmod(0o600)
        return path

    def test_tools_and_runtime_defaults(self) -> None:
        path = self.credentials()
        app = App(
            tools=[],
            credentials=str(path),
            runtime="echo1",
        )

        @app.tool
        def counter_add(delta: int) -> dict:
            """Add delta."""
            return {"value": delta}

        self.assertEqual(app.runtime, "echo1")
        manifest = app._app.tools_manifest()
        self.assertEqual(manifest[0]["name"], "counter_add")
        self.assertEqual(manifest[0]["schema"]["properties"]["delta"], {"type": "integer"})


if __name__ == "__main__":
    unittest.main()
