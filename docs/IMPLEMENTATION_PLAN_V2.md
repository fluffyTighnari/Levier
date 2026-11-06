# Middleware 实施计划 V2（重写版）

| 项目 | 内容 |
|---|---|
| 计划版本 | V2.0 |
| 日期 | 2026-11-03 |
| 状态 | P0–P2 完成；G2 通过；P3-23–P3-28 Harness 基础完成；取代 `IMPLEMENTATION_PLAN.md` v0.3 |
| 依据 | `docs/DESIGN.md` v0.1、`docs/WORKFLOW_ENGINE_AUDIT.md` |
| 总步骤 | 72 个审查单元 |
| 用法 | 一次只做一个 `Px-yy`；产出、验收、diff 都确认后再进入下一步 |

> 本版是**重写计划**：旧 `workflow_engine` 不改造、不包一层，直接归档；新 Workflow 与 Harness 都从零写。旧计划保留为历史，不再执行。

---

## 0. 本版的核心变化

1. **旧引擎整体退役**：`workflow_engine/` 归档到 tag，不进入新构建；新 Workflow 放在 middleware 主模块的 `internal/workflow`。
2. **Workflow 重写**：只做 PostgreSQL 持久化 **RunTaskQueue**，不做 DAG、Step、Executor、Effect、Saga、补偿、Async、独立 HTTP server。
3. **Harness 重写**：只做 middleware 需要的阉割版；保留 Channel、ReAct、LLM、工具/MCP、checkpoint，删除多 Agent、插件、GUI、复杂 Plan&Execute。
4. **统一使用 PostgreSQL**：RunStore + Workflow 同库同事务；本地开发/CI 用 Docker PostgreSQL。不实现 SQLite Store，避免维护第二套 SQL 方言；纯单元测试用内存 fake，SQL 集成测试用 PostgreSQL testcontainers。
5. **单 MW 实例**：DaemonHub 是内存态；v1 不启用多实例，但 PG + `SKIP LOCKED` 为后续多实例留口。
6. **清单化审查**：每步都有产出与验收；破坏性步骤（归档、删除、迁移）单独列出。

---

## 1. 已锁定决策

| # | 决策 | 结论 |
|---|---|---|
| D1 | Workflow | 重写为 PostgreSQL RunTaskQueue；旧 `workflow_engine` 归档，不复用代码 |
| D2 | Harness | 重写为阉割适配版；v1 只实现 `react` + `echo` |
| D3 | MW 实例 | v1 单活实例；不做多副本 HA |
| D4 | Run 归属 | `runs` 增加 `assigned_daemon_id / lease_epoch / assigned_at` |
| D5 | 事件序号 | 主键 `(run_id, attempt, seq)`；`Event` 带 `attempt`；MW 事件带 `source` |
| D6 | 状态写者 | Control 是唯一状态写者；事件只观测/校验/推进 `last_event_seq` |
| D7 | 首次注册 | `/admin/apps` 用 bootstrap token 创建并一次性返回 api_key |
| D8 | 幂等键 | `(app_id, idempotency_key)` + `request_hash`；不同 payload 返回 409 |
| D9 | 密钥 | v1 用 env/file provider；接口预留 Vault/K8s Secret；日志脱敏 |
| D10 | App 断线 | 挂起 local tool 立即失败 `APP_UNREACHABLE(retryable)` |
| D11 | WS 订阅 | 强制 `run.app_id == token.app_id` |
| D12 | Checkpoint | blob 与 `event_watermark` 同事务写入；只重放 watermark 之后 |
| D13 | 存储 | 统一 PostgreSQL；RunStore 与 TaskQueue 同库同事务；不实现 SQLite Store |
| D14 | 模块 | 根模块 `middleware`；新 Workflow 在 `internal/workflow`；Harness 在 `harness/` |
| D15 | 工具链 | Go 1.26 + Python 3.12；CI 锁定版本 |
| D16 | 旧能力 | DAG/Effect/Saga 打 tag 归档，主分支不保留 |
| D17 | MCP 归属 | 总控只存目录/ACL/`mcp_bindings`；连接由独立 `mcp_runtime` 进程负责；Harness 只持客户端；中央 MCP Hub 留 v2 |

---

## 2. 目标架构

```
App
 │ HTTP / WS
 ▼
internal/httpapi ──► internal/register (App/Tool/MCP 目录)
 │                        │
 ▼                        ▼
internal/control ──► internal/workflow (PostgreSQL run_tasks, SKIP LOCKED)
 │        │                │
 │        │                └─ Claim/Ack/Nack/Retry/Timeout/DLQ
 │        ▼
 │   internal/runstore (PostgreSQL runs/events/sessions/tool_invocations)
 │
 ├──► internal/connection (DaemonHub gRPC / AppHub WS)
 │           │
 │           ▼
 │      Python Harness (harness/)
 │        └─ McpRuntimeClient ──► mcp_runtime（独立进程）──► MCP servers
 │        └─ ToolBridge ──► internal/gateway（local/global）
 │
 └──► internal/gateway (local/global 工具路由 + 幂等)
```

