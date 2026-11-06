"""Middleware — strategy dispatch.

Routes each TaskAssign to the strategy named in the assignment.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

from typing import Any, Mapping

from middleware_harness.runtime.run import RunContext


class StrategyRouter:
    """Dispatch runs by TaskAssign.strategy."""

    def __init__(self, strategies: Mapping[str, Any]) -> None:
        self.strategies = dict(strategies)

    @property
    def names(self) -> tuple[str, ...]:
        """Supported strategy names."""
        return tuple(self.strategies)

    async def __call__(self, ctx: RunContext) -> str:
        """Run the strategy selected by the assignment."""
        strategy = self.strategies.get(ctx.assign.strategy)
        if strategy is None:
            raise ValueError(f"unsupported strategy {ctx.assign.strategy!r}")
        run = getattr(strategy, "run", None)
        if callable(run):
            return await run(ctx)
        return await strategy(ctx)
