"""LangGraph Runtime Driver.

The adapter owns only the boundary between a compiled LangGraph graph and a
Levier RunContext. Graph execution, interrupts, and state semantics remain
LangGraph's; Levier owns sessions, checkpoints, routing, and waiting/resume.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import json
from typing import Any, Callable

from levier_runtime import RunContext, RuntimeCapabilities

from .checkpoint import LangGraphCheckpointError, LevierCheckpointSaver

GraphFactory = Callable[[LevierCheckpointSaver], Any]
InputBuilder = Callable[[RunContext], Any]


def _default_input(ctx: RunContext) -> dict[str, Any]:
    return {
        "messages": [
            {"role": message.role, "content": message.content, "name": message.name}
            for message in ctx.input
        ]
    }


class LangGraphDriver:
    """Run one compiled LangGraph graph through the Levier Runtime SDK."""

    capabilities = RuntimeCapabilities(
        checkpoint=True,
        custom_events=True,
        waiting=True,
        longterm=True,
    )

    def __init__(
        self,
        graph_factory: GraphFactory,
        *,
        input_builder: InputBuilder | None = None,
        resume_value: Any = None,
        recursion_limit: int = 50,
        include_node_output: bool = False,
    ) -> None:
        if not callable(graph_factory):
            raise TypeError("graph_factory must be callable")
        self.graph_factory = graph_factory
        self.input_builder = input_builder or _default_input
        self.resume_value = resume_value
        self.recursion_limit = max(1, int(recursion_limit))
        self.include_node_output = include_node_output

    async def run(self, ctx: RunContext) -> str:
        """Execute the graph and return ``completed`` or ``waiting``."""
        thread_id = ctx.session_id or ctx.run_id
        saver = LevierCheckpointSaver(ctx, thread_id=thread_id)
        resuming = bool(ctx.resume_from)
        try:
            if ctx.resume_from:
                saver.load(ctx.resume_from)
        except LangGraphCheckpointError as exc:
            await ctx.error("INVALID_CHECKPOINT", str(exc), retryable=False)
            return "failed"

        graph = self.graph_factory(saver)
        config = {
            "configurable": {"thread_id": thread_id},
            "recursion_limit": self.recursion_limit,
        }

        graph_input: Any
        if resuming:
            state = await graph.aget_state(config)
            if state.interrupts:
                from langgraph.types import Command

                if getattr(ctx, "resume_value_set", False):
                    resume_value = ctx.resume_value
                else:
                    resume_value = self._resume_value(ctx)
                graph_input = Command(resume=resume_value)
            elif state.next:
                graph_input = None
            else:
                await ctx.error(
                    "CHECKPOINT_NOT_PAUSED",
                    "LangGraph checkpoint has no pending work to resume",
                    retryable=False,
                )
                return "failed"
        else:
            graph_input = self.input_builder(ctx)

        async for event in graph.astream_events(graph_input, config, version="v2"):
            await self._emit_graph_event(ctx, event)

        state = await graph.aget_state(config)
        if ctx.session_id:
            await ctx.checkpoint(saver.dump())

        if state.interrupts or state.next:
            await ctx.emit_custom(
                "graph.interrupted",
                {
                    "next": list(state.next),
                    "interrupts": [
                        _interrupt_payload(interrupt)
                        for interrupt in state.interrupts
                    ],
                },
                schema_version="1",
            )
            return "waiting"

        await ctx.emit_custom(
            "graph.completed",
            {"run_id": ctx.run_id, "thread_id": thread_id},
            schema_version="1",
        )
        return "completed"

    def _resume_value(self, ctx: RunContext) -> Any:
        if callable(self.resume_value):
            return self.resume_value(ctx)
        return self.resume_value

    async def _emit_graph_event(self, ctx: RunContext, event: Any) -> None:
        if not isinstance(event, dict):
            return
        metadata = event.get("metadata")
        if not isinstance(metadata, dict):
            return
        node = metadata.get("langgraph_node")
        if not isinstance(node, str) or not node:
            return
        event_type = event.get("event")
        if event_type not in {"on_chain_start", "on_chain_end"}:
            return
        step = metadata.get("langgraph_step")
        data: dict[str, Any] = {"node": node, "step": step}
        if event_type == "on_chain_start":
            name = "graph.node.started"
        else:
            name = "graph.node.finished"
            if self.include_node_output:
                output = event.get("data")
                if isinstance(output, dict):
                    output = output.get("output")
                if _is_jsonable(output):
                    data["output"] = output
        await ctx.emit_custom(name, data, schema_version="1")


def _interrupt_payload(interrupt: Any) -> dict[str, Any]:
    value = getattr(interrupt, "value", None)
    if not _is_jsonable(value):
        value = str(value)
    return {
        "id": str(getattr(interrupt, "id", "")),
        "value": value,
    }


def _is_jsonable(value: Any) -> bool:
    try:
        json.dumps(value, ensure_ascii=False)
    except (TypeError, ValueError):
        return False
    return True