**关键边界：**

- Workflow 只租约 dispatch/reconcile 操作，不租约整个 Run。
- Run 的长期执行由 daemon 事件驱动。
- PostgreSQL 是唯一权威；进程内不维护第二套队列。
- 队列时间优先使用数据库 `now()`，应用时间可注入 clock 便于测试。

---

## 3. 里程碑与审查门

| 阶段 | 步骤 | 目标 | Gate |
|---|---|---|---|
| P0 归档与地基 | P0-01–P0-07 | 旧引擎归档、两个 ADR、范围、仓库、配置、PostgreSQL 基础 | G0=P0-07 |
| P1 契约 | P1-08–P1-13 | proto、双端生成、协议辅助、App 契约、Fake Daemon、冻结 | G1=P1-13 |
| P2 新 Workflow | P2-14–P2-22 | RunTaskQueue、租约、延迟、重试、取消、DLQ、Worker | G2=P2-22 |
| P3 新 Harness | P3-23–P3-30 | Channel、任务生命周期、echo/react、LLM、工具/MCP、checkpoint | G3=P3-30 |
| P4 M1 垂直切片 | P4-31–P4-36 | App→MW→Workflow→Harness→事件落库→查询 | G4=P4-36（M1） |
| P5 RunStore/Register | P5-37–P5-42 | 完整 schema、CAS、事件、目录、密钥、材料组装 | G5=P5-42 |
| P6 Connection/Control | P6-43–P6-51 | DaemonHub/AppHub、背压、心跳、调度、取消、对账 | G6=P6-51 |
| P7 Gateway/API/WS | P7-52–P7-58 | 工具路由、幂等、builtin、REST、WS、回调闭环 | G7=P7-58 |
| P8 韧性/可观测 | P8-59–P8-64 | 全链路 E2E、故障矩阵、指标、压测、安全 | G8=P8-64（M2/M3） |
| P9 longterm | P9-65–P9-68 | Session、Checkpoint、粘滞续跑、跨重启、GC | G9=P9-68（M4） |
| P10 sandbox | P10-69–P10-72 | 镜像、预热池、NetworkPolicy、回收 | G10=P10-72（M5） |

**工期估算：** 单人约 55–75 个工作日；Go/Python 两人并行约 8–11 周；三人约 7–9 周。不含评审等待。

---

## 4. 执行清单

### P0 归档与地基（P0-01–P0-07）

- [x] **P0-01 归档旧 Workflow Engine**
  - 在 `workflow_engine/` 建分支，提交当前未提交改动，打 tag `archive/dag-effect-v0.1`。
  - 从新构建路径移除该目录（或只保留只读归档），确保主模块不 import 旧包。
  - 验收：可 checkout tag 回退；`go list ./...` 不含旧包；归档 commit/tag 记录进 `docs/ARCHIVE.md`。
  - 完成记录：commit `b999ac0`，branch/tag `archive/dag-effect-v0.1`，基线 `go test ./...` 全绿；目录暂保留只读，P0-05 建根模块时排除。

- [x] **P0-02 新 Workflow ADR**
  - `docs/adr/0001-workflow-engine.md`：Task 类型、状态机、租约、attempt、延迟、重试、取消、DLQ、错误分类、非目标。
  - 验收：状态图与 API 契约评审通过；明确“Workflow 不执行 Run，只调度”。
  - 完成记录：ADR 已产出；存储选型按推荐方向改为 **PostgreSQL 16+**；包含 `run_tasks` schema、`SKIP LOCKED` Claim、Ack/Nack/Cancel/Reap 语义、Worker 与 Control 集成、故障矩阵、验收标准。

- [x] **P0-03 精简 Harness ADR**
  - `docs/adr/0002-harness.md`：保留/裁剪能力矩阵、Python 版本、依赖、目录、与 MW 契约。
  - 验收：`react` + `echo` 为 v1 唯一策略；`plan_execute`/多 Agent/插件/GUI 明确不做。
  - 完成记录：ADR 已产出；确定 asyncio + grpc.aio、OpenAI-compatible 优先、持久事件才消耗 seq、checkpoint 复合 watermark；MCP 归属独立 `mcp_runtime` 进程，总控只存目录/ACL/`mcp_bindings`。

