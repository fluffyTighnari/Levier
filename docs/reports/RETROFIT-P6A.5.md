<!-- Middleware — retrofit P6A.5 DeepSeek credentials. -->

# Retrofit P6A.5：实验期 DeepSeek 凭据

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P6A.5 |
| 状态 | 完成，P6A 收口 |
| 代码提交 | `7463e94` |
| 范围 | Harness provider 环境变量；真实 DeepSeek smoke |

## 1. 凭据解析

仅当 `model.provider == "deepseek"` 且字段为空时使用：

```text
DEEPSEEK_API_KEY
DEEPSEEK_BASE_URL
DEEPSEEK_MODEL
```

规则：

```text
model.api_key 非空 → 优先
model.base_url 非空 → 优先
model.model 非空 → 优先
否则 → 环境变量
否则 base_url → https://api.deepseek.com
否则 model → deepseek-flash
```

缺失 key：

```text
PROVIDER_UNAVAILABLE
"DEEPSEEK_API_KEY is required for provider deepseek"
retryable=false
```

且不会发出 HTTP 请求。

## 2. 安全

- key 不打印。
- key 不写入日志。
- key 不写入 `runs.model_json`。
- key 只在 Harness 内存和 HTTP Authorization 头中使用。
- 显式传入的 `model.api_key` 优先于环境变量。

> 这是实验期路径。生产应由 Middleware 配置/密钥介质注入模型凭据，作为后续硬化项。

## 3. 单元验证

```text
test_p6a_5_env_credentials ... ok
test_p6a_5_missing_key_fails_before_request ... ok
test_p6a_5_explicit_key_overrides_env ... ok
Ran 3 tests
OK
```

断言：

- 环境变量生效。
- URL：`https://env.example/v1/chat/completions`
- Header：`Authorization: Bearer env-key`
- model 使用 `DEEPSEEK_MODEL`
- 缺失 key 时请求数为 0
- 显式 key 覆盖环境变量

全量 Harness：

```text
make harness-test
Ran 82 tests
OK
```

## 4. 真实 smoke

使用仓库 `.env` 的 `DEEPSEEK_API_KEY`，不打印 key：

```json
{
  "content": "OK",
  "model": "deepseek-flash",
  "usage": {
    "prompt_tokens": 40,
    "completion_tokens": 11,
    "total_tokens": 51
  },
  "tool_calls": 0
}
```

说明：

- 真实 DeepSeek `/chat/completions` 可调用。
- 环境变量凭据解析成功。
- Provider 返回内容/usage/model 正确。
- 没有输出密钥。

## 5. P6A 收口

| 步骤 | 状态 |
|---|---|
| P6A.1 非流式 JSON | 完成 |
| P6A.2 SSE 流式 | 完成 |
| P6A.3 错误映射 | 完成 |
| P6A.4 provider 路由 | 完成 |
| P6A.5 DeepSeek 凭据 | 完成 |

P6A 完成后：

```text
Harness
  ├─ AnthropicProvider
  ├─ OpenAICompatibleProvider
  ├─ ProviderRouter
  ├─ stream / non-stream
  ├─ tool_calls
  ├─ usage
  ├─ error mapping
  └─ DeepSeek env credentials
```

## 6. 证据

```text
docs/reports/artifacts/retrofit/P6A.5/
  test.log
  real_smoke.log
  summary.txt
```

## 7. 下一步

P6.1：真实 react Run。

```text
App 注册 counter tools
→ 提交 react Run（provider=deepseek）
→ TaskAssign 到达真实 Harness
→ Harness 用 DeepSeek 产生 tool_call
```
