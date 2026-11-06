"""Middleware — harness ULID tests.

Copyright 2026 The Middleware Authors.
"""

import unittest
from datetime import datetime, timezone

from middleware_harness.ids import new_ulid, ulid_timestamp_ms


class ULIDTest(unittest.TestCase):
    def test_length_alphabet_and_timestamp(self) -> None:
        now_ms = 1_730_000_000_000
        value = new_ulid(now_ms)
        self.assertEqual(len(value), 26)
        self.assertTrue(all(char in "0123456789ABCDEFGHJKMNPQRSTVWXYZ" for char in value))
        self.assertEqual(ulid_timestamp_ms(value), now_ms)
        self.assertEqual(
            datetime.fromtimestamp(now_ms / 1000, tz=timezone.utc).year,
            2024,
        )

    def test_unique(self) -> None:
        self.assertEqual(len({new_ulid() for _ in range(1000)}), 1000)


if __name__ == "__main__":
    unittest.main()
