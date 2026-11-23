"""Custom Runtime event contract tests."""

from __future__ import annotations

import unittest

from middleware.runtime.v1 import runtime_pb2

from levier_runtime.events import EventWriter


class FakeSender:
    def __init__(self) -> None:
        self.sent: list[runtime_pb2.Envelope] = []

    async def send(self, envelope: runtime_pb2.Envelope) -> None:
        self.sent.append(envelope)


class CustomEventTest(unittest.IsolatedAsyncioTestCase):
    async def test_custom_event_carries_name_and_version(self) -> None:
        sender = FakeSender()
        writer = EventWriter(sender, "run-1", 1, 1)
        await writer.emit(
            "CUSTOM",
            {"node": "research"},
            name="graph.node.started",
            schema_version="1",
        )

        self.assertEqual(len(sender.sent), 1)
        event = sender.sent[0].event
        self.assertEqual(event.type, runtime_pb2.EventType.CUSTOM)
        self.assertEqual(event.name, "graph.node.started")
        self.assertEqual(event.schema_version, "1")
        self.assertEqual(event.seq, 1)
        self.assertEqual(event.data.fields["node"].string_value, "research")

    async def test_custom_event_requires_name(self) -> None:
        writer = EventWriter(FakeSender(), "run-1", 1, 1)
        with self.assertRaises(ValueError):
            await writer.emit("CUSTOM", {})

    async def test_fixed_event_rejects_custom_metadata(self) -> None:
        writer = EventWriter(FakeSender(), "run-1", 1, 1)
        with self.assertRaises(ValueError):
            await writer.emit("MESSAGE_COMPLETED", {}, name="graph.node")


if __name__ == "__main__":
    unittest.main()
