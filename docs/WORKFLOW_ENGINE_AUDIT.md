# Workflow Engine 适配审计

> **决策更新（2026-11-03）：不改造，直接重写。** 旧 `workflow_engine` 整体归档到 tag `archive/dag-effect-v0.1`，新 Workflow 在 `internal/workflow` 从零实现。**存储方向已进一步调整为统一 PostgreSQL；不实现 SQLite Store。** 本审计保留为“为什么不能直接复用”的依据；具体执行以 `docs/IMPLEMENTATION_PLAN_V2.md` 为准。

| 项目 | 内容 |
|---|---|
| 审计对象 | `workflow_engine/`（module `github.com/fluffyTighnari/workflow-engine`） |
| 基线 | commit `7d69a6e` + 未提交改动（Effect/Lease/Reconcile 等） |
| 规模 | 13,466 行 Go；其中测试 4,429 行、93 个 Test 函数 |
| 日期 | 2026-11-03 |
| 结论 | **不建议直接包一层复用**。它是一个通用 DAG + Effect/Saga 引擎，middleware 只需要 Run 级持久化任务队列。保留租约/Worker/队列思路（新实现用 PostgreSQL），删除 DAG 执行、Effect、Saga、Async、HTTP server、缓存与示例。 |

---

## 1. 核心判断

当前引擎解决的是“**定义一个有依赖的 Step DAG，由进程内 Executor 同步执行，并对副作用做 Effect Journal / 对账 / 补偿**”。

middleware 需要的是“**把 Run 的调度、延迟超时、重试、取消做成持久化任务队列，由 Control 领取并调用 daemon，Run 的执行本身在 daemon 内进行**”。

两者关键差异：

| 维度 | 当前引擎 | middleware 需要 |
|---|---|---|
| 执行单元 | Step（DAG 节点） | Run（一次 agent 执行） |
| 执行位置 | 引擎进程内 Executor | 远端 Python daemon，长连接异步 |
| 执行时长 | 秒级/短任务 | 分钟级，可能 30min~24h |
| 状态推进 | `RunWorkflow` 同步跑到终态 | TaskAssign → TaskAck → 事件驱动终态 |
| 重试对象 | Step/Effect | Run 调度、daemon 丢失、超时 |
| 延迟任务 | 无 | run_timeout、退避重投必需 |
| 副作用 | Effect Journal + Saga 补偿 | 工具调用由 Gateway 幂等，Workflow 不执行副作用 |
| 队列 | 进程内 channel，重启丢失 | SQLite 持久化，可重启恢复 |

**因此：不要把 Run 伪装成一个单 Step workflow。** 这会保留 Step/Executor/Effect 的错误抽象，后续每个 daemon 交互都要绕过引擎。正确做法是把引擎裁剪成 `RunTaskQueue`。

---

## 2. 现有包清单

| 包 | 生产 LOC | 测试 LOC | 作用 | 结论 |
|---|---:|---:|---|---|
| `pkg/workflow` | 237 | 0 | WorkflowDefinition / Step / Run / StepExecution / DAG 校验 | 重写为 Task 模型 |
| `pkg/engine` | 3,163 | 2,016 | DAG 执行、Executor、Effect、Reconcile、Recover | 删除大部分，保留少量思路 |
| `pkg/store` | 2,728 | 1,068 | SQLite/Memory/Cached store、Lease、Attempt、Effect | 保留 SQLite 与 Lease 思路，重写 |
| `pkg/worker` | 776 | 819 | Queue 消费、RunLocker、Lease heartbeat、Pool | 保留 Pool/Instance 骨架，重写 Worker |
| `pkg/queue` | 137 | 147 | 进程内有界 channel | 删除；SQLite 才是队列 |
| `pkg/async` | 367 | 172 | Engine + Queue + Pool 的装配 | 删除 |
| `pkg/logging` | 275 | 98 | 结构化日志 + 每 Run 文件日志 | 保留基础 logger，删除 runfile |
| `pkg/metrics` | 371 | 109 | Workflow/Step/Effect 指标 | 重写为队列指标 |
| `cmd/server` | 488 | 0 | 通用 workflow HTTP server | 删除；middleware 自己提供 HTTP |
| `examples/*` | 495 | 0 | basic/async/custom_store/sqlite 示例 | 删除；改造后保留一个 runqueue 示例 |

---

