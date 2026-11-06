"""Middleware — TaskAssign/Cancel/Reconcile lifecycle manager.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import asyncio
import logging
from dataclasses import dataclass
from typing import Any, Protocol, Sequence

from middleware.runtime.v1 import runtime_pb2

from middleware_harness.runtime.checkpoint import CheckpointError, CheckpointState
from middleware_harness.runtime.events import EventSender, EventWriter
from middleware_harness.runtime.run import RunContext
from middleware_harness.strategies.echo import EchoStrategy


class Runner(Protocol):
    """Executes one run context; strategy work arrives in P3-26."""

    async def __call__(self, ctx: RunContext) -> str:
        """Return a terminal status: completed or failed."""


class NoopRunner:
    """Minimal runner used until an echo/react strategy exists."""

    async def __call__(self, ctx: RunContext) -> str:
        return "completed"


@dataclass
class ManagerConfig:
    """Limits for accepted runs."""

    max_concurrent_runs: int = 4
    supported_strategies: Sequence[str] = ("react", "echo")
    runtime_type: str = "normal"
    checkpoint_every_events: int = 20
    checkpoint_every_seconds: float = 60.0


class RunManager:
    """Tracks active runs and handles TaskAssign/Cancel/Reconcile."""

    def __init__(
        self,
        sender: EventSender,
        config: ManagerConfig | None = None,
        runner: Runner | None = None,
        logger: logging.Logger | None = None,
    ) -> None:
        self.sender = sender
        self.config = config or ManagerConfig()
        self.runner = runner or EchoStrategy()
        self.logger = logger or logging.getLogger("middleware_harness.runtime")
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
            if existing.lease_epoch == envelope.lease_epoch:
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

        resume, reason = self._resume_state(assign)
        if reason:
            await self._send_ack(envelope, accepted=False, reason=reason)
            return

        await self._send_ack(envelope, accepted=True)
        events = EventWriter(self.sender, assign.run_id, envelope.attempt, envelope.lease_epoch)
        ctx = RunContext(
            assign=assign,
            attempt=envelope.attempt,
            lease_epoch=envelope.lease_epoch,
            events=events,
            resume=resume,
            checkpoint_enabled=self.config.runtime_type == "longterm",
            checkpoint_every_events=self.config.checkpoint_every_events,
            checkpoint_every_seconds=self.config.checkpoint_every_seconds,
        )
        self.active[assign.run_id] = ctx
        await ctx.emit(
            runtime_pb2.EventType.RUN_STATUS,
            {"status": "running"},
        )
        ctx.task = asyncio.create_task(self._execute(ctx), name=f"run-{assign.run_id}")

    def _resume_state(self, assign: runtime_pb2.TaskAssign) -> tuple[CheckpointState | None, str]:
        """Decode resume_from and verify it belongs to this assignment."""
        data = assign.resume_from.state
        if not data:
            return None, ""
        if self.config.runtime_type != "longterm":
            return None, "resume_from requires longterm runtime"
        try:
            state = CheckpointState.decode(data)
        except CheckpointError as exc:
            return None, f"invalid checkpoint: {exc}"
        if state.run_id != assign.run_id:
            return None, "checkpoint run_id mismatch"
        if not assign.session_id:
            return None, "resume_from requires session_id"
        if state.session_id != assign.session_id:
            return None, "checkpoint session_id mismatch"
        if state.strategy != assign.strategy:
            return None, "checkpoint strategy mismatch"
        return state, ""

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
        if not assign.strategy:
            return "strategy is required"
        if assign.strategy not in self.config.supported_strategies:
            return f"unsupported strategy {assign.strategy!r}"
        if envelope.attempt < 1:
            return "attempt must be >= 1"
        if envelope.lease_epoch < 1:
            return "lease_epoch must be >= 1"
        if self._closing:
            return "daemon draining"
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
        try:
            status = await self.runner(ctx)
            if ctx.cancelled:
                status = "cancelled"
        except asyncio.CancelledError:
            status = "cancelled"
        except Exception as exc:  # runner failures become a failed run event
            status = "failed"
            reason = str(exc)
            self.logger.error("run failed", extra={
                "run_id": ctx.run_id,
                "error": str(exc),
            })
        finally:
            if ctx.cancelled:
                status = "cancelled"
            ctx.outcome = status
            data: dict[str, Any] = {"status": status}
            if reason:
                data["reason"] = reason
            try:
                await ctx.emit(runtime_pb2.EventType.RUN_STATUS, data)
            except Exception as exc:
                self.logger.error("run terminal event failed", extra={
                    "run_id": ctx.run_id,
                    "error": str(exc),
                })
            self.active.pop(ctx.run_id, None)

    async def _handle_cancel(self, envelope: runtime_pb2.Envelope) -> None:
        ctx = self.active.get(envelope.run_id)
        if ctx is None:
            return
        ctx.cancel_event.set()
        if ctx.task is not None and not ctx.task.done():
            ctx.task.cancel()

    async def _handle_reconcile(self, envelope: runtime_pb2.Envelope) -> None:
        for run_id in envelope.reconcile.cancel_run_ids:
            ctx = self.active.get(run_id)
            if ctx is None:
                continue
            ctx.cancel_event.set()
            if ctx.task is not None and not ctx.task.done():
                ctx.task.cancel()

    def running_runs(self) -> list[tuple[str, int]]:
        """Snapshot for Hello.running_runs."""
        return [
            (ctx.run_id, ctx.lease_epoch)
            for ctx in self.active.values()
        ]

    def running_count(self) -> int:
        """Current active run count for Heartbeat."""
        return len(self.active)

    async def close(self) -> None:
        """Cancel all active runs and wait for their tasks."""
        self._closing = True
        tasks = []
        for ctx in list(self.active.values()):
            ctx.cancel_event.set()
            if ctx.task is not None and not ctx.task.done():
                ctx.task.cancel()
                tasks.append(ctx.task)
        if tasks:
            await asyncio.gather(*tasks, return_exceptions=True)
        self.active.clear()
