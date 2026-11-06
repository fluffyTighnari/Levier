<!-- Middleware — full trace of the current workflow layer. -->

# Workflow 层现状全量追踪

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-06 |
| 范围 | `internal/workflow/` 全部生产文件与测试文件 |
| 目标 | 说清楚输入怎么进、输出怎么出、内部怎么走、哪些设计藏在代码里 |
| 结论 | 当前 Workflow 是一个**只操作 PostgreSQL 任务表的库**；还没有接 HTTP、gRPC、Control 或 daemon |

---

## 0. 一句话概括

当前 Workflow 层只负责一件事：

> 把“某个 Run 要做某个动作”变成 `run_tasks` 表里的一行，并保证这一行被领取、续租、完成、重试、取消或进入死信。

它**不创建 Run**，**不调用 daemon**，**不发送事件**，**不执行 Agent 循环**。这些都由调用方（Control、Worker 的 Handler）完成。

---

## 1. 输入与输出

### 1.1 输入

| 输入 | 从哪里来 | 进入哪个函数 | 说明 |
|---|---|---|---|
| PostgreSQL 连接池 | 调用方 | `NewPostgres(pool)` | Workflow 不自己建库 |
| `Task` | Control / 调度层 | `Enqueue` / `EnqueueTx` | 必须是 PENDING；`run_id` 必须已在 `runs` 表存在 |
| `pgx.Tx` | Control | `EnqueueTx` | 让 Run 创建与任务入队同事务 |
| `owner` + `ttl` | Worker | `Claim` | owner 是 worker 实例标识；ttl 是租约时长 |
| `Handler` | Control | `NewWorker` / `NewPool` | 真正执行“派发/重试/超时/对账”的代码 |
| `lease` + `ttl` | Worker | `Extend` | 心跳续租 |
| `task` + `lease` | Worker | `Ack` / `Nack` / `Fail` | 完成、重试、永久失败 |
| `run_id` + reason | Control | `CancelRun` / `CancelTask` | 取消尚未完成的任务 |
| `now` + `limit` | 维护循环 | `ReapExpired` | 回收过期租约 |
| 查询参数 | 运维/Control | `ListDead` / `Stats` | 死信和队列统计 |

### 1.2 输出

| 输出 | 形式 | 说明 |
|---|---|---|
| `run_tasks` 行 | PostgreSQL | 任务状态、租约、attempt、错误、payload |
| `Task` / `Lease` | Go 返回值 | 给 Handler 使用 |
| Handler 副作用 | 由调用方提供 | 目前还没有 Control Handler，所以不会调用 daemon |
| `DeadLetterHandler` 回调 | Go 回调 | 任务进入 DEAD 时通知调用方 |
| `Stats` | Go 返回值 | 队列深度、按状态/类型计数、最老 PENDING |
| `MetricsSnapshot` | Go 返回值 | 进程内计数与平均耗时 |
| 错误 | Go 返回值 | 没有任务、租约丢失、任务终态、非法输入等 |

### 1.3 调用关系

```text
Control
  │  Enqueue / EnqueueTx
  ▼
PostgresStore ───────────────► PostgreSQL run_tasks
  ▲                              │
  │ Claim / Ack / Nack / Fail     │
  │ Cancel / Reap / Stats         │
  │                              ▼
Worker / Pool ──► Handler ──► Control 的派发/对账逻辑
  │
  └── DeadLetterHandler / Metrics / OnError

维护循环 ──► ReapExpired
运维/Control ──► ListDead / Stats
```

**当前没有的连线：**

- `internal/workflow` 没有 import `internal/control`。
- `cmd/middleware` 没有启动 Worker/Pool。
- `Handler` 还没有实现，所以实际 daemon 派发尚未接上。
- 没有 `/metrics` HTTP 端点，指标只在进程内。

---

## 2. 数据模型

### 2.1 `Task`

文件：`internal/workflow/task.go`

| 字段 | 含义 |
|---|---|
| `ID` | 任务 ID，ULID |
| `RunID` | 所属 Run |
| `Type` | `RUN_DISPATCH` / `RUN_RETRY` / `RUN_TIMEOUT` / `RUN_RECONCILE` |
| `Status` | `PENDING` / `LEASED` / `DONE` / `DEAD` / `CANCELLED` |
| `AvailableAt` | 最早可被领取时间；延迟任务靠它 |
| `Attempt` | 已被领取次数 |
| `MaxAttempts` | 最大领取次数 |
| `LeaseOwner` | 当前持有租约的 worker |
| `LeaseToken` | 租约代次，防止旧 worker 写入 |
| `LeaseExpiresAt` | 租约到期时间 |
| `LastError` | 最近一次错误 |
| `Payload` | 调度所需 JSON 对象，不放密钥 |
| `CreatedAt` / `UpdatedAt` | 创建/更新时间 |

