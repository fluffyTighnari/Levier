<!-- Middleware — retrofit P3.1 real admin app registration. -->

# Retrofit P3.1：真实调用 `POST /admin/apps`

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P3.1 |
| 状态 | 完成 |
| 代码提交 | `c7c8b16` |
| 范围 | 真实 HTTP；真实 MW；真实 201 响应 |

## 1. 这一步真实调用了什么

App 进程：

```text
examples/counter_agent/register_app.py
  → config.from_env()
  → HttpClient(config, UrllibTransport())
  → POST http://127.0.0.1:8080/admin/apps
```

请求：

```http
POST /admin/apps
Authorization: Bearer <admin_token>
Content-Type: application/json
Accept: application/json
```

```json
{"name":"retrofit-p3-1794300014","multiagent":false}
```

这次不是 mock：

- `UrllibTransport` 打开真实 TCP。
- 命中真实 Middleware :8080。
- 返回真实 `201`。

## 2. MW 内部真实处理路径

```text
HTTP :8080
  → mux route: POST /admin/apps
  → handleAdminCreateApp
      → isAdmin
          → matchesToken(Bearer, admin_token)
      → readJSONBody
      → AppUpsertRequest.Validate
      → register.RegisterApp
          → app_id = app_<ULID>
          → api_key = mw_<32 random bytes base64url>
          → apps.api_key_hash = SHA-256(api_key)
          → INSERT INTO apps ...
      → 201 CreateAppResponse
```

代码：

```text
internal/httpapi/server.go       handleAdminCreateApp
internal/register/registry.go    RegisterApp
```

## 3. MW 真实返回

```json
{
  "status": 201,
  "app_id": "app_01M729TVY98X39WFKR7S76JF4M",
  "name": "retrofit-p3-1794300014",
  "has_api_key": true
}
```

这是 App 解析后的安全输出；真实 `api_key`：

- 只在响应 body 中出现一次。
- 被 `Secret` 包装。
- 没有打印。
- 没有写入证据。
- 后续 P3.3 才会用它。

MW 日志同时证明真实路由：

```json
{
  "level": "DEBUG",
  "msg": "http request",
  "request_id": "01M729TVY81SWTWR27340A6R26",
  "method": "POST",
  "route": "POST /admin/apps",
  "status": 201,
  "duration_ms": 2,
  "remote_addr": "127.0.0.1:61969"
}
```

## 4. 这一步证明了什么

| 项目 | 结果 |
|---|---|
| App 能发真实 HTTP | 已证明 |
| URL/path 正确 | 已证明 |
| admin token 真实有效 | 已证明 |
| MW 路由到 `handleAdminCreateApp` | 已证明 |
| MW 返回 201 | 已证明 |
| App 能解析 `app_id` | 已证明 |
| API key 只返回一次且未打印 | 已证明 |
| `apps` 表真实落库 | 留给 P3.2 |
| App key 可用 | 留给 P3.3 |

## 5. 真实架构位置

```text
counter_agent App
  │  POST /admin/apps
  ▼
Middleware HTTP API
  │  handleAdminCreateApp
  ▼
Register
  │  INSERT INTO apps
  ▼
PostgreSQL apps
  │  CreateAppResponse
  ▼
counter_agent http_client.parse_create_app_response
```

## 6. 和之前 mock 的区别

| 项 | P2.2/P2.3 | P3.1 |
|---|---|---|
| transport | RecordingTransport | UrllibTransport |
| 网络 | 无 | 真实 TCP |
| MW | 未经过 | 真实经过 |
| 鉴权 | 未验证 | 真实 admin token |
| 响应 | 写死 JSON | 真实 201 |
| DB | 未验证 | P3.2 验证 |
| 结论 | 只验证客户端形状 | 真实链路打通到响应 |

## 7. 证据

```text
docs/reports/artifacts/retrofit/P3.1/
  request.txt         请求形状（token 脱敏）
  response.json       App 安全输出
  middleware.log      真实路由/201 日志
  migrate.log         迁移
  register_app.err    空
  summary.txt
```

## 8. 当前不覆盖

- `apps` 表落库校验（P3.2）。
- App key 认证（P3.3）。
- 工具注册（P3.4）。
- tool_defs（P3.5）。
- WS（P5）。

## 9. 下一步

P3.2：直接查 `apps` 表，确认：

```text
app_id 存在
name 匹配
status=active
api_key_hash 长度 64 且不是明文
```
