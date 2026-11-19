# MCP

MCP servers are connected by the independent `mcp_runtime` process. Apps only
declare references; Runtimes receive resolved bindings. App code does not
implement an MCP client.

## 1. Register an MCP server

Create a config file for the MCP server. For stdio:

```json
{
  "command": "npx",
  "args": ["-y", "@modelcontextprotocol/server-github"],
  "env": {
    "GITHUB_PERSONAL_ACCESS_TOKEN": "${secret:github_token}"
  }
}
```

Register it:

```sh
levier mcp add \
  --name github \
  --transport stdio \
  --config-file ./github.mcp.json \
  --allowed-tools search_repositories,get_file_contents
```

List or remove:

```sh
levier mcp list
levier mcp remove --name github --yes [--force]
```

For a remote MCP server use a URL/header config:

```json
{
  "url": "https://mcp.example.com/mcp",
  "headers": {"Authorization": "Bearer ${secret:remote_token}"}
}
```

## 2. Give the App a reference

```sh
levier app init \
  --name counter-agent \
  --mcp-ref github \
  --credentials-out ./data/agent/credentials.json
```

or in the App:

```python
app = LevierApp.from_credentials(mcp_refs=["github"])
```

or:

```sh
export LEVIER_APP_MCP_REFS=github
```

Only registered references are accepted. An unknown reference fails App
registration.

## 3. Runtime

The React Runtime talks to the shared MCP Runtime socket:

```yaml
mcp_runtime:
  socket: "/tmp/levier-mcp.sock"
```

Start the MCP Runtime:

```sh
PYTHONPATH=execution/mcp_runtime/src \
  python -m levier_mcp_runtime \
  --socket /tmp/levier-mcp.sock \
  --secrets-file /path/to/mcp-secrets.json
```

Secrets can also come from:

```text
LEVIER_SECRET_<NAME>
```

The MCP Runtime resolves `${secret:...}` placeholders before connecting to the
MCP server.

## 4. Tool naming

MCP tools are exposed to the model as:

```text
mcp__<server>__<tool>
```

Example:

```text
mcp__github__search_repositories
```

`allowed_tools` is enforced by the MCP Runtime; other tools return
`TOOL_DENIED`.

## 5. Failure handling

| code | meaning |
|---|---|
| `MCP_RUNTIME_UNAVAILABLE` | shared MCP Runtime is not running |
| `MCP_CONNECT_FAILED` | MCP server could not start/connect |
| `SECRET_MISSING` | `${secret:...}` has no configured value |
| `TOOL_DENIED` | tool not in `allowed_tools` |
| `TOOL_TIMEOUT` | MCP tool call timed out |
| `MCP_RUNTIME_ERROR` | MCP Runtime returned an error |

MCP is a shared process and an independent failure domain: one bad MCP server
does not need to restart the control plane or the React Runtime.

For a full end-to-end example see [`../README.md`](../README.md) and
`tests/e2e/mcp_e2e.sh`.
