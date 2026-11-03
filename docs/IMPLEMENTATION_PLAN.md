# Middleware v1 实施计划（逐步审查版）

> **已被 `docs/IMPLEMENTATION_PLAN_V2.md` 取代。** 本文件保留为历史版本，不再执行；V2 采用“旧引擎归档 + Workflow/Harness 从零重写”的方案。

| 项目 | 内容 |
|---|---|
| 依据 | `docs/DESIGN.md` v0.1（评审稿） |
| 计划版本 | v0.3 |
| 日期 | 2026-11-03 |
| 状态 | 已加入本地 SQLite 引擎；D1 改为“改造现有引擎”；待审 `WORKFLOW_ENGINE_AUDIT.md` |
| 用法 | 每个 `Sxx / Wxx / Hxx` 是一个独立审查单元；一次只做一个，评审通过后再进入下一个 |

> 新增审计：`docs/WORKFLOW_ENGINE_AUDIT.md`（现有引擎 13.4k 行、93 个测试的保留/删除矩阵）。

> 本文件是 `DESIGN.md` 的派生执行计划，不替代 SSOT。实施中发现设计冲突时，先修订 `DESIGN.md`，再改代码。

---

## 0. 核心设计理解（阅读结论）

1. **它不是 Agent 框架，而是 Agent Runtime 的控制平面与中介层。** ReAct / Plan&Execute 的推理循环留在 Python harness daemon；middleware 只负责调度、路由、工具中介、持久化、可观测。
2. **四平面、单进程。** Register（目录/ACL/材料组装）、Control（Run 生命周期/调度/对账）、Tool Gateway（local/global 路由与幂等）、Connection（App WS + Daemon gRPC）以 Go package 边界隔离，接口预留未来拆分。
3. **双契约。** App 侧是 HTTP + WS；Daemon 侧是单条 gRPC 双向流。所有请求/响应复用同一条流，以 `correlation_id` 闭环，消除回调拓扑。
4. **Workflow 只建模 Run 级生命周期。** `queued → assigned → running → 终态` 走持久化队列；ReAct 内层循环不经过 MQ，避免延迟放大与状态爆炸。
5. **单一持久化骨架。** 改造后的 Workflow（SQLite RunTaskQueue）是调度与重试的权威骨架；Connection Plane 只是边缘投递适配层，避免“双消息系统”。
6. **Run 与连接解耦。** App WS 断开不影响 Run；事件按 `(run_id, attempt, seq)` 落库，重连按 `last_seq` 补拉；ephemeral token delta 只转发不落库。
7. **全链路幂等。** Run 提交用 `Idempotency-Key`；工具调用用 `invocation_id`（缓存 + single-flight）；事件写入用 `(run_id, attempt, seq)` 主键去重。语义是 at-least-once 投递 + 去重后的 effectively-once。
8. **三种 Runtime 是放置策略差异，不是协议差异。** normal 从头跑；longterm 从 checkpoint 恢复并优先粘滞；sandbox 只是 pod 级隔离与生命周期管理。
9. **故障收敛靠心跳/租约 + 对账，不靠分布式共识。** Daemon 30s 无心跳判死；重连 `Hello.running_run_ids` 与 MW 视角对账；MW 重启依赖 workflow 重投 + daemon 对账。
10. **信任模型是“可信内网 + 凭据下发 + sandbox 补偿”。** v1 明确接受 daemon 内存持有 LLM/MCP 明文凭据的风险，用同集群/可选 mTLS + sandbox egress 限制缓解。

**一句话总结：** middleware 把“会思考的 harness”变成“可多租户调度、可审计、可重连、可水平放置”的执行资源，同时把工具调用收敛到一个可幂等、可观测的中介点。

---

## 1. 开工前必须拍板的决策

以下问题不解决会导致返工。建议先只审这一节，再批准 P0。