- [x] **P0-04 锁定 v1 范围与验收**
  - `docs/adr/0003-scope.md`：M1–M4 范围、M5 后置、生产 PostgreSQL、单 MW 实例、非目标。
  - 把 ADR-0001/ADR-0002 的变更并入 `DESIGN.md`（尤其 `TaskAssign.mcp_bindings`、`McpBinding`、`lease_epoch`、`attempt`、复合 watermark）。
  - 产出每个里程碑的验收脚本草案。
  - 验收：范围、验收、工期确认；D1–D17 全部有结论；DESIGN 与 ADR 无冲突。
  - 完成记录：ADR-0003 已产出；`DESIGN.md` 升级 v0.2，加入 RunTaskQueue、`McpBinding`、`lease_epoch`、`(run_id,attempt,seq)`、复合 watermark、`/admin/apps`；里程碑以 V2 为准。

- [x] **P0-05 工程骨架与 CI**
  - 初始化根 git（当前目录无 `.git`）；根 `go.mod`、`cmd/middleware`、`internal/*`、`proto/`、`harness/`、`Makefile`、lint、CI。
  - 验收：`make build/test/lint` 可跑；空二进制可启动并优雅退出；根模块不 import 旧 `workflow_engine`。
  - 完成记录：根 commit `d0c712a`；Go/Python 骨架、CI、`docs/CODE_STYLE.md` 文件头规范已加入；`make lint/test/build` 全绿；二进制 SIGTERM 退出码 0。报告：`docs/reports/P0-05.md`。

- [x] **P0-06 配置与基础库**
  - `internal/config`（YAML + env + 校验）、`internal/id`（ULID）、`internal/errs`（§4.5 错误码）、`internal/clock`、slog 初始化。
  - 验收：非法配置启动失败并指出路径；错误码表全覆盖；时间可注入。
  - 完成记录：`config.example.yaml`；config 默认值/env/严格字段/校验；ULID；错误码/retryable/HTTP 映射；Fake clock；slog 字段常量；`make test/lint` 全绿，二进制加载配置后 SIGTERM 退出码 0。报告：`docs/reports/P0-06.md`。

- [x] **P0-07 PostgreSQL 基础与迁移**
  - `internal/db`：`pgx` 连接池、超时、健康检查；版本化 SQL 迁移（`schema_migrations`）；本地 SSH 隧道访问 PostgreSQL 16；`make migrate`。
  - 验收：空库迁移可重复；重启后数据在；`/healthz` 能检查 DB；连接池不泄漏。
  - 完成记录：远端 PostgreSQL 16.15；`middleware`/`middleware_test` 库；`0001_init.sql` 建 `runs/run_events/run_tasks`；`make migrate/test/lint/build` 全绿；二进制 `database connected` 后 SIGTERM 退出码 0。报告：`docs/reports/P0-07.md`。

**G0 审查物：** 归档 tag、两个 ADR、范围文档、工程基线 PR、PostgreSQL 迁移与健康检查、P0-07 报告。

### P1 契约（P1-08–P1-13）

- [x] **P1-08 proto v1 定稿**
  - `proto/middleware/runtime/v1/runtime.proto`；加入 D4/D5/D17 变更：`RunningRun{run_id, lease_epoch}`、`Envelope.attempt`、`Envelope.lease_epoch`、`TaskAssign.mcp_bindings`、`McpBinding`、`Event.source`（attempt/epoch 只在 Envelope，不重复）。
  - 验收：`buf lint/build` 通过；与 DESIGN 的差异逐条记录。
  - 完成记录：`buf lint`/`buf build` 通过；`buf.yaml` 记录对称 Envelope 与 enum 短名的 lint 例外。报告：`docs/reports/P1-08.md`。

- [x] **P1-09 双端代码生成**
  - Go + Python stub；`make proto`；CI 检查生成物漂移。
  - 验收：生成后工作区干净；Go/Python 均可 import/编译。
  - 完成记录：`gen/` Go stub + `harness/src/gen` Python stub；`make proto-lint/proto-check` 通过；CI 增加 proto job；报告：`docs/reports/P1-09.md`。

- [x] **P1-10 协议辅助层**
  - `internal/protocol`：Envelope builder/validator、correlation、oneof、大小上限、错误映射。
  - 验收：非法 envelope 表驱动测试；全部消息有 builder。
  - 完成记录：`New/NewRun/NewResponse`、`SetCorrelation`、`Validate`、`CheckSize`、MW/daemon seq 规则、`BAD_ENVELOPE` 映射；报告：`docs/reports/P1-10.md`。

- [x] **P1-11 App 契约类型**
  - HTTP DTO、WS 消息类型、版本握手、OpenAPI 骨架。
  - 验收：marshal/unmarshal 黄金测试；字段与 DESIGN §4.2/4.3 对齐。
  - 完成记录：`internal/httpapi` DTO/WS 类型/校验；`docs/api/openapi.yaml` 3.1 骨架；报告：`docs/reports/P1-11.md`。

- [x] **P1-12 Go Fake Daemon**
  - 测试用 `RuntimeService.Channel`：可编排 accept/reject/event/toolcall/断线/延迟。
  - 验收：能驱动完整消息序列；支持故障注入。
  - 完成记录：`internal/testutil/fakedaemon` client-side fake；AutoEcho/拒绝/工具闭环/断线测试通过；报告：`docs/reports/P1-12.md`。

