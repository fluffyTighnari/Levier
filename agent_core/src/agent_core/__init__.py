"""Middleware — language-neutral agent core.

The core owns the loop, tool orchestration, events, and cancellation.
Transport, MW event mapping, tool bridges, and persistence live in adapters.

Copyright 2026 The Middleware Authors.
"""

from agent_core.events import (
    CoreEvent,
    ErrorEvent,
    MessageCompleted,
    MessageDelta,
    RunFinished,
    RunStarted,
    StepStarted,
    ToolFinished,
    ToolStarted,
    UsageEvent,
    event_to_dict,
)
from agent_core.errors import ProviderFailure
from agent_core.loop import AgentCore
from agent_core.protocols import AbortSignal, DeltaCallback, ModelProvider, ToolProvider
from agent_core.types import (
    LLMResponse,
    Message,
    ModelRef,
    RunResult,
    RunSpec,
    ToolCall,
    ToolResult,
    ToolSpec,
    Usage,
)

__all__ = [
    "AbortSignal",
    "AgentCore",
    "ProviderFailure",
    "CoreEvent",
    "DeltaCallback",
    "ErrorEvent",
    "LLMResponse",
    "Message",
    "MessageCompleted",
    "MessageDelta",
    "ModelProvider",
    "ModelRef",
    "RunFinished",
    "RunResult",
    "RunSpec",
    "RunStarted",
    "StepStarted",
    "ToolCall",
    "ToolFinished",
    "ToolProvider",
    "ToolResult",
    "ToolSpec",
    "ToolStarted",
    "Usage",
    "UsageEvent",
    "event_to_dict",
]