| # | 决策 | 现状/风险 | 建议默认 | 阻塞步骤 |
|---|---|---|---|---|
| D1 | 改造现有 SQLite Workflow Engine | **已确认：以 `workflow_engine` 为基础改造**，不另写；当前引擎是 DAG+Effect+Saga，超出 middleware 需求 | 裁剪为 SQLite 持久化 **RunTaskQueue**：保留 Task/Lease/Worker 骨架，删除 DAG 执行、Effect、Saga、Async、HTTP server、缓存、示例；详见审计文档 | W01–W08 / S38 |
| D2 | 自研精简 Harness（重写） | **已确认：重写为阉割适配版** | 在 `harness/` 建最小 Python daemon：Channel 客户端 + ReAct + LLM/工具/MCP + checkpoint；不做多 Agent、插件生态、GUI、向量记忆 | H01–H07 / S19 / S57 |
| D3 | v1 是否只做单 MW 实例 | **已同意**：DaemonHub 是内存态，多副本会双投/双写 | v1 单活实例；HA 作为后续 ADR，不在本期实现 | S32/S45 |
| D4 | Run 的 daemon 归属与租约字段 | **已同意**：`runs` 表缺归属字段，重启后无法对账 | 增加 `assigned_daemon_id / lease_epoch / assigned_at`；`Hello` 上报 `(run_id, lease_epoch)` | S22/S45 |
| D5 | 重试后事件 seq 如何不冲突 | **已同意**：daemon 换实例后 seq 从 1 开始，旧主键会误判重复 | 事件主键改 `(run_id, attempt, seq)`；`Event` 增加 `attempt`；MW 生成事件加 `source` 并用 `ts_ms` 排序 | S09/S22/S24 |
| D6 | Run 状态以谁为准 | **已同意**：`Transition` 与 `RUN_STATUS` 投影存在双写竞态 | Control 是唯一状态写者；事件只做观测/校验/推进 `last_event_seq` | S23/S43 |
| D7 | 新 App 如何完成首次注册 | 待确认（建议默认）：`POST /apps` 要求 app key，但新 App 没有 key | 增加 `/admin/apps`（管理员 bootstrap token）创建并一次性返回 key；`/apps` 仅允许已认证 App 更新自身 | S27/S51 |
| D8 | `Idempotency-Key` 作用域与冲突语义 | 待确认（建议默认）：全局 UNIQUE，未定义同 key 不同 payload | 唯一键改 `(app_id, idempotency_key)`；存 `request_hash`；同 key 同 hash 返回原 Run，不同 hash 返回 409 | S39 |
| D9 | 密钥提供方 | 待确认（建议默认）：`auth_ref` 未定义解析实现 | v1：env + 文件 provider，接口预留 Vault/K8s Secret；日志与响应统一脱敏 | S29/S31 |
| D10 | App 断线时挂起的 local tool 行为 | 待确认（建议默认）：等 60s 还是立即失败未定义 | 立即失败 `APP_UNREACHABLE(retryable)`，由 daemon 策略层决定重试/降级；同时向 App 发取消错误 | S36 |
| D11 | WS `subscribe` 的越权校验 | 待确认（建议默认）：文档未显式要求校验 Run 归属 | 强制 `run.app_id == token.app_id`，跨 App 订阅返回 `TOOL_DENIED`/关闭连接 | S35/S53 |
| D12 | Checkpoint 与 watermark 的原子性 | 待确认（建议默认）：可能导致恢复时重复/丢事件 | `checkpoint_blob` 与 `event_watermark` 同事务写入；恢复只重放 watermark 之后的事件 | S26/S62 |
| D13 | RunStore 与 Workflow 是否共用同一个 SQLite 文件 | 决定 `POST /runs + Enqueue` 能否同事务；两库需要 outbox | **建议 v1 统一 SQLite**（单机本地）；Postgres 作为后续 Store 实现，不阻塞 v1 | S15/S22/W03/S39 |
| D14 | `workflow_engine` 是否并入 `internal/workflow` | 影响 import、仓库边界与部署 | 改造期保留独立 module，W08 后评估合并；不要边改边搬家 | W01/W08 |
| D15 | Go 工具链 | 当前环境 `go` 不在 PATH，无法编译/跑测试 | 安装 Go 1.26 或用容器；先把 `go.mod` 与 CI 版本对齐 | W01/W07 |
| D16 | 是否保留 DAG 能力给其他项目 | 删除后不可复用 | 打 tag/归档 `archive/dag-effect-v0.1`；middleware 主分支只保留 RunTaskQueue | W01/W07 |

**已确认项：** D1 改为改造现有 SQLite 引擎；D2 改为重写精简 Harness；D3–D6 同意。**D7–D16** 目前按“建议默认”写入计划，若你不反对，后续步骤直接按此实现；如需调整，在对应步骤开工前提出即可。

### 1.1 自研边界（本次确认）

**Workflow Engine v1 做（改造后）：** SQLite 持久化任务、Task 级租约/可见性超时、Ack/Nack、延迟任务/定时器、指数退避、按 `run_id` 取消、DLQ、幂等入队、并发 worker、队列指标。**删除：** DAG/Step/Input 解析、Executor 注册表、Effect Journal、Saga 补偿、Effect Reconcile、Async 装配层、进程内 Queue、独立 HTTP server、Cached/Memory Store、Step Attempt Journal、每 Run 文件日志、DAG 示例。**不做：** cron、跨 region、多活 HA、精确一次语义、独立部署。

> 关键边界：Workflow 只租约 **dispatch/reconcile 操作**（秒级），不租约整个 Run。Run 的长期执行由 daemon 事件驱动，避免 30min 长租约与 daemon 失联耦合。

**精简 Harness v1 做：** gRPC Channel 客户端、Hello/Heartbeat/TaskAck/Cancel/Reconcile、TaskAssign、策略接口 + `react` + `echo`、单 LLM provider 适配、local/global 工具调用、MCP 客户端、事件上报、checkpoint/resume。**不做：** 多 Agent 编排、插件市场、GUI、向量记忆、复杂 Plan&Execute（保留接口，v1 不实现）、跨 runtime 子 Agent。

> 自研意味着 M1 的关键路径从“适配既有资产”变成“先定义并实现两个最小内核”。因此计划把 Workflow 与 Harness 提升为独立阶段，并各自设置 Gate。

---

## 2. 计划总览

| 阶段 | 步骤 | 目标 | 里程碑 |
|---|---|---|---|
| P0 决策与地基 | S01–S08 | 两个自研内核的边界、仓库、配置、错误模型、本地依赖、可观测骨架 | 开工前置 |
| P1 契约 | S09–S14 | proto 定稿与双端 stub、协议辅助、App 契约、Fake Daemon、契约冻结 | M1 前置 |
| P1.5 改造 SQLite Workflow Engine | W01–W08 | 审计并裁剪 DAG/Effect/Saga，保留 SQLite 队列、租约、重试、DLQ，接入 Control | M1 前置 |
| P1.6 精简 Harness | H01–H07 | Channel 客户端、任务生命周期、ReAct、LLM/工具/MCP、checkpoint | M1 前置 |
| P2 最小垂直切片 | S15–S21 | 打通 echo E2E：App→MW→daemon→事件落库→查询 | **M1** |
| P3 RunStore 完整化 | S22–S26 | 全量表结构、CAS 状态机、乱序/幂等、会话/检查点/工具审计 | M2 前置 |
| P4 Register | S27–S31 | App/Tool/MCP 注册、密钥引用、解析与材料组装 | M2 前置 |
| P5 Connection 完整化 | S32–S37 | DaemonHub/AppHub、背压、心跳、local tool 挂起与断线 | M2 |
| P6 Control 完整化 | S38–S45 | 接入改造后的 TaskQueue、调度、Ack、重投、取消、对账与重启恢复 | M2/M3 |
| P7 Tool Gateway | S46–S50 | 路由、幂等、single-flight、builtin:http/webhook、审计 | M2 |
| P8 App API/WS | S51–S55 | 全量 REST、ws_token、补拉、local_tool_result 闭环 | M2 |
| P9 韧性/可观测 | S56–S61 | 真实 E2E、故障矩阵、指标/追踪、压测、安全测试 | **M3** |
| P10 longterm | S62–S65 | Session/Checkpoint/续跑/GC/跨重启恢复 | **M4** |
| P11 sandbox | S66–S70 | 镜像、预热池、NetworkPolicy、回收、验收 | **M5** |

