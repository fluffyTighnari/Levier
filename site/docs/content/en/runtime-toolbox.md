---
title: Toolbox
status: published
description: Aggregate local and MCP tools, invoke them and close the toolbox.
updated: 2026-11-21
---

## 1. toolbox

```python
tools = await ctx.toolbox()
```

It is cached per Run and aggregates:

```text
ctx.tools          local / global tools
ctx.mcp_bindings   open MCP sessions
```

## 2. Selecting tools

```python
tools.local()   # local / global only
tools.mcp()     # MCP only
tools.specs()   # convert to model tool specs
tools.find("counter_add")
```

## 3. Invoking

```python
result = await tools.call("counter_add", {"delta": 2})
if not result.ok:
    await ctx.error(result.error_code, result.error_message)
```

You can also go through the SDK tool bridge:

```python
result = await ctx.call_tool("counter_add", {"delta": 2})
```

`ctx.call_tool` emits `TOOL_CALL_BEGIN` / `TOOL_CALL_END`; `tools.call` is the
lower-level unified entry point.

## 4. MCP

```python
mcp_tools = tools.mcp()
```

MCP tool names are:

```text
mcp__<server>__<tool>
```

Sessions are opened per Run and closed when the Run ends. Do not reuse them across Runs.

## 5. Closing

```python
await ctx.close_toolbox()
```

Usually handled by the SDK at the end of a Run; closing manually is for long-lived runs or early exit.

## 6. Failures

| code | Meaning |
|---|---|
| `TOOL_NOT_FOUND` | Tool does not exist |
| `TOOL_TIMEOUT` | Tool timed out |
| `TOOL_EXEC_ERROR` | handler failed |
| `TOOL_DENIED` | Rejected by the MCP allowlist |
| `MCP_CONNECT_FAILED` | MCP connection failed |

## Next steps

- MCP configuration: [MCP config and allowed_tools](pre-mcp.md)
- Model calls: [chat and invoke](runtime-chat.md)
- Errors and cancellation: [Errors and cancellation](runtime-errors.md)
