<!-- Middleware — retrofit P1.8 main wiring and P1 closeout. -->

# Retrofit P1.8：main 接 dispatcher，P1 收口

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P1.8 |
| 状态 | 完成，P1 收口 |
| 代码提交 | `4c9eb11` |
| 范围 | 只改 standalone 入口接线；未接 MW |

## 1. main.py 改动

### 工具定义统一

之前 `main.py` 又手写了一份 `TOOLS`，和 `tools.py` 的 `TOOL_DEFINITIONS` 重复。

现在：

```python
def _openai_tools():
    return [
        {
            "type": "function",
            "function": {
                "name": tool["name"],
                "description": tool["description"],
                "parameters": tool["schema"],
            },
        }
        for tool in TOOL_DEFINITIONS
    ]
```

`TOOLS = _openai_tools()`。

这样：

```text
注册元数据 TOOL_DEFINITIONS
  → OpenAI/DeepSeek 工具定义
  → TaskAssign manifest (tool_manifests)
```

只有一个来源。

### execute_tool 接 dispatcher

之前：

```python
if name == "counter_add":
    return {"value": counter.add(...)}
```

现在：

```python
result = dispatch_counter_tool(counter, name, arguments_json)
if result["ok"]:
    return json.loads(result["result_json"])
return {"error": ..., "code": ..., "retryable": ...}
```

模型循环保持不变：

```text
DeepSeek → tool_calls → execute_tool → dispatcher → Counter
```

P1 的目标达成：工具执行已经收敛到 `dispatch_counter_tool`，但 standalone 行为不变。

## 2. 验证

单元测试：

```text
test_p1_8_execute_tool_delegates_to_dispatcher ... ok
test_p1_8_main_tools_match_registration ... ok
Ran 2 tests
OK
```

- `TOOLS` 与 `TOOL_DEFINITIONS` 名称/描述/schema 一致。
- `execute_tool` 成功时返回 `{"value":2}`。
- 未知工具返回 `TOOL_NOT_FOUND`。

全量：

```text
make counter-agent-test
tests 12
OK
```

基线：

```text
[tool] counter_add({"delta": 2}) -> {"value": 2}
[tool] counter_add({"delta": 3}) -> {"value": 5}
[tool] counter_get({}) -> {"value": 5}
[final] counter=5 steps=4
```

与 P0.1 一致。

## 3. 证据

```text
docs/reports/artifacts/retrofit/P1.8/test.log
docs/reports/artifacts/retrofit/P1.8/standalone.log
docs/reports/artifacts/retrofit/P1.8/summary.txt
```

## 4. P1 总收口

| 步骤 | 结果 |
|---|---|
| P1.1 Counter + 注册元数据 | 完成 |
| P1.2 dispatcher add | 完成 |
| P1.3 dispatcher get | 完成 |
| P1.4 dispatcher reset | 完成 |
| P1.5 unknown tool | 完成 |
| P1.6 invalid args | 完成 |
| P1.7 manifest 双形态 | 完成 |
| P1.8 main 接线 + 基线 | 完成 |

P1 完成后的结构：

```text
examples/counter_agent/
  main.py          模型循环 + 提交/打印
  tools.py         Counter + TOOL_DEFINITIONS
  dispatch.py      local tool 执行入口
  test_*.py        12 个本地测试
```

P1 完成后的能力：

- 工具函数独立。
- 注册元数据独立。
- 执行入口输入/输出与 WS 契约一致。
- 模型循环仍可运行，基线不回归。

P1 **没有**：

- App HTTP 注册。
- DB 落库。
- WebSocket 连接。
- 真实 Harness 调用。
- `invocation_id` 幂等。

这些属于 P3–P6。

## 5. 设计依据

```text
DESIGN §5.3  local tools executed by App
DESIGN §4.2  tools[].schema
DESIGN §4.5  ErrorInfo
proto ToolManifest / ToolCallResult
```

## 6. 下一步

P2.1：只做配置读取。

```text
MW_URL
MW_ADMIN_TOKEN
MW_APP_NAME
```

要求：

- 缺失时明确报错。
- 不打印值。
- 不发起任何网络请求。