- [x] **P1-13 契约冻结**
  - `tests/contract/`：Go↔Python Envelope 黄金文件、buf breaking 基线、字段兼容规则。
  - 验收：双端 round-trip 一致；破坏性变更 CI 拦截。
  - 完成记录：11 条 golden Envelope；Go `TestContractGolden` + Python `ContractGoldenTest` 通过；`docs/CONTRACT.md` 固化兼容规则；`make contract-check` 接入；报告：`docs/reports/P1-13.md`。

**G1 审查物：** proto diff、生成物、协议单测、跨语言黄金文件。

### P2 新 Workflow Engine（P2-14–P2-22）

- [x] **P2-14 Task 模型与 API**
  - `internal/workflow`：Task/TaskType/Status/Lease/Outcome、Store 接口、Handler、错误分类。
  - TaskType：`RUN_DISPATCH`、`RUN_RETRY`、`RUN_TIMEOUT`、`RUN_RECONCILE`。
  - 验收：状态图与 API 评审通过；错误可区分 retryable/permanent/lease lost。
  - 完成记录：Task/Store/Handler/DeadLetter 契约、`RetryAfter`/`Permanent` 分类、Task 校验测试；报告：`docs/reports/P2-14.md`。

- [x] **P2-15 PostgreSQL schema**
  - `run_tasks`、部分唯一索引、就绪/租约索引、`schema_migrations`；字段：`available_at/attempt/max_attempts/lease_owner/lease_token/lease_expires_at/last_error/payload JSONB`。
  - 验收：迁移可重复；`EXPLAIN (ANALYZE, BUFFERS)` 走索引；`SKIP LOCKED` 计划正确。
  - 完成记录：迁移 0002 增加 attempt/lease CHECK、run/dead 索引；真实 PG 16.15 验证唯一/约束；报告：`docs/reports/P2-15.md`。

- [x] **P2-16 入队与幂等**
  - `Enqueue` 幂等：同 `(run_id, task_type)` 活跃任务唯一；支持与 Run 创建同事务。
  - 验收：重复入队返回既有任务；并发入队无重复；事务回滚不留任务。
  - 完成记录：`PostgresStore.Enqueue/EnqueueTx`、partial unique conflict、normalize、真实 PG 幂等/事务测试；报告：`docs/reports/P2-16.md`。

- [x] **P2-17 Claim/Lease/Ack/Nack**
  - `FOR UPDATE SKIP LOCKED` + `UPDATE ... RETURNING` 选任务；`lease_token++`；`Extend/Ack/Nack` 校验 owner+token。
  - 验收：双 worker 不重复 Claim；旧 token 写入返回 `ErrLeaseLost`；Nack 保留 attempt。
  - 完成记录：Claim/Extend/Ack/Nack/Fail + fencing；8 并发 Claim 无重复；真实 PG 测试通过；报告：`docs/reports/P2-17.md`。

- [x] **P2-18 取消与竞态**
  - `CancelRun`：PENDING→CANCELLED；LEASED→CANCELLED 且 token++；handler 可感知。
  - 验收：Cancel 与 Ack 竞态只产生一个终态；取消后不再被 Claim。
  - 完成记录：`CancelRun/CancelTask`、token fencing、Ack/Cancel 竞态测试；真实 PG 通过；报告：`docs/reports/P2-18.md`。

- [x] **P2-19 延迟、退避、回收、DLQ**
  - `available_at`、指数退避、租约回收器、MaxAttempts、毒任务进 DEAD、启动恢复。
  - 验收：假时钟延迟 ≤1s；过期租约回收；超限进 DLQ；启动扫描不重复。
  - 完成记录：`Backoff`、`ReapExpired`、`ListDead`、`Stats`；真实 PG 回收/DLQ 测试；启动 Run 扫描留 P6-51；报告：`docs/reports/P2-19.md`。

- [x] **P2-20 Worker 与 Pool**
  - 轮询循环、Handler、heartbeat、panic 恢复、优雅停机、并发上限。
  - 验收：慢 handler 自动续租；Stop 不丢已 Claim 任务；panic 不杀进程。
  - 完成记录：`Worker`/`Pool`、heartbeat、panic 恢复、DeadLetter、优雅停机；真实 PG 并发/慢 handler 测试；报告：`docs/reports/P2-20.md`。

- [x] **P2-21 故障与属性测试**
  - 双 worker、进程崩溃、重复投递、时钟跳变、PostgreSQL 死锁/连接池耗尽、取消/完成竞态。
  - 验收：无任务丢失/双执行；失败可复现；测试可重复。
  - 完成记录：延迟/崩溃恢复/单次执行/取消后不执行/Nack-Cancel 竞态/连接池耗尽；`-race` 通过；报告：`docs/reports/P2-21.md`。

