<!-- Middleware — retrofit P6.8 run terminal state. -->

# Retrofit P6.8：Run 终态与最终回答

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-11 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P6.8 |
| 状态 | 完成，P6B 收口 |
| 代码提交 | 无（验证既有行为 + P6.3f） |
| 基准提交 | `3021dca` |

## 1. 本步唯一逻辑边

```text
Harness → 终态 completed → Control 投影 → runs.status
```

断言：

- 最终回答引用工具结果。
- 事件流以一条 `RUN_STATUS completed` 结束。
- `runs.status = completed`，`attempt = 1`。
- daemon 下线后不被 requeue（P6.3f 的回归）。
- 终态只有一条，无 cancelled 混入。

## 2. 真实运行

```text
app_id = app_01M749QBBE2S3Z9HBJTBM476EF
run_id = 01M749QBNSA0VE9HRBR62SFWSZ
prompt: add 2 → counter_get → 报告返回值
```

事件流：

```text
seq 1  RUN_STATUS running
seq 2  USAGE prompt=402 completion=51
seq 3  MESSAGE_COMPLETED step1  tool_calls=[counter_add {"delta":2}]
seq 4  TOOL_CALL_BEGIN  counter_add
seq 5  TOOL_CALL_END    ok=true {"value": 2}
seq 6  USAGE prompt=470 completion=27
seq 7  MESSAGE_COMPLETED step2  tool_calls=[counter_get {}]
seq 8  TOOL_CALL_BEGIN  counter_get
seq 9  TOOL_CALL_END    ok=true {"value": 2}
seq 10 USAGE prompt=514 completion=32
seq 11 MESSAGE_COMPLETED step3
       "Done. I called counter_add with delta 2, then called counter_get.
        Final value returned by counter_get: 2"
seq 12 RUN_STATUS completed
```

## 3. 终态前

```text
run_id             | 01M749QBNSA0VE9HRBR62SFWSZ
status             | completed
attempt            | 1
lease_epoch        | 1
assigned_daemon_id | daemon-local-1
```

## 4. daemon 下线后

主动 `SIGTERM` Harness，等待 3 秒后再查：

```text
status             | completed
attempt            | 1
lease_epoch        | 1        ← 未被 requeue 抬高
assigned_daemon_id | daemon-local-1
```

任务表：

```text
RUN_DISPATCH | DONE      | 1
RUN_TIMEOUT  | CANCELLED | 0
```

```text
terminal_events = 1
invocations = 2, completed = 2
```

这正是 P6.3 出问题的那条路径：

```text
P6.3：终态事件与 daemon 下线竞态 → runs.status 变 queued
P6.8：同一时序 → 仍是 completed，lease_epoch 不变
```

## 5. 这证明了什么

| 项目 | 结果 |
|---|---|
| 最终回答引用工具结果 | 是（2） |
| 事件以 RUN_STATUS completed 结束 | 是 |
| `runs.status = completed` | 是 |
| 只有一条终态事件 | 是 |
| 无 cancelled 混入 | 是 |
| 无 retry 任务 | 是（只有 DISPATCH DONE / TIMEOUT CANCELLED） |
| daemon 下线不 requeue | 是（P6.3f 回归通过） |
| 工具调用全部完成 | 是（2/2） |

## 6. P6B 收口

| 步骤 | 结果 |
|---|---|
| P6.1 react + DeepSeek + TaskAssign tools | 通过 |
| P6.2 DeepSeek tool_call → ToolCallRequest | 通过 |
| P6.3 App 收到 local_tool_call | 通过 |
| P6.3f 终态竞态修复 | 通过 |
| P6.4 App 返回 local_tool_result | 通过 |
| P6.5 结果进入下一轮模型输入 | 通过 |
| P6.6 tool_invocations 落库 | 通过 |
| P6.7 事件成对与字段完整 | 通过 |
| P6.8 终态 completed | 通过 |

## 7. 局限

- 单 App、单 Run 串行；未覆盖并发 Run 与多 App 竞争。
- 未验证 sandbox/longterm runtime。

## 8. 证据

```text
docs/reports/artifacts/retrofit/P6.8/
  bootstrap.json
  register_tools.json
  submit_run.json
  tool_host.log
  tool_host.err
  middleware.log
  harness.log
  db_terminal.txt     终态前后快照
  migrate.log
  summary.txt
```

## 9. 下一步

P6C：与基线对照。

```text
P6.9  final counter 与 P0.1 一致
P6.10 tool call 次数与 P0.1 一致
P6.11 调用顺序 add/add/get 一致
```
