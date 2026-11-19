"""Levier — longterm checkpoint encoding.

Checkpoint payloads are opaque to middleware. The React Runtime owns the schema,
versioning, and restore compatibility checks.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any

from levier_react_runtime.llm.base import Message, ToolCall

CHECKPOINT_VERSION = 1


class CheckpointError(ValueError):
    """Raised when checkpoint bytes are invalid or unsupported."""


@dataclass
class CheckpointState:
    """React-Runtime-owned longterm state."""

    session_id: str
    run_id: str
    attempt: int
    runtime_name: str
    step: int = 0
    messages: list[Message] = field(default_factory=list)
    last_event_attempt: int = 0
    last_event_seq: int = 0
    created_at: str = ""
    version: int = CHECKPOINT_VERSION

    def encode(self) -> bytes:
        """Encode the checkpoint as UTF-8 JSON."""
        payload = {
            "version": self.version,
            "session_id": self.session_id,
            "run_id": self.run_id,
            "attempt": self.attempt,
            "runtime_name": self.runtime_name,
            "step": self.step,
            "messages": [_message_to_dict(message) for message in self.messages],
            "last_event_attempt": self.last_event_attempt,
            "last_event_seq": self.last_event_seq,
            "created_at": self.created_at or _now(),
        }
        return json.dumps(payload, ensure_ascii=False, separators=(",", ":")).encode("utf-8")

    @classmethod
    def decode(cls, data: bytes) -> "CheckpointState":
        """Decode checkpoint bytes."""
        if not data:
            raise CheckpointError("checkpoint is empty")
        try:
            payload = json.loads(data.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as exc:
            raise CheckpointError("checkpoint is not valid JSON") from exc
        if not isinstance(payload, dict):
            raise CheckpointError("checkpoint must be an object")
        version = _int(payload.get("version"), "version")
        if version != CHECKPOINT_VERSION:
            raise CheckpointError(f"unsupported checkpoint version {version}")

        session_id = _str(payload.get("session_id"), "session_id", allow_empty=False)
        run_id = _str(payload.get("run_id"), "run_id", allow_empty=False)
        runtime_name = _str(payload.get("runtime_name"), "runtime_name", allow_empty=False)
        attempt = _int(payload.get("attempt"), "attempt", minimum=1)
        step = _int(payload.get("step"), "step", minimum=0)
        last_event_attempt = _int(payload.get("last_event_attempt"), "last_event_attempt", minimum=0)
        last_event_seq = _int(payload.get("last_event_seq"), "last_event_seq", minimum=0)
        created_at = _str(payload.get("created_at"), "created_at", allow_empty=True)

        raw_messages = payload.get("messages")
        if not isinstance(raw_messages, list):
            raise CheckpointError("messages must be a list")
        messages = [_message_from_dict(item) for item in raw_messages]
        return cls(
            session_id=session_id,
            run_id=run_id,
            attempt=attempt,
            runtime_name=runtime_name,
            step=step,
            messages=messages,
            last_event_attempt=last_event_attempt,
            last_event_seq=last_event_seq,
            created_at=created_at,
            version=version,
        )


def capture(
    *,
    session_id: str,
    run_id: str,
    attempt: int,
    runtime_name: str,
    step: int,
    messages: list[Message],
    last_event_seq: int,
    last_event_attempt: int | None = None,
) -> CheckpointState:
    """Capture a checkpoint at a safe point."""
    return CheckpointState(
        session_id=session_id,
        run_id=run_id,
        attempt=attempt,
        runtime_name=runtime_name,
        step=step,
        messages=list(messages),
        last_event_attempt=attempt if last_event_attempt is None else last_event_attempt,
        last_event_seq=last_event_seq,
        created_at=_now(),
    )


def _message_to_dict(message: Message) -> dict[str, Any]:
    payload: dict[str, Any] = {
        "role": message.role,
        "content": message.content,
    }
    if message.name:
        payload["name"] = message.name
    if message.tool_call_id:
        payload["tool_call_id"] = message.tool_call_id
    if message.tool_calls:
        payload["tool_calls"] = [
            {
                "call_id": call.call_id,
                "name": call.name,
                "arguments_json": call.arguments_json,
            }
            for call in message.tool_calls
        ]
    return payload


def _message_from_dict(value: Any) -> Message:
    if not isinstance(value, dict):
        raise CheckpointError("message must be an object")
    role = _str(value.get("role"), "message.role", allow_empty=False)
    content = _str(value.get("content"), "message.content", allow_empty=True)
    name = _str(value.get("name"), "message.name", allow_empty=True)
    tool_call_id = _str(value.get("tool_call_id"), "message.tool_call_id", allow_empty=True)
    raw_calls = value.get("tool_calls") or []
    if not isinstance(raw_calls, list):
        raise CheckpointError("message.tool_calls must be a list")
    calls = tuple(_tool_call_from_dict(item) for item in raw_calls)
    return Message(
        role=role,
        content=content,
        name=name,
        tool_call_id=tool_call_id,
        tool_calls=calls,
    )


def _tool_call_from_dict(value: Any) -> ToolCall:
    if not isinstance(value, dict):
        raise CheckpointError("tool call must be an object")
    return ToolCall(
        call_id=_str(value.get("call_id"), "tool_call.call_id", allow_empty=True),
        name=_str(value.get("name"), "tool_call.name", allow_empty=False),
        arguments_json=_str(value.get("arguments_json"), "tool_call.arguments_json", allow_empty=True),
    )


def _str(value: Any, field_name: str, *, allow_empty: bool) -> str:
    if value is None and allow_empty:
        return ""
    if not isinstance(value, str):
        raise CheckpointError(f"{field_name} must be a string")
    if not allow_empty and not value:
        raise CheckpointError(f"{field_name} is required")
    return value


def _int(value: Any, field_name: str, *, minimum: int | None = None) -> int:
    if isinstance(value, bool) or not isinstance(value, int):
        raise CheckpointError(f"{field_name} must be an integer")
    if minimum is not None and value < minimum:
        raise CheckpointError(f"{field_name} must be >= {minimum}")
    return value


def _now() -> str:
    return datetime.now(tz=timezone.utc).isoformat()
