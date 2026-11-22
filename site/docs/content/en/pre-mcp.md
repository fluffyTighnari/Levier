---
title: "MCP configuration"
status: published
description: "Register MCP servers and configure transports, secrets and allowed_tools."
updated: 2026-11-21
---

## Servers

> [!NOTE]
> An App does not implement an MCP client; it only declares references. The shared MCP Runtime makes the actual connection.

### 1. Who does what

```text
App           only declares mcp_refs
Levier        stores MCP Servers and allowed_tools
MCP Runtime   connects to MCP Servers, resolves secret placeholders
Runtime       uses the resolved tools through ctx.toolbox()
```

### 2. mcp add

```sh
levier mcp add \
  --name echo \
  --transport stdio \
  --config-file ./examples/mcp_echo/config.json
```

> [!OUTPUT]
> ```text
> MCP server saved: echo (stdio)
> ```

Parameters:

| Parameter | Required | Description |
|---|---|---|
| `--name` | Yes | The name the App references |
| `--config-file` | Yes | The MCP Server's own JSON config |
| `--transport` | No | `stdio` (default), `sse`, `streamable_http` |
| `--allowed-tools` | No | Comma-separated tool whitelist |
| `--server` | No | Control plane address |
| `--admin-token` | No | Admin token |
| `--json` | No | Machine-readable output |

`mcp add` is an administrative operation; on a local stack it works directly once `levier up` has written the profile.

### 3. mcp list

```sh
levier mcp list
```

> [!OUTPUT]
> ```text
> name                 transport          secret_refs
> echo                 stdio
> ```

Shows server metadata only, not secret values.

### 4. mcp remove

```sh
levier mcp remove --name echo --yes
```

If an existing App still references it:

```sh
levier mcp remove --name echo --yes --force
```

`--force` also clears the App references. After removal, the App's next connect or init fails until the reference is removed.

### 5. Next steps

- Write the config, secret placeholders and allowed_tools: see below.
- Declare references in an App: [Reference MCP servers](app-mcp.md)
- Use tools in a Runtime: [Toolbox](runtime-toolbox.md)
- Failure codes: [Errors and troubleshooting](errors.md)

## Config and tools

### 1. stdio config

```json
{
  "command": "npx",
  "args": ["-y", "@modelcontextprotocol/server-github"],
  "env": {
    "GITHUB_PERSONAL_ACCESS_TOKEN": "${secret:github_token}"
  }
}
```

```sh
levier mcp add \
  --name github \
  --transport stdio \
  --config-file ./github.mcp.json \
  --allowed-tools search_repositories,get_file_contents
```

### 2. remote config

```json
{
  "url": "https://mcp.example.com/mcp",
  "headers": {
    "Authorization": "Bearer ${secret:remote_token}"
  }
}
```

```sh
levier mcp add \
  --name remote \
  --transport streamable_http \
  --config-file ./remote.mcp.json
```

Available `transport` values:

```text
stdio             child process + stdin/stdout
sse               Server-Sent Events
streamable_http   HTTP streaming
```

### 3. Secret placeholders

`${secret:name}` is resolved by the MCP Runtime before connecting and never reaches the Runtime or events in plaintext.

Sources:

```text
LEVIER_SECRET_<NAME>
LEVIER_MCP_SECRETS_FILE points to a 0600 JSON file
```

Example:

```json
{
  "github_token": "ghp_...",
  "remote_token": "..."
}
```

### 4. allowed_tools

`allowed_tools` is a whitelist enforced by the MCP Runtime:

```sh
levier mcp add \
  --name github \
  --transport stdio \
  --config-file ./github.mcp.json \
  --allowed-tools search_repositories,get_file_contents
```

Tools outside the whitelist return `TOOL_DENIED`. When `allowed_tools` is omitted,
the MCP Runtime handles whatever tools the server exposes.

### 5. Tool naming

The model-facing name is always:

```text
mcp__<server>__<tool>
```

For example:

```text
mcp__github__search_repositories
```

Neither the App nor the Runtime needs to build this name; `ctx.toolbox()` handles it.

### 6. Failure codes

| code | Meaning |
|---|---|
| `MCP_RUNTIME_UNAVAILABLE` | The shared MCP Runtime is not running |
| `MCP_CONNECT_FAILED` | The MCP Server failed to start or connect |
| `SECRET_MISSING` | `${secret:...}` has no matching value |
| `TOOL_DENIED` | The tool is not in `allowed_tools` |
| `TOOL_TIMEOUT` | The MCP tool call timed out |
| `MCP_RUNTIME_ERROR` | The MCP Runtime returned an error |

A single failing MCP Server does not require restarting the control plane or the Runtime; they are independent failure domains.

## Next steps

- Model: [Model configuration](pre-model.md)
- Keys: [Environment and secrets](pre-env.md)
- App references: [Reference MCP servers](app-mcp.md)
