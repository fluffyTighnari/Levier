"""Levier - standalone echo runtime tests.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import asyncio
import unittest

from middleware.runtime.v1 import runtime_pb2

from levier_harness.channel.envelope import new_envelope, response_envelope
from levier_harness.runtime import ManagerConfig, RunManager
from levier_harness.echo import EchoRuntime


class FakeSender:
    def __init__(self) -> None:
        self.sent: list[runtime_pb2.Envelope] = []

    async def send(self, envelope: runtime_pb2.Envelope) -> None:
        self.sent.append(envelope)

    async def send_response(self, request: runtime_pb2.Envelope, payload, field: str) -> None:
        self.sent.append(response_envelope(request, payload, field))


def assign_envelope(content: str = "hello") -> runtime_pb2.Envelope:
    assign = runtime_pb2.TaskAssign(
        run_id="run-echo",
        app_id="app-1",
        input=[runtime_pb2.ChatMessage(role="user", content=content)],
    )
    return new_envelope(assign, "task_assign", run_id="run-echo", attempt=1, lease_epoch=1)


class EchoRuntimeTest(unittest.IsolatedAsyncioTestCase):
    async def test_echo_sequence(self) -> None:
        sender = FakeSender()
        manager = RunManager(
            sender,
            ManagerConfig(runtime_name="echo1"),
            EchoRuntime(),
        )
        try:
            await manager.handle(assign_envelope("hello"))
            for _ in range(200):
                if manager.running_count() == 0:
                    break
                await asyncio.sleep(0.005)
            self.assertEqual(manager.running_count(), 0)
        finally:
            await manager.close()

        events = [env for env in sender.sent if env.WhichOneof("payload") == "event"]
        self.assertEqual(
            [event.event.type for event in events],
            [
                runtime_pb2.EventType.RUN_STATUS,
                runtime_pb2.EventType.MESSAGE_COMPLETED,
                runtime_pb2.EventType.RUN_STATUS,
            ],
        )
        self.assertEqual([event.event.seq for event in events], [1, 2, 3])
        completed = events[1].event.data
        self.assertEqual(completed.fields["role"].string_value, "assistant")
        self.assertEqual(completed.fields["content"].string_value, "hello")

    async def test_echo_default_content(self) -> None:
        sender = FakeSender()
        manager = RunManager(
            sender,
            ManagerConfig(runtime_name="echo1"),
            EchoRuntime(),
        )
        try:
            await manager.handle(assign_envelope(""))
            for _ in range(200):
                if manager.running_count() == 0:
                    break
                await asyncio.sleep(0.005)
        finally:
            await manager.close()

        events = [env for env in sender.sent if env.WhichOneof("payload") == "event"]
        completed = next(
            event for event in events
            if event.event.type == runtime_pb2.EventType.MESSAGE_COMPLETED
        )
        self.assertEqual(completed.event.data.fields["content"].string_value, "echo")


if __name__ == "__main__":
    unittest.main()
