"""Middleware — agent core event vocabulary.

Adapters map these events to Middleware events; the core never emits MW types.
Every event is a dataclass with a stable `type` string and `to_dict()`.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import time
from dataclasses import asdict, dataclass, field
from typing import Any, ClassVar, Union

from agent_core.types import ToolCall, Usage


def _now_ms() -> int:
    return int(time.time() * 1000)


class _Event:
    EVENT_TYPE: ClassVar[str] = ""

    def to_dict(self) -> dict[str, Any]:
        return {"type": self.EVENT_TYPE, **asdict(self)}


@dataclass(frozen=True)
class RunStarted(_Event):
    run_id: str
    ts_ms: int = field(default_factory=_now_ms)
    EVENT_TYPE: ClassVar[str] = "run_started"


@dataclass(frozen=True)
class StepStarted(_Event):
    run_id: str
    step: int
    ts_ms: int = field(default_factory=_now_ms)
    EVENT_TYPE: ClassVar[str] = "step_started"


@dataclass(frozen=True)
class MessageDelta(_Event):
    run_id: str
    step: int
    message_id: str
    delta: str
    ts_ms: int = field(default_factory=_now_ms)
    EVENT_TYPE: ClassVar[str] = "message_delta"


@dataclass(frozen=True)
class MessageCompleted(_Event):
    run_id: str
    step: int
    message_id: str
    content: str = ""
    tool_calls: tuple[ToolCall, ...] = ()
    model: str = ""
    ts_ms: int = field(default_factory=_now_ms)
    EVENT_TYPE: ClassVar[str] = "message_completed"


@dataclass(frozen=True)
class UsageEvent(_Event):
    run_id: str
    step: int
    usage: Usage = field(default_factory=Usage)
    ts_ms: int = field(default_factory=_now_ms)
    EVENT_TYPE: ClassVar[str] = "usage"


@dataclass(frozen=True)
class ToolStarted(_Event):
    run_id: str
    step: int
    invocation_id: str
    tool_name: str
    arguments_json: str = ""
    ts_ms: int = field(default_factory=_now_ms)
    EVENT_TYPE: ClassVar[str] = "tool_started"


@dataclass(frozen=True)
class ToolFinished(_Event):
    run_id: str
    step: int
    invocation_id: str
    tool_name: str
    ok: bool
    result_json: str = ""
    error_code: str = ""
    error_message: str = ""
    retryable: bool = False
    duration_ms: int = 0
    ts_ms: int = field(default_factory=_now_ms)
    EVENT_TYPE: ClassVar[str] = "tool_finished"


@dataclass(frozen=True)
class RunFinished(_Event):
    run_id: str
    status: str
    steps: int = 0
    ts_ms: int = field(default_factory=_now_ms)
    EVENT_TYPE: ClassVar[str] = "run_finished"


@dataclass(frozen=True)
class ErrorEvent(_Event):
    run_id: str
    code: str
    message: str
    step: int = 0
    retryable: bool = False
    ts_ms: int = field(default_factory=_now_ms)
    EVENT_TYPE: ClassVar[str] = "error"


CoreEvent = Union[
    RunStarted,
    StepStarted,
    MessageDelta,
    MessageCompleted,
    UsageEvent,
    ToolStarted,
    ToolFinished,
    RunFinished,
    ErrorEvent,
]


def event_to_dict(event: CoreEvent) -> dict[str, Any]:
    """Serialize any core event to a JSON-ready dict."""
    return event.to_dict()
