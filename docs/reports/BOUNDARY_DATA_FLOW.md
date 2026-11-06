<!-- Middleware — middleware/daemon data structures and boundary flow. -->

# Middleware 与 Daemon 的数据结构与边界流

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-06 |
| 范围 | `runtime.proto`、Go 生成代码、Python 生成代码、`internal/workflow`、`internal/protocol`、`internal/testutil/fakedaemon`、`harness/channel`、`harness/runtime` |
| 重点 | Envelope 在两端的存在形式、Task 在两端的存在形式、哪些字段过线、哪些字段只在一侧 |

---

## 0. 先给结论

### 0.1 当前实际状态

- **协议已经定义**：`proto/middleware/runtime/v1/runtime.proto`。
- **两端 stub 已生成**：
  - Go：`gen/middleware/runtime/v1/runtime.pb.go`、`runtime_grpc.pb.go`
  - Python：`harness/src/gen/middleware/runtime/v1/runtime_pb2.py`、`runtime_pb2_grpc.py`
- **Daemon 侧客户端已实现**：
  - `harness/src/middleware_harness/channel/client.py`
  - 可以连接、发 Hello、心跳、收发 Envelope、重连。
- **Middleware 侧 gRPC 服务端还没有实现**：
  - `internal/connection/` 目前只有 `doc.go`。
- **Middleware 侧 TaskAssign 组装、事件落库、Tool Gateway 还没有实现**：
  - `internal/control/`、`internal/runstore/`、`internal/register/` 目前都只有 `doc.go`。
- **Workflow 库已经实现**：
  - `internal/workflow` 管理 `run_tasks` 表。
  - 但它目前没有和 gRPC/Control 连起来。

所以下面的“边界流”要分两层看：

1. **契约层**：proto 已经完整定义。
2. **实现层**：Daemon 侧的 Channel/RunManager 已经实现；Middleware 侧的 DaemonHub、TaskAssign 组装、事件持久化还没有实现。

### 0.2 最重要的一句话

> **`run_tasks` 是 Middleware 内部的持久化任务；`TaskAssign` 是发给 Daemon 的一次 Run 描述。二者不是同一个结构，也不是一一字段映射。**

- Task 的 `task_type`、`task_status`、`task_id`、`lease_owner`、`lease_token`、`task_attempt` **不会直接过线**。
- Daemon 不知道“这是一个 dispatch 任务还是 timeout 任务”；它只收到“执行哪个 Run、用什么策略”。
- Daemon 侧没有持久化 Task；只有一个内存中的 `RunContext`。

---

## 1. Envelope 的结构

### 1.1 proto 定义

文件：`proto/middleware/runtime/v1/runtime.proto`

```proto
message Envelope {
  string msg_id = 1;
  string correlation_id = 2;
  string run_id = 3;
  int64  ts_ms = 4;
  int32  attempt = 5;
  int64  lease_epoch = 6;
  oneof payload { ... }
}
```

### 1.2 各字段含义

| 字段 | 作用 | 谁生成 | 是否持久化 |
|---|---|---|---|
| `msg_id` | 当前 Envelope 的唯一 ID，ULID | 发送方 | 不持久化；用于日志/correlation |
| `correlation_id` | 响应关联 ID；等于请求的 `msg_id` | 响应方 | 不持久化 |
| `run_id` | 归属 Run；Hello/Heartbeat/Reconcile 为空 | 发送方 | 随事件/Task 关联 |
| `ts_ms` | 发送时间，Unix 毫秒 | 发送方 | 不单独持久化 |
| `attempt` | Run 执行 attempt | Middleware 下发 TaskAssign 时设置；Daemon 回传 | 随事件持久化 |
| `lease_epoch` | Run 归属代次，防旧 daemon | Middleware 下发；Daemon 回传 | 随事件/Hello 对账使用 |

### 1.3 oneof payload

`payload` 只能是以下之一：

| 方向 | payload | 作用 |
|---|---|---|
| Daemon → MW | `hello` | 连接后第一条消息，声明身份/容量/策略/运行中 Run |
| Daemon → MW | `heartbeat` | 每 10s 上报 running_count/负载 |
| MW → Daemon | `task_assign` | 派发一次 Run |
| Daemon → MW | `task_ack` | 接受/拒绝 TaskAssign |
| Daemon → MW | `event` | Run 事件 |
| Daemon → MW | `tool_call_request` | 请求 MW 执行工具 |
| MW → Daemon | `tool_call_result` | 返回工具结果 |
| MW → Daemon | `cancel` | 取消一个 Run |
| MW → Daemon | `reconcile` | 重连后取消 MW 认为已死的 Run |
| Daemon → MW | `checkpoint` | longterm 快照 |
| 双向 | `error` | 协议/运行错误 |

