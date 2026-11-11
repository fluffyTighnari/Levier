<!-- Middleware — retrofit P8 language independence. -->

# Retrofit P8：语言无关（P8.1–P8.5）

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-11 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P8 |
| 状态 | 完成 |
| 代码提交 | `292cc0e` |
| 参考实现 | `examples/node_counter_agent/serve.mjs` |

## 1. 目的

P1–P7 的接入实现都在 Python 里。P8 用另一种语言重写 App 侧，
只依赖公开的 HTTP/WS/JSON 合同，证明接入没有隐藏的 Python 或 Go 绑定。

## 2. P8.1 Python → HTTP

```sh
make http-smoke
```

```text
health: ok request_id=01M74FHMJQJMKQJSXS3XX66G81
app: app_01M6ZSK8AXQE6TQH69YEMXYW15
ws_token: url=/api/v1/ws expires_in=600
run: 01M74FHMJYZ9Q8Q8XETTAJ4SSJ status=queued
run_status: queued attempt=0
events: count=0 caught_up=True
error_envelope: ok
cancel: ok
```

覆盖：健康检查、bootstrap、ws-token、提交 Run、查询事件、错误信封、取消。

## 3. P8.2 Node → WS

`examples/node_counter_agent/serve.mjs`：

```text
Node v24.21.0
global WebSocket: function
imports:
  node:http
  node:https
  node:fs
```

无 `package.json`、无 `node_modules`、无 SDK、无生成代码。

启动日志：

```json
{"serving":true,"app_id":"app_01M74FHZ1N9Q6EZTNAWH7QNWPX",
 "registered_tools":["counter_add","counter_get","counter_reset"],"counter":0}
{"pong":true}
```

`pong` 证明握手之后的帧收发都正常。

## 4. P8.3 Node 处理 local_tool_call

同一个 Run 由 Python 提交，工具由 Node 执行（跨语言）：

```text
counter_add {"delta": 2} -> {"value": 2}
counter_add {"delta": 3} -> {"value": 5}
counter_get {}           -> {"value": 5}
handled=3 counter=5
```

服务端：

```text
run 01M74FJAQ4ZJXPG0S971D4FG6W completed
TOOL_CALL_END counter_add {"value":2}
TOOL_CALL_END counter_add {"value":5}
TOOL_CALL_END counter_get {"value":5}
tool_invocations 3 行 completed
```

最终结果与 Python 实现一致。

## 5. P8.4 无 SDK / 生成代码

```text
SDK / generated imports: 0
package.json: 无
node_modules: 无
node --check: ok
```

文件里出现 "Middleware"/"proto" 只在注释中，不是依赖。

## 6. P8.5 字段与 DESIGN 对齐

Node 回包：

```json
{
  "type": "local_tool_result",
  "correlation_id": "01M74FJEQNY0XYH9460R69RJ2Y",
  "invocation_id": "01M74FJEQE1S68F8ZCK2YS4VM1",
  "ok": true,
  "result_json": "{\"value\":2}"
}
```

DESIGN：

```text
msg_id         消息唯一 ID（ULID）
correlation_id 响应方回填请求方 msg_id
invocation_id  幂等键
arguments_json 工具参数（字符串）
result_json    ok=true 时的内容（字符串）
```

字段名、类型、语义完全一致；没有大小写或 snake/camel 偏差。

## 7. 汇总

| ID | 检查 | 结果 |
|---|---|---|
| P8.1 | Python → HTTP 全流程 | 通过 |
| P8.2 | Node → WS 握手与帧 | 通过 |
| P8.3 | Node 处理工具调用 | 通过 |
| P8.4 | 无 SDK/生成代码 | 通过 |
| P8.5 | 字段对齐 | 通过 |

## 8. 局限

- Node 实现只覆盖 WS 工具路径，未实现 bootstrap/事件订阅。
- 使用 Node 内置 WebSocket，未手写帧编解码；协议兼容性由标准客户端保证。
- 未测试 Node 与 Python 同时服务同一 App（多连接广播会重复投递）。

## 9. 证据

```text
docs/reports/artifacts/retrofit/P8/
  py_http_smoke.log
  node_serve.log
  node_serve.err
  bootstrap_node.json
  submit_run.json
  db_run.txt
  node_deps.txt
  field_alignment.txt
  middleware.log
  harness.log
  migrate.log
  summary.txt

examples/node_counter_agent/
  serve.mjs
  README.md
```

## 10. 下一步

P9 已完成主要故障项（见 P6D）；剩余为恢复与并发类场景。
P10：回归与收口，一键脚本可重复。
