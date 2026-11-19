"""Levier ? end-to-end Runtime SDK/MCP runtime integration test.

Starts the independent MCP runtime process with a real stdio MCP server and
drives it through the Runtime SDK MCP client.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import asyncio
import os
import tempfile
import unittest
from pathlib import Path

from middleware.runtime.v1 import runtime_pb2

from levier_runtime.mcp import McpRuntimeClient

_ROOT = Path(__file__).resolve().parents[2]
_RUNTIME_PYTHON = _ROOT / "mcp_runtime" / ".venv" / "bin" / "python"
_FAKE_SERVER = _ROOT / "mcp_runtime" / "tests" / "fake_mcp_server.py"


class McpRuntimeIntegrationTest(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self) -> None:
        if not _RUNTIME_PYTHON.exists():
            self.skipTest("mcp_runtime virtual environment is not available")
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.socket_path = str(Path(self.directory.name) / "mcp.sock")
        env = os.environ.copy()
        env["PYTHONPATH"] = str(_ROOT / "mcp_runtime" / "src")
        self.process = await asyncio.create_subprocess_exec(
            str(_RUNTIME_PYTHON),
            "-m",
            "levier_mcp_runtime",
            "--socket",
            self.socket_path,
            "--log-level",
            "warning",
            cwd=str(_ROOT),
            env=env,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        for _ in range(300):
            if Path(self.socket_path).exists():
                break
            if self.process.returncode is not None:
                stderr = await self.process.stderr.read() if self.process.stderr else b""
                self.fail(f"mcp runtime exited: {stderr.decode(errors='replace')}")
            await asyncio.sleep(0.01)
        else:
            self.fail("mcp runtime socket did not appear")
        self.client = McpRuntimeClient(self.socket_path)

    async def asyncTearDown(self) -> None:
        if hasattr(self, "client"):
            await self.client.close()
        if hasattr(self, "process") and self.process.returncode is None:
            self.process.terminate()
            try:
                await asyncio.wait_for(self.process.wait(), 5)
            except asyncio.TimeoutError:
                self.process.kill()
                await self.process.wait()

    async def test_open_call_and_close_through_runtime(self) -> None:
        health = await self.client.health()
        self.assertEqual(health["status"], "ok")

        binding = runtime_pb2.McpBinding(
            server_name="fake",
            transport="stdio",
            command=str(_RUNTIME_PYTHON),
            args=[str(_FAKE_SERVER)],
        )
        session = await self.client.open(binding)
        self.assertEqual([tool.name for tool in session.tools], ["mcp__fake__add"])

        result = await self.client.call(
            session.session_id,
            "mcp__fake__add",
            '{"a":2,"b":3}',
            10_000,
        )
        self.assertTrue(result.ok)
        self.assertIn("5", result.result_json)

        await self.client.close_session(session.session_id)


if __name__ == "__main__":
    unittest.main()
