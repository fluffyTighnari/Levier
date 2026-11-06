<!-- Middleware — independent MCP runtime process. -->

# MCP Runtime

`mcp_runtime` is an independent execution-side process. It owns MCP server
connections, stdio child processes, tool discovery, and tool calls.

It is **not** part of the Python Harness package and **not** part of the Go
middleware. A Harness connects to it through the local `McpRuntimeClient`;
the middleware only stores MCP catalog/ACL/bindings.

## Boundary

```text
Harness McpRuntimeClient  ── local JSON-RPC ──►  mcp_runtime  ──► MCP servers
Middleware Register/Control  ── TaskAssign.mcp_bindings ──► Harness
```

- The middleware never opens MCP connections.
- The runtime never talks to the middleware directly.
- TaskAssign carries connection descriptors, not live connections.
- Tool names are namespaced as `mcp__<server_name>__<tool_name>`.
- `allowed_tools` is enforced by the runtime.

## Status

Design and process skeleton are defined in `docs/adr/0004-mcp-ownership.md`.
Implementation lands in P3-29.