- [x] **P2-22 指标、基准与 Gate**
  - 队列深度、最老任务年龄、Claim 延迟、Ack/Nack/Dead、租约丢失；1k 任务/秒基准。
  - 验收：基准报告；P4 可直接接入；G2 通过。
  - 完成记录：`Metrics`/`Pool.Stats`、Worker/Pool 指标、Enqueue/ClaimAck benchmark；本机 PostgreSQL 验证 Enqueue 约 9.1k/s、ClaimAck 约 1.82k/s，G2 通过；报告：`docs/reports/P2-22.md`。

**G2 审查物：** Task API、schema、并发/重启/DLQ 报告、基准数据。

### P3 新 Harness（P3-23–P3-30）

- [x] **P3-23 Harness 骨架**
  - `harness/`：pyproject、配置、结构化日志、健康检查、入口。
  - 验收：可安装/启动；配置非法即失败。
  - 完成记录：YAML 配置/env/SecretToken、JSON 日志、HealthState、`--check`/SIGTERM；11 个 Python 测试通过；报告：`docs/reports/P3-23.md`。

- [x] **P3-24 Channel 客户端**
  - gRPC 连接、Hello 鉴权、心跳、重连退避、收发循环、correlation 管理。
  - 验收：与 MW 建连；断线自动重连；心跳可观测。
  - 完成记录：`channel/` 客户端、ULID、Envelope、重连/心跳/收发；16 个 Python 测试通过；报告：`docs/reports/P3-24.md`。

- [x] **P3-25 任务生命周期**
  - TaskAssign 校验、5s TaskAck、RUN_STATUS、Cancel/Reconcile、优雅退出。
  - 验收：接受/拒绝/取消/对账路径均可测。
  - 完成记录：`RunManager`/`RunContext`/`EventWriter`；Ack、容量、去重、Cancel/Reconcile、close；23 个 Python 测试通过；报告：`docs/reports/P3-25.md`。

- [x] **P3-26 策略接口与 echo**
  - 策略接口 + `echo`；step 循环骨架、超时、取消检查。
  - 验收：echo 策略跑通；错误映射正确。
  - 完成记录：`Strategy` 协议、`EchoStrategy`、默认 runner 切换；echo 事件序列 seq 1/2/3；25 个 Python 测试通过；报告：`docs/reports/P3-26.md`。

- [x] **P3-27 ReAct 循环**
  - `react`：消息构建、step 循环、max_steps、工具调用插入、终态事件。
  - 验收：假 LLM 下多步 ReAct 单测；取消可中断。
  - 完成记录：`ReactStrategy`、`LLMProvider`、`ToolExecutor`；max_steps/step 超时/工具回灌/错误事件；7 个 ReAct 测试、32 个 Python 测试通过；报告：`docs/reports/P3-27.md`。

- [x] **P3-28 LLM 适配**
  - provider 抽象、首个 provider（Anthropic/OpenAI 二选一）、流式 delta、USAGE、错误映射。
  - 验收：假流式响应产生正确事件；retryable 分类正确。
  - 完成记录：`AnthropicProvider`、`UrllibTransport`、`ModelConfig`、`LLMError`、`StrategyRouter`；SSE/JSON 解析、delta/USAGE/错误映射；8 个 provider 测试、42 个 Python 测试通过；报告：`docs/reports/P3-28.md`。

- [ ] **P3-29 工具与 MCP Runtime**
  - `ToolCallRequest/Result` 客户端、工具清单、超时；独立 `mcp_runtime/` 进程 + 本地 JSON-RPC；Harness 只实现 `McpRuntimeClient`；`mcp_bindings`/`allowed_tools`/命名空间；stdio/SSE/streamable_http。
  - 连接只在 `mcp_runtime` 建立；middleware 和 Harness 都不持有 MCP 连接。
  - 验收：local/global 工具闭环；Runtime 启动/健康/退出；MCP 连接/失败/白名单/session 隔离测试；未来 remote client 契约不变。

- [ ] **P3-30 Checkpoint、打包与 Gate**
  - checkpoint 编码/上报/`resume_from`、容器基础镜像、集成测试。
  - 验收：跨进程恢复无重复消息；镜像可跑 echo；G3 通过。

**G3 审查物：** 能力矩阵、Channel 录制、假 LLM 测试、MCP 测试、checkpoint 恢复演示。

### P4 M1 垂直切片（P4-31–P4-36）

- [ ] **P4-31 最小 RunStore**
  - 同库 PostgreSQL：`apps/daemons/runs/run_events` 最小列 + 索引；CreateRun/GetRun/AppendEvent。
  - 验收：重复事件不重复插入；状态可查；重启后仍在。

- [ ] **P4-32 最小 DaemonHub**
  - 连接注册、Hello 鉴权、Send、断线清理、心跳。
  - 验收：未 Hello 3s 断开；重复 daemon_id 策略明确。

