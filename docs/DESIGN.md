<!-- Middleware — core design, current single-node MVP. -->

# Middleware 核心设计

| 项目 | 内容 |
|---|---|
| 状态 | 单机 MVP 实现基线 |
| 范围 | 单 Middleware、单 PostgreSQL、单 Harness、单 App |
| Runtime | `normal` |
| 语义 | Run 调度 at-least-once；工具可能重复执行 |
| 详细契约 | `CONTRACT.md`、`DAEMON_CONTRACT.md`、`APP_ONBOARDING.md`、`MCP.md`、`AGENT_CORE_CONTRACT.md` |

本文只描述**当前真实实现**。`sandbox`、`longterm`、多实例/HA、长时间浸泡不在 MVP 范围内。

---

## 1. 目标与非目标

### 1.1 目标

```text
App 通过 HTTP/WS 接入，不需要自己写 Agent 循环
Middleware 负责注册、调度、租约、工具路由、事件持久化
Harness 负责单次 Run 的模型/工具循环
MCP 连接与凭据由独立 runtime 进程负责
Run 能排队、执行、取消、超时、恢复
```

### 1.2 非目标

```text
多 Middleware / 多 PostgreSQL / HA
sandbox 隔离与预热池
longterm checkpoint 续跑
工具 exactly-once
高并发/压测指标
跨地域调度
```

### 1.3 崩溃语义

```text
normal runtime 没有 checkpoint 续跑
Harness 中途死亡 → Run 重新排队 → 新 attempt 从头执行
因此 local 工具是 at-least-once
```

需要副作用幂等的 App 必须按 `invocation_id` 去重，或接受重复。

---

## 2. 角色与边界

```text
App          业务函数 + 工具元数据 + 一条 WebSocket
Middleware   控制面：注册、调度、租约、事件、工具路由
Harness      单次 Run 的 Agent 循环（当前 Python）
MCP Runtime  独立进程：MCP 连接、session、凭据解析
PostgreSQL   目录、Run、事件、任务、调用记录
```

边界纪律：

```text
Middleware 不执行 App 业务代码
Middleware 不代理 MCP 工具调用
Harness 不见 MCP 明文凭据
App 不见模型凭据
Agent Core 不依赖 Middleware/Harness/MCP/protobuf
```

---

## 3. 包与依赖方向

```text
contract     错误码/事件字段/WS 常量，无 internal 依赖
runstore     Run/Event/App/Session 持久化
register     App/工具/MCP/材料/密钥目录
workflow     PostgreSQL RunTaskQueue
control      调度、租约、事件投影、生命周期
gateway      工具路由与幂等：local / global
connection   Daemon Hub / App Hub / 在线状态 / 收发
httpapi      HTTP + WebSocket 适配
observe      指标、日志、脱敏
cmd/         进程入口与 wiring
```

依赖：

```text
httpapi   → register / runstore / connection / workflow
control   → register / runstore / workflow / connection / protocol
gateway   → register / connection
register  → runstore
runstore  → 无 internal 依赖
contract  → 无 internal 依赖
```

护栏：`make deps-check`。

---

## 4. 一次 Run

### 4.1 创建

```text
App POST /api/v1/runs
  → 校验 App key / app_id
  → 一个 PostgreSQL 事务：
       INSERT runs (queued)
       INSERT run_tasks RUN_DISPATCH
       INSERT run_tasks RUN_TIMEOUT
  → 202 + run_id + ws_token
```

任一步失败整体回滚，不会留下“Run 已创建但任务没入队”的孤儿。

### 4.2 调度

```text
RUN_DISPATCH 被 worker claim
  → control.Handle
  → 组装 TaskAssign（工具、MCP binding、模型、超时）
  → PickHealthy(runtime, strategy, labels)
  → AssignRun（attempt+1, lease_epoch+1）
  → 发送 TaskAssign
  → 等 TaskAck（默认 5s）
```

Ack 超时/拒绝：

```text
排除该 daemon，换下一个
没有可用 daemon → RUN_RETRY（退避）→ 最终 DEAD
```

### 4.3 执行