**总计 85 个审查单元**：S01–S70 + W01–W08 + H01–H07。

**粗略工期（不含评审等待）：** 单人约 60–80 个工作日；Go 与 Python 两人并行约 9–12 周；三人（Go/Workflow、Harness、测试/部署）约 7–9 周。Workflow 有现成 13.4k 行可裁剪，比从零写省一些，但删除/重写测试与 Control 适配仍需时间；Harness 重写仍是最大变量。

**审查门（Gate）：** G0=S08、G1=S14、G1.5=W08、G1.6=H07、G2=S21（M1）、G3=S26、G4=S31、G5=S37、G6=S45、G7=S50、G8=S55、G9=S61（M2/M3）、G10=S65（M4）、G11=S70（M5）。每个 Gate 必须跑通该阶段验收并更新本计划状态，才能进入下一阶段。

---

## 3. 详细步骤

### P0 决策与地基（S01–S08）

| 步骤 | 任务 | 产出 / 审查物 | 验收（可验证） | 依赖 |
|---|---|---|---|---|
| S01 | 审计并定稿 SQLite Workflow Engine 改造 | `docs/WORKFLOW_ENGINE_AUDIT.md` + `docs/adr/0001-workflow-engine.md`：Task 状态机、租约/attempt、延迟/重试/取消/DLQ、删除清单、非目标 | 审计矩阵与状态图评审通过；W01–W08 边界冻结；确认 D13–D16 | — |
| S02 | 定义精简 Harness 能力边界 | `docs/adr/0002-harness.md`：保留/裁剪能力矩阵、Python 版本、依赖、目录、与 MW 的契约 | 矩阵明确“做/不做”；H01–H07 边界冻结 | — |
| S03 | 锁定 v1 范围与验收 | `docs/adr/0003-scope.md` + 本计划基线（M1–M4 in，M5 后置） | 每个里程碑有可执行验收脚本草案；非目标清单确认 | S01/S02 |
| S04 | 仓库骨架与工程基线 | `go.mod`、`cmd/middleware`、`internal/*`、`harness/`、`Makefile`、lint、CI；确认 git 远端（当前目录无 `.git`） | `make build/test/lint` 全绿；空二进制可启动并优雅退出 | S03 |
| S05 | 配置系统 | `internal/config` + `config.example.yaml` + env 覆盖 + 启动校验 | 缺字段/非法值启动即失败并指出路径；示例配置可启动 | S04 |
| S06 | 基础类型与错误模型 | `internal/id`（ULID）、`internal/errs`（code/retryable/HTTP 映射）、`internal/clock`、日志字段约定 | §4.5 错误码全覆盖单测；ID 唯一性测试 | S04 |
| S07 | 本地依赖与迁移工具 | 默认 SQLite 文件（WAL/busy_timeout）、`migrate` 命令、健康检查、`make dev-up`；不引入独立 MQ；Redis 非必需 | 新机器无需外部服务即可启动；迁移可重复执行；SQLite 重启后数据在 | S01/S04 |
| S08 | 可观测骨架 | otel/prometheus/slog 初始化、`/healthz`、`/metrics`、request/run 字段规范 | 一次请求产生 trace、metric、结构化日志；无敏感字段 | S06/S07 |

**G0 审查物：** ADR-0001（Workflow）、ADR-0002（Harness）、ADR-0003（范围）、工程基线 PR、配置样例、错误码测试报告。

### P1 契约（S09–S14）

| 步骤 | 任务 | 产出 / 审查物 | 验收（可验证） | 依赖 |
|---|---|---|---|---|
| S09 | proto 定稿 | `proto/middleware/runtime/v1/runtime.proto`、`buf.yaml`、lint/breaking 规则 | `buf lint/build` 通过；与 DESIGN §4.1 的差异仅限 D4/D5/D12 并记录 | S03 |
| S10 | 代码生成 | Go/Python stub、`Makefile` proto 目标、CI 漂移检查 | `make proto` 后工作区干净；Go/Python 均可编译 | S09 |
| S11 | 协议辅助层 | `internal/protocol`：Envelope 构造/校验、correlation、oneof、大小上限、错误映射 | 非法 envelope 表驱动测试；builder 覆盖全部消息 | S10 |
| S12 | App 契约类型 | HTTP DTO、WS 消息类型、版本握手、JSON schema/OpenAPI 骨架 | marshal/unmarshal 黄金测试；字段与 §4.2/4.3 对齐 | S10 |
| S13 | Go Fake Daemon | 测试用 `RuntimeService.Channel` 实现：可编排 accept/reject/event/toolcall/断线/延迟 | 可在一个集成测试里驱动完整消息序列 | S11 |
| S14 | 契约冻结与跨语言基线 | `tests/contract/`：Go↔Python Envelope 黄金文件、buf breaking 基线、错误码/字段兼容规则 | 双端 round-trip 一致；后续破坏性变更 CI 拦截 | S11/S12 |

**G1 审查物：** proto diff、生成物、协议单测、跨语言黄金文件、契约冻结记录。

### P1.5 改造 SQLite Workflow Engine（W01–W08）

> 目标：把现有 `workflow_engine` 从“DAG + Effect/Saga 执行引擎”裁剪为“SQLite 持久化 RunTaskQueue”，承载 Run 级调度、延迟超时、重试与取消。保留 Task/Lease/Worker 骨架；删除 DAG 执行与副作用编排。

