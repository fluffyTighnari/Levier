<!-- Middleware — retrofit P6A.2 SSE streaming. -->

# Retrofit P6A.2：SSE 流式解析

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P6A.2 |
| 状态 | 完成 |
| 代码提交 | `77279b2` |
| 范围 | Harness 内部；fake transport |

## 1. 新增能力

`OpenAICompatibleProvider` 现在支持：

```python
OpenAICompatibleProvider(stream=True)   # 默认
```

请求：

```json
{
  "model": "deepseek-flash",
  "stream": true,
  "messages": [...],
  "tools": [...]
}
```

响应按 `Content-Type` 分流：

```text
application/json        → _parse_json_response
text/event-stream      → _parse_stream
```

## 2. SSE 解析

支持：

```text
data: {"choices":[{"delta":{"content":"Hel"}}]}
data: {"choices":[{"delta":{"content":"lo"}}]}
data: {"choices":[{"delta":{"tool_calls":[{"index":0,...}]}}]}
data: {"usage":{...}}
data: [DONE]
```

关键：

- `delta.content` 逐段回调 `on_delta`。
- `delta.tool_calls` 按 `index` 累积。
- `id` / `name` 可能只在第一片出现。
- `arguments` 可能被拆成多片，直接字符串拼接，不提前 JSON 解析。
- 最后一片可能带 `usage`。
- `[DONE]` 结束。
- SSE 字节流可能被 TCP 拆成任意 chunk，解析器按行缓冲。

## 3. 实际测试

```text
test_p6a_1_json_text_and_tool_call ... ok
test_p6a_1_message_conversion ... ok
test_p6a_1_plain_text_response ... ok
test_p6a_2_sse_text_and_tool_call ... ok
Ran 4 tests
OK
```

P6A.2 断言：

```text
deltas = ["Hel", "lo"]
tool_calls[0].call_id = call_1
tool_calls[0].name = counter_add
tool_calls[0].arguments_json = {"delta":2}
usage = prompt 3 / completion 4 / total 7
request.stream = true
```

测试把 SSE body 按 7 字节切片，验证跨 chunk 缓冲。

全量 Harness：

```text
make harness-test
Ran 74 tests
OK
```

## 4. 非流式 vs 流式：消息函数链路

两者最终都产出同一个：

```python
LLMResponse(content, tool_calls, usage, model)
```

区别只在 provider 内部和 MESSAGE_DELTA。

### 4.1 非流式

```text
ReactStrategy.run
  → _complete
      → provider.complete
          → build_request(stream=false)
          → HTTP JSON
          → _parse_json_response
          → LLMResponse
  → _assistant_message
  → 有 tool_calls ? tools.call : return
  → _tool_message
  → 下一轮
```

特点：

- 模型返回完整 JSON 后才有 `LLMResponse`。
- 不产生 `MESSAGE_DELTA`。
- `on_delta` 不会被调用。
- `MESSAGE_COMPLETED` 一次性发出。

### 4.2 流式

```text
ReactStrategy.run
  → _complete
      → provider.complete(on_delta)
          → build_request(stream=true)
          → HTTP SSE
          → _parse_stream
              → delta content → on_delta → MESSAGE_DELTA(ephemeral)
              → delta tool_calls → 按 index 累积
              → LLMResponse
  → _assistant_message
  → 有 tool_calls ? tools.call : return
  → _tool_message
  → 下一轮
```

特点：

- 文本增量通过 `MESSAGE_DELTA` 实时发出。
- `MESSAGE_DELTA` 是 ephemeral，`seq=0`，不落库。
- 流结束后仍产生同一个 `LLMResponse`。
- `MESSAGE_COMPLETED` 在流结束后发出，包含完整内容。
- 工具调用逻辑与非流式完全一致。

### 4.3 消息链

```text
Message[] (system/user/assistant/tool)
  → provider 转换
      → OpenAI messages
  → HTTP
  → LLMResponse
  → _assistant_message
      → assistant Message(tool_calls)
  → tools.call
  → _tool_message
      → role=tool, tool_call_id
  → 下一轮 Message[]
```

流式/非流式共享这套消息链，只是 provider 的传输与解析不同。

## 5. 设计依据

```text
DeepSeek /chat/completions SSE
harness/src/middleware_harness/llm/base.py
harness/src/middleware_harness/strategies/react.py
```

## 6. 当前不覆盖

- 错误映射正式测试（P6A.3）
- provider 路由（P6A.4）
- DEEPSEEK_API_KEY 读取（P6A.5）
- 真实 DeepSeek
- Middleware/App/WS

## 7. 证据

```text
docs/reports/artifacts/retrofit/P6A.2/
  test.log
  summary.txt
```

## 8. 下一步

P6A.3：401/429/5xx/超时的 `LLMError` 映射。

```text
401 → AUTH_FAILED
429 → RATE_LIMITED
5xx → PROVIDER_UNAVAILABLE
timeout → TIMEOUT
```
