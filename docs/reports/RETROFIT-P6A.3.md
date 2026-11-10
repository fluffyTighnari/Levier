<!-- Middleware — retrofit P6A.3 provider error mapping. -->

# Retrofit P6A.3：OpenAI-compatible 错误映射

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P6A.3 |
| 状态 | 完成 |
| 代码提交 | `ad7a3eb` |
| 范围 | Harness provider 内部；fake transport |

## 1. 错误映射

| HTTP/情况 | code | retryable |
|---|---|---|
| 401 / 403 | `AUTH_FAILED` | false |
| 429 | `RATE_LIMITED` | true |
| 408 | `TIMEOUT` | true |
| 5xx | `PROVIDER_UNAVAILABLE` | true |
| 400 / 404 / 422 | `BAD_REQUEST` | false |
| 其他 | `PROVIDER_ERROR` | false |
| 传输超时 | `TIMEOUT` | true |
| 网络失败 | `PROVIDER_UNAVAILABLE` | true |

## 2. 超时识别

之前 `HTTPTransportError` 只保留字符串，无法区分超时和网络错误。

现在：

```python
class HTTPTransportError(RuntimeError):
    def __init__(self, message, *, timeout=False):
        self.timeout = timeout
```

`UrllibTransport` 在 worker 中检测：

```python
isinstance(exc, (TimeoutError, socket.timeout))
```

超时：

```python
raise HTTPTransportError(..., timeout=True)
```

Provider：

```python
if exc.timeout:
    raise LLMError("TIMEOUT", ..., retryable=True)
else:
    raise LLMError("PROVIDER_UNAVAILABLE", ..., retryable=True)
```

## 3. SSE error

```json
data: {"error":{"type":"rate_limit_exceeded","message":"slow down"}}
```

→

```text
RATE_LIMITED, retryable=true
```

错误类型映射：

```text
authentication_error / invalid_api_key → AUTH_FAILED
invalid_request_error → BAD_REQUEST
rate_limit_error / rate_limit_exceeded → RATE_LIMITED
overloaded_error / server_error → PROVIDER_UNAVAILABLE
timeout / timeout_error → TIMEOUT
```

## 4. 实际测试

```text
test_p6a_3_http_errors ... ok
test_p6a_3_transport_timeout ... ok
test_p6a_3_stream_error ... ok
Ran 3 tests
OK
```

覆盖：

- 401
- 429
- 500
- 400
- 传输超时
- SSE error

全量 Harness：

```text
make harness-test
Ran 77 tests
OK
```

## 5. 设计依据

```text
DESIGN §4.5 错误模型
harness LLMError
harness HTTPTransportError
```

ReactStrategy 已有：

```text
LLMError → ERROR 事件 → run failed
```

所以 provider 只负责映射到 `LLMError`。

## 6. 当前不覆盖

- provider 路由（P6A.4）
- DEEPSEEK_API_KEY 读取（P6A.5）
- 真实 DeepSeek 错误响应
- 重试策略（Harness 策略层）

## 7. 证据

```text
docs/reports/artifacts/retrofit/P6A.3/
  test.log
  summary.txt
```

## 8. 下一步

P6A.4：provider 路由。

```text
provider=anthropic → AnthropicProvider
provider=deepseek/openai → OpenAICompatibleProvider
其他 → BAD_REQUEST
```