### 2.2 `TaskType`

| 类型 | 默认最大领取次数 | 用途 |
|---|---:|---|
| `RUN_DISPATCH` | 5 | 选 daemon、发 TaskAssign、等 TaskAck |
| `RUN_RETRY` | 3 | daemon 丢失/拒绝后把 Run 放回队列 |
| `RUN_TIMEOUT` | 3 | 到点强制 Run 超时 |
| `RUN_RECONCILE` | 3 | daemon 重连后对账 |

### 2.3 `Status`

| 状态 | 是否活跃 | 是否终态 | 含义 |
|---|---|---|---|
| `PENDING` | 是 | 否 | 等待领取 |
| `LEASED` | 是 | 否 | 被某个 worker 持有 |
| `DONE` | 否 | 是 | Handler 成功 |
| `DEAD` | 否 | 是 | 重试耗尽或永久失败 |
| `CANCELLED` | 否 | 是 | 被显式取消 |

### 2.4 `Lease`

文件：`internal/workflow/store.go`

| 字段 | 含义 |
|---|---|
| `TaskID` | 任务 ID |
| `Owner` | 租约持有者 |
| `Token` | 租约代次 |
| `ExpiresAt` | 到期时间 |

### 2.5 `Stats`

| 字段 | 含义 |
|---|---|
| `Total` | 任务总数 |
| `ByStatus` | 按状态计数 |
| `ByType` | 按类型计数 |
| `OldestPending` | 最老 PENDING 的 `available_at` |
| `OldestPendingAge` | 最老 PENDING 的年龄 |

### 2.6 数据库表 `run_tasks`

迁移文件：

- `internal/db/migrations/0001_init.sql`
- `internal/db/migrations/0002_run_tasks_constraints.sql`

| 列 | 说明 |
|---|---|
| `task_id` | 主键，ULID |
| `run_id` | 外键到 `runs`，删除 Run 时级联删除任务 |
| `task_type` | 四种类型 |
| `status` | 五种状态 |
| `available_at` | 延迟/重试就绪时间 |
| `attempt` | 已领取次数 |
| `max_attempts` | 最大领取次数 |
| `lease_owner` | 租约持有者 |
| `lease_token` | 租约代次 |
| `lease_expires_at` | 租约到期 |
| `last_error` | 最近错误 |
| `payload` | JSONB |
| `created_at` / `updated_at` | 时间 |

关键约束：

- `attempt >= 0 AND attempt <= max_attempts`
- `max_attempts > 0`
- `LEASED` 必须有 owner、token、expiry
- 非 `LEASED` 必须没有 owner、expiry（token 可以保留）
- 同一 `(run_id, task_type)` 最多一个 `PENDING/LEASED` 任务
- 索引：`idx_run_tasks_ready`、`idx_run_tasks_lease`、`idx_run_tasks_run`、`idx_run_tasks_dead`

---

## 3. 文件与函数逐个追踪

### 3.0 `internal/workflow/doc.go`

只有包注释，没有函数。说明该包负责 PostgreSQL RunTaskQueue。

### 3.1 `internal/workflow/task.go`

| 函数 | 输入 | 输出 | 内部逻辑 |
|---|---|---|---|
| `TaskType.Valid()` | 接收者 | bool | 只接受四种任务类型 |
| `TaskType.DefaultMaxAttempts()` | 接收者 | int | dispatch=5，其他=3，未知=1 |
| `Status.Valid()` | 接收者 | bool | 只接受五种状态 |
| `Status.Active()` | 接收者 | bool | PENDING 或 LEASED |
| `Status.Terminal()` | 接收者 | bool | DONE/DEAD/CANCELLED |
| `NewTask(runID, taskType, availableAt, maxAttempts)` | Run ID、类型、时间、最大次数 | `Task` | 生成 ULID；默认 PENDING；空时间用当前时间；`maxAttempts<=0` 用类型默认 |
| `Task.IsActive()` | 接收者 | bool | 调用 `Status.Active()` |
| `Task.Validate()` | 接收者 | error | 校验 ULID、run_id、类型、状态、attempt 范围、LEASED 租约完整性 |

注意：

- `Task.Validate` 不检查 payload 内容。
- 入队时额外由 `normalizeForEnqueue` 拒绝“非 PENDING、预置 attempt、预置租约”。