### 1.4 两端的存在形式

**Go 侧**

- 类型：`runtimev1.Envelope`
- 文件：`gen/middleware/runtime/v1/runtime.pb.go`
- 构造/校验：`internal/protocol/envelope.go`、`internal/protocol/validate.go`
- 关键函数：
  - `protocol.New(payload, opts...)`
  - `protocol.NewRun(runID, attempt, epoch, payload, opts...)`
  - `protocol.NewResponse(request, payload, opts...)`
  - `protocol.Validate(envelope)`
  - `protocol.PayloadName(envelope)`

**Python 侧**

- 类型：`runtime_pb2.Envelope`
- 文件：`harness/src/gen/middleware/runtime/v1/runtime_pb2.py`
- 构造：`harness/src/middleware_harness/channel/envelope.py`
  - `new_envelope(payload, field, ...)`
  - `response_envelope(request, payload, field, ...)`
- 客户端：`harness/src/middleware_harness/channel/client.py`
  - 负责写 stream、读 stream、重连、心跳、分发。

### 1.5 传输格式

- 线上是 **protobuf 二进制**，不是 JSON。
- proto 字段名不参与线上传输；线上是 field number + wire type。
- `tests/contract/envelopes.json` 是 protojson 黄金样本，只用于跨语言验证，不是线上格式。
- `Event.data` 是 `google.protobuf.Struct`，线上是一个结构化消息。
- `McpBinding.env/headers` 是 map；`TaskAssign.strategy_params` 是 Struct。

---

## 2. Task 在 Middleware 侧的存在形式

### 2.1 `workflow.Task`

文件：`internal/workflow/task.go`

| 字段 | 含义 | 是否过线 |
|---|---|---|
| `ID` | Task ID，ULID | **不过线** |
| `RunID` | 所属 Run | 过线为 `run_id` |
| `Type` | `RUN_DISPATCH` 等 | **不过线** |
| `Status` | PENDING/LEASED/... | **不过线** |
| `AvailableAt` | 最早可领取时间 | **不过线** |
| `Attempt` | Task 被 Claim 的次数 | **不过线** |
| `MaxAttempts` | Task 最大领取次数 | **不过线** |
| `LeaseOwner` | 哪个 worker 持有 | **不过线** |
| `LeaseToken` | Task 租约代次 | **不过线** |
| `LeaseExpiresAt` | Task 租约到期 | **不过线** |
| `LastError` | 最近错误 | **不过线** |
| `Payload` | 调度元数据 | **不直接过线**；TaskAssign 由 Control 组装 |
| `CreatedAt/UpdatedAt` | 时间 | **不过线** |

### 2.2 `run_tasks` 表

迁移：

- `internal/db/migrations/0001_init.sql`
- `internal/db/migrations/0002_run_tasks_constraints.sql`

关键约束：

- 同一 `(run_id, task_type)` 最多一个活跃任务。
- `attempt <= max_attempts`。
- LEASED 必须有 owner/token/expiry。
- 非 LEASED 必须没有 owner/expiry。

### 2.3 Task 状态与操作

| 操作 | 状态变化 | 文件 |
|---|---|---|
| `Enqueue` | 新建 PENDING | `internal/workflow/postgres.go` |
| `Claim` | PENDING → LEASED；attempt++、token++ | `internal/workflow/postgres_lease.go` |
| `Ack` | LEASED → DONE；清租约 | `internal/workflow/postgres_lease.go` |
| `Nack` | LEASED → PENDING 或 DEAD | `internal/workflow/postgres_lease.go` |
| `Fail` | LEASED → DEAD | `internal/workflow/postgres_lease.go` |
| `CancelRun` / `CancelTask` | PENDING/LEASED → CANCELLED；LEASED token++ | `internal/workflow/postgres_cancel.go` |
| `ReapExpired` | 过期 LEASED → PENDING/DEAD；token++ | `internal/workflow/postgres_reaper.go` |

### 2.4 Task 与 TaskAssign 的关系

**不是字段一一对应。**

