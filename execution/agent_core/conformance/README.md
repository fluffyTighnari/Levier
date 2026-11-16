<!-- Middleware — language-neutral agent core conformance fixtures. -->

# Agent Core Conformance

每个 fixture 是一段脚本化的 provider/tool 行为 + 期望的 CoreEvent 序列。
Python 和未来的 TS core 都必须跑通同一组 fixture。

## Fixture 格式

```json
{
  "name": "single_tool",
  "run_id": "run-1",
  "start_step": 1,
  "messages": [{"role": "user", "content": "..."}],
  "tools": [{"name": "add", "description": "", "input_schema_json": "{}"}],
  "model": {"provider": "fake", "model": "m"},
  "max_steps": 4,
  "step_timeout_ms": 1000,
  "tool_timeout_ms": 1000,
  "abort_after_steps": 0,
  "provider_script": [
    {"content": "", "tool_calls": [...], "usage": {...}, "model": "m", "sleep_ms": 0,
     "deltas": ["..."], "error": {"code": "...", "message": "...", "retryable": true}}
  ],
  "tool_script": [
    {"ok": true, "result_json": "...", "error_code": "", "error_message": "",
     "retryable": false, "sleep_ms": 0}
  ],
  "expected": {
    "status": "completed",
    "steps": 2,
    "provider_calls": 2,
    "tool_calls": 1,
    "tool_message_contains": ["..."],
    "events": [{"type": "run_started"}, ...]
  }
}
```

事件匹配是子集匹配：expected 里出现的字段必须一致，实际事件可以带额外字段
（例如 `ts_ms`、`duration_ms`、自动生成的 message_id）。

## 运行

```sh
make agent-core-test
```
