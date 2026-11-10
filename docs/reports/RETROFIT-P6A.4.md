<!-- Middleware — retrofit P6A.4 provider routing. -->

# Retrofit P6A.4：Provider 路由

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P6A.4 |
| 状态 | 完成 |
| 代码提交 | `795fe03` |
| 范围 | Harness provider 选择；无真实 API |

## 1. 新增

```text
harness/src/middleware_harness/llm/router.py
  ProviderRouter
```

路由规则：

| `model.provider` | Provider |
|---|---|
| `anthropic` | `AnthropicProvider` |
| `deepseek` | `OpenAICompatibleProvider` |
| `openai` | `OpenAICompatibleProvider` |
| 空字符串 | `AnthropicProvider`（兼容旧行为） |
| 其他 | `LLMError(BAD_REQUEST)` |

## 2. 接线

之前：

```python
ReactStrategy(AnthropicProvider(), tools)
```

现在：

```python
ReactStrategy(ProviderRouter(), tools)
```

`ReactStrategy` 本身不需要知道 provider 类型。

## 3. 请求链路

```text
ReactStrategy
  → _complete
  → ProviderRouter.complete(messages, tools, model, on_delta)
      → model.provider == "deepseek"
      → OpenAICompatibleProvider.complete
          → stream=true
          → SSE/JSON
          → LLMResponse
```

Anthropic：

```text
model.provider == "anthropic"
  → AnthropicProvider.complete
  → 原行为不变
```

## 4. 验证

```text
test_p6a_4_rejects_unknown_provider ... ok
test_p6a_4_routes_known_providers ... ok
Ran 2 tests
OK
```

覆盖：

- anthropic
- deepseek
- openai
- 空 provider → anthropic
- unknown → BAD_REQUEST

全量：

```text
make harness-test
Ran 79 tests
OK
```

## 5. 设计依据

```text
harness LLMProvider 协议
ReactStrategy provider 注入
ModelConfig.provider
```

## 6. 当前不覆盖

- DEEPSEEK_API_KEY 读取（P6A.5）
- 真实 DeepSeek 调用
- Middleware/App/WS

## 7. 证据

```text
docs/reports/artifacts/retrofit/P6A.4/
  test.log
  summary.txt
```

## 8. 下一步

P6A.5：读取实验期凭据。

```text
DEEPSEEK_API_KEY
DEEPSEEK_BASE_URL
DEEPSEEK_MODEL
```

要求：

- 缺失/空时明确失败。
- 日志不打印 key。
- 不写入 runs.model_json。
