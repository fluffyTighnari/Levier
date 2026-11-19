"""A custom runtime conformance test using only levier_runtime."""

from __future__ import annotations

import asyncio
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

    async def request(self, payload, field, **kwargs):
        return runtime_pb2.Envelope(
            tool_call_result=runtime_pb2.ToolCallResult(ok=True, result_json='"ok"'),
        )


def assign_envelope() -> runtime_pb2.Envelope:
    assign = runtime_pb2.TaskAssign(
        run_id="run-custom",
        app_id="app-1",
        input=[runtime_pb2.ChatMessage(role="user", content="hello")],
        model=runtime_pb2.ModelProfile(provider="test", model="test-model"),
    )
    assign.params.update({"max_steps": 3})
    return new_envelope(
        assign,
        "task_assign",
        run_id="run-custom",
        attempt=1,
        lease_epoch=1,
    )


async def custom_run(ctx) -> str:
    """A hand-written loop, with no ReactAgent or ReactRuntime dependency."""
    await ctx.reply("working")

    result = await ctx.call_tool("echo", {"value": "ok"})
    if not result.ok:
        await ctx.error(result.error_code, result.error_message, retryable=result.retryable)
        return "failed"

    await ctx.emit("MESSAGE_DELTA", {"message_id": "m-1", "delta": "done"}, ephemeral=True)
    await ctx.reply("done")
    return "completed"


class CustomRuntimeConformanceTest(unittest.IsolatedAsyncioTestCase):
    async def test_custom_runner_gets_sdk_observability(self) -> None:
        sender = FakeSender()
        manager = RunManager(
            sender,
            ManagerConfig(runtime_name="custom1"),
            custom_run,
        )
        try:
            await manager.handle(assign_envelope())
            for _ in range(200):
                if manager.running_count() == 0:
                    break
                await asyncio.sleep(0.005)
        finally:
            await manager.close()

        events = [
            envelope.event for envelope in sender.sent
            if envelope.WhichOneof("payload") == "event"
        ]
        self.assertEqual(
            [event.type for event in events],
            [
                runtime_pb2.EventType.RUN_STATUS,
                runtime_pb2.EventType.MESSAGE_COMPLETED,
                runtime_pb2.EventType.TOOL_CALL_BEGIN,
                runtime_pb2.EventType.TOOL_CALL_END,
                runtime_pb2.EventType.MESSAGE_DELTA,
                runtime_pb2.EventType.MESSAGE_COMPLETED,
                runtime_pb2.EventType.RUN_STATUS,
            ],
        )
        self.assertTrue(events[3].data.fields["ok"].bool_value)
        self.assertTrue(events[4].ephemeral)
        self.assertEqual(events[-1].data.fields["status"].string_value, "completed")


if __name__ == "__main__":
    unittest.main()
