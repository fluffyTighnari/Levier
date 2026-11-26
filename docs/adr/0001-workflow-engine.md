# ADR-0001：PostgreSQL RunTaskQueue（Workflow v1）

> **Historical design record.** Strategy was removed in the App/Runtime simplification step 0. The current protocol is defined by `proto/middleware/runtime/v1/runtime.proto` and `docs/DAEMON_CONTRACT.md`.


| 项目 | 内容 |
|---|---|
| 状态 | 提议（P0-02，存储方向已按推荐方案调整为 PostgreSQL） |
| 日期 | 2026-11-03 |
| 决策者 | middleware 项目 |
| 取代 | 旧 `workflow_engine`（DAG/Effect/Saga，已归档） |
| 关联 | `docs/DESIGN.md`、`docs/ARCHITECTURE.md` |

---

## 1. 背景

middleware 需要把一次 Run 的调度做成可靠、可恢复的任务，但不需要在 middleware 内执行 Agent 的 Step 逻辑：

- ReAct/Plan&Execute 内层循环在 Python Harness 中。
- middleware 只负责：创建 Run、选择 daemon、下发 TaskAssign、等待 TaskAck、处理超时/取消/daemon 丢失。
- Run 可能持续 30 分钟到 24 小时，不能用一个长事务或长租约把整个 Run 锁住。

旧 `workflow_engine` 是“DAG + Step Executor + Effect/Saga”的通用引擎，解决的是另一个问题。直接复用会引入错误抽象，因此本项目选择**从零实现一个最小的 RunTaskQueue**。

**存储选型：** 统一使用 PostgreSQL 16+。原因是 middleware 是共享控制平面，需要多实例、HA、行级锁、`SKIP LOCKED`、在线备份与事件归档；SQLite 的单写者和无网络访问不适合这个定位。为避免维护第二套 SQL 方言，v1 不实现 SQLite Store：纯单元测试用内存 fake，SQL 集成测试用 PostgreSQL testcontainers。

---

## 2. 决策

在 middleware 主模块的 `internal/state/workflow` 中实现 **PostgreSQL 持久化 RunTaskQueue**：

- 任务类型只覆盖 Run 级调度：`RUN_DISPATCH`、`RUN_RETRY`、`RUN_TIMEOUT`、`RUN_RECONCILE`。
- 任务状态：`PENDING`、`LEASED`、`DONE`、`DEAD`、`CANCELLED`。
- PostgreSQL 是唯一权威；不引入 MQ、Redis 或进程内第二套队列。
- v1 仍按单 MW 实例部署；但存储层使用 PostgreSQL，为后续多实例/HA 留出路径。
- 任务采用 **at-least-once** 投递；Handler 必须幂等。
- Workflow 只租约一次 dispatch/reconcile 操作（秒级），不租约整个 Run。
- RunStore 与 TaskQueue 使用同一个 PostgreSQL 数据库，Run 创建与任务入队同事务。

---

## 3. 目标与非目标

### 3.1 目标

1. Run 创建后，`RUN_DISPATCH` 任务持久化，进程重启不丢。
2. 支持延迟任务：`run_timeout`、重试退避。
3. 支持租约/可见性超时：worker 崩溃后任务可被重新领取。
4. 支持 attempt、最大尝试、DLQ。
5. 支持按 `run_id` 取消，并 fencing 掉旧 worker 的写入。
6. 支持启动恢复：扫描未终态 Run，幂等补齐任务。
7. 提供队列深度、最老任务年龄、Claim/Ack/Nack/Dead 指标。
8. Run 创建与任务入队同事务。
9. 为多 MW 实例预留：Claim 使用 `FOR UPDATE SKIP LOCKED`，不依赖进程内锁。

### 3.2 非目标

- 不做 DAG、Step、Input 解析、Step Executor。
- 不做 Effect Journal、Saga、补偿、外部副作用对账。
- 不做 cron、周期任务、子工作流、跨 region。
- 不做 exactly-once；只保证 at-least-once + Handler 幂等。
- 不提供独立 HTTP server；middleware 自己提供 API。
- 不持久化 token delta / 业务事件；那是 RunStore 的职责。
- v1 不做多活、leader election、跨 region 调度；但存储不阻塞后续演进。

---

## 4. 领域模型

### 4.1 TaskType

