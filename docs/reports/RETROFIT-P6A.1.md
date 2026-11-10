<!-- Middleware — retrofit P6A.1 OpenAI-compatible JSON parsing. -->

# Retrofit P6A.1：OpenAI-compatible 非流式解析

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P6A.1 |
| 状态 | 完成 |
| 代码提交 | `ad9b1e9` |
| 范围 | Harness 内部；fake transport；无真实 DeepSeek |

## 1. 新增

```text
harness/src/middleware_harness/llm/openai.py
  OpenAICompatibleProvider
```

能力：

```text
POST {base_url}/chat/completions
stream = false
解析 choices[0].message
```

## 2. 请求映射

Provider-neutral Message → OpenAI messages：

| 内部 | OpenAI |
|---|---|
| `role=system` | `{"role":"system","content":...}` |
| `role=user` | `{"role":"user","content":...}` |
| assistant + tool_calls | `{"role":"assistant","tool_calls":[...]}` |
| `role=tool` | `{"role":"tool","tool_call_id":"...","content":"..."}` |

ToolSpec：

```json
{
  "type": "function",
  "function": {
    "name": "counter_add",
    "description": "...",
    "parameters": {"type": "object", ...}
  }
}
```

请求头：

```text
Authorization: Bearer <model.api_key>
Content-Type: application/json
```

URL：

```text
{model.base_url or https://api.deepseek.com}/chat/completions
```

## 3. 响应映射

输入：

```json
{
  "model": "deepseek-flash",
  "choices": [
    {
      "message": {
        "content": "I will add.",
        "tool_calls": [
          {
            "id": "call_1",
            "type": "function",
            "function": {
              "name": "counter_add",
              "arguments": "{\"delta\":2}"
            }
          }
        ]
      }
    }
  ],
  "usage": {
    "prompt_tokens": 11,
    "completion_tokens": 7,
    "total_tokens": 18
  }
}
```

输出：

```python
LLMResponse(
    content="I will add.",
    tool_calls=[
        ToolCall(call_id="call_1", name="counter_add", arguments_json='{"delta":2}')
    ],
    usage={"prompt_tokens":11,"completion_tokens":7,"total_tokens":18},
    model="deepseek-flash",
)
```

## 4. 测试

```text
test_p6a_1_json_text_and_tool_call ... ok
test_p6a_1_message_conversion ... ok
test_p6a_1_plain_text_response ... ok
Ran 3 tests
OK
```

断言：

- 文本 content 正确。
- tool_calls 的 id/name/arguments 正确。
- usage 正确。
- 请求 URL/headers/body 正确。
- tool schema 转换正确。
- assistant tool_calls / tool message 转换正确。
- plain text 无工具。

全量 Harness：

```text
make harness-test
Ran 73 tests
OK
```

## 5. 设计依据

```text
docs/DESIGN.md §5.3 工具调用
harness/src/middleware_harness/llm/base.py
LLMProvider / LLMResponse / ToolCall / ToolSpec
DeepSeek /chat/completions 契约
```

## 6. 当前不覆盖

- SSE 流式解析（P6A.2）
- 错误映射正式测试（P6A.3）
- provider 路由（P6A.4）
- DEEPSEEK_API_KEY 读取（P6A.5）
- 真实 DeepSeek 调用
- Middleware/App/WS

## 7. 证据

```text
docs/reports/artifacts/retrofit/P6A.1/
  test.log
  summary.txt
```

## 8. 下一步

P6A.2：SSE 流式解析。

重点：

```text
data: {"choices":[{"delta":{"content":"..."}}]}
data: {"choices":[{"delta":{"tool_calls":[{"index":0,...}]}}]}
data: [DONE]
```

需要按 index 累积 tool_call arguments。
