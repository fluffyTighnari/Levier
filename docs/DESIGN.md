# Middleware 智能体总控平台 — 总体设计蓝图

| 项目 | 内容 |
|---|---|
| 版本 | v0.2（评审稿，已并入 ADR-0001/0002） |
| 状态 | 评审中 |
| 日期 | 2026-11-03 |

> v0.2 变更：Workflow 改为 PostgreSQL RunTaskQueue；Harness 改为精简重写；MCP 连接归属独立 `mcp_runtime` 进程；事件主键改为 `(run_id, attempt, seq)`；Run 增加 `lease_epoch`；Session 使用复合 watermark。详见 `docs/adr/0001-workflow-engine.md`、`docs/adr/0002-harness.md`、`docs/adr/0004-mcp-ownership.md`、`docs/IMPLEMENTATION_PLAN_V2.md`。

---

## 0. 已锁定决策记录（ADR 摘要）

| # | 决策 | 结论 | 权衡 |
|---|---|---|---|
| ADR-1 | Daemon 通道 | gRPC 双向 stream，单连接承载全部消息，回调拓扑消除 | 放弃无连接简单性，换统一契约与背压能力 |
| ADR-2 | MCP 托管 | 独立 `mcp_runtime` 进程持有连接；Harness 通过 `McpRuntimeClient` 调用；Register 只存目录/ACL，Control 下发 `mcp_bindings` | 放弃 MCP 统一审计/共享，换 middleware 零连接负担；多语言 Harness 共享 Runtime 契约 |
| ADR-3 | 首期形态 | 单进程模块化单体，四平面 = Go package；排期以 V2 为准 | 放弃分布式弹性，换全链路先跑通 |
| ADR-4 | 编排边界 | Workflow 只编排 Run 级生命周期；ReAct/Plan&Execute 内层循环归 Daemon | 避免内层循环过 MQ 导致延迟与状态爆炸 |
| ADR-5 | 消息主干 | PostgreSQL RunTaskQueue = 唯一持久化骨架；Connection Plane = 边缘投递适配层 | 消除"双消息系统"分裂 |
| ADR-6 | Run 与连接解耦 | WS 断开 Run 继续；事件溯源 + 按 `(attempt, seq)` 补拉 | 以存储换可靠的断线重连 |
| ADR-7 | 幂等语义 | 全链路幂等键：`run_id / invocation_id`；tool 结果可缓存重放 | 以去重表换 at-least-once 下的安全重试 |
| ADR-8 | Run 归属 fencing | `runs.lease_epoch` 防止旧 daemon 写状态/事件；`run_tasks.lease_token` 防止旧 worker 写任务 | 多一层代次检查，换重排队后的正确性 |

---

## 1. 设计目标与非目标

### 1.1 目标

1. **Harness 复用**：多个业务 Agent（App）复用同一套 Python harness daemon，middleware 负责调度、路由、工具中介、可观测。
2. **控制面/数据面分离**：注册与目录（Register）与执行调度（Control）解耦。
3. **统一通道契约**：App 侧一套 HTTP+WS 契约，Daemon 侧一套 gRPC stream 契约，版本化演进。
4. **三种 Runtime 同协议**：normal / sandbox / longterm 仅是调度与放置策略差异，协议一致。
5. **复用既有资产**：调度、持久化、缓存、可观测全部构建在已有 Golang workflow 技术栈上。

### 1.2 非目标（v1 不做）

- Middleware 不托管 MCP server 生命周期（ADR-2）。
- 不建模跨 runtime 的子 Agent 调度；multi-agent 编排发生在 harness 内部，事件原样上报。
- 不做 LLM 出口代理（密钥直接下发 daemon，见 §13.2 风险标注）。
- 不做多租户强隔离的计费系统（仅记录 usage 事件）。
- 不做 GUI（注册走 HTTP API；GUI 为后期里程碑）。

---

## 2. 总体架构

### 2.1 架构图

```
                        ┌────────────────────────────────────────────┐
   App A ──HTTP/WS────► │                                            │
   App B ──HTTP/WS────► │            MIDDLEWARE (单二进制)            │
                        │                                            │
                        │  ┌──────────┐   ┌───────────────────────┐  │
                        │  │ Register │   │ Control               │  │
                        │  │  Plane   │◄──│  - Scheduler          │  │
                        │  │ (目录/    │   │  - Run 生命周期        │  │
                        │  │  注册)    │   │  - workflow 对接       │  │
                        │  └────┬─────┘   └──────┬────────────┬───┘  │
                        │       │                │            │      │
                        │  ┌────▼─────────────────▼──┐   ┌────▼────┐ │
                        │  │   Tool Gateway          │   │RunStore │ │
                        │  │  - 路由 local/global     │   │(事件溯源)│ │
                        │  │  - 幂等去重              │   └─────────┘ │
                        │  │  - global executor      │               │
                        │  └────┬────────────────────┘               │
                        │       │                                    │
                        │  ┌────▼─────────────┐                      │
                        │  │ Connection Plane │                      │
                        │  │  AppHub (WS)     │                      │
                        │  │  DaemonHub(gRPC) │                      │
                        │  └────┬─────────────┘                      │
                        └───────┼────────────────────────────────────┘
                                │ gRPC 双向 stream (RuntimeChannel)
              ┌─────────────────┼──────────────────┐
              ▼                 ▼                  ▼
       ┌────────────┐   ┌────────────┐     ┌────────────┐
       │ Daemon     │   │ Daemon     │     │ Daemon Pod │
       │ (normal)   │   │ (longterm) │     │ (sandbox)  │
       └─────┬──────┘   └─────┬──────┘     └─────┬──────┘
             ▼                ▼                  ▼
          LLM API          LLM API            LLM API
          MCP servers      MCP servers        (egress 受限)
```

**外部依赖**：既有 workflow 技术栈（MQ + 缓存 + 持久化存储）作为 RunStore 与调度队列的基础设施。

### 2.2 平面职责矩阵

| 平面 | 职责 | 明确不负责 |
|---|---|---|
| Register | App/Daemon/Tool/MCP 的注册、目录查询、ACL、TaskAssign 材料组装 | 执行任何调用 |
| Control | Run 状态机、调度决策、重试/取消/超时、daemon 容量管理、对账 | 知道工具语义 |
| Tool Gateway | 按名解析工具 → 路由 local/global；幂等去重；global 工具进程内执行 | 不执行不可信代码 |
| Connection | App WS Hub、Daemon gRPC Stream Hub、背压、消息投递 | 无业务逻辑 |
| RunStore | append-only 事件日志、Run 投影、Session/Checkpoint 持久化 | 无调度逻辑 |

### 2.3 关键边界纪律

1. **Workflow 只建模 Run**（queued→assigned→running→终态），ReAct 内层循环完全在 daemon 内。
2. **ephemeral 事件（token delta）不落库**，仅经 Connection Plane 转发；持久化只存消息级事件。
3. **回调不存在**：一切请求/响应都在既有长连接上以 `correlation_id` 闭环。
4. **daemon 无状态于 Run 之外**：normal runtime 的 daemon 在 run 结束后不保留任何 run 现场；现场在 RunStore。

