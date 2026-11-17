"""Levier — agent core loop tests (P8C.2)."""

from __future__ import annotations

import asyncio
import unittest

from levier_agent_core import (
    AgentCore,
    ErrorEvent,
    LLMResponse,
    Message,
    ModelRef,
    ProviderFailure,
    RunFinished,
    RunSpec,
    ToolCall,
    ToolResult,
    ToolSpec,
    Usage,
)


class ScriptedProvider:
    def __init__(self, responses):
        self.responses = list(responses)
        self.calls = []

    async def complete(self, messages, tools, model, *, on_delta=None):
        self.calls.append((list(messages), list(tools), model))
        response = self.responses.pop(0)
        if isinstance(response, Exception):
            raise response
        if on_delta is not None and response.content:
            for chunk in response.content.split("|"):
                await on_delta(chunk)
        return response


class FakeTools:
    def __init__(self, results):
        self.results = list(results)
        self.calls = []

    async def call(self, call):
        self.calls.append(call)
        result = self.results.pop(0)
        if isinstance(result, Exception):
            raise result
        return result


class FakeAbort:
    def __init__(self, value=False):
        self.value = value

    def is_set(self):
        return self.value


class LoopTest(unittest.IsolatedAsyncioTestCase):
    async def run_spec(self, spec, provider, tools, *, abort=None):
        events = []

        async def emit(event):
            events.append(event)

        core = AgentCore(provider, tools, new_id=lambda: "id-1")
        result = await core.run(spec, emit, abort=abort)
        return result, events

    def spec(self, **kwargs):
        return RunSpec(
            run_id="run-1",
            messages=[Message(role="user", content="hi")],
            tools=[ToolSpec(name="add")],
            model=ModelRef(provider="fake", model="m"),
            **kwargs,
        )

    async def test_completes_without_tools(self):
        provider = ScriptedProvider([LLMResponse(content="done", usage=Usage(input_tokens=2, output_tokens=1))])
        result, events = await self.run_spec(self.spec(), provider, FakeTools([]))
        self.assertEqual(result.status, "completed")
        self.assertEqual(result.steps, 1)
        self.assertEqual(result.usage.total_tokens, 3)
        types = [event.to_dict()["type"] for event in events]
        self.assertEqual(types, ["run_started", "step_started", "message_delta",
                                 "usage", "message_completed", "run_finished"])
        self.assertEqual(events[-1].status, "completed")

    async def test_tool_call_and_continuation(self):
        provider = ScriptedProvider([
            LLMResponse(tool_calls=[ToolCall(call_id="c1", name="add", arguments_json='{"a":1,"b":2}')]),
            LLMResponse(content="3"),
        ])
        tools = FakeTools([ToolResult(ok=True, result_json="3")])
        result, events = await self.run_spec(self.spec(), provider, tools)
        self.assertEqual(result.status, "completed")
        self.assertEqual(result.steps, 2)
        self.assertEqual(tools.calls[0].invocation_id, "id-1")
        finish = [event for event in events if event.to_dict()["type"] == "tool_finished"][0]
        self.assertTrue(finish.ok)
        # provider receives the tool message back
        messages = provider.calls[1][0]
        self.assertEqual(messages[-1].role, "tool")
        self.assertEqual(messages[-1].tool_call_id, "c1")

    async def test_tool_error_is_fed_back(self):
        provider = ScriptedProvider([
            LLMResponse(tool_calls=[ToolCall(call_id="c1", name="add")]),
            LLMResponse(content="recovered"),
        ])
        tools = FakeTools([ToolResult(ok=False, error_code="TOOL_TIMEOUT", error_message="slow", retryable=True)])
        result, _ = await self.run_spec(self.spec(), provider, tools)
        self.assertEqual(result.status, "completed")
        tool_message = provider.calls[1][0][-1]
        self.assertIn("TOOL_TIMEOUT", tool_message.content)

    async def test_max_steps(self):
        provider = ScriptedProvider([
            LLMResponse(tool_calls=[ToolCall(call_id="c1", name="add")]),
            LLMResponse(tool_calls=[ToolCall(call_id="c2", name="add")]),
        ])
        result, events = await self.run_spec(self.spec(max_steps=2), provider, FakeTools([
            ToolResult(ok=True, result_json="1"),
            ToolResult(ok=True, result_json="1"),
        ]))
        self.assertEqual(result.status, "failed")
        error = [event for event in events if isinstance(event, ErrorEvent)][0]
        self.assertEqual(error.code, "MAX_STEPS")

    async def test_step_timeout(self):
        class Slow:
            async def complete(self, *args, **kwargs):
                await asyncio.sleep(1)
                return LLMResponse(content="late")

        result, events = await self.run_spec(self.spec(step_timeout_ms=10), Slow(), FakeTools([]))
        self.assertEqual(result.status, "failed")
        error = [event for event in events if isinstance(event, ErrorEvent)][0]
        self.assertEqual(error.code, "STEP_TIMEOUT")

    async def test_provider_failure(self):
        provider = ScriptedProvider([ProviderFailure("RATE_LIMITED", "slow down", retryable=True)])
        result, events = await self.run_spec(self.spec(), provider, FakeTools([]))
        self.assertEqual(result.status, "failed")
        error = [event for event in events if isinstance(event, ErrorEvent)][0]
        self.assertEqual(error.code, "RATE_LIMITED")
        self.assertTrue(error.retryable)

    async def test_abort(self):
        provider = ScriptedProvider([LLMResponse(content="never")])
        result, events = await self.run_spec(self.spec(), provider, FakeTools([]), abort=FakeAbort(True))
        self.assertEqual(result.status, "cancelled")
        self.assertEqual(len(provider.calls), 0)
        self.assertEqual(events[-1].status, "cancelled")


if __name__ == "__main__":
    unittest.main()
