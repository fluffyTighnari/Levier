<!-- Middleware — retrofit P9 fault recovery. -->

# Retrofit P9：故障与恢复

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-11 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P9 |
| 状态 | 通过（P9.10 未验证） |
| 代码提交 | `5be9bda` |

## 1. 覆盖

| ID | 故障 | 结果 |
|---|---|---|
| P9.1 | App WS 断开 / 重连 | 通过 |
| P9.2 | 重复结果 | 见 P6D P6.12 |
| P9.3 | 错误 correlation | 见 P6D P6.13 |
| P9.4 | 伪造 App | 见 P6D P6.15 |
| P9.5 | 工具超时 | 见 P6D P6.17 |
| P9.6 | LLM 429/5xx | 通过 |
| P9.7 | Daemon 断连 | 通过 |
| P9.8 | MW 重启 / 恢复 | 通过（修复后） |
| P9.9 | Queue/DB 不可用 | 通过（修复后） |
| P9.10 | 慢消费者 | 未验证（见 §8） |

## 2. P9.1 断开与重连

```text
run      01M74KZJH8MQDRDJ63R4DAY96D
宿主     只处理 1 次调用后退出
结果     第一次 counter_add 成功；后续两次 counter_get APP_UNREACHABLE
Run      继续到 completed
重连     subscribe(last_attempt=0, last_seq=0)
补拉     16 条事件，caught_up seq=16，最后 RUN_STATUS completed
```

App 断开不会暂停 Run；重连可以从任意 cursor 完整补拉。

## 3. P9.6 LLM 错误

用本地 stub 把 `/chat/completions` 固定返回 429 / 500：

```text
429 → ERROR RATE_LIMITED          retryable=true → RUN_STATUS failed
500 → ERROR PROVIDER_UNAVAILABLE  retryable=true → RUN_STATUS failed
```

错误码可解释，Run 不会悬挂。

## 4. P9.7 Daemon 断连

```text
运行中 kill -9 Harness
→ runs.status = queued
→ lease_epoch 1 → 2
→ assigned_daemon_id 清空
→ RUN_RETRY PENDING
```

旧 epoch 的执行被隔离；只有新租约可以写事件。

## 5. P9.8 MW 重启与恢复

### 5.1 初始故障

第一次实测发现：

```text
RUN_RETRY 在 7 秒内耗尽 3 次尝试 → DEAD
原因：Harness 尚未重连，所有尝试都是 "no eligible daemon"
Run 一直 queued，直到 RUN_TIMEOUT（30s）触发
```

根因：重连时没有重新调度队列积压；重试预算耗尽后没有任何补偿。

### 5.2 修复 P9-F1：daemon 上线时补调度

`Control.ReconcileDaemon` 在 Hello 时先执行 `scheduleQueuedRuns`：

```text
ListQueuedRuns()                -- 新增，最多 1000 条
  → runtime 与 daemon 匹配
  → Enqueue(RUN_RETRY)
```

`Enqueue` 对同一 (run_id, task_type) 在 PENDING/LEASED 时幂等；
旧任务 DEAD 后会创建新任务，所以恰好补上耗尽的预算。

同时 `serve.py` 增加自动重连（P9-F2）：

```text
连接关闭 → 记录 connection_lost
  → 重新取 ws_token
  → 重连并继续服务
```

### 5.3 修复后验证

```text
杀 Harness           → queued / epoch 2
重启 MW              → 仍 queued
等待 12s             → RUN_RETRY DEAD（3 次）
启动 Harness         → "control scheduled queued runs" 1 次
                     → 新 RUN_RETRY DONE
                     → 重新分配 attempt 2 / epoch 3
工具                 → counter_add ok {"value": 9}, counter_get ok {"value": 9}
终态                 → completed
```

证据：`P9.8f2/`

## 6. P9.9 依赖不可用

停掉 PostgreSQL 后提交 Run：

```text
修复前：401 UNAUTHORIZED          （把基础设施故障伪装成凭据错误）
修复后：503 DEPENDENCY_UNAVAILABLE retryable=true
恢复后：healthz 200，App 认证 200
```

修复 P9-F3：`appFromRequest` 区分 `ErrAppNotFound` 与后端错误：

```text
ErrAppNotFound → 401
其他错误        → 503 DEPENDENCY_UNAVAILABLE
```

影响 `POST /runs`、`POST /ws-token`、`PUT /apps`、`GET /apps/{id}`。
读接口的 `canAccessApp` 仍按失败关闭处理，未改为 503（见局限）。

## 7. 汇总

| ID | 结果 | 说明 |
|---|---|---|
| P9.1 | 通过 | 断开后 Run 继续；重连补拉 16 条 |
| P9.2–P9.5 | 通过 | P6D 实测 |
| P9.6 | 通过 | RATE_LIMITED / PROVIDER_UNAVAILABLE |
| P9.7 | 通过 | requeue + epoch fencing |
| P9.8 | 通过（修复后） | 补调度 + App 自动重连 |
| P9.9 | 通过（修复后） | 503 而非 401 |
| P9.10 | 未验证 | 同步写风险 |

## 8. P9.10 未验证

`AppHub.Publish` → `AppConn.SendRun` 是**同步 socket 写**：

```go
c.sendMu.Lock()
defer c.sendMu.Unlock()
return c.sendLocked(payload)
```

若订阅者停止读取，TCP 发送缓冲填满后写阻塞，会连带阻塞 Control 的事件协程。
本轮没有构造出足够大的事件流填满内核缓冲，因此**未验证**是否阻塞、也未见指标。
这是一个已知风险，需要专门的压力测试或改为有界队列 + 丢弃/断开策略。

## 9. 代码改动

```text
internal/runstore/store.go        ListQueuedRuns
internal/control/control.go       ReconcileDaemon → scheduleQueuedRuns
internal/httpapi/server.go        appFromRequest 返回 error；503 映射
examples/counter_agent/serve.py   断线自动重连
examples/counter_agent/ws_connect.py  重连日志包含 data.status
```

测试：

```text
go test ./...            全部 ok
make lint                ok
make harness-test        82 tests ok
```

新增用例：

```text
TestListQueuedRuns
TestReconcileDaemonCases/queued_run_is_rescheduled_on_reconnect
TestReconcileDaemonCases/queued_run_for_another_runtime_is_left_alone
TestRunAuthBackendUnavailable
```

## 10. 证据

```text
docs/reports/artifacts/retrofit/P9/
  go_test_final.log
  lint_final.log
  harness_test.log
  P9.1/    serve.log reconnect.log db.txt submit_run.json
  P9.6/    stub.log submit_429.json submit_500.json db_429.txt db_500.txt
  P9.7/    db_after_kill.txt submit_run.json harness_1.log
  P9.8/    修复前：db_after_restart.txt db_final.txt
  P9.8f2/  修复后：db_after_kill.txt db_after_restart.txt db_retry_dead.txt db_final.txt serve.log
  P9.9/    result.txt response.json middleware.log
```

## 11. 剩余风险

- 慢消费者未验证；同步写可能在订阅端停读时阻塞 Control。
- `canAccessApp` 仍把后端故障当 401/403。
- 恢复验证是单实例、单 Run；未覆盖多 Daemon 竞争与大批量积压。
- App 自动重连只覆盖 MW 侧断开，未覆盖 App 自身崩溃重启。
