"""LangGraph driver and checkpoint adapter tests."""

from __future__ import annotations

import asyncio
import unittest
from typing import Any, TypedDict

from langgraph.graph import END, START, StateGraph
from langgraph.types import interrupt

from levier_langgraph_runtime import LangGraphDriver, LevierCheckpointSaver
from levier_runtime.types import Message


class GraphState(TypedDict):
    value: int
    answer: str


async def add_one(state: GraphState) -> dict[str, int]:
    return {"value": state["value"] + 1}


async def ask(state: GraphState) -> dict[str, str]:
    answer = interrupt({"question": "approve?", "value": state["value"]})
    return {"answer": str(answer)}


async def finish(state: GraphState) -> dict[str, int]:
    return {"value": state["value"] * 10}


def build_graph(saver: LevierCheckpointSaver):
    graph = StateGraph(GraphState)
    graph.add_node("add", add_one)
    graph.add_node("ask", ask)
    graph.add_node("finish", finish)
    graph.add_edge(START, "add")
    graph.add_edge("add", "ask")
    graph.add_edge("ask", "finish")
    graph.add_edge("finish", END)
    return graph.compile(checkpointer=saver)


def build_finish_graph(saver: LevierCheckpointSaver):
    graph = StateGraph(GraphState)
    graph.add_node("add", add_one)
    graph.add_node("finish", finish)
    graph.add_edge(START, "add")
    graph.add_edge("add", "finish")
    graph.add_edge("finish", END)
    return graph.compile(checkpointer=saver)


class FakeContext:
    def __init__(self, *, resume_from: bytes | None = None) -> None:
        self.runtime_name = "graph1"
        self.run_id = "run-1"
        self.session_id = "session-1"
        self.input = [Message(role="user", content="start")]
        self.params: dict[str, Any] = {}
        self.resume_from = resume_from
        self.checkpoint_data: bytes | None = None
        self.checkpoint_calls = 0
        self.events: list[tuple[str, dict[str, Any], str]] = []
        self.errors: list[tuple[str, str, bool]] = []

    async def checkpoint(self, state: bytes) -> None:
        self.checkpoint_data = bytes(state)
        self.checkpoint_calls += 1

    async def emit_custom(
        self,
        name: str,
        data: dict[str, Any],
        *,
        schema_version: str = "1",
    ) -> None:
        self.events.append((name, data, schema_version))

    async def error(self, code: str, message: str, *, retryable: bool = False) -> None:
        self.errors.append((code, message, retryable))


class LangGraphDriverTest(unittest.IsolatedAsyncioTestCase):
    async def test_graph_completes_and_emits_node_events(self) -> None:
        ctx = FakeContext()
        driver = LangGraphDriver(
            build_finish_graph,
            input_builder=lambda _ctx: {"value": 1, "answer": ""},
        )

        status = await driver.run(ctx)

        self.assertEqual(status, "completed")
        self.assertIsNotNone(ctx.checkpoint_data)
        self.assertGreaterEqual(ctx.checkpoint_calls, 1)
        event_names = [name for name, _, _ in ctx.events]
        self.assertIn("graph.node.started", event_names)
        self.assertIn("graph.node.finished", event_names)
        self.assertIn("graph.completed", event_names)
        nodes = {
            data["node"]
            for name, data, _ in ctx.events
            if name == "graph.node.finished"
        }
        self.assertEqual(nodes, {"add", "finish"})

    async def test_interrupt_waits_then_resumes_from_checkpoint(self) -> None:
        first = FakeContext()
        driver = LangGraphDriver(
            build_graph,
            input_builder=lambda _ctx: {"value": 1, "answer": ""},
            resume_value="approved",
        )

        status = await driver.run(first)
        self.assertEqual(status, "waiting")
        self.assertIsNotNone(first.checkpoint_data)
        paused = next(
            data for name, data, _ in first.events
            if name == "graph.interrupted"
        )
        self.assertEqual(paused["next"], ["ask"])
        self.assertEqual(paused["interrupts"][0]["value"]["question"], "approve?")

        second = FakeContext(resume_from=first.checkpoint_data)
        resumed = LangGraphDriver(
            build_graph,
            input_builder=lambda _ctx: {"value": 1, "answer": ""},
            resume_value="approved",
        )
        status = await resumed.run(second)

        self.assertEqual(status, "completed")
        self.assertEqual(second.errors, [])
        self.assertIsNotNone(second.checkpoint_data)
        self.assertIn("graph.completed", [name for name, _, _ in second.events])

    async def test_checkpoint_loader_rejects_wrong_session(self) -> None:
        ctx = FakeContext()
        saver = LevierCheckpointSaver(ctx, thread_id="session-1")
        data = saver.dump()

        other = FakeContext()
        other.session_id = "other-session"
        with self.assertRaises(Exception):
            LevierCheckpointSaver(other, thread_id="other-session").load(data)


if __name__ == "__main__":
    unittest.main()