### 3.2 `internal/workflow/store.go`

| 函数 | 输入 | 输出 | 内部逻辑 |
|---|---|---|---|
| `DeadLetterFunc.HandleDead(ctx, task, cause)` | 上下文、任务、原因 | error | 把普通函数适配成 `DeadLetterHandler` |

`Store` 是接口，列出：

- `Enqueue`
- `Claim`
- `Extend`
- `Ack`
- `Nack`
- `Fail`
- `CancelRun`
- `CancelTask`
- `ReapExpired`
- `ListDead`
- `Stats`

### 3.3 `internal/workflow/handler.go`

| 函数 | 输入 | 输出 | 内部逻辑 |
|---|---|---|---|
| `HandlerFunc.Handle(ctx, task, lease)` | 上下文、任务、租约 | error | 把普通函数适配成 `Handler` |

`Handler` 是 Worker 与业务逻辑的唯一接口。它应该做的是派发/重试/超时/对账，不是执行 Agent 循环。

### 3.4 `internal/workflow/errors.go`

| 函数 | 输入 | 输出 | 内部逻辑 |
|---|---|---|---|
| `RetryAfter(err, delay)` | 原因、延迟 | error | 包装成“带显式延迟的可重试错误”；nil 原样返回 nil |
| `RetryAfterDuration(err)` | error | 延迟, bool | 从错误链里找显式延迟 |
| `Permanent(err)` | 原因 | error | 包装成“不可重试错误” |
| `IsPermanent(err)` | error | bool | 从错误链里判断是否永久失败 |
| `retryAfterError.Error()` | 接收者 | string | 输出原错误 + 延迟 |
| `retryAfterError.Unwrap()` | 接收者 | error | 支持 `errors.Is/As` |
| `permanentError.Error()` | 接收者 | string | 输出原错误 + `(permanent)` |
| `permanentError.Unwrap()` | 接收者 | error | 支持 `errors.Is/As` |

Worker 的判定顺序：

1. `nil` → Ack
2. `IsPermanent` → Fail
3. `RetryAfterDuration` → 指定延迟 Nack
4. 其他错误 → `Backoff` 计算延迟后 Nack

### 3.5 `internal/workflow/backoff.go`

| 函数 | 输入 | 输出 | 内部逻辑 |
|---|---|---|---|
| `Backoff(attempt, base, max)` | 1-based 次数、基础延迟、上限 | `time.Duration` | 每次翻倍，封顶；防溢出；非正 base/max 用默认值 |

默认值：

- base：2 秒
- max：5 分钟

### 3.6 `internal/workflow/metrics.go`

| 函数 | 输入 | 输出 | 内部逻辑 |
|---|---|---|---|
| `NewMetrics()` | 无 | `*Metrics` | 创建空指标 |
| `Metrics.Snapshot()` | 接收者 | `MetricsSnapshot` | 读取计数，计算 Claim/Handler 平均耗时；nil 安全 |
| `Metrics.recordClaim(duration)` | 耗时 | 无 | Claims++，累加 claim 时间 |
| `Metrics.recordHandler(duration)` | 耗时 | 无 | Handled++，累加 handler 时间 |

计数：

- `Claims`
- `Acks`
- `Nacks`
- `Fails`
- `Dead`
- `LeaseLost`
- `Panics`
- `Handled`

### 3.7 `internal/workflow/postgres.go`

| 函数 | 输入 | 输出 | 内部逻辑 |
|---|---|---|---|
| `NewPostgres(pool)` | `*pgxpool.Pool` | `*PostgresStore` | 只保存连接池 |
| `PostgresStore.Enqueue(ctx, task)` | 任务 | 任务, error | 调 `enqueue`，使用连接池 |
| `PostgresStore.EnqueueTx(ctx, tx, task)` | 任务、事务 | 任务, error | 调 `enqueue`，使用事务 |
| `enqueue(ctx, q, task)` | 查询器、任务 | 任务, error | 归一化 → JSON 编码 payload → INSERT ON CONFLICT → 冲突时查询既有活跃任务 |
| `getActiveTask(ctx, q, runID, taskType)` | Run、类型 | 任务, error | 查询同 Run+类型的 PENDING/LEASED |
| `scanTask(row)` | 数据库行 | 任务, error | 扫描 14 列，JSONB 转 map，NULL 租约转零值 |
| `normalizeForEnqueue(task)` | 任务 | 任务, error | 补 ULID/PENDING/默认 attempts/时间；拒绝非 PENDING、attempt>0、预置租约；最后 `Validate` |

入队 SQL 的关键行为：

