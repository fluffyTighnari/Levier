<!-- Middleware — retrofit P6.3f terminal vs requeue race. -->

# Retrofit P6.3f：终态与 requeue 竞态

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-11 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P6.3f |
| 状态 | 完成 |
| 代码提交 | `3e36fb6` |
| 触发 | `docs/reports/RETROFIT-P6.3.md` P6.3-F1 |

## 1. 故障

P6.3 真实运行结束后：

```text
run_events seq 10   RUN_STATUS cancelled
control log         control run terminal status=cancelled
runs.status         queued
runs.lease_epoch    2
```

同一个 Run，事件说已取消，运行表说在排队。

## 2. 根因

两个写入方并发，且没有共同的条件：

```text
A. 事件路径
   AppendEvent(RUN_STATUS cancelled)
   → 插入 run_events
   → Transition(running → cancelled)

B. 下线路径
   gRPC 流关闭 → HandleDaemonOffline
   → ListActiveRunsForDaemon → 读到 status=running
   → RequeueRun(running → queued, lease_epoch+1)
```

时序落入如下交错：

```text
1. 事件已插入（seq 10）
2. RequeueRun 先把 runs.status 改成 queued，lease_epoch=2
3. Transition 的 WHERE status='running' AND lease_epoch=1 不再匹配
   → ErrRunNotAssignable
4. control.go 对 ErrRunNotAssignable 静默放过
   → 仍然打印 "control run terminal"
   → 仍然 publishCompleted / Release
```

于是出现两个事实源矛盾，且 Run 可能被重试任务重新领取。

## 3. 修复

### 3.1 存储层：终态事件否决 requeue

`internal/runstore/store.go` `RequeueRun`：

```sql
UPDATE runs
SET status = 'queued', ...
WHERE run_id = $1
  AND status IN ('assigned', 'running')
  AND NOT EXISTS (
    SELECT 1 FROM run_events e
    WHERE e.run_id = runs.run_id
      AND e.attempt = runs.attempt
      AND e.type = 'RUN_STATUS'
      AND lower(coalesce(e.payload->>'status', '')) IN
          ('completed', 'failed', 'cancelled', 'timeout')
  )
```

关键：判定与写入在同一条 UPDATE 里，条件不可分。

`AppendEvent` 已对同一行 `SELECT ... FOR UPDATE`，两条路径因此串行：

```text
事件先拿到行锁
  → 终态事件落库
  → RequeueRun 看到 NOT EXISTS 为假 → 不 requeue
  → Transition 成功 → cancelled

requeue 先拿到行锁
  → lease_epoch 变成 2
  → 事件写入时发现 epoch 不匹配 → Stale，丢弃
  → Run 留在 queued 等待重试（一致状态）
```

两种交错都收敛到一致结果。

### 3.2 控制层：Transition 失败不得发布终态

`internal/control/control.go`：

```go
if _, err := c.runs.Transition(...); err != nil {
    if !errors.Is(err, runstore.ErrRunNotAssignable) {
        c.log.Warn("control transition run failed", ...)
        return
    }
    c.log.Info("control run terminal superseded", ...)
    return
}
```

不再出现「存储拒绝、日志宣称终态、客户端收到 `run_completed`」的割裂。

## 4. 验证

### 4.1 控制层单测

```text
--- PASS: TestTerminalEventAfterRequeueDoesNotPublish
--- PASS: TestTerminalEventReleasesCapacity
```

第一个用例的构造：

```text
status=running / attempt=1 / lease_epoch=1
→ 先用 RequeueRun 模拟下线路径胜出（queued / epoch 2）
→ 再投递 RUN_STATUS cancelled
断言：
  runs.status 仍为 queued
  没有 run_completed 发布
  没有 Release 容量
```

第二个用例保证正常终态仍然释放容量，未被修坏。

### 4.2 存储层集成测试（真实 PostgreSQL）

```text
--- PASS: TestListActiveRunsAndRequeue
--- PASS: TestRequeueRunKeepsTerminalAttempt
```

`TestRequeueRunKeepsTerminalAttempt`：

```text
CreateRun → AssignRun(epoch 1) → Transition(running)
→ AppendEvent(RUN_STATUS {"status":"cancelled"})
→ RequeueRun
断言：
  ErrRunNotAssignable
  status=running, lease_epoch=1（未被覆盖）
```

### 4.3 全量

```text
MIDDLEWARE_TEST_POSTGRES=1 go test -count=1 ./...
  全部包 ok

make lint
  go vet ./...
```

## 5. 影响

| 项目 | 修复前 | 修复后 |
|---|---|---|
| 事件 = cancelled，表 = queued | 可能 | 不再可能 |
| 终态后重复执行 | 可能 | 不会 |
| 错误发布 `run_completed` | 可能 | 不会 |
| 正常终态发布/释放 | 正常 | 不变 |

## 6. 当前不覆盖

- 若 requeue 先赢，本次取消事件按 `Stale` 丢弃，Run 会重试。这是既有语义，未改动。
- 未做真实 E2E 复现；P6.4 的真实 Run 会继续观察该路径。

## 7. 证据

```text
docs/reports/artifacts/retrofit/P6.3f/
  control_test.log
  runstore_test.log
  go_test_all.log
  lint.log
  summary.txt
```

## 8. 下一步

P6.4：App 返回 `local_tool_result`。

```text
收到 local_tool_call
→ 执行 counter_add(state, delta)
→ 发 local_tool_result
    correlation_id = msg_id
    invocation_id = ...
    ok = true
    result_json = ...
→ 保持连接等待下一次调用
```
