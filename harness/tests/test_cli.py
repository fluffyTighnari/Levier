"""Middleware — harness CLI skeleton tests.

Copyright 2026 The Middleware Authors.
"""

import tempfile
import unittest
from contextlib import redirect_stderr, redirect_stdout
from io import StringIO
from pathlib import Path

from middleware_harness.__main__ import main

VALID_YAML = """
daemon:
  id: "daemon-cli"
  auth_token: "token"
"""


class CLITest(unittest.TestCase):
    def test_check_mode(self) -> None:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        path = Path(directory.name) / "config.yaml"
        path.write_text(VALID_YAML, encoding="utf-8")

        output = StringIO()
        with redirect_stdout(output):
            code = main(["--config", str(path), "--check"])

        self.assertEqual(code, 0)
        self.assertIn("daemon-cli", output.getvalue())

    def test_invalid_config(self) -> None:
        error = StringIO()
        with redirect_stderr(error):
            code = main(["--config", "/nonexistent/config.yaml", "--check"])
        self.assertEqual(code, 1)
        self.assertIn("harness config error", error.getvalue())


if __name__ == "__main__":
    unittest.main()