---

## 3. 核心概念与数据模型

### 3.1 实体定义

| 实体 | 关键字段 | 说明 |
|---|---|---|
| **App** | app_id, name(唯一), api_key_hash, multiagent, status | 业务 Agent 服务 |
| **ToolDef** | tool_id, scope(local/global), app_id(local 时非空), name, description, schema_json, executor_ref(global), config_json(global), timeout_ms, acl | 工具定义；local 归 App 执行，global 归 Gateway 执行 |
| **McpServerDef** | mcp_id, name(唯一), transport(stdio/sse/streamable_http), config_json, auth_ref, allowed_tools_json | 仅存目录；连接由独立 `mcp_runtime` 建立 |
| **Daemon** | daemon_id, runtime_type, max_concurrency, labels, status, last_heartbeat_at | 注册中心视图；运行中实例 |
| **Run** | run_id, app_id, idempotency_key((app_id,key) 唯一), runtime, strategy, params, status, attempt, session_id?, timeouts, trace_id, assigned_daemon_id, lease_epoch, assigned_at | 一次执行的权威记录 |
| **RunTask** | task_id, run_id, type, status, available_at, attempt, max_attempts, lease_owner, lease_token, lease_expires_at | PostgreSQL RunTaskQueue；只承载 dispatch/retry/timeout/reconcile |
| **RunEvent** | (run_id, attempt, seq) 主键, source, type, payload_json, ts_ms | append-only；仅非 ephemeral；`source` = daemon/mw |
| **Session** | session_id, app_id, status, current_run_id, checkpoint_blob, event_attempt, event_watermark, expires_at | longterm 会话；复合 watermark |
| **ToolInvocation** | invocation_id 主键, run_id, tool_name, status, result_json, created_at, completed_at | 幂等去重 + 审计 |

### 3.2 存储表结构（DDL 示意）

```sql
CREATE TABLE apps (
  app_id        TEXT PRIMARY KEY,
  name          TEXT NOT NULL UNIQUE,
  api_key_hash  TEXT NOT NULL,
  multiagent    BOOLEAN NOT NULL DEFAULT FALSE,
  status        TEXT NOT NULL DEFAULT 'active',
  created_at    TIMESTAMP NOT NULL,
  updated_at    TIMESTAMP NOT NULL
);

CREATE TABLE tool_defs (
  tool_id      TEXT PRIMARY KEY,
  scope        TEXT NOT NULL CHECK (scope IN ('local','global')),
  app_id       TEXT REFERENCES apps(app_id),   -- scope=local 时必填
  name         TEXT NOT NULL,
  description  TEXT NOT NULL DEFAULT '',
  schema_json  TEXT NOT NULL,                  -- JSON Schema
  executor_ref TEXT,                           -- scope=global 时必填，如 "builtin:http"
  config_json  TEXT,                           -- global executor 配置，允许 ${secret:name}
  timeout_ms   INTEGER NOT NULL DEFAULT 60000,
  acl_json     TEXT,                           -- global 工具的 app 白名单，NULL=全部
  version      INTEGER NOT NULL DEFAULT 1,
  UNIQUE (scope, app_id, name)
);

CREATE TABLE mcp_server_defs (
  mcp_id     TEXT PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE,
  transport  TEXT NOT NULL CHECK (transport IN ('stdio','sse','streamable_http')),
  config_json TEXT NOT NULL,    -- command/args/env 或 url/headers（含 auth_ref 占位）
  allowed_tools_json TEXT,      -- NULL = 全部；非空 = 工具白名单
  created_at TIMESTAMP NOT NULL
);

CREATE TABLE daemons (
  daemon_id        TEXT PRIMARY KEY,
  runtime_type     TEXT NOT NULL CHECK (runtime_type IN ('normal','sandbox','longterm')),
  max_concurrency  INTEGER NOT NULL DEFAULT 4,
  labels_json      TEXT NOT NULL DEFAULT '{}',
  status           TEXT NOT NULL DEFAULT 'offline', -- online/offline/draining
  last_heartbeat_at TIMESTAMP
);

CREATE TABLE runs (
  run_id              TEXT PRIMARY KEY,
  app_id              TEXT NOT NULL REFERENCES apps(app_id),
  idempotency_key     TEXT,                   -- App 侧防重复提交
  request_hash        TEXT,                   -- 同 key 不同 payload 检测
  runtime             TEXT NOT NULL,
  strategy            TEXT NOT NULL,
  params_json         TEXT NOT NULL DEFAULT '{}',
  status              TEXT NOT NULL,          -- 见 §5.1 状态机
  attempt             INTEGER NOT NULL DEFAULT 0,
  session_id          TEXT,
  model_json          TEXT NOT NULL DEFAULT '{}',
  run_timeout_ms      INTEGER NOT NULL DEFAULT 1800000,
  trace_id            TEXT NOT NULL,
  assigned_daemon_id  TEXT,                   -- 当前归属 daemon
  lease_epoch         BIGINT NOT NULL DEFAULT 0, -- 防旧 daemon 写状态/事件
  assigned_at         TIMESTAMP,
  last_event_attempt  INTEGER NOT NULL DEFAULT 0, -- 已落库的最大事件 attempt
  last_event_seq      BIGINT NOT NULL DEFAULT 0,  -- 同 attempt 下最大 seq
  created_at          TIMESTAMP NOT NULL,
  updated_at          TIMESTAMP NOT NULL,
  UNIQUE (app_id, idempotency_key)
);
CREATE INDEX idx_runs_status ON runs(status);
CREATE INDEX idx_runs_daemon ON runs(assigned_daemon_id, status);

CREATE TABLE run_events (
  run_id      TEXT NOT NULL REFERENCES runs(run_id),
  attempt     INTEGER NOT NULL,              -- Run 执行 attempt
  seq         BIGINT NOT NULL,               -- 仅持久化事件递增
  source      TEXT NOT NULL CHECK (source IN ('daemon','mw')),
  type        TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  ts_ms       BIGINT NOT NULL,
  PRIMARY KEY (run_id, attempt, seq)
);

CREATE TABLE sessions (
  session_id      TEXT PRIMARY KEY,
  app_id          TEXT NOT NULL REFERENCES apps(app_id),
  status          TEXT NOT NULL,             -- active/paused/expired
  current_run_id  TEXT,
  checkpoint_blob BYTEA,
  event_attempt   INTEGER NOT NULL DEFAULT 0,
  event_watermark BIGINT NOT NULL DEFAULT 0,
  created_at      TIMESTAMP NOT NULL,
  updated_at      TIMESTAMP NOT NULL,
  expires_at      TIMESTAMP
);

-- RunTaskQueue：PostgreSQL 持久化任务，仅承载 Run 级调度
CREATE TABLE run_tasks (
  task_id          TEXT PRIMARY KEY,
  run_id           TEXT NOT NULL REFERENCES runs(run_id),
  task_type        TEXT NOT NULL CHECK (task_type IN
                     ('RUN_DISPATCH','RUN_RETRY','RUN_TIMEOUT','RUN_RECONCILE')),
  status           TEXT NOT NULL CHECK (status IN
                     ('PENDING','LEASED','DONE','DEAD','CANCELLED')),
  available_at     TIMESTAMPTZ NOT NULL,
  attempt          INTEGER NOT NULL DEFAULT 0,
  max_attempts     INTEGER NOT NULL DEFAULT 5,
  lease_owner      TEXT NOT NULL DEFAULT '',
  lease_token      BIGINT NOT NULL DEFAULT 0,
  lease_expires_at TIMESTAMPTZ,
  last_error       TEXT NOT NULL DEFAULT '',
  payload          JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at       TIMESTAMPTZ NOT NULL,
  updated_at       TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX uq_run_tasks_active
  ON run_tasks(run_id, task_type)
  WHERE status IN ('PENDING','LEASED');
CREATE INDEX idx_run_tasks_ready
  ON run_tasks(status, available_at, created_at);
CREATE INDEX idx_run_tasks_lease
  ON run_tasks(status, lease_expires_at)
  WHERE status = 'LEASED';

CREATE TABLE tool_invocations (
  invocation_id TEXT PRIMARY KEY,            -- 幂等键
  run_id        TEXT NOT NULL,
  tool_name     TEXT NOT NULL,
  status        TEXT NOT NULL,               -- in_flight/completed/failed
  result_json   TEXT,
  created_at    TIMESTAMP NOT NULL,
  completed_at  TIMESTAMP
);
CREATE INDEX idx_toolinv_run ON tool_invocations(run_id);
```

