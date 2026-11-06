"""Middleware — MCP session lifecycle and tool dispatch.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import asyncio
import json
import re
import uuid
from contextlib import AsyncExitStack
from dataclasses import dataclass, field
from typing import Any, Protocol

from mcp import ClientSession
from mcp.client.sse import sse_client
from mcp.client.stdio import StdioServerParameters, stdio_client
from mcp.client.streamable_http import create_mcp_http_client, streamable_http_client

from mcp_runtime.errors import McpRuntimeError

_OPEN_TIMEOUT_SECONDS = 30.0
_DEFAULT_CALL_TIMEOUT_MS = 60_000
_TRANSPORTS = {"stdio", "sse", "streamable_http"}


class Connector(Protocol):
    """Opens one MCP session from a binding."""

    async def connect(self, binding: dict[str, Any]) -> tuple[Any, AsyncExitStack]:
        """Return an initialized session and its cleanup stack."""


class SdkConnector:
    """MCP Python SDK connector for stdio, SSE, and streamable HTTP."""

    async def connect(self, binding: dict[str, Any]) -> tuple[Any, AsyncExitStack]:
        transport = _require_transport(binding)
        stack = AsyncExitStack()
        try:
            if transport == "stdio":
                command = str(binding.get("command") or "")
                if not command:
                    raise McpRuntimeError("BAD_BINDING", "stdio command is required")
                params = StdioServerParameters(
                    command=command,
                    args=[str(value) for value in binding.get("args") or []],
                    env=_string_map(binding.get("env")) or None,
                )
                streams = await stack.enter_async_context(stdio_client(params))
            elif transport == "sse":
                url = str(binding.get("url") or "")
                if not url:
                    raise McpRuntimeError("BAD_BINDING", "sse url is required")
                streams = await stack.enter_async_context(
                    sse_client(url, headers=_string_map(binding.get("headers")) or None)
                )
            else:
                url = str(binding.get("url") or "")
                if not url:
                    raise McpRuntimeError("BAD_BINDING", "streamable_http url is required")
                headers = _string_map(binding.get("headers"))
                http_client = None
                if headers:
                    http_client = await stack.enter_async_context(
                        create_mcp_http_client(headers=headers)
                    )
                streams = await stack.enter_async_context(
                    streamable_http_client(url, http_client=http_client)
                )

            read_stream, write_stream = streams[0], streams[1]
            session = await stack.enter_async_context(ClientSession(read_stream, write_stream))
            await session.initialize()
            return session, stack
        except BaseException:
            await stack.aclose()
            raise


@dataclass
class Session:
    """One open MCP session."""

    session_id: str
    owner: str
    binding: dict[str, Any]
    session: Any
    stack: AsyncExitStack
    tool_map: dict[str, str] = field(default_factory=dict)
    lock: asyncio.Lock = field(default_factory=asyncio.Lock)


class SessionManager:
    """Owns MCP sessions and enforces namespace and allowlists."""

    def __init__(self, connector: Connector | None = None) -> None:
        self.connector = connector or SdkConnector()
        self.sessions: dict[str, Session] = {}
        self._lock = asyncio.Lock()

    async def open(self, binding: dict[str, Any], owner: str) -> dict[str, Any]:
        """Open a binding and return its namespaced tool list."""
        if not isinstance(binding, dict):
            raise McpRuntimeError("BAD_BINDING", "binding must be an object")

        _require_transport(binding)
        server_name = str(binding.get("server_name") or "")
        if not server_name:
            raise McpRuntimeError("BAD_BINDING", "server_name is required")
        namespace = _namespace(binding.get("namespace"), server_name)
        allowed = {str(name) for name in binding.get("allowed_tools") or []}

        try:
            session, stack = await asyncio.wait_for(
                self.connector.connect(binding),
                _OPEN_TIMEOUT_SECONDS,
            )
        except asyncio.TimeoutError as exc:
            raise McpRuntimeError(
                "MCP_CONNECT_TIMEOUT",
                f"MCP server {server_name!r} did not initialize in time",
                retryable=True,
            ) from exc
        except McpRuntimeError:
            raise
        except Exception as exc:
            raise McpRuntimeError(
                "MCP_CONNECT_FAILED",
                f"connect MCP server {server_name!r}: {exc}",
                retryable=True,
            ) from exc

        session_id = uuid.uuid4().hex
        try:
            result = await asyncio.wait_for(
                session.list_tools(),
                _OPEN_TIMEOUT_SECONDS,
            )
        except asyncio.TimeoutError as exc:
            await _close_stack(stack)
            raise McpRuntimeError(
                "MCP_CONNECT_TIMEOUT",
                f"MCP server {server_name!r} did not list tools in time",
                retryable=True,
            ) from exc
        except Exception as exc:
            await _close_stack(stack)
            raise McpRuntimeError(
                "MCP_TOOL_LIST_FAILED",
                f"list tools from {server_name!r}: {exc}",
                retryable=True,
            ) from exc

        tool_map: dict[str, str] = {}
        tools: list[dict[str, str]] = []
        for tool in result.tools:
            raw_name = str(tool.name)
            if allowed and raw_name not in allowed:
                continue
            namespaced = _namespaced(namespace, raw_name)
            tool_map[namespaced] = raw_name
            schema = getattr(tool, "input_schema", None) or {"type": "object"}
            tools.append({
                "name": namespaced,
                "description": str(getattr(tool, "description", "") or ""),
                "input_schema_json": json.dumps(schema, ensure_ascii=False, separators=(",", ":")),
            })

        opened = Session(
            session_id=session_id,
            owner=owner,
            binding=dict(binding),
            session=session,
            stack=stack,
            tool_map=tool_map,
        )
        async with self._lock:
            self.sessions[session_id] = opened
        return {"session_id": session_id, "tools": tools}

    async def call(
        self,
        session_id: str,
        tool_name: str,
        arguments_json: str,
        timeout_ms: int,
    ) -> dict[str, Any]:
        """Call one namespaced MCP tool."""
        opened = await self._get(session_id)
        raw_name = opened.tool_map.get(tool_name)
        if raw_name is None:
            raise McpRuntimeError(
                "TOOL_DENIED",
                f"tool {tool_name!r} is not available in this MCP session",
            )
        try:
            arguments = json.loads(arguments_json or "{}")
        except json.JSONDecodeError as exc:
            raise McpRuntimeError("BAD_ARGUMENTS", "arguments_json is invalid") from exc
        if not isinstance(arguments, dict):
            raise McpRuntimeError("BAD_ARGUMENTS", "tool arguments must be an object")

        timeout = timeout_ms if timeout_ms > 0 else _DEFAULT_CALL_TIMEOUT_MS
        async with opened.lock:
            try:
                result = await asyncio.wait_for(
                    opened.session.call_tool(
                        raw_name,
                        arguments,
                        read_timeout_seconds=timeout / 1000,
                    ),
                    timeout / 1000,
                )
            except asyncio.TimeoutError as exc:
                raise McpRuntimeError(
                    "TOOL_TIMEOUT",
                    f"MCP tool {raw_name!r} timed out",
                    retryable=True,
                ) from exc
            except Exception as exc:
                raise McpRuntimeError(
                    "TOOL_EXEC_ERROR",
                    f"MCP tool {raw_name!r} failed: {exc}",
                    retryable=True,
                ) from exc

        return _call_result(result)

    async def close(self, session_id: str) -> None:
        """Close one session."""
        async with self._lock:
            opened = self.sessions.pop(session_id, None)
        if opened is not None:
            await _close_stack(opened.stack)

    async def close_owner(self, owner: str) -> None:
        """Close every session opened by one runtime connection."""
        async with self._lock:
            owned = [
                session_id
                for session_id, opened in self.sessions.items()
                if opened.owner == owner
            ]
        for session_id in owned:
            await self.close(session_id)

    async def close_all(self) -> None:
        """Close all sessions during shutdown."""
        async with self._lock:
            sessions = list(self.sessions.values())
            self.sessions.clear()
        for opened in sessions:
            await _close_stack(opened.stack)

    async def health(self) -> dict[str, Any]:
        """Return runtime health and session count."""
        async with self._lock:
            count = len(self.sessions)
        return {"status": "ok", "sessions": count}

    async def _get(self, session_id: str) -> Session:
        async with self._lock:
            opened = self.sessions.get(session_id)
        if opened is None:
            raise McpRuntimeError("SESSION_NOT_FOUND", "MCP session not found")
        return opened


def _call_result(result: Any) -> dict[str, Any]:
    is_error = bool(getattr(result, "is_error", False))
    content = [_content_block(block) for block in getattr(result, "content", []) or []]
    structured = getattr(result, "structured_content", None)
    result_json = _result_json(content, structured, is_error)
    payload: dict[str, Any] = {"ok": not is_error, "result_json": result_json}
    if is_error:
        payload["error"] = {
            "code": "TOOL_EXEC_ERROR",
            "message": _error_text(content) or "MCP tool returned an error",
            "retryable": False,
        }
    return payload


def _content_block(block: Any) -> dict[str, Any]:
    if isinstance(block, dict):
        return block
    dump = getattr(block, "model_dump", None)
    if callable(dump):
        data = dump(mode="json", exclude_none=True)
        return data if isinstance(data, dict) else {"type": "text", "text": str(data)}
    return {"type": "text", "text": str(block)}


def _result_json(content: list[dict[str, Any]], structured: Any, is_error: bool) -> str:
    if structured is not None:
        if hasattr(structured, "model_dump"):
            structured = structured.model_dump(mode="json")
        return json.dumps(
            {"content": content, "structured_content": structured, "is_error": is_error},
            ensure_ascii=False,
            separators=(",", ":"),
        )
    if len(content) == 1 and content[0].get("type") == "text":
        return str(content[0].get("text", ""))
    return json.dumps(
        {"content": content, "is_error": is_error},
        ensure_ascii=False,
        separators=(",", ":"),
    )


def _error_text(content: list[dict[str, Any]]) -> str:
    texts = [
        str(block.get("text", ""))
        for block in content
        if block.get("type") == "text"
    ]
    return "\n".join(text for text in texts if text)


def _require_transport(binding: dict[str, Any]) -> str:
    transport = str(binding.get("transport") or "")
    if transport not in _TRANSPORTS:
        raise McpRuntimeError(
            "BAD_BINDING",
            f"transport must be one of {sorted(_TRANSPORTS)}",
        )
    return transport


def _namespace(value: object, server_name: str) -> str:
    namespace = str(value or "").strip()
    if not namespace:
        namespace = f"mcp__{_sanitize(server_name)}__"
    if not namespace.endswith("__"):
        namespace = f"{namespace}__"
    return namespace


def _namespaced(namespace: str, tool_name: str) -> str:
    return tool_name if tool_name.startswith(namespace) else f"{namespace}{tool_name}"


def _sanitize(value: str) -> str:
    cleaned = re.sub(r"[^A-Za-z0-9_]", "_", value).strip("_")
    return cleaned or "server"


def _string_map(value: object) -> dict[str, str]:
    if not isinstance(value, dict):
        return {}
    return {str(key): str(item) for key, item in value.items()}


async def _close_stack(stack: AsyncExitStack) -> None:
    try:
        await stack.aclose()
    except Exception:
        pass