- [ ] **P4-33 最小 Control**
  - 领取 `RUN_DISPATCH`、按 runtime 选 daemon、TaskAssign、5s TaskAck、状态迁移。
  - 验收：单 daemon 单 run 可进入 assigned/running。

- [ ] **P4-34 最小 HTTP API**
  - `POST /runs`、`GET /runs/{id}`、`GET events`、测试用 Bearer。
  - 验收：curl 可创建并查到完整事件。

- [ ] **P4-35 Harness echo 集成**
  - Harness 接收 TaskAssign → Ack → RUN_STATUS/MESSAGE_COMPLETED → 终态。
  - 验收：seq 连续；终态正确；事件可回放。

- [ ] **P4-36 M1 E2E 与 Gate**
  - 脚本：App→MW→Workflow→Harness→固定回复→PostgreSQL→查询。
  - 验收：一条命令跑通；G4/M1 记录。

**G4 审查物：** E2E 录像/日志、Workflow 租约日志、事件表快照、重跑脚本。

### P5 RunStore/Register（P5-37–P5-42）

- [ ] **P5-37 完整 schema**
  - §3.2 表的 PostgreSQL schema + D4/D5 字段（`assigned_daemon_id/lease_epoch/attempt/source`）+ `sessions.event_attempt`（复合 watermark）+ 约束/部分索引。
  - 验收：迁移评审；`EXPLAIN (ANALYZE, BUFFERS)` 走索引；并发事务可用。

- [ ] **P5-38 状态机与 CAS**
  - `Transition(expected_status, attempt_delta, lease)`、终态守卫、Control 单一写者。
  - 验收：并发迁移仅一个成功；非法迁移拒绝；事件不回写状态。

- [ ] **P5-39 事件写入**
  - `(run_id, attempt, seq)` 幂等、`source`、乱序窗口 16、`SEQ_GAP`、ephemeral 分流；校验 `Envelope.lease_epoch == runs.lease_epoch`；MW 生成事件使用保留 seq 区间。
  - 验收：乱序/重复/缺口测试；旧 epoch 事件被丢弃并计数；崩溃后投影一致。

- [ ] **P5-40 事件查询与会话聚合**
  - `ListEvents` 分页 ≤1000、session 跨 run 聚合、稳定顺序。
  - 验收：分页无重无漏；跨 run 顺序稳定。

- [ ] **P5-41 App 注册与鉴权**
  - `/admin/apps` bootstrap、api_key 生成/hash/rotate、upsert by name、状态。
  - 验收：明文仅返回一次；重复 name 幂等；越权更新拒绝。

- [ ] **P5-42 Tool/MCP/组装与 Gate**
  - local/global ToolDef、MCP defs、`auth_ref` env/file、`ResolveTool` 遮蔽/ACL、`AssembleTaskMaterials` 产出 `mcp_bindings`（含 allowed_tools/namespace）。
  - 验收：遮蔽/ACL 矩阵；未授权 MCP 工具不下发；密钥不落日志；G5 通过。

**G5 审查物：** 迁移 diff、CAS/事件测试、注册 API 样例、密钥脱敏、材料组装快照。

### P6 Connection/Control（P6-43–P6-51）

- [ ] **P6-43 DaemonHub 生命周期**
  - Hello 3s、token 校验、重复连接、labels/strategies、优雅关闭。
  - 验收：鉴权失败关闭；重复连接不双写。

- [ ] **P6-44 出站与背压**
  - 单写 goroutine、队列 256、控制/持久/ephemeral 分类、慢消费者判定。
  - 验收：控制消息不丢；ephemeral 可丢并计数；慢消费者断开。

- [ ] **P6-45 入站与心跳**
  - 全消息分发、panic 隔离、10s/30s 心跳、容量校准、offline 事件。
  - 验收：假时钟 30s 判死；容量漂移可校准。

- [ ] **P6-46 AppHub WS**
  - ws_token 鉴权、app 绑定、连接注册、fanout、订阅归属校验。
  - 验收：跨 App 订阅拒绝；多连接同时收到。

- [ ] **P6-47 local tool 挂起表**
  - `CallLocalTool`、pending map、结果路由、超时/取消/断线快速失败。
  - 验收：重复结果幂等；断线立即 `APP_UNREACHABLE`。

- [ ] **P6-48 调度器**
  - runtime/容量/labels/strategy 过滤、least-loaded、longterm 粘滞。
  - 验收：表驱动选主；满载不分配；粘滞失败回退。

- [ ] **P6-49 TaskAssign/Ack/重投**
  - TaskAssign 组装/发送（携带 lease_epoch）、5s Ack、拒绝换候选、容量预留/释放、2s→5min 退避。
  - 验收：拒绝/超时换 daemon；成功分配时 `lease_epoch++`；容量无泄漏；退避曲线正确。

