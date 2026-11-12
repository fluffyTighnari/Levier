<!-- Middleware — P8R.3 file splits. -->

# P8R.3：拆分大文件

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-12 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P8R.3 |
| 状态 | 完成 |
| 代码提交 | 见下 |

## 1. 方法

纯移动，同包拆分，不改签名、不改逻辑。

新增一次性工具 `scripts/split_decls/`：

```text
go run ./scripts/split_decls -src a.go -map map.json
按 ast.Decl 把声明移动到目标文件，自动带上 doc 注释和 import 块
构建失败时自动删除未使用的 import
```

## 2. 拆分结果

### control

| 文件 | 行数 | 职责 |
|---|---:|---|
| `control.go` | 173 | 类型、接口、New/Start/Close |
| `schedule.go` | 309 | Handle / 分配 / TaskAssign 组装 |
| `events.go` | 259 | 事件循环、投影、计数器 |
| `lifecycle.go` | 311 | 取消、超时、requeue、reconcile |
| `tools.go` | 106 | ToolCall 分发 |

原 `control.go` 1006 行。

### httpapi

| 文件 | 行数 |
|---|---:|
| `server.go` | 258 |
| `runs.go` | 228 |
| `apps.go` | 169 |
| `auth.go` | 106 |
| `sessions.go` | 115 |
| `ws_token.go` | 27 |

原 `server.go` 836 行。

### register

| 文件 | 行数 |
|---|---:|
| `materials.go` | 132 |
| `mcp.go` | 284 |
| `tools.go` | 285 |
| `secrets.go` | 92 |

原 `materials.go` 752 行。

### runstore

| 文件 | 行数 |
|---|---:|
| `store.go` | 38 |
| `runs.go` | 366 |
| `queries.go` | 102 |
| `events.go` | 205 |
| `apps.go` | 82 |
| `sessions.go` | 110 |

原 `store.go` 838 行。

### connection（追加拆分）

| 文件 | 行数 |
|---|---:|
| `daemonhub.go` | 305 |
| `hello.go` | 165 |
| `send.go` | 143 |
| `placement.go` | 191 |
| `apphub.go` | 164 |
| `appconn.go` | 206 |
| `localtool.go` | 234 |

原 `daemonhub.go` 766、`apphub.go` 584。

## 3. 结果

```text
> 450 行的 Go 非测试文件：0
当前最大：internal/gateway/executor/http.go 413 行
```

函数长度仍有一批超过 80 行：

```text
main               261
handleEvent        167
Handle             146
Stats              121
handleWS           109
handleCreateRun     99
Validate            82
```

这些不是“纯移动”能解决的，放到 P8R.7 做行为保持的函数抽取。

## 4. 拆分时发现的隐藏问题

`runstore.Store.ListActiveRuns` 从未进入 HEAD：

```text
clean worktree 下 control 集成测试无法编译
working tree 里有一份未提交实现
拆分前恢复 store.go 时暴露了这个问题
```

已补回：

```go
func (s *Store) ListActiveRuns(ctx) ([]Run, error)
```

并新增集成测试 `TestListActiveRuns`，防止再次丢失。

教训：P8R.8 应加一次 **clean checkout 全量测试**，避免“只在本地工作区通过”。

## 5. 验证

```text
go test ./...          全部通过
make lint              OK
make build             OK
dependency check       OK (9 packages with rules, 17 packages scanned)
size report            无文件 > 450 行
```

## 6. 证据

```text
docs/reports/artifacts/P8R.3/
  go_test.log
  lint.log
  build.log
  deps.txt
  size.txt
```

## 7. 下一步

P8R.4：错误与事件一致性（统一映射、payload 字段、删除重复转换）。
