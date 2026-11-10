"""Middleware — OpenAI-compatible provider tests (P6A.1).

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import json
import os
import unittest
from typing import AsyncIterator
from unittest.mock import patch

from middleware_harness.llm import (
    HTTPRequest,
    HTTPResponse,
    HTTPTransportError,
    LLMError,
    Message,
    ModelConfig,
    OpenAICompatibleProvider,
    ToolCall,
    ToolSpec,
)


class FakeTransport:
    def __init__(
        self,
        body: bytes,
        status: int = 200,
        *,
        content_type: str = "application/json",
        chunk_size: int = 0,
    ) -> None:
        self.body = body
        self.status = status
        self.content_type = content_type
        self.chunk_size = chunk_size
        self.requests: list[HTTPRequest] = []

    async def open(self, request: HTTPRequest) -> HTTPResponse:
        self.requests.append(request)

        async def chunks() -> AsyncIterator[bytes]:
            if self.chunk_size <= 0:
                yield self.body
                return
            for start in range(0, len(self.body), self.chunk_size):
                yield self.body[start:start + self.chunk_size]

        return HTTPResponse(
            status=self.status,
            headers={"content-type": self.content_type},
            body=chunks(),
        )


MODEL = ModelConfig(
    provider="deepseek",
    model="deepseek-flash",
    api_key="test-key",
    base_url="https://api.deepseek.com",
)


class RaisingTransport:
    async def open(self, request: HTTPRequest) -> HTTPResponse:
        raise HTTPTransportError("timed out", timeout=True)


class OpenAIProviderTest(unittest.IsolatedAsyncioTestCase):
    async def test_p6a_1_json_text_and_tool_call(self) -> None:
        body = json.dumps(
            {
                "model": "deepseek-flash",
                "choices": [
                    {
                        "message": {
                            "role": "assistant",
                            "content": "I will add.",
                            "tool_calls": [
                                {
                                    "id": "call_1",
                                    "type": "function",
                                    "function": {
                                        "name": "counter_add",
                                        "arguments": '{"delta":2}',
                                    },
                                }
                            ],
                        }
                    }
                ],
                "usage": {
                    "prompt_tokens": 11,
                    "completion_tokens": 7,
                    "total_tokens": 18,
                },
            }
        ).encode()
        transport = FakeTransport(body)
        provider = OpenAICompatibleProvider(transport, stream=False)

        response = await provider.complete(
            [Message(role="user", content="add 2")],
            [ToolSpec("counter_add", "add", '{"type":"object"}')],
            MODEL,
        )

        self.assertEqual(response.content, "I will add.")
        self.assertEqual(response.model, "deepseek-flash")
        self.assertEqual(response.usage, {
            "prompt_tokens": 11,
            "completion_tokens": 7,
            "total_tokens": 18,
        })
        self.assertEqual(len(response.tool_calls), 1)
        call = response.tool_calls[0]
        self.assertEqual(call.call_id, "call_1")
        self.assertEqual(call.name, "counter_add")
        self.assertEqual(call.arguments_json, '{"delta":2}')

        request = transport.requests[0]
        self.assertEqual(request.url, "https://api.deepseek.com/chat/completions")
        self.assertEqual(request.headers["Authorization"], "Bearer test-key")
        payload = json.loads(request.body.decode())
        self.assertEqual(payload["model"], "deepseek-flash")
        self.assertFalse(payload["stream"])
        self.assertEqual(payload["tool_choice"], "auto")
        self.assertEqual(payload["tools"][0]["type"], "function")
        self.assertEqual(payload["tools"][0]["function"]["name"], "counter_add")

    async def test_p6a_1_plain_text_response(self) -> None:
        body = json.dumps(
            {
                "choices": [{"message": {"role": "assistant", "content": "done"}}],
            }
        ).encode()
        provider = OpenAICompatibleProvider(FakeTransport(body), stream=False)

        response = await provider.complete([Message(role="user", content="hi")], [], MODEL)
        self.assertEqual(response.content, "done")
        self.assertEqual(response.tool_calls, [])

    async def test_p6a_1_message_conversion(self) -> None:
        body = json.dumps({"choices": [{"message": {"content": "ok"}}]}).encode()
        transport = FakeTransport(body)
        provider = OpenAICompatibleProvider(transport, stream=False)
        messages = [
            Message(role="system", content="system"),
            Message(role="user", content="hi"),
            Message(
                role="assistant",
                content="calling",
                tool_calls=(ToolCall(call_id="call_1", name="counter_add", arguments_json='{"delta":2}'),),
            ),
            Message(role="tool", tool_call_id="call_1", content='{"value":2}'),
        ]

        await provider.complete(messages, [], MODEL)

        payload = json.loads(transport.requests[0].body.decode())
        self.assertEqual(payload["messages"][0], {"role": "system", "content": "system"})
        self.assertEqual(payload["messages"][2]["tool_calls"][0]["id"], "call_1")
        self.assertEqual(payload["messages"][3]["tool_call_id"], "call_1")

    async def test_p6a_2_sse_text_and_tool_call(self) -> None:
        events = [
            {"model": "deepseek-flash", "choices": [{"delta": {"content": "Hel"}}]},
            {"choices": [{"delta": {"content": "lo"}}]},
            {
                "choices": [
                    {
                        "delta": {
                            "tool_calls": [
                                {
                                    "index": 0,
                                    "id": "call_1",
                                    "type": "function",
                                    "function": {
                                        "name": "counter_add",
                                        "arguments": '{"delta":',
                                    },
                                }
                            ]
                        }
                    }
                ]
            },
            {
                "choices": [
                    {
                        "delta": {
                            "tool_calls": [
                                {"index": 0, "function": {"arguments": "2}"}}
                            ]
                        }
                    }
                ]
            },
            {"usage": {"prompt_tokens": 3, "completion_tokens": 4, "total_tokens": 7}},
        ]
        body = b"".join(
            f"data: {json.dumps(event)}\n\n".encode("utf-8") for event in events
        ) + b"data: [DONE]\n\n"
        transport = FakeTransport(
            body,
            content_type="text/event-stream",
            chunk_size=7,
        )
        provider = OpenAICompatibleProvider(transport)

        deltas: list[str] = []

        async def on_delta(text: str) -> None:
            deltas.append(text)

        response = await provider.complete(
            [Message(role="user", content="add 2")],
            [ToolSpec("counter_add", "add", '{"type":"object"}')],
            MODEL,
            on_delta=on_delta,
        )

        self.assertEqual(response.content, "Hello")
        self.assertEqual(deltas, ["Hel", "lo"])
        self.assertEqual(response.usage, {
            "prompt_tokens": 3,
            "completion_tokens": 4,
            "total_tokens": 7,
        })
        self.assertEqual(len(response.tool_calls), 1)
        call = response.tool_calls[0]
        self.assertEqual(call.call_id, "call_1")
        self.assertEqual(call.name, "counter_add")
        self.assertEqual(call.arguments_json, '{"delta":2}')
        payload = json.loads(transport.requests[0].body.decode())
        self.assertTrue(payload["stream"])

    async def test_p6a_3_http_errors(self) -> None:
        cases = [
            (401, "authentication_error", "AUTH_FAILED", False),
            (429, "rate_limit_exceeded", "RATE_LIMITED", True),
            (500, "server_error", "PROVIDER_UNAVAILABLE", True),
            (400, "invalid_request_error", "BAD_REQUEST", False),
        ]
        for status, error_type, code, retryable in cases:
            with self.subTest(status=status):
                body = json.dumps(
                    {
                        "error": {
                            "type": error_type,
                            "message": f"status {status}",
                        }
                    }
                ).encode()
                provider = OpenAICompatibleProvider(
                    FakeTransport(body, status=status),
                    stream=False,
                )
                with self.assertRaises(LLMError) as caught:
                    await provider.complete([Message(role="user", content="hi")], [], MODEL)
                self.assertEqual(caught.exception.code, code)
                self.assertEqual(caught.exception.retryable, retryable)
                self.assertEqual(caught.exception.status, status)

    async def test_p6a_3_transport_timeout(self) -> None:
        provider = OpenAICompatibleProvider(RaisingTransport(), stream=True)
        with self.assertRaises(LLMError) as caught:
            await provider.complete([Message(role="user", content="hi")], [], MODEL)
        self.assertEqual(caught.exception.code, "TIMEOUT")
        self.assertTrue(caught.exception.retryable)

    async def test_p6a_3_stream_error(self) -> None:
        body = b'data: {"error":{"type":"rate_limit_exceeded","message":"slow down"}}\n\n'
        provider = OpenAICompatibleProvider(
            FakeTransport(body, content_type="text/event-stream")
        )
        with self.assertRaises(LLMError) as caught:
            await provider.complete([Message(role="user", content="hi")], [], MODEL)
        self.assertEqual(caught.exception.code, "RATE_LIMITED")
        self.assertTrue(caught.exception.retryable)

    async def test_p6a_5_env_credentials(self) -> None:
        body = json.dumps({"choices": [{"message": {"content": "ok"}}]}).encode()
        transport = FakeTransport(body)
        provider = OpenAICompatibleProvider(transport, stream=False)
        with patch.dict(
            os.environ,
            {
                "DEEPSEEK_API_KEY": "env-key",
                "DEEPSEEK_BASE_URL": "https://env.example/v1",
                "DEEPSEEK_MODEL": "env-model",
            },
            clear=False,
        ):
            response = await provider.complete(
                [Message(role="user", content="hi")],
                [],
                ModelConfig(provider="deepseek"),
            )

        self.assertEqual(response.content, "ok")
        request = transport.requests[0]
        self.assertEqual(request.url, "https://env.example/v1/chat/completions")
        self.assertEqual(request.headers["Authorization"], "Bearer env-key")
        payload = json.loads(request.body.decode())
        self.assertEqual(payload["model"], "env-model")

    async def test_p6a_5_missing_key_fails_before_request(self) -> None:
        transport = FakeTransport(b"{}")
        provider = OpenAICompatibleProvider(transport, stream=False)
        with patch.dict(os.environ, {"DEEPSEEK_API_KEY": ""}, clear=False):
            with self.assertRaises(LLMError) as caught:
                await provider.complete(
                    [Message(role="user", content="hi")],
                    [],
                    ModelConfig(provider="deepseek"),
                )
        self.assertEqual(caught.exception.code, "PROVIDER_UNAVAILABLE")
        self.assertFalse(caught.exception.retryable)
        self.assertIn("DEEPSEEK_API_KEY", str(caught.exception))
        self.assertEqual(transport.requests, [])

    async def test_p6a_5_explicit_key_overrides_env(self) -> None:
        body = json.dumps({"choices": [{"message": {"content": "ok"}}]}).encode()
        transport = FakeTransport(body)
        provider = OpenAICompatibleProvider(transport, stream=False)
        with patch.dict(os.environ, {"DEEPSEEK_API_KEY": "env-key"}, clear=False):
            await provider.complete(
                [Message(role="user", content="hi")],
                [],
                ModelConfig(provider="deepseek", model="m", api_key="model-key"),
            )
        self.assertEqual(
            transport.requests[0].headers["Authorization"],
            "Bearer model-key",
        )


if __name__ == "__main__":
    unittest.main()
