# ADR-0003：v1 范围、里程碑与验收基线

> 路径说明：`agent_core/`、`harness/`、`mcp_runtime/` 已迁移到 `execution/`；本文保留历史路径。

| 项目 | 内容 |
|---|---|
| 状态 | 提议（P0-04） |
| 日期 | 2026-11-03 |
| 决策者 | middleware 项目 |
| 关联 | `docs/DESIGN.md`、`docs/adr/0001-workflow-engine.md`、`docs/adr/0002-harness.md`、`docs/adr/0004-mcp-ownership.md` |

---

## 1. 决策

middleware v1 交付 **M1–M4**：

- 单二进制 Go middleware；
- PostgreSQL 持久化；
- 精简 Python Harness；
- App HTTP + WS 契约；
- Run 级调度、工具中介、事件溯源、断线重连、longterm checkpoint；
- M5 sandbox 作为后续阶段，不在 v1 验收内。

v1 **不追求**多实例 HA、跨 region、多语言 Harness、中央 MCP Hub、GUI、计费。这些东西保留接口，但不实现。

---

## 2. 交付形态

```text
┌──────────────┐   HTTP/WS   ┌──────────────────────────┐   gRPC   ┌──────────────┐
│  App 服务    │ ──────────► │  middleware（单二进制）   │ ◄──────► │ Python       │
│              │             │  Register/Control/       │          │ Harness      │
└──────────────┘             │  Gateway/Connection      │          │ (精简版)     │
                             └────────────┬─────────────┘          └──────┬───────┘
                                          │                                │
                                          ▼                                ▼
                                   PostgreSQL 16+                    LLM / MCP
```

- **Go 1.26**，单二进制。
- **Python 3.12**，Harness 独立进程。
- **PostgreSQL 16+**，RunStore + RunTaskQueue 同库同事务。
- **不引入** MQ、Redis、SQLite、独立 MCP Hub。
- **单 MW 实例**；存储层为后续多实例保留 `SKIP LOCKED` 与 `lease_epoch`。

---

## 3. v1 范围

### 3.1 在范围内

| 模块 | 内容 |
|---|---|
| 契约 | gRPC RuntimeChannel、HTTP `/api/v1`、WS 事件流、OpenAPI |
| Workflow | PostgreSQL RunTaskQueue：dispatch/retry/timeout/reconcile、租约、attempt、DLQ |
| RunStore | Run 状态机、`(run_id, attempt, seq)` 事件、投影、session/checkpoint |
| Register | App/Tool/MCP 目录、ACL、密钥引用、`TaskAssign` 材料组装 |
| Control | 调度、TaskAssign、Ack、重试、取消、超时、对账、重启恢复 |
| Connection | DaemonHub gRPC、AppHub WS、背压、心跳、local tool 挂起表 |
| Gateway | local/global 工具路由、`invocation_id` 幂等、`builtin:http`/`builtin:webhook` |
| Harness | Channel、echo/react、LLM、工具闭环、MCP 客户端、checkpoint |
| 可观测 | 结构化日志、metrics、trace、事件回放 |
| 安全 | App API key、ws_token、daemon token、工具 ACL、密钥脱敏 |

### 3.2 明确不做

| 不做 | 原因 |
|---|---|
| 多实例 HA / leader election | v1 单活；DaemonHub 内存态 |
| 跨 region 调度 | 无多集群需求 |
| MQ / Redis | PostgreSQL 足够 v1；减少部署面 |
| SQLite 生产存储 | 单写者/无 HA；不适合共享控制平面 |
| 中央 MCP Hub | 独立数据面服务，v2 评估 |
| 多语言 Harness | v1 只有 Python；未来用 Harness 本地 sidecar |
| 多 Agent / 子 Agent 调度 | Harness 内部事件原样上报，v1 不做编排 |
| `plan_execute` | 保留 Strategy 接口，v1 不实现 |
| GUI / 计费 / 报表 | 非目标；先做 API |
| LLM 出口代理 | v1 密钥下发 Harness；风险见 DESIGN §13.2 |
| exactly-once | at-least-once + 幂等 |
| DAG / Step / Effect / Saga | 已归档旧引擎；RunTaskQueue 不建模 |
| 多租户强隔离计费 | 只记录 usage 事件 |