```text
run_tasks 行
  run_id ────────────────► TaskAssign.run_id
  task_type ────────────► 不传；MW 自己决定怎么处理
  task_attempt ─────────► 不传；这是队列投递次数
  lease_token ──────────► 不传；这是 worker 租约
  payload ──────────────► 不直接传；TaskAssign 由 Run/Register 组装
  runs.attempt ─────────► Envelope.attempt
  runs.lease_epoch ─────► Envelope.lease_epoch
```

必须区分三个 attempt：

| 名字 | 在哪 | 含义 |
|---|---|---|
| `run_tasks.attempt` | MW 任务表 | Task 被 Claim 的次数 |
| `runs.attempt` | MW Run 表 | Run 被执行/重排队的次数 |
| `Envelope.attempt` | 线上 | Run attempt，用于事件分区 |

`TaskAssign` 里没有 `task_type`、`task_id`、`task_attempt`、`lease_token`。Daemon 完全不知道这些。

---

## 3. Task 在 Daemon 侧的存在形式

### 3.1 没有持久化 Task

Daemon 侧没有 `run_tasks` 表，也没有 Task ID。

Daemon 收到 `TaskAssign` 后，在内存中创建：

文件：`harness/src/middleware_harness/runtime/run.py`

```text
RunContext
  assign: TaskAssign
  attempt: int          # 来自 Envelope.attempt
  lease_epoch: int      # 来自 Envelope.lease_epoch
  events: EventWriter
  cancel_event: asyncio.Event
  task: asyncio.Task
  started_at
  outcome
```

### 3.2 TaskAssign 在 Daemon 侧的字段

| TaskAssign 字段 | Daemon 用途 |
|---|---|
| `run_id` | RunContext 身份 |
| `app_id` | 日志/上下文 |
| `strategy` | 选择策略（P3-26 之后） |
| `strategy_params` | 策略参数 |
| `input` | 初始消息 |
| `model` | LLM provider/model/key |
| `tools` | 工具清单（local/global 已合并） |
| `mcp_bindings` | MCP 连接描述 |
| `timeouts` | Run/Tool 超时 |
| `session_id` | longterm 会话 |
| `resume_from` | longterm 恢复快照 |
| `trace_context` | W3C traceparent |

`Envelope.attempt` 和 `Envelope.lease_epoch` 不在 TaskAssign 消息体里，Daemon 从外层 Envelope 读取。

### 3.2.1 TaskAssign 内嵌结构

**ChatMessage**

| 字段 | 含义 |
|---|---|
| `role` | system/user/assistant/tool |
| `content` | v1 纯文本 |
| `name` | 可选工具/参与者名 |

**ModelProfile**

| 字段 | 含义 |
|---|---|
| `provider` | anthropic/openai/... |
| `model` | 模型名 |
| `api_key` | v1 由 MW 解析后明文下发 |
| `base_url` | 可选 endpoint |
| `params` | temperature 等字符串参数 |

**ToolManifest**

| 字段 | 含义 |
|---|---|
| `name` | 工具名 |
| `description` | 给模型看的描述 |
| `input_schema_json` | JSON Schema 字符串 |
| `scope` | LOCAL/GLOBAL；仅供 Daemon 观测，路由仍由 MW 决定 |

Daemon 不根据 `scope` 决定路由；它把所有非 MCP 工具统一通过 ToolCallRequest 发给 MW。

**McpBinding**

| 字段 | 含义 |
|---|---|
| `server_name` | MCP server 名 |
| `transport` | stdio/sse/streamable_http |
| `command` / `args` / `env` | stdio 启动信息 |
| `url` / `headers` | HTTP 连接信息 |
| `allowed_tools` | 工具白名单 |
| `namespace` | 默认 `mcp__<server_name>__` |

`McpBinding` 包含凭据；Harness 只把描述转交独立 `mcp_runtime`，Runtime 只在内存使用，不写日志/checkpoint。MW 和 Harness 都不持有 MCP 连接。

**TimeoutProfile**

| 字段 | 含义 |
|---|---|
| `run_timeout_ms` | 整个 Run 的超时 |
| `tool_timeout_ms` | 默认工具超时 |

**Checkpoint**

| 字段 | 含义 |
|---|---|
| `session_id` | 会话 ID |
| `event_seq_watermark` | 快照覆盖的持久事件 seq |
| `state` | 对 MW 不透明的字节 |

`attempt` 不在 Checkpoint 消息体里，在 Envelope 上；MW 用 `(Envelope.attempt, event_seq_watermark)` 做复合 watermark。

