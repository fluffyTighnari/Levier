"""Levier — MCP runtime JSON-RPC server tests.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import asyncio
import json
import tempfile
import unittest
from contextlib import AsyncExitStack
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from levier_mcp_runtime.server import McpRuntimeServer
from levier_mcp_runtime.sessions import SessionManager


@dataclass
class FakeTool:
    name: str
    description: str = ""
    input_schema: dict[str, Any] | None = None


@dataclass
class FakeListResult:
    tools: list[FakeTool]


@dataclass
class FakeCallResult:
    content: list[Any]
    structured_content: Any = None
    is_error: bool = False


class FakeSession:
    def __init__(self) -> None:
        self.tool = FakeTool("add", input_schema={"type": "object"})

    async def list_tools(self) -> FakeListResult:
        return FakeListResult([self.tool])

    async def call_tool(self, name: str, arguments: dict[str, Any], **kwargs: Any) -> FakeCallResult:
        value = arguments.get("a", 0) + arguments.get("b", 0)
        return FakeCallResult(content=[{"type": "text", "text": str(value)}])


class FakeConnector:
    async def connect(self, binding: dict[str, Any]):
        return FakeSession(), AsyncExitStack()


class McpRuntimeServerTest(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self) -> None:
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.socket_path = str(Path(self.directory.name) / "mcp.sock")
        self.manager = SessionManager(FakeConnector())
        self.server = McpRuntimeServer(
            self.manager,
            self.socket_path,
            version="test",
        )
        await self.server.start()
        self.reader, self.writer = await asyncio.open_unix_connection(self.socket_path)

    async def asyncTearDown(self) -> None:
        self.writer.close()
        await self.writer.wait_closed()
        await self.server.close()

    async def rpc(self, request_id: int, method: str, params: dict[str, Any] | None = None) -> dict[str, Any]:
        payload = {
            "jsonrpc": "2.0",
            "id": request_id,
            "method": method,
            "params": params or {},
        }
        self.writer.write((json.dumps(payload) + "\n").encode("utf-8"))
        await self.writer.drain()
        line = await self.reader.readline()
        return json.loads(line.decode("utf-8"))

    async def test_health_open_call_close(self) -> None:
        health = await self.rpc(1, "mcp.health")
        self.assertEqual(health["result"]["status"], "ok")
        self.assertEqual(health["result"]["version"], "test")

        opened = await self.rpc(2, "mcp.open", {
            "server_name": "math",
            "transport": "stdio",
            "command": "fake",
        })
        session_id = opened["result"]["session_id"]
        self.assertEqual(opened["result"]["tools"][0]["name"], "mcp__math__add")

        called = await self.rpc(3, "mcp.call", {
            "session_id": session_id,
            "tool_name": "mcp__math__add",
            "arguments_json": '{"a":2,"b":3}',
            "timeout_ms": 1000,
        })
        self.assertTrue(called["result"]["ok"])
        self.assertEqual(called["result"]["result_json"], "5")

        closed = await self.rpc(4, "mcp.close", {"session_id": session_id})
        self.assertEqual(closed["result"], {})

    async def test_unknown_method(self) -> None:
        response = await self.rpc(1, "mcp.unknown")
        self.assertEqual(response["error"]["code"], "METHOD_NOT_FOUND")

    async def test_invalid_json(self) -> None:
        self.writer.write(b"{not-json}\n")
        await self.writer.drain()
        line = await self.reader.readline()
        response = json.loads(line.decode("utf-8"))
        self.assertEqual(response["error"]["code"], "PARSE_ERROR")

    async def test_close_with_connected_client_completes(self) -> None:
        await asyncio.wait_for(self.server.close(), timeout=2.0)
        self.assertFalse(Path(self.socket_path).exists())
        self.assertNotIn(self.writer, self.server._writers)

    async def test_disconnect_closes_owned_sessions(self) -> None:
        opened = await self.rpc(1, "mcp.open", {
            "server_name": "math",
            "transport": "stdio",
            "command": "fake",
        })
        session_id = opened["result"]["session_id"]
        self.writer.close()
        await self.writer.wait_closed()
        for _ in range(100):
            if not self.manager.sessions:
                break
            await asyncio.sleep(0.005)
        self.assertNotIn(session_id, self.manager.sessions)


if __name__ == "__main__":
    unittest.main()