---

## 4. 里程碑与验收

### M1：契约与骨架（对应 P0–P4）

**交付：**

- proto v1 定稿、Go/Python stub、buf lint/breaking。
- PostgreSQL schema 最小集：`apps/daemons/runs/run_events/run_tasks`。
- RunTaskQueue：Enqueue/Claim/Ack/Nack/Cancel/Reap。
- DaemonHub：Hello、心跳、Send。
- Control：最小 dispatch。
- Harness：Channel + echo。
- HTTP：`POST /runs`、`GET /runs/{id}`、`GET events`。

**验收脚本：**

```text
POST /runs
  → run 创建为 queued
  → RunTaskQueue 生成 RUN_DISPATCH
  → Control Claim → TaskAssign
  → Harness TaskAck + echo 事件
  → run_events 落库
  → GET /runs/{id} 返回 completed
  → GET events 可完整回放
```

**通过标准：**

- 一条命令跑通 E2E。
- 事件 `(run_id, attempt, seq)` 连续、无重复。
- 进程重启后任务不丢。
- Harness 断线后 MW 能重排队。

### M2：工具与推送（对应 P5–P7）

**交付：**

- AppHub WS fanout、`subscribe(last_attempt,last_seq)`、`caught_up`。
- local tool 闭环、`APP_UNREACHABLE` 快速失败。
- global `builtin:http` / `builtin:webhook`。
- 独立 `mcp_runtime` + `mcp_bindings` + `allowed_tools` + 命名空间。
- 完整 Register：App/Tool/MCP、ACL、`AssembleTaskMaterials`。
- HTTP events 补拉、OpenAPI、ws_token。

**验收脚本：**

```text
一次 react run：
  → LLM 发起 local tool
  → MW → App WS → App 执行 → 回传
  → LLM 发起 global http tool
  → MW 进程内执行
  → LLM 发起 mcp__server__tool
  → Harness 经 McpRuntimeClient 调 mcp_runtime 执行
  → MESSAGE_COMPLETED / USAGE / completed
```

**通过标准：**

- local/global/MCP 三类工具都能闭环。
- 同一 `invocation_id` 重复调用只执行一次。
- WS 断线重连不丢持久化事件。
- 未授权 MCP 工具不可见/不可调用。

### M3：韧性（对应 P8）

**交付：**

- 取消、超时层级、重投退避、对账、幂等去重。
- 故障矩阵：App 断线、Daemon 断线、TaskAck 拒绝、MW 重启、Tool 超时、重复投递、TaskQueue 故障。
- metrics、trace、结构化日志、压测、安全清单。

**通过标准：**

- DESIGN §12.1 故障矩阵逐条通过。
- `CancelRun` 与 `Ack` 竞态只产生一个终态。
- 旧 `lease_epoch` 事件被丢弃。
- 1k 任务/秒基准报告（单机 PG）。
- 安全清单无 P0/P1 问题。

### M4：longterm（对应 P9）

**交付：**

- Session 实体、checkpoint 触发、`resume_from`。
- 粘滞调度与 daemon 迁移恢复。
- 复合 watermark `(event_attempt, event_seq)`。
- 会话 API、过期 GC。

**验收脚本：**

```text
longterm run
  → 20 条持久化事件后 checkpoint
  → 杀掉 Harness
  → MW 重排队
  → 新 Harness 从 checkpoint 恢复
  → 消息不重复、不丢失
```

**通过标准：**

- 跨进程重启可恢复。
- 恢复只重放 watermark 之后的事件。
- GC 不误删活跃会话。

### M5：sandbox（后续，不阻塞 v1）

**交付：**

- Harness 镜像、k8s 预热池、NetworkPolicy、跑后重建。
- egress 白名单、draining、回收。

**通过标准：**

- 不可信输入在隔离 pod 执行。
- egress 拦截验证。
- 残余风险记录。

---

## 5. 质量门（Definition of Done）

每个里程碑必须满足：

