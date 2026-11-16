"""Middleware — MCP SDK transport selection tests.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import unittest
from contextlib import AsyncExitStack, asynccontextmanager
from unittest.mock import patch

from levier_mcp_runtime.errors import McpRuntimeError
from levier_mcp_runtime.sessions import SdkConnector


class FakeSession:
    def __init__(self) -> None:
        self.initialized = False

    async def initialize(self) -> None:
        self.initialized = True


class SdkConnectorTest(unittest.IsolatedAsyncioTestCase):
    async def test_stdio_transport(self) -> None:
        calls = []

        @asynccontextmanager
        async def fake_stdio(params):
            calls.append(("stdio", params.command, params.args))
            yield ("read", "write")

        @asynccontextmanager
        async def fake_session(read, write):
            calls.append(("session", read, write))
            yield FakeSession()

        with patch("levier_mcp_runtime.sessions.stdio_client", fake_stdio), \
             patch("levier_mcp_runtime.sessions.ClientSession", fake_session):
            session, stack = await SdkConnector().connect({
                "transport": "stdio",
                "command": "python",
                "args": ["server.py"],
            })
            await stack.aclose()

        self.assertTrue(session.initialized)
        self.assertEqual(calls[0], ("stdio", "python", ["server.py"]))
        self.assertEqual(calls[1], ("session", "read", "write"))

    async def test_sse_transport(self) -> None:
        calls = []

        @asynccontextmanager
        async def fake_sse(url, headers=None):
            calls.append(("sse", url, headers))
            yield ("read", "write")

        @asynccontextmanager
        async def fake_session(read, write):
            yield FakeSession()

        with patch("levier_mcp_runtime.sessions.sse_client", fake_sse), \
             patch("levier_mcp_runtime.sessions.ClientSession", fake_session):
            _, stack = await SdkConnector().connect({
                "transport": "sse",
                "url": "https://example.test/sse",
                "headers": {"Authorization": "Bearer x"},
            })
            await stack.aclose()

        self.assertEqual(
            calls[0],
            ("sse", "https://example.test/sse", {"Authorization": "Bearer x"}),
        )

    async def test_streamable_http_transport(self) -> None:
        calls = []

        @asynccontextmanager
        async def fake_http(url, *, http_client=None):
            calls.append(("http", url))
            yield ("read", "write")

        @asynccontextmanager
        async def fake_session(read, write):
            yield FakeSession()

        with patch("levier_mcp_runtime.sessions.streamable_http_client", fake_http), \
             patch("levier_mcp_runtime.sessions.ClientSession", fake_session):
            _, stack = await SdkConnector().connect({
                "transport": "streamable_http",
                "url": "https://example.test/mcp",
            })
            await stack.aclose()

        self.assertEqual(calls, [("http", "https://example.test/mcp")])

    async def test_missing_command_or_url(self) -> None:
        with self.assertRaises(McpRuntimeError) as caught:
            await SdkConnector().connect({"transport": "stdio"})
        self.assertEqual(caught.exception.code, "BAD_BINDING")

        with self.assertRaises(McpRuntimeError) as caught:
            await SdkConnector().connect({"transport": "sse"})
        self.assertEqual(caught.exception.code, "BAD_BINDING")

        with self.assertRaises(McpRuntimeError) as caught:
            await SdkConnector().connect({"transport": "unknown"})
        self.assertEqual(caught.exception.code, "BAD_BINDING")


if __name__ == "__main__":
    unittest.main()
