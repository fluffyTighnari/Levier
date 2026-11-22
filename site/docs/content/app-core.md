---
title: App 核心流程
status: published
description: 安装、工具、init、dev、run 与错误处理的最小闭环。
updated: 2026-11-21
---

> [!NOTE]
> 这是 App 端的主干。参数细节、SDK 方法和 App 管理命令分别在后续页面展开。

## 1. 安装 SDK

```sh
pip install levier
```

从源码：

```sh
python -m pip install -e ./sdk/python -e ./execution/runtime_sdk -e ./sdk/levier
```

## 2. 工具写法

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

`@tool` 从类型注解生成 JSON Schema，从 docstring 第一行生成 description。
不需要手写 schema。

## 3. 项目初始化

```sh
levier init
```

> [!OUTPUT]
> ```text
> ready: counter-agent
> app:     app_01...
> server:  http://127.0.0.1:8080
> runtime: react1
> next:
>   levier dev            # serve this project's tools
>   levier run "hello"    # submit a run without flags
> ```

`init` 会写用户 profile、项目 `.levier.json` 和默认 0600 凭据文件。

## 4. 启动 App

```sh
levier dev
```

它查找 `./app.py` / `./levier_app.py`，注入凭据、server 和 MCP 引用。
进程必须保持在线，工具 handler 在 App 进程里执行。

也可以：

```sh
python app.py
```

效果相同。

## 5. 提交 Run

另一个终端：

```sh
levier run "Add 2, then add 3, then read the counter."
```

一条命令临时启动 App：

```sh
levier run "Add 2, then add 3, then read the counter." --with-app
```

SDK 一次性提交：

```python
app.run("Add 2, then add 3, then read the counter.")
```

> [!OUTPUT]
> ```text
> run submitted: 01J...
> status: queued
> status: completed
> runtime_name: react1
> final: 5
> ```

## 6. 错误处理

handler 可以直接抛结构化错误：

```python
from levier import ToolExecutionError


@tool
def my_tool(value: str) -> dict:
    if not value:
        raise ToolExecutionError("BAD_INPUT", "value is required")
    return {"ok": True}
```

SDK 会把它变成工具结果，不会让 WebSocket 崩溃。

工具故障先看：

```sh
levier run show <run_id> --events
```

重点事件：

```text
TOOL_CALL_BEGIN
TOOL_CALL_END
ERROR
```

## 下一步

- 安装与版本：[安装 SDK](app-install.md)
- 工具与 schema：[@tool 与自动 schema](app-install.md)
- 构造参数：[App 构造](app-install.md)
- 初始化：[项目初始化](app-dev.md)
- 启动：[dev 与 serve](app-dev.md)
- 提交 Run：[CLI 提交 Run](app-runs.md) · [SDK 提交 Run](app-runs.md)
- 错误与重连：[错误与重连](app-mcp.md)