| 类型 | 触发 | Handler 职责 | 默认 MaxAttempts |
|---|---|---|---|
| `RUN_DISPATCH` | `POST /runs` 或 retry 后 | 选 daemon → TaskAssign → 等 TaskAck；成功则 Ack | 5 |
| `RUN_RETRY` | daemon lost / Ack 拒绝 / 重排队 | 将 Run 置回 queued，`run.attempt++`，再入队 Dispatch | 3 |
| `RUN_TIMEOUT` | Run 创建时按 deadline 延迟入队 | 若 Run 非终态，置 timeout 并下发 Cancel | 3 |
| `RUN_RECONCILE` | daemon 重连 / running_run_ids 不一致 | 按 lease_epoch 对账：保留、取消或重排队 | 3 |

> `RUN_RETRY` 只改变 Run 的 attempt；它不直接执行 Agent。`RUN_DISPATCH` 的 Task attempt 是“投递尝试”，与 Run attempt 不是同一个计数器，禁止混用。

### 4.2 TaskStatus

```
PENDING ──Claim──► LEASED ──Ack──► DONE
   ▲                  │
   │                  ├──Nack（attempt < max）──► PENDING（available_at = retryAt）
   │                  ├──Nack/Fail（attempt >= max）──► DEAD
   │                  └──CancelRun──► CANCELLED
   └──ReapExpired─────┘
```

- `DONE`：Handler 成功，任务终结。
- `DEAD`：达到最大尝试或不可重试错误，需要 Control 将 Run 落失败或人工处理。
- `CANCELLED`：Run 被取消或任务被显式撤销。
- `LEASED`：某个 worker 持有租约；不是终态。

### 4.3 Task

```go
type Task struct {
    ID             string          // ULID
    RunID          string
    Type           TaskType
    Status         Status
    AvailableAt    time.Time
    Attempt        int             // 已被 Claim 的次数
    MaxAttempts    int
    LeaseOwner     string
    LeaseToken     uint64          // 队列层 fencing
    LeaseExpiresAt time.Time
    LastError      string
    Payload        map[string]any  // 只放调度所需，不放密钥
    CreatedAt      time.Time
    UpdatedAt      time.Time
}
```

### 4.4 Lease

```go
type Lease struct {
    TaskID    string
    Owner     string
    Token     uint64
    ExpiresAt time.Time
}
```

`TaskID + Owner + Token` 是写入凭证；任何 `Extend/Ack/Nack/Fail` 都必须同时匹配。

### 4.5 两个 fencing 不要混淆

| 名称 | 所在层 | 作用 | 何时递增 |
|---|---|---|---|
| `run_tasks.lease_token` | 队列层 | 防止旧 worker 写任务状态 | 每次 Claim / Reap / CancelTask |
| `runs.lease_epoch` | Run 层 | 防止旧 daemon 写 Run 状态/事件 | 每次 Run 被分配给 daemon、重排队、取消 |

`RUN_DISPATCH` 成功时，在 `queued → assigned` 的 CAS 中执行 `lease_epoch++`，并把新 epoch 放进 `TaskAssign`。Harness 必须：

- 在 `Hello.running_run_ids` 中回传 `(run_id, lease_epoch)`；
- 在 `Event` 中携带 `lease_epoch`。

RunStore 的 `AppendEvent` 校验：

```text
Envelope.lease_epoch == runs.lease_epoch
```

不匹配的事件丢弃并计数 `stale_daemon_events`，避免旧 daemon 在 Run 重排队后继续污染状态/事件流。

> 注意：`Envelope.attempt` 用于重试后事件 seq 分区；`Envelope.lease_epoch` 用于识别旧 daemon。两者不能互相替代。

---

## 5. 持久化

### 5.1 表结构

```sql
CREATE TABLE IF NOT EXISTS run_tasks (
  task_id          TEXT PRIMARY KEY,
  run_id           TEXT NOT NULL,
  task_type        TEXT NOT NULL,
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

-- 同一 Run 同一类型最多一个活跃任务（PENDING/LEASED）
CREATE UNIQUE INDEX IF NOT EXISTS uq_run_tasks_active
  ON run_tasks(run_id, task_type)
  WHERE status IN ('PENDING','LEASED');

-- 领取就绪任务
CREATE INDEX IF NOT EXISTS idx_run_tasks_ready
  ON run_tasks(status, available_at, created_at);

-- 回收过期租约
CREATE INDEX IF NOT EXISTS idx_run_tasks_lease
  ON run_tasks(status, lease_expires_at)
  WHERE status = 'LEASED';
```

