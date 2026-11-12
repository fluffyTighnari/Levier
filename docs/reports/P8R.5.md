<!-- Middleware — P8R.5 App onboarding kit. -->

# P8R.5：App 接入套件

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-12 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P8R.5 |
| 状态 | 完成 |
| 代码提交 | 见下 |

## 1. 机器可读契约

```text
HTTP      docs/api/openapi.yaml（已有，Go 测试校验路由一致）
WS        contracts/app-ws.schema.json（新增）
错误码    internal/errs/errors.go（已有）
索引      contracts/README.md
```

WS schema 覆盖：

```text
client: hello / subscribe / ping / local_tool_result
server: event / local_tool_call / run_completed / pong / caught_up / error
```

关键约束写进 schema：

```text
arguments_json / result_json 是字符串
correlation_id 必须存在
ok=true 时不允许 error，ok=false 时必须带 error
```

HTTP 没有复制一份新的 JSON Schema，直接复用 OpenAPI，避免两套契约漂移。

## 2. 自检工具

`scripts/app_check.py`，只用标准库，复用参考 WS 客户端。

### protocol 模式

```sh
python3 scripts/app_check.py --credentials /path/app.json
```

检查：

```text
GET /healthz
GET /apps/{id} 200；错误 key 401
可选：POST /apps 注册工具
POST /ws-token
WS hello + ping/pong
未知 invocation → LOCAL_TOOL_UNKNOWN
非法消息 → BAD_MESSAGE
非法 ws_token 被拒绝
可选：subscribe → caught_up
```

### run-tool 模式

```sh
python3 scripts/app_check.py --credentials /path/app.json \
  --run-tool --tool-name counter_add --arguments '{"delta":2}' \
  --expect-substring value
```

它会提交一次真实 Run，等待 `TOOL_CALL_END`，验证：

```text
App 真的收到调用并回包
result_json 命中期望
Run 终态
```

负向：

```sh
--expect-error APP_UNREACHABLE
```

## 3. 实测

Protocol：

```json
{
  "ok": true,
  "result": {
    "checks": {
      "health": "ok",
      "app_key": "ok",
      "tools": "registered 3",
      "ws": "connected/pong",
      "result_validation": "LOCAL_TOOL_UNKNOWN",
      "message_validation": "BAD_MESSAGE",
      "ws_token_rejected": "ok"
    }
  }
}
```

Tool 正向：

```json
{
  "run_id": "01M76VVZ1FNS2RYF46AJ8YXYKE",
  "status": "completed",
  "tool_name": "counter_add",
  "tool_ok": true,
  "result_json": "{\"value\": 2}",
  "calls": 1
}
```

Tool 负向（App 未启动）：

```json
{
  "run_id": "01M76VWAHK8QQFNCZ3SWWXRJ9E",
  "status": "completed",
  "tool_ok": false,
  "calls": 1
}
```

期望错误码 `APP_UNREACHABLE` 命中。

## 4. Schema 校验

用运行时 venv 的 `jsonschema` 做了一次性校验：

```json
{"schema": "ok", "samples": 5}
```

覆盖 local_tool_call、两种 local_tool_result、event、subscribe。

未加入 CI 依赖；后续可在 P8R.8 决定是否固化。

## 5. 文档

`docs/APP_ONBOARDING.md` 新增第 15 节：

```text
自检命令
检查项
退出码
机器可读契约路径
```

## 6. 局限

```text
run-tool 依赖模型按提示调用工具
schema 校验目前是一次性脚本，不在 CI
HTTP 契约依赖 OpenAPI，没有单独的 app-http.json
```

## 7. 证据

```text
docs/reports/artifacts/P8R.5/
  protocol.json
  tool.json
  tool_negative.json
  schema_validation.json
  tools.json
  middleware.log
  harness.log
  serve.log
```

## 8. 下一步

P8R.6：Daemon 接入套件。