## 3. 保留 / 重写 / 删除矩阵

### 3.1 直接删除（与 middleware 场景无关）

| 路径 | 原因 |
|---|---|
| `cmd/server/` | 独立 workflow HTTP server，与 middleware 的 Register/Control API 冲突 |
| `examples/basic/`、`examples/async/`、`examples/custom_store/` | DAG/Async/CustomStore 示例，改造后无意义 |
| `pkg/async/` | 只是旧 Engine + Queue + Pool 的装配层 |
| `pkg/queue/` | 进程内 channel，不是持久化队列；重启丢任务 |
| `pkg/engine/builtin.go` | `compute`/`double` 示例 executor |
| `pkg/engine/effects.go` | Effect 身份/结果模型，middleware 不需要 |
| `pkg/engine/execute.go` | DAG Step 执行主循环 |
| `pkg/engine/executor.go` | Step/Effect/Reconciler/Compensator 注册表 |
| `pkg/engine/reconcile.go` | Effect 对账与 Saga 补偿，不是 daemon 对账 |
| `pkg/engine/recover.go` | Step 级崩溃修复；新队列只需 Task 租约回收 |
| `pkg/engine/resolve.go` | DAG 输入解析 |
| `pkg/engine/semantics.go` | Deadlock/语义分析 |
| `pkg/engine/wait.go` | `WaitRun` 阻塞等待，Control 不需要 |
| `pkg/engine/validate.go` | DAG 依赖校验 |
| `pkg/workflow/validate.go` | DAG 依赖/ID 校验 |
| `pkg/store/cached*.go` | 进程内缓存，与“SQLite 为唯一权威”冲突 |
| `pkg/store/memory*.go` | 内存 store；测试直接用 SQLite `:memory:` 即可 |
| `pkg/store/sqlite_attempts.go` | Step Attempt Journal，middleware 用 `tool_invocations` |
| `pkg/store/sqlite_effects.go` | Effect Journal，同上 |
| `pkg/worker/lock.go` | 进程内 RunLocker；跨进程由 SQLite Lease 负责 |
| `pkg/logging/runfile.go` | 每 Run 文件日志，middleware 用统一结构化日志 |

### 3.2 保留骨架但重写

| 路径 | 保留什么 | 重写成什么 |
|---|---|---|
| `pkg/store/sqlite.go` | 连接配置（WAL/busy_timeout/单连接）、迁移思路 | 新 `run_tasks` schema；删除 DAG/Effect 表 |
| `pkg/store/sqlite_lease.go` | owner + token + expires_at 的 fencing 思路 | Task 级 `Claim/Extend/Release` |
| `pkg/worker/instance.go` | 实例 ID | 继续作为 lease owner 前缀 |
| `pkg/worker/pool.go` | worker 数量、Start/Stop 生命周期 | 改为轮询 SQLite 的 Pool |
| `pkg/worker/worker.go` | 单 worker 串行处理、panic 恢复、Stop | 改为 Claim → Handler → Ack/Nack |
| `pkg/metrics/metrics.go` | 轻量计数器/直方图 | 只保留队列深度、延迟、重试、DLQ、租约丢失 |
| `pkg/logging/logging.go` | Logger 接口 | 可保留；后续接 middleware 的 slog |
| `pkg/workflow/model.go` | RunStatus/Attempt 思路 | 改为 Task/TaskStatus/Lease |

### 3.3 新增

| 路径 | 作用 |
|---|---|
| `pkg/task/`（或 `pkg/runqueue/`） | Task、TaskType、Status、Lease、Outcome、错误分类 |
| `pkg/store/store.go` | `TaskStore` 窄接口 + sentinel errors |
| `pkg/worker/handler.go` | `Handler`/`HandlerFunc` 与 retry 语义 |
| `examples/runqueue/` | 最小可运行示例（可选） |

---

## 4. 目标模型（Run Task Queue）

### 4.1 任务类型

| TaskType | 触发 | 处理 |
|---|---|---|
| `RUN_DISPATCH` | `POST /runs` 创建 queued Run | 选 daemon → TaskAssign → 等 TaskAck |
| `RUN_RETRY` | daemon 丢失/拒绝/TaskAck 超时 | attempt++ → 重新调度 |
| `RUN_TIMEOUT` | Run 创建时延迟入队 | 到点强制终态并下发 Cancel |
| `RUN_RECONCILE` | daemon 重连/running_run_ids 不一致 | 对账并决定重排队或取消 |

