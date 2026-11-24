"""The single runtime-author-facing object.

`RunContext` deliberately exposes plain values and high-level methods only.
Transport details, protobuf messages, sequence numbers, and lease handling stay
inside the Runtime SDK.

The common loop is three calls: `await ctx.toolbox()` returns local and MCP
tools, `await ctx.chat(...)` calls the resolved model, and
`await ctx.invoke_all(reply, toolbox=tools)` dispatches every tool call.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import asyncio
import inspect
import json
import logging
import time
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Any, Callable, Mapping

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


@dataclass(frozen=True)
class _RuntimeTool:
    """One tool registered and executed inside a runtime process."""

    name: str
    description: str
    schema_json: str
    handler: Callable[..., Any]
    timeout_ms: int = 0


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
        self._toolbox = None
        self._checkpoint_saved = False
        self._runtime_tools: dict[str, _RuntimeTool] = {}
        self.logger = logging.LoggerAdapter(
            logging.getLogger("levier_runtime.run"),
            {
                "run_id": run_id,
                "app_id": app_id,
                "runtime_name": runtime_name,
                "attempt": attempt,
                "trace_id": trace_id,
            },
        )

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
        name: str = "",
        schema_version: str = "",
    ) -> None:
        """Emit one event by protocol name, e.g. MESSAGE_DELTA.

        The SDK assigns sequence numbers and envelope scope. Custom events use
        ``emit_custom``; this method keeps the fixed-event path compatible.
        """
        await self._events.emit(
            event_type,
            data,
            ephemeral=ephemeral,
            name=name,
            schema_version=schema_version,
        )

    async def emit_custom(
        self,
        name: str,
        data: dict[str, Any],
        *,
        schema_version: str = "1",
        ephemeral: bool = False,
    ) -> None:
        """Emit one runtime-defined event with name and schema version."""
        await self.emit(
            "CUSTOM",
            data,
            ephemeral=ephemeral,
            name=name,
            schema_version=schema_version,
        )

    def register_tool(
        self,
        name: str,
        handler: Callable[..., Any],
        *,
        description: str = "",
        schema: Mapping[str, Any] | str | None = None,
        timeout_ms: int = 0,
    ) -> None:
        """Register a tool executed inside this runtime process.

        Runtime-local tools join ``ctx.tools`` and therefore the same unified
        model tool list as App, global, and MCP tools. ``ctx.call_tool`` routes
        these tools directly and never sends them through the App Gateway.
        """
        if self._toolbox is not None:
            raise RuntimeUsageError(
                "register runtime tools before opening ctx.toolbox()"
            )
        normalized_name = str(name or "").strip()
        if not normalized_name:
            raise RuntimeUsageError("runtime tool name is required")
        if any(character.isspace() for character in normalized_name):
            raise RuntimeUsageError("runtime tool name must not contain whitespace")
        if not callable(handler):
            raise RuntimeUsageError("runtime tool handler must be callable")
        if normalized_name in self._runtime_tools:
            raise RuntimeUsageError(
                f"runtime tool {normalized_name!r} is already registered"
            )
        if any(tool.name == normalized_name for tool in self.tools):
            raise RuntimeUsageError(
                f"runtime tool {normalized_name!r} conflicts with a control-plane tool"
            )
        if timeout_ms < 0:
            raise RuntimeUsageError("runtime tool timeout_ms must not be negative")
        normalized_description = description or inspect.getdoc(handler) or ""
        self._runtime_tools[normalized_name] = _RuntimeTool(
            name=normalized_name,
            description=normalized_description,
            schema_json=_normalize_tool_schema(schema),
            handler=handler,
            timeout_ms=timeout_ms,
        )
        self.tools.append(
            ToolSpec(
                name=normalized_name,
                description=normalized_description,
                input_schema_json=self._runtime_tools[normalized_name].schema_json,
                scope="runtime",
            )
        )

    def tool(
        self,
        name: str | None = None,
        *,
        description: str = "",
        schema: Mapping[str, Any] | str | None = None,
        timeout_ms: int = 0,
    ) -> Callable[[Callable[..., Any]], Callable[..., Any]]:
        """Decorator form of :meth:`register_tool`."""

        def decorator(handler: Callable[..., Any]) -> Callable[..., Any]:
            self.register_tool(
                name or handler.__name__,
                handler,
                description=description,
                schema=schema,
                timeout_ms=timeout_ms,
            )
            return handler

        return decorator

    def _runtime_tool(self, name: str) -> _RuntimeTool | None:
        return self._runtime_tools.get(name)

    async def toolbox(self) -> "Any":
        """Return every visible tool as one callable sequence.

        The toolbox includes App, global, Runtime-local, and MCP tools. MCP
        sessions are opened lazily on the first call and closed when the Run
        ends. Runtime authors do not need to construct an MCP client.
        """
        if self._toolbox is None:
            from .toolbox import Toolbox

            self._toolbox = await Toolbox.open(self)
        return self._toolbox

    async def chat(
        self,
        messages: "list[Any] | None" = None,
        *,
        tools: "Any" = None,
        on_delta: "Any" = None,
    ) -> "Any":
        """Call the model already resolved for this Run.

        ``ctx.chat(ctx.input)`` uses every visible tool. The returned provider
        response has ``content`` and ``tool_calls``. Provider failures become an
        ERROR event and a failed Run automatically.
        """
        if self.model is None:
            raise RuntimeUsageError("this run has no model; configure a provider and make it the default")
        from .llm.base import Message as LLMMessage
        from .llm.base import ModelConfig as LLMModelConfig
        from .llm.router import ProviderRouter

        values = list(self.input if messages is None else messages)
        converted = []
        for message in values:
            if isinstance(message, LLMMessage):
                converted.append(message)
            elif isinstance(message, Mapping):
                converted.append(
                    LLMMessage(
                        role=str(message.get("role", "user")),
                        content=str(message.get("content", "")),
                        name=str(message.get("name", "")),
                        tool_call_id=str(message.get("tool_call_id", "")),
                    )
                )
            else:
                converted.append(
                    LLMMessage(
                        role=str(getattr(message, "role", "user")),
                        content=str(getattr(message, "content", "")),
                        name=str(getattr(message, "name", "")),
                    )
                )

        if tools is None:
            tools = await self.toolbox()
        specs = _tool_specs(tools)
        model = LLMModelConfig(
            provider=self.model.provider,
            model=self.model.model,
            api_key=self.model.api_key,
            base_url=self.model.base_url,
            params=dict(self.model.params),
        )
        return await ProviderRouter().complete(converted, specs, model, on_delta=on_delta)

    async def invoke(
        self,
        call: "Any",
        *,
        toolbox: "Any" = None,
    ) -> ToolResult:
        """Parse one model tool call and route it by its source."""
        name, arguments, invocation_id = _tool_call(call)
        active = toolbox if toolbox is not None else await self.toolbox()
        return await active.call(
            name,
            arguments,
            invocation_id=invocation_id,
        )

    async def invoke_all(
        self,
        response: "Any",
        *,
        toolbox: "Any" = None,
    ) -> "list[ToolResult]":
        """Execute every tool call in one provider response."""
        calls = getattr(response, "tool_calls", response)
        active = toolbox if toolbox is not None else await self.toolbox()
        results: list[ToolResult] = []
        for call in calls:
            results.append(await self.invoke(call, toolbox=active))
        return results

    async def call_tool(
        self,
        name: str,
        arguments: Mapping[str, Any] | str | None = None,
        *,
        timeout_ms: int | None = None,
        emit_events: bool = True,
        invocation_id: str = "",
    ) -> ToolResult:
        """Call one tool through the unified runtime tool router.

        Runtime-local tools execute in this process. App and global tools go
        through the middleware tool bridge. By default the SDK emits
        TOOL_CALL_BEGIN/END and owns tool-call observability. Runtimes that
        already emit those events can pass ``emit_events=False`` to avoid
        duplicates.
        """
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

        runtime_tool = self._runtime_tools.get(name)
        if runtime_tool is not None:
            return await self._call_runtime_tool(
                runtime_tool,
                arguments_json,
                timeout_ms=timeout_ms,
                invocation_id=invocation_id or new_ulid(),
                emit_events=emit_events,
            )

        invocation_id = invocation_id or new_ulid()
        if emit_events:
            await self._emit_safe(
                "TOOL_CALL_BEGIN",
                {
                    "invocation_id": invocation_id,
                    "tool_name": name,
                    "arguments_json": arguments_json,
                },
            )

        started = time.monotonic()
        timeout = timeout_ms or self.timeouts.tool_ms or 60_000
        try:
            response = await self._channel.request(
                runtime_pb2.ToolCallRequest(
                    invocation_id=invocation_id,
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
            result = ToolResult.from_proto(response.tool_call_result)
        except Exception as exc:
            result = ToolResult(
                ok=False,
                error_code="TOOL_TRANSPORT_ERROR",
                error_message=str(exc),
                retryable=True,
            )

        if emit_events:
            data: dict[str, Any] = {
                "invocation_id": invocation_id,
                "tool_name": name,
                "ok": result.ok,
                "duration_ms": int((time.monotonic() - started) * 1000),
            }
            if result.ok:
                data["result_json"] = result.result_json
            else:
                data["error_code"] = result.error_code
                data["error_message"] = result.error_message
                data["retryable"] = result.retryable
            await self._emit_safe("TOOL_CALL_END", data)
        return result

    async def _call_runtime_tool(
        self,
        tool: _RuntimeTool,
        arguments_json: str,
        *,
        timeout_ms: int | None,
        invocation_id: str,
        emit_events: bool,
    ) -> ToolResult:
        if emit_events:
            await self._emit_safe(
                "TOOL_CALL_BEGIN",
                {
                    "invocation_id": invocation_id,
                    "tool_name": tool.name,
                    "arguments_json": arguments_json,
                    "tool_scope": "runtime",
                },
            )

        started = time.monotonic()
        timeout = timeout_ms or tool.timeout_ms or self.timeouts.tool_ms or 60_000
        try:
            arguments = json.loads(arguments_json)
            if not isinstance(arguments, Mapping):
                raise ValueError("tool arguments must be a JSON object")

            async def invoke() -> Any:
                if inspect.iscoroutinefunction(tool.handler):
                    return await tool.handler(arguments)
                value = await asyncio.to_thread(tool.handler, arguments)
                if inspect.isawaitable(value):
                    return await value
                return value

            value = await asyncio.wait_for(invoke(), timeout=max(timeout, 1) / 1000.0)
            result = _runtime_tool_result(value)
        except asyncio.CancelledError:
            raise
        except asyncio.TimeoutError:
            result = ToolResult(
                ok=False,
                error_code="TOOL_TIMEOUT",
                error_message=f"runtime tool {tool.name!r} timed out",
                retryable=True,
            )
        except json.JSONDecodeError as exc:
            result = ToolResult(
                ok=False,
                error_code="BAD_TOOL_ARGUMENTS",
                error_message=f"runtime tool arguments are invalid JSON: {exc}",
            )
        except Exception as exc:  # runtime-local failures stay local
            result = ToolResult(
                ok=False,
                error_code="TOOL_EXEC_ERROR",
                error_message=str(exc),
            )

        if emit_events:
            data: dict[str, Any] = {
                "invocation_id": invocation_id,
                "tool_name": tool.name,
                "ok": result.ok,
                "duration_ms": int((time.monotonic() - started) * 1000),
                "tool_scope": "runtime",
            }
            if result.ok:
                data["result_json"] = result.result_json
            else:
                data["error_code"] = result.error_code
                data["error_message"] = result.error_message
                data["retryable"] = result.retryable
            await self._emit_safe("TOOL_CALL_END", data)
        return result

    async def error(
        self,
        code: str,
        message: str,
        *,
        retryable: bool = False,
        data: dict[str, Any] | None = None,
    ) -> None:
        """Emit one normalized ERROR event."""
        payload: dict[str, Any] = {
            "code": code,
            "message": message,
            "retryable": retryable,
        }
        if data:
            payload.update(data)
        await self.emit("ERROR", payload)

    async def _emit_safe(self, event_type: str, data: dict[str, Any]) -> None:
        """Emit an SDK observability event without failing the run itself."""
        try:
            await self.emit(event_type, data)
        except Exception as exc:  # noqa: BLE001 - observability must not mask run
            self.logger.warning("runtime event emission failed", extra={
                "event_type": event_type,
                "error": str(exc),
            })

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
        self._checkpoint_saved = True
        await self.emit(
            "CHECKPOINT_SAVED",
            {
                "session_id": self.session_id,
                "event_seq_watermark": watermark,
            },
        )

    async def wait(self, state: bytes | Mapping[str, Any]) -> str:
        """Persist a checkpoint and return the waiting status for ``run``."""
        await self.checkpoint(state)
        return "waiting"

    async def close_toolbox(self) -> None:
        """Close any MCP sessions opened by :meth:`toolbox`."""
        if self._toolbox is None:
            return
        await self._toolbox.close()
        self._toolbox = None


def _runtime_tool_result(value: Any) -> ToolResult:
    if isinstance(value, ToolResult):
        return value
    if all(hasattr(value, attribute) for attribute in (
        "ok", "result_json", "error_code", "error_message", "retryable",
    )):
        return ToolResult(
            ok=bool(value.ok),
            result_json=str(value.result_json),
            error_code=str(value.error_code),
            error_message=str(value.error_message),
            retryable=bool(value.retryable),
        )
    try:
        result_json = json.dumps(
            value,
            ensure_ascii=False,
            separators=(",", ":"),
        )
    except (TypeError, ValueError) as exc:
        return ToolResult(
            ok=False,
            error_code="TOOL_EXEC_ERROR",
            error_message=f"runtime tool result is not JSON serializable: {exc}",
        )
    return ToolResult(ok=True, result_json=result_json)


def _normalize_tool_schema(schema: Mapping[str, Any] | str | None) -> str:
    if schema is None:
        return '{"type":"object"}'
    if isinstance(schema, Mapping):
        value: Any = schema
    elif isinstance(schema, str):
        try:
            value = json.loads(schema)
        except json.JSONDecodeError as exc:
            raise RuntimeUsageError(f"runtime tool schema is invalid JSON: {exc}") from exc
    else:
        raise RuntimeUsageError("runtime tool schema must be a mapping or JSON string")
    if not isinstance(value, Mapping):
        raise RuntimeUsageError("runtime tool schema must be a JSON object")
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"))


def _tool_specs(tools: Any) -> list[Any]:
    from .llm.base import ToolSpec as LLMToolSpec

    if hasattr(tools, "specs") and callable(tools.specs):
        return list(tools.specs())
    result = []
    for tool in tools:
        if isinstance(tool, LLMToolSpec):
            result.append(tool)
            continue
        schema = getattr(tool, "schema_json", "") or getattr(tool, "input_schema_json", "")
        if not schema:
            schema_value = getattr(tool, "schema", None)
            if isinstance(schema_value, Mapping):
                schema = json.dumps(schema_value, ensure_ascii=False, separators=(",", ":"))
        result.append(
            LLMToolSpec(
                name=str(getattr(tool, "name", "")),
                description=str(getattr(tool, "description", "")),
                input_schema_json=str(schema or "{}"),
            )
        )
    return result


def _tool_call(call: Any) -> tuple[str, str, str]:
    if isinstance(call, Mapping):
        name = str(call.get("name", ""))
        arguments = call.get("arguments_json", call.get("arguments", "{}"))
        invocation_id = str(
            call.get("invocation_id", call.get("invocationId", ""))
        )
    else:
        name = str(getattr(call, "name", ""))
        arguments = getattr(call, "arguments_json", None)
        if arguments is None:
            arguments = getattr(call, "arguments", "{}")
        invocation_id = str(getattr(call, "invocation_id", ""))
    if not name:
        raise RuntimeUsageError("tool call has no name")
    if isinstance(arguments, Mapping):
        encoded = json.dumps(arguments, ensure_ascii=False, separators=(",", ":"))
    elif arguments is None:
        encoded = "{}"
    else:
        encoded = str(arguments)
    return name, encoded, invocation_id