| 步骤 | 任务 | 产出 / 审查物 | 验收（可验证） | 依赖 |
|---|---|---|---|---|
| W01 | 冻结基线并确认删除范围 | `workflow_engine` 分支/tag、当前未提交改动归档、Go 工具链基线、审计矩阵复核 | 可回退点存在；`go test ./...` 基线记录；删除清单签字 | S01 |
| W02 | 目标模型与 API 定稿 | `pkg/task`：Task/TaskType/TaskStatus/Lease/Outcome；`pkg/store` 窄接口与错误分类 | 状态图、Claim/Cancel/Nack 竞态语义、错误分类评审通过 | W01 |
| W03 | SQLite schema 与队列内核 | 新 `run_tasks` 表 + 索引 + 迁移；`Enqueue/Claim/Extend/Ack/Nack/Cancel/Stats` | 并发 Claim 不重复；token fencing 生效；空库迁移可重复 | S07/W02 |
| W04 | 延迟、重试、回收、DLQ | `available_at`、指数退避、租约回收器、MaxAttempts、幂等入队、启动恢复 | 假时钟延迟 ≤1s；过期回收；毒任务进 DLQ；无重复活跃任务 | W03 |
| W05 | Worker 与 Pool | 轮询循环、heartbeat、panic 恢复、优雅停机、Handler 接口 | 慢 Handler 自动续租；Stop 不丢已 Claim 任务；双 worker 不双跑 | W03/W04 |
| W06 | 删除旧代码并重写测试 | 删除 DAG/Step/Effect/Saga/Reconcile/Wait/Async/Queue/server/examples/cache/memory/runfile；重建队列测试 | `go list ./...` 无废弃包；`go test ./...` 全绿；旧测试替换完成 | W05 |
| W07 | Control 接入 | `Dispatch/Retry/Timeout/Reconcile` Handler；Run 创建 + 入队同事务（或 outbox） | mock daemon 下四条路径闭环；重复投递幂等 | W05/W06 |
| W08 | 故障、基准与文档 | 进程重启、双 worker、SQLite 锁竞争、取消竞态、1k 任务/秒基准、运维说明 | 故障报告通过；P6 可直接接入；无任务丢失/双执行 | W07 |

**G1.5 审查物：** 审计文档、`run_tasks` schema、队列 API、并发/重启/DLQ 测试报告、删除清单执行记录、基准数据。

### P1.6 精简 Harness（H01–H07）

> 目标：重写一个只服务本 middleware 的 Python daemon。保留“能跑 ReAct + 调工具 + 上报事件 + 恢复”这条主线，砍掉多 Agent、插件、GUI、复杂记忆。

| 步骤 | 任务 | 产出 / 审查物 | 验收（可验证） | 依赖 |
|---|---|---|---|---|
| H01 | 工程骨架与配置 | `harness/` 包结构、`pyproject.toml`、配置加载、结构化日志、健康检查 | 可安装/启动；配置非法即失败 | S04 |
| H02 | Channel 客户端 | gRPC 连接、Hello 鉴权、心跳、重连退避、收发循环、`correlation_id` 管理 | 与 MW 建连；断线自动重连；心跳可观测 | S13/S14 |
| H03 | 任务生命周期 | TaskAssign 校验、5s TaskAck、RUN_STATUS、Cancel/Reconcile、优雅退出 | 拒绝/接受/取消/对账路径均可测 | H02 |
| H04 | 策略运行时 | 策略接口 + `react` + `echo`；step 循环、max_steps、超时、取消检查点 | echo 策略 E2E 通过；react 可注入假 LLM 单测 | H03 |
| H05 | LLM 适配 | provider 抽象、首个 provider（Anthropic/OpenAI 二选一）、流式 delta、USAGE、错误映射 | 假流式响应产生正确事件；错误可重试性正确 | H04 |
| H06 | 工具与 MCP | `ToolCallRequest/Result` 客户端、工具清单、超时、MCP stdio/SSE/streamable_http 客户端 | local/global 工具调用闭环；MCP 连接/失败测试 | H05 |
| H07 | Checkpoint 与打包 | checkpoint 编码/上报/`resume_from`、容器基础镜像、集成测试 | 跨进程恢复无重复消息；镜像可跑 echo | H06 |

**G1.6 审查物：** Harness 能力矩阵、Channel 录制、假 LLM 测试、MCP 测试、checkpoint 恢复演示。

### P2 最小垂直切片（S15–S21，M1）

| 步骤 | 任务 | 产出 / 审查物 | 验收（可验证） | 依赖 |
|---|---|---|---|---|
| S15 | M1 最小 schema | SQLite 迁移：`apps/daemons/runs/run_events/run_tasks` 最小列 + 索引 | 空库 up/down/up；约束生效；WAL 重启可用 | W03 |
| S16 | 最小 RunStore | `CreateRun/GetRun/AppendEvent`（幂等）+ 状态投影 | 重复事件不重复插入；状态可查 | S15/W03 |
| S17 | 最小 DaemonHub | 连接注册、Hello 鉴权、`Send`、断线清理 | 未 Hello 3s 断开；重复 `daemon_id` 策略明确 | S13 |
| S18 | 最小 Control/调度 | 领取 `RUN_DISPATCH`、按 runtime 选 daemon、TaskAssign、5s TaskAck、状态迁移 | 单 daemon 单 run 可进入 assigned/running；任务由 TaskQueue 驱动 | S16/S17/W05 |
| S19 | Harness echo 策略 | 接收 TaskAssign → Ack → 发 RUN_STATUS/MESSAGE_COMPLETED → 终态 | echo run 的 seq 连续、终态正确 | H02/H03/H04/S18 |
| S20 | 最小 HTTP API | `POST /runs`、`GET /runs/{id}`、`GET events`、Bearer 鉴权（测试 key） | curl 可创建并查到完整事件 | S16/S18 |
| S21 | M1 垂直 E2E | Go 集成测试 + 脚本：App→MW→Workflow→daemon→固定回复→落库→查询 | 一条命令跑通；事件可回放；G2/M1 验收记录 | S19/S20 |

