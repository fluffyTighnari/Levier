"""Middleware — real stdio MCP integration test.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import sys
import unittest
from pathlib import Path

from levier_mcp_runtime.sessions import SessionManager


class StdioIntegrationTest(unittest.IsolatedAsyncioTestCase):
    async def test_open_and_call_stdio_server(self) -> None:
        script = Path(__file__).with_name("fake_mcp_server.py")
        manager = SessionManager()
        try:
            opened = await manager.open({
                "server_name": "fake",
                "transport": "stdio",
                "command": sys.executable,
                "args": [str(script)],
            }, "test-owner")
            self.assertEqual(
                [tool["name"] for tool in opened["tools"]],
                ["mcp__fake__add"],
            )

            result = await manager.call(
                opened["session_id"],
                "mcp__fake__add",
                '{"a":2,"b":3}',
                10_000,
            )
            self.assertTrue(result["ok"])
            self.assertIn("5", result["result_json"])
        finally:
            await manager.close_all()


if __name__ == "__main__":
    unittest.main()
