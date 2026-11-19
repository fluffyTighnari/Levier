"""Counter domain-state tests.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(ROOT / "examples" / "counter_agent"))

from tools import Counter  # noqa: E402


class CounterTest(unittest.TestCase):
    def test_add_get_reset(self) -> None:
        counter = Counter()
        self.assertEqual(counter.get(), 0)
        self.assertEqual(counter.add(2), 2)
        self.assertEqual(counter.add(3), 5)
        self.assertEqual(counter.get(), 5)
        self.assertEqual(counter.reset(), 0)

    def test_rejects_non_integer(self) -> None:
        counter = Counter()
        with self.assertRaises(TypeError):
            counter.add("2")  # type: ignore[arg-type]
        with self.assertRaises(TypeError):
            Counter(1.5)  # type: ignore[arg-type]


if __name__ == "__main__":
    unittest.main()
