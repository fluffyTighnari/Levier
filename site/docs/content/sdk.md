---
title: SDK 总览
status: published
description: 按用途归纳：App、Runtime、工具、Run、配置与安装。
updated: 2026-11-21
---

> [!NOTE]
> 这一页按“你要做什么”组织，不按包层级组织。大多数项目只使用
> `from levier import ...`。

## 1. 按用途选择

| 你要做 | 使用 |
|---|---|
| 把业务函数接成工具 | `App` + `@tool` |
| 提交/查询 Run | `App.submit_run` / `wait_run` / `get_events` |
| 写自己的执行循环 | `Runtime` + `run(ctx)` |
| 调模型或工具 | `ctx.chat` / `ctx.toolbox` |
| 显式控制凭据/传输 | `levier_app.LevierApp` |
| 读配置与 secret | profile / `.levier.json` / 环境变量 |

## 2. App 接入

```python
from levier import App, tool

@tool
def counter_add(delta: int) -> dict:
    """Add delta to the counter."""
    return {"value": delta}

app = App(tools=[counter_add])
app.run()
```

细节：[App 核心流程](app-core.md) · [App 构造](app-install.md) · [工具与 schema](app-install.md)

## 3. Runtime 接入

```python
from levier import Context, Runtime

async def run(ctx: Context) -> str:
    tools = await ctx.toolbox()
    reply = await ctx.chat(ctx.input, tools=tools)
    await ctx.invoke_all(reply, toolbox=tools)
    await ctx.reply(reply.content or "done")
    return "completed"

Runtime.start(run)
```

细节：[Runtime 核心流程](runtime-core.md) · [Context](runtime-context.md) · [Toolbox](runtime-toolbox.md)

## 4. 工具与 schema

```text
@tool
app.tool
App.add
Tool.invoke
Tool.__call__
schema_from_function
ToolExecutionError
```

不需要手写 JSON Schema；需要覆盖时显式传 `schema=`。

细节：[工具与 schema](app-install.md)

## 5. Run 与事件

```text
App.submit_run
App.wait_run
App.get_run
App.get_events
App.cancel_run
```

事件：

```text
RUN_STATUS / MESSAGE_DELTA / MESSAGE_COMPLETED
TOOL_CALL_BEGIN / TOOL_CALL_END / ERROR / USAGE
```

细节：[SDK 提交 Run](app-runs.md) · [Run 与事件](app-runs.md)

## 6. 配置与凭据

```text
用户 profile
项目 .levier.json
环境变量
managed secret
App 凭据文件
```

App 默认从 profile / `.levier.json` 读取；Runtime 默认从 profile / 环境变量读取。

细节：[配置与优先级](pre-env.md) · [凭据与密钥](pre-env.md)

## 7. 安装与版本

```sh
pip install levier
```

源码：

```sh
python -m pip install -e ./sdk/python -e ./execution/runtime_sdk -e ./sdk/levier
```

App-only 安装不会立即加载 Runtime 的 gRPC 依赖。

细节：[安装 SDK](app-install.md) · [Runtime 安装](runtime-start.md)

## 8. 低层包

只在需要显式控制时使用：

```text
levier_app.LevierApp       显式凭据/传输/工具注册
levier_runtime             Runtime 底层 API
levier_runtime.react       可选 ReAct helper
```

日常接入优先 `from levier import ...`。

## 下一步

- CLI：[CLI 总览](cli.md)
- 环境变量：[环境变量](env.md)
- 术语：[术语表](glossary.md)
