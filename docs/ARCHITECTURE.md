<!-- Middleware — one-page architecture map. -->

# 架构地图

这页只回答“东西放在哪、依赖往哪走、新东西加在哪”。
详细契约见 `docs/DESIGN.md`、`docs/APP_ONBOARDING.md`、`docs/DAEMON_CONTRACT.md`、`docs/MCP.md`。

## 1. 角色

```text
App           业务函数 + 工具元数据 + WebSocket
Middleware    注册、调度、路由、事件、持久化
Harness       单次 Run 的 Agent 循环（当前 Python）
MCP Runtime   独立进程：MCP 连接、会话、凭据解析
PostgreSQL    目录、运行、事件、任务、调用记录
```

## 2. 包分层

```text
contract        错误码/事件字段/WS 常量（无 internal 依赖）
runstore        持久化：runs / events / sessions / apps
register        目录与 ACL：apps / tools / mcp / materials / secrets
workflow        持久任务队列与 worker
control         调度、租约、事件投影、生命周期
gateway         工具路由与幂等：local / global
connection      daemon/app 在线状态与收发接口
httpapi         HTTP + WebSocket 适配
observe         指标与日志字段
cmd/middleware  进程入口与 wiring
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

强制执行：`scripts/check_deps.sh`（`make deps-check`）。

## 3. 一次 Run

```text
App POST /runs
  → handleCreateRun（httpapi/runs.go）
  → runstore.CreateRun(queued)
  → workflow RUN_DISPATCH
  → control.Handle（control/schedule.go）
      → register.AssembleTaskMaterials
      → AssignRun(attempt/lease_epoch)
      → TaskAssign → Harness
  → Harness events
      → control.handleEvent（control/events.go）
      → runstore.AppendEvent
      → appHub.Publish（订阅者）
  → RUN_STATUS terminal
      → control.Transition
      → cancelTasks / publishCompleted
```

## 4. 三种工具路径

```text
local    Harness → ToolCallRequest → gateway.Invoker → AppHub → App WS
global   Harness → ToolCallRequest → gateway.Invoker → executor (http/webhook)
MCP      Harness → mcp_runtime（本地 JSON-RPC）→ MCP server
```

- local/global 经过 Gateway，有 `tool_invocations`。
- MCP 不经过 Gateway，只有 Harness 事件。
- MCP 凭据由 runtime 解析，不进入 TaskAssign 明文。

## 5. 加东西的位置

| 需求 | 位置 |
|---|---|
| 新 App | 不改代码；按 `APP_ONBOARDING.md` |
| 新 Daemon | `DAEMON_CONTRACT.md` + `make daemon-check` |
| 新 HTTP 路由 | `httpapi/` 对应文件 + `openapi.yaml` |
| 新事件字段 | `internal/contract/events.go`（Go）并与 Harness 对齐 |
| 新错误码 | `internal/errs/errors.go` |
| 新 MCP transport | `mcp_runtime/sessions.py` + 测试 |
| 新存储查询 | 对应 `runstore/*.go` |

## 6. Agent Core（P8C）

```text
agent_core/          循环、工具编排、CoreEvent、取消（无 MW 依赖）
harness adapter      TaskAssign ↔ RunSpec、CoreEvent ↔ MW Event、工具桥
```

契约：`docs/AGENT_CORE_CONTRACT.md`。
P8C.2 已把 ReAct 循环搬入 core；harness 只保留 TaskAssign/事件/工具适配。

## 7. 明确不做的事

```text
不引入 DI / 六边形 / 通用领域层
不为行数强制拆函数
不让 Middleware 代理 MCP 调用
不让 Harness 见 MCP 凭据
不新增第二套事件/错误模型
```

## 8. 长函数评审结果

保留，不拆：

| 函数 | 行数 | 原因 |
|---|---:|---|
| `main` | 261 | 启动顺序与清理顺序必须在一处可见 |
| `handleEvent` | 167 | 事件处理有严格顺序，拆开反而隐藏时序 |
| `Handle` | 146 | 分配重试循环是一个完整状态机 |
| `Stats` | 121 | SQL 聚合字段与扫描顺序强耦合 |
| `handleWS` | 109 | 一条连接的消息 switch，拆开增加跳转 |
| `handleCreateRun` | 99 | 校验、落库、入队是一段事务性流程 |
| `Validate` | 82 | 配置校验按字段顺序阅读更直接 |

原则：

```text
只在重复逻辑或深层嵌套时抽取
提取后必须有清晰的名字和独立契约
不为满足行数指标拆
```
