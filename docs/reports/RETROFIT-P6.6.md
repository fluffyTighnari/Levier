<!-- Middleware — retrofit P6.6 tool_invocations persistence. -->

# Retrofit P6.6：tool_invocations 落库

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-11 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P6.6 |
| 状态 | 完成 |
| 代码提交 | 无（验证既有行为） |
| 基准提交 | `239d4da` |

## 1. 本步唯一逻辑边

```text
Gateway → Registry: tool_invocations 行
```

断言：每次本地工具调用恰好一行，`invocation_id` 与事件一致，
`status=completed`，`result_json` 与工具回传逐字节一致。

## 2. 真实运行

```text
app_id = app_01M749HVFQ7VV6SW6N05JE7Q4S
run_id = 01M749HVT28BBMZ2F29HPH027K
初始 counter = 5
提示词：add 7 → reset → get
```

宿主实际执行：

```text
counter_add   {"delta": 7} -> {"value": 12}
counter_reset {}           -> {"value": 0}
counter_get   {}           -> {"value": 0}
```

三个结果都 `ok=true`，顺序与提示词一致。

## 3. 事件与行对照

`run_events`：

| seq | type | tool | invocation_id | result_json |
|---|---|---|---|---|
| 4 | TOOL_CALL_BEGIN | counter_add | `01M749HXENBA5PQM4VKPNNJVWB` | — |
| 5 | TOOL_CALL_END | counter_add | `01M749HXENBA5PQM4VKPNNJVWB` | `{"value": 12}` |
| 8 | TOOL_CALL_BEGIN | counter_reset | `01M749HYG5PAP5XG59HKR9T4MH` | — |
| 9 | TOOL_CALL_END | counter_reset | `01M749HYG5PAP5XG59HKR9T4MH` | `{"value": 0}` |
| 12 | TOOL_CALL_BEGIN | counter_get | `01M749HZH3SKBM66DDG61ZBJYR` | — |
| 13 | TOOL_CALL_END | counter_get | `01M749HZH3SKBM66DDG61ZBJYR` | `{"value": 0}` |

`tool_invocations`：

```text
01M749HXENBA5PQM4VKPNNJVWB | counter_add   | completed | {"value": 12} | error NULL
01M749HYG5PAP5XG59HKR9T4MH | counter_reset | completed | {"value": 0}  | error NULL
01M749HZH3SKBM66DDG61ZBJYR | counter_get   | completed | {"value": 0}  | error NULL
```

`created_at < completed_at`：

```text
add    11:13:50.041107 → 11:13:50.045175
reset  11:13:51.114064 → 11:13:51.116331
get    11:13:52.167067 → 11:13:52.169906
```

## 4. 聚合校验

```sql
SELECT count(*) AS invocations,
       count(*) FILTER (WHERE status='completed') AS completed,
       count(*) FILTER (WHERE result_json IS NULL) AS missing_result
FROM tool_invocations WHERE run_id='...';
```

```text
invocations | completed | missing_result
------------+-----------+----------------
          3 |         3 |              0
```

## 5. Run

```text
status = completed
attempt = 1
```

## 6. 这证明了什么

| 项目 | 结果 |
|---|---|
| 每次调用一行 | 是（3 次 → 3 行） |
| `invocation_id` 与事件一致 | 是 |
| `status` 全部 completed | 是 |
| `result_json` 与回传一致 | 是 |
| `error_json` 为空 | 是 |
| 完成时间晚于创建时间 | 是 |
| 无重复行 | 是（主键 invocation_id） |

## 7. 局限

- 只验证成功路径；失败路径在 P6.2 观察过 `failed + error_json`，但未在本步重复。
- 未验证并发同 Run 大批量写入。

## 8. 证据

```text
docs/reports/artifacts/retrofit/P6.6/
  bootstrap.json
  register_tools.json
  submit_run.json
  tool_host.log
  tool_host.err
  harness.log
  middleware.log
  db_invocation.txt
  migrate.log
  summary.txt
```

## 9. 下一步

P6.7：工具事件成对且字段完整。

```text
TOOL_CALL_BEGIN  invocation_id / tool_name / arguments_json
TOOL_CALL_END    ok / result_json / duration_ms
BEGIN 与 END 一一对应
```
