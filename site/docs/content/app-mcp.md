---
title: MCP 引用与重连
status: published
description: 声明 MCP 引用，并处理断线与重连。
updated: 2026-11-21
---

## MCP 引用

### 1. 声明方式

初始化项目时写入：

```sh
levier init --mcp-ref echo
```

多个引用：

```sh
levier init --mcp-ref echo,github
```

在代码里：

```python
app = App(tools=[counter_add], mcp=["echo", "github"])
```

或环境变量：

```sh
export LEVIER_APP_MCP_REFS=echo,github
```

### 2. 谁负责连接

```text
App        只声明引用
Levier     保存 MCP Server 与 allowed_tools
MCP Runtime 连接 MCP Server、解析 secret
Runtime    通过 ctx.toolbox() 使用工具
```

App 代码不实现 MCP client，也不保存 MCP Server 的 key。

### 3. 引用必须已注册

```sh
levier mcp add \
  --name echo \
  --transport stdio \
  --config-file ./examples/mcp_echo/config.json
```

未注册的引用会让 App 初始化或连接失败。

### 4. 工具命名

模型侧：

```text
mcp__<server>__<tool>
```

例如：

```text
mcp__echo__echo_token
```

名字由 MCP Runtime 生成；App 和 Runtime 不需要手动拼接。

### 5. 排查

| 现象 | 先检查 |
|---|---|
| App 连接失败 | `mcp_ref` 是否已注册 |
| 模型看不到工具 | `allowed_tools` 是否包含该工具 |
| `SECRET_MISSING` | `${secret:name}` 是否有值 |
| `TOOL_DENIED` | 工具不在白名单 |
| `MCP_RUNTIME_UNAVAILABLE` | 共享 MCP Runtime 是否启动 |

## 错误与重连

### 1. ToolExecutionError

handler 可以主动抛结构化错误：

```python
from levier import ToolExecutionError


@tool
def my_tool(value: str) -> dict:
    if not value:
        raise ToolExecutionError("BAD_INPUT", "value is required")
    return {"ok": True}
```

SDK 会把它变成 `TOOL_CALL_END` 里的错误结果，不会让 WebSocket 崩溃。

### 2. 常见错误码

| code | 含义 | 先做什么 |
|---|---|---|
| `UNAUTHORIZED` | App key 或 admin token 错误 | 重新 `levier init` 或检查 token |
| `TOOL_NOT_FOUND` | App 没注册该工具 | 确认工具名与进程在线 |
| `TOOL_TIMEOUT` | 工具执行超时 | 看 handler 日志，调大 timeout |
| `TOOL_EXEC_ERROR` | handler 抛异常 | 看 App 进程日志 |
| `APP_UNREACHABLE` | App 断线，pending 工具失败 | 重启 App，重新提交 |
| `MODEL_PROVIDER_NOT_FOUND` | provider 未注册 | `levier model connect` |
| `SECRET_MISSING` | 模型 secret 未配置 | 重新 connect 或检查 secret 文件 |

### 3. 断线与重连

App 断线时：

```text
Run 不会暂停
pending local tool 会以 APP_UNREACHABLE 失败
App 重连后可以继续接收新的工具调用
```

SDK 负责 WebSocket 重连；业务侧只需要保证进程不退出。

`levier dev` 必须保持运行。一次性场景用：

```sh
levier run "..." --with-app
```

### 4. 事件定位

```sh
levier run show <run_id> --events
```

重点看：

```text
TOOL_CALL_BEGIN
TOOL_CALL_END
ERROR
```

### 5. SDK 异常类型

```text
ToolExecutionError   工具业务错误
AppInactiveError     App 不可用
HTTPError            HTTP/API 错误
ProtocolError        协议错误
TransportError       连接/传输错误
```

优先读取结构化 code，不要解析展示文本。

## 下一步

- MCP 配置：[MCP 配置](pre-mcp.md)
- App 核心流程：[App 核心流程](app-core.md)
- App 管理：[App 管理](app-manage.md)
