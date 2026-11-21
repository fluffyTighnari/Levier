---
title: App SDK
status: published
description: 保留已有业务函数，用一层工具声明把它交给模型调用；凭据、重连与调用帧由 SDK 负责。
updated: 2026-11-20
---

> [!NOTE]
> 读完之后：你能把一个已有函数注册成 local tool，启动 App 进程，
> 并在 `run show --events` 里看到 `TOOL_CALL_BEGIN` / `TOOL_CALL_END`。

## 1. 最小改动：已有函数变成工具

假设你的项目里已经有这个函数，它不需要改：

```python
# myproject/orders.py
def mark_shipped(order_id: str) -> dict:
    order = repo.get(order_id)
    order.status = "shipped"
    repo.save(order)
    return {"ok": True, "carrier": order.carrier}
```

新增一个适配文件，把参数转成业务函数需要的形状：

```python
# agent_bridge.py
from levier_app import LevierApp

from myproject.orders import mark_shipped


def register_tools(app):
    @app.tool(
        "orders_mark_shipped",
        description="把订单标记为已发货",
        schema={
            "type": "object",
            "properties": {"id": {"type": "string"}},
            "required": ["id"],
            "additionalProperties": False,
        },
        timeout_ms=10000,
    )
    def handler(arguments):
        return mark_shipped(arguments["id"])


if __name__ == "__main__":
    raise SystemExit(LevierApp.run_cli(register_tools))
```

工具名建议带命名空间（`orders_mark_shipped`），模型更容易区分。
多数模型服务商只接受字母、数字与下划线，所以不要用点或空格。
`schema` 是给模型看的参数约束，`timeout_ms` 是这个工具的调用上限。

> [!TIP]
> handler 只做参数转换和结果整理。业务逻辑留在原函数里，测试也仍然只测原函数。

## 2. 安装

```sh
python3 -m venv .venv
.venv/Scripts/python -m pip install -e ./sdk/python
```

Linux / macOS 使用 `.venv/bin/python`。

## 3. 启动 App 进程

```sh
.venv/Scripts/python agent_bridge.py --credentials ./my-first-app.json
```

PowerShell：

```powershell
.venv\Scripts\python .\agent_bridge.py --credentials .\my-first-app.json
```

Linux / macOS 用 `.venv/bin/python`。进程会建立一条 WebSocket，注册工具 manifest，
然后等待 `local_tool_call`。断线会自动重连；Ctrl+C 会正常退出。

> [!OUTPUT]
> 启动后，控制面的 `./bin/levier status` 里应该能看到 `apps ok 1 websocket`。

## 4. 凭据

```text
LEVIER_CREDENTIALS      App 凭据文件路径
LEVIER_APP_MCP_REFS     逗号分隔的 MCP 引用
```

App API key 只用于 Levier 鉴权，**不等于模型服务商 key**。
模型 key 由控制面解析，只随 TaskAssign 下发给 Runtime。

## 5. 声明 MCP 引用

App 不实现 MCP client，只声明引用：

```python
app = LevierApp.from_credentials(mcp_refs=["github", "e2e-echo"])
```

或：

```sh
export LEVIER_APP_MCP_REFS=github,e2e-echo
```

`mcp_ref` 必须已经在 Levier 中注册。详见 [MCP](mcp.md)。

## 6. 从 App 提交 Run

同一个凭据文件也可以作为客户端使用：

```python
from levier_app import LevierApp

app = LevierApp.from_credentials()

run = app.submit_run(
    "把订单 A-1042 标记为已发货",
    runtime="react1",
    provider="deepseek",
    model="deepseek-flash",
)
result = app.wait_run(run["run_id"], timeout=120)
events = app.get_events(run["run_id"])
```

`provider` 与 `model` 都可以省略，此时使用控制面的默认 provider 与默认模型。

## 7. 公开面

```text
LevierApp.from_credentials(mcp_refs=[...])
LevierApp.tool(...)
LevierApp.run_cli(...)
LevierApp.run()
LevierApp.mcp_refs

LevierApp.list_model_providers()
LevierApp.submit_run(...)
LevierApp.wait_run(...)
LevierApp.get_run(...)
LevierApp.get_events(...)
LevierApp.cancel_run(...)
```

## 8. 错误处理

优先读取结构化的 `error.code`，不要解析错误文本。

| code | 含义 | 先做什么 |
|---|---|---|
| `UNAUTHORIZED` | App key 或 Admin token 错误 | 重新 `app init` 或检查 token |
| `TOOL_NOT_FOUND` | App 未注册该工具 | 确认工具名与进程已连接 |
| `TOOL_TIMEOUT` | 工具执行超时 | 看 handler 日志，必要时调大 `timeout_ms` |
| `TOOL_EXEC_ERROR` | handler 抛异常 | 看 App 进程日志 |
| `MODEL_PROVIDER_NOT_FOUND` | provider 未注册 | 执行 `provider add` |
| `SECRET_MISSING` | 引用的 secret 未配置 | 检查 `LEVIER_SECRET_*` 或 secret 文件 |

App 断线时 Run 不会暂停；pending local tool 会以 `APP_UNREACHABLE` 失败。
App 重连后可以重新订阅或补拉事件。

## 9. 下一步

- 看一次真实工具调用的事件序列：[快速开始](quickstart.md)。
- 需要外部工具：[MCP](mcp.md)。
- 写自己的 Runtime：[Runtime SDK](runtime-sdk.md)。
