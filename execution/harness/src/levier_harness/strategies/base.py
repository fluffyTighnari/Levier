"""Middleware — strategy interface.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

from typing import Protocol

from levier_harness.runtime.run import RunContext


class Strategy(Protocol):
    """A strategy executes one RunContext and returns a terminal status."""

    name: str

    async def run(self, ctx: RunContext) -> str:
        """Execute the strategy; return completed/failed/cancelled."""