- [ ] **P6-50 取消与超时层级**
  - queued/assigned/running 取消、Cancel 下发、挂起工具取消、run_timeout。
  - 验收：取消/完成竞态单终态；超时必终态。

- [ ] **P6-51 对账与重启恢复**
  - Hello `running_run_ids`、daemon lost 重排队、MW 重启恢复、lease 防双跑。
  - 验收：DESIGN §12.2 三行逐条通过；G6 通过。

**G6 审查物：** 调度选主表、背压测试、12.2 对账报告、竞态测试。

### P7 Gateway/API/WS（P7-52–P7-58）

- [ ] **P7-52 Invoker 路由**
  - 解析/ACL/超时归属/local vs global/未知工具。
  - 验收：三分支路由测试；超时码正确。

- [ ] **P7-53 幂等与 single-flight**
  - 缓存→PostgreSQL、in-flight future、结果落库、TTL。
  - 验收：并发同 invocation 只执行一次；完成后命中。

- [ ] **P7-54 `builtin:http`**
  - 模板渲染、method/url/headers/body、超时/重试、响应大小限制。
  - 验收：httptest 覆盖成功/4xx/5xx/超时/大响应。

- [ ] **P7-55 `builtin:webhook`**
  - 预注册 URL、payload、响应映射、重试边界。
  - 验收：与 HTTP 共用执行器；配置校验。

- [ ] **P7-56 HTTP 中间件与 REST**
  - Bearer、App 作用域、错误信封、请求 ID、body 限制；apps/runs/events/cancel/sessions。
  - 验收：OpenAPI 与实现一致；越权/非法 JSON 统一错误。

- [ ] **P7-57 ws_token 与 WS**
  - 签发/校验 TTL 10min、连接、subscribe、补拉、`caught_up`、ping/pong。
  - 验收：过期拒绝；断线重连无重无漏。

- [ ] **P7-58 local_tool_result 与 Gate**
  - correlation/invocation 校验、幂等结果、未知调用、`run_completed`、OpenAPI 完整。
  - 验收：重发返回缓存；伪造 App 拒绝；G7 通过。

**G7 审查物：** 路由矩阵、并发去重、HTTP executor、OpenAPI、curl 全流程。

### P8 韧性/可观测（P8-59–P8-64）

- [ ] **P8-59 Go 全链路 E2E**
  - fake app + fake daemon：local+global 工具、事件回放。
  - 验收：单测/集成/脚本三层可重复。

- [ ] **P8-60 真实 Harness E2E**
  - echo→react，MCP 可选；事件语义核对。
  - 验收：真实 LLM 一轮含工具调用通过。

- [ ] **P8-61 故障矩阵**
  - DESIGN §12.1 每行 + Workflow 重启/DLQ/锁竞争。
  - 验收：矩阵报告逐条通过；已知限制标注。

- [ ] **P8-62 指标、追踪、日志**
  - §14.2 全指标、trace span、结构化字段、脱敏。
  - 验收：仪表盘/查询示例；无 PII/密钥。

- [ ] **P8-63 压力与安全**
  - N run 并发、慢消费者、队列上限；认证/ACL/ws_token/跨 App/密钥。
  - 验收：压测报告含拐点；安全清单通过。

- [ ] **P8-64 M2/M3 Gate**
  - 汇总 E2E、故障、指标、压测、安全。
  - 验收：G8/M2/M3 记录；遗留风险清单。

**G8 审查物：** 故障矩阵报告、压测报告、安全清单、M2/M3 验收记录。

### P9 longterm（P9-65–P9-68）

- [ ] **P9-65 Session 与 Checkpoint**
  - 创建/续跑、20 事件/60s 触发、`(event_attempt, event_seq)` 复合 watermark 原子性。
  - 验收：恢复无重无漏；checkpoint 不丢；跨 attempt 重放正确。

- [ ] **P9-66 粘滞调度与恢复**
  - 优先原 daemon，失败换 daemon + LoadSession。
  - 验收：daemon 迁移后从 checkpoint 续跑。

- [ ] **P9-67 跨重启与 GC**
  - MW/Harness 重启、expired 标记、blob 7 天清理。
  - 验收：演示脚本；GC 不误删活跃会话。

- [ ] **P9-68 M4 Gate**
  - 演示脚本、DESIGN 偏差附录。
  - 验收：G9/M4 通过。

**G9 审查物：** 跨重启录像、checkpoint 快照、GC 测试。

### P10 sandbox（P10-69–P10-72）

- [ ] **P10-69 镜像与预热池**
  - Dockerfile、非 root、健康检查；k8s Deployment、N 空闲、labels、`max_concurrency=1`。
  - 验收：新 Run 秒级分配；池容量可观测。

- [ ] **P10-70 网络隔离**
  - NetworkPolicy：仅 MW gRPC + LLM 端点；DNS 策略。
  - 验收：egress 拦截测试；白名单生效。

