"""Minimal MCP stdio server used by the beginner example.

Run it through the shared MCP Runtime; the App never imports this file.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

from mcp.server.mcpserver import MCPServer

server = MCPServer("echo")


@server.tool()
def echo_text(text: str) -> str:
    """Return the text unchanged."""
    return text


@server.tool()
def add_numbers(a: int, b: int) -> int:
    """Add two integers."""
    return a + b


if __name__ == "__main__":
    server.run("stdio")
