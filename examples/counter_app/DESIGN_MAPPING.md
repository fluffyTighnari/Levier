<!-- Middleware — P1 design traceability. -->

# Counter App 设计依据映射

本文件用于回答：**P1 的每一块代码对应 Middleware 的哪个设计/契约？**

## 1. 模块映射

| 模块 | 设计依据 | 说明 |
|---|---|---|
| `src/counter.js` | `docs/DESIGN.md` §5.3；§4.3 | Counter 是 App-owned 状态；local tool 由 App 执行，Gateway 只负责路由 |
| `src/tools.js` | `docs/DESIGN.md` §5.3；§4.5；`proto/.../runtime.proto` `ToolCallRequest`/`ToolCallResult`/`ErrorInfo`；`internal/gateway/invoker.go` | 输入是 `tool_name + arguments_json`，输出是 `ok + result_json` 或 `error{code,message,retryable}` |
| `src/manifests.js` | `docs/DESIGN.md` §4.2；§5.3；`proto/.../runtime.proto` `ToolManifest`；`internal/httpapi/contracts.go` `ToolDefinition` | 注册用 `schema` 对象；TaskAssign 用 `input_schema_json` 字符串 |
| `test/*.test.js` | 对应上述契约的边界 | 每个测试名以 P1.x 编号，与 `docs/APP_RETROFIT_PLAN.md` 对齐 |

## 2. 字段级映射

| Middleware 契约 | App 代码 |
|---|---|
| `local_tool_call.tool_name` | `dispatch(toolName, ...)` 的 `toolName` |
| `local_tool_call.arguments_json` | `dispatch(..., argumentsJson)`；JSON 解析为对象 |
| `local_tool_result.ok` | `dispatch` 返回的 `ok` |
| `local_tool_result.result_json` | 成功时 `JSON.stringify({value})` |
| `local_tool_result.error.code` | `TOOL_NOT_FOUND` / `TOOL_EXEC_ERROR` |
| `local_tool_result.error.message` | 可读错误信息 |
| `local_tool_result.error.retryable` | 当前均为 `false` |
| `ToolManifest.name` | `manifests.js` 的 `name` |
| `ToolManifest.description` | `manifests.js` 的 `description` |
| `ToolManifest.input_schema_json` | `JSON.stringify(schema)` |
| `ToolDefinition.schema` | `manifests.js` 的 `schema` 对象 |

## 3. 错误码依据

`docs/DESIGN.md` §4.5 的工具错误集合：

```text
TOOL_NOT_FOUND
TOOL_TIMEOUT
TOOL_DENIED
TOOL_EXEC_ERROR
APP_UNREACHABLE
```

P1 只产生 App 侧错误：

- 未知工具 → `TOOL_NOT_FOUND`
- 参数非法/执行异常 → `TOOL_EXEC_ERROR`

`TOOL_TIMEOUT`、`TOOL_DENIED`、`APP_UNREACHABLE` 由 Middleware/Gateway/AppHub 产生，不在本地 dispatcher 中伪造。

## 4. 当前不覆盖

P1 只做本地逻辑，不涉及：

- App 注册与 `apps/tool_defs` 落库（P3）
- TaskAssign 工具清单（P4）
- WS `local_tool_call/local_tool_result` 传输（P5/P6）
- `invocation_id` 幂等（P6）
- 超时、ACL、断线（P6/P9）

这些步骤不得在 P1 中提前实现或测试。

## 5. 验收路径

```text
P1.1 L→L
P1.2 T→L (add)
P1.3 T→L (get)
P1.4 T→L (reset)
P1.5 T→错误 (unknown)
P1.6 T→错误 (bad args)
P1.7 M→M (registration + TaskAssign shapes)
P1.8 L→基线 (standalone output unchanged)
```
