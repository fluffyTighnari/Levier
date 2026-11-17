"""Levier — run event writer.

Assigns per-run sequence numbers and sends events through the channel.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

from typing import Any, Protocol

from google.protobuf import struct_pb2
from middleware.runtime.v1 import runtime_pb2

from levier_harness.channel.envelope import new_envelope


class EventSender(Protocol):
    """Minimal sender interface used by EventWriter."""

    async def send(self, envelope: runtime_pb2.Envelope) -> None:
        """Send one envelope."""


class EventWriter:
    """Write persistent and ephemeral events for one run attempt."""

    def __init__(
        self,
        sender: EventSender,
        run_id: str,
        attempt: int,
        lease_epoch: int,
    ) -> None:
        self.sender = sender
        self.run_id = run_id
        self.attempt = attempt
        self.lease_epoch = lease_epoch
        self.seq = 0

    async def emit(
        self,
        event_type: runtime_pb2.EventType,
        data: dict[str, Any],
        *,
        ephemeral: bool = False,
    ) -> None:
        """Emit one event; only persistent events consume seq."""
        if ephemeral:
            seq = 0
        else:
            self.seq += 1
            seq = self.seq

        payload = struct_pb2.Struct()
        payload.update(data or {})
        event = runtime_pb2.Event(
            seq=seq,
            type=event_type,
            ephemeral=ephemeral,
            source="daemon",
            data=payload,
        )
        envelope = new_envelope(
            event,
            "event",
            run_id=self.run_id,
            attempt=self.attempt,
            lease_epoch=self.lease_epoch,
        )
        await self.sender.send(envelope)

    async def emit_checkpoint(
        self,
        session_id: str,
        state: bytes,
        *,
        watermark: int | None = None,
    ) -> int:
        """Send one checkpoint covering the current persistent event seq."""
        covered = self.seq if watermark is None else watermark
        checkpoint = runtime_pb2.Checkpoint(
            session_id=session_id,
            event_seq_watermark=covered,
            state=state,
        )
        envelope = new_envelope(
            checkpoint,
            "checkpoint",
            run_id=self.run_id,
            attempt=self.attempt,
            lease_epoch=self.lease_epoch,
        )
        await self.sender.send(envelope)
        return covered