### 5.2 PostgreSQL 配置

- PostgreSQL 16+，推荐 `pgx` 驱动。
- 连接池：`MaxOpenConns` 按实例配置；Claim/Ack 使用短事务，不持有 `idle in transaction`。
- 隔离级别：默认 `READ COMMITTED`；`SKIP LOCKED` 已足够。
- 所有时间用 `TIMESTAMPTZ`；队列时间优先用数据库 `now()`，避免多实例时钟偏差。
- `payload` 用 `JSONB`，不存密钥。
- 迁移使用版本化 SQL（`schema_migrations`），不使用 ORM 自动建表。

### 5.3 Claim 查询

```sql
WITH candidate AS (
  SELECT task_id
  FROM run_tasks
  WHERE status = 'PENDING'
    AND available_at <= now()
  ORDER BY available_at ASC, created_at ASC
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
UPDATE run_tasks t
SET status = 'LEASED',
    lease_owner = $1,
    lease_token = t.lease_token + 1,
    lease_expires_at = now() + make_interval(secs => $2),
    attempt = t.attempt + 1,
    updated_at = now()
FROM candidate c
WHERE t.task_id = c.task_id
RETURNING t.*;
```

- `$1` 是 lease owner，`$2` 是 TTL 秒数；`FOR UPDATE SKIP LOCKED` 让多个 MW 实例/worker 并发领取不同任务，不互相阻塞。
- `attempt` 在 Claim 时递增；`MaxAttempts` 表示最多被 Claim 的次数。
- 若 `attempt >= max_attempts`，Claim 前直接转 `DEAD`，不再执行。
- 无任务时返回 `ErrNoTask`，不是错误。

### 5.4 Enqueue 幂等

```sql
INSERT INTO run_tasks (
  task_id, run_id, task_type, status, available_at,
  attempt, max_attempts, payload, created_at, updated_at
) VALUES ($1,$2,$3,'PENDING',$4,0,$5,$6,now(),now())
ON CONFLICT (run_id, task_type)
  WHERE status IN ('PENDING','LEASED')
  DO NOTHING
RETURNING *;
```

- 如果 `RETURNING` 为空，说明已有活跃任务；再 `SELECT` 返回既有任务。
- 若既有任务是 `DONE/DEAD/CANCELLED`，允许新建（用于重试/重新对账）。
- 支持 `EnqueueTx`：与 Run 创建在同一 `pgx.Tx` 内提交。

---

## 6. 操作语义

### 6.1 Claim

- 只领取 `PENDING AND available_at <= now()`。
- 成功返回 `Task + Lease`；无任务返回 `ErrNoTask`。
- 租约 TTL 默认 30s；dispatch 最大耗时 5s，留足余量。
- 连接池耗尽/序列化失败：返回可重试错误，worker 退避后继续。

### 6.2 Extend

- 必须匹配 `task_id + owner + token`。
- 只延长 `lease_expires_at`，不改变 status/attempt。
- 不匹配返回 `ErrLeaseLost`；Handler 应停止并放弃写入。

### 6.3 Ack

- 必须匹配 `task_id + owner + token`，且当前状态为 `LEASED`。
- 置 `DONE`、清空 lease、记录 `updated_at`。
- 不匹配返回 `ErrLeaseLost`；已终态返回 `ErrTaskTerminal`。

### 6.4 Nack（可重试）

- 必须匹配 lease。
- 若 `attempt < max_attempts`：置 `PENDING`，`available_at = retryAt`，清空 lease。
- 若 `attempt >= max_attempts`：置 `DEAD`，记录 `last_error`。
- 调用方（Worker）负责计算退避；默认 `min(2s * 2^(attempt-1), 5min)` + jitter。

### 6.5 Fail（不可重试）

- 必须匹配 lease。
- 直接置 `DEAD`，记录 `last_error`。
- 用于 Handler 返回 `Permanent(err)` 或协议错误。

### 6.6 CancelRun

- 对指定 `run_id` 的所有 `PENDING/LEASED` 任务：
  - `PENDING` → `CANCELLED`
  - `LEASED` → `CANCELLED`，`lease_token++`，清空 owner/expires
