<!-- Middleware — retrofit P1 local counter logic. -->

# Retrofit P1：Counter 本地逻辑

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P1 |
| 状态 | 完成 |
| 代码提交 | `1aeb046` |

## 1. 范围

P1 只做本地逻辑，不连接 Middleware：

- Counter 状态与运算。
- local tool dispatcher。
- 工具 manifest。
- 错误结构。
- 与 0 依赖基线对照。

## 2. 新增文件

| 文件 | 内容 |
|---|---|
| `examples/counter_app/src/counter.js` | `Counter` 状态 |
| `examples/counter_app/src/tools.js` | dispatcher、参数解析、结构化错误 |
| `examples/counter_app/src/manifests.js` | 注册与 TaskAssign 两种 manifest 形态 |
| `examples/counter_app/test/counter.test.js` | P1.1 |
| `examples/counter_app/test/tools.test.js` | P1.2–P1.6 |
| `examples/counter_app/test/manifests.test.js` | P1.7 |
| `examples/counter_app/DESIGN_MAPPING.md` | 代码 → DESIGN/proto 依据 |
| `examples/counter_app/README.md` | P1 使用说明 |
| `examples/counter_app/package.json` | 无依赖，`node --test` |
| `Makefile` | `make retrofit-test` |

## 3. 设计依据可定位

每个模块头部注释都标注 Middleware 依据：

| 模块 | 设计依据 |
|---|---|
| `counter.js` | `docs/DESIGN.md` §5.3；§4.3 |
| `tools.js` | §5.3；§4.5；`proto/.../runtime.proto` `ToolCallRequest` / `ToolCallResult` / `ErrorInfo`；`internal/gateway/invoker.go` |
| `manifests.js` | §4.2；§5.3；`ToolManifest`；`internal/httpapi/contracts.go` `ToolDefinition` |

集中映射见 `examples/counter_app/DESIGN_MAPPING.md`。

字段级对应：

```text
local_tool_call.tool_name        → dispatch(toolName, ...)
local_tool_call.arguments_json   → JSON parse
local_tool_result.ok             → dispatch().ok
local_tool_result.result_json    → JSON.stringify({value})
local_tool_result.error.code     → TOOL_NOT_FOUND / TOOL_EXEC_ERROR
ToolManifest.input_schema_json   → JSON.stringify(schema)
ToolDefinition.schema            → schema object
```

## 4. P1 步骤结果

| ID | 逻辑链 | 结果 |
|---|---|---|
| P1.1 | L→L | add(2)=2、add(3)=5、reset=0；非整数拒绝 |
| P1.2 | T→L | `counter_add` 返回 `{"value":2}` |
| P1.3 | T→L | `counter_get` 返回当前值，不改变状态 |
| P1.4 | T→L | `counter_reset` 归零 |
| P1.5 | T→错误 | 未知工具 `TOOL_NOT_FOUND`，retryable=false |
| P1.6 | T→错误 | 非法 JSON/缺 delta/类型错误 → `TOOL_EXEC_ERROR` |
| P1.7 | M→M | 注册 schema 与 TaskAssign `input_schema_json` 一致 |
| P1.8 | L→基线 | standalone 仍 final=5，add/add/get 三次调用 |

测试总计：

```text
tests 9
pass 9
fail 0
```

命令：

```sh
make retrofit-test
```

## 5. 证据

```text
docs/reports/artifacts/retrofit/P1.0-all-tests.log
docs/reports/artifacts/retrofit/P1.1/test.log
docs/reports/artifacts/retrofit/P1.2/test.log
docs/reports/artifacts/retrofit/P1.3/test.log
docs/reports/artifacts/retrofit/P1.4/test.log
docs/reports/artifacts/retrofit/P1.5/test.log
docs/reports/artifacts/retrofit/P1.6/test.log
docs/reports/artifacts/retrofit/P1.7/test.log
docs/reports/artifacts/retrofit/P1.8/standalone.log
```

每个步骤目录均有 `summary.txt`，包含 commit、time、expected、observed、evidence、design。

## 6. P1.8 基线对照

```text
[tool] counter_add({"delta": 2}) -> {"value": 2}
[tool] counter_add({"delta": 3}) -> {"value": 5}
[tool] counter_get({}) -> {"value": 5}
[final] counter=5 steps=4
```

与 P0.1 一致。

## 7. 设计取舍

| 取舍 | 理由 |
|---|---|
| 参数错误使用 `TOOL_EXEC_ERROR` | DESIGN §4.5 工具错误集合不含通用 `BAD_REQUEST`；避免 App 自造 Middleware 错误码 |
| manifest 返回副本 | 防止测试或调用方修改共享 schema |
| 不提前做 `invocation_id` 幂等 | 属于 P6；P1 只验证本地逻辑 |
| 不提前做 WS | 属于 P5；P1 不引入网络状态 |

## 8. 风险

| 项 | 影响 | 处理 |
|---|---|---|
| Node 24 内置 test runner | 低版本 Node 不兼容 | P0 已记录 Node v24.21.0 |
| `counter_reset` 会破坏基线对照 | 若提示词误调用 reset | P6 使用固定提示词并检查调用顺序 |
| dispatcher 当前只覆盖本地错误 | Gateway 错误码尚未验证 | P6 做端到端错误映射 |
| manifest 目前只本地测试 | 未证明注册落库 | P3/P4 验证 |

## 9. 下一步

进入 P2：

- P2.1 配置读取。
- P2.2–P2.6 HTTP 请求构造/响应解析。
- P2.7–P2.10 WS 帧与结果。
