<!-- Middleware — retrofit P5.3 WebSocket ping/pong. -->

# Retrofit P5.3：真实 WebSocket ping/pong

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P5.3 |
| 状态 | 完成 |
| 代码提交 | `dcb9ad5` |
| 范围 | 真实连接、真实 JSON 文本帧、真实 pong |

## 1. 真实调用

```text
ws_connect.py --ping
  → POST /api/v1/ws-token
  → GET /api/v1/ws?app_id=&token=
  → 101
  → send {"type":"ping"}
  → recv {"type":"pong"}
```

实际输出：

```json
{"connected": true, "app_id": "app_01M72PNH8JHXTC4ECZETCTX989", "ws_url": "/api/v1/ws"}
{"pong": true, "reply": {"type": "pong"}}
{"closed": true}
```

## 2. MW 内部路径

```text
handleWS
  → websocket.Message.Receive
  → ParseClientMessage
      → PingMessage
  → s.sendWS(appConn, PongMessage{Type: "pong"})
      → AppConn.Send
      → wsSink.Send
      → websocket.Message.Send
```

代码：

```text
internal/httpapi/ws_server.go    handleWS
internal/httpapi/ws.go           ParseClientMessage / PingMessage / PongMessage
```

## 3. 帧层

P5.3 使用 P5.2 的 Python 标准库客户端：

- 客户端发送 masked 文本帧。
- MW 返回 unmasked 文本帧。
- JSON payload：

```json
{"type":"ping"}
```

```json
{"type":"pong"}
```

这验证了：

```text
JSON 文本帧编解码
客户端 mask
服务端 unmask
双向收发
```

## 4. 这证明了什么

| 项目 | 结果 |
|---|---|
| 真实客户端能发 ping | 是 |
| MW 能解析 PingMessage | 是 |
| MW 返回 pong | 是 |
| 客户端能解析 pong | 是 |
| 连接正常关闭 | 是 |
| subscribe/caught_up | 未验证（P5.4） |

## 5. 证据

```text
docs/reports/artifacts/retrofit/P5.3/
  bootstrap.json
  ping.log
  ping.err
  middleware.log
  migrate.log
  summary.txt
```

MW 日志：

```text
ws connected    app_id=app_01M72PNH8JHXTC4ECZETCTX989
ws disconnected app_id=app_01M72PNH8JHXTC4ECZETCTX989 duration_ms=0
```

## 6. 下一步

P5.4：真实 subscribe 空 Run，确认收到 `caught_up`，初始 cursor 为 0。
