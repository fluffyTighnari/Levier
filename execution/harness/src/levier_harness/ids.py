"""Levier — harness identifier helpers.

Generates ULIDs locally so the harness can create valid message IDs
without an extra dependency.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import os
import time

_ALPHABET = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"


def new_ulid(timestamp_ms: int | None = None) -> str:
    """Return a 26-character Crockford Base32 ULID."""
    timestamp = int(time.time() * 1000) if timestamp_ms is None else timestamp_ms
    timestamp &= (1 << 48) - 1
    randomness = int.from_bytes(os.urandom(10), "big")
    value = (timestamp << 80) | randomness
    return "".join(
        _ALPHABET[(value >> (5 * (25 - index))) & 0x1F]
        for index in range(26)
    )


def ulid_timestamp_ms(value: str) -> int:
    """Extract the millisecond timestamp from a 26-character ULID."""
    if len(value) != 26:
        raise ValueError("ULID must be 26 characters")
    number = 0
    for char in value.upper():
        number = (number << 5) | _ALPHABET.index(char)
    return (number >> 80) & ((1 << 48) - 1)
