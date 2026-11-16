"""Middleware — LLM provider interface.

Defines the provider contract consumed by the ReAct strategy, including
per-run model configuration, streaming deltas, and normalized provider
errors.

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


@dataclass(frozen=True)
class ModelConfig:
    """Per-run model and credential settings."""

    provider: str = ""
    model: str = ""
    api_key: str = ""
    base_url: str = ""
    params: dict[str, str] = field(default_factory=dict)

    @classmethod
    def from_profile(cls, profile: runtime_pb2.ModelProfile) -> "ModelConfig":
        """Convert a TaskAssign model profile."""
        return cls(
            provider=profile.provider,
            model=profile.model,
            api_key=profile.api_key,
            base_url=profile.base_url,
            params=dict(profile.params),
        )


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


class LLMError(RuntimeError):
    """Normalized provider error."""

    def __init__(
        self,
        code: str,
        message: str,
        *,
        retryable: bool,
        status: int = 0,
    ) -> None:
        super().__init__(message)
        self.code = code
        self.retryable = retryable
        self.status = status


class DeltaCallback(Protocol):
    """Receives streamed text increments."""

    async def __call__(self, text: str) -> None:
        """Handle one text delta."""


class LLMProvider(Protocol):
    """Provider contract consumed by the ReAct strategy."""

    async def complete(
        self,
        messages: Sequence[Message],
        tools: Sequence[ToolSpec],
        model: ModelConfig,
        *,
        on_delta: DeltaCallback | None = None,
    ) -> LLMResponse:
        """Return one completion, optionally streaming text deltas."""
