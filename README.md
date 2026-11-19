# Levier

Levier 把一个 App 的业务能力和一个 Runtime 的执行能力连接起来。

- **App**：持有业务状态，注册本地工具，接收工具调用。
- **Levier**：控制面，负责鉴权、调度、Run 状态、事件、MCP 目录和模型服务商目录。
- **Runtime**：执行 `run(ctx)`，决定是否调用模型、是否调用工具、何时结束。
- **MCP Runtime**：独立进程，连接外部 MCP Server，供 Runtime 共享。

真实接入代码见：

- App 示例：[`examples/counter_agent`](examples/counter_agent)
- Runtime 最小示例：[`execution/echo_runtime`](execution/echo_runtime)
- Runtime 可选 ReAct helper：[`execution/runtime_sdk/src/levier_runtime/react`](execution/runtime_sdk/src/levier_runtime/react)

---

## 1. 五分钟跑通

### 1.1 启动本地栈

```sh
cp .env.example .env
docker compose up --build -d postgres levier mcp-runtime react-runtime echo-runtime
bin/levier status
```

默认地址：

```text
HTTP   http://127.0.0.1:8080
gRPC   127.0.0.1:9090
```

### 1.2 注册模型服务商

控制面不保存模型 key，只保存 **provider 元数据和 secret ref**。key 由控制面从
环境变量或 0600 secret 文件解析。

开发环境可以直接放到 `.env`：

```sh
LEVIER_SECRET_DEEPSEEK_API_KEY=sk-...
```

然后注册 provider：

```sh
export LEVIER_ADMIN_TOKEN=admin-change-me

levier provider add \
  --name deepseek \
  --adapter openai_compatible \
  --base-url https://api.deepseek.com \
  --default-model deepseek-flash \
  --models deepseek-flash,deepseek-reasoner \
  --secret-ref deepseek_api_key \
  --default
```

检查：

```sh
levier provider list
levier provider check --name deepseek
```

生产环境推荐使用：

```sh
export LEVIER_SECRETS_FILE=/run/secrets/levier_model_secrets.json
```

```json
{
  "deepseek_api_key": "sk-...",
  "openai_api_key": "sk-...",
  "anthropic_api_key": "sk-..."
}
```

### 1.3 注册 App

```sh
levier app init \
  --name counter-agent \
  --server http://127.0.0.1:8080 \
  --credentials-out ./data/agent/credentials.json
```

需要 MCP 时：

```sh
levier app init \
  --name counter-agent \
  --mcp-ref e2e-echo \
  --credentials-out ./data/agent/credentials.json
```

### 1.4 启动 App

```sh
LEVIER_CREDENTIALS=./data/agent/credentials.json \
  python3 examples/counter_agent/app.py
```

### 1.5 提交一次 Run

```sh
levier run submit \
  --credentials ./data/agent/credentials.json \
  --runtime react1 \
  --prompt "Add 2, then add 3, then read the counter." \
  --watch
```

没传 `--provider` / `--model` 时，控制面使用默认 provider 和它的默认模型。

---

## 2. App 侧接入

### 2.1 安装 SDK

```sh
python3 -m venv .venv
.venv/Scripts/python -m pip install -e ./sdk/python
```

Linux/macOS 使用 `.venv/bin/python`。

### 2.2 注册工具

```python
from levier_app import LevierApp

counter = {"value": 0}


def register_tools(app):
    @app.tool(
        "counter_add",
        description="Add an integer to the counter.",
        schema={
            "type": "object",
            "properties": {"delta": {"type": "integer"}},
            "required": ["delta"],
            "additionalProperties": False,
        },
        timeout_ms=10000,
    )
    def counter_add(arguments):
        counter["value"] += arguments["delta"]
        return {"value": counter["value"]}

    @app.tool("counter_get", schema={"type": "object", "properties": {}})
    def counter_get(_arguments):
        return {"value": counter["value"]}


if __name__ == "__main__":
    raise SystemExit(LevierApp.run_cli(register_tools))
```

SDK 负责：

- 读取 `LEVIER_CREDENTIALS` 凭据文件
- 注册本地工具 manifest
- 建立并重连 WebSocket
- 接收 `local_tool_call`
- 调用 handler
- 返回 `local_tool_result`
- 工具异常和超时映射

### 2.3 MCP 引用

App 不实现 MCP client，只声明引用：

```python
app = LevierApp.from_credentials(mcp_refs=["github", "e2e-echo"])
```

或：

```sh
export LEVIER_APP_MCP_REFS=github,e2e-echo
```

`mcp_ref` 必须在 Levier 中已经注册。注册 MCP server：

```sh
levier mcp add \
  --name e2e-echo \
  --transport stdio \
  --config-file ./mcp-echo.json \
  --allowed-tools echo_token
```

