<!-- Middleware — retrofit P5.7 ws_token expiry and binding. -->

# Retrofit P5.7：ws_token 过期与跨 App 拒绝

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P5.7 |
| 状态 | 完成，P5 收口 |
| 代码提交 | `d5623af`、`4c1b079` |
| 范围 | 真实 TTL=1s、真实 App1/App2、真实 403 |

## 1. 新增配置

```yaml
http:
  ws_token_ttl: "10m"
```

环境变量：

```text
MIDDLEWARE_HTTP_WS_TOKEN_TTL
```

默认 10 分钟，必须为正。

验证时使用：

```text
MIDDLEWARE_HTTP_WS_TOKEN_TTL=1s
```

## 2. 握手校验前移

之前 token 在 Upgrade 之后由 `handleWS` 校验：

```text
101 成功
→ handleWS 校验失败
→ 立即关闭
```

客户端只能看到“连上后马上断开”。

现在改为在 Hijack / Upgrade 之前校验：

```go
func (s *Server) websocketHandler(...) http.Handler {
    return http.HandlerFunc(func(w, r) {
        appID := r.URL.Query().Get("app_id")
        token := r.URL.Query().Get("token")
        tokenAppID, ok := s.wsTokens.Validate(token)
        if !ok || appID == "" || tokenAppID != appID {
            http.Error(w, "invalid ws token", http.StatusForbidden)
            return
        }
        websocketServer.ServeHTTP(w, r)
    })
}
```

结果：

```text
过期/跨 App → 403
有效 token → 101
```

同时保留 `handleWS` 的二次校验。

## 3. 实际结果

### 有效 token

```json
{"connected": true, "app_id": "app_01M72RJWVDZ488JR1Y552AZXBN", "ws_url": "/api/v1/ws"}
{"closed": true}
```

MW：

```text
GET /api/v1/ws status=101
```

### 过期 token

获取 token 后等待 2 秒（TTL=1s）：

```text
ws connect failed: handshake failed: 403
```

MW：

```text
GET /api/v1/ws status=403
```

### 跨 App token

App1 的 token + App2 的 app_id：

```text
ws connect failed: handshake failed: 403
```

MW：

```text
GET /api/v1/ws status=403
```

## 4. 这证明了什么

| 项目 | 结果 |
|---|---|
| TTL 可配置 | 是 |
| 过期 token 被拒绝 | 是（403） |
| token 绑定 app_id | 是（跨 App 403） |
| 有效 token 仍可连接 | 是（101） |
| 拒绝发生在 Upgrade 前 | 是 |
| 指标/日志状态码正确 | 是 |

## 5. P5 收口

| 步骤 | 状态 |
|---|---|
| P5.1 App key → ws_token | 完成 |
| P5.2 真实 WS 连接 | 完成 |
| P5.3 ping/pong | 完成 |
| P5.4 subscribe/caught_up | 完成 |
| P5.5 实时事件 | 完成 |
| P5.6 cursor 重连 | 完成 |
| P5.7 过期/跨 App 拒绝 | 完成 |

P5 已验证：

```text
App key → ws_token → 101 → subscribe → replay/live → caught_up → cursor 重连
```

## 6. 证据

```text
docs/reports/artifacts/retrofit/P5.7/
  bootstrap_app1.json
  bootstrap_app2.json
  valid.log
  valid.err
  expired.log
  expired.err
  cross_app.log
  cross_app.err
  middleware.log
  migrate.log
  summary.txt
```

## 7. 下一步

P6 开始 local tool 闭环：

```text
P6.1 Harness react Run → TaskAssign tools
P6.2 DeepSeek 返回 tool_call
P6.3 App 收到 local_tool_call
P6.4 App 返回 local_tool_result
P6.5 Harness 收到 ToolCallResult
P6.6 tool_invocations / events
P6.7 终态与基线对照
```

首先需要 DeepSeek provider 与 Harness provider routing。
