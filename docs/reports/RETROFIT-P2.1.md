<!-- Middleware — retrofit P2.1 App HTTP configuration. -->

# Retrofit P2.1：App HTTP 配置读取

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P2.1 |
| 状态 | 完成 |
| 代码提交 | `1316e49` |
| 范围 | 只读环境变量；无网络、无注册 |

## 1. 配置项

| 变量 | 必填 | 默认 |
|---|---|---|
| `MW_URL` | 否 | `http://127.0.0.1:8080` |
| `MW_ADMIN_TOKEN` | 是 | — |
| `MW_APP_NAME` | 否 | `retrofit-counter` |

新增：

```text
examples/counter_agent/config.py
```

结构：

```python
@dataclass(frozen=True)
class AppConfig:
    mw_url: str
    admin_token: Secret
    app_name: str
```

## 2. 校验

- `MW_URL` 必须是绝对 `http(s)` URL。
- 末尾 `/` 自动去掉。
- `MW_ADMIN_TOKEN` 为空或全空白时报 `ConfigError`。
- `MW_APP_NAME` 空白时回退默认值。
- `Secret.__str__/__repr__` 输出 `***`。
- `summary()` 不包含 token 明文。

## 3. 设计依据

```text
DESIGN §4.2  App HTTP prefix / Bearer
DESIGN §13.1 admin token 独立于 App API key
```

## 4. 验证

```sh
cd examples/counter_agent
python3 -m unittest -v test_config
```

```text
test_p2_1_defaults ... ok
test_p2_1_overrides_and_trailing_slash ... ok
test_p2_1_rejects_bad_url ... ok
test_p2_1_requires_admin_token ... ok
test_p2_1_secret_not_printed ... ok
Ran 5 tests
OK
```

全量：

```sh
make counter-agent-test
```

```text
tests 17
OK
```

## 5. 证据

```text
docs/reports/artifacts/retrofit/P2.1/test.log
docs/reports/artifacts/retrofit/P2.1/summary.txt
```

## 6. 当前不覆盖

- 不发 HTTP 请求。
- 不读取/缓存 App API key。
- 不读取 DeepSeek 配置。
- 不修改 `main.py`。

## 7. 下一步

P2.2：只构造 `/admin/apps` 请求。

- method/header/body 正确。
- 通过注入的 transport 记录请求。
- 不真实联网。
