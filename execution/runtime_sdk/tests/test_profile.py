from __future__ import annotations

import os
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from levier_runtime.profile import (
    default_mcp_socket,
    default_runtime_config,
    profile_path,
)


class ProfileTest(unittest.TestCase):
    def test_profile_values_are_used_by_runtime_defaults(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            home = Path(directory)
            runtime_config = home / "runtime.yaml"
            (home / "config.yaml").write_text(
                "server: http://127.0.0.1:8080\n"
                f"runtime_config: {runtime_config}\n"
                f"mcp_socket: {home / 'mcp.sock'}\n",
                encoding="utf-8",
            )
            with mock.patch.dict(os.environ, {"LEVIER_HOME": directory}):
                self.assertEqual(profile_path(), home / "config.yaml")
                self.assertEqual(default_runtime_config(), str(runtime_config))
                self.assertEqual(default_mcp_socket(), str(home / "mcp.sock"))


if __name__ == "__main__":
    unittest.main()
