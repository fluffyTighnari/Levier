<!-- Middleware — MCP end-to-end TaskAssign path. -->

# MCP M3：目录 → TaskAssign → Harness → Runtime

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-11 |
| 状态 | 完成 |
| 样本 Run | `01M75B4S3F52Q901DXK0BVE23R` |
| 依赖 | M1 凭据解析、M2 目录 API |

## 1. 拓扑

```text
POST /admin/mcp             目录 m3echo
POST /api/v1/apps            App mcp_refs = [m3echo]
POST /api/v1/runs            react
        ↓
AssembleTaskMaterials
        ↓
TaskAssign.mcp_bindings      env 仍是 ${secret:m3_token}
        ↓
Harness ToolRouter.open
        ↓ Unix socket
mcp_runtime                 解析 secret → 启动 stdio server → tools/list
        ↓
模型调用 mcp__m3echo__echo_token
        ↓
Runtime.call → MCP server → 结果回 Harness
        ↓
Run completed
```

## 2. 目录与引用

```json
POST /admin/mcp
{
  "name": "m3echo",
  "transport": "stdio",
  "config": {
    "command": ".../mcp_runtime/.venv/bin/python",
    "args": [".../secret_echo_server.py"],
    "env": {"PROBE_TOKEN": "${secret:m3_token}"}
  },
  "allowed_tools": ["echo_token"]
}
```

```text
GET /admin/mcp → secret_refs=["m3_token"]，无 config
app_mcp_refs   → m3echo
```

Runtime 以 `MIDDLEWARE_SECRET_M3_TOKEN=m3-secret-value` 启动。

## 3. TaskAssign

Harness 日志：

```json
{
  "message": "task assign received",
  "run_id": "01M75B4S3F52Q901DXK0BVE23R",
  "tool_count": 0,
  "mcp_binding_count": 1,
  "mcp_bindings": [
    {"server_name": "m3echo", "transport": "stdio", "allowed_tool_count": 1}
  ]
}
```

说明绑定经过调度到达了 Harness，且没有本地工具参与。

## 4. Runtime

```text
mcp runtime listening
mcp session opened
mcp session closed
```

绑定里的 `PROBE_TOKEN=${secret:m3_token}` 在 runtime 内解析；
Harness 和 MW 日志中都没有出现明文。

## 5. 工具调用

```text
seq 4  TOOL_CALL_BEGIN  mcp__m3echo__echo_token
seq 5  TOOL_CALL_END    ok=true
       result_json =
       {"content":[{"type":"text","text":"m3-secret-value"}],
        "structured_content":{"result":"m3-secret-value"},
        "is_error":false}
seq 6  USAGE
seq 7  MESSAGE_COMPLETED  "The tool returned: **m3-secret-value**"
seq 8  RUN_STATUS completed
```

`tool_invocations`：

```text
mcp_invocations = 0
```

MCP 调用没有进入 Gateway 的幂等/审计表，符合设计边界。

## 6. 安全核对

```text
middleware.log 出现 m3-secret-value：0
harness.log    出现 m3-secret-value：0
runtime.log    出现 m3-secret-value：0
```

只有工具结果（事件与模型上下文）包含它——这是工具语义本身。

## 7. 这证明了什么

| 项目 | 结果 |
|---|---|
| 目录可注册/被 App 引用 | 是 |
| TaskAssign 携带 MCP 绑定 | 是 |
| Harness 能打开 runtime 会话 | 是 |
| 模型工具清单包含 MCP 工具 | 是（成功调用） |
| 凭据解析只在 runtime | 是 |
| 结果回到 Harness 并进入下一轮 | 是 |
| MCP 不写 tool_invocations | 是 |
| Run 终态 | completed |

## 8. 局限

- 只测了 stdio；SSE / streamable_http 未跑真实链路。
- 使用本地 fake server，不是第三方 MCP。
- 只覆盖成功路径；allowed_tools 拒绝、缺失 secret、runtime 停机属 M4。
- 单 Run；session 并发与回收未压测。

## 9. 证据

```text
docs/reports/artifacts/mcp/M3/
  catalog.json
  app.json
  app.status
  bootstrap.json
  db_refs.txt
  db_run.txt
  harness.log
  runtime.log
  middleware.log
  migrate.log
  submit_run.json
  result.txt
  summary.txt
```

## 10. 下一步

M4 负向：

```text
allowed_tools 之外不可见 / 直呼 TOOL_DENIED
缺失 secret → SECRET_MISSING，Run 仍可完成
runtime 未启动 → MCP_RUNTIME_UNAVAILABLE
stdio 子进程关闭后无泄漏
两个 Run 并发 → session 隔离
```
