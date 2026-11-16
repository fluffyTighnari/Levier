"""Middleware — tool execution interfaces.

The ReAct loop depends on a ToolProvider so local/global tools and MCP tools
can share one dispatch path. Concrete bridges live in sibling modules.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import TYPE_CHECKING, Protocol, Sequence

from levier_harness.llm.base import ToolCall, ToolSpec

if TYPE_CHECKING:
    from levier_harness.runtime.run import RunContext


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
        ctx: "RunContext",
        call: ToolCall,
        timeout_ms: int,
    ) -> ToolResult:
        """Execute the call and return a structured result."""


class ToolProvider(Protocol):
    """Opens, calls, and closes the tool sources for one run."""

    async def open(self, ctx: "RunContext") -> Sequence[ToolSpec]:
        """Return additional tools for this run."""

    async def call(
        self,
        ctx: "RunContext",
        call: ToolCall,
        timeout_ms: int,
    ) -> ToolResult:
        """Execute one tool call."""

    async def close(self, ctx: "RunContext") -> None:
        """Release any per-run resources."""


class UnavailableToolExecutor:
    """Default executor used when no tool bridge is configured."""

    async def __call__(
        self,
        ctx: "RunContext",
        call: ToolCall,
        timeout_ms: int,
    ) -> ToolResult:
        return ToolResult(
            ok=False,
            error_code="TOOL_NOT_AVAILABLE",
            error_message="tool execution is not wired",
        )


class ExecutorToolProvider:
    """Adapt a simple ToolExecutor to the ToolProvider interface."""

    def __init__(self, executor: ToolExecutor) -> None:
        self.executor = executor

    async def open(self, ctx: "RunContext") -> Sequence[ToolSpec]:
        return ()

    async def call(
        self,
        ctx: "RunContext",
        call: ToolCall,
        timeout_ms: int,
    ) -> ToolResult:
        return await self.executor(ctx, call, timeout_ms)

    async def close(self, ctx: "RunContext") -> None:
        return None


class UnavailableToolProvider:
    """Provider used when no tool execution is configured."""

    async def open(self, ctx: "RunContext") -> Sequence[ToolSpec]:
        return ()

    async def call(
        self,
        ctx: "RunContext",
        call: ToolCall,
        timeout_ms: int,
    ) -> ToolResult:
        return ToolResult(
            ok=False,
            error_code="TOOL_NOT_AVAILABLE",
            error_message="tool execution is not wired",
        )

    async def close(self, ctx: "RunContext") -> None:
        return None


def as_tool_provider(value: object | None) -> ToolProvider:
    """Normalize a ToolProvider or ToolExecutor into a ToolProvider."""
    if value is None:
        return UnavailableToolProvider()
    open_method = getattr(value, "open", None)
    call_method = getattr(value, "call", None)
    close_method = getattr(value, "close", None)
    if callable(open_method) and callable(call_method) and callable(close_method):
        return value  # type: ignore[return-value]
    if callable(value):
        return ExecutorToolProvider(value)
    return UnavailableToolProvider()
