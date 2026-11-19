"""Levier - React Runtime CLI tests.

Copyright 2026 The Levier Authors.
"""

import os
import tempfile
import unittest
from contextlib import redirect_stderr, redirect_stdout
from io import StringIO
from pathlib import Path
from unittest.mock import patch

from levier_react_runtime.__main__ import main

VALID_YAML = """
middleware:
  address: "127.0.0.1:9090"
  auth_token: "token"

runtime:
  name: "react1"
  instance_id: "daemon-cli"
  type: "normal"
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
        self.assertIn("react1", output.getvalue())
        self.assertIn("daemon-cli", output.getvalue())

    def test_legacy_env_names_are_mapped(self) -> None:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        path = Path(directory.name) / "config.yaml"
        path.write_text(VALID_YAML, encoding="utf-8")

        output = StringIO()
        with patch.dict(
            os.environ,
            {
                "LEVIER_REACT_RUNTIME_INSTANCE_ID": "legacy-instance",
                "LEVIER_REACT_RUNTIME_NAME": "legacy-runtime",
            },
            clear=False,
        ):
            with redirect_stdout(output):
                code = main(["--config", str(path), "--check"])

        self.assertEqual(code, 0)
        self.assertIn("legacy-runtime", output.getvalue())
        self.assertIn("legacy-instance", output.getvalue())

    def test_version_and_redacted_config(self) -> None:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        path = Path(directory.name) / "config.yaml"
        path.write_text(VALID_YAML, encoding="utf-8")

        output = StringIO()
        with redirect_stdout(output):
            self.assertEqual(main(["--config", str(path), "--version"]), 0)
        self.assertIn("levier-react-runtime", output.getvalue())

        output = StringIO()
        with redirect_stdout(output):
            self.assertEqual(main(["--config", str(path), "--print-config"]), 0)
        text = output.getvalue()
        self.assertIn("middleware_auth_token: ***", text)
        self.assertNotIn("token", text.split("middleware_auth_token")[1].splitlines()[0])

    def test_invalid_config(self) -> None:
        error = StringIO()
        with redirect_stderr(error):
            code = main(["--config", "/nonexistent/config.yaml", "--check"])
        self.assertEqual(code, 1)
        self.assertIn("levier-react-runtime config error", error.getvalue())


if __name__ == "__main__":
    unittest.main()
