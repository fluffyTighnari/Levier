<!-- Middleware — P8R.2 contract constants. -->

# P8R.2：契约单一来源

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-12 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P8R.2 |
| 状态 | 完成 |
| 代码提交 | 见下 |

## 1. 结构调整

### 1.1 错误码：沿用 `internal/errs`

`errs` 已经是错误码 + 可重试性 + HTTP 映射的既有模型，没有再新建一套。

本次补齐：

```text
BAD_REQUEST / UNAUTHORIZED / FORBIDDEN / NOT_IMPLEMENTED
DEPENDENCY_UNAVAILABLE
APP_NOT_FOUND / APP_NAME_CONFLICT
RUN_NOT_FOUND / SESSION_NOT_FOUND / IDEMPOTENCY_CONFLICT
BAD_TOOL / BAD_MCP / BAD_MCP_REF / MCP_NOT_FOUND / MCP_IN_USE
BAD_MESSAGE / REPLAY_FAILED / RESPONSE_TOO_LARGE
TOOL_NOT_AVAILABLE / TOOL_BUSY
LOCAL_TOOL_UNKNOWN / LOCAL_TOOL_FORBIDDEN / LOCAL_TOOL_CORRELATION_MISMATCH
```

每个都补了：

```text
retryable 默认值
HTTP status 映射
```

### 1.2 WS 常量：新增 `internal/contract`

`internal/contract/ws.go`：

```text
WSProtocolVersion = 1
hello / subscribe / local_tool_result / ping / event
local_tool_call / run_completed / pong / caught_up / error
```

- `contract` 不依赖任何 internal 包
- `httpapi` 的常量块已删除，全部改用 `contract.*`
- 测试也改用 contract，不再保留第二份定义

### 1.3 事件名不需要新常量

Go 生产代码里没有事件名字符串字面量（使用 `runtimev1.EventType` 枚举与 `EventType_name`）。
P8R.4 再处理事件 payload 字段一致性。

## 2. 字面量替换

替换为 `string(errs.CodeX)`：

```text
共 129 处
BAD_REQUEST                24
TOOL_EXEC_ERROR            20
INTERNAL                   16
NOT_IMPLEMENTED             9
RUN_NOT_FOUND               8
UNAUTHORIZED                6
TOOL_NOT_FOUND              6
...
```

涉及文件：

```text
internal/httpapi/server.go
internal/httpapi/ws_server.go
internal/httpapi/mcp_admin.go
internal/gateway/invoker.go
internal/gateway/invocation_store.go
internal/gateway/executor/http.go
internal/gateway/executor/webhook.go
internal/control/control.go
internal/connection/apphub.go
```

## 3. 依赖与测试

```text
dependency check: OK (9 packages with rules, 17 packages scanned)
go test ./...          全部通过
make lint              OK
make build             OK
```

新增测试：

```text
internal/contract/ws_test.go
  WS 消息字符串与协议版本固定，防止悄悄改协议
```

## 4. 剩余

```text
RUN_TIMEOUT 仍以字面量出现在 workflow.TaskType
  这是任务类型，不是错误码，本次不并入 errs
事件 payload 字段仍由 structpb/map 传递
  由 P8R.4 统一
```

## 5. 证据

```text
docs/reports/artifacts/P8R.2/
  go_test.log
  lint.log
  build.log
  deps.txt
```

## 6. 下一步

P8R.3：拆分 `control` / `httpapi` / `register` / `runstore` 四个大文件（纯移动，不改行为）。
