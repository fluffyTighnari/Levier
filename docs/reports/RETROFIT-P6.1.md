<!-- Middleware — retrofit P6.1 react run with DeepSeek. -->

# Retrofit P6.1：真实 react Run + TaskAssign tools

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-11 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P6.1 |
| 状态 | 完成 |
| 代码提交 | 无（使用已有 P6A 能力） |
| 基准提交 | `68a963f` |

## 1. 真实调用链

```text
App
  → POST /api/v1/runs
      strategy=react
      provider=deepseek
      model=deepseek-flash

MW Control
  → TaskAssign
      strategy=react
      tools = counter_add/get/reset
      trace_context = run.trace_id

真实 Harness
  → ProviderRouter → OpenAICompatibleProvider
  → DeepSeek /chat/completions
  → content=OK
  → RUN_STATUS completed
```

## 2. TaskAssign

Harness 日志：

```json
{
  "message": "task assign received",
  "run_id": "01M7418WJPXS8RFHPQ3MMG95RY",
  "strategy": "react",
  "app_id": "app_01M7418WBTB130KS0J0K642BYP",
  "trace_context": "01M7418WJNJBN81BG56EH8HV0G",
  "tool_count": 3,
  "tools": [
    {"name":"counter_add","scope":"LOCAL","schema_len":109},
    {"name":"counter_get","scope":"LOCAL","schema_len":62},
    {"name":"counter_reset","scope":"LOCAL","schema_len":62}
  ]
}
```

证明：

```text
tool_defs → TaskAssign → 真实 Harness
```

## 3. Run 结果

```text
run_id = 01M7418WJPXS8RFHPQ3MMG95RY
strategy = react
status = completed
attempt = 1
trace_id = 01M7418WJNJBN81BG56EH8HV0G
```

事件：

```text
seq 1 RUN_STATUS running
seq 2 USAGE {"prompt_tokens":388,"completion_tokens":2,"total_tokens":390,...}
seq 3 MESSAGE_COMPLETED {"role":"assistant","model":"deepseek-flash","content":"OK"}
seq 4 RUN_STATUS completed
```

## 4. 这证明了什么

| 项目 | 结果 |
|---|---|
| react strategy 被选中 | 是 |
| provider=deepseek 路由到 OpenAI-compatible | 是 |
| DeepSeek 真实调用成功 | 是 |
| TaskAssign 携带 3 个 local tools | 是 |
| trace_context 到达 Harness | 是 |
| USAGE 事件来自 DeepSeek | 是 |
| Run 完成 | 是 |
| local tool 调用 | 未触发（P6.2） |

## 5. 为什么没有 tool_call

本步故意使用：

```text
Reply with exactly OK and nothing else.
```

模型没有调用工具，只返回 `OK`。

这是为了把 P6.1 与 P6.2 分开：

- P6.1：react + DeepSeek + TaskAssign tools。
- P6.2：提示模型必须调用 counter tool，验证 `tool_call` 链路。

## 6. 证据

```text
docs/reports/artifacts/retrofit/P6.1/
  bootstrap.json
  register_tools.json
  submit_run.json
  harness.log
  middleware.log
  db_run.txt
  migrate.log
  summary.txt
```

## 7. 下一步

P6.2：让 DeepSeek 必须调用 `counter_add`，观察 Harness 发出 `ToolCallRequest`。

```text
prompt: add 2 using counter_add, do not calculate yourself
→ DeepSeek tool_calls
→ ToolCallRequest
```
