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

完整一页版见 [`docs/QUICKSTART.md`](docs/QUICKSTART.md)。黄金路径只有五步。

### 1.1 安装

发布版：

```sh
curl -fsSL https://raw.githubusercontent.com/fluffyTighnari/levier/main/scripts/install.sh | sh
```

从源码开发：

```sh
make install
levier setup --server http://127.0.0.1:8080
```

安装器负责 PATH、Compose 源码包和 Python SDK（`from levier import App, tool, Runtime`）。之后可以在任意业务目录使用 `levier`。

### 1.2 启动

```sh
levier up
levier status
```

`levier up` 启动本地控制面、PostgreSQL、MCP、React 和 Echo Runtime，等待就绪，
并把 server、默认 runtime 和本地 runtime token 写进用户配置。

### 1.3 配置模型

用户只需要 URL、key 和 model：

```sh
levier model connect \
  --url https://api.deepseek.com \
  --key sk-... \
  --model deepseek-chat
```

provider 名称、adapter、secret ref 和 secret 存储都由命令推断和完成。不需要设置
`LEVIER_SECRET_*`，也不需要理解 `--secret-ref`。

### 1.4 接入 App

```sh
pip install levier
```

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

app = App(tools=[counter_add, counter_get], mcp=[])

if __name__ == "__main__":
    app.run()
```

```sh
levier init     # 当前目录成为 App，默认凭据路径和 runtime 自动写入用户配置
levier dev      # 启动 ./app.py，自动注入凭据、server 和 MCP 引用
```

不需要 `--credentials-out`、`--credentials`、`--admin-token`，也不需要手写 JSON Schema。

### 1.5 提交并观测 Run

```sh
levier run "Add 2, then add 3, then read the counter."
```

默认使用当前 App、默认 runtime、默认 provider，并观测到终态。高级场景仍可使用
`levier run submit|watch|show|cancel`。

### 1.6 接入自己的 Runtime

```sh
levier runtime init
levier runtime start
```

```python
from levier import Context, Runtime

async def run(ctx: Context) -> str:
    tools = await ctx.toolbox()                  # 本地 + MCP，一次取齐
    reply = await ctx.chat(ctx.input, tools=tools)  # 模型已在 ctx.model
    await ctx.invoke_all(reply, toolbox=tools)   # local/MCP 自动分流
    await ctx.reply(reply.content or "done")
    return "completed"

if __name__ == "__main__":
    Runtime.start(run)
```

不需要 `runtime.yaml`、`runtime_token`、daemon id 或 gRPC 细节。

---

## 2. App 侧接入（低层参考）

> 首选接入面见 1.4 和 [`docs/QUICKSTART.md`](docs/QUICKSTART.md)。本节保留
> `levier_app` 低层适配器的完整说明，便于已有集成和排查。

### 2.1 安装 SDK

首选 `pip install levier`（见 1.4）。下面是从仓库安装低层 `levier_app` 的方式。

从仓库开发：

```sh
python3 -m venv .venv
.venv/Scripts/python -m pip install -e ./sdk/python
```

Linux/macOS 使用 `.venv/bin/python`。

使用 release wheel 时，在任意目录执行：

```sh
python -m venv .venv
.venv/bin/python -m pip install levier_app-0.1.0-py3-none-any.whl
```

App SDK 会读取 `levier setup` 写入的用户级配置，因此业务项目不需要重复保存 server 地址。

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
from levier_app import LevierApp

app = LevierApp.from_credentials()

providers = app.list_model_providers()

run = app.submit_run(
    "Add 2, then add 3, then read the counter.",
    runtime="react1",
    provider="deepseek",
    model="deepseek-flash",
)
result = app.wait_run(run["run_id"], timeout=120)
events = app.get_events(run["run_id"])
```

`provider` 和 `model` 都可以省略，此时使用控制面的默认 provider。

### 2.5 App SDK 公开面

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

App API key 只用于 Levier 鉴权，不等于模型服务商 key。

---

## 3. Runtime 侧接入（低层参考）

> 首选 Runtime 面见 1.6 和 [`docs/QUICKSTART.md`](docs/QUICKSTART.md)。本节保留
> runtime SDK、内置 Runtime 和自定义 Runtime 的完整说明。

### 3.1 使用内置 Echo

从 release wheel 安装后，在任意目录执行：

```sh
python -m pip install --find-links ./levier-wheels levier-echo-runtime
python -m levier_echo_runtime --config /path/to/runtime.yaml
```