- 返回受影响任务数。
- 旧 worker 的 `Extend/Ack/Nack/Fail` 因 token 不匹配返回 `ErrLeaseLost`。
- `CancelRun` 与 `Ack` 并发时，只有一个能成功；以 token 为准。

### 6.7 CancelTask

- 只取消某 `run_id + task_type` 的活跃任务（例如 Run 正常完成后取消尚未到期的 `RUN_TIMEOUT`）。
- 语义同 `CancelRun`，但范围更窄。

### 6.8 ReapExpired

- 使用 `FOR UPDATE SKIP LOCKED` 分批扫描 `LEASED AND lease_expires_at <= now()`：
  - `lease_token++`，清空 owner/expires。
  - 若 `attempt >= max_attempts` → `DEAD`，`last_error = "lease_expired"`。
  - 否则 → `PENDING`，`available_at = now()`，`last_error = "lease_expired"`。
- 每次限制 `limit`，避免长事务。
- 由 Worker 的空闲 ticker 或独立 reaper goroutine 调用。

### 6.9 启动恢复

1. `ReapExpired(now, limit)` 多轮直到清空。
2. Control 扫描非终态 Run：
   - `queued` 且无活跃 `RUN_DISPATCH` → `Enqueue`。
   - `assigned/running` 且对应 daemon offline → `Enqueue(RUN_RETRY)`。
   - 非终态且无 `RUN_TIMEOUT` → 按 deadline 入队（已过期则立即）。
3. 所有 Enqueue 都是幂等的；重复启动安全。

### 6.10 重复投递

- 任务级是 at-least-once：进程崩溃、租约过期、Ack 失败都可能导致同一任务再次执行。
- Handler 必须按业务键幂等：
  - `RUN_DISPATCH`：同一 Run 重复下发时，daemon 侧以 `run_id + lease_epoch` 去重。
  - `RUN_TIMEOUT`：重复执行只产生一个 timeout 终态。
  - `RUN_RETRY`：`attempt++` 必须在 Run CAS 中完成，重复执行不重复加。
  - `RUN_RECONCILE`：以 lease_epoch 判定新旧，重复对账结果一致。

### 6.11 MW 生成事件的 seq

Daemon 是 Run 事件的单写者，但 timeout/cancel/daemon_lost 等终态可能由 MW 生成。为避免与 daemon 的 `seq` 冲突：

- Daemon 事件使用 `seq` 区间 `[1, 2^62)`。
- MW 生成事件使用保留区间 `[2^62, 2^63)`，`source='mw'`。
- 主键仍是 `(run_id, attempt, seq)`；API 按 `(attempt, seq)` 排序时，MW 终态事件自然排在 daemon 事件之后。
- MW 事件同样要满足 `(run_id, attempt, seq)` 幂等；重复生成使用同一 seq 或冲突忽略。

---

## 7. Handler 与错误分类

```go
type Handler interface {
    Handle(ctx context.Context, task Task, lease Lease) error
}

type HandlerFunc func(context.Context, Task, Lease) error
```

映射规则：

| Handler 返回 | Worker 行为 |
|---|---|
| `nil` | `Ack` |
| `workflow.RetryAfter(err, d)` | `Nack(retryAt=now+d)` |
| `workflow.Permanent(err)` | `Fail`（DEAD） |
| `context.Canceled` / `DeadlineExceeded` | `Nack`（默认退避），除非 Run 已取消 |
| 其他 error | `Nack`（默认指数退避） |
| panic | 恢复后 `Nack`，记录 panic；连续 panic 达到上限 → DEAD |

**错误分类原则：**

- `ErrLeaseLost`：不是 Handler 的业务错误；worker 记录并停止，不再写。
- `ErrNoTask`：正常空闲，不是错误。
- `ErrTaskTerminal`：重复对账/取消后到达，忽略。
- PostgreSQL 连接失败/序列化失败/死锁：可重试；worker 自身退避，不消耗业务 attempt。
- **决策：** Claim 失败不创建任务，不消耗 attempt；Ack/Nack 失败重试一次，仍失败则让租约过期回收。

---

## 8. Worker 语义

