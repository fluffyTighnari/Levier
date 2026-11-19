"""Levier - ReAct runtime tests.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import asyncio
import json
import unittest

from middleware.runtime.v1 import runtime_pb2

from levier_runtime.channel.envelope import new_envelope, response_envelope
from levier_react_runtime.llm.base import LLMResponse, Message, ModelConfig, ToolCall, ToolSpec
from levier_runtime.manager import ManagerConfig, RunManager
from levier_react_runtime.runtime.react import ReactRuntime
from levier_react_runtime.tools.base import ToolResult


class FakeSender:
    def __init__(self) -> None:
        self.sent: list[runtime_pb2.Envelope] = []

    async def send(self, envelope: runtime_pb2.Envelope) -> None:
        self.sent.append(envelope)

    async def send_response(self, request: runtime_pb2.Envelope, payload, field: str) -> None:
        self.sent.append(response_envelope(request, payload, field))


class ScriptedProvider:
    def __init__(self, responses: list[LLMResponse | Exception]) -> None:
        self.responses = list(responses)
        self.calls: list[tuple[list[Message], list[ToolSpec]]] = []

    async def complete(self, messages, tools, model, *, on_delta=None) -> LLMResponse:
        self.calls.append((list(messages), list(tools)))
        self.model = model
        response = self.responses.pop(0)
        if isinstance(response, Exception):
            raise response
        return response


class BlockingProvider:
    def __init__(self) -> None:
        self.started = asyncio.Event()

    async def complete(self, messages, tools, model, *, on_delta=None) -> LLMResponse:
        self.started.set()
        await asyncio.Event().wait()
        return LLMResponse(content="never")


class SlowProvider:
    async def complete(self, messages, tools, model, *, on_delta=None) -> LLMResponse:
        await asyncio.sleep(1)
        return LLMResponse(content="late")


class StreamingProvider:
    async def complete(self, messages, tools, model, *, on_delta=None) -> LLMResponse:
        for text in ("Hel", "lo"):
            if on_delta is not None:
                await on_delta(text)
        return LLMResponse(
            content="Hello",
            usage={"input_tokens": 4, "output_tokens": 2},
            model=model.model,
        )


class FakeTools:
    def __init__(self, result: ToolResult) -> None:
        self.result = result
        self.calls: list[ToolCall] = []

    async def __call__(self, ctx, call: ToolCall, timeout_ms: int) -> ToolResult:
        self.calls.append(call)
        return self.result


def assign_envelope(
    *,
    params: dict[str, object] | None = None,
    tools: list[str] | None = None,
) -> runtime_pb2.Envelope:
    assign = runtime_pb2.TaskAssign(
        run_id="run-react",
        app_id="app-1",
        input=[
            runtime_pb2.ChatMessage(role="system", content="be brief"),
            runtime_pb2.ChatMessage(role="user", content="hello"),
        ],
        model=runtime_pb2.ModelProfile(
            provider="anthropic",
            model="claude-test",
            api_key="test-key",
        ),
        tools=[
            runtime_pb2.ToolManifest(
                name=name,
                description=f"{name} tool",
                input_schema_json='{"type":"object"}',
            )
            for name in (tools or [])
        ],
    )
    if params:
        assign.params.update(params)
    return new_envelope(assign, "task_assign", run_id="run-react", attempt=1, lease_epoch=1)


def payload_type(envelope: runtime_pb2.Envelope) -> str:
    return envelope.WhichOneof("payload")


class ReactRuntimeTest(unittest.IsolatedAsyncioTestCase):
    async def asyncTearDown(self) -> None:
        if hasattr(self, "manager"):
            await self.manager.close()

    async def wait_until(self, predicate, timeout: float = 1.0) -> None:
        async def inner() -> None:
            while not predicate():
                await asyncio.sleep(0.005)

        await asyncio.wait_for(inner(), timeout)

    async def start(self, provider, tools=None, **kwargs) -> FakeSender:
        sender = FakeSender()
        runner = ReactRuntime(provider, tools)
        self.manager = RunManager(
            sender,
            ManagerConfig(runtime_name="react1"),
            runner,
        )
        await self.manager.handle(assign_envelope(**kwargs))
        await self.wait_until(lambda: self.manager.running_count() == 0)
        return sender

    def events(self, sender: FakeSender) -> list[runtime_pb2.Envelope]:
        return [env for env in sender.sent if payload_type(env) == "event"]

    async def test_react_completes_without_tools(self) -> None:
        provider = ScriptedProvider([
            LLMResponse(
                content="hi there",
                usage={"input_tokens": 3, "output_tokens": 2},
                model="fake-1",
            ),
        ])
        sender = await self.start(provider)

        events = self.events(sender)
        self.assertEqual(
            [event.event.type for event in events],
            [
                runtime_pb2.EventType.RUN_STATUS,
                runtime_pb2.EventType.USAGE,
                runtime_pb2.EventType.MESSAGE_COMPLETED,
                runtime_pb2.EventType.RUN_STATUS,
            ],
        )
        self.assertEqual([event.event.seq for event in events], [1, 2, 3, 4])
        message = events[2].event.data
        self.assertEqual(message.fields["content"].string_value, "hi there")
        self.assertEqual(message.fields["role"].string_value, "assistant")
        self.assertEqual(message.fields["step"].number_value, 1)
        self.assertEqual(events[-1].event.data.fields["status"].string_value, "completed")
        self.assertEqual(len(provider.calls), 1)

    async def test_react_executes_tool_and_continues(self) -> None:
        provider = ScriptedProvider([
            LLMResponse(
                tool_calls=[
                    ToolCall(call_id="call-1", name="add", arguments_json='{"a":1,"b":2}'),
                ],
            ),
            LLMResponse(content="the answer is 3"),
        ])
        tools = FakeTools(ToolResult(ok=True, result_json="3"))
        sender = await self.start(provider, tools)

        events = self.events(sender)
        self.assertEqual(
            [event.event.type for event in events],
            [
                runtime_pb2.EventType.RUN_STATUS,
                runtime_pb2.EventType.MESSAGE_COMPLETED,
                runtime_pb2.EventType.TOOL_CALL_BEGIN,
                runtime_pb2.EventType.TOOL_CALL_END,
                runtime_pb2.EventType.MESSAGE_COMPLETED,
                runtime_pb2.EventType.RUN_STATUS,
            ],
        )
        begin = events[2].event.data
        end = events[3].event.data
        self.assertEqual(begin.fields["tool_name"].string_value, "add")
        self.assertTrue(begin.fields["invocation_id"].string_value)
        self.assertTrue(end.fields["ok"].bool_value)
        self.assertEqual(end.fields["result_json"].string_value, "3")
        self.assertEqual(events[-1].event.data.fields["status"].string_value, "completed")

        self.assertEqual(len(provider.calls), 2)
        second_messages = provider.calls[1][0]
        tool_message = next(message for message in second_messages if message.role == "tool")
        self.assertEqual(tool_message.tool_call_id, "call-1")
        self.assertEqual(tool_message.name, "add")
        self.assertEqual(tool_message.content, "3")
        self.assertEqual(tools.calls[0].invocation_id, begin.fields["invocation_id"].string_value)

    async def test_react_tool_error_is_fed_back(self) -> None:
        provider = ScriptedProvider([
            LLMResponse(tool_calls=[ToolCall(call_id="call-1", name="add")]),
            LLMResponse(content="recovered"),
        ])
        tools = FakeTools(
            ToolResult(
                ok=False,
                error_code="TOOL_TIMEOUT",
                error_message="too slow",
                retryable=True,
            )
        )
        sender = await self.start(provider, tools)

        events = self.events(sender)
        end = next(
            event for event in events
            if event.event.type == runtime_pb2.EventType.TOOL_CALL_END
        )
        self.assertFalse(end.event.data.fields["ok"].bool_value)
        self.assertEqual(end.event.data.fields["error_code"].string_value, "TOOL_TIMEOUT")
        self.assertTrue(end.event.data.fields["retryable"].bool_value)

        tool_message = next(
            message for message in provider.calls[1][0] if message.role == "tool"
        )
        error = json.loads(tool_message.content)
        self.assertEqual(error["code"], "TOOL_TIMEOUT")
        self.assertEqual(events[-1].event.data.fields["status"].string_value, "completed")

    async def test_react_max_steps(self) -> None:
        provider = ScriptedProvider([
            LLMResponse(tool_calls=[ToolCall(call_id=f"call-{n}", name="loop")])
            for n in range(2)
        ])
        tools = FakeTools(ToolResult(ok=True, result_json="ok"))
        sender = await self.start(
            provider,
            tools,
            params={"max_steps": 2},
        )

        events = self.events(sender)
        types = [event.event.type for event in events]
        self.assertEqual(types.count(runtime_pb2.EventType.TOOL_CALL_BEGIN), 2)
        error = next(event for event in events if event.event.type == runtime_pb2.EventType.ERROR)
        self.assertEqual(error.event.data.fields["code"].string_value, "MAX_STEPS_EXCEEDED")
        self.assertEqual(events[-1].event.data.fields["status"].string_value, "failed")
        self.assertEqual(len(provider.calls), 2)

    async def test_react_step_timeout(self) -> None:
        sender = await self.start(
            SlowProvider(),
            params={"step_timeout_ms": 10},
        )

        events = self.events(sender)
        error = next(event for event in events if event.event.type == runtime_pb2.EventType.ERROR)
        self.assertEqual(error.event.data.fields["code"].string_value, "STEP_TIMEOUT")
        self.assertEqual(events[-1].event.data.fields["status"].string_value, "failed")

    async def test_react_provider_error(self) -> None:
        sender = await self.start(ScriptedProvider([RuntimeError("boom")]))

        events = self.events(sender)
        error = next(event for event in events if event.event.type == runtime_pb2.EventType.ERROR)
        self.assertEqual(error.event.data.fields["code"].string_value, "LLM_ERROR")
        self.assertEqual(error.event.data.fields["message"].string_value, "boom")
        self.assertEqual(events[-1].event.data.fields["status"].string_value, "failed")

    async def test_react_streams_delta_events(self) -> None:
        sender = await self.start(StreamingProvider())

        events = self.events(sender)
        deltas = [
            event for event in events
            if event.event.type == runtime_pb2.EventType.MESSAGE_DELTA
        ]
        self.assertEqual(
            [event.event.data.fields["delta"].string_value for event in deltas],
            ["Hel", "lo"],
        )
        self.assertTrue(all(event.event.ephemeral for event in deltas))
        self.assertTrue(all(event.event.seq == 0 for event in deltas))
        self.assertTrue(all(event.event.source == "daemon" for event in deltas))
        message = next(
            event for event in events
            if event.event.type == runtime_pb2.EventType.MESSAGE_COMPLETED
        )
        self.assertEqual(message.event.data.fields["content"].string_value, "Hello")
        self.assertEqual(
            message.event.data.fields["message_id"].string_value,
            deltas[0].event.data.fields["message_id"].string_value,
        )
        persistent = [event.event.seq for event in events if not event.event.ephemeral]
        self.assertEqual(persistent, [1, 2, 3, 4])

    async def test_react_passes_model_config(self) -> None:
        provider = ScriptedProvider([LLMResponse(content="ok")])
        await self.start(provider)
        self.assertEqual(
            provider.model,
            ModelConfig(
                provider="anthropic",
                model="claude-test",
                api_key="test-key",
            ),
        )

    async def test_react_cancel_interrupts(self) -> None:
        provider = BlockingProvider()
        sender = FakeSender()
        runner = ReactRuntime(provider)
        self.manager = RunManager(
            sender,
            ManagerConfig(runtime_name="react1"),
            runner,
        )
        await self.manager.handle(assign_envelope())
        await asyncio.wait_for(provider.started.wait(), 1)
        cancel = new_envelope(
            runtime_pb2.Cancel(reason="user"),
            "cancel",
            run_id="run-react",
            attempt=1,
            lease_epoch=1,
        )
        await self.manager.handle(cancel)
        await self.wait_until(lambda: self.manager.running_count() == 0)

        events = self.events(sender)
        self.assertEqual(events[-1].event.data.fields["status"].string_value, "cancelled")