从仓库开发时：

```sh
PYTHONPATH=execution/runtime_sdk/src:execution/echo_runtime/src \
  python -m levier_echo_runtime \
  --config execution/echo_runtime/runtime.yaml
```

### 3.2 使用内置 React

```sh
python -m pip install --find-links ./levier-wheels levier-react-runtime
python -m levier_react_runtime --config /path/to/react.example.yaml
```

从仓库开发时：

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
    Runtime.start(run)
```

需要显式 YAML 时再用 `Runtime.from_config("runtime.yaml").run(run)`；黄金路径不需要。

SDK 自动处理：

- RuntimeChannel、Hello、心跳、重连
- Ack、取消、lease_epoch
- event seq 和 terminal 状态
- `ctx.call_tool` 的 `TOOL_CALL_BEGIN` / `TOOL_CALL_END`
- `ctx.error`
- 未捕获异常到 `ERROR` + failed
- `ctx.logger` 自动带 run 上下文

自定义 Runtime 可以完全不使用 React、LLM，工具可以直接走 `ctx.toolbox()`。

### 3.4 想复用 ReAct

```python
from levier_runtime.llm import ProviderRouter
from levier_runtime.react import ReactAgent

agent = ReactAgent(ProviderRouter())
await runtime.serve(agent)
```

ReAct、provider、checkpoint 都是可选 helper，不是 SDK 核心。MCP 工具由
`ctx.toolbox()` 自动接入，不需要单独的 MCP client 或工具路由层。

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

1. `tools = await ctx.toolbox()` 一次取齐本地和 MCP 工具；
2. `reply = await ctx.chat(ctx.input, tools=tools)` 调用已解析的模型；
3. `await ctx.invoke_all(reply, toolbox=tools)` 自动分流 local/MCP；
4. `await ctx.reply(...)` 返回最终文本。

低层写法（`ctx.tools`、`ctx.mcp_bindings`、`ctx.call_tool`）仍然可用。

---

## 5. CLI 操作面

### 5.1 黄金路径

```text
levier up
levier init [name]
levier model connect --url URL --key KEY --model MODEL
levier dev
levier run "prompt"
levier runtime init
levier runtime start
levier status
levier logs
levier down
```

示例：

```sh
levier status --json
levier run "hello" --json
levier run show <run_id> --events
levier runtime start --name my-runtime
```

### 5.2 高级 / 兼容

```text
levier setup
levier version
levier serve
levier doctor
levier app init|show|check|list|rotate-key   # 内部/自动化
levier mcp list|add|remove
levier run submit|watch|show|cancel
levier runtime list
levier stack up|down|ps|logs
levier admin ops|migrate|maintenance|config|daemon|describe|selfcheck
```

这些命令保留兼容入口，但不再出现在主帮助里。完整参考见
[`docs/CLI_REFERENCE.md`](docs/CLI_REFERENCE.md)。

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

用户只使用 `model` 命令：

```sh
levier model connect \
  --url https://api.deepseek.com \
  --key sk-... \
  --model deepseek-flash \
  --models deepseek-flash,deepseek-reasoner \
  --default

levier model list
levier model check --name deepseek
```

`provider` 是控制面内部术语，不再作为公开命令。`secret_ref`、adapter 和
managed secret 文件由 `model connect` 自动处理。

### 6.2 Secret

控制面解析顺序：

```text
LEVIER_SECRET_<REF>
LEVIER_SECRETS_FILE JSON
LEVIER_SECRETS_WRITE_FILE JSON   # levier model connect 写入的 0600 managed 文件
```

`levier model connect` 不要求用户设置前两项；它通过 Admin API 把 key 写入
managed 文件。生产环境仍可继续使用只读 secret 文件或环境变量。

示例：

```text
secret_ref=deepseek_api_key
-> LEVIER_SECRET_DEEPSEEK_API_KEY
```

生产 secret 文件必须是 0600；模型 key 不会通过 `/meta`、模型列表、
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

首选 `Runtime.start(run)`：不需要 YAML。下面是显式配置文件的高级写法。

Runtime YAML：

```yaml
middleware:
  address: "127.0.0.1:9090"
  auth_token: "change-me"

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
LEVIER_SECRETS_FILE                 0600 JSON 只读 secret 文件
LEVIER_SECRETS_WRITE_FILE           0600 JSON managed secret 文件
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
- 全部操作面：`docs/OPERATIONS_SURFACE.md`
