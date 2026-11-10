"""Middleware — OpenAI-compatible chat-completions provider (P6A).

The first step supports non-streaming JSON responses. SSE/tool-call streaming
and production credential injection are added in later P6A steps.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import json
from typing import AsyncIterator, Sequence

from middleware_harness.llm.base import (
    DeltaCallback,
    LLMError,
    LLMResponse,
    Message,
    ModelConfig,
    ToolCall,
    ToolSpec,
)
from middleware_harness.llm.http import (
    HTTPRequest,
    HTTPTransport,
    HTTPTransportError,
    UrllibTransport,
)

_DEFAULT_BASE_URL = "https://api.deepseek.com"
_RETRYABLE_STATUS = {408, 409, 429, 500, 502, 503, 504, 529}


class OpenAICompatibleProvider:
    """Provider for DeepSeek/OpenAI-style /chat/completions APIs."""

    def __init__(self, transport: HTTPTransport | None = None) -> None:
        self.transport = transport or UrllibTransport()

    async def complete(
        self,
        messages: Sequence[Message],
        tools: Sequence[ToolSpec],
        model: ModelConfig,
        *,
        on_delta: DeltaCallback | None = None,
    ) -> LLMResponse:
        """Call /chat/completions and parse the JSON response."""
        request = self.build_request(messages, tools, model)
        try:
            response = await self.transport.open(request)
        except HTTPTransportError as exc:
            raise LLMError("PROVIDER_UNAVAILABLE", str(exc), retryable=True) from exc

        body = await _read_body(response.body)
        if response.status != 200:
            raise _http_error(response.status, body)
        return _parse_json_response(body, model)

    def build_request(
        self,
        messages: Sequence[Message],
        tools: Sequence[ToolSpec],
        model: ModelConfig,
    ) -> HTTPRequest:
        """Build one non-streaming chat-completions request."""
        if not model.model:
            raise LLMError("BAD_REQUEST", "model is required", retryable=False)

        body: dict[str, object] = {
            "model": model.model,
            "messages": [_convert_message(message) for message in messages],
            "stream": False,
        }
        if tools:
            body["tools"] = [_tool_schema(tool) for tool in tools]
            body["tool_choice"] = "auto"
        if "temperature" in model.params:
            try:
                body["temperature"] = float(model.params["temperature"])
            except ValueError as exc:
                raise LLMError(
                    "BAD_REQUEST",
                    "model parameter temperature must be numeric",
                    retryable=False,
                ) from exc
        if "max_tokens" in model.params:
            try:
                body["max_tokens"] = int(model.params["max_tokens"])
            except ValueError:
                pass

        headers = {
            "Content-Type": "application/json",
            "Accept": "application/json",
        }
        if model.api_key:
            headers["Authorization"] = f"Bearer {model.api_key}"
        base_url = (model.base_url or _DEFAULT_BASE_URL).rstrip("/")
        return HTTPRequest(
            url=base_url + "/chat/completions",
            headers=headers,
            body=json.dumps(body, ensure_ascii=False).encode("utf-8"),
        )


def _convert_message(message: Message) -> dict[str, object]:
    converted: dict[str, object] = {
        "role": message.role,
        "content": message.content,
    }
    if message.role == "tool" and message.tool_call_id:
        converted["tool_call_id"] = message.tool_call_id
    if message.role == "assistant" and message.tool_calls:
        converted["tool_calls"] = [
            {
                "id": call.call_id,
                "type": "function",
                "function": {
                    "name": call.name,
                    "arguments": call.arguments_json or "{}",
                },
            }
            for call in message.tool_calls
        ]
    return converted


def _tool_schema(tool: ToolSpec) -> dict[str, object]:
    try:
        schema = json.loads(tool.input_schema_json or "{}")
    except json.JSONDecodeError as exc:
        raise LLMError(
            "BAD_REQUEST",
            f"tool {tool.name!r} has an invalid input schema",
            retryable=False,
        ) from exc
    return {
        "type": "function",
        "function": {
            "name": tool.name,
            "description": tool.description,
            "parameters": schema or {"type": "object"},
        },
    }


def _parse_json_response(body: bytes, model: ModelConfig) -> LLMResponse:
    try:
        payload = json.loads(body.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise LLMError("BAD_RESPONSE", "provider returned invalid JSON", retryable=False) from exc

    choices = payload.get("choices")
    if not isinstance(choices, list) or not choices:
        raise LLMError("BAD_RESPONSE", "provider returned no choices", retryable=False)
    message = choices[0].get("message") or {}
    content = message.get("content") or ""
    tool_calls: list[ToolCall] = []
    for call in message.get("tool_calls") or []:
        function = call.get("function") or {}
        arguments = function.get("arguments", "{}")
        if not isinstance(arguments, str):
            arguments = json.dumps(arguments, ensure_ascii=False, separators=(",", ":"))
        tool_calls.append(
            ToolCall(
                call_id=str(call.get("id") or ""),
                name=str(function.get("name") or ""),
                arguments_json=arguments,
            )
        )

    usage: dict[str, int] = {}
    raw_usage = payload.get("usage")
    if isinstance(raw_usage, dict):
        for key, value in raw_usage.items():
            if isinstance(value, int) and not isinstance(value, bool):
                usage[key] = value
    return LLMResponse(
        content=str(content),
        tool_calls=tool_calls,
        usage=usage,
        model=str(payload.get("model") or model.model),
    )


async def _read_body(chunks: AsyncIterator[bytes]) -> bytes:
    parts: list[bytes] = []
    try:
        async for chunk in chunks:
            parts.append(chunk)
    finally:
        aclose = getattr(chunks, "aclose", None)
        if aclose is not None:
            await aclose()
    return b"".join(parts)


def _http_error(status: int, body: bytes) -> LLMError:
    code = "PROVIDER_UNAVAILABLE" if status in _RETRYABLE_STATUS else "PROVIDER_ERROR"
    if status in {401, 403}:
        code = "AUTH_FAILED"
    elif status == 429:
        code = "RATE_LIMITED"
    message = f"provider returned HTTP {status}"
    try:
        payload = json.loads(body.decode("utf-8"))
        error = payload.get("error") or {}
        if isinstance(error, dict) and error.get("message"):
            message = str(error["message"])
    except (UnicodeDecodeError, json.JSONDecodeError):
        pass
    return LLMError(code, message, retryable=status in _RETRYABLE_STATUS, status=status)