`mcp-echo.json` 是 MCP Server 自身配置，例如：

```json
{
  "command": "python",
  "args": ["path/to/your_mcp_server.py"],
  "env": {
    "TOKEN": "${secret:e2e_token}"
  }
}
```

MCP 工具在模型侧的名字是：

```text
mcp__<server>__<tool>
```

### 2.4 App 提交 Run

同一个凭据文件也可以作为 App 的客户端使用：

```python
from levier_app import LevierClient

client = LevierClient.from_credentials()

providers = client.list_model_providers()

run = client.submit_run(
    "Add 2, then add 3, then read the counter.",
    runtime="react1",
    provider="deepseek",
    model="deepseek-flash",
)
result = client.wait_run(run["run_id"], timeout=120)
events = client.get_events(run["run_id"])
```

`provider` 和 `model` 都可以省略，此时使用控制面的默认 provider。

### 2.5 App SDK 公开面

```text
LevierApp.from_credentials(mcp_refs=[...])
LevierApp.tool(...)
LevierApp.run()
LevierApp.client
LevierApp.mcp_refs

LevierClient.from_credentials()
LevierClient.list_model_providers()
LevierClient.submit_run(...)
LevierClient.wait_run(...)
LevierClient.get_run(...)
LevierClient.get_events(...)
LevierClient.cancel_run(...)
```

App API key 只用于 Levier 鉴权，不等于模型服务商 key。

---

## 3. Runtime 侧接入

### 3.1 使用内置 Echo

```sh
PYTHONPATH=execution/runtime_sdk/src:execution/echo_runtime/src \
  python -m levier_echo_runtime \
  --config execution/echo_runtime/runtime.yaml
```

### 3.2 使用内置 React

```sh
PYTHONPATH=execution/agent_core/src:execution/runtime_sdk/src:execution/react_runtime/src \
  python -m levier_react_runtime \
  --config execution/react_runtime/react.example.yaml
```

React 启动前先通过控制面注册模型 provider；Runtime 会从 `TaskAssign` 收到
本次 Run 解析后的模型地址和 key，不需要再把模型 key 写进 Runtime 环境。

### 3.3 写自己的 Runtime

```python
from levier_runtime import RunContext, Runtime


async def run(ctx: RunContext) -> str:
    await ctx.reply("working")

    result = await ctx.call_tool("search", {"query": "levier"})
    if not result.ok:
        await ctx.error(result.error_code, result.error_message, retryable=True)
        return "failed"

    await ctx.reply(result.result_json)
    return "completed"


if __name__ == "__main__":
    Runtime.from_config("runtime.yaml").run(run)
```

SDK 自动处理：

- RuntimeChannel、Hello、心跳、重连
- Ack、取消、lease_epoch
- event seq 和 terminal 状态
- `ctx.call_tool` 的 `TOOL_CALL_BEGIN` / `TOOL_CALL_END`
- `ctx.error`
- 未捕获异常到 `ERROR` + failed
- `ctx.logger` 自动带 run 上下文

自定义 Runtime 可以完全不使用 React、LLM 或 ToolRouter。

### 3.4 想复用 ReAct

```python
from levier_runtime.llm import ProviderRouter
from levier_runtime.react import ReactAgent
from levier_runtime.tools import ToolRouter

agent = ReactAgent(ProviderRouter(), ToolRouter(mcp=mcp_client))
await runtime.serve(agent)
```

ReAct、provider、tool router、checkpoint 都是可选 helper，不是 SDK 核心。

### 3.5 Runtime CLI

```text
levier-echo-runtime
  --config PATH
  --check
  --print-config
  --version
  --log-level LEVEL

levier-react-runtime
  --config PATH
  --check
  --print-config
  --version
  --log-level LEVEL
  --mcp-socket PATH
```

`--print-config` 会隐藏 auth token。

---

## 4. 工具、MCP 与 Plan & Execute

一次典型的 Plan & Execute：

```text
用户：Add 2, then add 3, then read the counter.
  -> React Runtime 生成计划
  -> 调用 counter_add({"delta": 2})
  -> 调用 counter_add({"delta": 3})
  -> 调用 counter_get({})
  -> 返回最终值
```

本地工具路径：

```text
Runtime -> ToolCallRequest -> Levier -> App WebSocket -> App handler
        -> local_tool_result -> Levier -> Runtime
```

MCP 工具路径：

```text
Runtime -> mcp_runtime -> MCP Server
```

App 只需要：

1. 在 App 里注册本地工具；
2. 通过 `mcp_refs` 声明要使用的 MCP Server；
3. 保持 WebSocket 在线。

Runtime 只需要：

