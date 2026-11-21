# Minimal MCP echo server

This server is used by the beginner MCP walkthrough.

It is a stdio MCP server. The App never imports it; the shared `mcp_runtime`
starts it and exposes its tools to a Runtime.

## Docker Compose

The `mcp-runtime` service mounts this directory at:

```text
/app/examples/mcp_echo
```

Register it with:

```sh
levier mcp add \
  --name mcp-echo \
  --transport stdio \
  --config-file ./examples/mcp_echo/config.json \
  --allowed-tools echo_text,add_numbers
```

Give the App a reference:

```sh
levier app init \
  --name mcp-demo \
  --mcp-ref mcp-echo \
  --credentials-out ./data/mcp-demo.json
```

The model-visible tool name is:

```text
mcp__mcp_echo__echo_text
```
