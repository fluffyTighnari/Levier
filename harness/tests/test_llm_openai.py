"""Middleware — OpenAI-compatible provider tests (P6A.1).

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import json
import unittest
from typing import AsyncIterator

from middleware_harness.llm import (
    HTTPRequest,
    HTTPResponse,
    Message,
    ModelConfig,
    OpenAICompatibleProvider,
    ToolCall,
    ToolSpec,
)


class FakeTransport:
    def __init__(self, body: bytes, status: int = 200) -> None:
        self.body = body
        self.status = status
        self.requests: list[HTTPRequest] = []

    async def open(self, request: HTTPRequest) -> HTTPResponse:
        self.requests.append(request)

        async def chunks() -> AsyncIterator[bytes]:
            yield self.body

        return HTTPResponse(
            status=self.status,
            headers={"content-type": "application/json"},
            body=chunks(),
        )


MODEL = ModelConfig(
    provider="deepseek",
    model="deepseek-flash",
    api_key="test-key",
    base_url="https://api.deepseek.com",
)


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
        provider = OpenAICompatibleProvider(transport)

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
        provider = OpenAICompatibleProvider(FakeTransport(body))

        response = await provider.complete([Message(role="user", content="hi")], [], MODEL)
        self.assertEqual(response.content, "done")
        self.assertEqual(response.tool_calls, [])

    async def test_p6a_1_message_conversion(self) -> None:
        body = json.dumps({"choices": [{"message": {"content": "ok"}}]}).encode()
        transport = FakeTransport(body)
        provider = OpenAICompatibleProvider(transport)
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


if __name__ == "__main__":
    unittest.main()
