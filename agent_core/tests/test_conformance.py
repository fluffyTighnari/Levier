"""Middleware — language-neutral agent core conformance runner (P8C.4)."""

from __future__ import annotations

import asyncio
import json
import unittest
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from agent_core import (
    AgentCore,
    LLMResponse,
    Message,
    ModelRef,
    ProviderFailure,
    RunSpec,
    ToolCall,
    ToolResult,
    ToolSpec,
    Usage,
)

FIXTURES = Path(__file__).resolve().parents[1] / "conformance" / "fixtures"


class ScriptedProvider:
    def __init__(self, script: list[dict[str, Any]]) -> None:
        self.script = list(script)
        self.calls = 0

    async def complete(self, messages, tools, model, *, on_delta=None):
        self.calls += 1
        entry = self.script.pop(0)
        delay = entry.get("sleep_ms", 0)
        if delay:
            await asyncio.sleep(delay / 1000)
        error = entry.get("error")
        if error:
            raise ProviderFailure(
                error["code"], error.get("message", ""), retryable=error.get("retryable", False)
            )
        for delta in entry.get("deltas", []):
            if on_delta is not None:
                await on_delta(delta)
        return LLMResponse(
            content=entry.get("content", ""),
            tool_calls=[
                ToolCall(
                    call_id=call["call_id"],
                    name=call["name"],
                    arguments_json=call.get("arguments_json", ""),
                )
                for call in entry.get("tool_calls", [])
            ],
            usage=Usage(**entry.get("usage", {})),
            model=entry.get("model", ""),
        )


class ScriptedTools:
    def __init__(self, script: list[dict[str, Any]]) -> None:
        self.script = list(script)
        self.calls = 0

    async def call(self, call):
        self.calls += 1
        entry = self.script.pop(0)
        delay = entry.get("sleep_ms", 0)
        if delay:
            await asyncio.sleep(delay / 1000)
        return ToolResult(
            ok=entry.get("ok", False),
            result_json=entry.get("result_json", ""),
            error_code=entry.get("error_code", ""),
            error_message=entry.get("error_message", ""),
            retryable=entry.get("retryable", False),
        )


class AbortFlag:
    def __init__(self) -> None:
        self.value = False

    def is_set(self) -> bool:
        return self.value


class IdGen:
    def __init__(self) -> None:
        self.count = 0

    def __call__(self) -> str:
        self.count += 1
        return f"id-{self.count}"


def normalize(event) -> dict[str, Any]:
    payload = json.loads(json.dumps(event.to_dict()))
    payload.pop("ts_ms", None)
    return payload


def assert_subset(expected: Any, actual: Any, path: str = "$") -> None:
    if isinstance(expected, dict):
        if not isinstance(actual, dict):
            raise AssertionError(f"{path}: expected object, got {type(actual).__name__}")
        for key, value in expected.items():
            if key not in actual:
                raise AssertionError(f"{path}.{key}: missing")
            assert_subset(value, actual[key], f"{path}.{key}")
        return
    if isinstance(expected, list):
        if not isinstance(actual, list) or len(actual) != len(expected):
            raise AssertionError(f"{path}: expected {expected!r}, got {actual!r}")
        for index, value in enumerate(expected):
            assert_subset(value, actual[index], f"{path}[{index}]")
        return
    if expected != actual:
        raise AssertionError(f"{path}: expected {expected!r}, got {actual!r}")


def build_spec(data: dict[str, Any]) -> RunSpec:
    return RunSpec(
        run_id=data["run_id"],
        start_step=data.get("start_step", 1),
        messages=[Message(**message) for message in data.get("messages", [])],
        tools=[ToolSpec(**tool) for tool in data.get("tools", [])],
        model=ModelRef(**data.get("model", {})),
        max_steps=data.get("max_steps", 4),
        step_timeout_ms=data.get("step_timeout_ms", 1000),
        tool_timeout_ms=data.get("tool_timeout_ms", 1000),
        session_id=data.get("session_id", ""),
    )


class ConformanceTest(unittest.IsolatedAsyncioTestCase):
    async def test_fixtures(self) -> None:
        paths = sorted(FIXTURES.glob("*.json"))
        self.assertTrue(paths, "no conformance fixtures")
        for path in paths:
            with self.subTest(fixture=path.name):
                await self.run_fixture(json.loads(path.read_text()))

    async def run_fixture(self, data: dict[str, Any]) -> None:
        provider = ScriptedProvider(data["provider_script"])
        tools = ScriptedTools(data.get("tool_script", []))
        abort = AbortFlag()
        events: list[dict[str, Any]] = []

        async def emit(event) -> None:
            events.append(normalize(event))

        async def on_step_end(_messages, step: int) -> None:
            if data.get("abort_after_steps", 0) and step >= data["abort_after_steps"]:
                abort.value = True

        core = AgentCore(provider, tools, new_id=IdGen())
        result = await core.run(
            build_spec(data),
            emit,
            abort=abort,
            on_step_end=on_step_end,
        )

        expected = data["expected"]
        self.assertEqual(result.status, expected["status"])
        self.assertEqual(result.steps, expected["steps"])
        self.assertEqual(provider.calls, expected.get("provider_calls", provider.calls))
        self.assertEqual(tools.calls, expected.get("tool_calls", tools.calls))
        self.assertEqual(len(events), len(expected["events"]))
        for index, expected_event in enumerate(expected["events"]):
            assert_subset(expected_event, events[index], f"events[{index}]")
        for needle in expected.get("tool_message_contains", []):
            contents = [m.content for m in result.messages if m.role == "tool"]
            self.assertTrue(any(needle in content for content in contents), needle)


if __name__ == "__main__":
    unittest.main()
