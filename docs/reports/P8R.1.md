<!-- Middleware — P8R.1 baseline and guardrails. -->

# P8R.1：基线与护栏

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-12 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P8R.1 |
| 状态 | 完成 |
| 代码提交 | 见下 |

## 1. 新增的两条护栏

### 1.1 依赖检查

`scripts/check_deps.sh` / `make deps-check`：

```text
runstore  不得依赖 control/gateway/httpapi/connection/register/workflow
register  不得依赖 control/gateway/httpapi/connection
connection 不得依赖 control/gateway/httpapi/register/runstore
workflow  不得依赖 control/gateway/httpapi/connection/register
gateway   不得依赖 control/httpapi
httpapi   不得依赖 control
control   不得依赖 httpapi
config    不得依赖任何 internal
contract  不得依赖任何 internal（P8R.2 引入）
```

本次结果：

```text
dependency check: OK (8 packages with rules, 16 packages scanned)
```

### 1.2 规模报告

`scripts/size_report.sh` / `make size-report`：

```text
Go 非测试文件 top 20
Go 文件 > 450 行
Go 函数 > 80 行（go/ast 精确统计）
Python 模块 > 300 行
总文件数 / 总行数
```

## 2. 基线

### 2.1 总量

```text
Go 非测试文件   68 个
Go 行数         12378
Python 源文件   40 个
Python 行数     4981
```

### 2.2 超过 450 行的 Go 文件

| 文件 | 行数 | P8R.3 拆分方向 |
|---|---:|---|
| `internal/control/control.go` | 1006 | schedule / events / lifecycle / tools |
| `internal/runstore/store.go` | 838 | runs / events / apps / sessions |
| `internal/httpapi/server.go` | 836 | runs / apps / sessions / auth |
| `internal/connection/daemonhub.go` | 766 | hub / hello / heartbeat |
| `internal/register/materials.go` | 752 | apps / tools / mcp / materials / secrets |
| `internal/connection/apphub.go` | 584 | 连接 / pending / replay |

### 2.3 超过 80 行的 Go 函数

| 函数 | 行数 | 位置 |
|---|---:|---|
| `main` | 261 | `cmd/middleware/main.go` |
| `handleEvent` | 167 | `internal/control/control.go` |
| `Handle` | 146 | `internal/control/control.go` |
| `Stats` | 121 | `internal/workflow/postgres_reaper.go` |
| `handleWS` | 109 | `internal/httpapi/ws_server.go` |
| `handleCreateRun` | 99 | `internal/httpapi/server.go` |
| `Validate` | 82 | `internal/config/config.go` |

### 2.4 超过 300 行的 Python 模块

```text
harness/src/middleware_harness/llm/anthropic.py   408
mcp_runtime/src/mcp_runtime/sessions.py           393
harness/src/middleware_harness/llm/openai.py      363
harness/src/middleware_harness/config.py          325
harness/src/middleware_harness/strategies/react.py 322
```

## 3. 解读

```text
1. 依赖没有明显倒挂，问题主要是“单文件职责过多”，不是循环依赖。
2. control / httpapi / register / runstore 是第一批拆分对象。
3. main.go 的 261 行是 wiring，拆分优先级低于上面四个。
4. Python 侧最大问题是 provider 与 sessions，暂列 P8R.7 可选项。
```

## 4. 护栏怎么用

```sh
make deps-check     # 每次移动文件/加包后执行
make size-report    # 每个 P8R 步骤结束时对比
```

P8R.3 之后目标：

```text
最大非生成文件 < 450 行
最长函数 < 80 行
```

现在没有强制执行规模阈值，只记录；P8R.8 再决定是否在 CI 里硬失败。

## 5. 证据

```text
docs/reports/artifacts/P8R.1/
  deps.txt   依赖检查输出
  size.txt   文件/函数/模块规模
scripts/check_deps.sh
scripts/size_report.sh
scripts/size_functions.go
```

## 6. 下一步

P8R.2：新增 `internal/contract`，集中错误码、事件名、WS 消息类型，并替换现有字面量。
