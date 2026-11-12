<!-- Middleware — MCP M7 real HTTP transports. -->

# MCP M7：真实 SSE / streamable_http

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-12 |
| 状态 | 完成 |
| 样本 Run | `01M76QQ8BRC9S0DEEA3WC4QZ49` |
| 服务器 | 本地 MCP SDK server（sse :9101 / streamable-http :9102） |

## 1. 目标

补上唯一完全依赖 fake 的部分：真实 HTTP transport。

```text
sse                 GET /sse，消息走 /messages/
streamable_http     POST /mcp
```

## 2. 测试 server

`mcp_runtime/tests/http_echo_server.py`：

```text
echo(text)          回显文本
add(a,b)            两数相加
auth_header(ctx)    返回服务端看到的 Authorization 头（仅测试）
```

启动：

```sh
python http_echo_server.py --transport sse --port 9101
python http_echo_server.py --transport streamable-http --port 9102
```

## 3. Runtime 探针

绑定头：

```json
{"Authorization": "Bearer ${secret:http_token}"}
```

runtime 环境：`MIDDLEWARE_SECRET_HTTP_TOKEN=http-secret`。

结果：

| transport | tools | echo | auth_header |
|---|---|---|---|
| sse | echo / add / auth_header | `ok` | `Bearer http-secret` |
| streamable_http | echo / add / auth_header | `ok` | `Bearer http-secret` |

说明：

- 两条 transport 都能建立 session、`tools/list`、`tools/call`
- `${secret:...}` 在 HTTP header 中被解析并发送到服务端

## 4. 端到端 Run

目录：

```text
mcp-sse   transport=sse              url=http://127.0.0.1:9101/sse
mcp-http  transport=streamable_http  url=http://127.0.0.1:9102/mcp
```

App 同时引用两者，一次 Run 调用三个工具：

```text
seq 4/5  mcp__mcp_sse__echo        ok=true  "sse-ok"
seq 6/7  mcp__mcp_http__echo       ok=true  "http-ok"
seq 8/9  mcp__mcp_sse__auth_header ok=true  "Bearer http-secret"
seq 12   RUN_STATUS completed
```

其他：

```text
tool_invocations   0
runtime sessions   4 opened / 4 closed（探针 2 + Run 2）
ERROR 事件         0
```

## 5. 凭据卫生

```text
middleware.log  出现 http-secret：0
harness.log     出现 http-secret：0
runtime.log     出现 http-secret：0
sse_server.log  出现 http-secret：0
http_server.log 出现 http-secret：0
```

明文只出现在工具结果（事件与模型上下文）中，符合工具返回值语义。
模型自己也提示“这是凭据值，不要外传”，属于正常行为。

## 6. 结论

| 项目 | 结果 |
|---|---|
| SSE 真实链路 | 通过 |
| streamable_http 真实链路 | 通过 |
| HTTP header 占位符解析 | 通过 |
| 两种 transport 同一 Run 并存 | 通过 |
| 不写 Gateway invocation | 通过 |
| session 回收 | 4/4 |

## 7. 局限

- 本地明文 HTTP，没有 TLS / 反代 / 鉴权挑战。
- `auth_header` 是测试工具，不得进入真实 catalog。
- 没有测 SSE 断线重连、事件续传。
- 单 Run；并发与浸泡仍未覆盖。

## 8. 证据

```text
docs/reports/artifacts/mcp/M7/
  probe.py / probe.json        两条 transport 的 open/list/call
  catalog_sse.json
  catalog_http.json
  app.json / bootstrap.json
  db.txt                       事件、invocations=0、最终回答
  runtime.log                  4 opened / 4 closed
  sse_server.log
  http_server.log
  harness.log
  middleware.log
  submit_run.json
  summary.txt
```

## 9. 下一步

```text
SSE 断线重连 / 事件续传
TLS + 服务端鉴权挑战
并发 10 / 浸泡 100 轮
```
