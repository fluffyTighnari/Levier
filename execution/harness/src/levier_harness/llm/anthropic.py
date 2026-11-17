"""Levier — Anthropic Messages API provider.

Builds Anthropic requests from the provider-neutral message model, parses
server-sent events, streams text deltas, and normalizes errors.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import json
from typing import AsyncIterator, Sequence

from levier_harness.llm.base import (
    DeltaCallback,
    LLMError,
    LLMResponse,
    Message,
    ModelConfig,
    ToolCall,
    ToolSpec,
)
from levier_harness.llm.http import (
    HTTPRequest,
    HTTPResponse,
    HTTPTransport,
    HTTPTransportError,
    UrllibTransport,
)

_DEFAULT_BASE_URL = "https://api.anthropic.com/v1/messages"
_DEFAULT_MAX_TOKENS = 4096
_DEFAULT_ANTHROPIC_VERSION = "2023-06-01"

_RETRYABLE_STATUS = {408, 409, 429, 500, 502, 503, 504, 529}
_RETRYABLE_ERROR_TYPES = {
    "api_error",
    "overloaded_error",
    "rate_limit_error",
    "timeout_error",
}
_ERROR_CODE_BY_TYPE = {
    "api_error": "PROVIDER_ERROR",
    "authentication_error": "AUTH_FAILED",
    "invalid_request_error": "BAD_REQUEST",
    "not_found_error": "BAD_REQUEST",
    "overloaded_error": "PROVIDER_UNAVAILABLE",
    "permission_error": "AUTH_FAILED",
    "rate_limit_error": "RATE_LIMITED",
    "timeout_error": "TIMEOUT",
}


class AnthropicProvider:
    """Anthropic Messages API provider."""

    def __init__(
        self,
        transport: HTTPTransport | None = None,
        *,
        api_version: str = _DEFAULT_ANTHROPIC_VERSION,
        default_max_tokens: int = _DEFAULT_MAX_TOKENS,
    ) -> None:
        self.transport = transport or UrllibTransport()
        self.api_version = api_version
        self.default_max_tokens = default_max_tokens

    async def complete(
        self,
        messages: Sequence[Message],
        tools: Sequence[ToolSpec],
        model: ModelConfig,
        *,
        on_delta: DeltaCallback | None = None,
    ) -> LLMResponse:
        """Call the Messages API and parse either SSE or JSON responses."""
        request = self.build_request(messages, tools, model)
        try:
            response = await self.transport.open(request)
        except HTTPTransportError as exc:
            raise LLMError(
                "PROVIDER_UNAVAILABLE",
                str(exc),
                retryable=True,
            ) from exc

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
        """Build one streaming Messages API request."""
        if not model.model:
            raise LLMError("BAD_REQUEST", "model is required", retryable=False)

        system, payload_messages = _convert_messages(messages)
        body: dict[str, object] = {
            "model": model.model,
            "max_tokens": _int_param(model.params, "max_tokens", self.default_max_tokens),
            "stream": True,
            "messages": payload_messages,
        }
        if system:
            body["system"] = system
        if tools:
            body["tools"] = [_tool_schema(tool) for tool in tools]
        for key in ("temperature", "top_p", "top_k"):
            if key in model.params:
                try:
                    body[key] = float(model.params[key])
                except ValueError as exc:
                    raise LLMError(
                        "BAD_REQUEST",
                        f"model parameter {key} must be numeric",
                        retryable=False,
                    ) from exc

        headers = {
            "accept": "text/event-stream",
            "anthropic-version": self.api_version,
            "content-type": "application/json",
        }
        if model.api_key:
            headers["x-api-key"] = model.api_key
        return HTTPRequest(
            url=model.base_url or _DEFAULT_BASE_URL,
            headers=headers,
            body=json.dumps(body, ensure_ascii=False).encode("utf-8"),
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

    try:
        async for event in _iter_sse(chunks):
            kind = event.get("type")
            if kind == "message_start":
                message = event.get("message") or {}
                response_model = message.get("model") or response_model
                _merge_usage(usage, message.get("usage"))
            elif kind == "content_block_start":
                block = event.get("content_block") or {}
                index = int(event.get("index", 0))
                if block.get("type") == "tool_use":
                    tool_calls[index] = {
                        "call_id": str(block.get("id", "")),
                        "name": str(block.get("name", "")),
                        "arguments_json": "",
                    }
                elif block.get("text"):
                    text = str(block["text"])
                    text_parts.append(text)
                    if on_delta is not None:
                        await on_delta(text)
            elif kind == "content_block_delta":
                index = int(event.get("index", 0))
                delta = event.get("delta") or {}
                if delta.get("type") == "text_delta":
                    text = str(delta.get("text", ""))
                    if text:
                        text_parts.append(text)
                        if on_delta is not None:
                            await on_delta(text)
                elif delta.get("type") == "input_json_delta":
                    call = tool_calls.setdefault(
                        index,
                        {"call_id": "", "name": "", "arguments_json": ""},
                    )
                    call["arguments_json"] += str(delta.get("partial_json", ""))
            elif kind == "message_delta":
                _merge_usage(usage, event.get("usage"))
            elif kind == "error":
                raise _stream_error(event)
            elif kind == "message_stop":
                break
    finally:
        aclose = getattr(chunks, "aclose", None)
        if aclose is not None:
            await aclose()

    calls = [
        ToolCall(
            call_id=call["call_id"],
            name=call["name"],
            arguments_json=call["arguments_json"] or "{}",
        )
        for _, call in sorted(tool_calls.items())
    ]
    return LLMResponse(
        content="".join(text_parts),
        tool_calls=calls,
        usage=usage,
        model=response_model,
    )


async def _iter_sse(chunks: AsyncIterator[bytes]) -> AsyncIterator[dict[str, object]]:
    buffer = ""
    async for chunk in chunks:
        buffer += chunk.decode("utf-8", errors="replace").replace("\r\n", "\n")
        while "\n\n" in buffer:
            block, buffer = buffer.split("\n\n", 1)
            data_lines = [
                line[5:].strip()
                for line in block.splitlines()
                if line.startswith("data:")
            ]
            if not data_lines:
                continue
            data = "\n".join(data_lines)
            if data == "[DONE]":
                return
            try:
                parsed = json.loads(data)
            except json.JSONDecodeError:
                continue
            if isinstance(parsed, dict):
                yield parsed


def _parse_json_response(body: bytes, model: ModelConfig) -> LLMResponse:
    try:
        payload = json.loads(body.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise LLMError("BAD_RESPONSE", "provider returned invalid JSON", retryable=False) from exc

    text_parts: list[str] = []
    tool_calls: list[ToolCall] = []
    for block in payload.get("content") or []:
        if block.get("type") == "text":
            text_parts.append(str(block.get("text", "")))
        elif block.get("type") == "tool_use":
            tool_calls.append(
                ToolCall(
                    call_id=str(block.get("id", "")),
                    name=str(block.get("name", "")),
                    arguments_json=json.dumps(
                        block.get("input") or {},
                        ensure_ascii=False,
                        separators=(",", ":"),
                    ),
                )
            )
    usage: dict[str, int] = {}
    _merge_usage(usage, payload.get("usage"))
    return LLMResponse(
        content="".join(text_parts),
        tool_calls=tool_calls,
        usage=usage,
        model=str(payload.get("model") or model.model),
    )


def _convert_messages(messages: Sequence[Message]) -> tuple[str, list[dict[str, object]]]:
    system_parts: list[str] = []
    converted: list[dict[str, object]] = []
    pending_tool_results: list[dict[str, object]] = []

    def flush_tool_results() -> None:
        if pending_tool_results:
            converted.append({"role": "user", "content": list(pending_tool_results)})
            pending_tool_results.clear()

    for message in messages:
        if message.role == "system":
            if message.content:
                system_parts.append(message.content)
            continue
        if message.role == "tool":
            pending_tool_results.append(
                {
                    "type": "tool_result",
                    "tool_use_id": message.tool_call_id,
                    "content": message.content,
                }
            )
            continue

        flush_tool_results()
        if message.role == "assistant" and message.tool_calls:
            blocks: list[dict[str, object]] = []
            if message.content:
                blocks.append({"type": "text", "text": message.content})
            for call in message.tool_calls:
                blocks.append(
                    {
                        "type": "tool_use",
                        "id": call.call_id,
                        "name": call.name,
                        "input": _parse_arguments(call.arguments_json),
                    }
                )
            converted.append({"role": "assistant", "content": blocks})
        elif message.content:
            converted.append({"role": message.role, "content": message.content})

    flush_tool_results()
    return "\n\n".join(system_parts), converted


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
        "name": tool.name,
        "description": tool.description,
        "input_schema": schema or {"type": "object"},
    }


def _parse_arguments(arguments_json: str) -> dict[str, object]:
    if not arguments_json:
        return {}
    try:
        parsed = json.loads(arguments_json)
    except json.JSONDecodeError as exc:
        raise LLMError(
            "BAD_REQUEST",
            "model returned invalid tool arguments",
            retryable=False,
        ) from exc
    return parsed if isinstance(parsed, dict) else {"value": parsed}


def _merge_usage(target: dict[str, int], usage: object) -> None:
    if not isinstance(usage, dict):
        return
    for key, value in usage.items():
        if isinstance(value, bool):
            continue
        if isinstance(value, int):
            target[key] = value


def _http_error(status: int, body: bytes) -> LLMError:
    code = "PROVIDER_UNAVAILABLE" if status in _RETRYABLE_STATUS else "PROVIDER_ERROR"
    message = f"provider returned HTTP {status}"
    try:
        payload = json.loads(body.decode("utf-8"))
        error = payload.get("error") or {}
        if error.get("message"):
            message = str(error["message"])
        mapped = _ERROR_CODE_BY_TYPE.get(str(error.get("type", "")))
        if mapped:
            code = mapped
    except (UnicodeDecodeError, json.JSONDecodeError):
        pass
    return LLMError(code, message, retryable=status in _RETRYABLE_STATUS, status=status)


def _stream_error(event: dict[str, object]) -> LLMError:
    error = event.get("error") or {}
    if not isinstance(error, dict):
        error = {}
    error_type = str(error.get("type", ""))
    code = _ERROR_CODE_BY_TYPE.get(error_type, "PROVIDER_ERROR")
    return LLMError(
        code,
        str(error.get("message") or "provider stream error"),
        retryable=error_type in _RETRYABLE_ERROR_TYPES,
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


def _int_param(params: dict[str, str], name: str, default: int) -> int:
    value = params.get(name)
    if value is None:
        return default
    try:
        return int(value)
    except ValueError:
        return default
