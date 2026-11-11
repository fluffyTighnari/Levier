"""M4-A: allowlist filtering and direct-call denial."""

from __future__ import annotations

import asyncio
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[6]
sys.path.insert(0, str(ROOT / "harness" / "src"))
sys.path.insert(0, str(ROOT / "harness" / "src" / "gen"))

from middleware.runtime.v1 import runtime_pb2

from middleware_harness.mcp import McpRuntimeClient, McpRuntimeError


async def main() -> int:
    client = McpRuntimeClient("/tmp/middleware-mcp.sock")
    binding = runtime_pb2.McpBinding(
        server_name="m4allow",
        transport="stdio",
        command=str(ROOT / "mcp_runtime" / ".venv" / "bin" / "python"),
        args=[str(ROOT / "docs/reports/artifacts/mcp/M1/secret_echo_server.py")],
        env={"PROBE_TOKEN": "${secret:m4_token}"},
        allowed_tools=["echo_token"],
        namespace="mcp__m4allow__",
    )
    session = await client.open(binding)
    denied = None
    try:
        await client.call(session.session_id, "mcp__m4allow__other_tool", "{}", 5000)
    except McpRuntimeError as exc:
        denied = exc.code
    await client.close_session(session.session_id)
    print(json.dumps({
        "visible_tools": [tool.name for tool in session.tools],
        "direct_call_other_tool": denied,
    }, ensure_ascii=False))
    await client.close()
    return 0


if __name__ == "__main__":
    raise SystemExit(asyncio.run(main()))
