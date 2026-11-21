"""Levier — local JSON-RPC server for the MCP runtime.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import asyncio
import logging
import os
import uuid
from pathlib import Path
from typing import Any

from levier_mcp_runtime.errors import McpRuntimeError
from levier_mcp_runtime.protocol import decode_request, encode_error, encode_response
from levier_mcp_runtime.sessions import SessionManager


class McpRuntimeServer:
    """Local server exposing MCP sessions over JSON-RPC.

    It can listen on a Unix socket (Linux/macOS), a TCP address (all
    platforms), or both. TCP makes the shared MCP Runtime usable from a host
    runtime when Levier itself runs in Docker.
    """

    def __init__(
        self,
        manager: SessionManager,
        socket_path: str = "",
        *,
        tcp_address: str = "",
        version: str,
        logger: logging.Logger | None = None,
    ) -> None:
        self.manager = manager
        self.socket_path = socket_path
        self.tcp_address = tcp_address
        self.version = version
        self.logger = logger or logging.getLogger("mcp_runtime.server")
        self._servers: list[asyncio.AbstractServer] = []
        self._writers: set[asyncio.StreamWriter] = set()
        self._closed = asyncio.Event()

    async def start(self) -> None:
        """Start every configured listener."""
        if not self.socket_path and not self.tcp_address:
            raise ValueError("mcp runtime needs --socket or --addr")
        if self.socket_path:
            path = Path(self.socket_path)
            path.parent.mkdir(parents=True, exist_ok=True)
            try:
                path.unlink()
            except FileNotFoundError:
                pass
            server = await asyncio.start_unix_server(
                self._handle_client,
                path=self.socket_path,
            )
            try:
                os.chmod(self.socket_path, 0o600)
            except OSError:
                pass
            self._servers.append(server)
            self.logger.info("mcp runtime listening", extra={"socket": self.socket_path})
        if self.tcp_address:
            host, port = parse_address(self.tcp_address)
            server = await asyncio.start_server(self._handle_client, host=host, port=port)
            self._servers.append(server)
            self.logger.info("mcp runtime listening", extra={"address": self.tcp_address})

    async def serve_forever(self) -> None:
        """Serve until close is called."""
        if not self._servers:
            await self.start()
        await self._closed.wait()

    async def close(self) -> None:
        """Stop accepting clients and close every session."""
        self._closed.set()
        for server in self._servers:
            server.close()
            # Close connected clients first: since Python 3.12 wait_closed()
            # also waits for active connections, which would hang shutdown.
            for writer in list(self._writers):
                writer.close()
            try:
                await asyncio.wait_for(server.wait_closed(), timeout=2)
            except asyncio.TimeoutError:
                self.logger.warning("mcp runtime shutdown timed out")
        self._servers.clear()
        await self.manager.close_all()
        if self.socket_path:
            try:
                Path(self.socket_path).unlink()
            except FileNotFoundError:
                pass


def parse_address(value: str) -> tuple[str, int]:
    """Parse ``host:port`` or ``tcp://host:port``."""
    address = value.strip()
    if address.startswith("tcp://"):
        address = address[len("tcp://") :]
    if address.startswith("["):
        host, _, port = address[1:].partition("]:")
    else:
        host, _, port = address.rpartition(":")
    if not host or not port:
        raise ValueError(f"invalid TCP address: {value!r}")
    return host, int(port)

    async def _handle_client(
        self,
        reader: asyncio.StreamReader,
        writer: asyncio.StreamWriter,
    ) -> None:
        owner = uuid.uuid4().hex
        self._writers.add(writer)
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
            self._writers.discard(writer)
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
