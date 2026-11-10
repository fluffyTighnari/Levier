<!-- Middleware — retrofit P5.5 live event fanout. -->

# Retrofit P5.5：subscribe 后接收真实事件

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P5.5 |
| 状态 | 完成 |
| 代码提交 | `303d5da`、`de62f96` |
| 范围 | 真实 Harness、真实 echo Run、真实 WS 事件 |

## 1. 测试顺序

为了确保事件是“实时”而不是补推，按以下顺序：

```text
1. 启动 MW，不启动 Harness
2. 创建 echo Run → queued
3. App WS 连接并 subscribe
4. 收到 caught_up(0,0)
5. 启动 Harness
6. Run 被调度执行
7. WS 连接收到实时事件
```

## 2. App 实际收到

```json
{"caught_up": {"type":"caught_up","run_id":"01M72Q7QEDN123TV5X0HWGEWRX","attempt":0,"seq":0},"events_before":0}
```

```json
{
  "received": [
    {"type":"event","event_type":"RUN_STATUS","attempt":1,"seq":1},
    {"type":"event","event_type":"MESSAGE_COMPLETED","attempt":1,"seq":2},
    {"type":"event","event_type":"RUN_STATUS","attempt":1,"seq":3},
    {"type":"run_completed","attempt":1,"status":"completed"}
  ]
}
```

## 3. DB 对照

```text
run status = completed
last_event_attempt = 1
last_event_seq = 3
```

```text
1 | 1 | daemon | RUN_STATUS
1 | 2 | daemon | MESSAGE_COMPLETED
1 | 3 | daemon | RUN_STATUS
```

## 4. MW 内部路径

```text
Control 处理 daemon Event
  → AppendEvent
  → Publisher.Publish(app_id, run_id, payload)
  → AppHub.Publish
      → 查找订阅了 run_id 的 AppConn
      → conn.SendRun(run_id, payload)
          → 检查 cursor
          → sendMu 串行发送
          → wsSink.Send
```

终态：

```text
Control transition completed
  → publishCompleted
  → AppHub.Publish run_completed
```

## 5. 这证明了什么

| 项目 | 结果 |
|---|---|
| subscribe 后收到 caught_up | 是（0,0） |
| 后续事件是实时 fanout | 是 |
| RUN_STATUS/MESSAGE_COMPLETED 顺序 | 1,2,3 |
| run_completed 到达 | 是 |
| WS 与 DB 事件一致 | 是 |
| 重连补拉 | P5.6 |

## 6. 证据

```text
docs/reports/artifacts/retrofit/P5.5/
  bootstrap.json
  submit_run.json
  subscribe.log
  subscribe.err
  harness.log
  middleware.log
  db_run.txt
  migrate.log
  summary.txt
```

关键：

```text
subscribe.log   caught_up + received 事件
db_run.txt      status=completed + 3 events
harness.log     task assign received
```

## 7. 下一步

P5.6：断线重连。

```text
第一次连接收到 seq 1..3
断开
用 last_attempt=1,last_seq=1 重连
→ 只补 seq 2,3
→ 不重复 seq 1
→ 不丢 seq 2,3
```
