"""M5: open the official filesystem MCP server and list its tools."""

from __future__ import annotations

import asyncio
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[5]
sys.path.insert(0, str(ROOT / "harness" / "src"))
sys.path.insert(0, str(ROOT / "harness" / "src" / "gen"))

from middleware.runtime.v1 import runtime_pb2

from middleware_harness.mcp import McpRuntimeClient


async def main() -> int:
    client = McpRuntimeClient("/tmp/middleware-mcp.sock")
    binding = runtime_pb2.McpBinding(
        server_name="m5fs",
        transport="stdio",
        command="/Users/mac/.nvm/versions/node/v24.21.0/bin/node",
        args=[
            "/tmp/m5-fs/node_modules/@modelcontextprotocol/server-filesystem/dist/index.js",
            "/tmp/m5-mcp",
        ],
        namespace="mcp__m5fs__",
    )
    session = await client.open(binding)
    names = [tool.name for tool in session.tools]
    read_name = next((name for name in names if name.endswith("read_text_file")), "")
    result = None
    if read_name:
        result = await client.call(
            session.session_id,
            read_name,
            json.dumps({"path": "/tmp/m5-mcp/hello.txt"}),
            15000,
        )
    await client.close_session(session.session_id)
    print(json.dumps({
        "tool_count": len(names),
        "tools": names,
        "read_tool": read_name,
        "read_ok": bool(result and result.ok),
        "read_result": result.result_json if result else None,
    }, ensure_ascii=False))
    await client.close()
    return 0


if __name__ == "__main__":
    raise SystemExit(asyncio.run(main()))
