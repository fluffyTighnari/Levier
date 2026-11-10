<!-- Middleware — retrofit P1.2 counter_add dispatcher. -->

# Retrofit P1.2：dispatcher counter_add

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P1.2 |
| 状态 | 完成 |
| 代码提交 | `7d8e852` |
| 范围 | 仅 `counter_add`；未接入 main/WS/HTTP |

## 1. 代码

新增：

```text
examples/counter_agent/dispatch.py
```

入口：

```python
dispatch_counter_tool(counter, tool_name, arguments_json)
```

P1.2 只处理：

```text
tool_name = counter_add
```

流程：

```text
arguments_json
  → JSON 解析为对象
  → delta 必须为 int
  → Counter.add(delta)
  → {"ok": true, "result_json": "{\"value\":2}"}
```

错误结构：

```python
{
  "ok": False,
  "error": {
    "code": "TOOL_EXEC_ERROR",
    "message": "...",
    "retryable": False,
  },
}
```

未知工具当前返回 `TOOL_NOT_FOUND`，但其正式测试属于 P1.5。

## 2. 设计依据

`dispatch.py` 头部注释：

```text
docs/DESIGN.md §5.3
docs/DESIGN.md §4.5
proto/.../runtime.proto ToolCallResult / ErrorInfo
```

字段对应：

```text
local_tool_call.tool_name       → tool_name
local_tool_call.arguments_json  → arguments_json
local_tool_result.ok            → ok
local_tool_result.result_json   → result_json
local_tool_result.error.*       → error.code/message/retryable
```

WS 层后续只负责补：

```text
correlation_id
invocation_id
```

## 3. 验证

```sh
cd examples/counter_agent
python3 -m unittest -v test_dispatch.DispatcherTest.test_p1_2_dispatcher_counter_add
```

结果：

```text
test_p1_2_dispatcher_counter_add ... ok
Ran 1 test
OK
```

断言：

```text
result["ok"] == True
result["result_json"] == '{"value": 2}'
counter.get() == 2
```

## 4. 证据

```text
docs/reports/artifacts/retrofit/P1.2/
  test.log
  summary.txt
```

## 5. 当前不覆盖

- `counter_get`
- `counter_reset`
- 未知工具正式测试
- 非法 JSON/缺 delta 正式测试
- main.py 接线
- HTTP/WS/LLM

## 6. 下一步

P1.3：dispatcher `counter_get`。

只增加 get 分支和一条单测，不改 main/WS。
