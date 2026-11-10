<!-- Middleware — retrofit P3.3 App key authentication. -->

# Retrofit P3.3：App key 认证与可复用 bootstrap

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P3.3 |
| 状态 | 完成 |
| 代码提交 | `02e5da7` |
| 范围 | 真实 bootstrap、真实 App key、真实 200/401 |

## 1. 这一步真实发生了什么

### 1.1 Bootstrap（Admin 侧）

```text
bootstrap_app.py
  → 读取 MW_URL / MW_ADMIN_TOKEN / MW_APP_NAME
  → 真实 POST /admin/apps
  → 得到 app_id + 一次性 api_key
  → 写入 0600 credentials 文件
```

实际输出：

```json
{
  "app_id": "app_01M72B7V133Z0K257WKM1WRDVQ",
  "name": "retrofit-p3.3-1794301488",
  "credentials": "/tmp/p3.3-counter-credentials.json",
  "rotated": false
}
```

没有打印 key。

### 1.2 Runtime（App 侧）

```text
check_app.py
  → 读取 0600 credentials
  → GET /api/v1/apps/{app_id}
  → Authorization: Bearer <app_api_key>
```

真实结果：

```json
{
  "status": 200,
  "app_id": "app_01M72B7V133Z0K257WKM1WRDVQ",
  "name": "retrofit-p3.3-1794301488",
  "app_status": "active"
}
```

### 1.3 错误 key

```text
Authorization: Bearer mw_invalid
```

真实结果：

```json
{
  "status": 401,
  "code": "UNAUTHORIZED",
  "message": "app API key is required"
}
```

## 2. MW 内部真实路径

```text
GET /api/v1/apps/{app_id}
  → handleGetApp
  → appFromRequest
      → bearerToken
      → apps.AuthenticateAPIKey
          → hashAPIKey(key)
          → SELECT WHERE api_key_hash = $1 AND status='active'
  → app.AppID == path app_id
  → 200 AppView
```

错误 key：

```text
AuthenticateAPIKey → ErrAppNotFound
→ 401 UNAUTHORIZED
```

代码：

```text
internal/httpapi/server.go       handleGetApp
internal/httpapi/server.go       appFromRequest
internal/register/registry.go    AuthenticateAPIKey
```

## 3. Credentials 生命周期

```text
Admin token
  → 只用于 create/rotate
  → 不进入 App runtime

App key
  → bootstrap 时获得
  → 写入 0600 文件
  → runtime 读取
  → 用于 HTTP / ws-token
  → 不打印、不入库、不进 git

ws_token
  → 后续由 App key 换取
  → 10 分钟
  → 只放内存
```

凭证文件权限已验证：

```text
mode=600
```

测试结束后已删除临时凭证文件。

## 4. 可复用结构

新增：

```text
examples/counter_agent/
  bootstrap_app.py    Admin 侧：创建/rotate + 写 0600 凭证
  credentials.py      凭证保存/加载/权限校验
  check_app.py        App 侧：用 App key 验证身份
  http_client.py      AdminClient / AppClient / urllib transport
```

职责分离：

| 工具 | 凭据 | 用途 |
|---|---|---|
| `bootstrap_app.py` | Admin token | 只做接入准备 |
| `check_app.py` | App key | 验证运行时身份 |
| 未来 `counter_app` | App key | 注册工具、提交 Run、连 WS |

## 5. 对任意 App 的可复用步骤

```sh
# 1. 设置接入参数
export MW_URL=http://127.0.0.1:8080
export MW_ADMIN_TOKEN=...
export MW_APP_NAME=my-app
export COUNTER_APP_CREDENTIALS=~/.config/middleware/my-app.json

# 2. Admin 侧 bootstrap
python3 examples/counter_agent/bootstrap_app.py

# 3. App 侧验证 key
python3 examples/counter_agent/check_app.py
```

对其他项目：

- 保留自己的业务函数。
- 用同样的 bootstrap/credentials 机制。
- 注册自己的 tools。
- 使用同一个 `AppClient`/WS 客户端模式。

详细通用流程：

```text
docs/APP_ONBOARDING.md
```

## 6. 验证

单元测试：

```text
test_p3_3_round_trip_and_mode ... ok
test_p3_3_rejects_group_readable_file ... ok
test_p3_3_rejects_missing_fields ... ok
test_p3_3_rotate_key_request ... ok
test_p3_3_get_app_request_and_response ... ok
test_p3_3_get_app_rejects_bad_key ... ok
Ran 6 tests
OK
```

全量：

```text
make counter-agent-test
tests 29
OK
```

真实链路：

```text
POST /admin/apps            → 201
GET /api/v1/apps/{app_id}   → 200
GET /api/v1/apps/{app_id}   → 401（错误 key）
```

MW 日志：

```text
POST /admin/apps              status=201
GET /api/v1/apps/{app_id}     status=200
GET /api/v1/apps/{app_id}     status=401
```

## 7. 证据

```text
docs/reports/artifacts/retrofit/P3.3/
  bootstrap.json
  check_app.json
  wrong_key.json
  credentials_mode.txt
  middleware.log
  migrate.log
  summary.txt
```

没有保存 API key；临时凭证文件已删除。

## 8. 当前不覆盖

- 工具注册（P3.4）。
- `tool_defs`（P3.5）。
- ws_token（P5）。
- WS 连接（P5）。
- Harness 工具调用（P6）。

## 9. 下一步

P3.4：用 App key 真实注册工具。

```text
POST /api/v1/apps
Authorization: Bearer <app_api_key>
tools = TOOL_DEFINITIONS
```
