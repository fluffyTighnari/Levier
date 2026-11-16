"""Middleware — RuntimeChannel tool bridge.

Sends ToolCallRequest envelopes to middleware and resolves the correlated
ToolCallResult.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import asyncio
from typing import Protocol

from middleware.runtime.v1 import runtime_pb2

from levier_harness.llm.base import ToolCall
from levier_harness.runtime.run import RunContext
from levier_harness.tools.base import ToolResult

_DEFAULT_TIMEOUT_MS = 60_000


class RequestSender(Protocol):
    """Minimal channel request interface used by the bridge."""

    async def request(
        self,
        payload,
        field: str,
        *,
        run_id: str = "",
        attempt: int = 0,
        lease_epoch: int = 0,
        timeout: float = 30.0,
    ) -> runtime_pb2.Envelope:
        """Send a request and wait for its correlated response."""


class ToolBridge:
    """Execute non-MCP tools through the middleware gateway."""

    def __init__(
        self,
        channel: RequestSender,
        *,
        default_timeout_ms: int = _DEFAULT_TIMEOUT_MS,
    ) -> None:
        self.channel = channel
        self.default_timeout_ms = default_timeout_ms

    async def __call__(
        self,
        ctx: RunContext,
        call: ToolCall,
        timeout_ms: int,
    ) -> ToolResult:
        timeout = timeout_ms if timeout_ms > 0 else self.default_timeout_ms
        request = runtime_pb2.ToolCallRequest(
            invocation_id=call.invocation_id,
            tool_name=call.name,
            arguments_json=call.arguments_json,
            timeout_ms=timeout,
        )
        try:
            envelope = await self.channel.request(
                request,
                "tool_call_request",
                run_id=ctx.run_id,
                attempt=ctx.attempt,
                lease_epoch=ctx.lease_epoch,
                timeout=timeout / 1000,
            )
        except asyncio.TimeoutError:
            return ToolResult(
                ok=False,
                error_code="TOOL_TIMEOUT",
                error_message=f"tool {call.name!r} timed out",
                retryable=True,
            )
        except Exception as exc:
            return ToolResult(
                ok=False,
                error_code="CHANNEL_UNAVAILABLE",
                error_message=str(exc),
                retryable=True,
            )

        result = envelope.tool_call_result
        if result.ok:
            return ToolResult(ok=True, result_json=result.result_json)
        return ToolResult(
            ok=False,
            error_code=result.error.code or "TOOL_EXEC_ERROR",
            error_message=result.error.message,
            retryable=result.error.retryable,
        )