```sql
INSERT INTO run_tasks (...) VALUES (...)
ON CONFLICT (run_id, task_type)
  WHERE status IN ('PENDING', 'LEASED')
  DO NOTHING
RETURNING ...
```

- 插入成功：返回新任务。
- 冲突：返回既有活跃任务。
- 冲突任务在插入与查询之间消失：重试一次。

### 3.8 `internal/workflow/postgres_lease.go`

| 函数 | 输入 | 输出 | 内部逻辑 |
|---|---|---|---|
| `PostgresStore.Claim(ctx, owner, ttl)` | owner、TTL | 任务, 租约, error | `FOR UPDATE SKIP LOCKED` 选 PENDING、未过期、attempt<max；UPDATE 为 LEASED；token++、attempt++；返回租约 |
| `PostgresStore.Extend(ctx, lease, ttl)` | 租约、TTL | 新租约, error | 校验 owner+token；UPDATE expiry；失败时分类 |
| `PostgresStore.Ack(ctx, task, lease)` | 任务、租约 | error | 校验 task/lease；LEASED→DONE；清空 owner/expiry；失败时分类 |
| `PostgresStore.Nack(ctx, task, lease, retryAt, cause)` | 任务、租约、重试时间、原因 | error | attempt<max → PENDING + available_at；否则 DEAD；清空租约；写 last_error |
| `PostgresStore.Fail(ctx, task, lease, cause)` | 任务、租约、原因 | error | 直接 DEAD；清空租约；写 last_error |
| `leaseFailure(ctx, lease)` | 租约 | error | 查当前状态/owner/token；区分 TaskNotFound、TaskTerminal、LeaseLost |
| `validateLease(lease)` | 租约 | error | task_id/owner/token 必填 |
| `validateTaskLease(task, lease)` | 任务、租约 | error | 在 validateLease 基础上要求 task.ID == lease.TaskID |
| `errorString(err)` | error | string | nil 返回空串，否则 `err.Error()` |

`leaseFailure` 的判断：

- 任务不存在 → `ErrTaskNotFound`
- 任务终态且 token 相同 → `ErrTaskTerminal`
- 任务终态且 token 不同 → `ErrLeaseLost`（被取消/回收过）
- 任务活跃但 owner/token 不匹配 → `ErrLeaseLost`

### 3.9 `internal/workflow/postgres_cancel.go`

| 函数 | 输入 | 输出 | 内部逻辑 |
|---|---|---|---|
| `PostgresStore.CancelRun(ctx, runID, reason)` | Run、原因 | 影响行数, error | 取消该 Run 的所有 PENDING/LEASED；LEASED 的 token++；清空 owner/expiry；写 last_error |
| `PostgresStore.CancelTask(ctx, runID, taskType, reason)` | Run、类型、原因 | 影响行数, error | 只取消指定类型；其余同上 |

取消是幂等的：没有活跃任务时返回 0，不报错。

### 3.10 `internal/workflow/postgres_reaper.go`

| 函数 | 输入 | 输出 | 内部逻辑 |
|---|---|---|---|
| `PostgresStore.ReapExpired(ctx, now, limit)` | 当前时间、上限 | 回收数量, error | `FOR UPDATE SKIP LOCKED` 找过期 LEASED；attempt>=max → DEAD，否则 PENDING；token++；清空 owner/expiry；写 `lease_expired` |
| `PostgresStore.ListDead(ctx, limit)` | 上限 | 任务列表, error | 查询 DEAD，按 `updated_at DESC` |
| `PostgresStore.Stats(ctx)` | 无 | `Stats`, error | 按状态/类型计数；查最老 PENDING；用应用时钟算年龄 |

`ReapExpired` 的 `limit<=0` 默认 100；`now` 为零时用当前时间。

### 3.11 `internal/workflow/worker.go`

