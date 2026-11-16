"""Middleware — TaskAssign/Cancel/Reconcile lifecycle tests.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import asyncio
import unittest

from middleware.runtime.v1 import runtime_pb2

from levier_harness.channel.envelope import new_envelope, response_envelope
from levier_harness.runtime import ManagerConfig, NoopRunner, RunManager
from levier_harness.runtime.run import RunContext


class FakeSender:
    def __init__(self) -> None:
        self.sent: list[runtime_pb2.Envelope] = []

    async def send(self, envelope: runtime_pb2.Envelope) -> None:
        self.sent.append(envelope)

    async def send_response(self, request: runtime_pb2.Envelope, payload, field: str) -> None:
        self.sent.append(response_envelope(request, payload, field))


class BlockingRunner:
    def __init__(self) -> None:
        self.started = asyncio.Event()
        self.release = asyncio.Event()

    async def __call__(self, ctx: RunContext) -> str:
        self.started.set()
        await self.release.wait()
        return "completed"


def assign_envelope(run_id: str = "run-1", attempt: int = 1, epoch: int = 1, strategy: str = "echo") -> runtime_pb2.Envelope:
    assign = runtime_pb2.TaskAssign(
        run_id=run_id,
        app_id="app-1",
        strategy=strategy,
    )
    return new_envelope(assign, "task_assign", run_id=run_id, attempt=attempt, lease_epoch=epoch)


def event_status(envelope: runtime_pb2.Envelope) -> str:
    return envelope.event.data.fields["status"].string_value


class RunManagerTest(unittest.IsolatedAsyncioTestCase):
    async def asyncTearDown(self) -> None:
        if hasattr(self, "manager"):
            await self.manager.close()

    async def wait_until(self, predicate, timeout: float = 1.0) -> None:
        async def inner() -> None:
            while not predicate():
                await asyncio.sleep(0.005)

        await asyncio.wait_for(inner(), timeout)

    async def test_accept_and_complete(self) -> None:
        sender = FakeSender()
        self.manager = RunManager(sender, runner=NoopRunner())
        assign = assign_envelope()
        await self.manager.handle(assign)
        await self.wait_until(lambda: self.manager.running_count() == 0)

        ack = next(env for env in sender.sent if env.WhichOneof("payload") == "task_ack")
        self.assertTrue(ack.task_ack.accepted)
        self.assertEqual(ack.correlation_id, assign.msg_id)

        events = [env for env in sender.sent if env.WhichOneof("payload") == "event"]
        self.assertEqual([event_status(env) for env in events], ["running", "completed"])
        self.assertEqual([env.event.seq for env in events], [1, 2])

    async def test_unsupported_strategy_rejected(self) -> None:
        sender = FakeSender()
        self.manager = RunManager(sender)
        await self.manager.handle(assign_envelope(strategy="unknown"))
        ack = next(env for env in sender.sent if env.WhichOneof("payload") == "task_ack")
        self.assertFalse(ack.task_ack.accepted)
        self.assertIn("unsupported", ack.task_ack.reason)
        self.assertEqual(self.manager.running_count(), 0)

    async def test_capacity_rejected(self) -> None:
        sender = FakeSender()
        runner = BlockingRunner()
        self.manager = RunManager(sender, ManagerConfig(max_concurrent_runs=1), runner)
        await self.manager.handle(assign_envelope("run-1"))
        await asyncio.wait_for(runner.started.wait(), 1)
        await self.manager.handle(assign_envelope("run-2"))
        ack = next(
            env for env in sender.sent
            if env.WhichOneof("payload") == "task_ack" and not env.task_ack.accepted
        )
        self.assertEqual(ack.task_ack.reason, "capacity")
        runner.release.set()
        await self.wait_until(lambda: self.manager.running_count() == 0)

    async def test_duplicate_assign_is_acknowledged_without_duplicate_run(self) -> None:
        sender = FakeSender()
        runner = BlockingRunner()
        self.manager = RunManager(sender, runner=runner)
        await self.manager.handle(assign_envelope("run-1"))
        await asyncio.wait_for(runner.started.wait(), 1)
        await self.manager.handle(assign_envelope("run-1"))
        self.assertEqual(self.manager.running_count(), 1)
        accepted = [env for env in sender.sent if env.WhichOneof("payload") == "task_ack" and env.task_ack.accepted]
        self.assertEqual(len(accepted), 2)
        runner.release.set()
        await self.wait_until(lambda: self.manager.running_count() == 0)

    async def test_cancel_active_run(self) -> None:
        sender = FakeSender()
        runner = BlockingRunner()
        self.manager = RunManager(sender, runner=runner)
        assign = assign_envelope("run-cancel")
        await self.manager.handle(assign)
        await asyncio.wait_for(runner.started.wait(), 1)
        cancel = new_envelope(runtime_pb2.Cancel(reason="user"), "cancel", run_id="run-cancel", attempt=1, lease_epoch=1)
        await self.manager.handle(cancel)
        await self.wait_until(lambda: self.manager.running_count() == 0)
        events = [env for env in sender.sent if env.WhichOneof("payload") == "event"]
        self.assertEqual(event_status(events[-1]), "cancelled")

    async def test_close_cancels_active_runs(self) -> None:
        sender = FakeSender()
        runner = BlockingRunner()
        self.manager = RunManager(sender, runner=runner)
        await self.manager.handle(assign_envelope("run-close"))
        await asyncio.wait_for(runner.started.wait(), 1)
        await self.manager.close()
        self.assertEqual(self.manager.running_count(), 0)
        events = [env for env in sender.sent if env.WhichOneof("payload") == "event"]
        self.assertEqual(event_status(events[-1]), "cancelled")

    async def test_reconcile_cancels_runs(self) -> None:
        sender = FakeSender()
        runner = BlockingRunner()
        self.manager = RunManager(sender, runner=runner)
        await self.manager.handle(assign_envelope("run-reconcile"))
        await asyncio.wait_for(runner.started.wait(), 1)
        reconcile = new_envelope(
            runtime_pb2.Reconcile(cancel_run_ids=["run-reconcile"]),
            "reconcile",
        )
        await self.manager.handle(reconcile)
        await self.wait_until(lambda: self.manager.running_count() == 0)
        events = [env for env in sender.sent if env.WhichOneof("payload") == "event"]
        self.assertEqual(event_status(events[-1]), "cancelled")
