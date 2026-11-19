<!-- Levier — independent MCP runtime process. -->

# MCP Runtime

## 单元卡

| 项 | 内容 |
|---|---|
| 职责 | MCP 连接、会话、stdio 子进程、工具发现与调用 |
| 入口 | `python -m levier_mcp_runtime --socket /tmp/levier-mcp.sock` |
| 输入 / 输出 | 本地 JSON-RPC；MCP server 连接与工具结果 |
| 依赖 | `mcp` SDK、MCP Server、本地 socket |
| 不负责 | 控制面 API、Run 调度、事件持久化、App 注册 |
| 测试 | `make mcp-runtime-test` |

`mcp_runtime` is an independent execution-side process. It owns MCP server
connections, stdio child processes, tool discovery, and tool calls.

It is **not** part of the Runtime SDK and **not** part of the Go middleware.
A runtime connects to it through `levier_runtime.mcp.McpRuntimeClient`;
the middleware only stores MCP catalog/ACL/bindings.

## Boundary

```text
Runtime McpRuntimeClient  -- local JSON-RPC -->  mcp_runtime  --> MCP servers
Levier Register/Control  -- TaskAssign.mcp_bindings -->  Runtime
```

- The middleware never opens MCP connections.
- The runtime never talks to the middleware directly.
- TaskAssign carries connection descriptors, not live connections.
- Tool names are namespaced as `mcp__<server_name>__<tool_name>`.
- `allowed_tools` is enforced by the runtime.

## Run

From `execution/mcp_runtime/`:

```sh
uv sync --locked
PYTHONPATH=src .venv/bin/python -m levier_mcp_runtime \
  --socket /tmp/levier-mcp.sock
```

Single-client debug mode:

```sh
PYTHONPATH=src .venv/bin/python -m levier_mcp_runtime --stdio
```

## Tests

```sh
make mcp-runtime-test
```

The runtime includes a real stdio integration test using a small MCP server.
SSE and streamable HTTP transport selection is covered with fakes; live
network servers are a follow-up integration test.

Design: `docs/adr/0004-mcp-ownership.md`.
