"""The single runtime-author-facing object.

`RunContext` deliberately exposes plain values and high-level methods only.
Transport details, protobuf messages, sequence numbers, and lease handling stay
inside the Runtime SDK.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import asyncio
import json
from datetime import datetime, timezone
from typing import Any, Mapping

from middleware.runtime.v1 import runtime_pb2

from .channel.client import ChannelClient
from .events import EventWriter
from .ids import new_ulid
from .types import (
    McpBinding,
    Message,
    ModelConfig,
    Timeouts,
    ToolResult,
    ToolSpec,
)


class RuntimeUsageError(ValueError):
    """Raised when a runtime uses a context operation incorrectly."""


class RunContext:
    """State and operations available to one `run(ctx)` invocation."""

    def __init__(
        self,
        *,
        runtime_name: str,
        run_id: str,
        app_id: str,
        input_messages: list[Message],
        params: dict[str, Any],
        model: ModelConfig | None,
        tools: list[ToolSpec],
        mcp_bindings: list[McpBinding],
        timeouts: Timeouts,
        session_id: str | None,
        resume_from: bytes | None,
        trace_id: str,
        channel: ChannelClient,
        events: EventWriter,
        attempt: int,
        lease_epoch: int,
    ) -> None:
        self.runtime_name = runtime_name
        self.run_id = run_id
        self.app_id = app_id
        self.input = input_messages
        self.params = params
        self.model = model
        self.tools = tools
        self.mcp_bindings = mcp_bindings
        self.timeouts = timeouts
        self.session_id = session_id
        self.resume_from = resume_from
        self.trace_id = trace_id
        self.cancel_event = asyncio.Event()
        self.started_at = datetime.now(tz=timezone.utc)
        self.outcome = ""
        self.task: asyncio.Task[Any] | None = None
        # Useful for runtimes that include attempt information in their own
        # checkpoint or logs; it is not a control-plane action.
        self.attempt = attempt

        self._channel = channel
        self._events = events
        self._attempt = attempt
        self._lease_epoch = lease_epoch

    @property
    def cancelled(self) -> bool:
        """Whether the control plane has requested cancellation."""
        return self.cancel_event.is_set()

    async def reply(self, text: str, *, role: str = "assistant") -> None:
        """Emit one completed assistant message."""
        await self.emit(
            "MESSAGE_COMPLETED",
            {
                "message_id": new_ulid(),
                "role": role,
                "content": text,
            },
        )

    async def emit(
        self,
        event_type: str,
        data: dict[str, Any],
        *,
        ephemeral: bool = False,
    ) -> None:
        """Emit one event by protocol name, e.g. MESSAGE_DELTA.

        The SDK assigns sequence numbers and envelope scope.
        """
        await self._events.emit(event_type, data, ephemeral=ephemeral)

    async def call_tool(
        self,
        name: str,
        arguments: Mapping[str, Any] | str | None = None,
        *,
        timeout_ms: int | None = None,
    ) -> ToolResult:
        """Call one tool through the middleware tool bridge."""
        if not name:
            raise RuntimeUsageError("tool name is required")
        if isinstance(arguments, Mapping):
            arguments_json = json.dumps(
                arguments,
                ensure_ascii=False,
                separators=(",", ":"),
            )
        elif arguments is None:
            arguments_json = "{}"
        elif isinstance(arguments, str):
            arguments_json = arguments
        else:
            raise RuntimeUsageError("tool arguments must be a mapping or JSON string")

        timeout = timeout_ms or self.timeouts.tool_ms or 60_000
        try:
            response = await self._channel.request(
                runtime_pb2.ToolCallRequest(
                    invocation_id=new_ulid(),
                    tool_name=name,
                    arguments_json=arguments_json,
                    timeout_ms=timeout,
                ),
                "tool_call_request",
                run_id=self.run_id,
                attempt=self._attempt,
                lease_epoch=self._lease_epoch,
                timeout=max(timeout / 1000.0 + 1.0, 1.0),
            )
        except Exception as exc:
            return ToolResult(
                ok=False,
                error_code="TOOL_TRANSPORT_ERROR",
                error_message=str(exc),
                retryable=True,
            )
        return ToolResult.from_proto(response.tool_call_result)

    async def checkpoint(self, state: bytes | Mapping[str, Any]) -> None:
        """Send an opaque checkpoint for this session.

        The SDK transports the bytes; the runtime owns their schema.
        """
        if not self.session_id:
            raise RuntimeUsageError("checkpoint requires session_id")
        if isinstance(state, Mapping):
            data = json.dumps(
                state,
                ensure_ascii=False,
                separators=(",", ":"),
            ).encode("utf-8")
        elif isinstance(state, (bytes, bytearray)):
            data = bytes(state)
        else:
            raise RuntimeUsageError("checkpoint state must be bytes or a mapping")

        watermark = await self._events.emit_checkpoint(
            self.session_id,
            data,
            watermark=self._events.seq,
        )
        await self.emit(
            "CHECKPOINT_SAVED",
            {
                "session_id": self.session_id,
                "event_seq_watermark": watermark,
            },
        )