1. 读取 `ctx.tools` / `ctx.mcp_bindings`；
2. 选择模型和工具；
3. 通过 `ctx.call_tool` 或 `ToolRouter` 调用；
4. 用 `ctx.reply` 返回最终文本。

---

## 5. CLI 操作面

### 5.1 日常使用

```text
levier version
levier serve
levier status
levier doctor
levier app init|show|check|list|rotate-key
levier provider list|show|add|update|remove|check
levier mcp list|add|remove
levier run submit|watch|show|cancel
levier runtime list
```

示例：

```sh
levier status --json
levier doctor --credentials ./data/agent/credentials.json
levier runtime list
levier run show --run-id <id> --events
```

### 5.2 运维/内部

```text
levier admin ops runs|daemons|tasks
levier admin migrate status|up
levier admin maintenance status|prune
levier admin config check|print
levier admin daemon check
levier admin describe
levier admin selfcheck [--deep]
```

这些命令保留兼容入口，但不再出现在主帮助里。

---

## 6. 模型服务商

### 6.1 Provider 记录

```text
name             逻辑名称
adapter          openai_compatible / anthropic
base_url         服务地址
default_model    默认模型
models           允许的模型列表
secret_ref       密钥引用，不是密钥本身
enabled          是否可用
default          是否默认 provider
```

Provider 管理：

```sh
levier provider add \
  --name deepseek \
  --adapter openai_compatible \
  --base-url https://api.deepseek.com \
  --default-model deepseek-flash \
  --models deepseek-flash \
  --secret-ref deepseek_api_key \
  --default

levier provider list
levier provider check --name deepseek
```

### 6.2 Secret

控制面解析顺序：

```text
LEVIER_SECRET_<REF>
LEVIER_SECRETS_FILE JSON
```

示例：

```text
secret_ref=deepseek_api_key
-> LEVIER_SECRET_DEEPSEEK_API_KEY
```

生产 secret 文件必须是 0600；模型 key 不会通过 `/meta`、provider list、
CLI 输出、Run event 或 checkpoint 暴露。

### 6.3 选择模型

```json
{
  "runtime_name": "react1",
  "input": [{"role": "user", "content": "hello"}],
  "model": {
    "provider": "deepseek",
    "model": "deepseek-flash"
  }
}
```

省略 `model` 时使用默认 provider；省略 `model.model` 时使用 provider default。

---

## 7. 配置

Runtime YAML：

```yaml
middleware:
  address: "127.0.0.1:9090"
  auth_token: "change-me"

mcp_runtime:
  socket: "/tmp/levier-mcp.sock"

runtime:
  name: "react1"
  instance_id: "daemon-local-1"
  type: "normal"
  max_concurrent_runs: 4

logging:
  level: "info"
```

App 凭据：

```text
LEVIER_CREDENTIALS  App 凭据文件路径
LEVIER_APP_MCP_REFS 逗号分隔的 MCP 引用
```

控制面模型密钥：

```text
LEVIER_SECRETS_FILE                 0600 JSON secret 文件
LEVIER_SECRET_<UPPER_REF>           单密钥环境变量
```

---

## 8. 状态与错误

Run 终态：

```text
completed / failed / cancelled / timeout
```

常见错误：

| code | 含义 |
|---|---|
| `BAD_REQUEST` | 请求格式或参数错误 |
| `UNAUTHORIZED` | App key 或 Admin token 错误 |
| `MODEL_PROVIDER_NOT_FOUND` | provider 未注册 |
| `BAD_MODEL_PROVIDER` | provider 配置或模型不允许 |
| `TOOL_NOT_FOUND` | App 未注册该工具 |
| `TOOL_TIMEOUT` | 工具执行超时 |
| `TOOL_EXEC_ERROR` | 工具 handler 失败 |
| `MCP_CONNECT_FAILED` | MCP Server 连接失败 |
| `MCP_RUNTIME_UNAVAILABLE` | MCP Runtime 不可用 |
| `SECRET_MISSING` | 引用的 secret 未配置 |

App 侧应优先读取结构化的 `error.code`，不要解析中文错误文本。

---

## 9. 目录

```text
cmd/levier                 控制面 CLI / server
internal/                  控制面实现
sdk/python/                App SDK
examples/counter_agent/    App 示例
execution/runtime_sdk/     Runtime SDK + 可选 helper
execution/echo_runtime/    最小 Runtime
execution/react_runtime/   内置 ReAct Runtime
execution/mcp_runtime/     独立 MCP Runtime
execution/agent_core/      可选 ReAct 循环库
```

更多：

- App SDK：`sdk/python/README.md`
- Runtime SDK：`execution/runtime_sdk/README.md`
- Runtime 接入：`docs/RUNTIME_ONBOARDING.md`
- MCP：`docs/MCP.md`
- CLI：`docs/CLI_REFERENCE.md`
