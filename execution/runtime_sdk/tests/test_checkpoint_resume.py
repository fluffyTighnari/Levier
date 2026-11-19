"""Levier — checkpoint report and resume tests.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import asyncio
import unittest

from middleware.runtime.v1 import runtime_pb2

from levier_runtime.channel.envelope import new_envelope, response_envelope
from levier_runtime.manager import ManagerConfig, RunManager
from levier_runtime.llm.base import LLMResponse, Message, ModelConfig, ToolSpec
from levier_runtime.react.checkpoint import CheckpointState
from levier_runtime.react import ReactAgent


class FakeSender:
    def __init__(self) -> None:
        self.sent: list[runtime_pb2.Envelope] = []

    async def send(self, envelope: runtime_pb2.Envelope) -> None:
        self.sent.append(envelope)

    async def send_response(self, request: runtime_pb2.Envelope, payload, field: str) -> None:
        self.sent.append(response_envelope(request, payload, field))


class ScriptedProvider:
    def __init__(self, content: str) -> None:
        self.content = content
        self.calls: list[list[Message]] = []

    async def complete(self, messages, tools, model: ModelConfig, *, on_delta=None) -> LLMResponse:
        self.calls.append(list(messages))
        return LLMResponse(content=self.content, model=model.model)


def assign(
    *,
    session_id: str = "session-1",
    resume: bytes = b"",
    attempt: int = 1,
) -> runtime_pb2.Envelope:
    task = runtime_pb2.TaskAssign(
        run_id="run-1",
        app_id="app-1",
        session_id=session_id,
        input=[runtime_pb2.ChatMessage(role="user", content="hello")],
        model=runtime_pb2.ModelProfile(provider="anthropic", model="claude-test"),
        resume_from=runtime_pb2.Checkpoint(
            session_id=session_id,
            state=resume,
        ),
    )
    return new_envelope(task, "task_assign", run_id="run-1", attempt=attempt, lease_epoch=1)


def events(sender: FakeSender) -> list[runtime_pb2.Envelope]:
    return [
        envelope for envelope in sender.sent
        if envelope.WhichOneof("payload") == "event"
    ]


class CheckpointResumeTest(unittest.IsolatedAsyncioTestCase):
    async def run_once(self, provider, envelope) -> FakeSender:
        sender = FakeSender()
        manager = RunManager(
            sender,
            ManagerConfig(runtime_name="react1"),
            ReactAgent(provider),
        )
        try:
            await manager.handle(envelope)
            for _ in range(200):
                if manager.running_count() == 0:
                    break
                await asyncio.sleep(0.005)
        finally:
            await manager.close()
        return sender

    async def test_resume_does_not_repeat_previous_message(self) -> None:
        first = await self.run_once(ScriptedProvider("first"), assign(attempt=1))
        first_events = events(first)
        checkpoint_envelope = next(
            envelope for envelope in first.sent
            if envelope.WhichOneof("payload") == "checkpoint"
        )
        checkpoint = CheckpointState.decode(checkpoint_envelope.checkpoint.state)
        self.assertEqual(checkpoint.step, 1)
        self.assertEqual(
            [event.event.type for event in first_events].count(
                runtime_pb2.EventType.MESSAGE_COMPLETED
            ),
            1,
        )

        provider = ScriptedProvider("second")
        second = await self.run_once(
            provider,
            assign(resume=checkpoint.encode(), attempt=2),
        )
        second_events = events(second)
        messages = [
            event.event.data.fields["content"].string_value
            for event in second_events
            if event.event.type == runtime_pb2.EventType.MESSAGE_COMPLETED
        ]
        self.assertEqual(messages, ["second"])
        self.assertEqual(
            [message.role for message in provider.calls[0]],
            ["user", "assistant"],
        )
        self.assertEqual(provider.calls[0][1].content, "first")
        self.assertIn(
            runtime_pb2.EventType.CHECKPOINT_SAVED,
            [event.event.type for event in second_events],
        )

    async def test_normal_runtime_is_stateless(self) -> None:
        sender = FakeSender()
        manager = RunManager(
            sender,
            ManagerConfig(runtime_name="react1"),
            ReactAgent(ScriptedProvider("normal")),
        )
        try:
            await manager.handle(assign(session_id="", attempt=1))
            for _ in range(200):
                if manager.running_count() == 0:
                    break
                await asyncio.sleep(0.005)
        finally:
            await manager.close()
        self.assertFalse(any(
            envelope.WhichOneof("payload") == "checkpoint"
            for envelope in sender.sent
        ))
        self.assertFalse(any(
            envelope.WhichOneof("payload") == "event"
            and envelope.event.type == runtime_pb2.EventType.CHECKPOINT_SAVED
            for envelope in sender.sent
        ))

    async def test_invalid_checkpoint_is_rejected(self) -> None:
        sender = FakeSender()
        manager = RunManager(
            sender,
            ManagerConfig(runtime_name="react1"),
            ReactAgent(ScriptedProvider("never")),
        )
        try:
            await manager.handle(assign(resume=b"{bad", attempt=2))
            for _ in range(200):
                if manager.running_count() == 0:
                    break
                await asyncio.sleep(0.005)
        finally:
            await manager.close()

        ack = next(
            envelope for envelope in sender.sent
            if envelope.WhichOneof("payload") == "task_ack"
        )
        self.assertTrue(ack.task_ack.accepted)
        error = next(
            envelope for envelope in sender.sent
            if envelope.WhichOneof("payload") == "event"
            and envelope.event.type == runtime_pb2.EventType.ERROR
        )
        self.assertEqual(error.event.data.fields["code"].string_value, "INVALID_CHECKPOINT")

    async def test_mismatched_checkpoint_is_rejected(self) -> None:
        checkpoint = CheckpointState(
            session_id="other-session",
            run_id="run-1",
            attempt=1,
            runtime_name="react1",
            step=1,
        )
        sender = FakeSender()
        manager = RunManager(
            sender,
            ManagerConfig(runtime_name="react1"),
            ReactAgent(ScriptedProvider("never")),
        )
        try:
            await manager.handle(assign(resume=checkpoint.encode(), attempt=2))
            for _ in range(200):
                if manager.running_count() == 0:
                    break
                await asyncio.sleep(0.005)
        finally:
            await manager.close()

        ack = next(
            envelope for envelope in sender.sent
            if envelope.WhichOneof("payload") == "task_ack"
        )
        self.assertTrue(ack.task_ack.accepted)
        error = next(
            envelope for envelope in sender.sent
            if envelope.WhichOneof("payload") == "event"
            and envelope.event.type == runtime_pb2.EventType.ERROR
        )
        self.assertEqual(error.event.data.fields["code"].string_value, "CHECKPOINT_SESSION_MISMATCH")


if __name__ == "__main__":
    unittest.main()
