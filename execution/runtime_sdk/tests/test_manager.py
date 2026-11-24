"""Runtime SDK context and manager tests."""

from __future__ import annotations

import asyncio
import json
import unittest

from middleware.runtime.v1 import runtime_pb2

from levier_runtime.channel.envelope import new_envelope, response_envelope
from levier_runtime.manager import ManagerConfig, RunManager


class FakeSender:
    def __init__(self) -> None:
        self.sent: list[runtime_pb2.Envelope] = []

    async def send(self, envelope: runtime_pb2.Envelope) -> None:
        self.sent.append(envelope)

    async def send_response(self, request, payload, field: str) -> None:
        self.sent.append(response_envelope(request, payload, field))


def assign_envelope(
    *,
    run_id: str = "run-1",
    attempt: int = 1,
    epoch: int = 1,
    session_id: str = "",
) -> runtime_pb2.Envelope:
    assign = runtime_pb2.TaskAssign(
        run_id=run_id,
        app_id="app-1",
        input=[runtime_pb2.ChatMessage(role="user", content="hello")],
        model=runtime_pb2.ModelProfile(provider="test", model="test-model"),
        session_id=session_id,
    )
    assign.params.update({"max_steps": 3})
    return new_envelope(
        assign,
        "task_assign",
        run_id=run_id,
        attempt=attempt,
        lease_epoch=epoch,
    )


class CapturingRunner:
    def __init__(self, status: str = "completed") -> None:
        self.status = status
        self.context = None

    async def __call__(self, ctx):
        self.context = ctx
        await ctx.reply("answer")
        return self.status


class WaitingRunner:
    def __init__(self) -> None:
        self.context = None

    async def __call__(self, ctx):
        self.context = ctx
        return await ctx.wait({"step": 1})


