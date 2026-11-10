<!-- Middleware — reusable App onboarding procedure. -->

# App 接入 Middleware 标准流程

目标：任何已有项目都按同一流程接入，并能逐节点验证，而不是只跑一个 demo。

```text
业务函数
  → 工具元数据
  → Admin bootstrap
  → App key
  → 工具注册
  → TaskAssign 可见
  → WS 工具宿主
  → Harness 调用
  → 事件/结果闭环
```

## 1. 角色分离

| 角色 | 凭据 | 职责 |
|---|---|---|
| Admin / 运维 | `admin_token` | 创建 App、rotate key |
| App runtime | `app_api_key` | 注册工具、提交 Run、换 ws_token、连 WS |
| App WS | `ws_token` | 只用于 WS 连接 |
| Daemon | `daemon_token` | Hello 鉴权 |

Admin token **不进入 App 长期运行环境**。

## 2. 标准目录

```text
<app>/
  config.py           读 MW_URL / MW_APP_NAME / MW_ADMIN_TOKEN
  credentials.py      保存/加载 0600 App 凭证
  bootstrap_app.py    Admin 侧 bootstrap
  check_app.py        App key 验证
  http_client.py      AdminClient / AppClient
  tools.py            业务函数 + TOOL_DEFINITIONS
  dispatch.py         local_tool_call 执行入口
  register_tools.py   注册工具（后续）
  ws_client.py        WebSocket 工具宿主（后续）
```

## 3. 第 1 步：配置

环境变量：

```sh
MW_URL=http://127.0.0.1:8080
MW_ADMIN_TOKEN=admin-change-me
MW_APP_NAME=my-app
COUNTER_APP_CREDENTIALS=~/.config/middleware/my-app.json
```

规则：

- URL 可配置。
- App name 全局唯一。
- Admin token 只用于 bootstrap。
- 凭证文件默认 0600。

## 4. 第 2 步：Admin bootstrap

```sh
python3 bootstrap_app.py
```

真实调用：

```http
POST /admin/apps
Authorization: Bearer <admin_token>
```

```json
{"name": "my-app", "multiagent": false}
```

返回：

```text
201 + api_key（新 App）
200 + api_key=""（已存在，需要 rotate）
```

如果已存在或显式 `--rotate`：

```http
POST /admin/apps/{app_id}/rotate-key
```

凭证写入：

```json
{
  "app_id": "app_...",
  "app_name": "my-app",
  "api_key": "mw_..."
}
```

要求：

- 文件权限 0600。
- 不打印 key。
- 不进 git。
- 不进日志/证据。

## 5. 第 3 步：验证 App key

```sh
python3 check_app.py
```

真实调用：

```http
GET /api/v1/apps/{app_id}
Authorization: Bearer <app_api_key>
```

期望：

```text
200 + status=active
```

错误 key：

```text
401 UNAUTHORIZED
```

通过后再进行工具注册。

## 6. 第 4 步：注册工具

工具元数据：

```json
{
  "tools": [
    {
      "name": "counter_add",
      "description": "...",
      "schema": {"type": "object", "...": "..."},
      "timeout_ms": 10000
    }
  ]
}
```

真实调用：

```http
POST /api/v1/apps
Authorization: Bearer <app_api_key>
```

规则：

- `tools` 全量替换。
- 新增工具必须提交完整列表。
- `"tools": []` 清空。

## 7. 第 5 步：验证落库

```sql
SELECT app_id, name, status, api_key_hash
FROM apps WHERE app_id = '<app_id>';

SELECT name, scope, app_id, schema_json, timeout_ms, version
FROM tool_defs WHERE app_id = '<app_id>' ORDER BY name;
```

期望：

```text
apps.api_key_hash = 64 位 SHA-256 hex
tool_defs.scope = local
tool_defs.app_id = 当前 App
```

## 8. 第 6 步：验证 TaskAssign

创建 echo/react Run 后，真实 Harness 应看到：

```json
{
  "tools": [
    {
      "name": "counter_add",
      "description": "...",
      "input_schema_json": "{\"type\":\"object\",...}",
      "scope": "LOCAL"
    }
  ]
}
```

这一步证明：

```text
注册 → DB → Control 组装 → TaskAssign
```

## 9. 第 7 步：ws_token 与 WS

```http
POST /api/v1/ws-token
Authorization: Bearer <app_api_key>
```

```json
{"ws_token": "...", "expires_in": 600}
```

连接：

```text
GET /api/v1/ws?app_id=<app_id>&token=<ws_token>
```

只把 ws_token 放内存，不持久化。

## 10. 第 8 步：工具宿主

收到：

```json
{
  "type": "local_tool_call",
  "msg_id": "01...",
  "invocation_id": "01...",
  "tool_name": "counter_add",
  "arguments_json": "{\"delta\":2}",
  "timeout_ms": 10000
}
```

执行：

```text
dispatch(tool_name, arguments_json)
```

返回：

```json
{
  "type": "local_tool_result",
  "correlation_id": "01...",
  "invocation_id": "01...",
  "ok": true,
  "result_json": "{\"value\":2}"
}
```

## 11. 第 9 步：全链路验收

一条 Run 必须能核对：

```text
run_id
trace_id
apps 行
tool_defs 行
TaskAssign tools
tool_invocations 行
run_events TOOL_CALL_BEGIN/END
最终 run status
WS run_completed
```

## 12. 每步证据

```text
docs/reports/artifacts/retrofit/<step>/
  request.txt
  response.json
  db.txt
  middleware.log
  summary.txt
```

`summary.txt`：

```text
step / commit / time / app_id / run_id / trace_id
expected / observed / evidence / design
```

## 13. 安全规则

- Admin token 只用于 bootstrap。
- App key 只存 0600 文件或密钥介质。
- ws_token 只放内存。
- 不打印 key。
- 不把 key 写入 git/日志/证据。
- 跨 App 一律 404/403。
- 凭证文件在测试后删除或 rotate。

## 14. 当前 counter_app 进度

已完成：

```text
P1  本地工具函数/元数据/dispatcher
P2.1 配置
P3.1 真实 POST /admin/apps
P3.2 apps 落库
P3.3 App key 200/401
```

下一步：

```text
P3.4 真实注册工具
P3.5 tool_defs 落库
P4   TaskAssign 可见
P5   ws_token + WS
P6   Harness + 工具闭环
```
