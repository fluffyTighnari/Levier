"""Middleware — checkpoint encoding and run-context tests.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import time
import unittest

from middleware.runtime.v1 import runtime_pb2

from middleware_harness.llm.base import Message, ToolCall
from middleware_harness.runtime.checkpoint import (
    CheckpointError,
    CheckpointState,
    capture,
)
from middleware_harness.runtime.events import EventWriter
from middleware_harness.runtime.run import RunContext


class FakeSender:
    def __init__(self) -> None:
        self.sent: list[runtime_pb2.Envelope] = []

    async def send(self, envelope: runtime_pb2.Envelope) -> None:
        self.sent.append(envelope)


def state() -> CheckpointState:
    return capture(
        session_id="session-1",
        run_id="run-1",
        attempt=2,
        strategy="react",
        step=3,
        messages=[
            Message(role="user", content="hello"),
            Message(
                role="assistant",
                content="thinking",
                tool_calls=(ToolCall(
                    call_id="call-1",
                    name="add",
                    arguments_json='{"a":1}',
                ),),
            ),
            Message(role="tool", content="1", name="add", tool_call_id="call-1"),
        ],
        last_event_seq=11,
    )


class CheckpointCodecTest(unittest.TestCase):
    def test_round_trip(self) -> None:
        original = state()
        decoded = CheckpointState.decode(original.encode())
        self.assertEqual(decoded.session_id, "session-1")
        self.assertEqual(decoded.run_id, "run-1")
        self.assertEqual(decoded.attempt, 2)
        self.assertEqual(decoded.strategy, "react")
        self.assertEqual(decoded.step, 3)
        self.assertEqual(decoded.last_event_attempt, 2)
        self.assertEqual(decoded.last_event_seq, 11)
        self.assertEqual(len(decoded.messages), 3)
        self.assertEqual(decoded.messages[1].tool_calls[0].name, "add")
        self.assertEqual(decoded.messages[2].tool_call_id, "call-1")

    def test_invalid_version(self) -> None:
        payload = b'{"version":99,"session_id":"s","run_id":"r","strategy":"react","attempt":1,"step":0,"messages":[]}'
        with self.assertRaises(CheckpointError):
            CheckpointState.decode(payload)

    def test_invalid_json(self) -> None:
        with self.assertRaises(CheckpointError):
            CheckpointState.decode(b"{not-json}")

    def test_missing_fields(self) -> None:
        with self.assertRaises(CheckpointError):
            CheckpointState.decode(b'{"version":1}')


class RunContextCheckpointTest(unittest.IsolatedAsyncioTestCase):
    def make_context(self, *, every_events: int = 20, every_seconds: float = 60.0) -> tuple[RunContext, FakeSender]:
        sender = FakeSender()
        assign = runtime_pb2.TaskAssign(
            run_id="run-1",
            app_id="app-1",
            strategy="react",
            session_id="session-1",
        )
        ctx = RunContext(
            assign=assign,
            attempt=2,
            lease_epoch=5,
            events=EventWriter(sender, "run-1", 2, 5),
            checkpoint_enabled=True,
            checkpoint_every_events=every_events,
            checkpoint_every_seconds=every_seconds,
        )
        return ctx, sender

    async def test_save_checkpoint_sends_envelope_and_event(self) -> None:
        ctx, sender = self.make_context()
        await ctx.emit(runtime_pb2.EventType.RUN_STATUS, {"status": "running"})
        await ctx.save_checkpoint(state())

        checkpoint = next(
            envelope for envelope in sender.sent
            if envelope.WhichOneof("payload") == "checkpoint"
        )
        self.assertEqual(checkpoint.checkpoint.session_id, "session-1")
        self.assertEqual(checkpoint.checkpoint.event_seq_watermark, 1)
        self.assertEqual(checkpoint.attempt, 2)
        self.assertEqual(checkpoint.lease_epoch, 5)
        decoded = CheckpointState.decode(checkpoint.checkpoint.state)
        self.assertEqual(decoded.last_event_attempt, 2)
        self.assertEqual(decoded.last_event_seq, 1)

        saved = next(
            envelope for envelope in sender.sent
            if envelope.WhichOneof("payload") == "event"
            and envelope.event.type == runtime_pb2.EventType.CHECKPOINT_SAVED
        )
        self.assertEqual(saved.event.data.fields["event_seq_watermark"].number_value, 1)
        self.assertEqual(saved.event.seq, 2)
        self.assertFalse(ctx.checkpoint_due())

    async def test_checkpoint_due_by_event_count(self) -> None:
        ctx, _ = self.make_context(every_events=1)
        self.assertFalse(ctx.checkpoint_due())
        await ctx.emit(runtime_pb2.EventType.RUN_STATUS, {"status": "running"})
        self.assertTrue(ctx.checkpoint_due())

    async def test_checkpoint_due_by_time(self) -> None:
        ctx, _ = self.make_context(every_seconds=0.001)
        ctx._last_checkpoint_at = time.monotonic() - 1
        self.assertTrue(ctx.checkpoint_due())

    async def test_no_checkpoint_without_session(self) -> None:
        ctx, sender = self.make_context()
        ctx.assign.session_id = ""
        await ctx.save_checkpoint(state())
        self.assertFalse(any(
            envelope.WhichOneof("payload") == "checkpoint"
            for envelope in sender.sent
        ))
        self.assertFalse(ctx.checkpoint_due())


if __name__ == "__main__":
    unittest.main()
