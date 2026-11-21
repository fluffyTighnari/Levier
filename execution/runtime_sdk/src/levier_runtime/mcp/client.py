"""Levier ? Runtime SDK client for the independent MCP runtime.

The client speaks newline-delimited JSON-RPC over a Unix domain socket. It
never imports the MCP SDK and never holds an MCP connection itself.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import asyncio
import json
from dataclasses import dataclass, field
from typing import Any

from ..types import McpBinding, ToolResult, ToolSpec

_DEFAULT_TIMEOUT_SECONDS = 30.0


class McpRuntimeError(RuntimeError):
    """A normalized MCP runtime failure."""

    def __init__(
        self,
        code: str,
        message: str,
        *,
        retryable: bool = False,
    ) -> None:
        super().__init__(message)
        self.code = code
        self.retryable = retryable


@dataclass
class McpSessionInfo:
    """One open MCP session and its namespaced tools."""

    session_id: str
    tools: list[ToolSpec] = field(default_factory=list)


class McpRuntimeClient:
    """JSON-RPC client for `mcp_runtime`."""

    def __init__(
        self,
        socket_path: str,
        *,
        default_timeout: float = _DEFAULT_TIMEOUT_SECONDS,
    ) -> None:
        if not socket_path:
            raise ValueError("socket_path is required")
        self.socket_path = socket_path
        self.default_timeout = default_timeout
        self._reader: asyncio.StreamReader | None = None
        self._writer: asyncio.StreamWriter | None = None
        self._reader_task: asyncio.Task[None] | None = None
        self._connect_lock = asyncio.Lock()
        self._pending: dict[int, asyncio.Future[dict[str, Any]]] = {}
        self._next_id = 1
        self._closing = False

    async def health(self) -> dict[str, Any]:
        """Return runtime health."""
        result = await self._request("mcp.health", {})
        return result if isinstance(result, dict) else {}

    async def open(self, binding: McpBinding) -> McpSessionInfo:
        """Open one binding and return its namespaced tools."""
        result = await self._request("mcp.open", _binding_dict(binding), timeout=60.0)
        if not isinstance(result, dict):
            raise McpRuntimeError("BAD_RESPONSE", "mcp.open returned an invalid result")
        tools = [
            ToolSpec(
                name=str(tool.get("name", "")),
                description=str(tool.get("description", "")),
                input_schema_json=str(tool.get("input_schema_json", "")),
            )
            for tool in result.get("tools", [])
            if isinstance(tool, dict)
        ]
        return McpSessionInfo(
            session_id=str(result.get("session_id", "")),
            tools=tools,
        )

    async def call(
        self,
        session_id: str,
        tool_name: str,
        arguments_json: str,
        timeout_ms: int,
    ) -> ToolResult:
        """Call one namespaced MCP tool."""
        timeout = max(1, timeout_ms) / 1000
        result = await self._request(
            "mcp.call",
            {
                "session_id": session_id,
                "tool_name": tool_name,
                "arguments_json": arguments_json,
                "timeout_ms": timeout_ms,
            },
            timeout=timeout + 5,
        )
        if not isinstance(result, dict):
            raise McpRuntimeError("BAD_RESPONSE", "mcp.call returned an invalid result")
        if bool(result.get("ok")):
            return ToolResult(ok=True, result_json=str(result.get("result_json", "")))
        error = result.get("error") or {}
        return ToolResult(
            ok=False,
            error_code=str(error.get("code", "TOOL_EXEC_ERROR")),
            error_message=str(error.get("message", "")),
            retryable=bool(error.get("retryable", False)),
        )

    async def close_session(self, session_id: str) -> None:
        """Close one MCP session."""
        if not session_id:
            return
        await self._request("mcp.close", {"session_id": session_id})

    async def close(self) -> None:
        """Close the client and its connection."""
        self._closing = True
        self._fail_pending(McpRuntimeError("MCP_RUNTIME_UNAVAILABLE", "client closed"))
        if self._reader_task is not None:
            self._reader_task.cancel()
            try:
                await self._reader_task
            except asyncio.CancelledError:
                pass
            self._reader_task = None
        if self._writer is not None:
            self._writer.close()
            try:
                await self._writer.wait_closed()
            except ConnectionError:
                pass
            self._writer = None
        self._reader = None

    async def _request(
        self,
        method: str,
        params: dict[str, Any],
        *,
        timeout: float | None = None,
    ) -> Any:
        await self._ensure_connected()
        assert self._writer is not None
        request_id = self._next_id
        self._next_id += 1
        loop = asyncio.get_running_loop()
        future: asyncio.Future[dict[str, Any]] = loop.create_future()
        self._pending[request_id] = future
        payload = {
            "jsonrpc": "2.0",
            "id": request_id,
            "method": method,
            "params": params,
        }
        try:
            self._writer.write((json.dumps(payload, ensure_ascii=False) + "\n").encode("utf-8"))
            await self._writer.drain()
            response = await asyncio.wait_for(
                future,
                timeout if timeout is not None else self.default_timeout,
            )
        except asyncio.TimeoutError as exc:
            raise McpRuntimeError(
                "MCP_RUNTIME_TIMEOUT",
                f"MCP runtime did not answer {method!r} in time",
                retryable=True,
            ) from exc
        finally:
            self._pending.pop(request_id, None)

        error = response.get("error")
        if isinstance(error, dict):
            data = error.get("data") or {}
            raise McpRuntimeError(
                str(error.get("code", "MCP_RUNTIME_ERROR")),
                str(error.get("message", "MCP runtime error")),
                retryable=bool(data.get("retryable", False)),
            )
        return response.get("result")

    async def _ensure_connected(self) -> None:
        if self._writer is not None and not self._closing:
            return
        async with self._connect_lock:
            if self._writer is not None and not self._closing:
                return
            if self._closing:
                raise McpRuntimeError("MCP_RUNTIME_UNAVAILABLE", "client is closed")
            try:
                reader, writer = await _open_connection(self.socket_path)
            except OSError as exc:
                raise McpRuntimeError(
                    "MCP_RUNTIME_UNAVAILABLE",
                    f"connect MCP runtime at {self.socket_path}: {exc}",
                    retryable=True,
                ) from exc
            self._reader = reader
            self._writer = writer
            self._reader_task = asyncio.create_task(
                self._read_loop(reader, writer),
                name="mcp-runtime-reader",
            )

    async def _read_loop(
        self,
        reader: asyncio.StreamReader,
        writer: asyncio.StreamWriter,
    ) -> None:
        try:
            while True:
                line = await reader.readline()
                if not line:
                    break
                try:
                    response = json.loads(line.decode("utf-8"))
                except (UnicodeDecodeError, json.JSONDecodeError):
                    continue
                if not isinstance(response, dict):
                    continue
                request_id = response.get("id")
                if not isinstance(request_id, int):
                    continue
                future = self._pending.get(request_id)
                if future is not None and not future.done():
                    future.set_result(response)
        finally:
            if self._writer is writer:
                self._writer = None
                self._reader = None
            self._fail_pending(
                McpRuntimeError(
                    "MCP_RUNTIME_UNAVAILABLE",
                    "MCP runtime connection closed",
                    retryable=True,
                )
            )
            writer.close()
            try:
                await writer.wait_closed()
            except ConnectionError:
                pass

    def _fail_pending(self, exc: BaseException) -> None:
        pending = list(self._pending.values())
        self._pending.clear()
        for future in pending:
            if not future.done():
                future.set_exception(exc)


async def _open_connection(endpoint: str):
    """Open a Unix socket or a TCP endpoint such as tcp://127.0.0.1:9091."""
    value = endpoint.strip()
    if value.startswith("tcp://"):
        value = value[len("tcp://") :]
        host, _, port = value.rpartition(":")
        if not host or not port:
            raise OSError(f"invalid MCP runtime TCP endpoint: {endpoint!r}")
        return await asyncio.open_connection(host, int(port))
    if "://" not in value and "/" not in value and "\\" not in value and ":" in value:
        host, _, port = value.rpartition(":")
        if host and port.isdigit():
            return await asyncio.open_connection(host, int(port))
    return await asyncio.open_unix_connection(value)


def _binding_dict(binding: McpBinding) -> dict[str, Any]:
    return {
        "server_name": binding.server_name,
        "transport": binding.transport,
        "command": binding.command,
        "args": list(binding.args),
        "env": dict(binding.env),
        "url": binding.url,
        "headers": dict(binding.headers),
        "allowed_tools": list(binding.allowed_tools),
        "namespace": binding.namespace,
    }
