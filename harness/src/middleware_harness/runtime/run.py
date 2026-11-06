"""Middleware — one active run context.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import asyncio
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any

from middleware.runtime.v1 import runtime_pb2

from middleware_harness.runtime.events import EventWriter


@dataclass
class RunContext:
    """State and collaborators for one TaskAssign."""

    assign: runtime_pb2.TaskAssign
    attempt: int
    lease_epoch: int
    events: EventWriter
    cancel_event: asyncio.Event = field(default_factory=asyncio.Event)
    task: asyncio.Task[Any] | None = None
    started_at: datetime = field(default_factory=lambda: datetime.now(tz=timezone.utc))
    outcome: str = ""
    tool_sessions: list[str] = field(default_factory=list)
    tool_routes: dict[str, str] = field(default_factory=dict)

    @property
    def run_id(self) -> str:
        """Return the run identifier."""
        return self.assign.run_id

    @property
    def cancelled(self) -> bool:
        """Whether cancel was requested."""
        return self.cancel_event.is_set()

    async def emit(
        self,
        event_type: runtime_pb2.EventType,
        data: dict[str, Any],
        *,
        ephemeral: bool = False,
    ) -> None:
        """Emit one run event."""
        await self.events.emit(event_type, data, ephemeral=ephemeral)
