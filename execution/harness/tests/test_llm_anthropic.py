"""Levier — Anthropic provider tests.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import asyncio
import json
import unittest
from typing import AsyncIterator

from middleware.runtime.v1 import runtime_pb2

from levier_runtime.channel.envelope import new_envelope, response_envelope
from levier_harness.llm import (
    AnthropicProvider,
    HTTPRequest,
    HTTPResponse,
    LLMError,
    Message,
    ModelConfig,
    ToolCall,
    ToolSpec,
)
from levier_runtime.manager import ManagerConfig, RunManager
from levier_harness.runtime.react import ReactRuntime


class FakeTransport:
    def __init__(
        self,
        status: int = 200,
        headers: dict[str, str] | None = None,
        body: bytes = b"",
        *,
        content_type: str = "text/event-stream",
    ) -> None:
        self.status = status
        self.headers = {"content-type": content_type, **(headers or {})}
        self.body = body
        self.requests: list[HTTPRequest] = []

    async def open(self, request: HTTPRequest) -> HTTPResponse:
        self.requests.append(request)

        async def chunks() -> AsyncIterator[bytes]:
            yield self.body

        return HTTPResponse(status=self.status, headers=self.headers, body=chunks())


def sse(events: list[dict[str, object]]) -> bytes:
    return b"".join(
        f"event: {event['type']}\ndata: {json.dumps(event)}\n\n".encode("utf-8")
        for event in events
    )


def text_stream(text: str) -> bytes:
    return sse([
        {
            "type": "message_start",
            "message": {
                "id": "msg_1",
                "model": "claude-test",
                "usage": {"input_tokens": 11, "output_tokens": 0},
            },
        },
        {
            "type": "content_block_start",
            "index": 0,
            "content_block": {"type": "text", "text": ""},
        },
        {
            "type": "content_block_delta",
            "index": 0,
            "delta": {"type": "text_delta", "text": text},
        },
        {
            "type": "content_block_stop",
            "index": 0,
        },
        {
            "type": "message_delta",
            "delta": {"stop_reason": "end_turn"},
            "usage": {"output_tokens": 5},
        },
        {"type": "message_stop"},
    ])


MODEL = ModelConfig(provider="anthropic", model="claude-test", api_key="test-key")


class AnthropicProviderTest(unittest.IsolatedAsyncioTestCase):
    async def test_stream_text_and_tool_call(self) -> None:
        body = sse([
            {
                "type": "message_start",
                "message": {
                    "id": "msg_1",
                    "model": "claude-test",
                    "usage": {"input_tokens": 11, "output_tokens": 0},
                },
            },
            {
                "type": "content_block_start",
                "index": 0,
                "content_block": {"type": "text", "text": ""},
            },
            {
                "type": "content_block_delta",
                "index": 0,
                "delta": {"type": "text_delta", "text": "Hel"},
            },
            {
                "type": "content_block_delta",
                "index": 0,
                "delta": {"type": "text_delta", "text": "lo"},
            },
            {
                "type": "content_block_start",
                "index": 1,
                "content_block": {
                    "type": "tool_use",
                    "id": "toolu_1",
                    "name": "add",
                    "input": {},
                },
            },
            {
                "type": "content_block_delta",
                "index": 1,
                "delta": {
                    "type": "input_json_delta",
                    "partial_json": '{"a":1,"b":2}',
                },
            },
            {
                "type": "message_delta",
                "delta": {"stop_reason": "tool_use"},
                "usage": {"output_tokens": 7},
            },
            {"type": "message_stop"},
        ])
        provider = AnthropicProvider(FakeTransport(body=body))
        deltas: list[str] = []

        async def on_delta(text: str) -> None:
            deltas.append(text)

        response = await provider.complete([Message(role="user", content="hi")], [], MODEL, on_delta=on_delta)
        self.assertEqual(response.content, "Hello")
        self.assertEqual(deltas, ["Hel", "lo"])
        self.assertEqual(response.model, "claude-test")
        self.assertEqual(response.usage, {"input_tokens": 11, "output_tokens": 7})
        self.assertEqual(len(response.tool_calls), 1)
        self.assertEqual(response.tool_calls[0].call_id, "toolu_1")
        self.assertEqual(response.tool_calls[0].name, "add")
        self.assertEqual(json.loads(response.tool_calls[0].arguments_json), {"a": 1, "b": 2})

    async def test_json_response_fallback(self) -> None:
        body = json.dumps({
            "model": "claude-test",
            "content": [
                {"type": "text", "text": "done"},
                {"type": "tool_use", "id": "toolu_1", "name": "add", "input": {"a": 1}},
            ],
            "usage": {"input_tokens": 3, "output_tokens": 4},
        }).encode("utf-8")
        provider = AnthropicProvider(FakeTransport(body=body, content_type="application/json"))

        response = await provider.complete([Message(role="user", content="hi")], [], MODEL)
        self.assertEqual(response.content, "done")
        self.assertEqual(response.tool_calls[0].arguments_json, '{"a":1}')
        self.assertEqual(response.usage, {"input_tokens": 3, "output_tokens": 4})

    async def test_http_rate_limit_is_retryable(self) -> None:
        body = json.dumps({
            "type": "error",
            "error": {"type": "rate_limit_error", "message": "slow down"},
        }).encode("utf-8")
        provider = AnthropicProvider(FakeTransport(status=429, body=body))

        with self.assertRaises(LLMError) as caught:
            await provider.complete([Message(role="user", content="hi")], [], MODEL)
        self.assertEqual(caught.exception.code, "RATE_LIMITED")
        self.assertTrue(caught.exception.retryable)
        self.assertEqual(caught.exception.status, 429)

    async def test_http_auth_error_is_not_retryable(self) -> None:
        body = json.dumps({
            "type": "error",
            "error": {"type": "authentication_error", "message": "bad key"},
        }).encode("utf-8")
        provider = AnthropicProvider(FakeTransport(status=401, body=body))

        with self.assertRaises(LLMError) as caught:
            await provider.complete([Message(role="user", content="hi")], [], MODEL)
        self.assertEqual(caught.exception.code, "AUTH_FAILED")
        self.assertFalse(caught.exception.retryable)

    async def test_stream_error_is_mapped(self) -> None:
        body = sse([
            {
                "type": "error",
                "error": {"type": "overloaded_error", "message": "busy"},
            },
        ])
        provider = AnthropicProvider(FakeTransport(body=body))

        with self.assertRaises(LLMError) as caught:
            await provider.complete([Message(role="user", content="hi")], [], MODEL)
        self.assertEqual(caught.exception.code, "PROVIDER_UNAVAILABLE")
        self.assertTrue(caught.exception.retryable)

    async def test_build_request_converts_messages_and_tools(self) -> None:
        transport = FakeTransport(body=text_stream("ok"))
        provider = AnthropicProvider(transport)
        model = ModelConfig(
            provider="anthropic",
            model="claude-test",
            api_key="test-key",
            base_url="https://example.test/v1/messages",
            params={"max_tokens": "1024", "temperature": "0.2"},
        )
        messages = [
            Message(role="system", content="be brief"),
            Message(role="user", content="hi"),
            Message(
                role="assistant",
                content="thinking",
                tool_calls=(ToolCall(call_id="toolu_1", name="add", arguments_json='{"a":1}'),),
            ),
            Message(role="tool", content="3", tool_call_id="toolu_1", name="add"),
        ]
        tools = [
            ToolSpec(
                name="add",
                description="add numbers",
                input_schema_json='{"type":"object","properties":{"a":{"type":"number"}}}',
            ),
        ]

        await provider.complete(messages, tools, model)

        request = transport.requests[0]
        body = json.loads(request.body)
        self.assertEqual(request.url, "https://example.test/v1/messages")
        self.assertEqual(request.headers["x-api-key"], "test-key")
        self.assertEqual(request.headers["anthropic-version"], "2023-06-01")
        self.assertEqual(body["system"], "be brief")
        self.assertEqual(body["max_tokens"], 1024)
        self.assertEqual(body["temperature"], 0.2)
        self.assertEqual(body["messages"][0], {"role": "user", "content": "hi"})
        self.assertEqual(body["messages"][1]["role"], "assistant")
        self.assertEqual(body["messages"][1]["content"][0], {"type": "text", "text": "thinking"})
        self.assertEqual(body["messages"][1]["content"][1]["type"], "tool_use")
        self.assertEqual(body["messages"][2]["content"][0]["type"], "tool_result")
        self.assertEqual(body["messages"][2]["content"][0]["tool_use_id"], "toolu_1")
        self.assertEqual(body["tools"][0]["name"], "add")
        self.assertEqual(body["tools"][0]["input_schema"]["type"], "object")

    async def test_missing_model_is_rejected_before_http(self) -> None:
        provider = AnthropicProvider(FakeTransport(body=text_stream("ok")))
        with self.assertRaises(LLMError) as caught:
            await provider.complete(
                [Message(role="user", content="hi")],
                [],
                ModelConfig(provider="anthropic"),
            )
        self.assertEqual(caught.exception.code, "BAD_REQUEST")
        self.assertFalse(caught.exception.retryable)


class FakeSender:
    def __init__(self) -> None:
        self.sent: list[runtime_pb2.Envelope] = []

    async def send(self, envelope: runtime_pb2.Envelope) -> None:
        self.sent.append(envelope)

    async def send_response(self, request: runtime_pb2.Envelope, payload, field: str) -> None:
        self.sent.append(response_envelope(request, payload, field))


class AnthropicReactIntegrationTest(unittest.IsolatedAsyncioTestCase):
    async def test_streamed_react_run_emits_deltas(self) -> None:
        transport = FakeTransport(body=text_stream("Hello"))
        provider = AnthropicProvider(transport)
        sender = FakeSender()
        manager = RunManager(
            sender,
            ManagerConfig(runtime_name="react1"),
            ReactRuntime(provider),
        )
        assign = runtime_pb2.TaskAssign(
            run_id="run-1",
            app_id="app-1",
            input=[runtime_pb2.ChatMessage(role="user", content="hi")],
            model=runtime_pb2.ModelProfile(
                provider="anthropic",
                model="claude-test",
                api_key="test-key",
            ),
        )
        envelope = new_envelope(assign, "task_assign", run_id="run-1", attempt=1, lease_epoch=1)
        try:
            await manager.handle(envelope)
            for _ in range(200):
                if manager.running_count() == 0:
                    break
                await asyncio.sleep(0.005)
        finally:
            await manager.close()

        events = [env for env in sender.sent if env.WhichOneof("payload") == "event"]
        types = [event.event.type for event in events]
        self.assertIn(runtime_pb2.EventType.MESSAGE_DELTA, types)
        self.assertIn(runtime_pb2.EventType.USAGE, types)
        completed = next(
            event for event in events
            if event.event.type == runtime_pb2.EventType.MESSAGE_COMPLETED
        )
        self.assertEqual(completed.event.data.fields["content"].string_value, "Hello")
        self.assertEqual(events[-1].event.data.fields["status"].string_value, "completed")