### 3.3 活跃 Run 表

文件：`harness/src/middleware_harness/runtime/manager.py`

```text
RunManager.active: dict[run_id]RunContext
```

- `running_runs()`：返回 `(run_id, lease_epoch)` 列表，用于 Hello 重连对账。
- `running_count()`：用于 Heartbeat。
- `Cancel`/`Reconcile`：按 `run_id` 找 RunContext，设置 cancel_event，取消 task。
- `close()`：取消所有活跃 Run。

### 3.4 Daemon 侧 Task 生命周期

```text
TaskAssign 到达
  → 校验 run_id/app_id/strategy/attempt/lease_epoch
  → 容量检查
  → 发 TaskAck
  → 创建 RunContext
  → 发 RUN_STATUS running
  → 启动 runner
  → runner 结束
  → 发 RUN_STATUS completed/failed/cancelled
  → 从 active 删除
```

当前 `NoopRunner` 直接返回 `completed`；P3-26 会替换为 echo。

---

## 4. 线上消息逐个分析

### 4.1 Hello

**方向：Daemon → MW**

字段：

| 字段 | 含义 |
|---|---|
| `daemon_id` | 预分配 daemon ID |
| `auth_token` | 预分配 token |
| `runtime_type` | NORMAL/SANDBOX/LONGTERM |
| `max_concurrent_runs` | 容量 |
| `supported_strategies` | 支持的策略 |
| `version` | harness 版本 |
| `labels` | 调度标签 |
| `running_runs` | 重连时仍在运行的 `(run_id, lease_epoch)` |

Daemon 侧：

- `ChannelClient._hello()` 构造。
- 每次新建 stream 后直接写入，作为第一条消息。
- `connected` 在写入 Hello 后立即置位。

MW 侧（未来）：

- 校验 token。
- 注册/更新 daemon。
- 用 `running_runs` 触发对账。
- **目前没有 HelloAck**。Daemon 不会收到“认证成功”响应；认证失败通常表现为 stream 关闭，然后 Daemon 继续重连。

### 4.2 Heartbeat

**方向：Daemon → MW**

字段：

- `running_count`
- `cpu_load`
- `mem_mb`

Daemon 侧：

- 默认 10s 一次。
- 通过出站队列发送，不阻塞 reader。
- `running_count` 来自 `RunManager.running_count()`。

MW 侧（未来）：

- 更新 daemon 容量和最后心跳时间。
- 30s 未收到判 dead。

### 4.3 TaskAssign

**方向：MW → Daemon**

字段见 §3.2。

Daemon 侧：

- `RunManager._handle_task_assign` 校验。
- 5s 内 TaskAck（当前是立即发送）。
- 校验失败发 `TaskAck{accepted=false, reason=...}`。
- 接受后创建 RunContext。

MW 侧（未来）：

- Control 从 `runs`、`run_tasks`、Register 组装。
- 选择 daemon。
- 发送 TaskAssign。
- 等 TaskAck。
- 成功后 Run 状态进入 assigned/running。

### 4.4 TaskAck

**方向：Daemon → MW**

字段：

- `accepted`
- `reason`

关联：

- `Envelope.correlation_id = TaskAssign.msg_id`
- `Envelope.run_id/attempt/lease_epoch` 复制自 TaskAssign。

Daemon 侧：

- `ChannelClient.send_response(request, ack, "task_ack")` 自动填 correlation。

MW 侧（未来）：

- `accepted=false` → 换 daemon 或重排队。
- 5s 未收到 → 重排队。

### 4.5 Event

**方向：Daemon → MW**

字段：

- `seq`
- `type`
- `ephemeral`
- `source`
- `data`（Struct）

seq 规则：

- 持久事件：seq 从 1 递增。
- ephemeral：seq=0。
- Daemon 事件 source=`daemon`，seq < 2^62。
- MW 生成的事件 source=`mw`，seq >= 2^62（未来）。

Daemon 侧：

- `EventWriter` 负责 seq 和 Struct。
- 当前只发 `RUN_STATUS`。
- `MESSAGE_DELTA`/`MESSAGE_COMPLETED`/`TOOL_CALL_*`/`USAGE` 在 P3-26 之后。

MW 侧（未来）：

- 按 `(run_id, attempt, seq)` 幂等落库。
- 校验 `Envelope.lease_epoch == runs.lease_epoch`。
- ephemeral 只走 AppHub，不落库。