class RunManagerTest(unittest.IsolatedAsyncioTestCase):
    async def wait_done(self, manager: RunManager) -> None:
        for _ in range(200):
            if manager.running_count() == 0:
                return
            await asyncio.sleep(0.005)
        self.fail("run did not finish")

    async def test_assign_uses_plain_context_and_terminal_event(self) -> None:
        sender = FakeSender()
        runner = CapturingRunner()
        manager = RunManager(
            sender,
            ManagerConfig(runtime_name="echo1", max_concurrent_runs=2),
            runner,
        )
        await manager.handle(assign_envelope())
        await self.wait_done(manager)

        self.assertIsNotNone(runner.context)
        ctx = runner.context
        self.assertEqual(ctx.runtime_name, "echo1")
        self.assertEqual(ctx.input[0].content, "hello")
        self.assertEqual(ctx.params["max_steps"], 3)
        self.assertEqual(ctx.model.provider, "test")
        self.assertFalse(hasattr(ctx, "assign"))

        ack = next(item for item in sender.sent if item.WhichOneof("payload") == "task_ack")
        self.assertTrue(ack.task_ack.accepted)
        events = [item for item in sender.sent if item.WhichOneof("payload") == "event"]
        self.assertEqual(
            [event.event.type for event in events],
            [
                runtime_pb2.EventType.RUN_STATUS,
                runtime_pb2.EventType.MESSAGE_COMPLETED,
                runtime_pb2.EventType.RUN_STATUS,
            ],
        )
        self.assertEqual(events[1].event.data.fields["content"].string_value, "answer")
        self.assertEqual(events[-1].event.data.fields["status"].string_value, "completed")

    async def test_waiting_status_is_accepted(self) -> None:
        sender = FakeSender()
        runner = WaitingRunner()
        manager = RunManager(
            sender,
            ManagerConfig(runtime_name="echo1"),
            runner,
        )
        await manager.handle(assign_envelope(session_id="session-1"))
        await self.wait_done(manager)

        events = [
            item for item in sender.sent
            if item.WhichOneof("payload") == "event"
            and item.event.type == runtime_pb2.EventType.RUN_STATUS
        ]
        statuses = [event.event.data.fields["status"].string_value for event in events]
        self.assertEqual(statuses, ["running", "waiting"])
        self.assertEqual(manager.running_count(), 0)

    async def test_waiting_without_checkpoint_fails(self) -> None:
        sender = FakeSender()
        runner = CapturingRunner("waiting")
        manager = RunManager(
            sender,
            ManagerConfig(runtime_name="echo1"),
            runner,
        )
        await manager.handle(assign_envelope())
        await self.wait_done(manager)

        events = [
            item for item in sender.sent
            if item.WhichOneof("payload") == "event"
            and item.event.type == runtime_pb2.EventType.RUN_STATUS
        ]
        self.assertEqual(events[-1].event.data.fields["status"].string_value, "failed")
        self.assertIn(
            "checkpoint",
            events[-1].event.data.fields["reason"].string_value,
        )

    async def test_capacity_rejects_second_assignment(self) -> None:
        sender = FakeSender()
        started = asyncio.Event()
        release = asyncio.Event()

        async def runner(ctx):
            started.set()
            await release.wait()
            return "completed"

        manager = RunManager(
            sender,
            ManagerConfig(runtime_name="echo1", max_concurrent_runs=1),
            runner,
        )
        await manager.handle(assign_envelope(run_id="run-1"))
        await asyncio.wait_for(started.wait(), 1)
        await manager.handle(assign_envelope(run_id="run-2"))
        ack = next(
            item for item in sender.sent
            if item.WhichOneof("payload") == "task_ack"
            and not item.task_ack.accepted
        )
        self.assertEqual(ack.task_ack.reason, "capacity")
        release.set()
        await self.wait_done(manager)

    async def test_cancel_sets_context_cancelled(self) -> None:
        sender = FakeSender()
        started = asyncio.Event()
        release = asyncio.Event()

        async def runner(ctx):
            started.set()
            await release.wait()
            return "completed"

        manager = RunManager(
            sender,
            ManagerConfig(runtime_name="echo1"),
            runner,
        )
        await manager.handle(assign_envelope())
        await asyncio.wait_for(started.wait(), 1)
        cancel = new_envelope(
            runtime_pb2.Cancel(reason="user"),
            "cancel",
            run_id="run-1",
            attempt=1,
            lease_epoch=1,
        )
        await manager.handle(cancel)
        await self.wait_done(manager)
        events = [item for item in sender.sent if item.WhichOneof("payload") == "event"]
        self.assertEqual(events[-1].event.data.fields["status"].string_value, "cancelled")

    async def test_invalid_status_becomes_failed(self) -> None:
        sender = FakeSender()
        runner = CapturingRunner("unknown")
        manager = RunManager(
            sender,
            ManagerConfig(runtime_name="echo1"),
            runner,
        )
        await manager.handle(assign_envelope())
        await self.wait_done(manager)
        events = [item for item in sender.sent if item.WhichOneof("payload") == "event"]
        self.assertEqual(
            [event.event.type for event in events],
            [
                runtime_pb2.EventType.RUN_STATUS,
                runtime_pb2.EventType.MESSAGE_COMPLETED,
                runtime_pb2.EventType.ERROR,
                runtime_pb2.EventType.RUN_STATUS,
            ],
        )
        self.assertEqual(events[-1].event.data.fields["status"].string_value, "failed")
        self.assertIn("invalid status", events[-1].event.data.fields["reason"].string_value)
        self.assertEqual(events[-2].event.data.fields["code"].string_value, "INVALID_RUNTIME_STATUS")

    async def test_uncaught_exception_emits_error_and_failed(self) -> None:
        sender = FakeSender()

        async def runner(ctx):
            raise RuntimeError("boom")

        manager = RunManager(
            sender,
            ManagerConfig(runtime_name="echo1"),
            runner,
        )
        await manager.handle(assign_envelope())
        await self.wait_done(manager)
        events = [item for item in sender.sent if item.WhichOneof("payload") == "event"]
        self.assertEqual(
            [event.event.type for event in events],
            [
                runtime_pb2.EventType.RUN_STATUS,
                runtime_pb2.EventType.ERROR,
                runtime_pb2.EventType.RUN_STATUS,
            ],
        )
        self.assertEqual(events[-2].event.data.fields["code"].string_value, "RUNTIME_ERROR")
        self.assertEqual(events[-1].event.data.fields["status"].string_value, "failed")


if __name__ == "__main__":
    unittest.main()
