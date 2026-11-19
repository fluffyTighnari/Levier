# Runtime 接入

> 目标：Runtime 作者只写 `runtime_name`、`run(ctx)` 和可选能力。
>
> Runtime SDK 负责 RuntimeChannel、心跳、重连、Ack、事件序号、取消和
> tool bridge。Runtime 作者不需要理解 gRPC、protobuf、lease_epoch 或 event seq。

---

## 1. 最小 Runtime

```python
from levier_runtime import RunContext, Runtime

async def run(ctx: RunContext) -> str:
    await ctx.reply("echo")
    return "completed"

if __name__ == "__main__":
    Runtime.from_config("runtime.yaml").run(run)
```

`Runtime.from_config()` 是唯一入口；也可以用 `LEVIER_RUNTIME_CONFIG`
指定配置文件。

## 2. 配置

```yaml
middleware:
  address: "127.0.0.1:9090"
  tls: false
  auth_token_file: "/run/secrets/runtime_token"

runtime:
  name: "react1"
  instance_id: "react-local-1"
  type: "normal"
  max_concurrent_runs: 4
  labels:
    pool: "local"

logging:
  level: "info"
```

- `runtime.name`：App 路由键
- `runtime.instance_id`：实例标识；同一个 name 可以有多个实例
- `runtime.type`：控制面/部署属性
- capacity 和 labels 由 Runtime 声明
- 不要在 Runtime 配置里写 strategy

## 3. `RunContext`

### 核心字段

```text
ctx.runtime_name
ctx.run_id
ctx.input
ctx.params
ctx.timeouts
ctx.cancelled
```

### 可选字段

```text
ctx.model
ctx.tools
ctx.mcp_bindings
ctx.session_id
ctx.resume_from
```

### 核心方法

```text
await ctx.reply(text)
await ctx.call_tool(name, arguments, timeout_ms=...)
await ctx.error(code, message, retryable=False)
```

`ctx.call_tool` 默认自动发 `TOOL_CALL_BEGIN` / `TOOL_CALL_END`，Runtime 作者
不需要自己维护 invocation id、耗时或工具错误事件。自定义循环如果已经
自行发这些事件，可传 `emit_events=False`。

SDK 还会把未捕获的 `run(ctx)` 异常统一映射为 `ERROR` + failed terminal
状态；`ctx.logger` 自动带 run 上下文。

### 可选方法

```text
await ctx.emit(event_type, data, ephemeral=False)
await ctx.checkpoint(state)
```

`ctx.reply` 由 SDK 生成消息 ID 和事件序号。`ctx.call_tool` 返回普通
`ToolResult`，不暴露 protobuf。

## 4. 可选能力

- **Model**：读取 `ctx.model`，自行调用 LLM；SDK 不包含 LLM。
- **Tools**：读取 `ctx.tools`，通过 `ctx.call_tool` 调用。
- **MCP**：读取 `ctx.mcp_bindings`，由 Runtime 自己决定是否打开 MCP 会话。
- **Longterm**：读取 `ctx.session_id` / `ctx.resume_from`，按需调用
  `ctx.checkpoint`。
- **Streaming**：按需使用 `ctx.emit("MESSAGE_DELTA", ...)`。

## 5. 启动和验证

```sh
PYTHONPATH=execution/runtime_sdk/src:execution/echo_runtime/src \
  python -m levier_echo_runtime \
  --config execution/echo_runtime/runtime.yaml
```

最小验证：

```sh
levier run submit \
  --runtime echo1 \
  --prompt "hello" \
  --watch
```

> `model` 可选；Echo Runtime 不需要 model。

## 6. 边界

Runtime SDK 不包含：

```text
LLM
ReAct loop
MCP client
ToolRouter
Strategy / StrategyRouter
Runtime Admin API
多语言 SDK
```

`agent_core` 是可选循环辅助库，不是 Runtime Core。
