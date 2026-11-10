"""Middleware — Counter App dispatcher tests (P1.2).

Copyright 2026 The Middleware Authors.
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


if __name__ == "__main__":
    unittest.main()
