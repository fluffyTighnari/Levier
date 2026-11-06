"""Middleware — fake MCP server for stdio integration tests.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

from mcp.server.mcpserver import MCPServer

server = MCPServer("fake")


@server.tool()
def add(a: int, b: int) -> int:
    """Add two integers."""
    return a + b


if __name__ == "__main__":
    server.run("stdio")
