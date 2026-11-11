<!-- Middleware — retrofit P6.3 App receives local_tool_call. -->

# Retrofit P6.3：App 收到 local_tool_call

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-11 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P6.3 |
| 状态 | 边界通过；发现 1 个终态竞态（P6.3-F1） |
| 代码提交 | `e94137f`（`ws_tool_host.py`） |
| 基准提交 | `8a500d2` |

## 1. 本步唯一逻辑边

```text
Harness → ToolCallRequest → MW Invoker → AppHub → App WebSocket
                                                    ↓
                                         收到 local_tool_call
```

只验证一件事：真实的 App 通过真实 WebSocket 收到工具调用，且字段正确。
本步宿主**故意不返回结果**。

## 2. 调用链

```text
App
  → POST /api/v1/ws-token
  → GET /api/v1/ws            （101）
  → POST /api/v1/runs         react / deepseek / deepseek-flash
      tools = counter_add, counter_get, counter_reset

Harness
  → ProviderRouter → DeepSeek
  → tool_calls: counter_add {"delta": 2}
  → ToolBridge → ToolCallRequest

MW
  → Invoker local
  → AppHub.CallLocalTool(app_id, ...)
  → App WS: local_tool_call
```

## 3. 宿主收到的消息

```json
{
  "msg_id": "01M741S3GFV98P3J1ZSE9C8F67",
  "invocation_id": "01M741S3G67EGX3XQH1TS19652",
  "tool_name": "counter_add",
  "arguments_json": "{\"delta\": 2}",
  "timeout_ms": 10000
}
```

对照：

| 字段 | 值 | 来源 |
|---|---|---|
| `msg_id` | `01M741S3GFV98P3J1ZSE9C8F67` | AppHub 生成的 WS 消息 ID |
| `invocation_id` | `01M741S3G67EGX3XQH1TS19652` | `tool_invocations.invocation_id` |
| `tool_name` | `counter_add` | ToolCallRequest |
| `arguments_json` | `{"delta": 2}` | DeepSeek tool_call |
| `timeout_ms` | `10000` | `runs.tool_timeout_ms` |

`arguments_json` 与 DeepSeek 返回的字符串逐字节一致，未被二次编码或改写。

## 4. WS 生命周期

`middleware.log`：

```text
08:57:55.783 ws connected    app_id=app_01M741S23G63MZ5J9ANJZVZYDP
08:57:57.008 ws disconnected app_id=app_01M741S23G63MZ5J9ANJZVZYDP duration_ms=1224
```

宿主 `--receive-count 1`，收到一个调用后主动关闭，因此连接在 1.2 秒后断开。

## 5. 预期内的后续失败

宿主关闭后，DeepSeek 又请求了一次 `counter_add`：

```text
seq 5 TOOL_CALL_END ok=false error_code=APP_UNREACHABLE duration_ms=11
seq 9 TOOL_CALL_END ok=false error_code=APP_UNREACHABLE duration_ms=7
```

两次 `tool_invocations`：

```text
01M741S3G67EGX3XQH1TS19652 | counter_add | failed | APP_UNREACHABLE
01M741S4JX4R71J8MGH9VBJM3H | counter_add | failed | APP_UNREACHABLE
```

这是本步的**预期行为**：宿主本就不返回结果，断开连接即等价于 App 不可达。
它证明「连接存在时收到调用」与「连接消失后失败」是可区分的两条路径。

Run 终态事件：

```text
seq 10 RUN_STATUS {"status": "cancelled"}
```

原因：模型重试工具两次后仍失败，达到重试上限，Harness 主动取消 Run。

## 6. 发现：P6.3-F1 终态与 requeue 竞态

`runs` 表最终状态与事件不一致：

```text
runs.status        = queued
runs.lease_epoch   = 2
runs.attempt       = 1
runs.updated_at    = 08:57:59.212391
run_events seq 10  = RUN_STATUS cancelled
control log        = "control run terminal" status=cancelled  (08:57:59.214401)
```

时序：

```text
1. Harness 发出 RUN_STATUS cancelled，随后 SIGTERM 退出
2. MW 事件协程写入 seq 10
3. 同时 gRPC 流关闭 → OnOffline → HandleDaemonOffline
4. HandleDaemonOffline 读到 status=running → RequeueRun
      → status=queued, lease_epoch=2, assigned_daemon_id=NULL
5. 终态 Transition(running → cancelled) 不再匹配（状态已是 queued）
      → 返回 ErrRunNotAssignable
6. control.go 对 ErrRunNotAssignable 静默继续
      → 仍打印 "control run terminal" 并 publishCompleted
```

`internal/control/control.go`：

```go
if _, err := c.runs.Transition(ctx, ...); err != nil && !errors.Is(err, runstore.ErrRunNotAssignable) {
    c.log.Warn(...); return
}
if runstore.TerminalStatus(status) {
    c.log.Info("control run terminal", ...)   // 即使 Transition 失败也执行
    ...
}
```

影响：

- 事件流显示 `cancelled`，运行表显示 `queued`，两个事实源矛盾。
- Run 可能被重试任务再次领取，产生重复工具调用。
- 客户端/轮询看到的状态与事件不同。

本步判定：

- P6.3 目标边（App 收到 `local_tool_call`）**通过**。
- P6.3-F1 属 Run 终态收尾路径，独立于本次链路；修复应作为单独一小步（见 P6.3f），
  不混入本步证据。

## 7. 这证明了什么

| 项目 | 结果 |
|---|---|
| WS 握手与鉴权 | 是 |
| AppHub 能将调用路由到具体 App | 是 |
| `local_tool_call` 结构符合契约 | 是 |
| `invocation_id` 与 DB 一致 | 是 |
| `arguments_json` 原样传递 | 是 |
| `timeout_ms` 来自 Run 配置 | 是 |
| 宿主断线后返回 `APP_UNREACHABLE` | 是 |
| 返回 `local_tool_result` | 未做（P6.4） |
| 终态状态一致性 | 否（P6.3-F1） |

## 8. 证据

```text
docs/reports/artifacts/retrofit/P6.3/
  bootstrap.json
  register_tools.json
  submit_run.json
  tool_host.log
  tool_host.err
  harness.log
  middleware.log
  db_run.txt       快照（status=running）
  db_final.txt     终态快照（status=queued + cancelled 事件）
  migrate.log
  summary.txt
```

## 9. 下一步

P6.3f（可选，建议先做）：修复终态与 requeue 竞态。

```text
终态 Transition 失败时不得继续发布 terminal
requeue 前需确认 Run 仍非终态（或按 epoch 条件更新）
```

然后 P6.4：宿主返回结果。

```text
收到 local_tool_call
→ 执行 counter_add
→ 发 local_tool_result
    correlation_id = msg_id
    invocation_id = ...
    ok = true
    result_json = ...
→ 保持连接等待下一次调用
```
