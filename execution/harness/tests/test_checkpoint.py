"""Levier — checkpoint encoding and run-context tests.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import time
import unittest

from middleware.runtime.v1 import runtime_pb2

from levier_harness.llm.base import Message, ToolCall
from levier_harness.runtime.checkpoint import (
    CheckpointError,
    CheckpointState,
    capture,
)


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
        runtime_name="react1",
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
        self.assertEqual(decoded.runtime_name, "react1")
        self.assertEqual(decoded.step, 3)
        self.assertEqual(decoded.last_event_attempt, 2)
        self.assertEqual(decoded.last_event_seq, 11)
        self.assertEqual(len(decoded.messages), 3)
        self.assertEqual(decoded.messages[1].tool_calls[0].name, "add")
        self.assertEqual(decoded.messages[2].tool_call_id, "call-1")

    def test_invalid_version(self) -> None:
        payload = b'{"version":99,"session_id":"s","run_id":"r","runtime_name":"react1","attempt":1,"step":0,"messages":[]}'
        with self.assertRaises(CheckpointError):
            CheckpointState.decode(payload)

    def test_invalid_json(self) -> None:
        with self.assertRaises(CheckpointError):
            CheckpointState.decode(b"{not-json}")

    def test_missing_fields(self) -> None:
        with self.assertRaises(CheckpointError):
            CheckpointState.decode(b'{"version":1}')


if __name__ == "__main__":
    unittest.main()