### 3.3 缓存模型（v1 可选，非必需）

> v1 默认不引入 Redis；以下键仅作为后续缓存层的设计参考。工具幂等以 PostgreSQL 为准，缓存只做快速路径。

| 键 | 内容 | 用途 |
|---|---|---|
| `run:state:{run_id}` | Run 投影热拷贝 | 状态查询、WS fanout |
| `tool:result:{invocation_id}` | 已完成工具结果（TTL 1h） | 幂等去重快速路径（DB 兜底） |
| `daemon:cap:{daemon_id}` | running_count 计数 | 调度过滤 |
| `mcp:def:{name}` / `tool:def:{...}` | 目录缓存（失效：注册写操作） | TaskAssign 组装加速 |

ephemeral 事件**不进缓存**；断线重连补拉走 `run_events` 表（§10.3）。

---

## 4. 协议契约

### 4.1 Daemon ↔ Middleware：RuntimeChannel（gRPC 双向 stream）

`proto/middleware/runtime/v1/runtime.proto`：

```proto
syntax = "proto3";
package middleware.runtime.v1;

import "google/protobuf/struct.proto";

service RuntimeService {
  // Daemon 启动后建立的唯一长连接，双向承载全部控制与数据消息。
  rpc Channel(stream Envelope) returns (stream Envelope);
}

// ---------- 信封 ----------
message Envelope {
  string msg_id = 1;          // 消息唯一 ID（ULID）
  string correlation_id = 2;  // 请求-响应关联：响应方回填请求方 msg_id
  string run_id = 3;          // 归属 run（Hello/Heartbeat 为空）
  int64  ts_ms = 4;
  int32  attempt = 5;         // Run 执行 attempt（TaskAssign 下发；Event 回传）
  int64  lease_epoch = 6;     // Run 归属代次（防旧 daemon 写状态/事件）
  oneof payload {
    Hello           hello             = 10;  // up
    Heartbeat       heartbeat         = 11;  // up
    TaskAssign      task_assign       = 12;  // down
    TaskAck         task_ack          = 13;  // up
    Event           event             = 14;  // up
    ToolCallRequest tool_call_request = 15;  // up
    ToolCallResult  tool_call_result  = 16;  // down
    Cancel          cancel            = 17;  // down
    Reconcile       reconcile         = 18;  // down
    Checkpoint      checkpoint        = 19;  // up
    ErrorReport     error             = 20;  // both
  }
}

enum RuntimeType {
  RUNTIME_TYPE_UNSPECIFIED = 0;
  NORMAL   = 1;
  SANDBOX  = 2;
  LONGTERM = 3;
}

// ---------- 注册与心跳 ----------
message Hello {                       // 连接建立后 daemon 发送的第一条消息
  string daemon_id = 1;
  string auth_token = 2;              // 注册令牌（每 daemon 预分配）
  RuntimeType runtime_type = 3;
  int32 max_concurrent_runs = 4;
  repeated string supported_strategies = 5;  // v1: ["react","echo"]；plan_execute 保留后续
  string version = 6;                 // harness 版本
  map<string,string> labels = 7;      // 如 {"pool":"sandbox","region":"cn1"}
  repeated RunningRun running_runs = 8;// 重连对账：daemon 侧仍在执行的 run
}

message RunningRun {
  string run_id = 1;
  int64  lease_epoch = 2;
}

message Reconcile {                   // MW 对账结果：这些 run MW 认为已死，请取消
  repeated string cancel_run_ids = 1;
}

message Heartbeat {                   // 每 10s；30s 未达判 dead
  int32 running_count = 1;
  double cpu_load = 2;
  double mem_mb = 3;
}

// ---------- 任务下发 ----------
message TaskAssign {
  string run_id = 1;
  string app_id = 2;
  string strategy = 3;                       // v1: "react" | "echo"；plan_execute 保留后续
  google.protobuf.Struct strategy_params = 4;
  repeated ChatMessage input = 5;
  ModelProfile model = 6;
  repeated ToolManifest tools = 7;           // 已合并去重的工具清单（local 遮蔽同名 global）
  repeated McpBinding mcp_bindings = 8;      // 已解析的 MCP 连接描述（含凭据，见 §13.2）
  TimeoutProfile timeouts = 9;
  string session_id = 10;                    // longterm：空 = 新会话
  Checkpoint resume_from = 11;               // longterm：恢复快照（可空）
  string trace_context = 12;                 // W3C traceparent
  // attempt / lease_epoch 使用 Envelope 字段；不在此重复
}

message ChatMessage {
  string role = 1;
  string content = 2;                 // v1 纯文本；多模态后续扩展 parts
  string name = 3;
}

message ModelProfile {
  string provider = 1;                // "anthropic" | "openai" | ...
  string model = 2;
  string api_key = 3;                 // v1 由 MW 解析注入（风险见 §13.2）
  string base_url = 4;
  map<string,string> params = 5;      // temperature 等
}

enum ToolScope {
  TOOL_SCOPE_UNSPECIFIED = 0;
  LOCAL  = 1;                          // 由 App 执行，MW 中介
  GLOBAL = 2;                          // 由 MW Tool Gateway 执行
}

message ToolManifest {
  string name = 1;
  string description = 2;
  string input_schema_json = 3;       // JSON Schema
  ToolScope scope = 4;                // 仅供 daemon 观测/日志，路由决策在 MW
}

message McpBinding {
  string server_name = 1;
  string transport = 2;               // "stdio" | "sse" | "streamable_http"
  string command = 3;                 // stdio
  repeated string args = 4;
  map<string,string> env = 5;
  string url = 6;                     // sse / streamable_http
  map<string,string> headers = 7;
  repeated string allowed_tools = 8;  // 空 = 全部；非空 = 白名单
  string namespace = 9;               // 默认 mcp__<server_name>__
}

message TimeoutProfile {
  int64 run_timeout_ms = 1;
  int64 tool_timeout_ms = 2;
}

message TaskAck {                     // daemon 5s 内必须应答，超时重排队
  bool accepted = 1;
  string reason = 2;                  // 拒绝原因（如容量不足竞态）
}

// ---------- 事件 ----------
enum EventType {
  EVENT_TYPE_UNSPECIFIED = 0;
  RUN_STATUS        = 1;   // data: {"status":"running",...}
  MESSAGE_DELTA     = 2;   // ephemeral，token 增量
  MESSAGE_COMPLETED = 3;   // 完整消息（含 tool_use blocks）
  TOOL_CALL_BEGIN   = 4;
  TOOL_CALL_END     = 5;
  USAGE             = 6;   // data: {"input_tokens":N,"output_tokens":M,...}
  ERROR             = 7;
  CHECKPOINT_SAVED  = 8;
}

message Event {
  int64 seq = 1;                      // 仅持久化事件递增；ephemeral 固定 0
  EventType type = 2;
  bool ephemeral = 3;                 // true = 不落库，仅转发
  google.protobuf.Struct data = 4;
  string source = 5;                  // "daemon" | "mw"
  // attempt / lease_epoch 使用 Envelope 字段；不在此重复
}

// ---------- 工具调用闭环 ----------
message ToolCallRequest {
  string invocation_id = 1;           // ULID，幂等键（daemon 生成）
  string tool_name = 2;
  string arguments_json = 3;
  int64 timeout_ms = 4;
}

message ToolCallResult {
  string invocation_id = 1;
  bool ok = 2;
  string result_json = 3;             // ok=true 时的内容（harness 协议契约对齐）
  ErrorInfo error = 4;                // ok=false
}

message ErrorInfo {
  string code = 1;                    // 见 §4.5 错误模型
  string message = 2;
  bool retryable = 3;
}

// ---------- 取消与检查点 ----------
message Cancel {
  string reason = 1;                  // "user" | "timeout" | "reconcile" | "daemon_lost"
}

message Checkpoint {                  // longterm：daemon 主动上报 + TaskAssign 下发复用
  string session_id = 1;
  int64 event_seq_watermark = 2;      // 快照覆盖到的事件 seq
  bytes state = 3;                    // 对 MW 不透明（harness 自有格式）
  // 快照覆盖的 attempt 使用 Envelope.attempt；与 seq 组成复合 watermark
}

message ErrorReport {
  string code = 1;
  string message = 2;
}
```

