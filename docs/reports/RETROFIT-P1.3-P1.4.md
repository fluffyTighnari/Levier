<!-- Middleware — retrofit P1.3/P1.4 counter get and reset. -->

# Retrofit P1.3–P1.4：dispatcher counter_get / counter_reset

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P1.3–P1.4 |
| 状态 | 完成 |
| 代码提交 | `e172bdc` |
| 范围 | 只加 get/reset 成功路径；未接 main/HTTP/WS |

## 1. 做了什么

`dispatch_counter_tool` 现在支持三个成功路径：

```text
counter_add(delta)  → {"value": counter + delta}
counter_get()       → {"value": counter}
counter_reset()     → {"value": 0}
```

实现方式：

```python
if tool_name == "counter_get":
    return _success(counter.get())
if tool_name == "counter_reset":
    return _success(counter.reset())
```

`main.py` 仍然没有接 dispatcher；这是故意的，P1.8 才接线。

## 2. 设计依据

```text
DESIGN §5.3  local tools executed by the App
DESIGN §4.5  tool error codes
proto ToolCallResult.result_json
```

字段对应：

```text
local_tool_call.tool_name       → tool_name
local_tool_call.arguments_json  → arguments_json
local_tool_result.ok            → ok
local_tool_result.result_json   → result_json
```

## 3. 验证

### P1.3

```sh
cd examples/counter_agent
python3 -m unittest -v test_dispatch.DispatcherTest.test_p1_3_dispatcher_counter_get
```

```text
test_p1_3_dispatcher_counter_get ... ok
Ran 1 test
OK
```

断言：

```text
Counter(7) + counter_get
→ result_json = {"value":7}
→ counter 仍为 7
```

### P1.4

```sh
python3 -m unittest -v test_dispatch.DispatcherTest.test_p1_4_dispatcher_counter_reset
```

```text
test_p1_4_dispatcher_counter_reset ... ok
Ran 1 test
OK
```

断言：

```text
Counter(9) + counter_reset
→ result_json = {"value":0}
→ counter 变为 0
```

## 4. 全量本地测试

```sh
make counter-agent-test
```

```text
tests 6
OK
```

覆盖 P1.1 + P1.2 + P1.3 + P1.4。

## 5. 证据

```text
docs/reports/artifacts/retrofit/P1.3/test.log
docs/reports/artifacts/retrofit/P1.3/summary.txt
docs/reports/artifacts/retrofit/P1.4/test.log
docs/reports/artifacts/retrofit/P1.4/summary.txt
```

## 6. P1 当前进度

| 步骤 | 状态 |
|---|---|
| P1.1 Counter + 注册元数据 | 完成 |
| P1.2 dispatcher add | 完成 |
| P1.3 dispatcher get | 完成 |
| P1.4 dispatcher reset | 完成 |
| P1.5 unknown tool | 待做 |
| P1.6 invalid args | 待做 |
| P1.7 manifest 双形态 | 待做 |
| P1.8 main 接 dispatcher + 基线 | 待做 |

## 7. 下一步

P1.5：正式测试未知工具，只验证：

```text
dispatch_counter_tool(counter, "counter_missing", "{}")
→ ok=false
→ error.code=TOOL_NOT_FOUND
→ retryable=false
```