| 函数 | 输入 | 输出 | 内部逻辑 |
|---|---|---|---|
| `DefaultWorkerConfig(owner)` | owner | `WorkerConfig` | 30s TTL、200ms 轮询、10s 心跳、2s/5min 退避、5s 停机 |
| `NewWorker(store, handler, cfg)` | 存储、Handler、配置 | `*Worker` | 补默认配置、创建 stop/done channel、创建 Metrics |
| `Worker.SetMetrics(metrics)` | 指标 | 无 | 替换指标；nil 忽略 |
| `Worker.Metrics()` | 无 | `*Metrics` | 返回指标 |
| `Worker.SetDeadLetterHandler(handler)` | 回调 | 无 | 设置 DEAD 回调 |
| `Worker.SetErrorHandler(handler)` | 回调 | 无 | 设置循环/存储错误回调 |
| `Worker.Start()` | 无 | 无 | 创建可取消 context，启动 `run` goroutine；不可重启 |
| `Worker.Stop()` | 无 | 无 | 关闭 stop channel；等待当前 Handler；超过 ShutdownTimeout 取消 context 再等 |
| `Worker.ProcessOne(ctx)` | 上下文 | 是否处理, error | Claim → 启动心跳 → safeHandle → 停止心跳 → finish（Ack/Nack/Fail） |
| `Worker.run(ctx)` | 上下文 | 无 | 循环 ProcessOne；无任务/错误时等待 PollInterval；收到 stop/ctx 结束 |
| `Worker.safeHandle(ctx, task, lease)` | 上下文、任务、租约 | error | 调用 Handler；捕获 panic，计 Panics，转普通错误 |
| `Worker.finish(ctx, task, lease, handlerErr)` | 任务、租约、Handler 错误 | error | nil→Ack；Permanent→Fail+Dead；RetryAfter/Backoff→Nack；记录指标 |
| `Worker.heartbeat(ctx, lease, cancel, stop, done)` | 上下文、租约、取消函数、停止/完成 channel | 无 | 定时 Extend；失败时计 LeaseLost 并取消 Handler context |
| `Worker.notifyDead(ctx, task, cause)` | 任务、原因 | 无 | 调用 DeadLetterHandler；回调错误走 OnError |
| `Worker.reportError(err)` | error | 无 | 调用 OnError |
| `Worker.wait(ctx, delay)` | 上下文、延迟 | bool | 等待 delay；收到 stop/ctx 返回 false |
| `isTerminalLeaseError(err)` | error | bool | 判断 ErrLeaseLost/ErrTaskTerminal/ErrTaskNotFound |

Worker 的三条结果路径：

1. **成功**：Ack，`Acks++`。
2. **永久失败**：Fail，`Fails++`、`Dead++`，调用 DeadLetter。
3. **可重试**：Nack；如果 `attempt >= max_attempts`，`Dead++` 并调用 DeadLetter。

### 3.12 `internal/workflow/pool.go`

| 函数 | 输入 | 输出 | 内部逻辑 |
|---|---|---|---|
| `NewPool(store, handler, workerCount, cfg)` | 存储、Handler、数量、配置 | `*Pool` | workerCount 至少 1；每个 worker owner = `pool-worker-N`；共享 Metrics |
| `Pool.Size()` | 无 | int | worker 数 |
| `Pool.SetMetrics(metrics)` | 指标 | 无 | 设置所有 worker + pool |
| `Pool.Metrics()` | 无 | `*Metrics` | 返回共享指标 |
| `Pool.Stats(ctx)` | 上下文 | `Stats`, error | 代理 Store.Stats |
| `Pool.SetDeadLetterHandler(handler)` | 回调 | 无 | 设置所有 worker |
| `Pool.SetErrorHandler(handler)` | 回调 | 无 | 设置所有 worker |
| `Pool.Start()` | 无 | 无 | 启动所有 worker；重复调用无效果 |
| `Pool.Stop()` | 无 | 无 | 逐个 Stop；重复调用无效果 |

Pool **不自动运行 ReapExpired**。需要维护循环单独调用。

---

## 4. 端到端流程

### 4.1 入队

```text
Control
  → PostgresStore.Enqueue(task)
  → normalizeForEnqueue
      - 补 ID/状态/默认次数/时间
      - 拒绝非 PENDING、attempt>0、预置租约
      - Task.Validate
  → JSON 编码 payload
  → INSERT ... ON CONFLICT ... RETURNING
      - 成功：新 Task
      - 冲突：查询同 Run+类型的 PENDING/LEASED
  → 返回 Task
```

同事务版本：

```text
BEGIN
  INSERT runs (...)
  EnqueueTx(tx, task)
COMMIT
```

回滚时 Run 和 Task 都不存在。

### 4.2 领取 → 执行 → 完成

```text
Worker.run
  → ProcessOne
  → Store.Claim(owner, ttl)
      - SELECT ... FOR UPDATE SKIP LOCKED
      - UPDATE status=LEASED, token++, attempt++
  → 启动 heartbeat goroutine
  → Handler.Handle(task, lease)
  → 停止 heartbeat
  → finish
      - nil → Ack
      - Permanent → Fail + DeadLetter
      - 其他 → Nack + Backoff
```

### 4.3 重试

