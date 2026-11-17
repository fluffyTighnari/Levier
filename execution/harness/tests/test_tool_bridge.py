"""Levier — RuntimeChannel tool bridge tests.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import asyncio
import unittest
from typing import Any

from middleware.runtime.v1 import runtime_pb2

from levier_harness.llm.base import ToolCall
from levier_harness.runtime.run import RunContext
from levier_harness.tools.bridge import ToolBridge


class FakeEvents:
    async def emit(self, event_type, data, *, ephemeral=False) -> None:
        return None


class FakeChannel:
    def __init__(self, result: runtime_pb2.ToolCallResult | Exception) -> None:
        self.result = result
        self.requests: list[tuple[Any, str, dict[str, Any]]] = []

    async def request(self, payload, field, **kwargs):
        self.requests.append((payload, field, kwargs))
        if isinstance(self.result, Exception):
            raise self.result
        return runtime_pb2.Envelope(tool_call_result=self.result)


def make_ctx() -> RunContext:
    assign = runtime_pb2.TaskAssign(run_id="run-1", app_id="app-1", strategy="react")
    return RunContext(
        assign=assign,
        attempt=2,
        lease_epoch=7,
        events=FakeEvents(),
    )


class ToolBridgeTest(unittest.IsolatedAsyncioTestCase):
    async def test_success(self) -> None:
        channel = FakeChannel(runtime_pb2.ToolCallResult(ok=True, result_json="42"))
        bridge = ToolBridge(channel)
        result = await bridge(
            make_ctx(),
            ToolCall(call_id="call-1", name="add", arguments_json="{}", invocation_id="inv-1"),
            5000,
        )
        self.assertTrue(result.ok)
        self.assertEqual(result.result_json, "42")
        request, field, kwargs = channel.requests[0]
        self.assertEqual(field, "tool_call_request")
        self.assertEqual(request.invocation_id, "inv-1")
        self.assertEqual(request.tool_name, "add")
        self.assertEqual(kwargs["run_id"], "run-1")
        self.assertEqual(kwargs["attempt"], 2)
        self.assertEqual(kwargs["lease_epoch"], 7)
        self.assertEqual(kwargs["timeout"], 5.0)

    async def test_error_result(self) -> None:
        channel = FakeChannel(runtime_pb2.ToolCallResult(
            ok=False,
            error=runtime_pb2.ErrorInfo(
                code="TOOL_TIMEOUT",
                message="too slow",
                retryable=True,
            ),
        ))
        result = await ToolBridge(channel)(
            make_ctx(),
            ToolCall(call_id="call-1", name="add", invocation_id="inv-1"),
            1000,
        )
        self.assertFalse(result.ok)
        self.assertEqual(result.error_code, "TOOL_TIMEOUT")
        self.assertTrue(result.retryable)

    async def test_timeout(self) -> None:
        channel = FakeChannel(asyncio.TimeoutError())
        result = await ToolBridge(channel)(
            make_ctx(),
            ToolCall(call_id="call-1", name="add", invocation_id="inv-1"),
            10,
        )
        self.assertFalse(result.ok)
        self.assertEqual(result.error_code, "TOOL_TIMEOUT")
        self.assertTrue(result.retryable)

    async def test_channel_failure(self) -> None:
        channel = FakeChannel(RuntimeError("disconnected"))
        result = await ToolBridge(channel)(
            make_ctx(),
            ToolCall(call_id="call-1", name="add", invocation_id="inv-1"),
            1000,
        )
        self.assertFalse(result.ok)
        self.assertEqual(result.error_code, "CHANNEL_UNAVAILABLE")
        self.assertTrue(result.retryable)


if __name__ == "__main__":
    unittest.main()