**G2 审查物：** echo E2E 录像/日志、Workflow 租约日志、事件表快照、重跑脚本、M1 偏差记录。

### P3 RunStore 完整化（S22–S26）

| 步骤 | 任务 | 产出 / 审查物 | 验收（可验证） | 依赖 |
|---|---|---|---|---|
| S22 | 完整迁移与决议落地 | 全部 §3.2 表的 SQLite 等价 schema + D4/D5 字段（`assigned_daemon_id/lease_epoch/attempt/source`）+ 约束 | 迁移评审通过；`EXPLAIN QUERY PLAN` 走索引；WAL 并发可用 | S21 |
| S23 | 状态机与 CAS | `Transition(expected_status, attempt_delta, lease)`、终态守卫、投影更新 | 并发迁移仅一个成功；非法迁移拒绝 | S22 |
| S24 | 事件完整写入 | 乱序窗口 16、`SEQ_GAP`、ephemeral 分流、缓存热拷贝、事务边界 | 乱序/重复/缺口测试；崩溃后投影一致 | S22/S23 |
| S25 | 事件查询与会话聚合 | `ListEvents` 分页 ≤1000、session 跨 run 聚合、顺序定义 | 分页无重无漏；跨 run 顺序稳定 | S24 |
| S26 | Checkpoint/Session/ToolInvocation 存储 | `SaveCheckpoint` 原子 watermark、`LoadSession`、GC、invocation 状态 | 恢复只重放 watermark 之后；过期 GC 测试 | S22/S24 |

**G3 审查物：** 迁移 diff、并发测试、乱序测试、恢复测试、查询计划。

### P4 Register（S27–S31）

| 步骤 | 任务 | 产出 / 审查物 | 验收（可验证） | 依赖 |
|---|---|---|---|---|
| S27 | App 注册与密钥 | upsert by name、api_key 生成/hash/rotate、状态、D7 bootstrap | 明文仅返回一次；重复 name 幂等；越权更新拒绝 | S22 |
| S28 | ToolDef 管理 | local 全量替换/version++、global admin CRUD、schema 校验、唯一约束 | local 遮蔽数据准备正确；非法 schema 拒绝 | S27 |
| S29 | MCP 目录与密钥引用 | mcp CRUD、transport 校验、`auth_ref` 解析接口（env/file）、脱敏 | 未引用不下发；日志/响应无明文密钥 | S27 |
| S30 | 工具解析与 ACL | `ResolveTool`：local 优先、global ACL、禁用/缺失错误 | 冲突/越权/不存在矩阵测试 | S28/S29 |
| S31 | TaskAssign 材料组装 | local+global 合并、mcp 解析、model 凭据、缓存失效 | 组装快照与注册变更一致；密钥不落日志 | S29/S30 |

**G4 审查物：** 注册 API 样例、遮蔽/ACL 矩阵、密钥脱敏截图、缓存失效测试。

### P5 Connection 完整化（S32–S37）

| 步骤 | 任务 | 产出 / 审查物 | 验收（可验证） | 依赖 |
|---|---|---|---|---|
| S32 | DaemonHub 生命周期 | Hello 3s、token 校验、重复连接、labels/strategies、优雅关闭 | 鉴权失败关闭；重复连接不双写 | S17/S31 |
| S33 | 出站写入与背压 | 单写 goroutine、队列 256、控制/持久/ephemeral 分类、慢消费者判定 | 阻塞客户端下控制消息不丢、ephemeral 计数丢弃 | S32 |
| S34 | 入站分发与心跳 | 全消息分发、panic 隔离、10s/30s 心跳、容量校准、offline 事件 | 假时钟下 30s 判死；容量漂移可校准 | S32 |
| S35 | AppHub WS 基础 | ws_token 鉴权、app 绑定、连接注册、fanout、订阅归属校验（D11） | 跨 App 订阅拒绝；多连接同时收到 | S12/S32 |
| S36 | local tool 挂起表 | `CallLocalTool`、pending map、结果路由、超时/取消/断线快速失败（D10） | 重复结果幂等；断线立即 `APP_UNREACHABLE` | S35 |
| S37 | 背压与连接测试 | 慢消费者断连、指标、日志、压力测试 | 持续慢消费者 5s 断开并可重连对账 | S33/S36 |

**G5 审查物：** 背压策略测试、断线/重连日志、挂起表竞态测试、连接指标。

### P6 Control 完整化（S38–S45）

| 步骤 | 任务 | 产出 / 审查物 | 验收（可验证） | 依赖 |
|---|---|---|---|---|
| S38 | TaskQueue 接入层 | `internal/control` 适配改造后的 TaskQueue：入队、领取、Ack/Nack、延迟超时、取消、DLQ 观测 | 重复投递幂等；延迟任务准时；队列故障可观测 | W08/S23 |
| S39 | Submit 与幂等 | 创建 Run + `Enqueue(RUN_DISPATCH)`（同事务/outbox）、D8 语义、503 映射 | 同 key 同 payload 返回同 Run；不同 payload 409；不丢任务 | S38 |
| S40 | 调度器 | runtime/容量/labels/strategy 过滤、least-loaded、longterm 粘滞 | 表驱动选主测试；满载不分配 | S31/S38 |
| S41 | 下发与 Ack | TaskAssign 组装/发送、5s Ack、拒绝换候选、容量预留/释放 | 拒绝/超时都能换 daemon；容量无泄漏 | S40 |
| S42 | 重投与超时 | 无候选 2s→5min 退避、run_timeout 延迟消息、超限终态 | 假时钟下退避曲线正确；超时必终态 | S41 |
| S43 | 状态机与 attempt | 全迁移表、normal=2/longterm=3、终态不可逆、D6 单一写者 | 所有边测试；事件不覆盖 Control 状态 | S39/S41 |
| S44 | 取消链路 | queued/assigned/running 取消、`Cancel` 下发、挂起工具取消、幂等 | 取消与完成竞态只产生一个终态 | S43 |
| S45 | 对账与重启恢复 | Hello `running_run_ids` 三表、daemon lost 重排队、MW 重启恢复 | 12.2 三行逐条测试；lease 防双跑 | S34/S43 |