- `Nack` 用 SQL 的 `CASE` 判断 `attempt < max_attempts`。
- 未超限：PENDING，`available_at=retryAt`，attempt 保留。
- 超限：DEAD，写 `last_error`。
- Worker 在 Nack 成功后，如果 `task.Attempt >= task.MaxAttempts`，调用 DeadLetter。

### 4.4 取消

```text
Control.CancelRun / CancelTask
  → UPDATE active rows
  → status=CANCELLED
  → LEASED 时 token++
  → 清空 owner/expiry
  → 旧 worker 的 Extend/Ack/Nack/Fail 返回 ErrLeaseLost
```

### 4.5 过期回收

```text
维护循环.ReapExpired(now, limit)
  → SELECT expired LEASED FOR UPDATE SKIP LOCKED
  → attempt>=max ? DEAD : PENDING
  → token++
  → 清空 owner/expiry
  → 旧 worker 的写入返回 ErrLeaseLost
```

### 4.6 查询

- `Stats`：按状态/类型计数，最老 PENDING。
- `ListDead`：DEAD 任务，按更新时间倒序。
- `Metrics.Snapshot`：进程内计数与平均耗时。

---

## 5. 嵌入在逻辑里的设计功能

| 设计功能 | 藏在哪里 | 实际行为 |
|---|---|---|
| 幂等入队 | `postgres.go` + `uq_run_tasks_active` | 同 Run+类型只有一个活跃任务；冲突返回既有任务 |
| 同事务入队 | `EnqueueTx` | 调用方可用一个 `pgx.Tx` 同时写 Run 和 Task |
| 延迟任务 | `available_at` + Claim 条件 | 未到时间的任务不会被领取 |
| 租约/可见性超时 | `Claim` + `lease_expires_at` | worker 崩溃后租约过期可被回收 |
| 租约续期 | `Worker.heartbeat` + `Extend` | 慢 Handler 不会因 TTL 到期被回收 |
| 防旧 worker 写入 | `lease_token` + owner/token 校验 | 取消/回收后旧 token 写入失败 |
| 领取竞争 | `FOR UPDATE SKIP LOCKED` | 多 worker 领取不同行，不互相阻塞 |
| 任务重试 | `Nack` + `Backoff` | 可重试错误回到 PENDING；次数耗尽进 DEAD |
| 永久失败 | `Permanent` + `Fail` | 直接进 DEAD |
| 死信 | `status='DEAD'`、`ListDead`、DeadLetterHandler | 不单独建 DLQ 表；DEAD 就是死信 |
| 取消 | `CancelRun` / `CancelTask` | 只影响活跃任务；终态不变 |
| panic 隔离 | `Worker.safeHandle` | 单个 Handler panic 不杀进程，按普通错误重试 |
| 优雅停机 | `Worker.Start/Stop/run/wait` | 停止 Claim，等待当前 Handler，超时才取消 |
| 进程内指标 | `metrics.go` + Worker/Pool | 计数和平均耗时，不依赖 Prometheus |
| 数据库约束 | `0002_run_tasks_constraints.sql` | attempt、lease、状态一致性由数据库兜底 |

**明确没有嵌入的功能：**

- 不创建 Run。
- 不调用 daemon。
- 不发送事件。
- 不执行 ReAct。
- 不做 Run 级启动扫描（P6-51）。
- 不做 HTTP `/metrics`。
- 不做多进程指标聚合。

---

## 6. 测试文件逐个说明

所有真实数据库测试都受两个环境变量控制：

- `MIDDLEWARE_TEST_POSTGRES=1`
- `MIDDLEWARE_TEST_DATABASE_URL=<DSN>`

否则跳过。`testStorePool` 会为每个测试创建独立 PostgreSQL schema，设置 `search_path`，运行迁移，测试结束 `DROP SCHEMA ... CASCADE`。

### 6.1 `task_test.go`

| 测试 | 覆盖 |
|---|---|
| `TestNewTaskDefaults` | 默认 PENDING、max_attempts=5、ULID、AvailableAt |
| `TestTaskTypeDefaults` | 四种类型及默认次数、非法类型 |
| `TestStatusPredicates` | Active/Terminal/Valid |
| `TestTaskValidate` | ULID、run_id、类型、状态、attempt 上限、LEASED 租约完整性 |

### 6.2 `errors_test.go`

| 测试 | 覆盖 |
|---|---|
| `TestRetryAfter` | 错误链、显式延迟、nil |
| `TestPermanent` | 永久错误标记、错误链、nil |
| `TestHandlerFunc` | HandlerFunc 适配 |

### 6.3 `backoff_test.go`

