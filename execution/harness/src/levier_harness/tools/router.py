"""Route MCP tools to the MCP runtime and all other tools to the SDK bridge.

This is a React-runtime helper, not part of the Runtime SDK. It deliberately
uses the plain `RunContext` exposed by `levier_runtime`.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from typing import Sequence

from levier_runtime import RunContext

from levier_harness.llm.base import ToolCall, ToolSpec
from levier_harness.mcp.client import McpRuntimeClient, McpRuntimeError
from levier_harness.tools.base import ToolResult

@dataclass
class _RunToolState:
    """Per-run MCP sessions and namespaced tool routes."""

    sessions: list[str] = field(default_factory=list)
    routes: dict[str, str] = field(default_factory=dict)


class ToolRouter:
    """Open MCP sessions and dispatch each tool call to the right source."""

    def __init__(
        self,
        mcp: McpRuntimeClient | None = None,
        logger: logging.Logger | None = None,
    ) -> None:
        self.mcp = mcp
        self.logger = logger or logging.getLogger("levier_harness.tools")
        self._states: dict[str, _RunToolState] = {}

    async def open(self, ctx: RunContext) -> Sequence[ToolSpec]:
        """Open every MCP binding and return its namespaced tools."""
        state = self._states.setdefault(ctx.run_id, _RunToolState())
        tools: list[ToolSpec] = []
        if self.mcp is None:
            return tools
        for binding in ctx.mcp_bindings:
            try:
                session = await self.mcp.open(binding)
            except McpRuntimeError as exc:
                await self._emit_error(ctx, exc.code, str(exc), exc.retryable)
                continue
            except Exception as exc:  # noqa: BLE001 - runtime boundary
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
            state.sessions.append(session.session_id)
            for tool in session.tools:
                if tool.name in state.routes:
                    self.logger.warning("duplicate MCP tool ignored", extra={
                        "run_id": ctx.run_id,
                        "tool_name": tool.name,
                    })
                    continue
                state.routes[tool.name] = session.session_id
                tools.append(tool)
        return tools

    async def call(
        self,
        ctx: RunContext,
        call: ToolCall,
        timeout_ms: int,
    ) -> ToolResult:
        """Dispatch one tool call."""
        state = self._states.get(ctx.run_id)
        if call.name.startswith("mcp__") or (
            state is not None and call.name in state.routes
        ):
            return await self._call_mcp(
                ctx,
                state or _RunToolState(),
                call,
                timeout_ms,
            )

        result = await ctx.call_tool(
            call.name,
            call.arguments_json,
            timeout_ms=timeout_ms,
        )
        return ToolResult(
            ok=result.ok,
            result_json=result.result_json,
            error_code=result.error_code,
            error_message=result.error_message,
            retryable=result.retryable,
        )

    async def close(self, ctx: RunContext) -> None:
        """Close every MCP session opened for this run."""
        state = self._states.pop(ctx.run_id, None)
        if state is None:
            return
        sessions = list(state.sessions)
        state.sessions.clear()
        state.routes.clear()
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
            except Exception as exc:  # noqa: BLE001 - runtime boundary
                self.logger.warning("close MCP session failed", extra={
                    "run_id": ctx.run_id,
                    "error": str(exc),
                })

    async def _call_mcp(
        self,
        ctx: RunContext,
        state: _RunToolState,
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
        session_id = state.routes.get(call.name)
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
        ctx: RunContext,
        code: str,
        message: str,
        retryable: bool,
    ) -> None:
        await ctx.emit(
            "ERROR",
            {"code": code, "message": message, "retryable": retryable},
        )
