<!-- Middleware — MCP operational guide. -->

# MCP 接入与运行

这份文档只讲当前实现的 MCP 链路，不重复 App 本地工具。

## 1. 边界

```text
总控（MW）
  目录：mcp_server_defs
  引用：app_mcp_refs
  下发：TaskAssign.mcp_bindings（只含描述和 ${secret:...} 占位）
  不建立连接、不解析凭据、不代理调用

mcp_runtime（独立进程）
  建立 MCP 连接 / 拉起 stdio 子进程
  解析 ${secret:...}
  按 allowed_tools 过滤
  按 Run 持有 session，结束即关闭

Harness
  每个 Run 通过本地 JSON-RPC 打开 binding
  把 mcp__<server>__<tool> 合并进模型工具清单
  调用结果回填给模型

MW 事件
  只能看到 Harness 发出的 TOOL_CALL_BEGIN / END
  没有 tool_invocations 行（MCP 不经过 Gateway）
```

## 2. 注册 MCP server

```http
POST /admin/mcp
Authorization: Bearer {ADMIN_TOKEN}
```

```json
{
  "name": "github",
  "transport": "stdio",
  "config": {
    "command": "npx",
    "args": ["-y", "@modelcontextprotocol/server-github"],
    "env": {"GITHUB_PERSONAL_ACCESS_TOKEN": "${secret:github_token}"}
  },
  "allowed_tools": ["search_repositories", "get_file_contents"]
}
```

HTTP 形态：

```json
{
  "name": "remote",
  "transport": "streamable_http",
  "config": {
    "url": "https://mcp.example.com/mcp",
    "headers": {"Authorization": "Bearer ${secret:remote_token}"}
  }
}
```

查询与删除：

```http
GET    /admin/mcp
DELETE /admin/mcp/{name}
DELETE /admin/mcp/{name}?force=true
```

- `GET` 不回显 config，只返回 `secret_refs`。
- 被 App 引用时删除返回 409；`force=true` 同时清理引用。

## 3. App 引用

随 App 注册：

```http
POST /api/v1/apps
Authorization: Bearer {APP_KEY}
```

```json
{"name": "my-app", "mcp_refs": ["github"]}
```

只引用已注册的名字；未知名字会 400。

## 4. Runtime

启动：

```sh
PYTHONPATH=execution/mcp_runtime/src execution/mcp_runtime/.venv/bin/python -m levier_mcp_runtime \
  --socket /tmp/levier-mcp.sock \
  --secrets-file /path/to/secrets.json \
  --log-level info
```

凭据来源：

```text
LEVIER_SECRET_<NAME>     环境变量（旧名 MIDDLEWARE_SECRET_ 仍兼容）
--secrets-file               0600 JSON，{"name":"value"}
```

规则：

```text
socket 文件 0600
只解析 ${secret:name}
缺失 → SECRET_MISSING，连接不会建立
日志只记录 server / session / 工具数 / 耗时
```

Harness 必须指向同一个 socket（`execution/harness/levier.example.yaml` 的 `mcp_runtime.socket`）。

## 5. 工具命名与过滤

```text
namespace = mcp__<sanitized_server_name>__
tool      = namespace + <raw tool name>
```

示例：

```text
server github → mcp__github__search_repositories
server e2e-echo → mcp__e2e_echo__echo_token
```

`allowed_tools`：

- 未列出的工具不出现在 `tools/list`
- 直接调用未授权工具返回 `TOOL_DENIED`

## 6. 失败语义

| 故障 | 看到什么 | Run |
|---|---|---|
| runtime 未启动 | `MCP_RUNTIME_UNAVAILABLE` | 继续，MCP 工具不可用 |
| server 起不来 | `MCP_CONNECT_FAILED` | 继续 |
| secret 缺失 | `SECRET_MISSING` | 继续 |
| 未授权工具 | `TOOL_DENIED` | 模型收到错误 |
| 工具超时 | `TOOL_TIMEOUT` | 模型收到错误 |
| 客户端断开 | 该 owner 的 session 回收 | — |

单个 MCP server 失败不会中断 Run；错误以 `ERROR` 事件出现。

## 7. 安全

```text
总控 DB：只存占位符
TaskAssign：只带占位符
Harness：不见明文
Runtime：解析后只放在内存/子进程环境
日志：不打印 config / env / header 值
```

## 8. 一键回归

```sh
make mcp-e2e
```

它会：

```text
启动 MW + runtime + Harness + App
注册 e2e-echo（stdio，${secret:e2e_token}）
设置 App mcp_refs
跑一次真实模型调用
断言：completed / 1 次 ok / 结果含 secret / 0 invocation / 0 ERROR
清理进程与凭据
```

回归入口：`make mcp-e2e`。

## 9. 当前限制

- 真实链路已验证 stdio 与本地 SSE / streamable_http；TLS、反向代理、服务端鉴权挑战未测。
- MCP 调用不进入 `tool_invocations`，没有 Gateway 幂等/配额。
- MW 不负责启动或托管 runtime，部署方需要保证 socket 可达。
- 第三方 server 依赖外部安装（例如 npm）；生产应固定版本或使用离线镜像。
- session 按 Run 建立；没有跨 Run 连接池。
### 传输中断

```text
SSE 没有自动重连
streamable_http 的 GET 事件流可以带 Last-Event-ID 重连
正在进行的 tools/call 不会重放

调用中断 → TOOL_EXEC_ERROR
会话保留到 close
server 恢复后新会话可以继续
```

## 10. 回归

```sh
make mcp-e2e
```
