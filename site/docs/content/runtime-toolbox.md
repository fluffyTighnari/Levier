---
title: Toolbox
status: published
description: 本地与 MCP 工具聚合、调用与关闭。
updated: 2026-11-21
---

## 1. toolbox

```python
tools = await ctx.toolbox()
```

它按 Run 缓存，聚合：

```text
ctx.tools          本地 / global 工具
ctx.mcp_bindings   打开的 MCP session
```

## 2. 选择工具

```python
tools.local()   # 只取本地 / global
tools.mcp()     # 只取 MCP
tools.specs()   # 转成模型工具描述
tools.find("counter_add")
```

## 3. 调用

```python
result = await tools.call("counter_add", {"delta": 2})
if not result.ok:
    await ctx.error(result.error_code, result.error_message)
```

也可以走 SDK 工具桥：

```python
result = await ctx.call_tool("counter_add", {"delta": 2})
```

`ctx.call_tool` 会发 `TOOL_CALL_BEGIN` / `TOOL_CALL_END`；`tools.call`
是更底层的统一入口。

## 4. MCP

```python
mcp_tools = tools.mcp()
```

MCP tool 的名字是：

```text
mcp__<server>__<tool>
```

session 按 Run 打开，Run 结束后关闭。不要跨 Run 复用。

## 5. 关闭

```python
await ctx.close_toolbox()
```

通常由 SDK 在 Run 结束时处理；手动关闭用于长生命周期或提前退出。

## 6. 失败

| code | 含义 |
|---|---|
| `TOOL_NOT_FOUND` | 工具不存在 |
| `TOOL_TIMEOUT` | 工具超时 |
| `TOOL_EXEC_ERROR` | handler 失败 |
| `TOOL_DENIED` | MCP 白名单拒绝 |
| `MCP_CONNECT_FAILED` | MCP 连接失败 |

## 下一步

- MCP 配置：[MCP 配置与 allowed_tools](pre-mcp.md)
- 模型调用：[chat 与 invoke](runtime-chat.md)
- 错误与取消：[错误与取消](runtime-errors.md)
