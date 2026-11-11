<!-- Middleware — retrofit P6.5 tool result enters next model turn. -->

# Retrofit P6.5：ToolCallResult 进入下一轮模型输入

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-11 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P6.5 |
| 状态 | 完成 |
| 代码提交 | 无（验证既有行为） |
| 基准提交 | `6c1f214` |

## 1. 本步唯一逻辑边

```text
Gateway → Harness: ToolCallResult
  → ReactStrategy 把它加入 messages（role=tool）
  → 下一次模型请求能看到该结果
```

只验证「结果进入下一轮模型输入」。

## 2. 可证伪设计

如果只是让计数器从 0 加 2，模型完全可以在没看到工具结果时也算出 2，
无法证明结果真的进入了对话。

因此这一步：

```text
宿主以 --initial 41 启动
提示词：调用 counter_get，报告它返回的精确整数
提示词与 input 都不出现 41
```

只有一种可能得到 41：工具结果确实进入了模型输入。

## 3. 输入侧（不含答案）

`submit_run.json`：

```json
{
  "status": 202,
  "run_id": "01M749EJEGMWHH39SXMFCB43SS",
  "run_status": "queued"
}
```

`runs.input_json`：

```json
[{"role": "user",
  "content": "Call the counter_get tool exactly once and report the exact integer value it returns. Do not guess and do not call any other tool."}]
```

```text
grep -c 41 submit_run.json -> 0
input_json 中也没有 41
```

## 4. 工具结果

宿主：

```json
{
  "tool_name": "counter_get",
  "arguments_json": "{}",
  "result_json": "{\"value\": 41}"
}
```

事件：

```text
seq 4 TOOL_CALL_BEGIN counter_get invocation_id=01M749EM87KHS9A1MT3F6EGTZ7
seq 5 TOOL_CALL_END   ok=true result_json={"value": 41} duration_ms=15
```

## 5. 模型下一轮

```text
seq 6 USAGE prompt_tokens 407 → 467
seq 7 MESSAGE_COMPLETED step=2
  content = "The `counter_get` tool returned the exact integer value **41**."
```

两点证据：

1. `41` 只能来自工具结果，输入中不存在。
2. 工具消息加入后 prompt_tokens 从 407 增至 467，对话确实变长。

对应代码路径 `harness/src/middleware_harness/strategies/react.py`：

```python
result = await self._run_tool(ctx, call, tool_timeout_ms)
messages.append(_tool_message(call, result))
```

```python
def _tool_message(call, result) -> Message:
    return Message(
        role="tool",
        content=result.result_json,
        name=call.name,
        tool_call_id=call.call_id,
    )
```

## 6. Run 结果

```text
status = completed
seq 8 RUN_STATUS completed
```

## 7. 这证明了什么

| 项目 | 结果 |
|---|---|
| Gateway 返回 ToolCallResult 给 Harness | 是 |
| Harness 把它追加为 role=tool 消息 | 是 |
| 下一轮请求包含该结果 | 是（模型回答了输入中不存在的 41） |
| 对话长度增长 | 是（407 → 467 prompt tokens） |
| 工具结果是否正确 | 是（`{"value": 41}`） |
| 落库终态 | P6.6 |
| 事件成对 | P6.7 |

## 8. 局限

- 没有直接抓取 HTTP 请求体；结论基于「输入不含 41 + 模型输出 41 + token 增长」。
- 未覆盖多工具并行、超大结果截断。

## 9. 证据

```text
docs/reports/artifacts/retrofit/P6.5/
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

## 10. 下一步

P6.6：`tool_invocations` 落库正确。

```text
invocation_id 与事件一致
status=completed
result_json={"value": 41}
```
