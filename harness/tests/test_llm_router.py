"""Middleware — provider router tests (P6A.4).

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import unittest
from typing import Sequence

from middleware_harness.llm import (
    DeltaCallback,
    LLMError,
    LLMResponse,
    Message,
    ModelConfig,
    ProviderRouter,
    ToolSpec,
)


class FakeProvider:
    def __init__(self, name: str) -> None:
        self.name = name
        self.calls = 0
        self.last_model: ModelConfig | None = None

    async def complete(
        self,
        messages: Sequence[Message],
        tools: Sequence[ToolSpec],
        model: ModelConfig,
        *,
        on_delta: DeltaCallback | None = None,
    ) -> LLMResponse:
        self.calls += 1
        self.last_model = model
        return LLMResponse(content=self.name, model=model.model)


class ProviderRouterTest(unittest.IsolatedAsyncioTestCase):
    def setUp(self) -> None:
        self.anthropic = FakeProvider("anthropic")
        self.openai = FakeProvider("openai")
        self.router = ProviderRouter(self.anthropic, self.openai)

    async def test_p6a_4_routes_known_providers(self) -> None:
        cases = [
            ("anthropic", self.anthropic),
            ("deepseek", self.openai),
            ("openai", self.openai),
            ("", self.anthropic),
        ]
        for provider, expected in cases:
            with self.subTest(provider=provider):
                self.anthropic.calls = 0
                self.openai.calls = 0
                response = await self.router.complete(
                    [Message(role="user", content="hi")],
                    [],
                    ModelConfig(provider=provider, model="model"),
                )
                self.assertEqual(response.content, expected.name)
                self.assertEqual(expected.calls, 1)

    async def test_p6a_4_rejects_unknown_provider(self) -> None:
        with self.assertRaises(LLMError) as caught:
            await self.router.complete(
                [Message(role="user", content="hi")],
                [],
                ModelConfig(provider="unknown", model="model"),
            )
        self.assertEqual(caught.exception.code, "BAD_REQUEST")
        self.assertFalse(caught.exception.retryable)


if __name__ == "__main__":
    unittest.main()
