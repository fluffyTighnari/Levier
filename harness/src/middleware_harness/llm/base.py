"""Middleware — LLM provider interface.

Defines the minimal provider contract used by the ReAct strategy. Concrete
providers, streaming deltas, and provider-specific error mapping arrive in
P3-28.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Protocol, Sequence

from middleware.runtime.v1 import runtime_pb2


@dataclass(frozen=True)
class ToolSpec:
    """One tool definition exposed to the model."""

    name: str
    description: str = ""
    input_schema_json: str = ""

    @classmethod
    def from_manifest(cls, manifest: runtime_pb2.ToolManifest) -> "ToolSpec":
        """Convert a TaskAssign tool manifest into a provider-facing spec."""
        return cls(
            name=manifest.name,
            description=manifest.description,
            input_schema_json=manifest.input_schema_json,
        )


@dataclass(frozen=True)
class ToolCall:
    """A tool invocation requested by the model."""

    call_id: str
    name: str
    arguments_json: str = ""
    invocation_id: str = ""


@dataclass
class Message:
    """One conversation message passed to the provider."""

    role: str
    content: str = ""
    name: str = ""
    tool_call_id: str = ""
    tool_calls: tuple[ToolCall, ...] = ()

    @classmethod
    def from_proto(cls, message: runtime_pb2.ChatMessage) -> "Message":
        """Convert a TaskAssign input message."""
        return cls(role=message.role, content=message.content, name=message.name)


@dataclass
class LLMResponse:
    """One provider completion."""

    content: str = ""
    tool_calls: list[ToolCall] = field(default_factory=list)
    usage: dict[str, int] = field(default_factory=dict)
    model: str = ""

    @property
    def has_output(self) -> bool:
        """Whether the response contains text or tool calls."""
        return bool(self.content or self.tool_calls)


class LLMProvider(Protocol):
    """Provider contract consumed by the ReAct strategy."""

    async def complete(
        self,
        messages: Sequence[Message],
        tools: Sequence[ToolSpec],
    ) -> LLMResponse:
        """Return one completion for the current conversation."""