### 4.6 ToolCallRequest / ToolCallResult

**方向：**

- Daemon → MW：ToolCallRequest
- MW → Daemon：ToolCallResult

ToolCallRequest：

- `invocation_id`：Daemon 生成，幂等键
- `tool_name`
- `arguments_json`
- `timeout_ms`

ToolCallResult：

- `invocation_id`
- `ok`
- `result_json`
- `error{code,message,retryable}`

关联：

- `correlation_id = ToolCallRequest.msg_id`
- `invocation_id` 是业务幂等键。

当前状态：

- Daemon 侧还没有 ToolBridge（P3-29）。
- MW 侧还没有 Tool Gateway（P7）。

### 4.7 Cancel

**方向：MW → Daemon**

字段：

- `reason`

关联：

- `Envelope.run_id`

Daemon 侧：

- `RunManager._handle_cancel` 找 active Run。
- 设置 cancel_event，取消 task。
- 最终发 `RUN_STATUS cancelled`。
- 不回复 ack。

MW 侧（未来）：

- 标记 Run cancelled。
- 发送 Cancel。
- 等 daemon 终态事件或按超时兜底。

### 4.8 Reconcile

**方向：MW → Daemon**

字段：

- `cancel_run_ids`

关联：

- Envelope 的 `run_id` 为空（连接级消息）。

Daemon 侧：

- 对每个 id 找 active Run，取消。
- 不回复 ack。

MW 侧（未来）：

- Daemon 重连后对比 `Hello.running_runs` 与 DB。
- 发送 Reconcile。

### 4.9 Checkpoint

**方向：Daemon → MW**

字段：

- `session_id`
- `event_seq_watermark`
- `state`

注意：

- `attempt` 不在 Checkpoint 消息里，在 Envelope 上。
- MW 需要把 `(Envelope.attempt, event_seq_watermark)` 作为复合 watermark。
- `state` 对 MW 不透明。

当前状态：

- Daemon 还没有 checkpoint 实现（P3-30）。
- MW 还没有 session 存储（P5/P9）。

### 4.10 ErrorReport

**方向：双向**

字段：

- `code`
- `message`

与 ToolCallResult.error 不同：

- `ErrorInfo` 有 `retryable`。
- `ErrorReport` 没有 `retryable`，是协议/运行级错误。

---

## 5. Middleware 侧内部数据与边界的映射

### 5.1 一个 Run 在 MW 侧可能出现的行

| 表 | 关键字段 | 与边界的关系 |
|---|---|---|
| `runs` | `run_id`, `app_id`, `status`, `attempt`, `lease_epoch`, `assigned_daemon_id` | 组装 TaskAssign；提供 `Envelope.attempt/lease_epoch` |
| `run_tasks` | `task_id`, `run_id`, `task_type`, `status`, `attempt`, `lease_token` | 队列内部；不直接过线 |
| `run_events` | `(run_id, attempt, seq)`, `source`, `type`, `payload` | 由 Event 落库（未来） |
| `tool_invocations` | `invocation_id`, `run_id`, `tool_name`, `result` | 由 ToolCall 落库（未来） |
| `sessions` | `session_id`, `checkpoint_blob`, `event_attempt`, `event_watermark` | 由 Checkpoint 落库（未来） |

### 5.2 哪些消息会落库

| 消息 | 是否落库 | 落到哪里 |
|---|---|---|
| Hello | 否 | daemon 注册表/心跳时间（未来） |
| Heartbeat | 否 | daemon 状态/容量（未来） |
| TaskAssign | 否 | 由 Run/Task 组装，不单独存 |
| TaskAck | 否 | 调度结果，不单独存 |
| Event 持久 | 是 | `run_events`（未来） |
| Event ephemeral | 否 | AppHub 实时转发（未来） |
| ToolCallRequest/Result | 可选 | `tool_invocations` 审计（未来） |
| Cancel | 否 | 驱动 Run 状态，不单独存 |
| Reconcile | 否 | 对账动作，不单独存 |
| Checkpoint | 是 | `sessions.checkpoint_blob`/watermark（未来） |
| ErrorReport | 日志 | 不一定落库 |

### 5.3 哪些 ID 用于什么

