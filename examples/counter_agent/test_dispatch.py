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


if __name__ == "__main__":
    unittest.main()
