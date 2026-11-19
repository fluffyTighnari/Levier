"""React runtime tool router tests."""

from __future__ import annotations

import unittest

from middleware.runtime.v1 import runtime_pb2

from levier_runtime import McpBinding
from levier_runtime.context import RunContext
from levier_runtime.events import EventWriter
from levier_runtime.types import Timeouts

from levier_harness.llm.base import ToolCall, ToolSpec
from levier_harness.mcp import McpRuntimeError, McpSessionInfo
from levier_harness.tools.base import ToolResult
from levier_harness.tools.router import ToolRouter


class FakeChannel:
    def __init__(self) -> None:
        self.sent: list[runtime_pb2.Envelope] = []
        self.requests: list[runtime_pb2.ToolCallRequest] = []

    async def send(self, envelope: runtime_pb2.Envelope) -> None:
        self.sent.append(envelope)

    async def request(self, payload, field, **kwargs):
        self.requests.append(payload)
        return runtime_pb2.Envelope(
            tool_call_result=runtime_pb2.ToolCallResult(ok=True, result_json="bridge"),
        )


class FakeMcp:
    def __init__(self, *, error: Exception | None = None) -> None:
        self.error = error
        self.opened: list[McpBinding] = []
        self.calls: list[tuple[str, str, str, int]] = []
        self.closed: list[str] = []

    async def open(self, binding: McpBinding) -> McpSessionInfo:
        if self.error is not None:
            raise self.error
        self.opened.append(binding)
        return McpSessionInfo(
            session_id="session-1",
            tools=[ToolSpec(name="mcp__math__add", input_schema_json="{}")],
        )

    async def call(self, session_id: str, tool_name: str, arguments_json: str, timeout_ms: int) -> ToolResult:
        self.calls.append((session_id, tool_name, arguments_json, timeout_ms))
        return ToolResult(ok=True, result_json="3")

    async def close_session(self, session_id: str) -> None:
        self.closed.append(session_id)


def make_ctx(channel: FakeChannel, *bindings: McpBinding) -> RunContext:
    return RunContext(
        runtime_name="react1",
        run_id="run-1",
        app_id="app-1",
        input_messages=[],
        params={},
        model=None,
        tools=[],
        mcp_bindings=list(bindings),
        timeouts=Timeouts(),
        session_id=None,
        resume_from=None,
        trace_id="trace-1",
        channel=channel,
        events=EventWriter(channel, "run-1", 1, 1),
        attempt=1,
        lease_epoch=1,
    )


class ToolRouterTest(unittest.IsolatedAsyncioTestCase):
    async def test_open_and_route_mcp_and_bridge(self) -> None:
        channel = FakeChannel()
        mcp = FakeMcp()
        router = ToolRouter(mcp=mcp)
        ctx = make_ctx(channel, McpBinding(
            server_name="math",
            transport="stdio",
            command="fake",
        ))

        tools = await router.open(ctx)
        self.assertEqual([tool.name for tool in tools], ["mcp__math__add"])

        mcp_result = await router.call(
            ctx,
            ToolCall(call_id="c1", name="mcp__math__add", arguments_json="{}", invocation_id="i1"),
            1000,
        )
        self.assertTrue(mcp_result.ok)
        self.assertEqual(mcp.calls[0][0], "session-1")

        bridge_result = await router.call(
            ctx,
            ToolCall(call_id="c2", name="local_tool", arguments_json='{"x":1}', invocation_id="i2"),
            1000,
        )
        self.assertTrue(bridge_result.ok)
        self.assertEqual(bridge_result.result_json, "bridge")
        self.assertEqual(channel.requests[0].tool_name, "local_tool")

        await router.close(ctx)
        self.assertEqual(mcp.closed, ["session-1"])

    async def test_open_failure_emits_error_and_continues(self) -> None:
        channel = FakeChannel()
        mcp = FakeMcp(error=McpRuntimeError("MCP_CONNECT_FAILED", "boom", retryable=True))
        router = ToolRouter(mcp=mcp)
        ctx = make_ctx(channel, McpBinding(
            server_name="math",
            transport="stdio",
            command="fake",
        ))

        tools = await router.open(ctx)
        self.assertEqual(tools, [])
        events = [
            envelope.event for envelope in channel.sent
            if envelope.WhichOneof("payload") == "event"
        ]
        self.assertEqual(len(events), 1)
        self.assertEqual(events[0].data.fields["code"].string_value, "MCP_CONNECT_FAILED")
        self.assertTrue(events[0].data.fields["retryable"].bool_value)

    async def test_mcp_unavailable(self) -> None:
        channel = FakeChannel()
        router = ToolRouter(mcp=None)
        ctx = make_ctx(channel)
        result = await router.call(
            ctx,
            ToolCall(call_id="c1", name="mcp__math__add", arguments_json="{}", invocation_id="i1"),
            1000,
        )
        self.assertFalse(result.ok)
        self.assertEqual(result.error_code, "MCP_RUNTIME_UNAVAILABLE")
        self.assertTrue(result.retryable)


if __name__ == "__main__":
    unittest.main()