- 默认 worker 数：4；可配置。
- 轮询间隔：200ms；后续可用 `LISTEN/NOTIFY` 唤醒（v1 不强制）。
- 每次 Claim 1 个任务；处理完再 Claim，避免单 worker 并发。
- Handler 执行期间每 `TTL/3` 调用 `Extend`；`ErrLeaseLost` 时取消 Handler context。
- 优雅停机：
  1. 停止 Claim。
  2. 等待当前 Handler（受 shutdown timeout 约束）。
  3. 未完成的 Handler 用 `Nack` 放回 PENDING；若 `Nack` 失败，租约到期后回收。
  4. 关闭 DB。
- panic：恢复、记录、`Nack`；不杀进程。
- DeadLetter 回调：任务进入 `DEAD` 时通知 Control，由 Control 决定 Run 终态（例如 `failed(WORKFLOW_TASK_DEAD)`）。

---

## 9. 与 Control 的集成

### 9.1 创建 Run

```text
POST /runs
  ├─ RunStore.CreateRun(status=queued)
  └─ Workflow.Enqueue(RUN_DISPATCH, available_at=now)
     （同 PostgreSQL 事务）
```

如果未来 RunStore 与 Workflow 分库，则使用 outbox + 启动补偿；v1 同库同事务。

### 9.2 Dispatch Handler

```text
Claim(RUN_DISPATCH)
  → 加载 Run（非 queued 则 Ack）
  → 过滤 daemon（runtime/容量/labels/strategy）
  → 组装 TaskAssign（Register，携带当前 lease_epoch 候选值）
  → 发送 TaskAssign
  → 5s 内等 TaskAck
      accepted=true  → Run CAS queued→assigned + lease_epoch++；Ack task
      accepted=false → Nack（2s 退避，换候选）
      超时/无 daemon  → Nack（指数退避，上限 run deadline）
  → 达到 run deadline → 不再重试，交给 RUN_TIMEOUT
```

### 9.3 Timeout Handler

```text
Claim(RUN_TIMEOUT)
  → 加载 Run
  → 已终态：Ack
  → 非终态：CAS 到 timeout；向 daemon 发 Cancel；Ack
```

### 9.4 Retry Handler

```text
Claim(RUN_RETRY)
  → 加载 Run
  → 已终态：Ack
  → CAS：attempt++，status=queued，清 assigned_daemon_id/lease_epoch
  → Enqueue(RUN_DISPATCH)
  → Ack
```

### 9.5 Reconcile Handler

```text
Claim(RUN_RECONCILE)
  → 读取 daemon Hello.running_run_ids（含 run_id + lease_epoch）
  → 对比 runs.assigned_daemon_id / lease_epoch：
      一致 → Ack
      MW 认为终态但 daemon 在跑 → 下发 Reconcile{cancel}
      MW 认为 running 但 daemon 没有 → Enqueue(RUN_RETRY)
      epoch 不匹配 → 取消旧执行，按新 epoch 处理
```

### 9.6 Run 终态

Run 进入 `completed/failed/cancelled/timeout` 后：

- `CancelTask(RUN_TIMEOUT)`；
- 其他活跃任务按需 `CancelTask`；
- 不把 Run 事件写成 Workflow 任务。

---

## 10. 可观测性

### 10.1 Metrics

| 指标 | 类型 | 标签 |
|---|---|---|
| `levier_workflow_tasks` | gauge | type, status |
| `levier_workflow_task_age_seconds` | gauge | type（最老 PENDING 年龄） |
| `levier_workflow_claim_duration_seconds` | histogram | type |
| `levier_workflow_task_attempts_total` | counter | type, outcome |
| `levier_workflow_tasks_dead_total` | counter | type, reason |
| `levier_workflow_lease_lost_total` | counter | type |
| `levier_workflow_handler_duration_seconds` | histogram | type |

### 10.2 Logs / Trace

- 日志字段：`task_id`、`run_id`、`task_type`、`attempt`、`lease_owner`、`lease_token`。
- 不记录 `payload` 中的敏感信息；v1 payload 只放调度元数据。
- Span：`workflow.claim`、`workflow.handle`、`workflow.ack`、`workflow.nack`、`workflow.reap`。

---

## 11. 故障矩阵

