<!-- Middleware — retrofit P2.3 admin app response. -->

# Retrofit P2.3：解析 Admin App 响应

> 说明：本步解析的是 mock 响应，不是真实 MW 响应；真实链路见 `docs/reports/RETROFIT-P3.1.md`。

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P2.3 |
| 状态 | 完成 |
| 代码提交 | `e7b0666` |
| 范围 | 只解析响应；无网络 |

## 1. 新增

`http_client.py`：

```python
@dataclass(frozen=True)
class AppRegistration:
    app_id: str
    name: str
    api_key: Secret

    @property
    def has_api_key(self) -> bool
```

```python
parse_create_app_response(response: HttpResponse) -> AppRegistration
```

```python
class HttpError(RuntimeError):
    status, code, message, retryable
```

## 2. 解析规则

### 201 新建

```json
{
  "app": {"app_id": "app-1", "name": "retrofit-counter"},
  "api_key": "mw_..."
}
```

→

```text
app_id = app-1
name = retrofit-counter
api_key = Secret("mw_...")
has_api_key = true
```

### 200 App 已存在

```json
{
  "app": {"app_id": "app-1", "name": "retrofit-counter"},
  "api_key": ""
}
```

→

```text
has_api_key = false
```

这符合 Middleware 的语义：API key 只返回一次；已存在的 App 需要 rotate。

### 错误

非 200/201：

```json
{"error": {"code": "UNAUTHORIZED", "message": "...", "retryable": false}}
```

→

```text
HttpError(status, code, message, retryable)
```

## 3. 安全

- `api_key` 存为 `Secret`。
- `repr(AppRegistration)` 输出 `Secret('***')`。
- 测试断言 `"mw_secret" not in repr(result)`。
- 不打印响应体。

## 4. 设计依据

```text
DESIGN §4.2
internal/httpapi/contracts.go CreateAppResponse
internal/httpapi/server.go handleAdminCreateApp
```

状态码语义：

```text
201 → 新建 App + key
200 → 已存在 App，key 为空
```

## 5. 验证

```sh
cd examples/counter_agent
python3 -m unittest -v test_app_response
```

```text
test_p2_3_parses_created_app ... ok
test_p2_3_duplicate_app_has_no_key ... ok
test_p2_3_missing_app_id_fails ... ok
test_p2_3_error_envelope ... ok
test_p2_3_invalid_json_fails ... ok
Ran 5 tests
OK
```

全量：

```text
make counter-agent-test
tests 23
OK
```

## 6. 证据

```text
docs/reports/artifacts/retrofit/P2.3/test.log
docs/reports/artifacts/retrofit/P2.3/summary.txt
```

## 7. 当前不覆盖

- 真实 urllib transport。
- 自动 rotate 已存在 App。
- 工具注册请求。
- App key 持久化。
- WS。

## 8. 下一步

P2.4：构造工具注册请求。

只验证：

```text
POST /api/v1/apps
Authorization: Bearer <app_api_key>
body.tools[] = TOOL_DEFINITIONS
```
