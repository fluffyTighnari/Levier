<!-- Middleware — MCP M6 one-click regression and closure. -->

# MCP M6：一键回归与收口

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-12 |
| 状态 | 完成，M1–M6 收口 |
| 命令 | `make mcp-e2e` |
| 样本 Run | `01M76PRD7KZ22J5PDK6Z8A4Y35` |

## 1. 一键脚本

`scripts/mcp_e2e.sh`：

```text
preflight   检查 env / psql / python / 两端 venv / fixture
start       Middleware → mcp_runtime → Harness
register    POST /admin/mcp（stdio + ${secret:e2e_token}）
            bootstrap App → POST /api/v1/apps {mcp_refs}
run         真实 DeepSeek Run，提示词点名 mcp__e2e_echo__echo_token
wait        runs.status 到终态
validate    TOOL_CALL_END / 结果含 secret / 0 invocation / 0 ERROR
            TaskAssign 日志含 1 个 binding / runtime 打开 session
evidence    docs/reports/artifacts/mcp/M6/run-<UTC>/
cleanup     杀 Harness/runtime/MW，删除凭据与 socket
```

脚本使用仓库内的 stdio fixture，不依赖 npm，保证可重复。

## 2. 本次结果

```text
run_id: 01M76PRD7KZ22J5PDK6Z8A4Y35
status: completed
mcp_tool: mcp__e2e_echo__echo_token
tool_call_end_ok: 1
result_contains_secret: 1
gateway_invocations: 0
error_events: 0
final_message_hit: 1
task_assign_binding: 1

mcp-e2e: PASS
```

## 3. 断言含义

| 断言 | 证明 |
|---|---|
| status=completed | Run 正常结束 |
| `TOOL_CALL_END ok=1` | 模型真实调用了 MCP 工具 |
| 结果含 `mcp-e2e-secret` | runtime 解析了占位符，子进程拿到值 |
| gateway_invocations=0 | MCP 未经过 Gateway |
| error_events=0 | 没有隐藏失败 |
| final_message_hit=1 | 结果进入下一轮模型输入 |
| task_assign_binding=1 | 目录经过调度到达 Harness |
| runtime session opened | 连接确由 runtime 建立 |

## 4. 文档

新增 `docs/MCP.md`：

```text
边界
注册 API
App 引用
runtime 启动与凭据
工具命名与 allowlist
失败语义
安全
make mcp-e2e
当前限制
```

## 5. M1–M6 收口

| 步骤 | 内容 | 状态 |
|---|---|---|
| M1 | runtime 解析 `${secret:...}` | 完成 |
| M2 | 目录 API + 绑定保留占位符 | 完成 |
| M3 | 目录 → TaskAssign → runtime 端到端 | 完成 |
| M4 | allowlist / 缺失 secret / 并发 / 停机 | 完成 |
| M5 | 第三方 filesystem server | 完成 |
| M6 | 一键回归 + 文档 | 完成 |

## 6. 当前限制

```text
真实链路只验证 stdio
SSE / streamable_http 只有 fake 单测
MCP 不写 tool_invocations，无 Gateway 幂等/配额
MW 不托管 runtime 生命周期
session 按 Run 建立，无跨 Run 池化
第三方 server 需要外部安装
```

## 7. 证据

```text
docs/reports/artifacts/mcp/M6/
  mcp_e2e.log
  run-20261112T014259Z/
    catalog_request.json
    catalog.json
    app_request.json
    app.json
    bootstrap.json
    bootstrap.err
    submit_run.json
    submit_run.err
    middleware.log
    runtime.log
    harness.log
    migrate.log
    summary.txt
```

## 8. 后续

```text
SSE / streamable_http 真实链路
跨 Run 的 runtime 连接池（按需）
MCP 审计/配额（若需要，单独设计，不默认进 Gateway）
pi-core 抽取（另行评估）
```
