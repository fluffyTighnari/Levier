"""MCP HTTP server for transport tests (SSE / streamable HTTP).

Run:
    python http_echo_server.py --transport sse --port 9101
    python http_echo_server.py --transport streamable-http --port 9102
"""

from __future__ import annotations

import argparse

from mcp.server.mcpserver import Context, MCPServer

server = MCPServer("http-echo")


@server.tool()
def echo(text: str) -> str:
    """Echo the given text."""
    return text


@server.tool()
def add(a: int, b: int) -> int:
    """Add two integers."""
    return a + b


@server.tool()
def auth_header(ctx: Context) -> str:
    """Return the Authorization header seen by the server (test only)."""
    headers = ctx.headers or {}
    for key, value in headers.items():
        if str(key).lower() == "authorization":
            return str(value)
    return ""


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--transport", choices=["sse", "streamable-http"], default="sse")
    parser.add_argument("--port", type=int, default=9101)
    args = parser.parse_args()
    server.run(args.transport, host="127.0.0.1", port=args.port)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
