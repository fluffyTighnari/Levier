<!-- Middleware — retrofit P6.2 DeepSeek tool calls. -->

# Retrofit P6.2：DeepSeek tool_call → ToolCallRequest

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-11 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P6.2 |
| 状态 | 完成 |
| 代码提交 | 无（使用 P6A provider） |
| 基准提交 | `efa2c17` |

## 1. 真实链路

```text
App
  → POST /api/v1/runs
      strategy=react
      provider=deepseek
      model=deepseek-flash
      tools=counter_add/get/reset

Harness
  → ReactStrategy
  → ProviderRouter → OpenAICompatibleProvider
  → DeepSeek returns tool_calls
  → ToolBridge sends ToolCallRequest

MW Gateway
  → Invoker local
  → AppHub.CallLocalTool
  → no App WS connected
  → APP_UNREACHABLE
```

## 2. DeepSeek 返回

第 1 步：

```json
{
  "content": "I'll add 2 to the counter, then read the current value.",
  "tool_calls": [
    {
      "call_id": "call_00_ET_hCEN1EFfQKVs8iBbpKuf8415",
      "tool_name": "counter_add",
      "arguments_json": "{\"delta\": 2}"
    }
  ]
}
```

第 2 步：

```json
{
  "content": "",
  "tool_calls": [
    {
      "call_id": "call_00_Q1oqSwvorCLJh8YlmA2Y8809",
      "tool_name": "counter_get",
      "arguments_json": "{}"
    }
  ]
}
```

## 3. Harness → MW

`run_events`：

```text
seq 4 TOOL_CALL_BEGIN
  tool_name=counter_add
  invocation_id=01M741G1CPYGXPYTSES0VRXRZF
  arguments_json={"delta": 2}

seq 5 TOOL_CALL_END
  ok=false
  error_code=APP_UNREACHABLE
  retryable=true
  duration_ms=7
```

`tool_invocations`：

```text
counter_add | failed | {"code":"APP_UNREACHABLE","message":"app is unreachable","retryable":true}
counter_get | failed | {"code":"APP_UNREACHABLE",...}
```

## 4. 为什么是 APP_UNREACHABLE

P6.2 还没有 App WebSocket 工具宿主。

所以：

```text
Harness ToolCallRequest
  → MW Invoker
  → AppHub.CallLocalTool
  → 没有 App WS 连接
  → ErrAppUnreachable
```

这不是 P6.2 的失败，而是 P6.3 要补的下一段：

```text
App WS 连接
→ 收到 local_tool_call
→ 返回 local_tool_result
```

## 5. Run 结果

```text
run_id = 01M741G0DBA550VQZ0HQKAYE2T
strategy = react
status = completed
attempt = 1
trace_id = 01M741G0DABJNTRY8KXT4QM66M
```

模型在工具失败后没有瞎猜，最终回复说明了 `APP_UNREACHABLE`。

## 6. 这证明了什么

| 项目 | 结果 |
|---|---|
| DeepSeek 返回 tool_calls | 是 |
| Harness 解析 tool_calls | 是 |
| Harness 发出 ToolCallRequest | 是 |
| MW 创建 tool_invocations | 是 |
| local 路由命中 | 是 |
| 无 App 时返回 APP_UNREACHABLE | 是 |
| App 返回结果 | P6.3 |
| 工具成功执行 | P6.4/P6.5 |

## 7. 证据

```text
docs/reports/artifacts/retrofit/P6.2/
  bootstrap.json
  register_tools.json
  submit_run.json
  harness.log
  middleware.log
  db_run.txt
  migrate.log
  summary.txt
```

`db_run.txt` 包含：

- run 状态
- 12 条 run_events
- 2 条 tool_invocations

## 8. 下一步

P6.3：App WS 工具宿主。

```text
App 连接 WS
→ 收到 local_tool_call
→ 记录 msg_id/invocation_id/tool_name/arguments
```
