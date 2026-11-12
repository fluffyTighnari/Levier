"""Middleware — agent core type and event tests (P8C.1)."""

from __future__ import annotations

import json
import unittest
from pathlib import Path

from agent_core import (
    ErrorEvent,
    Message,
    MessageCompleted,
    ModelRef,
    RunFinished,
    RunSpec,
    ToolCall,
    ToolFinished,
    ToolResult,
    ToolSpec,
    Usage,
    UsageEvent,
    event_to_dict,
)


class UsageTest(unittest.TestCase):
    def test_total_is_derived(self) -> None:
        usage = Usage(input_tokens=10, output_tokens=5)
        self.assertEqual(usage.total_tokens, 15)

    def test_explicit_total_is_kept(self) -> None:
        usage = Usage(input_tokens=10, output_tokens=5, total_tokens=99)
        self.assertEqual(usage.total_tokens, 99)

    def test_dict_shape(self) -> None:
        self.assertEqual(
            Usage(input_tokens=1, output_tokens=2).to_dict(),
            {
                "input_tokens": 1,
                "output_tokens": 2,
                "total_tokens": 3,
                "cache_read_tokens": 0,
                "cache_write_tokens": 0,
            },
        )


class EventTest(unittest.TestCase):
    def test_event_types_are_stable(self) -> None:
        event = ToolFinished(
            run_id="r1",
            step=2,
            invocation_id="i1",
            tool_name="counter_add",
            ok=True,
            result_json='{"value":2}',
            duration_ms=9,
        )
        payload = event_to_dict(event)
        self.assertEqual(payload["type"], "tool_finished")
        self.assertEqual(payload["invocation_id"], "i1")
        json.dumps(payload)

    def test_usage_event_serializes_nested_usage(self) -> None:
        payload = event_to_dict(UsageEvent(run_id="r1", step=1, usage=Usage(input_tokens=2)))
        self.assertEqual(payload["usage"]["total_tokens"], 2)

    def test_error_and_finish(self) -> None:
        error = event_to_dict(ErrorEvent(run_id="r1", code="STEP_TIMEOUT", message="slow"))
        self.assertEqual(error["type"], "error")
        finish = event_to_dict(RunFinished(run_id="r1", status="completed", steps=3))
        self.assertEqual(finish["status"], "completed")


class TypeTest(unittest.TestCase):
    def test_run_spec_defaults(self) -> None:
        spec = RunSpec(messages=[Message(role="user", content="hi")])
        self.assertEqual(spec.max_steps, 8)
        self.assertEqual(spec.model, ModelRef())

    def test_tool_result(self) -> None:
        result = ToolResult(ok=False, error_code="TOOL_TIMEOUT", retryable=True)
        self.assertFalse(result.ok)
        self.assertTrue(result.retryable)

    def test_message_completed_carries_calls(self) -> None:
        call = ToolCall(call_id="c1", name="add", arguments_json='{"a":1,"b":2}')
        event = MessageCompleted(run_id="r", step=1, message_id="m", tool_calls=(call,))
        self.assertEqual(event.tool_calls[0].name, "add")


class ImportBoundaryTest(unittest.TestCase):
    def test_core_has_no_middleware_or_mcp_imports(self) -> None:
        root = Path(__file__).resolve().parents[1] / "src" / "agent_core"
        forbidden = ("middleware_harness", "mcp_runtime", "middleware.runtime", "google.protobuf")
        for path in root.rglob("*.py"):
            text = path.read_text()
            for token in forbidden:
                self.assertNotIn(token, text, f"{path} imports {token}")


if __name__ == "__main__":
    unittest.main()
