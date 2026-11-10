<!-- Middleware — retrofit P3.4 real tool registration. -->

# Retrofit P3.4：真实注册 local tools

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P3.4 |
| 状态 | 完成 |
| 代码提交 | `3c4a1a8` |
| 范围 | 真实 App key + 真实 MW + 真实工具注册 |

## 1. 真实调用

App：

```text
bootstrap_app.py        → 创建 App、写 0600 凭证
register_tools.py       → 读取 App key
  → POST /api/v1/apps
```

请求：

```http
POST /api/v1/apps
Authorization: Bearer <app_api_key>
Content-Type: application/json
```

Body：

```json
{
  "name": "retrofit-p3.4-1794301946",
  "multiagent": false,
  "tools": [
    {
      "name": "counter_add",
      "description": "...",
      "schema": {"type": "object", "...": "..."},
      "timeout_ms": 10000
    },
    ...
  ]
}
```

## 2. MW 内部真实路径

```text
POST /api/v1/apps
  → handleUpdateApp
      → appFromRequest
          → AuthenticateAPIKey
      → readJSONBody
      → AppUpsertRequest.Validate
      → UpdateApp
      → applyAppCatalog
          → ReplaceLocalTools
              → DELETE local tools not in list
              → UPSERT each tool
              → version++
```

代码：

```text
internal/httpapi/server.go       handleUpdateApp / applyAppCatalog
internal/register/materials.go   ReplaceLocalTools
```

## 3. 真实返回

```json
{
  "status": 200,
  "app_id": "app_01M72BNV0VX8A1BHP7W6CN7KE6",
  "name": "retrofit-p3.4-1794301946",
  "app_status": "active",
  "tool_count": 3,
  "tools": ["counter_add", "counter_get", "counter_reset"]
}
```

注意：

- 这个接口返回的是 `AppView`。
- 不返回工具列表。
- 工具是否真的写入，必须查 `tool_defs`（P3.5）。

## 4. MW 日志证明真实路由

```json
{
  "msg": "http request",
  "method": "POST",
  "route": "POST /api/v1/apps",
  "status": 200,
  "duration_ms": 5
}
```

完整链路日志：

```text
POST /admin/apps       status=201
POST /api/v1/apps      status=200
```

## 5. 这一步证明了什么

| 项目 | 结果 |
|---|---|
| bootstrap 写入 0600 凭证 | 已证明 |
| App key 真实可用 | 已证明 |
| toolbar 注册请求到达 MW | 已证明 |
| MW 返回 200 | 已证明 |
| App key 只能改自己 App | 由 `handleUpdateApp` 保证 |
| `tool_defs` 真实落库 | 留给 P3.5 |
| TaskAssign 可见 | 留给 P4 |

## 6. 可复用命令

任意 App：

```sh
export MW_URL=http://127.0.0.1:8080
export MW_ADMIN_TOKEN=...
export MW_APP_NAME=my-app
export COUNTER_APP_CREDENTIALS=~/.config/middleware/my-app.json

python3 bootstrap_app.py
python3 register_tools.py
```

只需要把 `TOOL_DEFINITIONS` 换成目标项目的工具列表。

## 7. 安全

- 注册只用 App key。
- Admin token 不参与工具注册。
- 凭证文件 0600。
- 证据不包含 key。
- 临时凭证测试后删除。

## 8. 证据

```text
docs/reports/artifacts/retrofit/P3.4/
  bootstrap.json
  registration_payload.json
  register_tools.json
  middleware.log
  migrate.log
  bootstrap.err
  register_tools.err
  summary.txt
```

## 9. 下一步

P3.5：查 `tool_defs`，确认：

```text
counter_add / counter_get / counter_reset
scope=local
app_id 正确
schema_json 与注册一致
timeout_ms=10000
version=1
```
