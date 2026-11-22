---
title: Context
status: published
description: ctx 字段、input/model/tools/mcp、超时与取消。
updated: 2026-11-21
---

## 1. 字段

| 字段 | 内容 |
|---|---|
| `ctx.runtime_name` | 当前 Runtime 名 |
| `ctx.run_id` | Run ID |
| `ctx.app_id` | App ID |
| `ctx.trace_id` | trace ID |
| `ctx.attempt` | 当前 attempt |
| `ctx.input` | 输入消息列表 |
| `ctx.params` | Runtime 自定义参数 |
| `ctx.model` | 本次解析后的模型配置，可能为 None |
| `ctx.tools` | 本地 / global 工具描述 |
| `ctx.mcp_bindings` | MCP 连接描述 |
| `ctx.timeouts` | Run / 工具超时 |
| `ctx.session_id` | 可选 session |
| `ctx.resume_from` | checkpoint 状态 |
| `ctx.cancelled` | 是否已取消 |
| `ctx.cancel_event` | asyncio 取消事件 |
| `ctx.logger` | 带 run 上下文的 logger |

## 2. model

```python
if ctx.model is None:
    await ctx.reply("no model configured")
    return "failed"

reply = await ctx.chat(ctx.input)
```

模型地址、provider、key 已由控制面解析，不需要从环境变量读取。

## 3. 工具

```python
tools = await ctx.toolbox()
local = tools.local()
mcp = tools.mcp()
```

`ctx.tools` 是描述列表；`toolbox()` 才会打开 MCP session。

## 4. 超时

```python
ctx.timeouts.run_ms
ctx.timeouts.tool_ms
```

超时由控制面下发；Runtime 可以据此设置自己的循环上限。

## 5. 取消

```python
if ctx.cancelled:
    return "cancelled"

await ctx.cancel_event.wait()
```

取消是协作式的；长时间循环应定期检查。

## 6. 日志

```python
ctx.logger.info("step %s", step)
```

自动带 `run_id` / `app_id` / `runtime` / `attempt` / `trace_id`。

## 下一步

- 工具聚合：[Toolbox](runtime-toolbox.md)
- 模型调用：[chat 与 invoke](runtime-chat.md)
- 取消与错误：[错误与取消](runtime-errors.md)
