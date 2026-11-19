"""Levier - React Runtime logging tests.

Copyright 2026 The Levier Authors.
"""

import io
import json
import logging
import unittest

from levier_react_runtime.logging import JsonFormatter, configure_logging


class LoggingTest(unittest.TestCase):
    def test_json_formatter_includes_fields(self) -> None:
        record = logging.LogRecord(
            name="react_runtime",
            level=logging.INFO,
            pathname=__file__,
            lineno=1,
            msg="run started",
            args=(),
            exc_info=None,
        )
        record.run_id = "run-1"
        record.attempt = 2
        payload = json.loads(JsonFormatter().format(record))
        self.assertEqual(payload["message"], "run started")
        self.assertEqual(payload["run_id"], "run-1")
        self.assertEqual(payload["attempt"], 2)

    def test_configure_logging_json(self) -> None:
        stream = io.StringIO()
        configure_logging("debug", "json", stream)
        logging.getLogger("react-runtime-test").info("hello")
        payload = json.loads(stream.getvalue().strip())
        self.assertEqual(payload["level"], "INFO")
        self.assertEqual(payload["message"], "hello")


if __name__ == "__main__":
    unittest.main()
