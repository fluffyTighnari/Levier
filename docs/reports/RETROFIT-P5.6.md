<!-- Middleware — retrofit P5.6 cursor replay. -->

# Retrofit P5.6：断线重连与 cursor

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P5.6 |
| 状态 | 完成 |
| 代码提交 | `c318e60` |
| 范围 | 真实已完成 Run、两次不同 cursor 的 replay |

## 1. cursor 是什么

cursor 是：

```text
(attempt, seq)
```

表示：

> 客户端已经处理到的最后一个事件位置。

它对应 `run_events` 的主键：

```text
(run_id, attempt, seq)
```

## 2. 为什么需要 cursor

重连时不能从头重放，也不能漏掉断线期间的事件。

```text
没有 cursor：
  要么从头重放 → 重复
  要么从现在开始 → 漏事件

有 cursor：
  只补 cursor 之后的事件
```

## 3. subscribe 时如何传

```json
{
  "type": "subscribe",
  "run_id": "...",
  "last_attempt": 1,
  "last_seq": 1
}
```

MW：

```text
ListEvents(run_id, after_attempt=1, after_seq=1)
```

SQL 条件本质是：

```sql
(attempt > 1) OR (attempt = 1 AND seq > 1)
```

所以 `(1,1)` 之后是 `(1,2)`、`(1,3)`。

## 4. 本次真实运行

Run：

```text
run_id = 01M72QTKBP5FEP93KNCB6SV48H
status = completed
attempt = 1
last_event_seq = 3
```

DB 事件：

```text
(1,1) RUN_STATUS
(1,2) MESSAGE_COMPLETED
(1,3) RUN_STATUS
```

### 第一次重放：cursor (0,0)

```json
{
  "event_seqs": [1,2,3],
  "caught_up": {"attempt":1,"seq":3}
}
```

### 第二次重放：cursor (1,1)

```json
{
  "event_seqs": [2,3],
  "caught_up": {"attempt":1,"seq":3}
}
```

## 5. 断言结果

```json
{
  "first_ok": true,
  "second_ok": true,
  "no_duplicate_seq1": true,
  "no_missing_seq2_3": true
}
```

含义：

- 第一次收到 1,2,3。
- 第二次只收到 2,3。
- 没有重复 seq 1。
- 没有漏 seq 2/3。
- 两次 `caught_up` 都指向 (1,3)。

## 6. MW 内部路径

```text
subscribe(last_attempt, last_seq)
  → runs.GetRun
  → 校验 App 归属
  → appConn.BeginReplay(run_id, last_attempt, last_seq)
  → appConn.Subscribe(run_id)
  → replayWS:
      ListEvents(after_attempt, after_seq, limit=1000)
      逐条 SendReplay
      每页不足 1000 停止
  → appConn.EndReplay(run_id, 最终 cursor)
  → caught_up(最终 cursor)
```

内存中：

```text
AppConn.cursors[run_id] = (attempt, seq)
```

用途：

- 补推期间实时事件按 cursor 去重。
- 补推结束后旧事件丢弃。
- 重连时客户端提供 cursor。

## 7. 这证明了什么

| 项目 | 结果 |
|---|---|
| cursor 是排他边界 | 是（(1,1) 不含 1） |
| 重连不重复 seq 1 | 是 |
| 重连不丢 seq 2/3 | 是 |
| caught_up 返回最终 cursor | 是 |
| 事件顺序稳定 | 是 |
| 多页补推 | P5.6 事件少，未触发 1000 页 |

## 8. 证据

```text
docs/reports/artifacts/retrofit/P5.6/
  bootstrap.json
  submit_run.json
  replay_first.log
  replay_second.log
  replay_compare.json
  db_events.txt
  harness.log
  middleware.log
  migrate.log
  summary.txt
```

## 9. 下一步

P5.7：ws_token 过期/跨 App 拒绝。

```text
过期 token → 连接失败
错误 app_id → 连接失败
```
