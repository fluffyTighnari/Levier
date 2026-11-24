"""TaskAssign/Cancel/Reconcile lifecycle for one runtime instance.

This module is SDK-internal. Runtime authors only see `RunContext`.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import asyncio
import json
import logging
from dataclasses import dataclass
from typing import Any, Protocol

from middleware.runtime.v1 import runtime_pb2

from .channel.client import ChannelClient
from .context import RunContext
from .events import EventWriter
from .types import (
    McpBinding,
    Message,
    ModelConfig,
    Timeouts,
    ToolSpec,
    params_from_proto,
)

TERMINAL_STATUSES = {"completed", "failed", "cancelled"}
WAITING_STATUS = "waiting"
VALID_OUTCOMES = TERMINAL_STATUSES | {WAITING_STATUS}


class Runner(Protocol):
    """A runtime's `run(ctx)` function."""

    async def __call__(self, ctx: RunContext) -> str:
        """Return a terminal status or ``waiting`` for a resumable run."""


class NoopRunner:
    """Fallback runner used only for internal tests."""

    async def __call__(self, ctx: RunContext) -> str:
        return "completed"


@dataclass(frozen=True)
class ManagerConfig:
    """Runtime identity and capacity."""

    runtime_name: str
    max_concurrent_runs: int = 4


