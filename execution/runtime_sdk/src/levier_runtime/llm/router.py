"""Levier — LLM provider routing.

Dispatches one completion by the provider named in ModelConfig.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

from typing import Sequence

from levier_runtime.llm.base import (
    DeltaCallback,
    LLMError,
    LLMProvider,
    LLMResponse,
    Message,
    ModelConfig,
    ToolSpec,
)
from levier_runtime.llm.anthropic import AnthropicProvider
from levier_runtime.llm.openai import OpenAICompatibleProvider


class ProviderRouter:
    """Route completions to Anthropic or an OpenAI-compatible provider."""

    def __init__(
        self,
        anthropic: LLMProvider | None = None,
        openai: LLMProvider | None = None,
    ) -> None:
        self.anthropic = anthropic or AnthropicProvider()
        self.openai = openai or OpenAICompatibleProvider()

    async def complete(
        self,
        messages: Sequence[Message],
        tools: Sequence[ToolSpec],
        model: ModelConfig,
        *,
        on_delta: DeltaCallback | None = None,
    ) -> LLMResponse:
        """Dispatch by model.provider."""
        provider = (model.provider or "anthropic").lower()
        if provider == "anthropic":
            target = self.anthropic
        elif provider in {"deepseek", "openai"}:
            target = self.openai
        else:
            raise LLMError(
                "BAD_REQUEST",
                f"unsupported model provider {model.provider!r}",
                retryable=False,
            )
        return await target.complete(messages, tools, model, on_delta=on_delta)
