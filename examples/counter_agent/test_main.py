"""Middleware — Counter App main wiring tests (P1.8).

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import unittest

from main import TOOLS, execute_tool
from tools import TOOL_DEFINITIONS, Counter


class MainWiringTest(unittest.TestCase):
    def test_p1_8_main_tools_match_registration(self) -> None:
        self.assertEqual(len(TOOLS), len(TOOL_DEFINITIONS))
        for spec, definition in zip(TOOLS, TOOL_DEFINITIONS):
            function = spec["function"]
            self.assertEqual(function["name"], definition["name"])
            self.assertEqual(function["description"], definition["description"])
            self.assertEqual(function["parameters"], definition["schema"])

    def test_p1_8_execute_tool_delegates_to_dispatcher(self) -> None:
        counter = Counter()
        self.assertEqual(
            execute_tool(counter, "counter_add", '{"delta": 2}'),
            {"value": 2},
        )
        self.assertEqual(
            execute_tool(counter, "counter_get", "{}"),
            {"value": 2},
        )
        error = execute_tool(counter, "counter_missing", "{}")
        self.assertEqual(error["code"], "TOOL_NOT_FOUND")
        self.assertEqual(error["retryable"], False)


if __name__ == "__main__":
    unittest.main()