**通道纪律**：

1. 连接建立后 daemon 必须在 3s 内发 `Hello`，否则 MW 关闭连接。
2. 所有请求/响应成对消息用 `correlation_id = 请求方 msg_id` 闭环。
3. `Event.seq` 仅对持久化事件递增，由 daemon 单写者分配；MW 按 `(run_id, attempt, seq)` 幂等落库；乱序到达时 MW 缓冲重排窗口 = 16，超窗记 `SEQ_GAP` 并继续（不回退）。
4. MW 校验 `Envelope.lease_epoch == runs.lease_epoch`；不匹配的旧 daemon 事件丢弃并计数 `stale_daemon_events`。
5. `ToolCallRequest` 是**长等待**消息：MW 侧挂起直到 ToolCallResult 或超时，期间 stream 正常承载其他消息（多路复用，互不阻塞）。
6. 单向消息（Event/Heartbeat/Checkpoint）不需要 ack；可靠性由 seq 幂等 + 重连对账保证。

### 4.2 App ↔ Middleware：HTTP API

统一前缀 `/api/v1`，认证 `Authorization: Bearer <app_api_key>`。

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/admin/apps` | 管理员 bootstrap：创建 App 并一次性返回 api_key（见 D7） |
| POST | `/apps` | 更新自身（按 name upsert）。Body 含 tools(local)、mcp_refs、multiagent |
| GET | `/apps/{app_id}` | 查询自身注册信息 |
| POST | `/runs` | 发起 run。支持 `Idempotency-Key` 头防重复 |
| GET | `/runs/{run_id}` | Run 状态查询 |
| GET | `/runs/{run_id}/events?after_attempt=&after_seq=&limit=` | 事件补拉/回放（仅非 ephemeral） |
| POST | `/runs/{run_id}/cancel` | 取消 |
| GET | `/sessions/{session_id}` | longterm 会话信息 |
| GET | `/sessions/{session_id}/events` | 跨 run 的会话事件流 |

`POST /runs` 请求体：

```json
{
  "app_id": "app_01H...",
  "runtime": "normal | sandbox | longterm",
  "strategy": { "name": "react", "params": { "max_steps": 32 } },
  "input": [ { "role": "user", "content": "..." } ],
  "session_id": "可选，longterm 续跑",
  "model": { "provider": "anthropic", "model": "claude-sonnet-4-5", "params": { "temperature": "0.7" } },
  "timeouts": { "run_ms": 1800000, "tool_ms": 60000 }
}
```

响应：`{ "run_id": "...", "status": "queued", "ws_url": "/api/v1/ws", "ws_token": "..." }`

- `ws_token`：短期令牌（TTL 10min，绑定 app_id），用于 WS 鉴权；过期可凭 api_key 重新获取。
- 错误响应统一：`{ "error": { "code": "...", "message": "..." } }`。

### 4.3 App ↔ Middleware：WebSocket 协议

连接：`GET /api/v1/ws?app_id=&token=`。JSON 文本帧。

**Client → Server**：

```json
{ "type": "subscribe",         "run_id": "...", "last_attempt": 0, "last_seq": 0 }
{ "type": "local_tool_result", "correlation_id": "<原请求msg_id>", "invocation_id": "...",
  "ok": true, "result_json": "..." }
{ "type": "ping" }
```

**Server → Client**：

```json
{ "type": "event",          "run_id": "...", "seq": 42, "event_type": "MESSAGE_COMPLETED", "data": { } }
{ "type": "local_tool_call","msg_id": "...", "invocation_id": "...", "tool_name": "...",
  "arguments_json": "...", "timeout_ms": 60000 }
{ "type": "run_completed",  "run_id": "...", "status": "completed" }
{ "type": "pong" }
```

纪律：

1. `subscribe` 带 `last_attempt/last_seq` 时，MW 从 `run_events` 补推 `(last_attempt,last_seq)` 之后的持久化事件；ephemeral 事件不补（断线期间 token 流丢失由最终 `MESSAGE_COMPLETED` 兜底）。
2. App 侧 `local_tool_result` 必须按 `invocation_id` 幂等：同一调用重复投递（重连重发）时返回缓存结果而非重复执行。
3. App 断开不影响 Run 运行（ADR-6）；其挂起的 local tool 调用立即失败 `APP_UNREACHABLE`（retryable），由 Harness 策略层决定重试或降级。

### 4.4 版本化策略

- proto 包路径携带主版本：`middleware.runtime.v1`；破坏性变更升 `v2`，同包内只做新增字段/新增消息。
- WS 协议以 `{"type":"hello","protocol":1}` 握手（v1 可省略，引入 v2 时补）。
- HTTP API 以路径 `/api/v1` 版本化。

### 4.5 错误模型

统一错误码（`ErrorInfo.code` / HTTP error.code）：

| 分类 | code | retryable |
|---|---|---|
| 调度 | `NO_DAEMON_AVAILABLE` `QUEUE_OVERFLOW` | 是 / 否 |
| 工具 | `TOOL_NOT_FOUND` `TOOL_TIMEOUT` `TOOL_DENIED` `TOOL_EXEC_ERROR` `APP_UNREACHABLE` | 否/是/否/否/是 |
| 运行 | `RUN_TIMEOUT` `RUN_CANCELLED` `DAEMON_LOST` `MAX_ATTEMPTS_EXCEEDED` | 否 |
| 协议 | `BAD_ENVELOPE` `SEQ_GAP` `AUTH_FAILED` `TOKEN_EXPIRED` | 否 |
| 通用 | `INTERNAL` | 视情况 |

---

## 5. Run 生命周期

### 5.1 状态机

```
                 ┌────────┐  调度成功+TaskAck   ┌──────────┐  daemon 首个 RUN_STATUS   ┌─────────┐
   创建 ───────► │ queued │ ──────────────────► │ assigned │ ────────────────────────► │ running │
                 └───┬────┘                     └────┬─────┘                           └────┬────┘
                     │                               │                                      │
                     │ daemon 死/Ack 超时(attempt++) │                                      │
                     ◄───────────────────────────────┘                                      │
                     │                                                                      │
        ┌────────────┴──────────────────────────────────────────────────────────────────────┤
        ▼ 终态（持久化，不可再迁移）                                                          ▼
   completed / failed / cancelled / timeout                                        （事件流驱动）
