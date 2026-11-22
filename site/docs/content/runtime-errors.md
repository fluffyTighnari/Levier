---
title: 错误与 checkpoint
status: published
description: 处理工具与模型错误、取消与 checkpoint 状态。
updated: 2026-11-21
---

## 错误与取消

### 1. ctx.error

```python
await ctx.error(
    "MY_ERROR",
    "something failed",
    retryable=False,
)
```

参数：

| 参数 | 说明 |
|---|---|
| `code` | 稳定错误码 |
| `message` | 可读信息 |
| `retryable` | 是否可重试 |
| `data` | 可选结构化数据 |

### 2. ToolResult

```python
result = await ctx.call_tool("search", {"query": "levier"})
if not result.ok:
    await ctx.error(result.error_code, result.error_message)
```

`ToolResult` 已经包含错误码；不要解析展示文本。

### 3. 未捕获异常

Runtime 里未捕获的异常会自动变成：

```text
ERROR 事件
Run failed
```

所以不需要在每个分支手写 try/except，但关键步骤仍应给出明确错误码。

### 4. 取消

```python
if ctx.cancelled:
    return "cancelled"
```

或等待：

```python
await ctx.cancel_event.wait()
```

取消是协作式的。长循环、长工具调用、长模型调用都应定期检查。

### 5. 事件

错误会出现在：

```text
ERROR
TOOL_CALL_END
RUN_STATUS
```

用：

```sh
levier run show <run_id> --events
```

定位。

### 6. 返回终态

```python
return "completed"
return "failed"
return "cancelled"
```

返回值是 Run 终态；如果返回未知字符串，按实现映射，建议只使用这三个。

## checkpoint

### 1. 保存

```python
await ctx.checkpoint({"step": step, "messages": messages})
```

支持 `bytes` 或 mapping。需要 `ctx.session_id`。

### 2. session

```python
ctx.session_id
ctx.resume_from
```

`resume_from` 是控制面下发的 checkpoint 状态；没有时为 `None`。

### 3. 恢复

```python
state = ctx.resume_from
if state:
    messages = state["messages"]
```

checkpoint 的格式由 Runtime 自己定义，SDK 只负责传输。

### 4. 边界

当前 MVP：

```text
checkpoint 传输与保存可用
长期 checkpoint 续跑未完全实现
```

不要把它当作持久化工作流引擎；需要强恢复时自己落业务存储。

### 5. 建议

- checkpoint 尽量小。
- 不要放 secret。
- 版本化自己的状态结构。
- 失败时能从头重跑，而不是只依赖 checkpoint。

## 下一步

- Runtime 核心流程：[Runtime 核心流程](runtime-core.md)
- Context：[Context](runtime-context.md)
- Toolbox：[Toolbox](runtime-toolbox.md)
