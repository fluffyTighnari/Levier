<!-- Middleware — retrofit P2.2 admin app request. -->

# Retrofit P2.2：构造 `/admin/apps` 请求

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P2.2 |
| 状态 | 完成 |
| 代码提交 | `0ff6b0f` |
| 范围 | 只构造请求；注入 transport；无真实网络 |

## 1. 新增

```text
examples/counter_agent/http_client.py
```

最小契约：

```python
HttpRequest(method, url, headers, body)
HttpResponse(status, headers, body)

Transport.send(request) -> response
HttpClient(config, transport)
HttpClient.create_app()
```

`HttpClient.create_app()` 生成：

```text
POST http://127.0.0.1:8080/admin/apps
Authorization: Bearer <admin_token>
Content-Type: application/json
Accept: application/json
```

Body：

```json
{"name":"retrofit-counter","multiagent":false}
```

## 2. 为什么先有 Transport

P2.2 只验证“请求构造”，不验证网络：

- 测试注入 `RecordingTransport`。
- transport 记录请求并返回固定响应。
- 不打开 socket，不依赖 MW 运行。

这样失败时可以确定：

```text
是请求构造错了
还是网络/服务端错了
```

## 3. 设计依据

```text
DESIGN §4.2  POST /admin/apps bootstrap
DESIGN §13.1 admin token
internal/httpapi/server.go  handleAdminCreateApp
internal/httpapi/contracts.go AppUpsertRequest
```

## 4. 验证

```sh
cd examples/counter_agent
python3 -m unittest -v test_http_client
```

```text
test_p2_2_builds_admin_app_request ... ok
Ran 1 test
OK
```

断言：

- method = POST
- URL = `http://127.0.0.1:8080/admin/apps`
- `Authorization: Bearer admin-secret`
- `Content-Type: application/json`
- body 精确等于 `{"name":"retrofit-counter","multiagent":false}`

全量：

```text
make counter-agent-test
tests 18
OK
```

## 5. 证据

```text
docs/reports/artifacts/retrofit/P2.2/test.log
docs/reports/artifacts/retrofit/P2.2/summary.txt
```

## 6. 当前不覆盖

- 真实 urllib transport。
- 响应解析/错误码。
- App key 保存。
- 工具注册请求。
- WS。

## 7. 下一步

P2.3：解析 App 响应。

只验证：

```text
201/200
→ app_id
→ api_key
→ key 不进日志
→ 缺失字段明确报错
```
