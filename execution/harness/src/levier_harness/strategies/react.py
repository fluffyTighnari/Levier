"""Middleware — ReAct strategy adapter.

The loop lives in `levier_agent_core`; this module only translates between
Middleware/TaskAssign types and the core contract.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import asyncio
import time
from typing import Any, Sequence

from google.protobuf import struct_pb2
from middleware.runtime.v1 import runtime_pb2

from levier_agent_core import (
    AgentCore,
    LLMResponse as CoreLLMResponse,
    Message as CoreMessage,
    ModelRef,
    ProviderFailure,
    RunSpec,
    ToolCall as CoreToolCall,
    ToolResult as CoreToolResult,
    ToolSpec as CoreToolSpec,
    Usage,
)
from levier_agent_core.events import (
    ErrorEvent,
    MessageCompleted,
    MessageDelta,
    ToolFinished,
    ToolStarted,
    UsageEvent,
)
from levier_harness.ids import new_ulid
from levier_harness.llm.base import (
    DeltaCallback,
    LLMError,
    LLMProvider,
    LLMResponse,
    Message,
    ModelConfig,
    ToolCall,
    ToolSpec,
)
from levier_harness.runtime.checkpoint import capture
from levier_harness.runtime.run import RunContext
from levier_harness.tools.base import ToolProvider, as_tool_provider

_DEFAULT_MAX_STEPS = 32
_DEFAULT_STEP_TIMEOUT_MS = 120_000
_DEFAULT_TOOL_TIMEOUT_MS = 60_000


class ReactStrategy:
    """Alternate model completions and tool calls until a terminal answer."""

    name = "react"

    def __init__(
        self,
        provider: LLMProvider,
        tools: ToolProvider | object | None = None,
        *,
        default_step_timeout_ms: int = _DEFAULT_STEP_TIMEOUT_MS,
        default_tool_timeout_ms: int = _DEFAULT_TOOL_TIMEOUT_MS,
    ) -> None:
        self.provider = provider
        self.tools = as_tool_provider(tools)
        self.default_step_timeout_ms = default_step_timeout_ms
        self.default_tool_timeout_ms = default_tool_timeout_ms

    async def run(self, ctx: RunContext) -> str:
        """Adapt one TaskAssign to the agent core loop."""
        max_steps = max(1, _int_param(ctx.assign.strategy_params, "max_steps", _DEFAULT_MAX_STEPS))
        step_timeout_ms = _int_param(
            ctx.assign.strategy_params,
            "step_timeout_ms",
            self.default_step_timeout_ms,
        )
        tool_timeout_ms = ctx.assign.timeouts.tool_timeout_ms or self.default_tool_timeout_ms

        messages = [Message.from_proto(message) for message in ctx.assign.input]
        start_step = 1
        if ctx.resume is not None:
            if ctx.resume.strategy != self.name:
                await self._emit_error(
                    ctx,
                    "CHECKPOINT_STRATEGY_MISMATCH",
                    f"checkpoint strategy {ctx.resume.strategy!r} does not match {self.name!r}",
                )
                return "failed"
            if ctx.resume.messages:
                messages = list(ctx.resume.messages)
            start_step = max(1, ctx.resume.step + 1)

        tools = [ToolSpec.from_manifest(manifest) for manifest in ctx.assign.tools]
        model = ModelConfig.from_profile(ctx.assign.model)

        result = None
        saved_on_cancel = False
        try:
            tools.extend(await self.tools.open(ctx))
            spec = RunSpec(
                run_id=ctx.run_id,
                start_step=start_step,
                messages=[_to_core_message(message) for message in messages],
                tools=[_to_core_tool(tool) for tool in tools],
                model=ModelRef(
                    provider=model.provider,
                    model=model.model,
                    params=dict(model.params),
                ),
                max_steps=max_steps,
                step_timeout_ms=step_timeout_ms,
                tool_timeout_ms=tool_timeout_ms,
                session_id=ctx.assign.session_id or "",
            )
            core = AgentCore(
                _ProviderAdapter(self.provider, model),
                _ToolAdapter(self.tools, ctx, tool_timeout_ms),
                new_id=new_ulid,
            )

            async def on_step_end(core_messages: Sequence[CoreMessage], step: int) -> None:
                if ctx.checkpoint_due():
                    await self._save_checkpoint(
                        ctx,
                        [_to_harness_message(message) for message in core_messages],
                        step,
                    )

            async def on_cancel(core_messages: Sequence[CoreMessage], step: int) -> None:
                nonlocal saved_on_cancel
                if ctx.assign.session_id and step > 0:
                    try:
                        await self._save_checkpoint(
                            ctx,
                            [_to_harness_message(message) for message in core_messages],
                            step,
                        )
                        saved_on_cancel = True
                    except Exception:
                        pass

            result = await core.run(
                spec,
                lambda event: _emit_core_event(ctx, event),
                abort=_CtxAbort(ctx),
                on_step_end=on_step_end,
                on_cancel=on_cancel,
            )
        finally:
            if (
                not saved_on_cancel
                and ctx.assign.session_id
                and result is not None
                and result.steps > 0
            ):
                try:
                    await self._save_checkpoint(
                        ctx,
                        [_to_harness_message(message) for message in result.messages],
                        result.steps,
                    )
                except Exception:
                    pass
            try:
                await self.tools.close(ctx)
            except Exception:
                pass
        return result.status if result is not None else "failed"

    async def _save_checkpoint(
        self,
        ctx: RunContext,
        messages: Sequence[Message],
        step: int,
    ) -> None:
        if not ctx.assign.session_id:
            return
        await ctx.save_checkpoint(capture(
            session_id=ctx.assign.session_id,
            run_id=ctx.run_id,
            attempt=ctx.attempt,
            strategy=self.name,
            step=step,
            messages=list(messages),
            last_event_seq=ctx.events.seq,
        ))

    async def __call__(self, ctx: RunContext) -> str:
        """Allow the strategy to be used as a RunManager runner."""
        return await self.run(ctx)

    async def _emit_error(
        self,
        ctx: RunContext,
        code: str,
        message: str,
        *,
        retryable: bool = False,
    ) -> None:
        await ctx.emit(
            runtime_pb2.EventType.ERROR,
            {"code": code, "message": message, "retryable": retryable},
        )


class _CtxAbort:
    def __init__(self, ctx: RunContext) -> None:
        self.ctx = ctx

    def is_set(self) -> bool:
        return self.ctx.cancelled


class _ProviderAdapter:
    def __init__(self, provider: LLMProvider, model: ModelConfig) -> None:
        self.provider = provider
        self.model = model

    async def complete(
        self,
        messages: Sequence[CoreMessage],
        tools: Sequence[CoreToolSpec],
        model: ModelRef,
        *,
        on_delta: DeltaCallback | None = None,
    ) -> CoreLLMResponse:
        try:
            response = await self.provider.complete(
                [_to_harness_message(message) for message in messages],
                [_to_harness_tool(tool) for tool in tools],
                self.model,
                on_delta=on_delta,
            )
        except LLMError as exc:
            raise ProviderFailure(exc.code, str(exc), retryable=exc.retryable) from exc
        except Exception as exc:  # noqa: BLE001 - adapter boundary
            raise ProviderFailure("LLM_ERROR", str(exc)) from exc
        return _to_core_response(response)


class _ToolAdapter:
    def __init__(self, tools: ToolProvider, ctx: RunContext, timeout_ms: int) -> None:
        self.tools = tools
        self.ctx = ctx
        self.timeout_ms = timeout_ms

    async def call(self, call: CoreToolCall) -> CoreToolResult:
        result = await self.tools.call(
            self.ctx,
            ToolCall(
                call_id=call.call_id,
                name=call.name,
                arguments_json=call.arguments_json,
                invocation_id=call.invocation_id,
            ),
            self.timeout_ms,
        )
        return CoreToolResult(
            ok=result.ok,
            result_json=result.result_json,
            error_code=result.error_code,
            error_message=result.error_message,
            retryable=result.retryable,
        )


async def _emit_core_event(ctx: RunContext, event: Any) -> None:
    if isinstance(event, MessageDelta):
        await ctx.emit(
            runtime_pb2.EventType.MESSAGE_DELTA,
            {"message_id": event.message_id, "delta": event.delta, "step": event.step},
            ephemeral=True,
        )
        return
    if isinstance(event, MessageCompleted):
        data: dict[str, Any] = {
            "message_id": event.message_id,
            "role": "assistant",
            "content": event.content,
            "step": event.step,
        }
        if event.model:
            data["model"] = event.model
        if event.tool_calls:
            data["tool_calls"] = [
                {
                    "call_id": call.call_id,
                    "tool_name": call.name,
                    "arguments_json": call.arguments_json,
                }
                for call in event.tool_calls
            ]
        await ctx.emit(runtime_pb2.EventType.MESSAGE_COMPLETED, data)
        return
    if isinstance(event, UsageEvent):
        await ctx.emit(runtime_pb2.EventType.USAGE, event.usage.to_dict())
        return
    if isinstance(event, ToolStarted):
        await ctx.emit(
            runtime_pb2.EventType.TOOL_CALL_BEGIN,
            {
                "invocation_id": event.invocation_id,
                "tool_name": event.tool_name,
                "arguments_json": event.arguments_json,
            },
        )
        return
    if isinstance(event, ToolFinished):
        data = {
            "invocation_id": event.invocation_id,
            "tool_name": event.tool_name,
            "ok": event.ok,
            "duration_ms": event.duration_ms,
        }
        if event.ok:
            data["result_json"] = event.result_json
        else:
            data["error_code"] = event.error_code
            data["error_message"] = event.error_message
            data["retryable"] = event.retryable
        await ctx.emit(runtime_pb2.EventType.TOOL_CALL_END, data)
        return
    if isinstance(event, ErrorEvent):
        code = "MAX_STEPS_EXCEEDED" if event.code == "MAX_STEPS" else event.code
        await ctx.emit(
            runtime_pb2.EventType.ERROR,
            {"code": code, "message": event.message, "retryable": event.retryable},
        )


def _to_core_message(message: Message) -> CoreMessage:
    return CoreMessage(
        role=message.role,
        content=message.content,
        name=message.name,
        tool_call_id=message.tool_call_id,
        tool_calls=tuple(
            CoreToolCall(
                call_id=call.call_id,
                name=call.name,
                arguments_json=call.arguments_json,
                invocation_id=call.invocation_id,
            )
            for call in message.tool_calls
        ),
    )


def _to_harness_message(message: CoreMessage) -> Message:
    return Message(
        role=message.role,
        content=message.content,
        name=message.name,
        tool_call_id=message.tool_call_id,
        tool_calls=tuple(
            ToolCall(
                call_id=call.call_id,
                name=call.name,
                arguments_json=call.arguments_json,
                invocation_id=call.invocation_id,
            )
            for call in message.tool_calls
        ),
    )


def _to_core_tool(tool: ToolSpec) -> CoreToolSpec:
    return CoreToolSpec(
        name=tool.name,
        description=tool.description,
        input_schema_json=tool.input_schema_json,
    )


def _to_harness_tool(tool: CoreToolSpec) -> ToolSpec:
    return ToolSpec(
        name=tool.name,
        description=tool.description,
        input_schema_json=tool.input_schema_json,
    )


def _to_core_response(response: LLMResponse) -> CoreLLMResponse:
    return CoreLLMResponse(
        content=response.content,
        tool_calls=[
            CoreToolCall(
                call_id=call.call_id,
                name=call.name,
                arguments_json=call.arguments_json,
                invocation_id=call.invocation_id,
            )
            for call in response.tool_calls
        ],
        usage=_to_core_usage(response.usage),
        model=response.model,
    )


def _to_core_usage(raw: dict[str, int]) -> Usage:
    input_tokens = int(raw.get("input_tokens", raw.get("prompt_tokens", 0)))
    output_tokens = int(raw.get("output_tokens", raw.get("completion_tokens", 0)))
    return Usage(
        input_tokens=input_tokens,
        output_tokens=output_tokens,
        total_tokens=int(raw.get("total_tokens", input_tokens + output_tokens)),
        cache_read_tokens=int(raw.get("cache_read_tokens", raw.get("prompt_cache_hit_tokens", 0))),
        cache_write_tokens=int(raw.get("cache_write_tokens", 0)),
    )


def _int_param(params: struct_pb2.Struct, name: str, default: int) -> int:
    value = params.fields.get(name)
    if value is None:
        return default
    if value.WhichOneof("kind") == "number_value":
        return int(value.number_value)
    if value.WhichOneof("kind") == "string_value":
        try:
            return int(value.string_value)
        except ValueError:
            return default
    return default