| 测试 | 覆盖 |
|---|---|
| `TestBackoff` | 1/2/3 次、上限、0 次、极大次数、默认值 |

### 6.4 `metrics_test.go`

| 测试 | 覆盖 |
|---|---|
| `TestMetricsSnapshot` | 计数、Dead/LeaseLost/Panic、平均耗时 |
| `TestNilMetricsSnapshot` | nil Metrics 安全 |

### 6.5 `postgres_test.go`

| 函数/测试 | 覆盖 |
|---|---|
| `testStorePool` | 独立 schema、迁移、清理、环境变量跳过 |
| `TestEnqueueIdempotent` | 重复入队返回同一任务；终态后允许新任务 |
| `TestEnqueueWithRunTx` | Run + Task 同事务提交 |
| `TestEnqueueTxRollback` | 回滚后 Run/Task 都不存在 |
| `TestNormalizeForEnqueue` | 缺 Run、非 PENDING、attempt>0、预置租约、非法类型 |
| `countTasks` | 测试辅助：按 Run 统计任务行 |

### 6.6 `postgres_lease_test.go`

| 函数/测试 | 覆盖 |
|---|---|
| `TestClaimExtendAck` | Claim → Extend → Ack；第二次 Claim 无任务；旧 token Extend 失败；重复 Ack 终态 |
| `TestNackRetryAndDead` | Nack 回 PENDING、attempt 保留；第二次 Claim attempt++；达到上限进 DEAD |
| `TestFail` | Fail 直接 DEAD |
| `TestConcurrentClaimsAreUnique` | 8 个 goroutine 并发 Claim，任务 ID 不重复 |
| `TestClaimValidation` | nil pool/空 owner/空租约 |
| `insertRunForTest` | 测试辅助：插入 Run |
| `assertTaskStatus` | 测试辅助：检查状态、attempt、last_error、租约清空 |
| `idSuffix` | 测试辅助：生成简单后缀 |

### 6.7 `postgres_cancel_test.go`

| 测试 | 覆盖 |
|---|---|
| `TestCancelRunPending` | PENDING 取消；取消后可重新入队 |
| `TestCancelRunLeasedInvalidatesLease` | LEASED 取消后旧 lease 四类写入均 LeaseLost |
| `TestCancelTaskScoped` | 只取消指定 task_type |
| `TestAckBeforeCancel` | Ack 先完成，Cancel 返回 0，任务保持 DONE |
| `TestCancelAckRace` | 5 轮 Ack/Cancel 并发，只产生一个终态 |
| `TestCancelValidation` | 空 Run、非法类型 |

### 6.8 `postgres_reaper_test.go`

| 测试 | 覆盖 |
|---|---|
| `TestReapExpiredReturnsToPending` | 过期租约回 PENDING；旧 Ack 失败；可再次 Claim |
| `TestReapExpiredAtMaxGoesDead` | attempt==max 的过期任务进 DEAD |
| `TestReapExpiredLimit` | limit=2 分两批回收 3 个任务 |
| `TestStatsAndListDead` | 状态/类型计数、最老 PENDING、DEAD 查询 |

### 6.9 `postgres_fault_test.go`

| 测试 | 覆盖 |
|---|---|
| `TestDelayedTaskNotClaimedEarly` | 未来 `available_at` 不被提前 Claim；改到过去后可 Claim |
| `TestCrashRecoveryAfterLeaseExpiry` | 模拟 worker 崩溃；Reap 后新 worker 接管；attempt、token 递增 |
| `TestPoolProcessesEachTaskOnce` | 4 worker 处理 20 任务，每个任务只执行一次 |
| `TestCancelledTaskIsNotExecuted` | 取消后 `ProcessOne` 不执行 Handler |
| `TestCancelNackRace` | 5 轮 Nack/Cancel 并发，接受三种合法结果 |
| `TestConnectionPoolExhaustion` | 占满连接池后 Claim 超时，不误报 NoTask |

### 6.10 `worker_test.go`

| 函数/测试 | 覆盖 |
|---|---|
| `TestWorkerProcessOneAck` | Handler 成功 → DONE；Metrics Claims/Handled/Acks=1 |
| `TestWorkerProcessOneNoTask` | 无任务返回 false |
| `TestWorkerRetryAndPermanent` | 普通错误 Nack；Permanent Fail + DeadLetter |
| `TestWorkerPanicIsRecovered` | Handler panic 不杀进程，进入重试 |
| `TestWorkerHeartbeatKeepsLease` | 慢 Handler 期间 heartbeat 续租，ReapExpired=0 |
| `TestWorkerStopGraceful` | Stop 等待 Handler；放行后任务 DONE |
| `TestPoolRunsConcurrently` | 4 worker 处理 8 任务，全部 DONE；Pool.Stats 正确 |
| `taskIDByRun` | 测试辅助：按 Run 查 task ID |