```text
Harness 发送 RUN_STATUS running
  → MW 置 running
Harness 调模型
  → 工具调用经 RuntimeChannel 回 MW
  → MW 路由到 App/global executor/MCP runtime
Harness 发送 MESSAGE/USAGE/TOOL/ERROR 事件
  → MW 持久化并推送给 App
Harness 发送 RUN_STATUS terminal
  → MW CAS 到 completed/failed/cancelled/timeout
  → 取消剩余 tasks
```

### 4.4 状态机

```text
queued → assigned → running → completed
   │         │         ├──→ failed
   │         │         ├──→ cancelled
   │         │         └──→ timeout
   │         ├──→ queued（daemon lost / reconcile）
   │         └──→ cancelled（user）
   └──→ cancelled / timeout
```

终态不可逆。`attempt` 和 `lease_epoch` 用于 fencing：

```text
旧 attempt / 旧 epoch 的事件不落库
旧 daemon 的终态不会覆盖新终态
```

---

## 5. RunTaskQueue

表：`run_tasks`。

```text
PENDING → LEASED → DONE
   ▲         │
   │         ├── Nack(retry) → PENDING
   │         └── 超过 max_attempts → DEAD
   └── ReapExpired
```

任务类型：

```text
RUN_DISPATCH   首次调度
RUN_RETRY      daemon 丢失/拒绝后重试
RUN_TIMEOUT    Run 截止时间
RUN_RECONCILE  预留给后续对账
```

关键机制：

```text
Claim 使用 FOR UPDATE SKIP LOCKED
租约 = owner + token + expires_at
worker 定期 Extend 租约
Reaper 回收过期 LEASED
Enqueue 对同一 (run_id, task_type) 的 active task 幂等
CancelRun 取消 Run 的所有 active task
```

启动恢复：

```text
RecoverQueued  给 queued Run 补 dispatch/timeout
RecoverActive  grace period 后重排 daemon 不在线的 assigned/running Run
```

---

## 6. 连接平面

### 6.1 Daemon

```text
gRPC 双向 RuntimeChannel
第一条消息必须是 Hello（daemon_id + token）
Heartbeat 10s，超时 30s 判离线
出站有界队列 + 写协程
慢消费者会断连
```

daemon 断线：

```text
OnOffline → 该 daemon 的 active Run 重新排队
新 attempt / 新 lease_epoch
旧执行的事件被 fencing 丢弃
```

重连对账（Hello）：

```text
daemon 未报告的 assigned/running → requeue
epoch 不一致 → 取消旧执行 + requeue
terminal Run 仍被报告 → 取消旧执行 + 失败 local tools
```

### 6.2 App

```text
HTTP 注册/查询
WebSocket 接收 local_tool_call / 事件 / run_completed
WebSocket 回 local_tool_result
```

App 断线：

```text
Run 不暂停
pending local tool 失败（APP_UNREACHABLE）
App 重连后可重新订阅/补拉
```

`/healthz` 是 liveness；`/readyz` 检查 PostgreSQL。

---

## 7. 事件模型

事件主键：`(run_id, attempt, seq)`。

```text
daemon 事件：source=daemon，seq 从 1 递增
MW 事件：   source=mw，seq 在 [2^62, 2^63)
```

规则：

```text
只有持久事件消耗 seq
MESSAGE_DELTA 是 ephemeral，不落库
允许窗口内乱序（16）
超过窗口记录 gap
旧 epoch 事件丢弃
MW 事件不推进 daemon watermark
```

控制面终态事件：

```text
cancel  → source=mw RUN_STATUS cancelled
timeout → source=mw RUN_STATUS timeout
requeue → source=mw RUN_STATUS queued
```

事件缓冲：

```text
普通事件满 → 丢弃并计数
终态/ERROR 满 → 短暂等待，仍满则 requeue daemon + 断连
```

补拉：

```text
GET /api/v1/runs/{id}/events?after_attempt=&after_seq=&limit=
WS subscribe(last_attempt, last_seq)
```

---

## 8. 工具路径

### 8.1 local

```text
Harness → ToolCallRequest
  → Gateway 幂等/single-flight
  → AppHub 广播到 App 连接
  → App 回 local_tool_result(correlation_id, invocation_id)
  → Gateway 落 tool_invocations
  → 返回 Harness
```