```

- `attempt` 上限：normal=2，longterm=3（从 checkpoint 恢复）；超限 → `failed(MAX_ATTEMPTS_EXCEEDED)`。
- 取消可发生在任意非终态：queued 直接标记 cancelled（消费者出队时跳过）；assigned/running 下发 `Cancel`。

### 5.2 完整时序（happy path，含一次 local 工具调用）

```
App          MW-HTTP      Control      Workflow     DaemonHub     Daemon        Gateway       AppHub/WS
 │  POST /runs  │            │             │            │            │              │             │
 │─────────────►│ 建 Run(queued)          │            │            │              │             │
 │              │───────────►│ publish run task        │            │              │             │
 │  {run_id,ws_token}        │────────────►│            │            │              │             │
 │◄─────────────│            │             │            │            │              │             │
 │ WS connect+subscribe(last_attempt=0,last_seq=0) ───────────────────────────────────────────────►│
 │              │            │  consume    │            │            │              │             │
 │              │            │◄────────────│            │            │              │             │
 │              │            │ 选 daemon(类型+容量+labels)            │              │             │
 │              │            │ 组装 TaskAssign(Register 解析 tool/mcp/model 凭据)     │             │
 │              │            │──────────────────────────►│ TaskAssign │             │             │
 │              │            │            │            │───────────►│             │             │
 │              │            │            │            │◄── TaskAck │             │             │
 │              │            │ status=assigned/running  │            │             │             │
 │              │            │            │            │  Event(RUN_STATUS/MESSAGE_*)             │
 │  ◄─────────────────────────── event fanout ───────────────────────────────────────────────────│
 │              │            │            │            │  ToolCallRequest(inv_id)   │             │
 │              │            │─────────────────────────────────────────────────────►│             │
 │              │            │            │            │            │ 解析 scope=local            │
 │              │            │            │            │            │              │ local_tool_call
 │              │            │            │            │            │              │────────────►│
 │ local_tool_result(inv_id) ────────────────────────────────────────────────────────────────────│
 │─────────────►│            │            │            │◄ ToolCallResult            │◄────────────│
 │              │            │            │            │───────────►│ (循环继续)     │             │
 │              │            │            │            │  Event(MESSAGE_COMPLETED/USAGE/RUN_STATUS=completed)
 │  ◄──────────────────────── run_completed ─────────────────────────────────────────────────────│
 │              │            │ workflow 落终态          │            │              │             │
```

### 5.3 工具调用三分支

| 分支 | 链路 | 超时归属 | 备注 |
|---|---|---|---|
| local | daemon → MW Gateway → App WS → App 执行 → 原路闭环 | Gateway 计时（默认 60s，ToolDef 可配） | App 侧幂等（§4.3） |
| global | daemon → MW Gateway → 进程内 executor → 闭环 | Gateway 计时（默认 30s） | v1 executor：`builtin:http`（HTTP 请求模板）、`builtin:webhook`；**不执行不可信代码** |
| MCP | 独立 `mcp_runtime` 直连 | Runtime 自治 | Harness 通过本地 RPC 调用；配置来自 TaskAssign.mcp_bindings；总控不持有连接 |

工具解析规则：按 `tool_name` 查 ToolDef；**local 遮蔽同名 global**（TaskAssign 组装时已按此规则合并，daemon 无感知）。

### 5.4 取消、超时、断线重连

- **取消**：App `POST /cancel` → Control 标记+下发 `Cancel` → daemon 中止 LLM 循环与挂起的 ToolCall（挂起的 local 调用向 AppHub 发取消错误，幂等键下 App 若已执行可安全忽略）。
- **超时层级**：`run_timeout`（Control 用 workflow 延迟消息强制终态）> 策略 step 超时（daemon 内部）> `tool_timeout`（Gateway）。
- **App 断线**：Run 继续；重连 `subscribe(last_attempt, last_seq)` 补拉。
- **Daemon 断线**：stream 关闭或 30s 无心跳 → daemon 标 offline → 其 running runs attempt++ 重排队（normal 从头重跑；longterm 从最近 Checkpoint 恢复）；daemon 重连 `Hello.running_runs` 触发对账（§12.2）。
- **MW 重启**：workflow 持久化保证 queued/assigned 任务不丢；running runs 依赖 daemon 重连对账收敛。

---

## 6. Control Plane 设计

### 6.1 与 workflow 的分工

| workflow 负责 | Control 负责 |
|---|---|
| run task 的持久化队列、重投、延迟消息（run 超时定时） | 调度决策、TaskAssign 组装与下发、状态机迁移、对账 |
| 可观测基础设施 | 领域事件落 RunStore |

集成方式：Control 内嵌 workflow 的 consumer；`Submit(run)` = 写 runs 表（queued）+ publish task{run_id}。消费者幂等：出队后先查状态，非 queued 直接跳过。

### 6.2 调度策略

1. **过滤**：runtime_type 匹配；`running_count < max_concurrent_runs`；labels 匹配（sandbox 池、region）；strategy ∈ daemon.supported_strategies。
2. **排序**：least-loaded（running/max 最小）；longterm 会话优先**粘滞**到上次执行的 daemon（快照局部性），失败则任意 longterm daemon + 从 RunStore 加载 checkpoint。
3. **下发与确认**：TaskAssign 后 5s 内等 TaskAck；拒绝/超时 → 重新过滤下一候选；无候选 → task 延迟 2s 重投（指数退避，上限 5min，超过 run_timeout 直接终态）。

### 6.3 容量与心跳

- 容量计数 `daemon:cap:{id}` 随 TaskAssign/TaskAck/完成事件增减，心跳携带 `running_count` 做对账校准（防计数漂移）。
- draining：运维接口标记 daemon draining → 不再分配新 run，存量跑完后下线。

---

## 7. Tool Gateway 设计

### 7.1 路由与执行

```
ToolCallRequest(invocation_id, tool_name)
   │
   ├─ 幂等检查（缓存→DB）─命中→ 直接返回缓存结果
   │
   ├─ 解析 ToolDef：local 遮蔽 global；ACL 校验（global 白名单）
   │
   ├─ local ─► AppHub.CallLocalTool(ctx, app_id, req) ─► 等待 local_tool_result / 超时
   │
   └─ global ─► Executor 注册表查 executor_ref ─► 进程内执行 ─► 结果
