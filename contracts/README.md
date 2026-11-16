<!-- Middleware — machine-readable App contracts. -->

# 契约文件

## 单元卡

| 项 | 内容 |
|---|---|
| 职责 | App 侧 HTTP / WebSocket 机器可读契约 |
| 入口 | `contracts/app-ws.schema.json`、`docs/api/openapi.yaml` |
| 输入 / 输出 | 协议定义；供客户端、测试、生成物使用 |
| 依赖 | 无运行时依赖 |
| 不负责 | 实现、部署、示例 |
| 测试 | `make contract-check` |

| 文件 | 覆盖 |
|---|---|
| `docs/api/openapi.yaml` | App/Admin HTTP API（OpenAPI 3.1） |
| `contracts/app-ws.schema.json` | App WebSocket v1（JSON Schema 2020-12） |
| `internal/errs/errors.go` | 错误码、retryable、HTTP 映射 |
| `docs/DAEMON_CONTRACT.md` | Daemon RuntimeChannel 契约与自检 |

## HTTP

`docs/api/openapi.yaml` 是唯一 HTTP 契约，包含：

```text
/healthz
/admin/apps
/admin/apps/{app_id}/rotate-key
/apps
/apps/{app_id}
/runs
/runs/{run_id}
/runs/{run_id}/events
/runs/{run_id}/cancel
/sessions/{session_id}
/sessions/{session_id}/events
/ws-token
/ws
```

Go 测试 `TestOpenAPIMatchesHTTPRoutes` 会检查契约与路由一致。

## WebSocket

`app-ws.schema.json` 覆盖：

```text
client: hello / subscribe / ping / local_tool_result
server: event / local_tool_call / run_completed / pong / caught_up / error
```

约定：

```text
arguments_json / result_json 是字符串，不是对象
correlation_id 必须等于收到的 msg_id
协议版本 = 1
```

## 错误

错误码以 `internal/errs/errors.go` 为准。
HTTP 状态由错误码推导，客户端应读取 `error.code`，不要只依赖状态码。

## 自检

```sh
python3 tests/app/app_check.py --credentials /path/app.json
python3 tests/app/app_check.py --credentials /path/app.json \
  --run-tool --tool-name counter_add --arguments '{"delta":2}'
```

## 版本

```text
HTTP: OpenAPI 3.1
WS:   protocol 1
```
