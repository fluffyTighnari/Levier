"""Middleware — OpenAI-compatible chat-completions provider (P6A).

The first step supports non-streaming JSON responses. SSE/tool-call streaming
and production credential injection are added in later P6A steps.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import json
import os
from dataclasses import replace
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
_DEFAULT_DEEPSEEK_MODEL = "deepseek-flash"
_RETRYABLE_STATUS = {408, 409, 429, 500, 502, 503, 504, 529}


class OpenAICompatibleProvider:
    """Provider for DeepSeek/OpenAI-style /chat/completions APIs."""

    def __init__(
        self,
        transport: HTTPTransport | None = None,
        *,
        stream: bool = True,
    ) -> None:
        self.transport = transport or UrllibTransport()
        self.stream = stream

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
            if getattr(exc, "timeout", False):
                raise LLMError("TIMEOUT", str(exc), retryable=True) from exc
            raise LLMError("PROVIDER_UNAVAILABLE", str(exc), retryable=True) from exc

        if response.status != 200:
            body = await _read_body(response.body)
            raise _http_error(response.status, body)
        if "application/json" in response.headers.get("content-type", ""):
            body = await _read_body(response.body)
            return _parse_json_response(body, model)
        return await _parse_stream(response.body, model, on_delta)

    def build_request(
        self,
        messages: Sequence[Message],
        tools: Sequence[ToolSpec],
        model: ModelConfig,
    ) -> HTTPRequest:
        """Build one non-streaming chat-completions request."""
        model = _resolve_model(model)
        if not model.model:
            raise LLMError("BAD_REQUEST", "model is required", retryable=False)

        body: dict[str, object] = {
            "model": model.model,
            "messages": [_convert_message(message) for message in messages],
            "stream": self.stream,
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


def _resolve_model(model: ModelConfig) -> ModelConfig:
    """Apply the DeepSeek experiment environment when fields are empty."""
    if (model.provider or "").lower() != "deepseek":
        return model
    api_key = model.api_key or os.environ.get("DEEPSEEK_API_KEY", "").strip()
    if not api_key:
        raise LLMError(
            "PROVIDER_UNAVAILABLE",
            "DEEPSEEK_API_KEY is required for provider deepseek",
            retryable=False,
        )
    base_url = model.base_url or os.environ.get("DEEPSEEK_BASE_URL", "").strip() or _DEFAULT_BASE_URL
    model_name = model.model or os.environ.get("DEEPSEEK_MODEL", "").strip() or _DEFAULT_DEEPSEEK_MODEL
    return replace(model, api_key=api_key, base_url=base_url, model=model_name)


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


async def _parse_stream(
    chunks: AsyncIterator[bytes],
    model: ModelConfig,
    on_delta: DeltaCallback | None,
) -> LLMResponse:
    text_parts: list[str] = []
    tool_calls: dict[int, dict[str, str]] = {}
    usage: dict[str, int] = {}
    response_model = model.model

    async for event in _iter_sse(chunks):
        if event.get("error"):
            raise _stream_error(event)
        if event.get("model"):
            response_model = str(event["model"])
        raw_usage = event.get("usage")
        if isinstance(raw_usage, dict):
            for key, value in raw_usage.items():
                if isinstance(value, int) and not isinstance(value, bool):
                    usage[key] = value
        for choice in event.get("choices") or []:
            delta = choice.get("delta") or {}
            content = delta.get("content")
            if content:
                text_parts.append(str(content))
                if on_delta is not None:
                    await on_delta(str(content))
            for call in delta.get("tool_calls") or []:
                index = int(call.get("index", 0))
                entry = tool_calls.setdefault(index, {
                    "call_id": "",
                    "name": "",
                    "arguments": "",
                })
                if call.get("id"):
                    entry["call_id"] = str(call["id"])
                function = call.get("function") or {}
                if function.get("name"):
                    entry["name"] = str(function["name"])
                arguments = function.get("arguments")
                if arguments:
                    entry["arguments"] += str(arguments)

    return LLMResponse(
        content="".join(text_parts),
        tool_calls=[
            ToolCall(
                call_id=entry["call_id"] or f"call_{index}",
                name=entry["name"],
                arguments_json=entry["arguments"] or "{}",
            )
            for index, entry in sorted(tool_calls.items())
        ],
        usage=usage,
        model=response_model,
    )


async def _iter_sse(chunks: AsyncIterator[bytes]) -> AsyncIterator[dict[str, object]]:
    buffer = ""
    async for chunk in chunks:
        buffer += chunk.decode("utf-8", errors="replace")
        while "\n" in buffer:
            line, buffer = buffer.split("\n", 1)
            line = line.rstrip("\r")
            if not line or line.startswith(":"):
                continue
            if not line.startswith("data:"):
                continue
            data = line[len("data:"):].strip()
            if data == "[DONE]":
                return
            try:
                parsed = json.loads(data)
            except json.JSONDecodeError as exc:
                raise LLMError(
                    "BAD_RESPONSE",
                    "provider returned invalid SSE JSON",
                    retryable=False,
                ) from exc
            if isinstance(parsed, dict):
                yield parsed


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
    elif status == 408:
        code = "TIMEOUT"
    elif status == 429:
        code = "RATE_LIMITED"
    elif status in {400, 404, 422}:
        code = "BAD_REQUEST"
    message = f"provider returned HTTP {status}"
    try:
        payload = json.loads(body.decode("utf-8"))
        error = payload.get("error") or {}
        if isinstance(error, dict) and error.get("message"):
            message = str(error["message"])
        mapped = _ERROR_CODE_BY_TYPE.get(str(error.get("type", ""))) if isinstance(error, dict) else None
        if mapped:
            code = mapped
    except (UnicodeDecodeError, json.JSONDecodeError):
        pass
    retryable = code in {"TIMEOUT", "RATE_LIMITED", "PROVIDER_UNAVAILABLE"}
    return LLMError(code, message, retryable=retryable, status=status)


def _stream_error(event: dict[str, object]) -> LLMError:
    error = event.get("error") or {}
    if not isinstance(error, dict):
        error = {}
    error_type = str(error.get("type", ""))
    code = _ERROR_CODE_BY_TYPE.get(error_type, "PROVIDER_ERROR")
    return LLMError(
        code,
        str(error.get("message") or "provider stream error"),
        retryable=code in {"TIMEOUT", "RATE_LIMITED", "PROVIDER_UNAVAILABLE"},
    )


_ERROR_CODE_BY_TYPE = {
    "authentication_error": "AUTH_FAILED",
    "invalid_api_key": "AUTH_FAILED",
    "invalid_request_error": "BAD_REQUEST",
    "rate_limit_error": "RATE_LIMITED",
    "rate_limit_exceeded": "RATE_LIMITED",
    "overloaded_error": "PROVIDER_UNAVAILABLE",
    "server_error": "PROVIDER_UNAVAILABLE",
    "timeout": "TIMEOUT",
    "timeout_error": "TIMEOUT",
}

