<!-- Middleware — P8R.8 regression and evaluation. -->

# P8R.8：回归与评估

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-12 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P8R.8 |
| 状态 | 完成，P8R 收口 |
| 结论 | PASS |

## 1. 回归矩阵

| 项目 | 命令 | 结果 |
|---|---|---|
| Go 单元/集成 | `MIDDLEWARE_TEST_POSTGRES=1 go test ./...` | 通过 |
| Clean checkout | HEAD worktree 全量测试 | 通过 |
| Harness | `make harness-test` | 82 tests |
| MCP Runtime | `make mcp-runtime-test` | 27 tests |
| Lint | `make lint` | 通过 |
| Build | `make build` | 通过 |
| 依赖规则 | `make deps-check` | 0 违规 |
| 规模 | `make size-report` | 无文件 > 450 行 |
| local 工具 e2e | `make retrofit-e2e` | PASS |
| MCP e2e | `make mcp-e2e` | PASS |
| Daemon 协议 | `go run ./cmd/mw-daemon-check` | PASS |

## 2. 端到端结果

### local 工具

```text
run_id: 01M76WYD2KNNW7BPC3VK9AMPXW
status: completed
tool_calls: begin=3 end=3 invocations=3
final_value: {"value": 5}
events: 16/16/16
metrics: runs_completed=1 counter_add_ok=2 counter_get_ok=1 tool_begin=3
```

### MCP

```text
run_id: 01M76WZ1PMDX9W1K0071TZD0CT
status: completed
mcp_tool: mcp__e2e_echo__echo_token
tool_call_end_ok: 1
result_contains_secret: 1
gateway_invocations: 0
error_events: 0
task_assign_binding: 1
```

### Daemon

```json
{
  "ok": true,
  "checks": {
    "hello": "ok",
    "ack": "accepted",
    "events": "running/completed",
    "completed": "ok",
    "cancel": "daemon-check",
    "cancelled": "ok"
  }
}
```

## 3. 结构与通用性

```text
最大非生成文件：413 行（gateway/executor/http.go）
> 450 行文件：0
长函数：8 个，按 P8R.7 评审保留
依赖违规：0
```

接入面：

```text
App      APP_ONBOARDING + app-ws.schema.json + app_check.py
Daemon   DAEMON_CONTRACT + mw-daemon-check
MCP      MCP.md + make mcp-e2e
```

## 4. 评分卡

```text
functional      pass
clean checkout  pass
e2e (3 paths)   pass
dependencies    pass
size            pass
long functions  accepted (reviewed)
verdict         PASS
```

## 5. 说明

这次回归在 clean checkout 中执行过，避免了“只在本地工作区通过”的问题。

长函数没有为了指标拆分；评审结论记录在 `docs/ARCHITECTURE.md`。

## 6. 证据

```text
docs/reports/artifacts/P8R.8/
  go_test.log
  clean_checkout.log
  harness_test.log
  mcp_runtime_test.log
  lint.log
  build.log
  deps.txt
  size.txt
  retrofit_e2e.log
  retrofit-e2e/      完整运行目录
  mcp_e2e.log
  mcp-e2e/           完整运行目录
  daemon_check.log
  daemon_check_middleware.log
```

## 7. P8R 收口

| 步骤 | 结果 |
|---|---|
| P8R.1 基线护栏 | 完成 |
| P8R.2 契约常量 | 完成 |
| P8R.3 文件拆分 | 完成 |
| P8R.4 错误/事件一致性 | 完成 |
| P8R.5 App 接入套件 | 完成 |
| P8R.6 Daemon 接入套件 | 完成 |
| P8R.7 架构文档与函数评审 | 完成 |
| P8R.8 回归与评估 | 完成 |

## 8. 后续

```text
P9 故障与恢复（已完成主要项）
P10 回归收口（已有 retrofit-e2e / mcp-e2e / daemon-check）
可选：MCP SSE 断线续传、并发/浸泡、pi-core 抽取
```
