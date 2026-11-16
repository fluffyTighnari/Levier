"""Middleware — tool source router tests.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import unittest

from middleware.runtime.v1 import runtime_pb2

from levier_harness.llm.base import ToolCall, ToolSpec
from levier_harness.mcp import McpRuntimeError, McpSessionInfo
from levier_harness.runtime.run import RunContext
from levier_harness.tools.base import ToolResult
from levier_harness.tools.router import ToolRouter


class FakeEvents:
    def __init__(self) -> None:
        self.events: list[tuple[object, dict]] = []

    async def emit(self, event_type, data, *, ephemeral=False) -> None:
        self.events.append((event_type, data))


class FakeMcp:
    def __init__(self, *, error: Exception | None = None) -> None:
        self.error = error
        self.opened: list[runtime_pb2.McpBinding] = []
        self.calls: list[tuple[str, str, str, int]] = []
        self.closed: list[str] = []

    async def open(self, binding: runtime_pb2.McpBinding) -> McpSessionInfo:
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


class FakeBridge:
    def __init__(self) -> None:
        self.calls: list[ToolCall] = []

    async def __call__(self, ctx, call, timeout_ms):
        self.calls.append(call)
        return ToolResult(ok=True, result_json="bridge")


def make_ctx(*bindings: runtime_pb2.McpBinding) -> RunContext:
    assign = runtime_pb2.TaskAssign(
        run_id="run-1",
        app_id="app-1",
        strategy="react",
        mcp_bindings=list(bindings),
    )
    return RunContext(
        assign=assign,
        attempt=1,
        lease_epoch=1,
        events=FakeEvents(),
    )


class ToolRouterTest(unittest.IsolatedAsyncioTestCase):
    async def test_open_and_route_mcp_and_bridge(self) -> None:
        mcp = FakeMcp()
        bridge = FakeBridge()
        router = ToolRouter(bridge=bridge, mcp=mcp)
        ctx = make_ctx(runtime_pb2.McpBinding(
            server_name="math",
            transport="stdio",
            command="fake",
        ))

        tools = await router.open(ctx)
        self.assertEqual([tool.name for tool in tools], ["mcp__math__add"])
        self.assertIn("mcp__math__add", ctx.tool_routes)

        mcp_result = await router.call(
            ctx,
            ToolCall(call_id="c1", name="mcp__math__add", arguments_json="{}", invocation_id="i1"),
            1000,
        )
        self.assertTrue(mcp_result.ok)
        self.assertEqual(mcp.calls[0][0], "session-1")

        bridge_result = await router.call(
            ctx,
            ToolCall(call_id="c2", name="local_tool", arguments_json="{}", invocation_id="i2"),
            1000,
        )
        self.assertTrue(bridge_result.ok)
        self.assertEqual(bridge.calls[0].name, "local_tool")

        await router.close(ctx)
        self.assertEqual(mcp.closed, ["session-1"])
        self.assertEqual(ctx.tool_sessions, [])
        self.assertEqual(ctx.tool_routes, {})

    async def test_open_failure_emits_error_and_continues(self) -> None:
        mcp = FakeMcp(error=McpRuntimeError("MCP_CONNECT_FAILED", "boom", retryable=True))
        router = ToolRouter(bridge=FakeBridge(), mcp=mcp)
        ctx = make_ctx(runtime_pb2.McpBinding(
            server_name="math",
            transport="stdio",
            command="fake",
        ))

        tools = await router.open(ctx)
        self.assertEqual(tools, [])
        self.assertEqual(len(ctx.events.events), 1)
        event_type, data = ctx.events.events[0]
        self.assertEqual(data["code"], "MCP_CONNECT_FAILED")
        self.assertTrue(data["retryable"])

    async def test_mcp_unavailable(self) -> None:
        router = ToolRouter(bridge=FakeBridge(), mcp=None)
        ctx = make_ctx()
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