**G6 审查物：** 调度选主表、退避曲线、竞态测试、12.2 对账报告。

### P7 Tool Gateway（S46–S50）

| 步骤 | 任务 | 产出 / 审查物 | 验收（可验证） | 依赖 |
|---|---|---|---|---|
| S46 | Invoker 路由 | 解析/ACL/超时归属/local vs global/未知工具 | 三分支路由测试；超时码正确 | S31/S36 |
| S47 | 幂等与 single-flight | 缓存→DB、in-flight future、结果落库、TTL | 并发同 invocation 只执行一次；完成后命中缓存 | S46 |
| S48 | `builtin:http` | 模板渲染、method/url/headers/body、超时/重试、响应大小限制 | httptest 覆盖成功/4xx/5xx/超时/大响应 | S47 |
| S49 | `builtin:webhook` | 预注册 URL、payload、响应映射、重试边界 | 与 HTTP 共用执行器；配置校验 | S48 |
| S50 | Gateway 错误与审计 | `TOOL_*` 错误映射、invocation 审计、日志脱敏 | 错误码矩阵测试；审计行完整 | S47/S49 |

**G7 审查物：** 路由矩阵、并发去重测试、HTTP executor 测试、审计样例。

### P8 App API/WS（S51–S55）

| 步骤 | 任务 | 产出 / 审查物 | 验收（可验证） | 依赖 |
|---|---|---|---|---|
| S51 | HTTP 中间件 | Bearer、App 作用域、错误信封、请求 ID、body 限制、限流（可选） | 越权/超限/非法 JSON 统一错误 | S27 |
| S52 | REST 全量 | apps/runs/events/cancel/sessions 端点、状态码、分页 | OpenAPI 与实现一致；契约测试 | S51 |
| S53 | ws_token 与 WS | 签发/校验 TTL 10min、连接、subscribe、补拉、`caught_up`、ping/pong | 过期拒绝；断线重连无重无漏 | S35/S52 |
| S54 | `local_tool_result` 闭环 | correlation/invocation 校验、幂等结果、未知调用处理、`run_completed` | 重发返回缓存；伪造 App 拒绝 | S36/S53 |
| S55 | App 契约验收 | OpenAPI 完整、curl 示例、SDK 片段、错误码表 | 外部不读源码可完成创建/订阅/工具回调 | S52/S54 |

**G8 审查物：** OpenAPI、curl 全流程、重连补拉演示、错误码表。

### P9 韧性/可观测（S56–S61，M2/M3）

| 步骤 | 任务 | 产出 / 审查物 | 验收（可验证） | 依赖 |
|---|---|---|---|---|
| S56 | Go 全链路 E2E | fake app + fake daemon：local+global 工具、事件回放 | 单测/集成/脚本三层可重复 | S50/S55 |
| S57 | 重写后的 Harness 真实 E2E | echo→react，MCP 可选；事件语义核对 | 真实 LLM 一轮含工具调用通过 | S56/H07 |
| S58 | 故障矩阵 | §12.1 每行自动化：断线/重连/重复/超时/重启/workflow 故障 | 矩阵报告逐条通过；已知限制标注 | S45/S56 |
| S59 | 可观测验收 | §14.2 全指标、trace span、日志字段、脱敏 | 仪表盘/查询示例；无 PII/密钥 | S08/S58 |
| S60 | 压力与背压 | N run 并发、慢消费者、队列上限、内存/句柄 | 报告含拐点与降级行为；无泄漏 | S37/S58 |
| S61 | 安全测试 | 认证矩阵、ACL、ws_token 过期、跨 App、密钥脱敏、gRPC 明文风险 | 渗透清单通过；残余风险记录 | S55/S58 |

**G9 审查物：** 故障矩阵报告、压测报告、安全清单、M2/M3 验收记录。

### P10 longterm（S62–S65，M4）

| 步骤 | 任务 | 产出 / 审查物 | 验收（可验证） | 依赖 |
|---|---|---|---|---|
| S62 | Session 与 Checkpoint | 创建/续跑、20 事件/60s 触发、watermark 原子性 | 恢复事件无重无漏；checkpoint 不丢 | S26/S45 |
| S63 | 粘滞调度与恢复 | 优先原 daemon，失败换 daemon + `LoadSession` | daemon 迁移后从 checkpoint 续跑 | S62 |
| S64 | 跨重启恢复与 GC | MW/daemon 重启、expired 标记、blob 7 天清理 | 演示脚本通过；GC 不误删活跃会话 | S63 |
| S65 | M4 验收与偏差 | 演示脚本、DESIGN 偏差附录更新 | 验收人可按脚本复现 | S64 |

**G10 审查物：** 跨重启录像/日志、checkpoint 快照、GC 测试。

### P11 sandbox（S66–S70，M5）

| 步骤 | 任务 | 产出 / 审查物 | 验收（可验证） | 依赖 |
|---|---|---|---|---|
| S66 | daemon 镜像 | Dockerfile、依赖锁定、非 root、健康检查 | 镜像可连接 MW 并跑 echo | S57 |
| S67 | 预热池 | k8s Deployment、N 空闲、labels、`max_concurrency=1` | 新 Run 秒级分配；池容量可观测 | S66 |
| S68 | 网络隔离 | NetworkPolicy：仅 MW gRPC + LLM 端点；DNS 策略 | egress 拦截测试；必要域名白名单 | S67 |
| S69 | 回收与 draining | K run 后重建、draining 接口、优雅退出 | 存量跑完再下线；不接新 Run | S67 |
| S70 | M5 验收 | 不可信输入隔离执行、egress 验证、残余风险 | 验收报告 + 运维手册 | S68/S69 |