约束：

```text
App 多连接时会广播；App 必须按 invocation_id 幂等
工具超时默认 60s，最大 4m
```

### 8.2 global

```text
Harness → Gateway → executor(http/webhook)
同一 invocation_id 幂等
结果落 tool_invocations
```

### 8.3 MCP

```text
Harness → mcp_runtime（本地 JSON-RPC）
  → MCP server
```

```text
不经过 MW Gateway
不写 tool_invocations
Harness 只拿到 runtime 解析后的连接描述
凭据不出 runtime
allowed_tools 由 runtime 强制
```

---

## 9. 安全

```text
Admin token   只用于创建/轮换 App，不进入运行时
App key       只在创建/轮换时返回一次；DB 存 hash
daemon token  预配置，Hello 校验
ws_token      短期（默认 10m），绑定 app_id
```

```text
App key 不写日志/证据
MCP secret 用 ${secret:name} 占位，由 runtime 解析
日志只记录安全摘要
make secrets-check 扫描 tracked 文件
```

---

## 10. 可观测性

```text
request_id = X-Request-ID 或自动生成
trace_id = Run 的关联 ID
访问日志：成功 API 请求 INFO；/healthz /readyz DEBUG
指标：HTTP、Run、事件、工具、队列、daemon、token
```

关键指标：

```text
mw_runs_total{status,runtime}
mw_events_total{type}
mw_events_dropped_total{reason}
mw_event_append_duration_seconds
mw_tool_invocations_total{tool,scope,status}
mw_workflow_*（claim/lease/reaper）
mw_daemon_capacity
```

---

## 11. Agent Core

```text
agent_core/  语言无关的循环、工具编排、事件、取消
harness/     适配层：TaskAssign ↔ RunSpec、CoreEvent ↔ MW Event、工具桥
```

规则：

```text
core 默认工具串行
core 不做重试
checkpoint 由 MW/adapter 持有
core 不 import Middleware/Harness/MCP/protobuf
```

Conformance fixtures：`agent_core/conformance/fixtures/*.json`。

---

## 12. 故障与恢复

已验证矩阵（`make fault-matrix`）：

| 故障 | 行为 |
|---|---|
| Harness kill -9 | Run requeue，新 attempt 完成 |
| MW 重启 | Harness 重连，Run 完成 |
| App 断线 | pending tool 失败，Run 收敛；App 恢复后新 Run 正常 |
| cancel | cancelled + MW 事件 + 任务取消 |
| timeout | timeout + MW 事件 + 任务取消 |
| DB 队列失败 | 创建 Run 事务回滚，503 |
| 终态事件缓冲满 | 断连 requeue，不静默丢终态 |
| 工具 panic | TOOL_EXEC_ERROR，不杀进程 |
| 慢 App / replay 过多 | 有界队列/缓冲，断连后补拉 |

未覆盖：

```text
PostgreSQL 整体停机演练
40s 长工具真实 E2E
多 Harness / 多 MW
Docker Compose 真实构建
```

---

## 13. 测试与运行

```sh
make build
make lint                 # gofmt + vet + secrets-check
make mvp-demo             # echo
make mvp-demo-react       # 真实模型 + local tools
make fault-matrix
make retrofit-e2e
make mcp-e2e
make agent-core-test
make harness-test
make mcp-runtime-test
make counter-agent-test

MIDDLEWARE_TEST_POSTGRES=1 go test -count=1 ./...
MIDDLEWARE_TEST_POSTGRES=1 go test -race -count=1 ./...
```

目录：

```text
tests/contract/   协议黄金测试
tests/e2e/        mvp-demo / fault-matrix / retrofit / mcp
tests/app/        App 接入自检
tests/checks/     依赖、规模、密钥扫描
```

Go 测试按 Go 约定与被测包同目录；Python 测试集中在各组件 `tests/`。

---

## 14. 已知缺口

```text
sandbox runtime             未实现
longterm checkpoint 续跑    未实现；API 字符串仍保留但不应使用
多实例/HA                   未实现
工具 exactly-once           不保证
Docker Compose              未验证
OTel / dashboard            未实现
```

新增范围前先更新本文档与对应契约，再改代码。