- [ ] **P10-71 回收与 draining**
  - K run 后重建、draining 接口、优雅退出。
  - 验收：存量跑完再下线；不接新 Run。

- [ ] **P10-72 M5 Gate**
  - 不可信输入隔离执行、egress 验证、残余风险。
  - 验收：G10/M5 报告 + 运维手册。

**G10 审查物：** 镜像扫描、k8s 清单、egress 测试、回收演练。

---

## 5. 每步审查协议

1. **开工前**：改动 >2 个文件时，先给 ≤1 页设计说明：文件、接口、测试点、风险。
2. **实现**：只做一个 `Px-yy`；不提前做后续步骤；不顺手重构无关代码。
3. **写报告**：在 `docs/reports/Px-yy.md` 记录新增/修改文件、关键决策、验证命令与结果、风险、下一步；模板见 `docs/reports/TEMPLATE.md`。
4. **自检**：`make test`、`make lint`、该步验收命令；输出附在审查消息与报告中。
5. **提交审查**：`git diff --stat`、关键 diff、测试输出、已知风险、报告链接；等待确认后再进入下一步。
6. **Gate**：每阶段结束跑验收，更新本清单勾选状态；设计冲突先改 `DESIGN.md` 再改代码。

**DoD：** 代码 + 单测 + 必要集成测试 + 文档 + 实施报告 + 无未登记 TODO + 无敏感信息日志 + 可重复验收命令。

> 报告规则：从 P0-05 起，每轮必须在同一提交/同一轮内新增 `docs/reports/Px-yy.md`；没有报告不得进入下一步。

---

## 6. 测试策略

| 层级 | 范围 | 方式 |
|---|---|---|
| 单元 | 错误映射、协议、状态机、调度过滤、模板 | Go `testing` 表驱动 |
| Workflow | Claim/Lease/Ack/Nack/取消/延迟/DLQ/重启 | 真实 PostgreSQL（testcontainers）+ 假时钟 + 属性测试 |
| 存储 | 迁移、CAS、事件幂等/乱序、checkpoint 原子性 | 真实 PostgreSQL（testcontainers） |
| 契约 | proto lint/breaking、HTTP/WS schema、OpenAPI | buf、黄金文件 |
| E2E | fake app + fake daemon；真实 Harness | Go 集成测试 + 脚本 |
| 故障 | §12.1 全矩阵 + Workflow 锁竞争/DLQ | 故障注入 + 假时钟 |
| 压力 | 并发 Run、慢消费者、队列上限 | 脚本 + pprof/指标 |
| 安全 | 认证/ACL/越权/密钥/WS token | 表驱动 + 手工清单 |

**纪律：** 时间逻辑必须可注入 clock；不能只靠人工点击验收。

---

## 7. 风险与缓冲

| 风险 | 影响 | 缓解 |
|---|---|---|
| Workflow 与 Harness 同时从零写 | M1 延期 | P2/P3 各自 Gate；先 echo 后 react；Go/Python 分人并行 |
| PostgreSQL 连接池/死锁/迁移 | Claim/Ack 失败、启动失败 | 短事务 + `SKIP LOCKED` + 重试；P2-22 压测；版本化迁移 |
| 双库事务边界 | Run 创建成功但任务未入队 | D13 统一 PostgreSQL；`Enqueue` 支持同事务 |
| 本地开发依赖变重 | 启动慢、CI 复杂 | Docker Compose + testcontainers；纯单元测试用内存 fake |
| 旧引擎归档不彻底 | 误 import/两套模型 | P0-01 从构建路径移除；CI 禁止旧包 |
| 租约/取消竞态 | 双执行/状态回退 | token fencing + 单终态测试；P2-18/P2-21 |
| Harness 裁剪过度/不足 | M2 返工 | P0-03 能力矩阵冻结；P3 分层验收 |
| 多 Harness 重复实现 MCP | 行为不一致、维护成本上升 | 共享独立 `mcp_runtime` 进程和 JSON-RPC 契约；Harness 只持客户端 |
| 密钥下发 | daemon 内存泄露 | 网络可信 + 脱敏 + 审计；v2 短期凭据代理 |
| sandbox egress | 数据外渗 | 记录残余风险；P10-70 白名单验证 |
| 范围蔓延 | 里程碑失控 | 严格按 P0-04 非目标；新需求进 backlog |

---

## 8. 建议的下一步

先审本清单的 **P0-01–P0-04**，只做文档与归档决策，不写业务代码：

1. 是否同意**旧引擎整体归档、不改造**。
2. 是否同意**根模块 + `internal/workflow` + `harness/`** 的布局。
3. 是否同意 **生产统一 PostgreSQL、v1 单 MW 实例**。
4. 是否同意 **72 步、55–75 人日** 的基线。

确认后从 **P0-01** 开始：先冻结旧 `workflow_engine` 的未提交改动并打 tag，再进入 P0-02 ADR。
