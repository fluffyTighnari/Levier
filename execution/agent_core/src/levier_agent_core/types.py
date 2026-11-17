"""Levier — agent core data types.

These are the only structures shared with adapters. They must not import
Levier, Harness, or MCP runtime code.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any


@dataclass(frozen=True)
class ToolSpec:
    """One tool exposed to the model."""

    name: str
    description: str = ""
    input_schema_json: str = "{}"


@dataclass(frozen=True)
class ToolCall:
    """One model-requested tool invocation."""

    call_id: str
    name: str
    arguments_json: str = ""
    invocation_id: str = ""


@dataclass
class Message:
    """One conversation message."""

    role: str
    content: str = ""
    name: str = ""
    tool_call_id: str = ""
    tool_calls: tuple[ToolCall, ...] = ()


@dataclass(frozen=True)
class ModelRef:
    """Provider/model selection.

    Credentials and endpoints belong to the provider adapter; the core never
    sees secrets.
    """

    provider: str = ""
    model: str = ""
    params: dict[str, str] = field(default_factory=dict)


@dataclass
class Usage:
    """Normalized token usage.

    Adapters map provider-specific keys (prompt_tokens/completion_tokens or
    input_tokens/output_tokens) into this shape.
    """

    input_tokens: int = 0
    output_tokens: int = 0
    total_tokens: int = 0
    cache_read_tokens: int = 0
    cache_write_tokens: int = 0

    def __post_init__(self) -> None:
        if self.total_tokens == 0:
            self.total_tokens = self.input_tokens + self.output_tokens

    def to_dict(self) -> dict[str, int]:
        return {
            "input_tokens": self.input_tokens,
            "output_tokens": self.output_tokens,
            "total_tokens": self.total_tokens,
            "cache_read_tokens": self.cache_read_tokens,
            "cache_write_tokens": self.cache_write_tokens,
        }


@dataclass
class LLMResponse:
    """One provider completion."""

    content: str = ""
    tool_calls: list[ToolCall] = field(default_factory=list)
    usage: Usage = field(default_factory=Usage)
    model: str = ""
    stop_reason: str = ""


@dataclass(frozen=True)
class ToolResult:
    """Normalized result of one tool call."""

    ok: bool
    result_json: str = ""
    error_code: str = ""
    error_message: str = ""
    retryable: bool = False
    duration_ms: int = 0


@dataclass
class RunSpec:
    """Everything one run attempt needs."""

    run_id: str = ""
    start_step: int = 1
    messages: list[Message] = field(default_factory=list)
    tools: list[ToolSpec] = field(default_factory=list)
    model: ModelRef = field(default_factory=ModelRef)
    max_steps: int = 8
    step_timeout_ms: int = 120_000
    tool_timeout_ms: int = 60_000
    session_id: str = ""
    metadata: dict[str, str] = field(default_factory=dict)


@dataclass
class RunResult:
    """Final outcome of one run attempt."""

    status: str
    messages: list[Message] = field(default_factory=list)
    steps: int = 0
    usage: Usage = field(default_factory=Usage)
    error_code: str = ""
    error_message: str = ""

    def to_dict(self) -> dict[str, Any]:
        return {
            "status": self.status,
            "steps": self.steps,
            "usage": self.usage.to_dict(),
            "error_code": self.error_code,
            "error_message": self.error_message,
        }
