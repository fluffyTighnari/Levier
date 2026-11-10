<!-- Middleware — language-neutral App onboarding procedure. -->

# App 接入 Middleware 标准流程（语言无关）

Middleware 不托管 App 代码，也不要求 App 使用特定语言或 SDK。  
App 只需要能发 HTTP、解析 JSON、安全保存凭据。

下面用自然语言和伪代码描述“任何 App 如何接入”。

---

## 1. 接入前必须确定的三个值

每个 App 接入前，先确定：

```text
MW_URL          Middleware HTTP 地址，例如 http://127.0.0.1:8080
MW_ADMIN_TOKEN  管理凭据，只用于创建/轮换 App
APP_NAME        App 自己选择的唯一名称
```

说明：

- `MW_URL`：由 Middleware 部署方提供，不是秘密。
- `MW_ADMIN_TOKEN`：由 Middleware 运维方提供，是 bootstrap 凭据。
- `APP_NAME`：App 自己决定，但必须全局唯一。

App 长期运行时还需要：

```text
APP_ID           Middleware 生成，客户端保存
APP_KEY          Middleware 生成，客户端安全保存
```

---

## 2. 第一步：创建或找回 App

App 或运维工具发送：

```http
POST {MW_URL}/admin/apps
Authorization: Bearer {MW_ADMIN_TOKEN}
Content-Type: application/json
```

Body：

```json
{
  "name": "{APP_NAME}",
  "multiagent": false
}
```

响应可能是：

### 新建成功

```http
201 Created
```

```json
{
  "app": {
    "app_id": "app_...",
    "name": "{APP_NAME}",
    "status": "active"
  },
  "api_key": "mw_..."
}
```

### App 已存在

```http
200 OK
```

```json
{
  "app": {
    "app_id": "app_...",
    "name": "{APP_NAME}",
    "status": "active"
  },
  "api_key": ""
}
```

因为 API key 只在创建时返回一次。

如果 `api_key` 为空，需要 rotate：

```http
POST {MW_URL}/admin/apps/{app_id}/rotate-key
Authorization: Bearer {MW_ADMIN_TOKEN}
```

```json
{"api_key": "mw_..."}
```

伪代码：

```text
resp = POST /admin/apps {name: APP_NAME}
app_id = resp.app.app_id
if resp.api_key is not empty:
    app_key = resp.api_key
else:
    app_key = POST /admin/apps/{app_id}/rotate-key .api_key

save_secret(app_id, app_key)
```

---

## 3. 第二步：安全保存 App 凭据

需要保存：

```text
APP_ID   非秘密，但运行时需要
APP_KEY  秘密
```

要求：

- 不写入源码。
- 不提交 git。
- 不打印日志。
- 不放进错误信息。
- 不放进普通数据库明文。
- 推荐：环境变量、密钥管理服务、权限 0600 的本地密钥文件。

App 运行时读取：

```text
APP_ID
APP_KEY
MW_URL
```

Admin token 不应进入 App 长期运行环境。

伪代码：

```text
credentials = load_secret_store()
if credentials.missing:
    fail("run bootstrap first")

runtime_config = {
    mw_url: MW_URL,
    app_id: credentials.app_id,
    app_key: credentials.app_key,
}
```

---

## 4. 第三步：验证 App key

发送：

```http
GET {MW_URL}/api/v1/apps/{APP_ID}
Authorization: Bearer {APP_KEY}
```

期望：

```text
200 + status=active
```

错误：

```text
401 UNAUTHORIZED
```

这一步只验证：

```text
APP_KEY 是否属于这个 APP_ID
```

不验证工具，不验证 Run。

伪代码：

```text
resp = GET /api/v1/apps/{APP_ID} with Bearer APP_KEY
if resp.status != 200:
    fail("app key invalid")
```

---

## 5. 第四步：准备本地工具元数据

App 把自己已有的函数整理成工具：

```text
name         工具名，必须唯一
description  给模型看的说明
schema       JSON Schema，描述参数
timeout_ms   工具超时
```

例如：

```json
{
  "name": "counter_add",
  "description": "Add an integer delta to the counter.",
  "schema": {
    "type": "object",
    "properties": {
      "delta": {"type": "integer"}
    },
    "required": ["delta"],
    "additionalProperties": false
  },
  "timeout_ms": 10000
}
```

注意：

- 业务函数仍然是 App 自己的代码。
- Middleware 只存元数据。
- 模型看到的是 schema，不看到实现。

---

## 6. 第五步：注册工具

发送：

```http
POST {MW_URL}/api/v1/apps
Authorization: Bearer {APP_KEY}
Content-Type: application/json
```

Body：

```json
{
  "name": "{APP_NAME}",
  "multiagent": false,
  "tools": [
    { "name": "...", "description": "...", "schema": {...}, "timeout_ms": 10000 }
  ]
}
```

