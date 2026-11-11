<!-- Middleware — retrofit P6.7 tool event pairing. -->

# Retrofit P6.7：工具事件成对与字段完整

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-11 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P6.7 |
| 状态 | 完成 |
| 代码提交 | 无（验证既有行为） |
| 基准提交 | `343f707` |

## 1. 本步唯一逻辑边

```text
Harness/Gateway → run_events: TOOL_CALL_BEGIN / TOOL_CALL_END
```

断言：

- 每个 `invocation_id` 恰好一条 BEGIN、一条 END。
- BEGIN 的 seq 小于 END 的 seq。
- BEGIN 字段：`invocation_id`、`tool_name`、`arguments_json`。
- END 字段：`invocation_id`、`ok`、`duration_ms`，成功时 `result_json`。
- 不存在孤立 BEGIN 或孤立 END。

## 2. 样本

使用 P6.6 的真实 Run：

```text
run_id = 01M749HVT28BBMZ2F29HPH027K
```

原始事件：

```text
seq 4  TOOL_CALL_BEGIN counter_add   {"delta": 7}   inv=01M749HXENBA5PQM4VKPNNJVWB
seq 5  TOOL_CALL_END   counter_add   ok=true duration_ms=9  result={"value": 12}
seq 8  TOOL_CALL_BEGIN counter_reset {}             inv=01M749HYG5PAP5XG59HKR9T4MH
seq 9  TOOL_CALL_END   counter_reset ok=true duration_ms=8  result={"value": 0}
seq 12 TOOL_CALL_BEGIN counter_get   {}             inv=01M749HZH3SKBM66DDG61ZBJYR
seq 13 TOOL_CALL_END   counter_get   ok=true duration_ms=7  result={"value": 0}
```

## 3. 配对检查

```sql
SELECT payload->>'invocation_id', 
       count(*) FILTER (WHERE type='TOOL_CALL_BEGIN'),
       count(*) FILTER (WHERE type='TOOL_CALL_END'),
       min(seq) FILTER (WHERE type='TOOL_CALL_BEGIN'),
       min(seq) FILTER (WHERE type='TOOL_CALL_END')
FROM run_events
WHERE run_id='...' AND type IN ('TOOL_CALL_BEGIN','TOOL_CALL_END')
GROUP BY payload->>'invocation_id';
```

结果：

```text
01M749HXENBA5PQM4VKPNNJVWB | 1 | 1 |  4 |  5
01M749HYG5PAP5XG59HKR9T4MH | 1 | 1 |  8 |  9
01M749HZH3SKBM66DDG61ZBJYR | 1 | 1 | 12 | 13
```

- BEGIN 数量 = END 数量 = 1
- 每条 `begin_seq < end_seq`

## 4. 孤立事件检查

本 Run：

```text
SELECT payload->>'invocation_id'
FROM run_events
WHERE run_id='...' AND type IN (...)
GROUP BY payload->>'invocation_id'
HAVING count(*) FILTER (WHERE type='TOOL_CALL_BEGIN') <> 1
    OR count(*) FILTER (WHERE type='TOOL_CALL_END') <> 1;
```

```text
 invocation_id
---------------
(0 rows)
```

全库扫描（所有历史 Run，含 P6.2 的失败路径与 P6.3 的断连路径）：

```text
 run_id | invocation_id | begins | ends
--------+---------------+--------+------
(0 rows)
```

## 5. 字段完整性

```sql
SELECT type, count(*)
FROM run_events
WHERE run_id='...' AND type IN ('TOOL_CALL_BEGIN','TOOL_CALL_END')
  AND (
    payload->>'invocation_id' IS NULL
    OR (type='TOOL_CALL_BEGIN'
        AND (payload->>'tool_name' IS NULL OR payload->>'arguments_json' IS NULL))
    OR (type='TOOL_CALL_END'
        AND (payload->>'ok' IS NULL OR payload->>'duration_ms' IS NULL
             OR (payload->>'ok'='true' AND payload->>'result_json' IS NULL)))
  )
GROUP BY type;
```

```text
 type | count
------+-------
(0 rows)
```

## 6. 这证明了什么

| 项目 | 结果 |
|---|---|
| BEGIN/END 一一对应 | 是 |
| BEGIN 在 END 之前 | 是 |
| BEGIN 含 invocation_id/tool_name/arguments_json | 是 |
| END 含 ok/duration_ms/result_json | 是 |
| 孤立 BEGIN | 无 |
| 孤立 END | 无 |
| 历史全部 Run 无配对错误 | 是 |
| 失败路径也成对（P6.2 证据） | 是 |

## 7. 局限

- 并发多工具调用时的事件交错未覆盖（当前 ReAct 串行执行）。
- 未校验 `duration_ms` 数值的合理性，仅校验存在。

## 8. 证据

```text
docs/reports/artifacts/retrofit/P6.7/
  tool_events.txt     配对、孤立、字段、全库扫描
  summary.txt
```

关联证据：

```text
docs/reports/artifacts/retrofit/P6.6/
  db_invocation.txt
  tool_host.log
```

## 9. 下一步

P6.8：Run 终态与最终回答。

```text
MESSAGE_COMPLETED 最终回答包含工具结果
RUN_STATUS completed
runs.status = completed
无 requeue、无 cancelled
```