### 4.2 任务状态

```
PENDING ──Claim──► LEASED ──Ack──► DONE
   ▲                  │
   │                  ├──Nack(retry)──► PENDING (available_at = now+backoff)
   │                  ├──Nack(exhausted)──► DEAD
   │                  └──CancelRun──► CANCELLED
   └──ReapExpired─────┘
```

### 4.3 API

```go
type Task struct {
    ID             string
    RunID          string
    Type           TaskType
    Status         Status
    AvailableAt    time.Time
    Attempt        int
    MaxAttempts    int
    LeaseOwner     string
    LeaseToken     uint64
    LeaseExpiresAt time.Time
    LastError      string
    Payload        map[string]any
    CreatedAt      time.Time
    UpdatedAt      time.Time
}

type Lease struct {
    TaskID    string
    Owner     string
    Token     uint64
    ExpiresAt time.Time
}

type Store interface {
    Enqueue(ctx context.Context, task Task) (Task, error) // 幂等：同 run+type 活跃任务唯一
    Claim(ctx context.Context, owner string, ttl time.Duration) (Task, Lease, error)
    Extend(ctx context.Context, lease Lease, ttl time.Duration) (Lease, error)
    Ack(ctx context.Context, task Task, lease Lease) error
    Nack(ctx context.Context, task Task, lease Lease, retryAt time.Time, cause error) error
    CancelRun(ctx context.Context, runID, reason string) (int, error)
    ReapExpired(ctx context.Context, now time.Time, limit int) (int, error)
    Stats(ctx context.Context) (Stats, error)
}

type Handler interface {
    Handle(ctx context.Context, task Task, lease Lease) error
}
```

### 4.4 SQLite schema（核心表）

```sql
CREATE TABLE run_tasks (
  task_id          TEXT PRIMARY KEY,
  run_id           TEXT NOT NULL,
  task_type        TEXT NOT NULL,
  status           TEXT NOT NULL CHECK (status IN
                     ('PENDING','LEASED','DONE','DEAD','CANCELLED')),
  available_at     INTEGER NOT NULL,
  attempt          INTEGER NOT NULL DEFAULT 0,
  max_attempts     INTEGER NOT NULL DEFAULT 3,
  lease_owner      TEXT NOT NULL DEFAULT '',
  lease_token      INTEGER NOT NULL DEFAULT 0,
  lease_expires_at INTEGER NOT NULL DEFAULT 0,
  last_error       TEXT NOT NULL DEFAULT '',
  payload_json     TEXT NOT NULL DEFAULT '{}',
  created_at       INTEGER NOT NULL,
  updated_at       INTEGER NOT NULL
);

CREATE UNIQUE INDEX uq_run_tasks_active
  ON run_tasks(run_id, task_type)
  WHERE status IN ('PENDING','LEASED');

CREATE INDEX idx_run_tasks_ready
  ON run_tasks(status, available_at, created_at);

CREATE INDEX idx_run_tasks_lease
  ON run_tasks(status, lease_expires_at);
```

**Claim 语义：** `BEGIN IMMEDIATE` → 选一条 `PENDING AND available_at <= now` → `UPDATE ... SET status='LEASED', lease_token=lease_token+1, attempt=attempt+1` → `COMMIT`。SQLite 单写者 + `busy_timeout` 保证多进程下不重复领取。

**取消语义：** `CancelRun` 将 PENDING 置 CANCELLED；将 LEASED 置 CANCELLED 并 `lease_token++`、清 owner。旧 handler 的 `Ack/Extend` 因 token 不匹配返回 `ErrLeaseLost`。

**重试语义：** `Nack` 在 owner+token 匹配时：attempt < max → PENDING + `available_at=retryAt`；否则 DEAD。

**租约回收：** `ReapExpired` 把过期 LEASED 置回 PENDING（或 DEAD），并记录 `last_error=lease_expired`。

### 4.5 与 Control 的边界

- **只租约 dispatch/reconcile 操作，不租约整个 Run。** 一次 TaskAssign/Ack 通常 <5s，TTL 30s 足够。
- Run 的长期执行状态由 daemon 事件驱动，不占用任务租约。
- `RUN_TIMEOUT` 是独立延迟任务，保证 daemon 失联时也能落终态。
- `POST /runs` 与 `Enqueue(RUN_DISPATCH)` 应同事务（如果 RunStore 与 TaskStore 共用 SQLite）；否则需要 outbox。
- 启动恢复：扫描非终态 Run，幂等 `Enqueue` 缺失的任务。

