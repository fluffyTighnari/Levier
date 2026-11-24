"""Runtime SDK context operations tests."""

from __future__ import annotations

import json
import unittest

from middleware.runtime.v1 import runtime_pb2

from levier_runtime.context import RunContext, RuntimeUsageError
from levier_runtime.events import EventWriter
from levier_runtime.types import Message, Timeouts


class FakeChannel:
    def __init__(self, result: runtime_pb2.ToolCallResult | None = None) -> None:
        self.result = result or runtime_pb2.ToolCallResult(ok=True, result_json="42")
        self.sent: list[runtime_pb2.Envelope] = []
        self.requests: list[tuple[runtime_pb2.ToolCallRequest, dict]] = []

    async def send(self, envelope: runtime_pb2.Envelope) -> None:
        self.sent.append(envelope)

    async def request(self, payload, field, **kwargs):
        self.requests.append((payload, kwargs))
        return runtime_pb2.Envelope(tool_call_result=self.result)


def make_context(channel: FakeChannel, *, session_id: str = "") -> RunContext:
    return RunContext(
        runtime_name="test1",
        run_id="run-1",
        app_id="app-1",
        input_messages=[Message(role="user", content="hello")],
        params={"k": "v"},
        model=None,
        tools=[],
        mcp_bindings=[],
        timeouts=Timeouts(run_ms=1000, tool_ms=500),
        session_id=session_id or None,
        resume_from=None,
        trace_id="trace",
        channel=channel,
        events=EventWriter(channel, "run-1", 1, 1),
        attempt=1,
        lease_epoch=1,
    )


class ContextTest(unittest.IsolatedAsyncioTestCase):
    async def test_reply_uses_one_persistent_event(self) -> None:
        channel = FakeChannel()
        ctx = make_context(channel)
        await ctx.reply("hi")
        event = channel.sent[0].event
        self.assertEqual(event.type, runtime_pb2.EventType.MESSAGE_COMPLETED)
        self.assertEqual(event.seq, 1)
        self.assertEqual(event.data.fields["content"].string_value, "hi")
        self.assertTrue(event.data.fields["message_id"].string_value)

    async def test_call_tool_round_trip(self) -> None:
        channel = FakeChannel()
        ctx = make_context(channel)
        result = await ctx.call_tool("add", {"a": 1})
        self.assertTrue(result.ok)
        self.assertEqual(result.result_json, "42")
        request, kwargs = channel.requests[0]
        self.assertEqual(request.tool_name, "add")
        self.assertEqual(json.loads(request.arguments_json), {"a": 1})
        self.assertEqual(kwargs["run_id"], "run-1")
        events = [
            item.event for item in channel.sent
            if item.WhichOneof("payload") == "event"
        ]
        self.assertEqual(
            [event.type for event in events],
            [
                runtime_pb2.EventType.TOOL_CALL_BEGIN,
                runtime_pb2.EventType.TOOL_CALL_END,
            ],
        )
        self.assertEqual(
            events[1].data.fields["result_json"].string_value,
            "42",
        )

    async def test_error_emits_normalized_error_event(self) -> None:
        channel = FakeChannel()
        ctx = make_context(channel)
        await ctx.error("MODEL_TIMEOUT", "model timed out", retryable=True)
        event = channel.sent[0].event
        self.assertEqual(event.type, runtime_pb2.EventType.ERROR)
        self.assertEqual(event.data.fields["code"].string_value, "MODEL_TIMEOUT")
        self.assertTrue(event.data.fields["retryable"].bool_value)

    async def test_logger_carries_run_context(self) -> None:
        ctx = make_context(FakeChannel())
        self.assertEqual(ctx.logger.extra["run_id"], "run-1")
        self.assertEqual(ctx.logger.extra["app_id"], "app-1")
        self.assertEqual(ctx.logger.extra["runtime_name"], "test1")
        self.assertEqual(ctx.logger.extra["trace_id"], "trace")

    async def test_checkpoint_requires_session(self) -> None:
        ctx = make_context(FakeChannel())
        with self.assertRaises(RuntimeUsageError):
            await ctx.checkpoint(b"state")

    async def test_checkpoint_emits_checkpoint_and_event(self) -> None:
        channel = FakeChannel()
        ctx = make_context(channel, session_id="session-1")
        await ctx.reply("one")
        await ctx.checkpoint({"step": 1})
        checkpoint = next(
            item for item in channel.sent if item.WhichOneof("payload") == "checkpoint"
        )
        self.assertEqual(checkpoint.checkpoint.session_id, "session-1")
        self.assertEqual(checkpoint.checkpoint.event_seq_watermark, 1)
        saved = [
            item for item in channel.sent
            if item.WhichOneof("payload") == "event"
            and item.event.type == runtime_pb2.EventType.CHECKPOINT_SAVED
        ]
        self.assertEqual(len(saved), 1)

    async def test_wait_saves_checkpoint_and_returns_waiting(self) -> None:
        channel = FakeChannel()
        ctx = make_context(channel, session_id="session-1")
        status = await ctx.wait({"step": 2})
        self.assertEqual(status, "waiting")
        checkpoint = next(
            item for item in channel.sent if item.WhichOneof("payload") == "checkpoint"
        )
        self.assertEqual(checkpoint.checkpoint.state, b'{"step":2}')


if __name__ == "__main__":
    unittest.main()
