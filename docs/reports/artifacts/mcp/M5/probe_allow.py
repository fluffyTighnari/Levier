"""M5: allowlist applied to the real filesystem server."""

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
        allowed_tools=["read_text_file", "list_directory"],
        namespace="mcp__m5fs__",
    )
    session = await client.open(binding)
    print(json.dumps({"visible_tools": [tool.name for tool in session.tools]}, ensure_ascii=False))
    await client.close_session(session.session_id)
    await client.close()
    return 0


if __name__ == "__main__":
    raise SystemExit(asyncio.run(main()))
