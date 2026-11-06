"""Middleware — ReAct strategy.

Runs a bounded model/tool loop inside the daemon. The provider and tool
executor are injected so tests and P3-29 can supply implementations.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import asyncio
import json
import time
from dataclasses import replace
from typing import Any, Sequence

from google.protobuf import struct_pb2
from middleware.runtime.v1 import runtime_pb2

from middleware_harness.ids import new_ulid
from middleware_harness.llm.base import (
    DeltaCallback,
    LLMError,
    LLMProvider,
    LLMResponse,
    Message,
    ModelConfig,
    ToolCall,
    ToolSpec,
)
from middleware_harness.runtime.run import RunContext
from middleware_harness.tools.base import ToolExecutor, ToolResult, UnavailableToolExecutor

_DEFAULT_MAX_STEPS = 32
_DEFAULT_STEP_TIMEOUT_MS = 120_000
_DEFAULT_TOOL_TIMEOUT_MS = 60_000


class ReactStrategy:
    """Alternate model completions and tool calls until a terminal answer."""

    name = "react"

    def __init__(
        self,
        provider: LLMProvider,
        tools: ToolExecutor | None = None,
        *,
        default_step_timeout_ms: int = _DEFAULT_STEP_TIMEOUT_MS,
        default_tool_timeout_ms: int = _DEFAULT_TOOL_TIMEOUT_MS,
    ) -> None:
        self.provider = provider
        self.tools = tools or UnavailableToolExecutor()
        self.default_step_timeout_ms = default_step_timeout_ms
        self.default_tool_timeout_ms = default_tool_timeout_ms

    async def run(self, ctx: RunContext) -> str:
        """Execute the ReAct loop."""
        max_steps = max(1, _int_param(ctx.assign.strategy_params, "max_steps", _DEFAULT_MAX_STEPS))
        step_timeout_ms = _int_param(
            ctx.assign.strategy_params,
            "step_timeout_ms",
            self.default_step_timeout_ms,
        )
        tool_timeout_ms = ctx.assign.timeouts.tool_timeout_ms or self.default_tool_timeout_ms

        messages = [Message.from_proto(message) for message in ctx.assign.input]
        tools = [ToolSpec.from_manifest(manifest) for manifest in ctx.assign.tools]
        model = ModelConfig.from_profile(ctx.assign.model)

        for step in range(1, max_steps + 1):
            if ctx.cancelled:
                return "cancelled"

            message_id = new_ulid()
            streamed = False

            async def on_delta(text: str) -> None:
                nonlocal streamed
                streamed = True
                await ctx.emit(
                    runtime_pb2.EventType.MESSAGE_DELTA,
                    {"message_id": message_id, "delta": text, "step": step},
                    ephemeral=True,
                )

            try:
                response = await _complete(
                    self.provider,
                    messages,
                    tools,
                    model,
                    step_timeout_ms,
                    on_delta,
                )
            except asyncio.CancelledError:
                raise
            except asyncio.TimeoutError:
                await self._emit_error(ctx, "STEP_TIMEOUT", f"step {step} timed out")
                return "failed"
            except LLMError as exc:
                await self._emit_error(ctx, exc.code, str(exc), retryable=exc.retryable)
                return "failed"
            except Exception as exc:
                await self._emit_error(ctx, "LLM_ERROR", str(exc))
                return "failed"

            if response.usage:
                await ctx.emit(runtime_pb2.EventType.USAGE, dict(response.usage))
            if response.has_output or streamed:
                await self._emit_message(ctx, response, step, message_id)

            messages.append(_assistant_message(response))

            if not response.tool_calls:
                return "completed"

            for call in response.tool_calls:
                if ctx.cancelled:
                    return "cancelled"
                call = replace(call, invocation_id=call.invocation_id or new_ulid())
                result = await self._run_tool(ctx, call, tool_timeout_ms)
                messages.append(_tool_message(call, result))

        await self._emit_error(
            ctx,
            "MAX_STEPS_EXCEEDED",
            f"react loop exceeded {max_steps} steps",
        )
        return "failed"

    async def __call__(self, ctx: RunContext) -> str:
        """Allow the strategy to be used as a RunManager runner."""
        return await self.run(ctx)

    async def _run_tool(self, ctx: RunContext, call: ToolCall, timeout_ms: int) -> ToolResult:
        await ctx.emit(
            runtime_pb2.EventType.TOOL_CALL_BEGIN,
            {
                "invocation_id": call.invocation_id,
                "tool_name": call.name,
                "arguments_json": call.arguments_json,
            },
        )
        started = time.monotonic()
        try:
            result = await self.tools(ctx, call, timeout_ms)
        except asyncio.CancelledError:
            raise
        except Exception as exc:
            result = ToolResult(
                ok=False,
                error_code="TOOL_EXEC_ERROR",
                error_message=str(exc),
            )

        data: dict[str, Any] = {
            "invocation_id": call.invocation_id,
            "tool_name": call.name,
            "ok": result.ok,
            "duration_ms": int((time.monotonic() - started) * 1000),
        }
        if result.ok:
            data["result_json"] = result.result_json
        else:
            data["error_code"] = result.error_code
            data["error_message"] = result.error_message
            data["retryable"] = result.retryable
        await ctx.emit(runtime_pb2.EventType.TOOL_CALL_END, data)
        return result

    async def _emit_message(
        self,
        ctx: RunContext,
        response: LLMResponse,
        step: int,
        message_id: str,
    ) -> None:
        data: dict[str, Any] = {
            "message_id": message_id,
            "role": "assistant",
            "content": response.content,
            "step": step,
        }
        if response.model:
            data["model"] = response.model
        if response.tool_calls:
            data["tool_calls"] = [
                {
                    "call_id": call.call_id,
                    "tool_name": call.name,
                    "arguments_json": call.arguments_json,
                }
                for call in response.tool_calls
            ]
        await ctx.emit(runtime_pb2.EventType.MESSAGE_COMPLETED, data)

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


async def _complete(
    provider: LLMProvider,
    messages: Sequence[Message],
    tools: Sequence[ToolSpec],
    model: ModelConfig,
    timeout_ms: int,
    on_delta: DeltaCallback,
) -> LLMResponse:
    call = provider.complete(messages, tools, model, on_delta=on_delta)
    if timeout_ms <= 0:
        return await call
    return await asyncio.wait_for(call, timeout_ms / 1000)


def _assistant_message(response: LLMResponse) -> Message:
    return Message(
        role="assistant",
        content=response.content,
        tool_calls=tuple(response.tool_calls),
    )


def _tool_message(call: ToolCall, result: ToolResult) -> Message:
    content = result.result_json
    if not result.ok:
        content = json.dumps(
            {"code": result.error_code, "message": result.error_message},
            ensure_ascii=False,
        )
    return Message(
        role="tool",
        content=content,
        name=call.name,
        tool_call_id=call.call_id,
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