---

## 5. 改造步骤（W01–W08，与实施计划一致）

| 步骤 | 内容 | 验收 |
|---|---|---|
| W01 | 冻结基线：建分支、归档当前未提交改动或打 tag；记录 Go 工具链与测试基线 | 分支/tag 存在；`go test ./...` 基线结果记录；删除清单签字 |
| W02 | 写 ADR：Task 状态机、租约/attempt 语义、错误分类、API、非目标 | 状态图/API/失败矩阵评审通过 |
| W03 | 新建 `pkg/task` 与 SQLite `TaskStore`：Enqueue/Claim/Extend/Ack/Nack/Cancel/Reap/Stats | 并发 Claim、token fencing、取消竞态测试 |
| W04 | 实现重试/退避/延迟/DLQ/启动恢复 | 假时钟下退避曲线、过期回收、DLQ 测试 |
| W05 | 重写 Worker/Pool：轮询、heartbeat、panic 恢复、优雅停机 | 慢 handler 续租、Stop 不丢任务 |
| W06 | 删除旧代码并重写测试：DAG/Effect/Saga/Async/Queue/server/examples/cache/memory/runfile | `go list ./...` 无废弃包；全量测试通过 |
| W07 | Control 适配：Dispatch/Retry/Timeout/Reconcile handler + 幂等入队 | mock daemon 下完整调度闭环 |
| W08 | 故障与基准：进程重启、双 worker、SQLite 锁竞争、1k 任务/秒 | 报告 + 运维说明 |

> W01/W06 是破坏性步骤，必须先审查；不要在未提交改动上直接删除。

---

## 6. 删除顺序与安全措施

1. **先冻结**：`workflow_engine` 当前有大量未提交改动，先在分支上提交或打 tag `archive/dag-effect-v0.1`，否则删除后无法回退。
2. **先建新路径**：W02–W05 完成前不删旧 `pkg/engine`，避免仓库中途不可编译。
3. **按包删除**：W06 一次性删除旧包与对应测试，不保留“兼容层”；兼容层会继续携带错误抽象。
4. **保留一个可运行示例**：`examples/runqueue` 只演示 Enqueue → Claim → Ack，不保留 DAG 示例。
5. **删除后跑**：`go test ./...`、`go vet ./...`、`go build ./...`；CI 禁止引用已删除包。

---

## 7. 需要先确认的决策

| # | 决策 | 影响 | 建议 |
|---|---|---|---|
| D13 | middleware 的 RunStore 是否与 Workflow 共用同一个 PostgreSQL 数据库？ | 决定 `POST /runs + Enqueue` 能否同事务；两库需要 outbox | **v1 统一 PostgreSQL**；不实现 SQLite Store |
| D14 | `workflow_engine` 保持独立 Go module，还是并入 `internal/workflow`？ | 影响 import 路径、部署与仓库边界 | 改造期保留独立 module；W08 后再评估合并 |
| D15 | Go 工具链 | 当前环境 `go` 不在 PATH，无法编译/跑测试 | 安装 Go 1.26 或用容器；先把 `go.mod` 对齐实际版本 |
| D16 | 是否需要保留 DAG 能力给其他项目 | 删除后无法复用 | 打 tag/归档；middleware 主分支只保留 RunTaskQueue |

---

## 8. 验收标准

- [ ] `go test ./...` 全绿，且无 DAG/Effect/Saga/Async 包引用。
- [ ] 并发 2 个 worker 对同一 Run 只会成功 Claim 一次。
- [ ] 进程在 Handler 执行中崩溃，重启后过期租约被回收并重试。
- [ ] `CancelRun` 与 `Ack` 竞态只产生一个终态；旧 token 写入返回 `ErrLeaseLost`。
- [ ] 延迟任务在 `available_at` 后 1s 内被领取。
- [ ] 达到 `MaxAttempts` 的任务进入 DEAD，不无限重试。
- [ ] Control 的 dispatch/retry/timeout/reconcile 四条路径有集成测试。
- [ ] SQLite 文件在 WAL 模式下支持重启恢复；无内存队列依赖。
