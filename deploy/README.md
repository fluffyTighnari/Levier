<!-- Middleware — deployment images. -->

# Deployment Images

Build from the repository root:

```sh
docker build -f deploy/harness.Dockerfile -t middleware-harness:dev .
docker build -f deploy/mcp-runtime.Dockerfile -t middleware-mcp-runtime:dev .
```

The Harness image contains the local `mcp_runtime` client only. It does not
contain MCP connections or MCP server processes. Run `middleware-mcp-runtime`
as a separate process or sidecar and point the Harness at its Unix socket:

```yaml
mcp_runtime:
  socket: "/run/middleware/mcp.sock"
```

The Harness image can start with the example configuration and `--check`:

```sh
docker run --rm middleware-harness:dev --check
```

A full echo run also requires a reachable Middleware RuntimeChannel endpoint.