| ID | 层级 | 用途 |
|---|---|---|
| `run_id` | Run | 跨消息、事件、任务、会话关联 |
| `task_id` | Task | 只在 MW 任务表内部 |
| `Envelope.msg_id` | 消息 | 唯一消息 ID、日志、correlation 目标 |
| `Envelope.correlation_id` | 请求-响应 | TaskAck/ToolCallResult 回填请求 msg_id |
| `invocation_id` | 工具调用 | 幂等键、结果缓存 |
| `session_id` | longterm | 会话/checkpoint |
| `lease_token` | Task 租约 | 只在 MW 任务表；不过线 |
| `lease_epoch` | Run 归属 | 过线；防旧 daemon |
| `Event.seq` | Run attempt 内 | 持久事件顺序 |

---

## 6. 一次完整边界流（含当前实现状态）

### 6.1 连接与 Hello

```text
1. Daemon 启动
2. ChannelClient._session()
3. grpc.aio channel_ready()
4. RuntimeServiceStub.Channel()
5. client 写 Hello
6. MW（未来）校验 auth_token
7. MW 记录 daemon / 返回对账结果（Reconcile）
```

当前：

- Daemon 已实现 1–5。
- MW 尚未实现 6–7。
- 没有 HelloAck；Daemon 在写 Hello 后立即认为 connected。

### 6.2 派发

```text
1. MW Control 从 run_tasks Claim 到 RUN_DISPATCH
2. 读取 Run/Register
3. 组装 TaskAssign
4. Envelope.attempt = runs.attempt
5. Envelope.lease_epoch = runs.lease_epoch
6. 发送 TaskAssign
7. Daemon 校验
8. Daemon 发 TaskAck(correlation=TaskAssign.msg_id)
9. MW 5s 内收到 accepted=true
10. Run 进入 assigned/running
```

当前：

- MW 侧 1–6/9–10 尚未实现。
- Daemon 侧 7–8 已实现。
- 当前 Daemon 的 TaskAck 是即时发送。

### 6.3 事件

```text
1. Daemon RunContext 创建 EventWriter
2. EventWriter 分配 seq
3. 发送 Event Envelope
4. MW（未来）校验 lease_epoch
5. MW 按 (run_id, attempt, seq) 落库
6. MW 更新 Run 投影
7. MW 向 AppHub fanout
```

当前：

- Daemon 侧只发 `RUN_STATUS`。
- MW 侧落库/fanout 尚未实现。

### 6.4 取消

```text
1. Control 标记 Run cancelled
2. 发送 Cancel(run_id)
3. Daemon RunManager 找 RunContext
4. 设置 cancel_event、取消 task
5. Daemon 发 RUN_STATUS cancelled
6. MW 记录终态
```

当前：

- Daemon 3–5 已实现。
- MW 1–2/6 尚未实现。

### 6.5 重连

```text
1. stream 断开
2. Daemon 重连
3. Hello(running_runs=[(run_id, lease_epoch)])
4. MW 对比 DB 中的 assigned_daemon_id/lease_epoch
5. MW 发 Reconcile(cancel_run_ids)
6. Daemon 取消对应 Run
7. 其余 Run 继续
```

当前：

- Daemon 1–3/5–6 已实现（Reconcile 处理已接）。
- MW 4 尚未实现。
- Daemon 的 RunManager 在 stream 断开时不会停止运行，符合设计。

---

## 7. 当前实现的边界细节与坑

### 7.1 Daemon 不知道 TaskType

`TaskAssign` 没有 `task_type`。Daemon 只看到策略和 Run 信息。

这意味着：

- `RUN_DISPATCH`、`RUN_RETRY`、`RUN_TIMEOUT`、`RUN_RECONCILE` 的差异完全在 MW。
- Daemon 不需要知道“这是重试还是第一次”。
- Daemon 只按 `attempt` 和 `lease_epoch` 处理事件/对账。

### 7.2 Daemon 不知道 Task ID / lease_token

- `TaskAssign` 没有 `task_id`。
- `lease_token` 不过线。
- Daemon 只持有 `run_id`、`attempt`、`lease_epoch`。
- Task 的 worker 租约与 Daemon 的 Run 归属是两套独立 fencing。

### 7.3 `attempt` 有三个

- `run_tasks.attempt`：任务投递次数。
- `runs.attempt`：Run 执行次数。
- `Envelope.attempt`：线上的 Run attempt。

`TaskAssign` 携带的是 `Envelope.attempt`（Run attempt），不是 Task attempt。

### 7.4 `lease_epoch` 只在 Envelope / Run

