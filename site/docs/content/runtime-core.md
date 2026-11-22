---
title: Runtime 核心流程
status: published
description: runtime init、run(ctx)、start 与错误处理。
updated: 2026-11-21
---

> [!NOTE]
> 本地栈默认已经有 `react1` 与 `echo1`。只有需要自定义执行逻辑时才写 Runtime。

## 1. 什么时候需要 Runtime

```text
内置 react1 满足常规 ReAct 循环   → 不需要写
想控制 prompt、工具顺序、循环    → 写自己的 Runtime
只想验证链路                      → 用 echo1
```

## 2. runtime init

```sh
levier runtime init
```

> [!OUTPUT]
> ```text
> created runtime.py
> start it with: levier runtime start
> ```

默认生成 `./runtime.py`。建议把 Runtime 放在独立目录：

```text
my-runtime/
  runtime.py
  pyproject.toml
```

## 3. run(ctx)

```python
from levier import Context, Runtime


async def run(ctx: Context) -> str:
    tools = await ctx.toolbox()

    # 自己决定 prompt 怎么拼
    reply = await ctx.chat(ctx.input, tools=tools)

    # 一行执行模型请求的所有工具，local/MCP 自动分流
    await ctx.invoke_all(reply, toolbox=tools)

    await ctx.reply(reply.content or "done")
    return "completed"


if __name__ == "__main__":
    Runtime.start(run)
```

返回的字符串是终态：通常 `completed` / `failed`。也可以用 `ctx.error`。

## 4. runtime start

```sh
levier runtime start
```

> [!OUTPUT]
> ```text
> runtime: my-runtime
> file:    /path/to/runtime.py
> server:  127.0.0.1:9090
> ```

默认从 profile 读取 runtime name、gRPC 地址和 token。

## 5. 错误处理

未捕获异常会自动变成：

```text
ERROR 事件
Run failed
```

显式错误：

```python
await ctx.error("MY_ERROR", "something failed", retryable=False)
```

工具错误：

```python
result = await ctx.call_tool("search", {"query": "levier"})
if not result.ok:
    await ctx.error(result.error_code, result.error_message)
```

## 6. 不需要

```text
runtime.yaml
runtime_token
daemon id
gRPC / protobuf
event seq / lease_epoch / ack
MCP client
```

这些由 SDK 处理。需要覆盖时再看后续页面。

## 下一步

- 生成与目录：[runtime init](runtime-start.md)
- 启动参数：[runtime start](runtime-start.md)
- 上下文 API：[Context](runtime-context.md)
- 错误与取消：[错误与取消](runtime-errors.md)
