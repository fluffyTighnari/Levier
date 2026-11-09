<!-- Middleware — language-agnostic HTTP client verification. -->

# 语言无关 HTTP 验证

`client.py` 只用 Python 标准库（`json`、`os`、`urllib`），不导入 Middleware
代码、不使用生成代码、不依赖第三方包。

它验证的是“语言无关”的实际含义：

- 认证只用 HTTP `Authorization` 头。
- 请求/响应只用 JSON。
- 路径、状态码、错误信封和 `X-Request-ID` 与 Go 实现一致。
- Admin App bootstrap 与 App key 走普通 HTTP。
- Run 创建、状态、事件、ws-token、cancel、session 404 全流程。

## 运行

先启动 Middleware：

```sh
make m1-e2e   # 或单独启动 bin/middleware
```

再运行：

```sh
MW_URL=http://127.0.0.1:8080 \
MW_ADMIN_TOKEN=admin-change-me \
python3 examples/http_client/client.py
```

输出示例：

```text
health: ok request_id=01...
app: app_01...
ws_token: url=/api/v1/ws expires_in=600
run: 01... status=queued request_id=01...
run_status: queued attempt=0
events: count=0 caught_up=True
error_envelope: ok
cancel: ok
```

## 为什么这能证明语言无关

不是“用 curl 调一次”就算证明，而是：

1. 客户端不共享 Go 类型或生成的 protobuf/OpenAPI 代码。
2. 客户端自行构造 JSON 与 HTTP 请求。
3. 客户端自行解析错误信封与请求 ID。
4. 同一套契约在 Python 进程中完成认证、创建、查询、取消。

如果这条链路通过，说明 HTTP 层没有隐式依赖 Go 运行时或内部类型。

## WS 的验证

HTTP 通过不代表 WebSocket 也语言无关。WS 还需要额外验证：

- 握手头与鉴权 query。
- JSON 文本帧。
- `hello` / `subscribe` / `ping`。
- `event` / `caught_up` / `pong`。
- `local_tool_call` / `local_tool_result`。

最小方案是使用 Node 内置 `WebSocket`，或 Python 标准库手写最小的
WebSocket 握手与帧编解码（约 80–120 行）。该客户端不应导入 Middleware 代码。

## 边界

- 该脚本只验证 HTTP，不验证 WS。
- 默认只要求 Run 创建成功，不要求 Harness 已完成；需要完整事件时先启动 Harness。
- 脚本会 rotate App key；不要与手工调试共用同一个 App。
