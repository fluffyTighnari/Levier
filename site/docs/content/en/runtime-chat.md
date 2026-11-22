---
title: chat and invoke
status: published
description: ctx.chat, ctx.invoke and ctx.invoke_all.
updated: 2026-11-21
---

## 1. ctx.chat

```python
tools = await ctx.toolbox()
reply = await ctx.chat(ctx.input, tools=tools)
```

When `tools=None`, the toolbox is used automatically.

Parameters:

```text
messages       defaults to ctx.input
tools          model tool descriptions
on_delta       streaming callback
```

## 2. Streaming

```python
async def on_delta(text: str) -> None:
    await ctx.reply(text, role="assistant")

reply = await ctx.chat(ctx.input, tools=tools, on_delta=on_delta)
```

Whether streaming is used depends on the model and the Runtime loop design.

## 3. ctx.invoke

Executes a single model tool call:

```python
result = await ctx.invoke(reply.tool_calls[0], toolbox=tools)
```

## 4. ctx.invoke_all

Executes all tool calls in the response:

```python
results = await ctx.invoke_all(reply, toolbox=tools)
```

Local / MCP calls are routed automatically, and `TOOL_CALL_BEGIN` / `TOOL_CALL_END`
are emitted automatically.

## 5. Building your own loop

```python
for step in range(4):
    tools = await ctx.toolbox()
    reply = await ctx.chat(ctx.input, tools=tools)

    if not reply.tool_calls:
        await ctx.reply(reply.content or "done")
        return "completed"

    await ctx.invoke_all(reply, toolbox=tools)
```

This is a structural sketch. How tool results are appended to the next round of
messages, whether history is kept, and when to stop are all up to your Runtime;
for concrete message types, refer to the Runtime SDK type definitions.

## 6. Return value

`ctx.chat` returns:

```text
content
tool_calls
usage (if provided by the model)
```

For concrete fields, refer to the Runtime SDK type definitions.

## Next steps

- Context fields: [Context](runtime-context.md)
- Toolbox: [Toolbox](runtime-toolbox.md)
- Errors and cancellation: [Errors and cancellation](runtime-errors.md)
