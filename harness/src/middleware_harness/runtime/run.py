"""Middleware — one active run context.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import asyncio
import time
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any

from middleware.runtime.v1 import runtime_pb2

from middleware_harness.runtime.checkpoint import CheckpointState
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
    resume: CheckpointState | None = None
    checkpoint_enabled: bool = False
    checkpoint_every_events: int = 20
    checkpoint_every_seconds: float = 60.0
    _last_checkpoint_events: int = 0
    _last_checkpoint_at: float = field(default_factory=time.monotonic)
    _checkpoint_lock: asyncio.Lock = field(default_factory=asyncio.Lock)

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

    def checkpoint_due(self) -> bool:
        """Whether a longterm checkpoint should be written at this safe point."""
        if not self.checkpoint_enabled or not self.assign.session_id:
            return False
        if self.events.seq - self._last_checkpoint_events >= self.checkpoint_every_events:
            return True
        return time.monotonic() - self._last_checkpoint_at >= self.checkpoint_every_seconds

    async def save_checkpoint(self, state: CheckpointState) -> None:
        """Send a checkpoint and record its watermark."""
        if not self.checkpoint_enabled or not self.assign.session_id:
            return
        async with self._checkpoint_lock:
            state.last_event_attempt = self.attempt
            state.last_event_seq = self.events.seq
            await self.events.emit_checkpoint(
                self.assign.session_id,
                state.encode(),
                watermark=self.events.seq,
            )
            await self.emit(
                runtime_pb2.EventType.CHECKPOINT_SAVED,
                {
                    "session_id": self.assign.session_id,
                    "event_seq_watermark": self.events.seq,
                },
            )
            self._last_checkpoint_events = self.events.seq
            self._last_checkpoint_at = time.monotonic()
