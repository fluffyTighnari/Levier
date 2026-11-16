"""Middleware — agent core adapter protocols."""

from __future__ import annotations

from typing import Protocol, Sequence

from levier_agent_core.types import LLMResponse, Message, ModelRef, ToolCall, ToolResult, ToolSpec


class DeltaCallback(Protocol):
    """Receives streamed text increments."""

    async def __call__(self, delta: str) -> None:
        ...


class ModelProvider(Protocol):
    """Adapter-supplied model completion."""

    async def complete(
        self,
        messages: Sequence[Message],
        tools: Sequence[ToolSpec],
        model: ModelRef,
        *,
        on_delta: DeltaCallback | None = None,
    ) -> LLMResponse:
        ...


class ToolProvider(Protocol):
    """Adapter-supplied tool execution (local, global, or MCP)."""

    async def call(self, call: ToolCall) -> ToolResult:
        ...


class AbortSignal(Protocol):
    """Cancellation token shared by the adapter and the core."""

    def is_set(self) -> bool:
        ...
