<!-- Middleware — MCP link refactor M1/M2. -->

# MCP 链路改造：M1 运行时凭据解析 + M2 目录 API

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-11 |
| 状态 | M1/M2 完成；M3 端到端待做 |
| 范围 | 总控目录、TaskAssign 绑定语义、runtime 凭据解析 |
| 影响 | MCP 凭据不再进入 TaskAssign/Harness |

## 1. 改了什么

### 1.1 总控不再解析 MCP 凭据

`internal/register/materials.go`：

```text
buildMCPMaterial
  之前：resolveString / resolveMap 把 ${secret:*} 替换成明文
  现在：保留占位符，原样进入 McpBinding
```

仍然解析凭据的只有 global tool 的 `config_json`（与 MCP 无关）。

### 1.2 新增 MCP 目录 API

```http
POST   /admin/mcp
GET    /admin/mcp
DELETE /admin/mcp/{name}[?force=true]
Authorization: Bearer {ADMIN_TOKEN}
```

- `POST`：name / transport / config / allowed_tools；返回 `secret_refs`，不回显 config。
- `GET`：只返回 name / transport / allowed_tools / secret_refs。
- `DELETE`：被 App 引用时 409；`force=true` 同时清理引用。

校验：

```text
name 必填
transport ∈ stdio / sse / streamable_http
stdio → config.command
sse / streamable_http → config.url
config 必须是 JSON 对象
```

### 1.3 runtime 解析凭据

`mcp_runtime/src/mcp_runtime/secrets.py`：

```text
EnvSecretResolver    MIDDLEWARE_SECRET_<NAME>
FileSecretResolver   --secrets-file 0600 JSON
ChainResolver        文件优先，环境变量兜底
resolve_binding()    command / args / env / url / headers
```

`SessionManager.open` 在连接前解析；缺失时返回：

```text
SECRET_MISSING   retryable=false
```

并记录 `mcp session opened/closed`（只记 server/session/tools/耗时）。

### 1.4 Harness 只转发

`task assign received` 日志增加：

```text
mcp_binding_count
mcp_bindings: [{server_name, transport, allowed_tool_count}]
```

不打印 config/env/header 值。Harness 不解析占位符。

## 2. 验证

### 2.1 M1：真实 stdio MCP server

探针启动 runtime，绑定：

```text
command = mcp_runtime/.venv/bin/python
args    = [secret_echo_server.py]
env     = {"PROBE_TOKEN": "${secret:probe_token}"}
```

结果：

```json
{
  "health": {"status": "ok", "sessions": 0, "version": "0.1.0"},
  "tools": ["mcp__secret-echo__echo_token"],
  "call_ok": true,
  "call_result": "... \"text\":\"resolved-from-env\" ...",
  "missing_secret_code": "SECRET_MISSING"
}
```

即：占位符由 runtime 用环境变量 `MIDDLEWARE_SECRET_PROBE_TOKEN` 解析，
子进程拿到 `resolved-from-env`；缺失时在连接前失败。

### 2.2 M2：真实目录 API

```text
POST /admin/mcp            → 200
{
  "name": "probe-...",
  "transport": "stdio",
  "allowed_tools": ["echo"],
  "secret_refs": ["probe_token"]
}

GET /admin/mcp             → 不含 config
DB config_json             → ${secret:probe_token}（无明文）
POST /api/v1/apps mcp_refs → 200
DELETE 未 force            → 409
DELETE force               → 200
```

### 2.3 测试

```text
make mcp-runtime-test   26 tests OK（新增 secret 解析/缺失）
make harness-test       82 tests OK
go test ./...           OK（新增目录 API、占位符保持）
make lint               OK
make build              OK
```

新增用例：

```text
internal/register: TestMCPBindingAssemblyKeepsPlaceholders
                   TestListAndDeleteMCPServers
internal/httpapi:  TestAdminMCPCatalogLifecycle
                   TestAdminMCPValidation
mcp_runtime:       test_secrets.py
                   test_open_resolves_secret_placeholders
                   test_open_missing_secret_fails_before_connect
```

## 3. 边界

```text
总控：目录 + ACL + 绑定描述（占位符）
Runtime：连接 + 会话 + 凭据解析 + allowed_tools
Harness：转发描述、调用 runtime，不见明文
```

MCP 调用仍不经过 MW Gateway，`tool_invocations` 无 MCP 行。

## 4. 未做 / 下一步

| 项 | 状态 |
|---|---|
| M3 端到端：MW 目录 → TaskAssign → Harness → runtime 真实调用 | 待做 |
| allowed_tools 负向 / 缺失 secret 的真实 Run | 待做 |
| 真实第三方 MCP server | 待做 |
| `make mcp-e2e` | 待做 |
| 目录 API 的 App 侧可见性（GET /apps 返回 mcp_refs） | 未做 |

## 5. 证据

```text
docs/reports/artifacts/mcp/M1/
  probe.py
  secret_echo_server.py
  probe.json
  runtime.log
  summary.txt

docs/reports/artifacts/mcp/M2/
  result.txt
  upsert.json
  list.json
  app_update.json
  delete_in_use.json
  delete_force.json
  db.txt
  middleware.log
  summary.txt
```

## 6. 设计文档同步

- `docs/DESIGN.md`：MCP 注册、材料组装、proto 注释、密钥管理改为“runtime 解析”。
- `docs/adr/0004-mcp-ownership.md`：新增“凭据解析归属”补充。
