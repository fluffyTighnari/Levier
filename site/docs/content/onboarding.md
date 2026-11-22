---
title: 完整接入
status: published
description: 从安装到第一次 Run 的最小完整路径；细节指向各管理面。
updated: 2026-11-21
---

> [!NOTE]
> 这一页只走完一条完整路径。命令参数、错误处理和更多能力在每节末尾的链接里。

## 1. 安装 CLI

发布版：

```sh
curl -fsSL https://raw.githubusercontent.com/fluffyTighnari/levier/main/scripts/install.sh | sh
```

从源码开发：

```sh
make install
levier setup --server http://127.0.0.1:8080
```

确认命令可用：

```sh
levier version
```

> [!OUTPUT]
> ```text
> levier v0.1.0
> ```

细节：[安装与升级](levier-install.md)

## 2. 启动

```sh
levier up
```

> [!OUTPUT]
> ```text
> waiting for http://127.0.0.1:8080 ...
> levier is ready
> ```

```sh
levier status
```

> [!OUTPUT]
> `health`、`ready`、`database` 和 `daemons` 状态正常：
>
> ```text
> status      ok
> health      ok
> ready       ok
> database    up
> daemons     connected=2 healthy=2 capacity=8 used=0
> ```

停止：

```sh
levier down
```

细节：[启动](levier-install.md) · [状态](levier-status.md) · [停止](levier-status.md)

## 按需配置：模型

你只需要知道三件事：

```text
URL
API key
model 名称
```

执行：

```sh
levier model connect \
  --url https://api.deepseek.com \
  --key sk-... \
  --model deepseek-chat
```

> [!OUTPUT]
> ```text
> model connected: deepseek
> default:          true
> ```

确认：

```sh
levier model list
levier model check --name deepseek
```

> [!OUTPUT]
> ```text
> provider check: PASS deepseek
> ```

细节：[模型连接](pre-model.md) · [模型验证](pre-model.md)

## 按需配置：MCP

```sh
levier mcp add \
  --name echo \
  --transport stdio \
  --config-file ./examples/mcp_echo/config.json
```

确认：

```sh
levier mcp list
```

> [!OUTPUT]
> ```text
> name                 transport          secret_refs
> echo                 stdio
> ```

细节：[MCP 接入](pre-mcp.md) · [MCP 配置与 allowed_tools](pre-mcp.md)

## 3. 接入 App

安装 SDK：

```sh
pip install levier
```

在项目里新增 `app.py`：

```python
from levier import App, tool

counter = {"value": 0}


@tool
def counter_add(delta: int) -> dict:
    """Add delta to the counter."""
    counter["value"] += delta
    return {"value": counter["value"]}


@tool
def counter_get() -> dict:
    """Read the counter."""
    return {"value": counter["value"]}


app = App(tools=[counter_add, counter_get])


if __name__ == "__main__":
    app.run()
```

在项目目录执行：

```sh
levier init
```

> [!OUTPUT]
> ```text
> ready: counter-agent
> app:     app_01...
> server:  http://127.0.0.1:8080
> runtime: react1
> ```

启动 App：

```sh
levier dev
```

另开一个终端提交 Run：

```sh
levier run "Add 2, then add 3, then read the counter."
```

> [!OUTPUT]
> ```text
> run submitted: 01J...
> status: queued
> status: completed
> runtime_name: react1
> final: 5
> ```

也可以让 CLI 临时启动 App：

```sh
levier run "Add 2, then add 3, then read the counter." --with-app
```

SDK 一次性提交：

```python
app.run("Add 2, then add 3, then read the counter.")
```

细节：[App 核心流程](app-core.md) · [@tool 与 schema](app-install.md) · [项目初始化](app-dev.md) · [dev 与 serve](app-dev.md) · [CLI 提交 Run](app-runs.md) · [SDK 提交 Run](app-runs.md) · [错误与重连](app-mcp.md)

更多 App 能力：[App 自检](app-manage.md) · [App 管理](app-manage.md) · [Run 与事件](app-runs.md) · [MCP 引用](app-mcp.md)

## 4. 接入 Runtime

本地栈默认已经有：

```text
react1    内置 ReAct Runtime
echo1     最小 Echo Runtime
```

生成模板：

```sh
levier runtime init
```

> [!OUTPUT]
> ```text
> created runtime.py
> start it with: levier runtime start
> ```

编辑生成的 `runtime.py`：

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

启动：

```sh
levier runtime start
```

> [!OUTPUT]
> ```text
> runtime: counter-runtime
> file:    /path/to/runtime.py
> server:  127.0.0.1:9090
> ```

Runtime 里未捕获的异常会自动变成：

```text
ERROR 事件
Run failed
```

需要显式错误时：

```python
await ctx.error("MY_ERROR", "something failed", retryable=False)
```

工具错误已经在 `ToolResult` 里：

```python
result = await ctx.call_tool("search", {"query": "levier"})
if not result.ok:
    await ctx.error(result.error_code, result.error_message)
```

细节：[Runtime 核心流程](runtime-core.md) · [runtime init](runtime-start.md) · [runtime start](runtime-start.md) · [错误与取消](runtime-errors.md)

更多 Runtime 能力：[内置 Runtime](runtime-builtin.md) · [Context](runtime-context.md) · [Toolbox](runtime-toolbox.md) · [chat 与 invoke](runtime-chat.md) · [checkpoint](runtime-errors.md) · [实例与容量](runtime-remote.md) · [远程部署与 YAML](runtime-remote.md)

## 5. 观测

```sh
levier status
```

```sh
levier run show <run_id> --events
```

```sh
levier runtime list
```

```sh
levier logs --follow
```

细节：[状态](levier-status.md) · [查看与取消 Run](levier-run.md) · [Runtime 列表](levier-runtime-list.md) · [日志与进程](levier-status.md)

## 6. 更多

App 管理：

```text
app list / app show / app rotate-key
get run / wait / get events / cancel run
```

细节：[App 管理](app-manage.md) · [Run 与事件](app-runs.md)

Levier 运维：

```text
doctor / describe / selfcheck
config / migrate / maintenance
ops runs / ops daemons / ops tasks
```

细节：[doctor / describe / selfcheck](levier-doctor.md) · [config / migrate / maintenance](levier-doctor.md) · [ops](levier-ops.md) · [远程控制面](levier-remote.md)

## 7. 停止

```sh
levier down
```

细节：[停止](levier-status.md)
