"""Levier ? Runtime SDK MCP client tests.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import asyncio
import json
import tempfile
import unittest
from pathlib import Path
from typing import Any

from middleware.runtime.v1 import runtime_pb2

from levier_runtime.mcp import McpRuntimeClient, McpRuntimeError


class FakeRuntime:
    def __init__(self) -> None:
        self.requests: list[dict[str, Any]] = []
        self.fail_method = ""
        self.server: asyncio.AbstractServer | None = None

    async def start(self, socket_path: str) -> None:
        self.server = await asyncio.start_unix_server(self._handle, path=socket_path)

    async def close(self) -> None:
        if self.server is not None:
            self.server.close()
            await self.server.wait_closed()

    async def _handle(self, reader: asyncio.StreamReader, writer: asyncio.StreamWriter) -> None:
        try:
            while True:
                line = await reader.readline()
                if not line:
                    break
                request = json.loads(line.decode("utf-8"))
                self.requests.append(request)
                method = request["method"]
                if method == self.fail_method:
                    response = {
                        "jsonrpc": "2.0",
                        "id": request["id"],
                        "error": {
                            "code": "TOOL_TIMEOUT",
                            "message": "tool timed out",
                            "data": {"retryable": True},
                        },
                    }
                else:
                    response = {
                        "jsonrpc": "2.0",
                        "id": request["id"],
                        "result": self._result(method),
                    }
                writer.write((json.dumps(response) + "\n").encode("utf-8"))
                await writer.drain()
        finally:
            writer.close()
            try:
                await writer.wait_closed()
            except ConnectionError:
                pass

    def _result(self, method: str) -> dict[str, Any]:
        if method == "mcp.health":
            return {"status": "ok", "version": "test", "sessions": 0}
        if method == "mcp.open":
            return {
                "session_id": "session-1",
                "tools": [
                    {
                        "name": "mcp__math__add",
                        "description": "add",
                        "input_schema_json": '{"type":"object"}',
                    },
                ],
            }
        if method == "mcp.call":
            return {"ok": True, "result_json": "5"}
        if method == "mcp.close":
            return {}
        return {}


class McpRuntimeClientTest(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self) -> None:
        if not hasattr(asyncio, "start_unix_server"):
            self.skipTest("Unix domain sockets are not available on this platform")
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.socket_path = str(Path(self.directory.name) / "mcp.sock")
        self.runtime = FakeRuntime()
        await self.runtime.start(self.socket_path)
        self.client = McpRuntimeClient(self.socket_path)

    async def asyncTearDown(self) -> None:
        await self.client.close()
        await self.runtime.close()

    async def test_health_open_call_close(self) -> None:
        health = await self.client.health()
        self.assertEqual(health["status"], "ok")

        binding = runtime_pb2.McpBinding(
            server_name="math",
            transport="stdio",
            command="fake",
        )
        session = await self.client.open(binding)
        self.assertEqual(session.session_id, "session-1")
        self.assertEqual([tool.name for tool in session.tools], ["mcp__math__add"])

        result = await self.client.call(
            session.session_id,
            "mcp__math__add",
            '{"a":2,"b":3}',
            1000,
        )
        self.assertTrue(result.ok)
        self.assertEqual(result.result_json, "5")

        await self.client.close_session(session.session_id)

        methods = [request["method"] for request in self.runtime.requests]
        self.assertEqual(methods, ["mcp.health", "mcp.open", "mcp.call", "mcp.close"])
        open_params = self.runtime.requests[1]["params"]
        self.assertEqual(open_params["server_name"], "math")
        self.assertEqual(open_params["allowed_tools"], [])

    async def test_runtime_error_is_mapped(self) -> None:
        self.runtime.fail_method = "mcp.call"
        with self.assertRaises(McpRuntimeError) as caught:
            await self.client.call("session-1", "mcp__math__add", "{}", 1000)
        self.assertEqual(caught.exception.code, "TOOL_TIMEOUT")
        self.assertTrue(caught.exception.retryable)

    async def test_missing_socket_is_retryable(self) -> None:
        client = McpRuntimeClient(str(Path(self.directory.name) / "missing.sock"))
        try:
            with self.assertRaises(McpRuntimeError) as caught:
                await client.health()
            self.assertEqual(caught.exception.code, "MCP_RUNTIME_UNAVAILABLE")
            self.assertTrue(caught.exception.retryable)
        finally:
            await client.close()


if __name__ == "__main__":
    unittest.main()
