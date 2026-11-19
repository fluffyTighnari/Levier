"""Counter App business state.

This module contains no transport or protocol code.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations


class Counter:
    """The App-owned counter state."""

    def __init__(self, initial: int = 0) -> None:
        if not isinstance(initial, int) or isinstance(initial, bool):
            raise TypeError("initial counter value must be an integer")
        self.value = initial

    def add(self, delta: int) -> int:
        if not isinstance(delta, int) or isinstance(delta, bool):
            raise TypeError("delta must be an integer")
        self.value += delta
        return self.value

    def get(self) -> int:
        return self.value

    def reset(self) -> int:
        self.value = 0
        return self.value
