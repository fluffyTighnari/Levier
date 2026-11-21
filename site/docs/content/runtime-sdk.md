---
title: Runtime SDK
status: published
description: 一个 runtime_name，一个 run(ctx)。传输、心跳、事件序号与取消由 SDK 负责。
updated: 2026-11-20
---

> [!NOTE]
> 读完之后：你能写出最小 Runtime，把它接到控制面，并知道哪些东西不需要自己实现。

## 1. 最小 Runtime

一个 Runtime 就是一个进程、一个 `runtime_name`、一个 `run(ctx)`：

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

`run` 的返回值就是这次 Run 的终态字符串；抛出的未捕获异常会被映射成 `ERROR` + `failed`。

## 2. SDK 负责什么

- RuntimeChannel、Hello、心跳与重连
- Ack、取消、`lease_epoch`
- event seq 与 terminal 状态
- `ctx.call_tool` 的 `TOOL_CALL_BEGIN` / `TOOL_CALL_END`
- `ctx.error`
- 未捕获异常映射为 `ERROR` + failed
- `ctx.logger` 自动带 run 上下文

自定义 Runtime 可以完全不使用 React 或 LLM；工具统一走 `ctx.toolbox()`。

> [!TIP]
> 如果你只是想验证 Runtime 契约，直接跑内置 Echo Runtime，不要从零写。

## 3. 配置

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

启动前先通过控制面注册模型 provider。Runtime 会从 `TaskAssign` 收到本次 Run 解析后的模型地址与 key，
**不需要把模型 key 写进 Runtime 环境**。

## 4. 启动与确认

```sh
python -m your_runtime --config runtime.yaml
```

> [!OUTPUT]
> 启动后，`./bin/levier runtime list` 应该能看到这个 `runtime_name`；
> `./bin/levier status` 的 `daemons` 计数会增加。

> [!FAIL]
> 进程起来了但列表里没有：先检查 `middleware.address` 与 `auth_token`，
> 再看 `docker compose logs --tail=100 levier` 中的 Hello 校验记录。

## 5. 内置 Runtime

```sh
# Echo：最小 conformance 示例
PYTHONPATH=execution/runtime_sdk/src:execution/echo_runtime/src \
  python -m levier_echo_runtime \
  --config execution/echo_runtime/runtime.yaml

# React：默认 ReAct 执行侧
PYTHONPATH=execution/agent_core/src:execution/runtime_sdk/src:execution/react_runtime/src \
  python -m levier_react_runtime \
  --config execution/react_runtime/react.example.yaml
```

## 6. 想复用 ReAct

```python
from levier_runtime.llm import ProviderRouter
from levier_runtime.react import ReactAgent

agent = ReactAgent(ProviderRouter())
await runtime.serve(agent)
```

ReAct、provider、checkpoint 都是可选 helper，不是 SDK 核心。MCP 工具由
`ctx.toolbox()` 自动接入。
把它们换掉不会影响 Runtime 契约。

## 7. CLI

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

## 8. 下一步

- Runtime 在架构里的位置：[架构](architecture.md)。
- 需要 MCP 工具：[MCP](mcp.md)。
- App 侧接入：[App SDK](app-sdk.md)。
