"""Middleware — tool executor interface.

The ReAct loop only depends on this interface. The RuntimeChannel bridge and
MCP providers arrive in P3-29.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol

from middleware_harness.llm.base import ToolCall
from middleware_harness.runtime.run import RunContext


@dataclass
class ToolResult:
    """Result of one tool invocation."""

    ok: bool
    result_json: str = ""
    error_code: str = ""
    error_message: str = ""
    retryable: bool = False


class ToolExecutor(Protocol):
    """Executes one tool call for a run."""

    async def __call__(
        self,
        ctx: RunContext,
        call: ToolCall,
        timeout_ms: int,
    ) -> ToolResult:
        """Execute the call and return a structured result."""


class UnavailableToolExecutor:
    """Default executor used until the channel bridge exists."""

    async def __call__(
        self,
        ctx: RunContext,
        call: ToolCall,
        timeout_ms: int,
    ) -> ToolResult:
        return ToolResult(
            ok=False,
            error_code="TOOL_NOT_AVAILABLE",
            error_message="tool execution is not wired yet",
        )
