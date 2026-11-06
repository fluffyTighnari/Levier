"""Middleware — local JSON-RPC server for the MCP runtime.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import asyncio
import logging
import os
import uuid
from pathlib import Path
from typing import Any

from mcp_runtime.errors import McpRuntimeError
from mcp_runtime.protocol import decode_request, encode_error, encode_response
from mcp_runtime.sessions import SessionManager


class McpRuntimeServer:
    """Unix-socket server exposing MCP sessions over JSON-RPC."""

    def __init__(
        self,
        manager: SessionManager,
        socket_path: str,
        *,
        version: str,
        logger: logging.Logger | None = None,
    ) -> None:
        self.manager = manager
        self.socket_path = socket_path
        self.version = version
        self.logger = logger or logging.getLogger("mcp_runtime.server")
        self._server: asyncio.AbstractServer | None = None
        self._closed = asyncio.Event()

    async def start(self) -> None:
        """Create the Unix socket and start accepting clients."""
        path = Path(self.socket_path)
        path.parent.mkdir(parents=True, exist_ok=True)
        try:
            path.unlink()
        except FileNotFoundError:
            pass
        self._server = await asyncio.start_unix_server(
            self._handle_client,
            path=self.socket_path,
        )
        try:
            os.chmod(self.socket_path, 0o600)
        except OSError:
            pass
        self.logger.info("mcp runtime listening", extra={"socket": self.socket_path})

    async def serve_forever(self) -> None:
        """Serve until close is called."""
        if self._server is None:
            await self.start()
        assert self._server is not None
        async with self._server:
            await self._closed.wait()

    async def close(self) -> None:
        """Stop accepting clients and close every session."""
        self._closed.set()
        if self._server is not None:
            self._server.close()
            await self._server.wait_closed()
            self._server = None
        await self.manager.close_all()
        try:
            Path(self.socket_path).unlink()
        except FileNotFoundError:
            pass

    async def _handle_client(
        self,
        reader: asyncio.StreamReader,
        writer: asyncio.StreamWriter,
    ) -> None:
        owner = uuid.uuid4().hex
        try:
            while True:
                line = await reader.readline()
                if not line:
                    break
                response = await self.handle_line(line, owner)
                if response is not None:
                    writer.write(response)
                    await writer.drain()
        finally:
            await self.manager.close_owner(owner)
            writer.close()
            try:
                await writer.wait_closed()
            except ConnectionError:
                pass

    async def handle_line(self, line: bytes, owner: str) -> bytes | None:
        """Handle one JSON-RPC request line."""
        request_id: Any = None
        try:
            request = decode_request(line)
            request_id = request.get("id")
            result = await handle_request(
                self.manager,
                request,
                owner,
                version=self.version,
            )
            if request_id is None:
                return None
            return encode_response(request_id, result)
        except ValueError as exc:
            return encode_error(request_id, {
                "code": "PARSE_ERROR",
                "message": str(exc),
                "data": {"retryable": False},
            })
        except McpRuntimeError as exc:
            return encode_error(request_id, exc.as_dict())
        except Exception as exc:
            self.logger.exception("mcp runtime request failed")
            return encode_error(request_id, {
                "code": "INTERNAL",
                "message": str(exc),
                "data": {"retryable": False},
            })


async def handle_request(
    manager: SessionManager,
    request: dict[str, Any],
    owner: str,
    *,
    version: str,
) -> dict[str, Any]:
    """Dispatch one decoded JSON-RPC request."""
    method = str(request.get("method", ""))
    params = request.get("params") or {}
    if not isinstance(params, dict):
        raise McpRuntimeError("BAD_PARAMS", "params must be an object")

    if method == "mcp.health":
        health = await manager.health()
        health["version"] = version
        return health
    if method == "mcp.open":
        return await manager.open(params, owner)
    if method == "mcp.call":
        return await manager.call(
            session_id=str(params.get("session_id") or ""),
            tool_name=str(params.get("tool_name") or ""),
            arguments_json=str(params.get("arguments_json") or "{}"),
            timeout_ms=int(params.get("timeout_ms") or 0),
        )
    if method == "mcp.close":
        await manager.close(str(params.get("session_id") or ""))
        return {}
    raise McpRuntimeError("METHOD_NOT_FOUND", f"unknown method {method!r}")


async def serve_stdio(
    manager: SessionManager,
    *,
    version: str,
    logger: logging.Logger | None = None,
) -> None:
    """Serve one client over stdin/stdout."""
    logger = logger or logging.getLogger("mcp_runtime.stdio")
    owner = "stdio"
    server = McpRuntimeServer(manager, socket_path="", version=version, logger=logger)
    try:
        while True:
            line = await asyncio.to_thread(_readline)
            if not line:
                break
            response = await server.handle_line(line, owner)
            if response is not None:
                await asyncio.to_thread(_write, response)
    finally:
        await manager.close_owner(owner)


def _readline() -> bytes:
    import sys

    return sys.stdin.buffer.readline()


def _write(payload: bytes) -> None:
    import sys

    sys.stdout.buffer.write(payload)
    sys.stdout.buffer.flush()