| 故障 | 行为 |
|---|---|
| Handler 执行中进程崩溃 | 租约过期 → ReapExpired → PENDING/DEAD → 重新执行 |
| Handler 成功但 Ack 前崩溃 | 租约过期 → 重新执行；Handler 必须幂等 |
| PostgreSQL 连接失败/死锁 | Claim/Ack 返回可重试错误；worker 退避；不消耗 attempt |
| 租约被取消 | 旧 worker 的 Extend/Ack/Nack 返回 `ErrLeaseLost`，停止写入 |
| 两个 worker 同时 Claim | `FOR UPDATE SKIP LOCKED`，各自领取不同行 |
| Run 完成时 timeout 任务未到期 | Control 调用 CancelTask；或到期后 Handler 发现终态并 Ack |
| 重试达到上限 | 任务 DEAD；DeadLetter 通知 Control 落 Run 失败或告警 |
| 连接池耗尽 | Claim 超时；worker 退避；健康检查告警 |
| 时钟偏差 | 队列时间使用数据库 `now()`；应用层时间仅做展示/超时判断 |

---

## 12. 备选方案

| 方案 | 结论 | 原因 |
|---|---|---|
| 改造旧 DAG 引擎 | 拒绝 | 保留 Step/Executor/Effect 错误抽象，Run 长执行与 Step 短执行不匹配 |
| 包一层旧引擎，把 Run 当单 Step | 拒绝 | 仍需绕过 Effect/Reconcile/Executor，代码更绕 |
| SQLite | 拒绝作为生产 | 单写者、无网络访问、无 HA；只适合单机/边缘/测试 |
| MySQL 8 | 次选 | 可行，但部分索引、JSONB/GIN、`LISTEN/NOTIFY` 等能力不如 PG 直接；仅在组织标准要求时选 |
| 外部 MQ（Kafka/RabbitMQ） | v1 拒绝 | 增加部署复杂度；PG 的 `SKIP LOCKED` 已足够 v1 |
| 进程内 channel 队列 | 拒绝 | 重启丢任务，不满足持久化恢复 |
| exactly-once | 拒绝 | PG + 外部 daemon 无法承担；at-least-once + 幂等更现实 |

---

## 13. 后果与风险

**正面：**

- 模型与 middleware 的 Run 生命周期一致。
- `SKIP LOCKED` 支持多 worker/多实例并发领取。
- PostgreSQL 事务可保证 Run+Task 原子。
- 部分唯一索引、`RETURNING`、`JSONB`、分区、PITR 直接可用。
- 与 DESIGN §3.2 的 PostgreSQL 假设一致。

**负面/风险：**

- 多一个外部依赖；本地开发/CI 需要 PostgreSQL。
- 连接池、迁移、备份、监控需要运维。
- 任务级 at-least-once 要求所有 Handler 幂等，测试必须覆盖重复投递。
- 多实例下仍需 Run 级 `lease_epoch` fencing，不能只依赖 PG 行锁。
- 未来切 MySQL 需要重写 SQL 方言与部分索引逻辑；不要同时维护两套。

---

## 14. 验收标准

- [ ] `Enqueue` 幂等：同 Run+Type 活跃任务唯一。
- [ ] 双 worker 并发 Claim，同一任务只被一个成功领取（`SKIP LOCKED`）。
- [ ] 旧 lease token 的 `Ack/Extend/Nack/Fail` 全部返回 `ErrLeaseLost`。
- [ ] 进程崩溃后，过期租约被回收并重试。
- [ ] `CancelRun` 与 `Ack` 竞态只产生一个终态。
- [ ] 延迟任务在 `available_at` 后 1s 内被领取。
- [ ] 达到 `MaxAttempts` 的任务进入 `DEAD`，不无限重试。
- [ ] Run 创建与 `RUN_DISPATCH` 入队同事务；回滚不留任务。
- [ ] `RUN_TIMEOUT` 在 Run 正常完成后被取消或安全 Ack。
- [ ] 旧 `lease_epoch` 的 daemon 事件被丢弃并计数，不推进 Run 状态。
- [ ] 指标、日志、trace 字段齐全且无敏感信息。

---

## 15. 实施映射

| ADR 内容 | 计划步骤 |
|---|---|
| Task/Lease/Store/Handler 模型 | P2-14 |
| `run_tasks` schema | P2-15 |
| Enqueue/幂等/同事务 | P2-16 |
| Claim/Lease/Ack/Nack | P2-17 |
| Cancel/竞态 | P2-18 |
| 延迟/退避/回收/DLQ | P2-19 |
| Worker/Pool | P2-20 |
| 故障/属性测试 | P2-21 |
| 指标/基准 | P2-22 |
| Control Dispatch/Retry/Timeout/Reconcile | P6-49/P6-50/P6-51 |