### 6.11 `benchmark_test.go`

| 基准 | 覆盖 |
|---|---|
| `BenchmarkEnqueue` | 单条 Enqueue 吞吐 |
| `BenchmarkClaimAck` | Claim + Ack 吞吐 |

当前测得的数字受 SSH 隧道 RTT 主导：

- Enqueue 约 35.6 ms/op
- ClaimAck 约 71.2 ms/op

不能代表本机数据库性能，1k tasks/s 需要在数据库本机复测。

---

## 7. 竞态是怎么被解决的

### 7.1 两个 worker 同时 Claim

- SQL 使用 `FOR UPDATE SKIP LOCKED`。
- 两个事务不会同时锁住同一行；后到者跳过已锁行。
- 测试：`TestConcurrentClaimsAreUnique`、`TestPoolProcessesEachTaskOnce`。

### 7.2 取消与 Ack 同时发生

- 取消 LEASED 任务时 `lease_token++`，并清空 owner。
- Ack 要求 owner+token 匹配。
- 结果只有两种：
  - Ack 先成功 → DONE，取消影响 0 行。
  - 取消先成功 → CANCELLED，Ack 返回 `ErrLeaseLost`。
- 测试：`TestCancelAckRace`。

### 7.3 取消与 Nack 同时发生

Nack 比 Ack 多一种合法结果：

- Nack 先回到 PENDING，Cancel 随后取消 → Nack 成功 + CANCELLED。
- Cancel 先赢 → Nack `ErrLeaseLost` + CANCELLED。
- Nack 先且 Cancel 未观察到活跃任务 → PENDING。
- 测试：`TestCancelNackRace`。

### 7.4 进程崩溃/租约过期

- worker 持有 LEASED 但不 Ack。
- 租约到期后 `ReapExpired` 把任务放回 PENDING 或 DEAD。
- token++ 让旧 worker 的写入失效。
- 测试：`TestCrashRecoveryAfterLeaseExpiry`。

### 7.5 慢 Handler

- Handler 执行时 heartbeat 定时 Extend。
- 测试中在 Handler 运行期间调用 ReapExpired，回收数为 0。
- 测试：`TestWorkerHeartbeatKeepsLease`。

### 7.6 连接池耗尽

- 占满 `MaxConns` 后带超时 Claim。
- 预期是超时错误，不是 NoTask。
- 测试：`TestConnectionPoolExhaustion`。

### 7.7 数据竞争

- 运行 `go test -race ./internal/workflow`。
- Worker/Pool/并发生成测试均通过 race detector。

---

## 8. 当前最重要的事实与缺口

1. **Workflow 还没有接入主程序。** `cmd/middleware` 只连数据库，没有创建 `PostgresStore`、`Worker`、`Pool`。
2. **没有 Control Handler。** 所以 `RUN_DISPATCH` 等任务目前不会真正派发 daemon。
3. **没有自动 Reaper。** 需要维护循环显式调用 `ReapExpired`。
4. **没有启动 Run 扫描。** 非终态 Run 缺任务时的补队由 P6-51 Control 负责。
5. **没有 HTTP 指标端点。** `Metrics` 只是进程内快照。
6. **指标不跨进程聚合。**
7. **1k tasks/s 未验证。** 当前基准通过 SSH 隧道，数字无效。
8. **没有真实进程 kill/restart 测试。** 崩溃恢复是“租约过期”模拟。
9. **没有并发同 key Enqueue 测试。** 依赖数据库唯一索引，但测试是顺序重复入队。
10. **没有 Cursor 分页的 DLQ**。`ListDead` 只有 limit。

---

## 9. 建议阅读顺序

1. `internal/workflow/task.go`：任务是什么。
2. `internal/workflow/store.go`：对外能力。
3. `internal/workflow/postgres.go`：任务怎么进库。
4. `internal/workflow/postgres_lease.go`：任务怎么被领取/完成/重试。
5. `internal/workflow/postgres_cancel.go`：怎么取消。
6. `internal/workflow/postgres_reaper.go`：崩溃后怎么回收。
7. `internal/workflow/worker.go`：循环怎么跑。
8. `internal/workflow/pool.go`：并发怎么组织。
9. `internal/workflow/metrics.go`：指标。
10. 测试文件：按第 6 节顺序看。
