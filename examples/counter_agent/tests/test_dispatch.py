"""Levier — Counter App dispatcher tests (P1.2).

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import unittest

from dispatch import dispatch_counter_tool
from tools import Counter


class DispatcherTest(unittest.TestCase):
    def test_p1_2_dispatcher_counter_add(self) -> None:
        counter = Counter()
        result = dispatch_counter_tool(counter, "counter_add", '{"delta": 2}')

        self.assertEqual(result["ok"], True)
        self.assertEqual(result["result_json"], '{"value": 2}')
        self.assertEqual(counter.get(), 2)

    def test_p1_3_dispatcher_counter_get(self) -> None:
        counter = Counter(7)
        result = dispatch_counter_tool(counter, "counter_get", "{}")

        self.assertEqual(result["ok"], True)
        self.assertEqual(result["result_json"], '{"value": 7}')
        self.assertEqual(counter.get(), 7)

    def test_p1_4_dispatcher_counter_reset(self) -> None:
        counter = Counter(9)
        result = dispatch_counter_tool(counter, "counter_reset", "{}")

        self.assertEqual(result["ok"], True)
        self.assertEqual(result["result_json"], '{"value": 0}')
        self.assertEqual(counter.get(), 0)

    def test_p1_5_unknown_tool(self) -> None:
        counter = Counter(3)
        result = dispatch_counter_tool(counter, "counter_missing", "{}")

        self.assertEqual(result["ok"], False)
        self.assertEqual(result["error"]["code"], "TOOL_NOT_FOUND")
        self.assertEqual(result["error"]["retryable"], False)
        self.assertIn("counter_missing", result["error"]["message"])
        self.assertEqual(counter.get(), 3)

    def test_p1_6_invalid_arguments(self) -> None:
        cases = [
            ("malformed json", "{", "invalid arguments_json"),
            ("array arguments", "[]", "must be a JSON object"),
            ("missing delta", "{}", "delta is required"),
            ("string delta", '{"delta": "2"}', "must be an integer"),
            ("bool delta", '{"delta": true}', "must be an integer"),
            ("non-string arguments", 123, "must be a string"),
        ]
        for name, arguments, message in cases:
            with self.subTest(name=name):
                counter = Counter(4)
                result = dispatch_counter_tool(counter, "counter_add", arguments)  # type: ignore[arg-type]
                self.assertEqual(result["ok"], False)
                self.assertEqual(result["error"]["code"], "TOOL_EXEC_ERROR")
                self.assertEqual(result["error"]["retryable"], False)
                self.assertIn(message, result["error"]["message"])
                self.assertEqual(counter.get(), 4)


if __name__ == "__main__":
    unittest.main()
