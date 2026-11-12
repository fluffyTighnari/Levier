"""Middleware — bounded model/tool loop.

The core never imports Middleware, Harness, MCP runtime, or protobuf.
Adapters supply the provider, tools, event sink, and abort signal.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import asyncio
import json
import time
import uuid
from dataclasses import replace
from typing import Awaitable, Callable, Sequence

from agent_core.errors import ProviderFailure
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
)
from agent_core.protocols import AbortSignal, ModelProvider, ToolProvider
from agent_core.types import Message, RunResult, RunSpec, ToolCall, ToolResult, Usage

Emit = Callable[[CoreEvent], Awaitable[None]]
StepHook = Callable[[Sequence[Message], int], Awaitable[None]]
CancelHook = Callable[[Sequence[Message], int], Awaitable[None]]


class _NeverAbort:
    def is_set(self) -> bool:
        return False


class AgentCore:
    """Alternate provider completions and tool calls until a terminal answer."""

    def __init__(
        self,
        provider: ModelProvider,
        tools: ToolProvider,
        *,
        new_id: Callable[[], str] | None = None,
    ) -> None:
        self.provider = provider
        self.tools = tools
        self.new_id = new_id or (lambda: uuid.uuid4().hex)

    async def run(
        self,
        spec: RunSpec,
        emit: Emit,
        *,
        abort: AbortSignal | None = None,
        on_step_end: StepHook | None = None,
        on_cancel: CancelHook | None = None,
    ) -> RunResult:
        """Run one attempt and return the terminal result."""
        abort = abort or _NeverAbort()
        messages = list(spec.messages)
        total = Usage()
        step = spec.start_step - 1
        status = "failed"
        error_code = ""
        error_message = ""
        task_cancelled = False

        await emit(RunStarted(run_id=spec.run_id))
        try:
            for step in range(spec.start_step, spec.max_steps + 1):
                if abort.is_set():
                    status = "cancelled"
                    break
                await emit(StepStarted(run_id=spec.run_id, step=step))

                message_id = self.new_id()
                streamed = False

                async def on_delta(text: str) -> None:
                    nonlocal streamed
                    streamed = True
                    await emit(MessageDelta(
                        run_id=spec.run_id,
                        step=step,
                        message_id=message_id,
                        delta=text,
                    ))

                try:
                    response = await _with_timeout(
                        self.provider.complete(
                            messages, spec.tools, spec.model, on_delta=on_delta
                        ),
                        spec.step_timeout_ms,
                    )
                except asyncio.CancelledError:
                    raise
                except asyncio.TimeoutError:
                    error_code = "STEP_TIMEOUT"
                    error_message = f"step {step} timed out"
                    await emit(ErrorEvent(
                        run_id=spec.run_id, step=step,
                        code=error_code, message=error_message,
                    ))
                    status = "failed"
                    break
                except ProviderFailure as exc:
                    error_code = exc.code
                    error_message = str(exc)
                    await emit(ErrorEvent(
                        run_id=spec.run_id, step=step,
                        code=error_code, message=error_message,
                        retryable=exc.retryable,
                    ))
                    status = "failed"
                    break
                except Exception as exc:  # noqa: BLE001 - core boundary
                    error_code = "PROVIDER_ERROR"
                    error_message = str(exc)
                    await emit(ErrorEvent(
                        run_id=spec.run_id, step=step,
                        code=error_code, message=error_message,
                    ))
                    status = "failed"
                    break

                if _has_usage(response.usage):
                    total = _add_usage(total, response.usage)
                    await emit(UsageEvent(run_id=spec.run_id, step=step, usage=response.usage))

                if response.content or response.tool_calls or streamed:
                    await emit(MessageCompleted(
                        run_id=spec.run_id,
                        step=step,
                        message_id=message_id,
                        content=response.content,
                        tool_calls=tuple(response.tool_calls),
                        model=response.model,
                    ))

                messages.append(Message(
                    role="assistant",
                    content=response.content,
                    tool_calls=tuple(response.tool_calls),
                ))

                if not response.tool_calls:
                    status = "completed"
                    break

                for call in response.tool_calls:
                    if abort.is_set():
                        status = "cancelled"
                        break
                    if not call.invocation_id:
                        call = replace(call, invocation_id=self.new_id())
                    await emit(ToolStarted(
                        run_id=spec.run_id,
                        step=step,
                        invocation_id=call.invocation_id,
                        tool_name=call.name,
                        arguments_json=call.arguments_json,
                    ))
                    started = time.monotonic()
                    try:
                        result = await _with_timeout(self.tools.call(call), spec.tool_timeout_ms)
                    except asyncio.CancelledError:
                        raise
                    except asyncio.TimeoutError:
                        result = ToolResult(
                            ok=False,
                            error_code="TOOL_TIMEOUT",
                            error_message=f"tool {call.name!r} timed out",
                            retryable=True,
                        )
                    except Exception as exc:  # noqa: BLE001 - core boundary
                        result = ToolResult(
                            ok=False,
                            error_code="TOOL_EXEC_ERROR",
                            error_message=str(exc),
                        )
                    duration_ms = int((time.monotonic() - started) * 1000)
                    await emit(ToolFinished(
                        run_id=spec.run_id,
                        step=step,
                        invocation_id=call.invocation_id,
                        tool_name=call.name,
                        ok=result.ok,
                        result_json=result.result_json,
                        error_code=result.error_code,
                        error_message=result.error_message,
                        retryable=result.retryable,
                        duration_ms=duration_ms,
                    ))
                    messages.append(_tool_message(call, result))
                if status == "cancelled":
                    break
                if on_step_end is not None:
                    await on_step_end(messages, step)
            else:
                error_code = "MAX_STEPS"
                error_message = f"loop exceeded {spec.max_steps} steps"
                await emit(ErrorEvent(
                    run_id=spec.run_id, step=step,
                    code=error_code, message=error_message,
                ))
                status = "failed"
        except asyncio.CancelledError:
            task_cancelled = True
            status = "cancelled"
            if on_cancel is not None:
                try:
                    await on_cancel(messages, max(step, 0))
                except Exception:  # noqa: BLE001 - never mask cancellation
                    pass
            raise
        finally:
            if not task_cancelled:
                await emit(RunFinished(run_id=spec.run_id, status=status, steps=max(step, 0)))

        return RunResult(
            status=status,
            messages=messages,
            steps=max(step, 0),
            usage=total,
            error_code=error_code,
            error_message=error_message,
        )


async def _with_timeout(awaitable: Awaitable, timeout_ms: int):
    if timeout_ms <= 0:
        return await awaitable
    return await asyncio.wait_for(awaitable, timeout_ms / 1000)


def _has_usage(usage: Usage) -> bool:
    return any((
        usage.input_tokens,
        usage.output_tokens,
        usage.cache_read_tokens,
        usage.cache_write_tokens,
    ))


def _add_usage(total: Usage, step: Usage) -> Usage:
    return Usage(
        input_tokens=total.input_tokens + step.input_tokens,
        output_tokens=total.output_tokens + step.output_tokens,
        total_tokens=total.total_tokens + step.total_tokens,
        cache_read_tokens=total.cache_read_tokens + step.cache_read_tokens,
        cache_write_tokens=total.cache_write_tokens + step.cache_write_tokens,
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