- `run_tasks.lease_token` 不过线。
- `runs.lease_epoch` 过线。
- Daemon 在 Hello/Event/Checkpoint 中回传 `lease_epoch`。
- MW 必须校验事件 epoch 与 Run 当前 epoch。

### 7.5 Hello 没有 HelloAck

- Daemon 在写 Hello 后立即认为连接成功。
- 如果 MW 因 token 错误关闭 stream，Daemon 只会重连，不会区分 `AUTH_FAILED`。
- 这是 P3-25 已知风险，后续需要策略。

### 7.6 断线期间 Daemon 的 Run 继续

- `ChannelClient` 重连不影响 `RunManager.active`。
- 事件会通过出站队列缓冲，重连后继续发。
- 队列容量 256；超出会阻塞发送方。
- MW 未实现时，事件可能积压或连接失败。

### 7.7 Daemon 侧没有持久化 Task

- 进程崩溃后 RunManager 和 RunContext 消失。
- MW 通过 daemon lost 重新排队。
- longterm 需要 checkpoint 才能恢复，P3-30。

### 7.8 MW 侧 Task 还没有接上 gRPC

- `internal/workflow` 的 Task 目前只被测试和 Benchmark 使用。
- `cmd/middleware` 只连数据库。
- `internal/connection` 还没有 DaemonHub。
- 所以“Task → TaskAssign”的完整链路尚未实现。

### 7.9 两端校验不对称

- Go 侧有 `internal/protocol.Validate`：
  - msg_id/correlation 必须是 ULID
  - payload 必须存在
  - run/attempt/epoch 范围
  - Event seq/source 规则
  - ToolCall 结果一致性
  - 大小上限
- Python 侧 `new_envelope`/`ChannelClient.send` 只做类型构造，不做同等校验。
- Python 侧只有 `RunManager` 做 TaskAssign 的部分业务校验。
- 真实 daemon 如果构造了非法 Envelope，MW 未来会在 `Validate` 拒绝；当前没有 MW 服务端，所以问题不会暴露。
- `internal/testutil/fakedaemon` 是 Go 测试工具，会调用 `protocol.Validate`，因此比真实 Python daemon 更严格。

### 7.10 `TaskAssign` 的组装还没有代码

- 当前没有 `Control` 代码把 `runs`、`run_tasks`、Register 目录组装成 TaskAssign。
- 因此 TaskAssign 的字段组合规则目前只存在于设计文档和 proto 中。
- 当前唯一会发送 TaskAssign 的地方是 Go Fake Daemon 的测试服务端，以及 Python 测试里的假服务端；它们都不是正式 MW。

---

## 8. 建议的后续接线顺序

1. **P3-26 echo**：把 NoopRunner 换成 echo，确认 Event/TaskAck 语义。
2. **P4/P6 Control + DaemonHub**：把 `run_tasks` 的 Claim 结果变成 TaskAssign。
3. **Event 落库**：按 `(run_id, attempt, seq)` 幂等写入 `run_events`。
4. **Tool Bridge/Gateway**：接 ToolCallRequest/Result。
5. **Reconcile**：用 Hello.running_runs + lease_epoch 对比 DB。
6. **Checkpoint**：接 session/watermark。

---

## 9. 文件索引

| 内容 | 文件 |
|---|---|
| proto 契约 | `proto/middleware/runtime/v1/runtime.proto` |
| Go 生成 | `gen/middleware/runtime/v1/runtime.pb.go`、`runtime_grpc.pb.go` |
| Python 生成 | `harness/src/gen/middleware/runtime/v1/runtime_pb2.py`、`runtime_pb2_grpc.py` |
| Go Envelope 构造/校验 | `internal/protocol/envelope.go`、`validate.go` |
| Go Task 模型 | `internal/workflow/task.go` |
| Go Task 持久化 | `internal/workflow/postgres*.go` |
| Go Fake Daemon | `internal/testutil/fakedaemon/fakedaemon.go` |
| Python Channel | `harness/src/middleware_harness/channel/client.py` |
| Python Envelope | `harness/src/middleware_harness/channel/envelope.py` |
| Python ULID | `harness/src/middleware_harness/ids.py` |
| Python Run 生命周期 | `harness/src/middleware_harness/runtime/manager.py`、`run.py`、`events.py` |
| Python 入口 | `harness/src/middleware_harness/__main__.py` |
| 迁移 | `internal/db/migrations/0001_init.sql`、`0002_run_tasks_constraints.sql` |