**G11 审查物：** 镜像扫描、k8s 清单、egress 测试、回收演练。

---

## 4. 每一步的审查与交付协议

为保证“逐步审查”，每个 `Sxx / Wxx / Hxx` 按以下节奏执行：

1. **开工前（仅当改动 > 2 个文件）**：给出不超过 1 页的设计说明：改哪些文件、关键接口、测试点、风险。
2. **实现**：只做该步骤范围内的事；不顺手重构无关代码；不提前实现后续步骤。
3. **自检**：`make test`、`make lint`、该步骤的验收命令；输出保存在 PR 描述或审查消息中。
4. **提交审查**：给出 `git diff --stat`、关键 diff、测试输出、已知风险；**等待确认后再提交/进入下一步**。
5. **Gate**：每阶段结束跑该阶段验收，更新本计划 `状态` 列，必要时更新 `DESIGN.md` 偏差附录。

**完成定义（DoD）：** 代码 + 单测 + 必要集成测试 + 文档/注释 + 无未登记的 TODO + 无敏感信息日志 + 可重复的验收命令。

**建议节奏：** 先审 S01–S03 的 ADR；确认后按“一次一个 S/W/H”推进。S01/S02 的边界未冻结前，不要开始 W/H 的实现；W08 与 H07 各自 Gate 通过后，再进入 M1 垂直切片。

---

## 5. 测试策略

| 层级 | 范围 | 工具/方式 |
|---|---|---|
| 单元 | 错误映射、协议构造、调度过滤、状态机、模板渲染 | Go `testing` + 表驱动 |
| 存储集成 | 迁移、幂等、并发 CAS、乱序、checkpoint 原子性 | 真实 SQLite 文件（WAL）；必要时 `:memory:` |
| Workflow 集成 | 租约、并发消费、延迟/退避、续租、重启、DLQ、取消竞态 | 真实 SQLite + 假时钟 + 属性测试 |
| 契约 | proto lint/breaking、HTTP/WS JSON schema、OpenAPI | buf、黄金文件、契约测试 |
| 端到端 | fake app + fake daemon，local+global 工具 | Go 集成测试 + 脚本 |
| 真实链路 | Python harness echo/react/MCP | docker-compose 或本地进程 |
| 故障 | §12.1 全矩阵 | 故障注入开关 + 假时钟 |
| 压力 | 并发 Run、慢消费者、队列上限 | 脚本 + pprof/指标 |
| 安全 | 认证/ACL/越权/密钥脱敏/WS token | 表驱动 + 手工渗透清单 |

**测试纪律：** 每个步骤的验收必须可脚本化；不能只靠人工点击。涉及时间的逻辑（心跳、Ack、退避、超时）必须可注入 clock。

---

## 6. 风险与缓冲

| 风险 | 影响 | 缓解 |
|---|---|---|
| Workflow 改造与 Harness 重写同时进行 | 两条关键路径都延期，M1 联调风险高 | 各自独立 Gate；先做最小可用（W01–W05、H01–H04），接口冻结后并行；Go/Python 分人 |
| Workflow 租约/重试语义有缺陷 | 任务重复执行或丢失 | W02 先定状态图；W03/W04/W08 做并发、重启、属性测试；Control 消费保持幂等 |
| 现有引擎有大量未提交改动 + 大规模删除 | 误删/不可回退 | W01 先建分支或打 tag；W06 前不删旧包；按审计清单逐包删除并跑测试 |
| SQLite 单写者/锁竞争 | 并发 Claim 阻塞、busy timeout | WAL + `busy_timeout` + `BEGIN IMMEDIATE`；W08 压测；v1 单 MW 实例 |
| RunStore 与 TaskQueue 双库 | Run 创建成功但任务未入队 | D13 优先统一 SQLite；若分库则 outbox + 启动补偿扫描 |
| Harness 裁剪过度或不足 | M1/M2 返工，或后期补能力 | S02 用能力矩阵冻结边界；H04/H05/H06 分层验收；能力变更走 ADR |
| Run 归属/seq 设计缺口 | 重试丢事件、重启双跑 | D4/D5 在 S09/S22 前定稿，加 lease 与 attempt |
| 状态双写 | 状态回退/竞态 | D6：Control 单一写者；事件只观测 |
| 单 MW 实例 | 无 HA，重启有窗口 | v1 明示；workflow 重投 + daemon 对账兜底；HA 另立 ADR |
| 密钥下发 | daemon 内存泄露 | 网络可信 + 脱敏 + 审计；v2 短期凭据代理 |
| sandbox egress 通过 LLM API 外渗 | 数据安全 | 记录为残余风险；M5 验证 egress 白名单；Q1 评估代理 |
| 范围蔓延（GUI/多租户计费/跨 region） | 里程碑失控 | 严格按 §1.2 非目标；新需求进 backlog |

---

## 7. 已确认事项与下一步

已确认：

1. **D1**：以现有 `workflow_engine` 为基础改造为 SQLite **RunTaskQueue**；不再从零写，也不直接复用 DAG/Effect/Saga 层。
2. **D2**：重写 Harness，做阉割适配版；在 `harness/` 实现，只保留 Channel、ReAct、LLM、工具/MCP、checkpoint。
3. **D3–D6**：接受单 MW 实例、daemon 归属/租约字段、事件按 `(run_id, attempt, seq)`、Control 单一状态写者。

仍需你确认（影响 W 步骤）：

4. **D13**：middleware 的 RunStore 是否与 TaskQueue 共用同一个 SQLite 文件？建议 v1 统一 SQLite，避免双库 outbox。
5. **D14**：`workflow_engine` 改造期保持独立 module，还是立即并入 `internal/`？建议先独立、后合并。
6. **D15/D16**：Go 1.26 工具链安装；是否允许删除 DAG/Effect/Saga 并打归档 tag。
7. **D7–D12**：目前按“建议默认”执行；如无异议，后续步骤直接落地。
8. **仓库形态**：本计划默认 Go middleware 与 Python Harness 先放在同一仓库（`cmd/`、`internal/`、`harness/`），Harness 可独立拆分。

