<!-- Middleware — retrofit P1.7 tool manifests. -->

# Retrofit P1.7：注册 schema 与 TaskAssign manifest

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P1.7 |
| 状态 | 完成 |
| 代码提交 | `671bc32` |
| 范围 | 只生成两种工具元数据形态；未接 HTTP/WS |

## 1. 两种形态

### 注册形态

用于：

```http
POST /api/v1/apps
```

```json
{
  "tools": [
    {
      "name": "counter_add",
      "description": "...",
      "schema": {"type": "object", "..." : "..."},
      "timeout_ms": 10000
    }
  ]
}
```

代码：

```python
TOOL_DEFINITIONS
```

### TaskAssign 形态

Harness 收到：

```json
{
  "name": "counter_add",
  "description": "...",
  "input_schema_json": "{\"type\":\"object\",...}"
}
```

代码：

```python
tool_manifests()
```

转换：

```text
TOOL_DEFINITIONS[].schema
  → json.dumps
  → ToolManifest.input_schema_json
```

## 2. 设计依据

```text
DESIGN §4.2  POST /api/v1/apps tools[].schema
DESIGN §5.3  local tools
proto/.../runtime.proto ToolManifest
internal/httpapi/contracts.go ToolDefinition
```

字段对应：

| 注册 | TaskAssign |
|---|---|
| `name` | `name` |
| `description` | `description` |
| `schema`（对象） | `input_schema_json`（字符串） |
| `timeout_ms` | 不直接携带 |
| 不传 scope | Middleware 标记 `LOCAL` |

## 3. 验证

```sh
cd examples/counter_agent
python3 -m unittest -v test_manifests
```

```text
test_p1_7_manifest_dual_shape ... ok
test_p1_7_manifest_is_isolated ... ok
Ran 2 tests
OK
```

断言：

- 三个 manifest 与三个注册定义一一对应。
- `input_schema_json` `json.loads` 后等于原 `schema`。
- 注册定义与 manifest 都可 JSON 序列化。
- manifest 不包含 `timeout_ms`。
- 修改返回的 manifest 不影响下一次生成。

全量：

```sh
make counter-agent-test
```

```text
tests 10
OK
```

## 4. 证据

```text
docs/reports/artifacts/retrofit/P1.7/test.log
docs/reports/artifacts/retrofit/P1.7/manifests.json
docs/reports/artifacts/retrofit/P1.7/summary.txt
```

`manifests.json` 同时包含：

```text
registration.tools
task_assign.tools
```

便于人工对照。

## 5. P1 当前进度

| 步骤 | 状态 |
|---|---|
| P1.1 Counter + 注册元数据 | 完成 |
| P1.2 dispatcher add | 完成 |
| P1.3 dispatcher get | 完成 |
| P1.4 dispatcher reset | 完成 |
| P1.5 unknown tool | 完成 |
| P1.6 invalid args | 完成 |
| P1.7 manifest 双形态 | 完成 |
| P1.8 main 接 dispatcher + 基线 | 待做 |

## 6. 下一步

P1.8：

- 把 `main.py` 的 `execute_tool` 接到 `dispatch_counter_tool`。
- 用 `tool_manifests()`/`TOOL_DEFINITIONS` 统一工具元数据，去掉重复定义。
- 跑同一提示词，要求 final=5、add/add/get。
