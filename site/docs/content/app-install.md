---
title: App SDK、工具与构造
status: published
description: 安装 App SDK、定义工具并构造 App。
updated: 2026-11-21
---

## 安装 SDK

### 1. PyPI

```sh
pip install levier
```

App-only 安装不会立即导入 Runtime SDK 的 gRPC 依赖；`Context` / `Runtime`
是懒加载的。

### 2. 源码

```sh
cd /path/to/middleware
python -m pip install -e ./sdk/python -e ./execution/runtime_sdk -e ./sdk/levier
```

只做 App、不写 Runtime 时，可以只装 App 侧：

```sh
python -m pip install -e ./sdk/python -e ./sdk/levier
```

### 3. 版本

```sh
python -c "import levier; print(levier.__version__)"
```

建议在项目里固定版本：

```text
levier==0.1.0
```

### 4. 验证

```python
from levier import App, tool

@tool
def ping() -> dict:
    """Return pong."""
    return {"pong": True}

print(App(tools=[ping]).tools_manifest())
```

如果这里报 `No module named levier`，说明当前 Python 不是安装 SDK 的那个。

## @tool 与 schema

### 1. @tool

```python
from levier import tool


@tool
def counter_add(delta: int) -> dict:
    """Add delta to the counter."""
    return {"value": delta}
```

`@tool` 把普通函数转成 `Tool`：

```text
name         函数名，可用参数覆盖
description  docstring 第一行
schema      从函数签名生成
timeout_ms   默认 10000
```

### 2. 类型注解

支持的基础类型：

```text
str / int / float / bool
list / dict
Optional[...]
```

复杂对象建议用 `dict` 或 `list[dict]`，不要依赖自定义类的自动序列化。

### 3. docstring

第一行就是模型看到的工具说明：

```python
@tool
def counter_add(delta: int) -> dict:
    """Add an integer delta to the counter."""
```

保持一句话、动词开头，写清楚参数含义。

### 4. 自动 schema

生成结果类似：

```json
{
  "type": "object",
  "properties": {
    "delta": {"type": "integer"}
  },
  "required": ["delta"],
  "additionalProperties": false
}
```

也可以显式覆盖：

```python
@tool(
    name="counter_add",
    description="Add delta to the counter.",
    schema={
        "type": "object",
        "properties": {"delta": {"type": "integer"}},
        "required": ["delta"],
        "additionalProperties": False,
    },
    timeout_ms=10000,
)
def counter_add(delta: int) -> dict:
    ...
```

### 5. timeout

```python
@tool(timeout_ms=30000)
def slow_tool(...):
    ...
```

超时由控制面/网关映射成 `TOOL_TIMEOUT`。不要用无限等待。

### 6. 工具名

建议只用字母、数字与下划线：

```text
counter_add
orders_mark_shipped
```

多数模型服务商不接受点、空格或斜杠。命名空间用下划线，不要用点。

### 7. 测试

```python
tool = counter_add
assert tool.invoke({"delta": 2}) == {"value": 2}
assert tool(delta=2) == {"value": 2}
```

`schema_from_function` 可以单独检查生成结果。

## App 构造

### 1. 构造参数

```python
App(
    tools=(),
    *,
    mcp=None,
    runtime=None,
    credentials=None,
    server=None,
    logger=None,
    on_event=None,
)
```

| 参数 | 说明 |
|---|---|
| `tools` | `Tool` 或普通函数列表 |
| `mcp` | 字符串或字符串列表，MCP 引用 |
| `runtime` | 默认 runtime_name |
| `credentials` | 显式凭据路径 |
| `server` | 显式控制面地址 |
| `logger` | 自定义 logger |
| `on_event` | 事件回调 |

不传 `credentials` / `server` 时，从项目 `.levier.json` 和用户 profile 读取。

### 2. 默认配置

```python
app = App(tools=[counter_add])
```

等价于：

```text
tools        counter_add
mcp          profile / .levier.json 中的 mcp_refs
runtime      profile / .levier.json 中的 runtime
credentials  profile / .levier.json 中的 credentials
server       profile / .levier.json 中的 server
```

### 3. App.run

```python
# 服务工具调用，阻塞
app.run()

# 一次性提交并打印最终消息
app.run("Add 2 and 3.")

# 指定 runtime
app.run("hello", runtime="echo1")

# 不打印最终消息
app.run("hello", print_output=False)
```

签名：

```text
App.run(prompt=None, *, runtime=None, provider="", model="",
        max_steps=8, run_ms=60000, tool_ms=10000,
        timeout=300.0, print_output=True)
```

### 4. App.serve

```python
app.serve()
```

连接 Levier 并阻塞服务工具调用。`app.run()` 不带 prompt 时等价于 `serve()`。

停止：

```python
app.stop()
```

### 5. 动态注册

```python
app = App()

@app.tool
def ping() -> dict:
    """Return pong."""
    return {"pong": True}

app.run()
```

也支持：

```python
@app.tool("ping")
def ping_tool() -> dict:
    ...

@app.tool(name="ping", timeout_ms=5000)
def ping_tool() -> dict:
    ...
```

### 6. 查询

```text
app.tools_manifest()
app.list_model_providers()
app.submit_run(...)
app.wait_run(...)
app.get_run(...)
app.get_events(...)
app.cancel_run(...)
```

详见 [SDK 提交 Run](app-runs.md)。

## 下一步

- App 核心流程：[App 核心流程](app-core.md)
- 初始化与运行：[初始化与运行 App](app-dev.md)
- Run 与事件：[Run 与事件](app-runs.md)
