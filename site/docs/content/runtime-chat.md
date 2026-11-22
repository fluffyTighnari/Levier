---
title: chat 与 invoke
status: published
description: ctx.chat、ctx.invoke、ctx.invoke_all。
updated: 2026-11-21
---

## 1. ctx.chat

```python
tools = await ctx.toolbox()
reply = await ctx.chat(ctx.input, tools=tools)
```

`tools=None` 时自动使用 toolbox。

参数：

```text
messages       默认 ctx.input
tools          模型工具描述
on_delta       流式回调
```

## 2. 流式

```python
async def on_delta(text: str) -> None:
    await ctx.reply(text, role="assistant")

reply = await ctx.chat(ctx.input, tools=tools, on_delta=on_delta)
```

是否流式取决于模型和 Runtime 的循环设计。

## 3. ctx.invoke

执行单个模型 tool call：

```python
result = await ctx.invoke(reply.tool_calls[0], toolbox=tools)
```

## 4. ctx.invoke_all

执行响应中的所有 tool calls：

```python
results = await ctx.invoke_all(reply, toolbox=tools)
```

local / MCP 自动分流，并自动发 `TOOL_CALL_BEGIN` / `TOOL_CALL_END`。

## 5. 自己拼循环

```python
for step in range(4):
    tools = await ctx.toolbox()
    reply = await ctx.chat(ctx.input, tools=tools)

    if not reply.tool_calls:
        await ctx.reply(reply.content or "done")
        return "completed"

    await ctx.invoke_all(reply, toolbox=tools)
```

这是结构示意。如何把工具结果拼回下一轮消息、是否保留历史、何时终止，
都由你的 Runtime 决定；具体消息类型以 Runtime SDK 的类型定义为准。

## 6. 返回

`ctx.chat` 返回包含：

```text
content
tool_calls
usage（如果模型提供）
```

具体字段以 Runtime SDK 的类型定义为准。

## 下一步

- Context 字段：[Context](runtime-context.md)
- Toolbox：[Toolbox](runtime-toolbox.md)
- 错误与取消：[错误与取消](runtime-errors.md)
