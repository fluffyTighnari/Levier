"""Middleware — harness skeleton smoke test.

Copyright 2026 The Middleware Authors.
"""

import unittest

from middleware_harness import __version__


class SmokeTest(unittest.TestCase):
    def test_version(self) -> None:
        self.assertEqual(__version__, "0.1.0")


if __name__ == "__main__":
    unittest.main()
