<!-- Middleware — retrofit P5.1 ws_token issuance. -->

# Retrofit P5.1：App key 换 ws_token

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P5.1 |
| 状态 | 完成 |
| 代码提交 | `171fdcd` |
| 范围 | 真实 App key、真实 MW、真实 ws_token |

## 1. 真实调用

App：

```text
ws_token.py
  → 读取 0600 credentials
  → POST /api/v1/ws-token
```

请求：

```http
POST /api/v1/ws-token
Authorization: Bearer <app_api_key>
Accept: application/json
```

## 2. MW 内部路径

```text
POST /api/v1/ws-token
  → handleIssueWSToken
      → resolveRequestApp
          → AuthenticateAPIKey
      → wsTokens.Issue(app_id)
          → 32 random bytes
          → base64url
          → expires = now + 10min
      → 200 WSTokenResponse
```

代码：

```text
internal/httpapi/server.go       handleIssueWSToken
internal/httpapi/ws_tokens.go    WSTokenStore.Issue
```

## 3. 真实返回

App 安全输出：

```json
{
  "status": 200,
  "ws_url": "/api/v1/ws",
  "expires_in": 600,
  "has_ws_token": true
}
```

真实 `ws_token`：

- 只在响应中出现。
- 被 `Secret` 包装。
- 没有打印。
- 没有写入证据。
- 只应在后续 WS 连接时放内存。

MW 日志：

```json
{
  "msg": "http request",
  "method": "POST",
  "route": "POST /api/v1/ws-token",
  "status": 200,
  "duration_ms": 0
}
```

## 4. 这证明了什么

| 项目 | 结果 |
|---|---|
| App key 可换 ws_token | 已证明 |
| 请求到达 `handleIssueWSToken` | 已证明 |
| TTL 默认 600 秒 | 已证明 |
| token 未打印 | 已证明 |
| token 绑定 app_id | 由实现保证，P5.2 连接时验证 |
| WS 能建立 | 未证明（P5.2） |
| 过期拒绝 | 未证明（P5.7） |

## 5. 凭据生命周期

```text
App key
  → 换 ws_token
  → ws_token 10 分钟
  → 只放内存
  → 过期后重新用 App key 换
```

## 6. 可复用命令

```sh
python3 ws_token.py --credentials ~/.config/middleware/my-app.json
```

任意 App 都可以用同样方式换取 ws_token。

## 7. 证据

```text
docs/reports/artifacts/retrofit/P5.1/
  bootstrap.json
  ws_token.json
  middleware.log
  migrate.log
  summary.txt
```

没有保存 ws_token；临时凭证已删除。

## 8. 下一步

P5.2：用 ws_token 建立真实 WebSocket 连接。

```text
GET /api/v1/ws?app_id=<app_id>&token=<ws_token>
→ 101 Switching Protocols
→ mw_ws_connections +1
→ 关闭后 -1
```
