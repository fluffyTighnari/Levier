---
title: 架构
status: published
description: 进程视图、控制面包、Run 生命周期与三条工具路径。排查与扩展时再读。
updated: 2026-11-20
---

> [!NOTE]
> 这一页不是接入前置。第一次接入只需要 [快速开始](quickstart.md) 与 [App SDK](app-sdk.md)。
> 当你要排查并发、重复执行、路由或部署问题时，再回到这里。

## 1. 进程视图

```text
App             App SDK + WebSocket
Levier          控制面：API、run store、scheduler、gateway、events
React Runtime   内置 ReAct runtime（薄组合层）
Echo Runtime    最小 run(ctx) 参考实现
MCP Runtime     独立共享 MCP 进程与 socket
PostgreSQL      runs、events、sessions、apps、tools、registrations
```

## 2. 控制面的包

```text
contract        wire contracts and event names
runstore        runs / events / sessions / apps persistence
register        app, tool, MCP, model-provider, material, secret registration
workflow        durable run workflow and worker loop
control         scheduling, attempts, leases, event handling, transitions
gateway         local/global tool invocation routing
connection      runtime/app connections and channels
httpapi         HTTP + WebSocket API
observe         logs, metrics, health
cmd/levier      dependency wiring and entry points
```

依赖方向：

```text
httpapi   → register / runstore / connection / workflow
control   → register / runstore / workflow / connection / protocol
gateway   → register / connection
register  → runstore
runstore  → 无 internal 依赖
contract  → 无 internal 依赖
```

`tests/checks/check_deps.sh` 强制这个方向。

## 3. Run 生命周期

```text
App POST /runs
  → handleCreateRun
  → runstore: RUN_DISPATCH / RUN_TIMEOUT
  → workflow
  → control.Handle
      → register.AssembleTaskMaterials
      → resolve model provider + secret ref
      → AssignRun(attempt, lease_epoch)
      → TaskAssign → selected Runtime
  → Runtime events over RuntimeChannel
      → control.handleEvent
      → runstore.AppendEvent
      → appHub.Publish
  → RUN_STATUS terminal
      → control.Transition
      → cancel tasks / publish completion
```

## 4. 三条工具路径

```text
local    Runtime → ToolCallRequest → gateway.Invoker → AppHub → App WS
global   Runtime → ToolCallRequest → gateway.Invoker → executor (HTTP/webhook)
MCP      Runtime → mcp_runtime JSON-RPC → MCP server
```

local / global 经过 Gateway，并记录进 `tool_invocations`。MCP 不经过 Gateway。

## 5. Runtime 边界

Runtime 作者只写 `run(ctx)` 与循环逻辑。SDK 负责：

- channel connection、Hello、heartbeat、reconnect
- Ack 与 `lease_epoch`
- event sequence numbers 与 terminal status
- cancellation 与 checkpoint transport
- 自动 `TOOL_CALL_BEGIN` / `TOOL_CALL_END`
- `ctx.error`、`ctx.logger`、未捕获异常映射

React / LLM / tools / checkpoint 都是可选 helper，不是 Runtime core。
Echo 不依赖它们，是最小 conformance 示例。

## 6. 安全与可观测性

```text
Admin token   只用于创建 / 轮换 App，不进入运行时
App key       只在创建 / 轮换时返回一次；DB 存 hash
daemon token  Hello 校验
ws_token      短期（默认 10m），绑定 app_id
secret_ref    只保存引用，不保存明文
```

关键指标前缀为 `levier_`：

```text
levier_runs_total{status,runtime}
levier_events_total{type}
levier_tool_invocations_total{tool,scope,status}
levier_daemon_capacity
```

## 7. 已知缺口

- sandbox runtime 未实现。
- longterm checkpoint 续跑未实现。
- 多实例 / HA 未实现。
- 工具 exactly-once 不保证。
- docker compose 的完整构建需在有 Docker 的环境验证。

> [!WARNING]
> 不要把 MVP 当成生产高可用方案。单机、无 HA、无 exactly-once，
> 这些边界会直接影响你的重试与幂等设计。

## 8. 下一步

- 接入：[App SDK](app-sdk.md)、[Runtime SDK](runtime-sdk.md)、[MCP](mcp.md)。
- 操作：[CLI 参考](cli.md)。
- 出问题：[排查](troubleshooting.md)。