下一步：审 `docs/WORKFLOW_ENGINE_AUDIT.md` 与 `docs/adr/0001-workflow-engine.md`（S01）。确认删除清单后，W01 先冻结当前 `workflow_engine` 的未提交改动，再进入改造。

---

## 附录 A：`DESIGN.md` 需要补齐/修订的点

| # | 位置 | 问题 | 建议 | 阻塞 |
|---|---|---|---|---|
| A1 | §3.2 `runs` | 缺 `assigned_daemon_id/lease_epoch/assigned_at`，§12.2 无法判断 Run 归属 | 增列；`Hello.running_run_ids` 改为携带 `(run_id, lease_epoch)` | S22/S45 |
| A2 | §3.2/§4.1 | `(run_id,seq)` 主键与重试 attempt 冲突；新 daemon 从 1 开始会被当重复 | 改 `(run_id, attempt, seq)`；`Event.attempt` | S09/S24 |
| A3 | §4.1/§10.2 | MW 生成的终态事件（timeout/cancel/daemon_lost）没有 seq 规则，与 daemon 单写者冲突 | 增 `source`，MW 事件独立命名空间；API 按 `(attempt, ts_ms, source)` 排序 | S22/S24 |
| A4 | §5.1/§10.2 | Control `Transition` 与 `RUN_STATUS` 投影双写 | Control 唯一状态写者；事件只推进 `last_event_seq` | S23/S43 |
| A5 | §4.2 | `POST /apps` 需要 app key，但新 App 没有 key（引导问题） | 增加 `/admin/apps` bootstrap token；`/apps` 仅更新自身 | S27/S51 |
| A6 | §3.2 | `idempotency_key` 全局 UNIQUE，未定义同 key 不同 payload | 改 `(app_id, key)` + `request_hash`；冲突 409 | S39 |
| A7 | §13.2 | 未指定 secret provider | `auth_ref` 解析接口 + env/file v1，Vault 后续 | S29 |
| A8 | ADR-3/§8.1 | 单二进制但未声明单实例；DaemonHub 内存态，多副本会双投 | v1 单活；HA 另立 ADR | S01/S45 |
| A9 | §4.3 | `subscribe` 未要求校验 Run 归属 | 强制 `run.app_id == token.app_id` | S35/S53 |
| A10 | §5.4/§8.2 | App 断线时挂起 local tool 是等 60s 还是立即失败未定义 | 立即 `APP_UNREACHABLE`，并通知 App 取消 | S36 |
| A11 | §11.2 | Checkpoint 与 `event_watermark` 原子性未定义 | 同事务写入；恢复只重放 watermark 之后 | S26/S62 |
| A12 | §4.1 | `TaskAssign` 含明文 api_key，未定义日志/大小限制 | 统一脱敏；设置 gRPC max message size；审计 | S31/S51 |
| A13 | §6.1/§12.1 | `Submit` 先写库还是先 publish 未定义；工作流不可用时可能丢任务 | 事务 outbox 或“提交后扫描 queued 补偿”，二选一并测试 | S38/S39 |
| A14 | §6.2 | longterm 粘滞需要记录上次 daemon，但 `sessions` 无字段 | 增 `last_daemon_id` 或用 `runs.assigned_daemon_id` 推导 | S40/S63 |
| A15 | §4.1 | `Reconcile` 只处理“MW 认为终态但 daemon 在跑”；MW 认为 running 但归属未知时无解 | 依赖 A1 的 lease；定义未知归属的降级策略 | S45 |
| A16 | §6.1/§6.2 | 改造为 TaskQueue 后，Run 创建与任务入队的事务边界未定义 | 同事务写 `runs + run_tasks`（outbox 等价），或提交后补偿扫描；二选一并测试 | W03/S39 |
| A19 | §2.1/§3.3/§12.1 | DESIGN 假定 PostgreSQL/Redis/MQ；当前落地为本地 SQLite 工作流引擎 | 明确 v1 本地模式：SQLite 为唯一持久化，Redis/MQ 非必需；扩缩容时再切 Postgres | S01/S07/S15 |
| A20 | §15 | `workflow_engine` 是嵌套独立 Go module（含未提交改动），与 middleware 主模块边界未定义 | 改造期独立、W08 后评估合并；先打归档 tag，避免删除不可回退 | W01/W06 |
| A17 | §1.1/§4.1/§11 | 精简 Harness v1 只实现 `react`，与文档中的 `plan_execute`、多 Agent 叙述不一致 | 在 DESIGN 标注 v1 能力矩阵；策略接口保留，`plan_execute` 延后 | S02/H04 |
| A18 | §16 | Workflow 改造 + Harness 重写后，原 M1–M4 四周路线不成立 | 用本计划 85 步与 60–80 人日重排里程碑；每个 Gate 更新偏差附录 | S03 |

---

## 附录 B：本计划状态

| 阶段 | 状态 | 备注 |
|---|---|---|
| P0 | 待审 | 已产出 `WORKFLOW_ENGINE_AUDIT.md`；下一步 S01 定稿 ADR |
| P1 | 未开始 | 依赖 P0 |
| P1.5 Workflow 改造 | 待审 | 已识别 13.4k 行引擎的删除/保留矩阵；等 D13–D16 |
| P1.6 Harness | 未开始 | 依赖 S02/S04；关键路径，可与 W 并行 |
| P2（M1） | 未开始 | 依赖 W/H Gate |
| P3–P8（M2） | 未开始 | 依赖 M1 |
| P9（M3） | 未开始 | 依赖 M2 |
| P10（M4） | 未开始 | 依赖 M3 |
| P11（M5） | 未开始 | 依赖 M4 |

**已完成：** 计划评审 v0.2；D1/D2 改成自研；D3–D6 确认；v0.3 加入现有 SQLite 引擎审计与改造方案。