class RunManager:
    """Tracks active runs and handles TaskAssign/Cancel/Reconcile."""

    def __init__(
        self,
        sender: ChannelClient,
        config: ManagerConfig,
        runner: Runner,
        *,
        logger: logging.Logger | None = None,
    ) -> None:
        self.sender = sender
        self.config = config
        self.runner = runner
        self.logger = logger or logging.getLogger("levier_runtime.manager")
        self.active: dict[str, RunContext] = {}
        self._closing = False

    async def handle(self, envelope: runtime_pb2.Envelope) -> None:
        """Dispatch one inbound RuntimeChannel envelope."""
        payload = envelope.WhichOneof("payload")
        if payload == "task_assign":
            await self._handle_task_assign(envelope)
        elif payload == "cancel":
            await self._handle_cancel(envelope)
        elif payload == "reconcile":
            await self._handle_reconcile(envelope)

    async def _handle_task_assign(self, envelope: runtime_pb2.Envelope) -> None:
        assign = envelope.task_assign
        reason = self._validation_error(envelope, assign)
        if reason:
            await self._send_ack(envelope, accepted=False, reason=reason)
            return

        existing = self.active.get(assign.run_id)
        if existing is not None:
            if existing._lease_epoch == envelope.lease_epoch:
                await self._send_ack(envelope, accepted=True)
                return
            await self._send_ack(
                envelope,
                accepted=False,
                reason="run already active with different lease_epoch",
            )
            return

        if len(self.active) >= self.config.max_concurrent_runs:
            await self._send_ack(envelope, accepted=False, reason="capacity")
            return

        await self._send_ack(envelope, accepted=True)
        ctx = self._new_context(assign, envelope)
        self.active[ctx.run_id] = ctx
        await ctx.emit("RUN_STATUS", {"status": "running"})
        ctx.task = asyncio.create_task(self._execute(ctx), name=f"run-{ctx.run_id}")

    def _new_context(
        self,
        assign: runtime_pb2.TaskAssign,
        envelope: runtime_pb2.Envelope,
    ) -> RunContext:
        timeouts = assign.timeouts
        events = EventWriter(
            self.sender,
            assign.run_id,
            envelope.attempt,
            envelope.lease_epoch,
        )
        resume_value, resume_value_set = _parse_resume_value(assign.resume_value_json)
        ctx = RunContext(
            runtime_name=self.config.runtime_name,
            run_id=assign.run_id,
            app_id=assign.app_id,
            input_messages=[Message.from_proto(value) for value in assign.input],
            params=params_from_proto(assign.params),
            model=ModelConfig.from_proto(assign.model),
            tools=[ToolSpec.from_proto(value) for value in assign.tools],
            mcp_bindings=[McpBinding.from_proto(value) for value in assign.mcp_bindings],
            timeouts=Timeouts(
                run_ms=timeouts.run_timeout_ms if timeouts is not None else 0,
                tool_ms=timeouts.tool_timeout_ms if timeouts is not None else 0,
            ),
            session_id=assign.session_id or None,
            resume_from=assign.resume_from.state or None,
            resume_value=resume_value,
            resume_value_set=resume_value_set,
            trace_id=assign.trace_context,
            channel=self.sender,
            events=events,
            attempt=envelope.attempt,
            lease_epoch=envelope.lease_epoch,
        )
        ctx.task = None
        return ctx

    def _validation_error(
        self,
        envelope: runtime_pb2.Envelope,
        assign: runtime_pb2.TaskAssign,
    ) -> str:
        if envelope.run_id != assign.run_id:
            return "run_id mismatch"
        if not assign.run_id:
            return "run_id is required"
        if not assign.app_id:
            return "app_id is required"
        if envelope.attempt < 1:
            return "attempt must be >= 1"
        if envelope.lease_epoch < 1:
            return "lease_epoch must be >= 1"
        if self._closing:
            return "runtime draining"
        if assign.resume_value_json:
            try:
                json.loads(assign.resume_value_json.decode("utf-8"))
            except (UnicodeDecodeError, json.JSONDecodeError):
                return "resume_value_json is invalid"
        return ""

    async def _send_ack(
        self,
        request: runtime_pb2.Envelope,
        *,
        accepted: bool,
        reason: str = "",
    ) -> None:
        ack = runtime_pb2.TaskAck(accepted=accepted, reason=reason)
        await self.sender.send_response(request, ack, "task_ack")

    async def _execute(self, ctx: RunContext) -> None:
        status = "failed"
        reason = ""
        error_code = ""
        try:
            status = await self.runner(ctx)
            if status not in VALID_OUTCOMES:
                reason = f"runtime returned invalid status {status!r}"
                error_code = "INVALID_RUNTIME_STATUS"
                status = "failed"
            elif status == WAITING_STATUS and (
                not ctx.session_id or not ctx._checkpoint_saved
            ):
                reason = "waiting requires a saved longterm checkpoint"
                error_code = "WAITING_WITHOUT_CHECKPOINT"
                status = "failed"
            if ctx.cancelled:
                status = "cancelled"
        except asyncio.CancelledError:
            status = "cancelled"
        except Exception as exc:  # runtime failures become a failed run
            status = "failed"
            reason = str(exc)
            error_code = "RUNTIME_ERROR"
            self.logger.error("runtime run failed", extra={
                "run_id": ctx.run_id,
                "error": str(exc),
            })

        if error_code and not ctx.cancelled:
            await self._emit_error(ctx, error_code, reason)
        if ctx.cancelled:
            status = "cancelled"
        ctx.outcome = status
        data: dict[str, Any] = {"status": status}
        if reason:
            data["reason"] = reason
        try:
            await ctx.emit("RUN_STATUS", data)
        except Exception as exc:
            self.logger.error("runtime terminal event failed", extra={
                "run_id": ctx.run_id,
                "error": str(exc),
            })
        finally:
            try:
                await asyncio.shield(ctx.close_toolbox())
            except asyncio.CancelledError:
                pass
            except Exception as exc:
                self.logger.warning("close run toolbox failed", extra={
                    "run_id": ctx.run_id,
                    "error": str(exc),
                })
            self.active.pop(ctx.run_id, None)

    async def _emit_error(self, ctx: RunContext, code: str, message: str) -> None:
        try:
            await ctx.error(code, message)
        except Exception as exc:
            self.logger.error("runtime error event failed", extra={
                "run_id": ctx.run_id,
                "error": str(exc),
            })

    async def _handle_cancel(self, envelope: runtime_pb2.Envelope) -> None:
        self._cancel_run(envelope.run_id)

    async def _handle_reconcile(self, envelope: runtime_pb2.Envelope) -> None:
        for run_id in envelope.reconcile.cancel_run_ids:
            self._cancel_run(run_id)

    def _cancel_run(self, run_id: str) -> None:
        ctx = self.active.get(run_id)
        if ctx is None:
            return
        ctx.cancel_event.set()
        if ctx.task is not None and not ctx.task.done():
            ctx.task.cancel()

    def running_runs(self) -> list[tuple[str, int]]:
        """Snapshot for Hello.running_runs."""
        return [
            (ctx.run_id, ctx._lease_epoch)
            for ctx in self.active.values()
        ]

    def running_count(self) -> int:
        """Current active run count for Heartbeat."""
        return len(self.active)

    async def close(self) -> None:
        """Cancel active runs and wait for their tasks."""
        self._closing = True
        tasks: list[asyncio.Task[Any]] = []
        for ctx in list(self.active.values()):
            ctx.cancel_event.set()
            if ctx.task is not None and not ctx.task.done():
                ctx.task.cancel()
                tasks.append(ctx.task)
        if tasks:
            await asyncio.gather(*tasks, return_exceptions=True)
        self.active.clear()


def _parse_resume_value(raw: bytes) -> tuple[Any, bool]:
    if not raw:
        return None, False
    try:
        return json.loads(raw.decode("utf-8")), True
    except (UnicodeDecodeError, json.JSONDecodeError):
        return None, False
