"""Levier — echo strategy.

Returns the last user message without calling an LLM. It is the first
end-to-end strategy for M1 and contract tests.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

from middleware.runtime.v1 import runtime_pb2

from levier_harness.ids import new_ulid
from levier_harness.llm.base import Message
from levier_harness.runtime.checkpoint import capture
from levier_harness.runtime.run import RunContext


class EchoStrategy:
    """Emit one assistant message and complete the run."""

    name = "echo"

    async def run(self, ctx: RunContext) -> str:
        if ctx.cancelled:
            return "cancelled"
        if ctx.resume is not None:
            return "completed"

        content = "echo"
        if ctx.assign.input:
            content = ctx.assign.input[-1].content or content

        await ctx.emit(
            runtime_pb2.EventType.MESSAGE_COMPLETED,
            {
                "message_id": new_ulid(),
                "role": "assistant",
                "content": content,
            },
        )
        if ctx.assign.session_id:
            messages = [Message.from_proto(message) for message in ctx.assign.input]
            messages.append(Message(role="assistant", content=content))
            await ctx.save_checkpoint(capture(
                session_id=ctx.assign.session_id,
                run_id=ctx.run_id,
                attempt=ctx.attempt,
                strategy=self.name,
                step=1,
                messages=messages,
                last_event_seq=ctx.events.seq,
            ))
        return "completed"

    async def __call__(self, ctx: RunContext) -> str:
        """Allow the strategy to be used as a RunManager runner."""
        return await self.run(ctx)