规则：

- `tools` 是**全量替换**。
- 新增工具时必须提交完整列表。
- `"tools": []` 清空。
- 不传 `tools` 不修改。
- `schema` 为空时 Middleware 会补 `{}`。
- 同名工具重写会使 `version` 增加。

伪代码：

```text
resp = POST /api/v1/apps {
    name: APP_NAME,
    tools: local_tool_definitions,
} with Bearer APP_KEY

if resp.status != 200:
    fail(resp.error)
```

---

## 7. 第六步：验证工具真的写入

注册接口返回的是 App 信息，不返回工具列表。  
要验证工具，需要查 Middleware 的 PostgreSQL：

```sql
SELECT name, scope, app_id, schema_json, timeout_ms, version
FROM tool_defs
WHERE app_id = '{APP_ID}'
ORDER BY name;
```

期望：

```text
scope = local
app_id = APP_ID
schema_json = 注册时的 schema
timeout_ms = 注册值
version = 1
```

这一步确认：

```text
HTTP 注册 → tool_defs 落库
```

---

## 8. 第七步：确认 TaskAssign 可见

App 自己不直接看到 TaskAssign；它由 Middleware 发给 Harness/Daemon。

运维或诊断工具应确认 TaskAssign 中有：

```text
tools[].name
tools[].input_schema_json
tools[].scope = LOCAL
```

这证明：

```text
tool_defs
  → Control 组装
  → TaskAssign
  → Harness
```

---

## 9. 第八步：提交 Run

App 运行时发送：

```http
POST {MW_URL}/api/v1/runs
Authorization: Bearer {APP_KEY}
Content-Type: application/json
```

示例：

```json
{
  "runtime": "normal",
  "strategy": {"name": "react", "params": {"max_steps": 8}},
  "input": [
    {"role": "user", "content": "Add 2 then 3 and read the counter."}
  ],
  "model": {"provider": "deepseek", "model": "deepseek-flash"},
  "timeouts": {"run_ms": 60000, "tool_ms": 10000}
}
```

期望：

```http
202 Accepted
```

```json
{
  "run_id": "01...",
  "status": "queued",
  "ws_url": "/api/v1/ws",
  "ws_token": "..."
}
```

App 需要保存：

```text
run_id   非秘密，用于查询/WS
ws_token 短期秘密，只放内存
```

---

## 10. 第九步：查询状态与事件

```http
GET {MW_URL}/api/v1/runs/{run_id}
GET {MW_URL}/api/v1/runs/{run_id}/events
Authorization: Bearer {APP_KEY}
```

用途：

- 查询终态。
- 断线后补拉事件。
- 调试工具调用事件。

---

## 11. 第十步：WebSocket（后续单独文档）

WS 接入尚未写入本流程。

后续需要说明：

- 如何用 App key 换 ws_token。
- 如何建立 WS。
- 如何接收 `local_tool_call`。
- 如何返回 `local_tool_result`。
- 如何订阅事件、补拉、ping/pong。

在 WS 文档完成前，App 只能提交/查询 Run，不能执行 local tools。

---

## 12. 任意语言的实现要点

只需要满足：

```text
1. 能发 HTTPS/HTTP 请求
2. 能解析和生成 JSON
3. 能安全保存 APP_ID/APP_KEY
4. 能按字符串处理 schema/arguments/result JSON
```

不需要：

```text
Middleware SDK
特定语言
生成的 protobuf 代码
文件布局
```

伪代码总览：

```text
bootstrap:
    ensure APP_NAME
    POST /admin/apps
    if no key: POST rotate-key
    save_secret(app_id, app_key)

runtime:
    load MW_URL, APP_ID, APP_KEY
    GET /apps/{APP_ID} to verify
    POST /api/v1/apps with tools
    POST /api/v1/runs
    poll/stream events
```

---

## 13. 验收清单

| 检查 | 期望 |
|---|---|
| App 创建 | 201/200 |
| App key | 只返回一次 |
| DB `apps` | hash 非明文 |
| App key 认证 | 200 |
| 错误 key | 401 |
| 工具注册 | 200 |
| `tool_defs` | local + app_id + schema + timeout |
| TaskAssign | 包含工具 |
| Run 提交 | 202 |
| Run 终态 | completed/failed |
| 日志/证据 | 无 key/token/secret |

---

## 14. 参考实现

`examples/counter_agent/` 是一个具体实现，用于演示上述流程：

```text
P3.1 真实 POST /admin/apps
P3.2 apps 落库
P3.3 App key 200/401
P3.4 工具注册
P3.5 tool_defs 落库
P4.1–P4.3 TaskAssign 可见
```

它只是参考，不是接入标准。  
其他语言按本文件第 1–13 节实现同样的 HTTP 行为即可。
