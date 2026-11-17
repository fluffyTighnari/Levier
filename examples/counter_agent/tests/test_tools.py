"""Levier — Counter App local tool tests (P1.1).

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import json
import unittest

from tools import TOOL_DEFINITIONS, Counter


class CounterTest(unittest.TestCase):
    def test_p1_1_counter_add_get_reset(self) -> None:
        counter = Counter()
        self.assertEqual(counter.get(), 0)
        self.assertEqual(counter.add(2), 2)
        self.assertEqual(counter.add(3), 5)
        self.assertEqual(counter.get(), 5)
        self.assertEqual(counter.reset(), 0)
        self.assertEqual(counter.get(), 0)

    def test_p1_1_counter_rejects_non_integer(self) -> None:
        counter = Counter()
        with self.assertRaises(TypeError):
            counter.add("2")  # type: ignore[arg-type]
        with self.assertRaises(TypeError):
            Counter(1.5)  # type: ignore[arg-type]

    def test_p1_1_tool_definitions_match_registration_shape(self) -> None:
        self.assertEqual(
            [tool["name"] for tool in TOOL_DEFINITIONS],
            ["counter_add", "counter_get", "counter_reset"],
        )
        for tool in TOOL_DEFINITIONS:
            self.assertIsInstance(tool["description"], str)
            self.assertIsInstance(tool["timeout_ms"], int)
            self.assertEqual(tool["schema"]["type"], "object")
            # The HTTP registration body must be JSON serializable.
            json.dumps(tool)
        self.assertEqual(
            TOOL_DEFINITIONS[0]["schema"]["required"],
            ["delta"],
        )


if __name__ == "__main__":
    unittest.main()
