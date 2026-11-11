<!-- Middleware — retrofit P6.4 App returns local_tool_result. -->

# Retrofit P6.4：App 返回 local_tool_result

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-11 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P6.4 |
| 状态 | 完成 |
| 代码提交 | `e27afd3` |
| 基准提交 | `396bfa4` |

## 1. 本步唯一逻辑边

```text
App 执行本地工具 → 返回 local_tool_result → Gateway 收到并被接受
```

只验证「返回」这一段。工具结果是否进入模型、是否落库完成，属 P6.5/P6.6。

## 2. 宿主改动

`examples/counter_agent/ws_tool_host.py`：

```text
之前：收到 local_tool_call 后打印并退出
现在：收到 → dispatch_counter_tool → send_json(local_tool_result)
```

结果消息：

```json
{
  "type": "local_tool_result",
  "correlation_id": "<msg_id of the call>",
  "invocation_id": "<invocation_id of the call>",
  "ok": true,
  "result_json": "{\"value\": 2}"
}
```

失败时：

```json
{
  "type": "local_tool_result",
  "correlation_id": "...",
  "invocation_id": "...",
  "ok": false,
  "error": {"code": "TOOL_EXEC_ERROR", "message": "...", "retryable": false}
}
```

其他行为：

- 复用 `dispatch.py`，不复制工具语义。
- 计数器状态在进程内跨调用保持。
- `--receive-count 0` 持续服务；空闲超时后退出。
- 收到 `run_completed` 直接退出。

## 3. 真实运行

```text
app_id = app_01M7495H9P27CZE393YYSY6M3R
run_id = 01M7495HKZ1CD5SDCHNXACFKG2
strategy = react / deepseek-flash
```

宿主日志（两次调用 + 两次结果）：

```text
counter_add  msg_id=01M7495JY1G7FFKYMTVS0GX77V
             invocation_id=01M7495JXPBAT81TJZDPXGJ6GT
             result_json={"value": 2}

counter_get  msg_id=01M7495MN9Z786TEAHFBN1PHF9
             invocation_id=01M7495MN388B1XZAGZAFDXY9D
             result_json={"value": 2}
```

`correlation_id` 与收到的 `msg_id` 完全一致，`invocation_id` 原样回传。

## 4. Gateway 侧确认

`run_events`：

```text
seq 5 TOOL_CALL_END counter_add ok=true duration_ms=14 result_json={"value": 2}
seq 9 TOOL_CALL_END counter_get ok=true duration_ms=8  result_json={"value": 2}
```

`tool_invocations`：

```text
01M7495JXPBAT81TJZDPXGJ6GT | counter_add | completed | {"value": 2}
01M7495MN388B1XZAGZAFDXY9D | counter_get | completed | {"value": 2}
```

`TOOL_CALL_END` 的时间点晚于宿主发送结果，且 `ok=true`；说明结果经 WS 被 Gateway 接受，而不是超时兜底。

## 5. Run 结果

```text
status = completed
attempt = 1
lease_epoch = 1
```

事件尾部：

```text
seq 11 MESSAGE_COMPLETED
  "Done. ... Final value reported by the tool: 2"
seq 12 RUN_STATUS completed
```

## 6. 这证明了什么

| 项目 | 结果 |
|---|---|
| App 收到 `local_tool_call` | 是 |
| App 本地执行工具 | 是 |
| 计数器状态跨调用保持 | 是（add→get 都是 2） |
| 返回 `local_tool_result` | 是 |
| `correlation_id == msg_id` | 是 |
| `invocation_id` 原样回传 | 是 |
| Gateway 接受结果 | 是（`TOOL_CALL_END ok=true`） |
| 工具失败路径 | 单测覆盖，未真实触发 |
| 结果进入模型下一轮 | P6.5 |
| 终态一致 | 本次 `runs.status=completed`，未见 requeue |

## 7. 单元测试

```text
Ran 39 tests
OK
```

新增 `examples/counter_agent/test_ws_tool_host.py`：

- add 返回 `{"value": 2}` 且 correlation 正确
- get/reset
- 两次 add 后 get = 5（状态保持）
- 非法参数 → `TOOL_EXEC_ERROR`，状态不变
- 未知工具 → `TOOL_NOT_FOUND`

全量 counter-agent 测试通过。

## 8. 证据

```text
docs/reports/artifacts/retrofit/P6.4/
  bootstrap.json
  register_tools.json
  submit_run.json
  tool_host.log
  tool_host.err
  harness.log
  middleware.log
  db_run.txt
  migrate.log
  summary.txt
```

## 9. 下一步

P6.5：确认工具结果进入 Harness 的下一轮模型输入。

```text
TOOL_CALL_END counter_add ok=true {"value": 2}
  → Harness 把 ToolCallResult 拼进消息
  → DeepSeek 下一步调用 counter_get
  → 最终回答引用 2
```