```

### 7.2 幂等与 single-flight

- 重复 `ToolCallRequest`（同 invocation_id）：已完成 → 返回缓存；in_flight → 挂到同一 future 等待（single-flight），不重复下发。
- 结果写入：完成时同事务更新 `tool_invocations` + 缓存。
- 审计：`tool_invocations` 全量留存，供观测与计费。

### 7.3 Global Executor v1 清单

| executor_ref | 能力 | 配置（ToolDef.config_json） |
|---|---|---|
| `builtin:http` | 发起 HTTP 请求（method/url/headers/body 模板，超时/重试） | `method`、`url`、`headers`、`body`、`max_response_bytes`、`retries`、`retry_backoff_ms` |
| `builtin:webhook` | 向预注册 URL POST 并等待响应 | URL 在 ToolDef 配置，App 只传 payload |

`config_json` 在工具解析时解析 `${secret:name}`，明文不落库。`builtin:http` 的 `url`、`headers`、`body` 使用 Go `text/template`，数据为：

```json
{
  "args": {"q": "..."},
  "args_json": "{\"q\":\"...\"}",
  "run_id": "...",
  "app_id": "...",
  "invocation_id": "...",
  "tool_name": "..."
}
```

可用函数：`json`（序列化）、`urlquery`（URL 转义）。模板缺失键直接报错，不渲染空值。

v2 候选：`builtin:expression`（表达式求值）、沙箱化脚本执行（复用 sandbox runtime 思路）。

---

## 8. Connection Plane 设计

### 8.1 DaemonHub

- `map[daemon_id]*conn`，conn 内含出站缓冲 channel（容量 256）+ 单写 goroutine（gRPC stream 写非并发安全）。
- **背压**：缓冲满时 —— ephemeral Event 丢弃（计数器+日志）；非 ephemeral 阻塞 100ms 后判 slow consumer → 停止向其分配新 run，持续 5s 则断连（触发 daemon 重连对账）。
- 入站：goroutine 循环 Recv，按 payload 分发（Hello/Heartbeat→Control 注册模块；Event→RunStore+AppHub；ToolCallRequest→Gateway；TaskAck→Control）。

### 8.2 AppHub

- `map[app_id]set[wsConn]`；事件 fanout 给该 app 全部连接。
- local tool 挂起表：`map[invocation_id]chan result`，超时由 Gateway 控制。
- **背压**：WS 写缓冲满 → 丢 ephemeral；持久化事件不丢（App 可用 HTTP events API 补拉）；持续慢消费者强制断连。

### 8.3 流量分类纪律

| 类别 | 例子 | 丢失策略 |
|---|---|---|
| 控制消息 | TaskAssign/Cancel/ToolCall* | 绝不主动丢；慢则断连重连对账 |
| 持久化事件 | MESSAGE_COMPLETED 等 | 已落库，可补拉，不阻塞主链路 |
| ephemeral | MESSAGE_DELTA | 可丢，计数监控 |

---

## 9. Register Plane 设计

### 9.1 注册模型

- **App 注册**：`POST /apps` 幂等 upsert（按 name）；携带 local ToolDef 列表、mcp_refs（按 name 引用 McpServerDef）、multiagent 标志。返回 api_key（仅注册时明文返回一次，库存 hash）。
- **Tool 注册**：global 工具独立管理接口（运维侧）：`POST /admin/tools`；local 工具随 App 注册全量替换（版本 +1）。
- **MCP 注册**：`POST /admin/mcp`；`config_json` 中凭据以 `auth_ref` 占位符（如 `${secret:github_token}`）存储；`allowed_tools_json` 控制工具白名单。连接由独立 `mcp_runtime` 建立，Register 不持有连接。
- **Daemon 注册**：预分配模式 —— 运维在配置表中创建 daemon_id + auth_token；daemon 启动持令牌 Hello。**不支持**运行时自助注册（v1 简化，避免注册风暴与伪造）。

### 9.2 TaskAssign 材料组装（Register 提供给 Control 的查询）

`AssembleTaskMaterials(app_id) → {tools[], mcp_bindings[], model_credentials}`：

1. local tools = ToolDef(scope=local, app_id)；global tools = ACL 通过的 ToolDef(scope=global)；local 遮蔽同名 global。
2. mcp_refs → McpServerDef → 合并 `allowed_tools_json` → 生成 `McpBinding`（含 namespace）→ 解析 `${secret:*}` 占位（§13.2）。
3. model：App 请求中的 model profile + MW 配置中的 provider 凭据。
4. MCP 连接不在本函数中建立；只产出连接描述。

---

## 10. RunStore 与事件溯源

### 10.1 事件类型与持久化规则

| EventType | ephemeral | 落库 | 说明 |
|---|---|---|---|
| RUN_STATUS | 否 | ✅ | 驱动 runs 投影状态迁移 |
| MESSAGE_DELTA | 是 | ❌ | 仅转发 |
| MESSAGE_COMPLETED | 否 | ✅ | 对话历史主体 |
| TOOL_CALL_BEGIN/END | 否 | ✅ | 含 invocation_id 关联 tool_invocations |
| USAGE | 否 | ✅ | 计费/统计原料 |
| ERROR | 否 | ✅ | |
| CHECKPOINT_SAVED | 否 | ✅ | watermark 记录 |

### 10.2 写入路径

`AppendEvent`：单事务内 —— 校验 `lease_epoch` 与 `runs.lease_epoch`；INSERT `run_events (run_id, attempt, seq, source)` 冲突则忽略（幂等）+ 更新 runs 投影（`last_event_*`，不覆盖 Control 状态）+ 更新缓存热拷贝。ephemeral=true 直接走 AppHub，不进本路径。

> Daemon 事件 seq 使用 `[1, 2^62)`；MW 生成的 timeout/cancel/daemon_lost 事件使用保留区间 `[2^62, 2^63)`，`source='mw'`，避免与 daemon seq 冲突。

### 10.3 重连补拉

App `subscribe(last_attempt, last_seq)` → MW 查 `run_events WHERE run_id=? AND (attempt, seq) > (?, ?)`（上限 1000 条/批）按序补推；补推完毕发 `{"type":"caught_up"}` 后进入实时流。

---

## 11. 三种 Runtime 实现策略

### 11.1 normal

常驻 daemon（`max_concurrent_runs` 默认 4），直接调度。Run 间隔离依赖 harness 的 run 级上下文隔离（要求 harness 无跨 run 可变全局状态 —— 需在 daemon 适配层验收）。

### 11.2 longterm

- Session 实体（§3.1）；`POST /runs {runtime:"longterm", session_id?}`：无 session_id 创建新会话。
- Checkpoint 触发：Harness 策略 —— 每 20 条持久化事件或 60s，以及收到 Cancel 前；上报 `Checkpoint` → MW 原子更新 `sessions.checkpoint_blob` 与 `(event_attempt, event_watermark)`。
- 续跑：调度器注入 `resume_from`；优先粘滞调度（§6.2）。
- 会话 API：`GET /sessions/{id}`、`GET /sessions/{id}/events`（跨 run 聚合）；过期 GC：`expires_at` 到期标记 expired，blob 保留 7 天后清除。

### 11.3 sandbox

- 镜像：harness daemon 基础镜像（Python + 依赖预装）。
- 预热池：k8s Deployment 维持 N 个空闲 pod（v1 手工定 N，后续 sandbox-controller 自动伸缩）；每 pod = 一个 daemon 实例，`max_concurrent_runs=1`，labels `{"pool":"sandbox"}`。
- 网络：NetworkPolicy egress 仅允许 → middleware（gRPC）+ LLM API 端点；拒绝集群内其他地址与任意互联网。
- 生命周期：pod 每执行 K 个 run（默认 1）后自杀式重建（draining → 退出 → Deployment 拉起新 pod），保证不可信残留不累积。
- ⚠️ 已知限制：不可信代码可通过 LLM API 出口进行数据外渗的理论通道存在；v2 评估 LLM 出口收敛至 MW 代理（见 §17）。

---

## 12. 可靠性与容错

### 12.1 故障矩阵

| 故障 | 检测 | 系统行为 |
|---|---|---|
| App WS 断开 | conn close | Run 继续；事件落库；重连按 seq 补拉 |
| App 重连后重复 tool 投递 | invocation_id | App 侧幂等返回缓存结果 |
| Daemon stream 断开/心跳死 | 30s | daemon offline；其 running runs attempt++ 重排队（normal 重跑 / longterm 从 checkpoint 恢复） |
| Daemon 拒绝 TaskAck | Ack(accepted=false) | 调度下一候选 daemon |
| MW 进程重启 | — | queued/assigned 由 workflow 重投；running 由 daemon 重连对账收敛 |
| Tool 调用超时 | Gateway 计时器 | 返回 TOOL_TIMEOUT(retryable) 给 daemon，策略层决策 |
| 重复 ToolCallRequest | invocation_id 去重 | 缓存返回 / single-flight |
| TaskQueue 不可用 | Enqueue 失败 | `POST /runs` 返回 503(QUEUE_UNAVAILABLE)，App 重试（Idempotency-Key 防重） |
| run_events 写入冲突 | (run_id, attempt, seq) PK | 幂等忽略，继续 |
| 旧 daemon 事件 | lease_epoch 不匹配 | 丢弃并计数 `stale_daemon_events` |

### 12.2 对账协议（Reconciliation）

Daemon 重连 `Hello.running_runs`（`run_id + lease_epoch`）≠ MW 视角时：

| MW 认为 | Daemon 实际 | 动作 |
|---|---|---|
| running 于该 daemon | 不存在 | attempt++ 重排队（或 failed） |
| 终态 | 仍在跑 | 下发 `Reconcile{cancel_run_ids}` |
| running 于该 daemon | 仍在跑，epoch 一致 | 一致，继续 |
| running 于该 daemon | 仍在跑，epoch 不匹配 | 取消旧执行，按新 epoch 处理 |

### 12.3 超时参数表

| 参数 | 默认 | 位置 |
|---|---|---|
| Hello 首包 | 3s | DaemonHub |
| TaskAck | 5s | Control |
| 心跳间隔 / 死亡判定 | 10s / 30s | DaemonHub |
| tool local / global | 60s / 30s（ToolDef 可配） | Gateway |
| run | 30min（longterm 上限 24h） | Control（RunTaskQueue `RUN_TIMEOUT`） |
| 无候选重投退避 | 2s 起指数，上限 5min | Control |
| ws_token TTL | 10min | HTTP 层 |

---

## 13. 安全设计

### 13.1 认证与鉴权

| 主体 | 机制 |
|---|---|
| App → MW | API key（Bearer，库存 SHA-256 hash）；WS 用短期 ws_token |
| Daemon → MW | 预分配 daemon_id + auth_token（Hello 校验） |
| 工具 ACL | local：仅属主 App；global：ToolDef.acl_json 白名单（NULL=全量） |
| MCP | App 注册时按 name 引用；未引用不下发 `mcp_bindings`；`allowed_tools` 二次过滤 |

### 13.2 密钥管理

- LLM api_key、MCP 凭据：存储于 MW 配置/密钥介质，注册表只存 `auth_ref`；TaskAssign 组装时解析注入（明文过 gRPC）。
- ⚠️ **v1 风险**：凭据到达 Harness 进程内存。缓解：v1 网络层可信（同集群/同 VPC，可选 mTLS）；v2 引入短期凭据代理（Harness 不持长期密钥）。
- MCP 连接只由独立 `mcp_runtime` 建立；middleware 不持有连接、不代理工具结果。
- api_key 仅注册时明文返回一次。

### 13.3 隔离

- sandbox：pod 级隔离 + egress 限制 + 定期重建（§11.3）。
- 数据隔离：所有查询按 app_id 过滤；管理接口（`/admin/*`）独立认证。

---

## 14. 可观测性

### 14.1 Trace

- `run_id` = trace 根；`TaskAssign.trace_context` 传播 W3C traceparent 到 daemon。
- Span 规划：`mw.run.dispatch` → `daemon.strategy.step`（daemon 上报事件关联）→ `mw.tool.invoke{local|global}`。

### 14.2 Metrics 清单

| 指标 | 类型 | 标签 |
|---|---|---|
| mw_runs_total | counter | status, runtime |
| mw_run_queue_duration_seconds | histogram | runtime |
| mw_workflow_tasks | gauge | type, status |
| mw_workflow_task_age_seconds | gauge | type |
| mw_workflow_claim_duration_seconds | histogram | type |
| mw_workflow_tasks_dead_total | counter | type, reason |
| mw_tool_invocations_total | counter | tool, scope, status |
| mw_tool_duration_seconds | histogram | tool, scope |
| mw_daemon_capacity | gauge | daemon, runtime（used/total） |
| mw_events_total | counter | type, ephemeral |
| mw_events_dropped_total | counter | reason（背压丢弃监控） |
| mw_ws_connections | gauge | app |
| mw_token_usage_total | counter | app, model（来自 USAGE 事件聚合） |

### 14.3 日志与回放

- 结构化日志全链路带 `run_id / invocation_id / trace_id`。
- `GET /runs/{id}/events` = 完整回放能力；后期 debug UI 直接消费该接口。

---

## 15. 代码结构（Go package 与接口）

```
middleware/
├── cmd/middleware/main.go        # 装配：config → store → planes → http/grpc server
├── proto/middleware/runtime/v1/  # proto + 生成代码
├── harness/                      # 精简 Python Harness（只含 McpRuntimeClient）
├── mcp_runtime/                  # 独立 MCP Runtime 进程（连接/session/tools）
├── internal/
│   ├── config/                   # YAML 配置加载
│   ├── db/                       # PostgreSQL 连接池 + 版本化迁移
│   ├── protocol/                 # Envelope 编解码辅助、消息构造器
│   ├── workflow/                 # PostgreSQL RunTaskQueue（Claim/Ack/Nack/DLQ）
│   ├── connection/
│   │   ├── daemonhub.go          # DaemonHub：注册/发送/背压/心跳检测
│   │   └── apphub.go             # AppHub：WS fanout/local tool 挂起表
│   ├── control/
│   │   ├── scheduler.go          # 过滤/排序/下发/重投
│   │   ├── lifecycle.go          # Run 状态机、取消、超时
│   │   └── reconcile.go          # 对账
│   ├── register/
│   │   ├── registry.go           # CRUD + 目录缓存失效
│   │   └── assemble.go           # TaskAssign 材料组装
│   ├── gateway/
│   │   ├── invoker.go            # 路由 + 幂等 + single-flight
│   │   └── executor/             # builtin:http, builtin:webhook
│   ├── runstore/
│   │   ├── store.go              # 事件追加 + 投影（事务）
│   │   └── session.go            # session/checkpoint
│   ├── httpapi/                  # App HTTP + WS 端点、auth 中间件
│   └── observe/                  # metrics/trace 初始化
└── docs/DESIGN.md
```

**核心接口**（包间只依赖接口，为远期 gRPC 拆分留口）：

```go
// internal/connection
type DaemonHub interface {
    Send(daemonID string, env *pb.Envelope) error
    PickHealthy(rt pb.RuntimeType, strategy string, labels map[string]string) (daemonID string, ok bool)
}
type AppHub interface {
    PublishEvent(appID string, ev EventView)
    CallLocalTool(ctx context.Context, appID string, req LocalToolCall) (LocalToolResult, error)
    NotifyRunCompleted(appID, runID, status string)
}

// internal/gateway
type Invoker interface {
    Invoke(ctx context.Context, run RunContext, req *pb.ToolCallRequest) *pb.ToolCallResult
}

// internal/workflow（PostgreSQL RunTaskQueue）
type TaskQueue interface {
    Enqueue(ctx context.Context, task Task) (Task, error)
    Claim(ctx context.Context, owner string, ttl time.Duration) (Task, Lease, error)
    Extend(ctx context.Context, lease Lease, ttl time.Duration) (Lease, error)
    Ack(ctx context.Context, task Task, lease Lease) error
    Nack(ctx context.Context, task Task, lease Lease, retryAt time.Time, cause error) error
    Fail(ctx context.Context, task Task, lease Lease, cause error) error
    CancelRun(ctx context.Context, runID, reason string) (int, error)
    CancelTask(ctx context.Context, runID string, typ TaskType, reason string) (int, error)
    ReapExpired(ctx context.Context, now time.Time, limit int) (int, error)
}

// internal/runstore
type Store interface {
    CreateRun(ctx context.Context, r Run) error
    AppendEvent(ctx context.Context, runID string, ev Event) error // 幂等 + epoch 校验 + 投影
    Transition(ctx context.Context, runID, to string, attemptDelta int) error
    GetRun(ctx context.Context, runID string) (Run, error)
    ListEvents(ctx context.Context, runID string, afterAttempt int32, afterSeq int64, limit int) ([]Event, error)
    SaveCheckpoint(ctx context.Context, cp Checkpoint) error
    LoadSession(ctx context.Context, sessionID string) (Session, error)
}

// internal/register
type Registry interface {
    UpsertApp(ctx context.Context, a App) (apiKey string, err error)
    GetApp(ctx context.Context, appID string) (App, error)
    AssembleTaskMaterials(ctx context.Context, appID string, model ModelReq) (TaskMaterials, error)
    ResolveTool(toolName, appID string) (ToolDef, error) // local 遮蔽 global
}

// internal/control
type Scheduler interface {
    Submit(ctx context.Context, run Run) error
    Cancel(ctx context.Context, runID, reason string) error
}
```

---

## 16. 里程碑与分期路线

| 里程碑 | 内容 | 验收标准 |
|---|---|---|
| **M1 契约与骨架** | proto 定稿生成；PostgreSQL RunTaskQueue；runstore + register + DaemonHub + 最简调度；精简 Harness（Channel + echo） | echo run E2E：App→MW→Workflow→Harness→固定回复→事件落库可查 |
| **M2 工具与推送** | AppHub WS fanout；local tool 闭环；global `builtin:http`；HTTP events 补拉；独立 `mcp_runtime` | 一次真实 react run 含 local+global 工具调用全链路通过 |
| **M3 韧性** | 取消/超时层级/重投退避/对账/幂等去重；metrics + trace | 故障矩阵 12.1 逐条演练通过 |
| **M4 longterm** | Session/Checkpoint/续跑/会话 API/GC；复合 watermark | 跨进程重启的会话恢复演示 |
| **M5 sandbox** | Harness 镜像、Deployment 预热池、NetworkPolicy、跑后重建 | 不可信输入在隔离 pod 执行，egress 拦截验证 |
| **M6 后期** | GUI 注册台、multi-agent 事件可视化、usage 报表 | — |

> 实际排期、步骤与验收脚本以 `docs/IMPLEMENTATION_PLAN_V2.md` 为准；自研 Workflow/Harness 后，原“W1–W4”周数不再成立。每个里程碑结束更新本文档“实际偏差”附录。

---

## 17. 开放问题与未来扩展

| # | 问题 | 触发时机 |
|---|---|---|
| Q1 | LLM 出口是否收敛到 MW 代理（计费/审计/密钥不下发 vs 多一跳延迟 + MW 成为带宽瓶颈） | sandbox 承载真实不可信流量前 |
| Q2 | 远程共享 MCP Hub（跨 App 共享昂贵 MCP server） | 作为独立数据面服务评估；出现多语言 Harness/共享/集中审计需求时 |
| Q3 | global 工具引入表达式/脚本执行（需沙箱化 executor，可复用 sandbox runtime） | v2 |
| Q4 | 平面拆分独立部署（接口已预留） | 单二进制出现瓶颈时 |
| Q5 | 跨 region 调度与就近接入 | 多集群需求出现时 |
| Q6 | ephemeral delta 的弱持久化（崩溃后恢复最后 token 流体验） | 用户反馈驱动 |

---

*本文档为 v1 设计的单一事实来源（SSOT）。实现与本文档冲突时，先修订文档，再改代码。*
