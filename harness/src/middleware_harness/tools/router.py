"""Middleware — tool source router.

Routes MCP tool names to the MCP runtime and other tools to the middleware
tool bridge.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING, Sequence

from middleware.runtime.v1 import runtime_pb2

from middleware_harness.llm.base import ToolCall, ToolSpec
from middleware_harness.mcp.client import McpRuntimeClient, McpRuntimeError
from middleware_harness.tools.base import ToolExecutor, ToolResult

if TYPE_CHECKING:
    from middleware_harness.runtime.run import RunContext


class ToolRouter:
    """Open MCP sessions and dispatch each tool call to the right source."""

    def __init__(
        self,
        bridge: ToolExecutor | None = None,
        mcp: McpRuntimeClient | None = None,
        logger: logging.Logger | None = None,
    ) -> None:
        self.bridge = bridge
        self.mcp = mcp
        self.logger = logger or logging.getLogger("middleware_harness.tools")

    async def open(self, ctx: "RunContext") -> Sequence[ToolSpec]:
        """Open every MCP binding and return its namespaced tools."""
        tools: list[ToolSpec] = []
        if self.mcp is None:
            return tools
        for binding in ctx.assign.mcp_bindings:
            try:
                session = await self.mcp.open(binding)
            except McpRuntimeError as exc:
                await self._emit_error(ctx, exc.code, str(exc), exc.retryable)
                continue
            except Exception as exc:
                await self._emit_error(ctx, "MCP_RUNTIME_ERROR", str(exc), True)
                continue

            if not session.session_id:
                await self._emit_error(
                    ctx,
                    "MCP_RUNTIME_ERROR",
                    f"MCP server {binding.server_name!r} returned no session",
                    True,
                )
                continue
            ctx.tool_sessions.append(session.session_id)
            for tool in session.tools:
                if tool.name in ctx.tool_routes:
                    self.logger.warning("duplicate MCP tool ignored", extra={
                        "run_id": ctx.run_id,
                        "tool_name": tool.name,
                    })
                    continue
                ctx.tool_routes[tool.name] = session.session_id
                tools.append(tool)
        return tools

    async def call(
        self,
        ctx: "RunContext",
        call: ToolCall,
        timeout_ms: int,
    ) -> ToolResult:
        """Dispatch one tool call."""
        if call.name in ctx.tool_routes or call.name.startswith("mcp__"):
            return await self._call_mcp(ctx, call, timeout_ms)
        if self.bridge is None:
            return ToolResult(
                ok=False,
                error_code="TOOL_NOT_AVAILABLE",
                error_message="middleware tool bridge is not configured",
            )
        return await self.bridge(ctx, call, timeout_ms)

    async def close(self, ctx: "RunContext") -> None:
        """Close every MCP session opened for this run."""
        sessions = list(ctx.tool_sessions)
        ctx.tool_sessions.clear()
        ctx.tool_routes.clear()
        if self.mcp is None:
            return
        for session_id in sessions:
            try:
                await self.mcp.close_session(session_id)
            except McpRuntimeError as exc:
                self.logger.warning("close MCP session failed", extra={
                    "run_id": ctx.run_id,
                    "error": str(exc),
                })
            except Exception as exc:
                self.logger.warning("close MCP session failed", extra={
                    "run_id": ctx.run_id,
                    "error": str(exc),
                })

    async def _call_mcp(
        self,
        ctx: "RunContext",
        call: ToolCall,
        timeout_ms: int,
    ) -> ToolResult:
        if self.mcp is None:
            return ToolResult(
                ok=False,
                error_code="MCP_RUNTIME_UNAVAILABLE",
                error_message="MCP runtime is not configured",
                retryable=True,
            )
        session_id = ctx.tool_routes.get(call.name)
        if not session_id:
            return ToolResult(
                ok=False,
                error_code="TOOL_NOT_FOUND",
                error_message=f"MCP tool {call.name!r} is not open for this run",
            )
        try:
            return await self.mcp.call(
                session_id,
                call.name,
                call.arguments_json,
                timeout_ms,
            )
        except McpRuntimeError as exc:
            return ToolResult(
                ok=False,
                error_code=exc.code,
                error_message=str(exc),
                retryable=exc.retryable,
            )

    async def _emit_error(
        self,
        ctx: "RunContext",
        code: str,
        message: str,
        retryable: bool,
    ) -> None:
        await ctx.emit(
            runtime_pb2.EventType.ERROR,
            {"code": code, "message": message, "retryable": retryable},
        )
