"""MCP stdio server that echoes one environment secret (M1 probe)."""

from __future__ import annotations

import os

from mcp.server.mcpserver import MCPServer

server = MCPServer("secret-echo")


@server.tool()
def echo_token() -> str:
    """Return the PROBE_TOKEN environment value."""
    return os.environ.get("PROBE_TOKEN", "")


@server.tool()
def echo_pid() -> str:
    """Return this MCP server process id."""
    return str(os.getpid())


@server.tool()
def other_tool() -> str:
    """A second tool used for allowlist tests."""
    return "other"


if __name__ == "__main__":
    server.run("stdio")