1. **测试**：单测 + 集成测试 + 该里程碑验收脚本；失败用例有记录。
2. **迁移**：PostgreSQL 迁移可重复执行；有回滚/前滚说明。
3. **安全**：无密钥入日志/事件/checkpoint；认证/ACL 测试通过。
4. **可观测**：关键路径有 metrics/trace/结构化日志。
5. **文档**：`DESIGN.md`、ADR、OpenAPI、运维说明同步更新。
6. **无未登记 TODO**：遗留项必须进 backlog 或风险清单。
7. **审查**：每个 `Px-yy` 完成并确认；Gate 通过才进入下一阶段。

---

## 6. 工期与人力

| 配置 | 估算 |
|---|---|
| 单人 | 55–75 个工作日 |
| Go + Python 两人并行 | 8–11 周 |
| 三人（Go/Workflow、Harness、测试/部署） | 7–9 周 |

不含评审等待、环境审批、LLM/MCP 真实账号申请时间。

**关键路径：**

- P2 RunTaskQueue（PostgreSQL 租约/重试/DLQ）。
- P3 Harness（Channel + react + 工具闭环）。
- P4 M1 垂直切片。

**可并行：**

- P2 与 P3 在 proto 冻结后并行。
- P5/P6/P7 在 M1 后并行度较高。
- P8 依赖 M2/M3 主链路。

---

## 7. 约束与假设

1. v1 单 MW 实例；DaemonHub 不跨实例共享。
2. PostgreSQL 由平台/运维提供；本地用 Docker Compose。
3. 网络同集群/同 VPC 可信；可选 mTLS 不在 v1 必做。
4. Harness 进程内持有 LLM/MCP 凭据；v1 接受该风险。
5. App 通过 API key 认证；首次注册用 `/admin/apps` bootstrap。
6. Harness 只实现 Python；其他语言不在 v1。
7. MCP 连接由独立 `mcp_runtime` 建立；总控不持有连接。
8. 旧 `workflow_engine` 已归档，不进入新构建。

---

## 8. 决策索引

| 决策 | 结论 | 位置 |
|---|---|---|
| D1 Workflow | PostgreSQL RunTaskQueue，重写 | ADR-0001 |
| D2 Harness | 精简 Python Harness | ADR-0002 |
| D3 单实例 | v1 单 MW | 本 ADR |
| D4 Run 归属 | `assigned_daemon_id/lease_epoch/assigned_at` | ADR-0001/DESIGN |
| D5 事件序号 | `(run_id, attempt, seq)` + `source` | ADR-0001/DESIGN |
| D6 状态写者 | Control 唯一 | DESIGN |
| D7 首次注册 | `/admin/apps` bootstrap | DESIGN §4.2 |
| D8 幂等键 | `(app_id, key)` + `request_hash` | DESIGN §3.2 |
| D9 密钥 | env/file provider | DESIGN §13.2 |
| D10 App 断线 | local tool 立即 `APP_UNREACHABLE` | DESIGN §5.4 |
| D11 WS 订阅 | 校验 app 归属 | ADR-0002 |
| D12 Checkpoint | 复合 watermark | ADR-0002 |
| D13 存储 | 统一 PostgreSQL | ADR-0001 |
| D14 模块 | 根模块 + `internal/workflow` + `harness/` | 本 ADR |
| D15 工具链 | Go 1.26 / Python 3.12 / PG 16+ | 本 ADR |
| D16 旧能力 | 归档 DAG/Effect/Saga | 工作流引擎已归档 |
| D17 MCP | 独立 `mcp_runtime`，总控只存目录/ACL | ADR-0002、ADR-0004 |

---

## 9. 变更控制

- 本 ADR 是 v1 范围的基线。
- 新增范围必须满足以下之一：
  - 进入 M5+；
  - 有明确业务阻塞；
  - 安全/正确性缺陷。
- 范围变更必须更新：
  1. 本 ADR；
  2. `docs/MVP.md`；
  3. `DESIGN.md`（如涉及架构）；
  4. 验收脚本。
- 未更新文档的实现变更不合并。

---

## 10. 验收签署

| 里程碑 | 验收人 | 日期 | 结果 |
|---|---|---|---|
| M1 | 待定 | 待定 | 待定 |
| M2 | 待定 | 待定 | 待定 |
| M3 | 待定 | 待定 | 待定 |
| M4 | 待定 | 待定 | 待定 |
| M5 | 待定 | 待定 | 待定 |

> 每个 Gate 完成后填写；未通过必须记录阻塞项与负责人。
