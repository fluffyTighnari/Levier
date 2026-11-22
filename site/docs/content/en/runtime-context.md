---
title: Context
status: published
description: ctx fields, input, model, tools, MCP, timeouts and cancellation.
updated: 2026-11-21
---

## 1. Fields

| Field | Content |
|---|---|
| `ctx.runtime_name` | Current Runtime name |
| `ctx.run_id` | Run ID |
| `ctx.app_id` | App ID |
| `ctx.trace_id` | trace ID |
| `ctx.attempt` | Current attempt |
| `ctx.input` | Input message list |
| `ctx.params` | Runtime-specific parameters |
| `ctx.model` | Model configuration resolved for this run; may be None |
| `ctx.tools` | Local / global tool descriptions |
| `ctx.mcp_bindings` | MCP connection descriptions |
| `ctx.timeouts` | Run / tool timeouts |
| `ctx.session_id` | Optional session |
| `ctx.resume_from` | checkpoint state |
| `ctx.cancelled` | Whether the run has been cancelled |
| `ctx.cancel_event` | asyncio cancellation event |
| `ctx.logger` | Logger with run context |

## 2. model

```python
if ctx.model is None:
    await ctx.reply("no model configured")
    return "failed"

reply = await ctx.chat(ctx.input)
```

The model address, provider and key have already been resolved by the control
plane; there is no need to read them from environment variables.

## 3. Tools

```python
tools = await ctx.toolbox()
local = tools.local()
mcp = tools.mcp()
```

`ctx.tools` is a description list; only `toolbox()` opens MCP sessions.

## 4. Timeouts

```python
ctx.timeouts.run_ms
ctx.timeouts.tool_ms
```

Timeouts are issued by the control plane; the Runtime can use them to bound its own loop.

## 5. Cancellation

```python
if ctx.cancelled:
    return "cancelled"

await ctx.cancel_event.wait()
```

Cancellation is cooperative; long-running loops should check it periodically.

## 6. Logging

```python
ctx.logger.info("step %s", step)
```

Automatically carries `run_id` / `app_id` / `runtime` / `attempt` / `trace_id`.

## Next steps

- Tool aggregation: [Toolbox](runtime-toolbox.md)
- Model calls: [chat and invoke](runtime-chat.md)
- Cancellation and errors: [Errors and cancellation](runtime-errors.md)
