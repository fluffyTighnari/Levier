"""M7: real SSE and streamable_http transports through mcp_runtime."""

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


async def probe(client: McpRuntimeClient, transport: str, url: str, namespace: str, server_name: str) -> dict:
    binding = runtime_pb2.McpBinding(
        server_name=server_name,
        transport=transport,
        url=url,
        headers={"Authorization": "Bearer ${secret:http_token}"},
        namespace=namespace,
    )
    session = await client.open(binding)
    echo = await client.call(session.session_id, f"{namespace}echo", '{"text":"ok"}', 5000)
    header = await client.call(session.session_id, f"{namespace}auth_header", "{}", 5000)
    await client.close_session(session.session_id)
    return {
        "transport": transport,
        "tools": [tool.name for tool in session.tools],
        "echo_ok": echo.ok,
        "echo_result": echo.result_json,
        "auth_header_ok": header.ok,
        "auth_header_result": header.result_json,
    }


async def main() -> int:
    client = McpRuntimeClient("/tmp/middleware-mcp.sock")
    results = []
    results.append(await probe(client, "sse", "http://127.0.0.1:9101/sse", "mcp__sse_echo__", "sse-echo"))
    results.append(await probe(client, "streamable_http", "http://127.0.0.1:9102/mcp", "mcp__http_echo__", "http-echo"))
    await client.close()
    print(json.dumps({"results": results}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(asyncio.run(main()))
