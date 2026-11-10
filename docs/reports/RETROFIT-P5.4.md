<!-- Middleware — retrofit P5.4 WebSocket subscribe/caught_up. -->

# Retrofit P5.4：subscribe 空 Run 与 caught_up

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P5.4 |
| 状态 | 完成 |
| 代码提交 | `c9d40e8` |
| 范围 | 真实 WS、真实 queued Run、无事件补推 |

## 1. 为什么用 queued Run

本步只验证：

```text
subscribe → ListEvents → caught_up
```

不需要 Harness。

所以：

```text
创建 Run
→ 不启动 Harness
→ Run 保持 queued
→ run_events = 0
```

这样 `caught_up` 的初始 cursor 应明确为：

```text
attempt = 0
seq = 0
```

## 2. 真实调用

```text
ws_connect.py --subscribe-run <run_id>
  → POST /ws-token
  → GET /ws
  → send {"type":"subscribe","run_id":"...","last_attempt":0,"last_seq":0}
```

实际：

```json
{
  "type": "subscribe",
  "run_id": "01M72PRFYS7T24H35CFAXJBK79",
  "last_attempt": 0,
  "last_seq": 0
}
```

## 3. MW 内部路径

```text
handleWS
  → ParseClientMessage
  → SubscribeMessage
  → runs.GetRun(run_id)
  → 校验 run.AppID == appID
  → appConn.BeginReplay(run_id, 0, 0)
  → appConn.Subscribe(run_id)
  → replayWS
      → runs.ListEvents(..., limit=1000)
      → 空
  → appConn.EndReplay(run_id, 0, 0)
  → send caught_up
```

代码：

```text
internal/httpapi/ws_server.go   handleWS / replayWS
internal/httpapi/ws.go          SubscribeMessage / CaughtUpMessage
```

## 4. 实际结果

```json
{
  "caught_up": {
    "type": "caught_up",
    "run_id": "01M72PRFYS7T24H35CFAXJBK79",
    "attempt": 0,
    "seq": 0
  },
  "events": 0
}
```

DB：

```text
run status = queued
attempt = 0
last_event_attempt = 0
last_event_seq = 0
run_events count = 0
```

## 5. 这证明了什么

| 项目 | 结果 |
|---|---|
| subscribe 到达 handleWS | 是 |
| Run 归属校验 | 是 |
| ListEvents 空结果 | 是 |
| caught_up 发送 | 是 |
| 初始 cursor = (0,0) | 是 |
| 事件补推 | P5.5 |
| 重连 cursor | P5.6 |

## 6. 证据

```text
docs/reports/artifacts/retrofit/P5.4/
  bootstrap.json
  submit_run.json
  subscribe.log
  subscribe.err
  db_run.txt
  middleware.log
  migrate.log
  summary.txt
```

## 7. 下一步

P5.5：创建真实 echo Run，启动 Harness，订阅并确认收到：

```text
event RUN_STATUS
event MESSAGE_COMPLETED
event RUN_STATUS
caught_up
```
