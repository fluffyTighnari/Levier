<!-- Middleware — retrofit P1.1 counter tools and registration metadata. -->

# Retrofit P1.1：抽出 Counter 函数与注册元数据

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P1.1 |
| 状态 | 完成 |
| 代码提交 | `76e35ca` |
| 范围 | 仅本地 Python；不联网、不注册、不改 WS/LLM |

## 1. 做了什么

### 1.1 抽出 `Counter`

从 `main.py` 移到 `examples/counter_agent/tools.py`：

```text
Counter.add(delta) -> int
Counter.get() -> int
Counter.reset() -> int
```

`main.py` 仍负责把返回值包装成工具结果：

```python
return {"value": counter.add(delta)}
```

这样保持原基线输出不变。

### 1.2 明确注册元数据

新增：

```python
TOOL_DEFINITIONS = [
    {
        "name": "counter_add",
        "description": "Add an integer delta to the counter.",
        "schema": {...},
        "timeout_ms": 10000,
    },
    ...
]
```

这正是：

```http
POST /api/v1/apps
Authorization: Bearer <app_api_key>
```

```json
{"name": "...", "multiagent": false, "tools": TOOL_DEFINITIONS}
```

的 `tools[]` 形态。

### 1.3 设计依据

`tools.py` 头部注释：

```text
docs/DESIGN.md §5.3
docs/DESIGN.md §4.2
proto/middleware/runtime/v1/runtime.proto ToolManifest
```

字段对应：

```text
注册 schema               → TaskAssign input_schema_json
local_tool_call.tool_name → TOOL_DEFINITIONS[].name
timeout_ms                → local_tool_call.timeout_ms / 注册默认值
```

## 2. 验证

命令：

```sh
make counter-agent-test
```

结果：

```text
test_p1_1_counter_add_get_reset ... ok
test_p1_1_counter_rejects_non_integer ... ok
test_p1_1_tool_definitions_match_registration_shape ... ok
Ran 3 tests
OK
```

注册元数据序列化：

```text
docs/reports/artifacts/retrofit/P1.1/tool_definitions.json
```

基线回归：

```text
[tool] counter_add({"delta": 2}) -> {"value": 2}
[tool] counter_add({"delta": 3}) -> {"value": 5}
[tool] counter_get({}) -> {"value": 5}
[final] counter=5 steps=4
```

与 P0.1 一致。

## 3. 新增/修改文件

| 文件 | 改动 |
|---|---|
| `examples/counter_agent/tools.py` | 新增 Counter + TOOL_DEFINITIONS |
| `examples/counter_agent/test_tools.py` | 新增 P1.1 单测 |
| `examples/counter_agent/main.py` | 改为 import Counter；包装 `{"value": n}` |
| `Makefile` | 新增 `counter-agent-test` |

## 4. 证据

```text
docs/reports/artifacts/retrofit/P1.1/
  test.log
  tool_definitions.json
  standalone.log
  summary.txt
```

## 5. 当前不覆盖

- P1.2 dispatcher
- P1.3–P1.6 其他工具与错误
- P1.7 manifest 双形态
- P1.8 完整基线对照
- HTTP 注册、DB、WS、LLM

## 6. 下一步

只做 **P1.2：dispatcher counter_add**：

- 从 `local_tool_call` 形态出发：`name + arguments_json`
- 解析参数
- 调用 `Counter.add`
- 返回 `ok/result_json` 或结构化错误
- 单测只覆盖 `counter_add`
