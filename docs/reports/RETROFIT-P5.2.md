<!-- Middleware — retrofit P5.2 real WebSocket connection. -->

# Retrofit P5.2：真实 WebSocket 连接

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P5.2 |
| 状态 | 完成 |
| 代码提交 | `f6b262b`、`81559ac` |
| 范围 | 真实 ws_token、真实握手、连接 gauge |

## 1. 真实调用

```text
ws_connect.py
  → 读取 0600 credentials
  → POST /api/v1/ws-token
  → 得到 ws_token（内存）
  → GET /api/v1/ws?app_id=<app_id>&token=<ws_token>
  → 101 Switching Protocols
```

## 2. 客户端实现

新增 Python 标准库 WebSocket 客户端：

```text
examples/counter_agent/ws_client.py
```

只实现所需能力：

- HTTP Upgrade
- `Sec-WebSocket-Key/Accept`
- 客户端 masked 文本帧
- 服务端 unmasked 文本帧
- ping/pong
- close

无第三方包。

## 3. 发现并修复：非浏览器客户端被 403

x/net 默认 `websocket.Handler` 要求 `Origin`：

```text
非浏览器客户端不发送 Origin
→ checkOrigin 返回 null origin
→ 403
```

这对“语言无关接入”是错误限制。

Middleware 已改为：

```go
websocket.Server{
    Handshake: func(*websocket.Config, *http.Request) error { return nil },
    Handler:   handler,
}
```

安全依据：

- App 鉴权已经由 `ws_token + app_id` 完成。
- 不依赖 cookie，因此不需要 Origin 作为 CSRF 防护。
- 握手后仍由 `handleWS` 校验 token。

## 4. 真实结果

App：

```text
app_id = app_01M72N2PK1YN9NWG52J75SDA00
```

连接：

```json
{"connected": true, "app_id": "app_01M72N2PK1YN9NWG52J75SDA00", "ws_url": "/api/v1/ws"}
```

保持 3 秒后关闭：

```json
{"closed": true}
```

MW 日志：

```text
ws connected   app_id=... remote_addr=127.0.0.1:57793
ws disconnected app_id=... duration_ms=3005
```

## 5. 指标

连接期间：

```text
mw_ws_connections{app="app_01M72N2PK1YN9NWG52J75SDA00"} 1
```

关闭后：

```text
mw_ws_connections{app="app_01M72N2PK1YN9NWG52J75SDA00"} 0
```

## 6. 这证明了什么

| 项目 | 结果 |
|---|---|
| ws_token 可用于真实 WS 连接 | 是 |
| 非浏览器客户端可握手 | 是（已修复 Origin 限制） |
| MW 接受 Upgrade | 是 |
| 连接 gauge +1 | 是 |
| 关闭后 gauge -1 | 是 |
| ping/pong | 留给 P5.3 |
| subscribe/caught_up | 留给 P5.4 |

## 7. 证据

```text
docs/reports/artifacts/retrofit/P5.2/
  bootstrap.json
  ws_connect.log
  ws_connect.err
  metrics_connected.txt
  metrics_closed.txt
  middleware.log
  migrate.log
  summary.txt
```

## 8. 下一步

P5.3：真实 ping/pong。

```text
client → {"type":"ping"}
server → {"type":"pong"}
```
