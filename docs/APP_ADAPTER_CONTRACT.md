<!-- Levier — Python App adapter contract (C0). -->

# Python App Adapter 契约

> 状态：第一版已实现；范围仅 Python App。
>
> 本契约描述适配器与 App 业务代码之间的边界。WebSocket 服务端协议仍然是
> 已有的 App 协议（`contracts/app-ws.schema.json`）；本文件不改变服务端协议。

---

## 1. 版本

```text
HTTP API            /api/v1
WebSocket protocol  2（服务端继续接受 v1）
Python adapter      0.1.0
```

适配器启动时发送：

```json
{
  "type": "hello",
  "protocol": 2,
  "protocol_version": 2,
  "client": "levier-app-python",
  "version": "0.1.0",
  "capabilities": [
    "tool_call_v2",
    "tool_progress",
    "tool_cancel",
    "idempotent_result",
    "resume_cursor"
  ]
}
```

服务端返回 `hello_ack`，其中包含 `connection_id`、协商后的 `protocol`、
服务端能力以及心跳间隔。v1 客户端继续使用 `protocol: 1`，服务端不发送
`hello_ack`，旧语义保持不变。适配器不兼容时必须直接失败，不应继续进入
半连接状态。

---

## 2. App 业务代码边界

App 只负责：

```text
持有业务状态
定义工具名称、描述、JSON Schema
实现同步 handler(arguments: dict) -> JSON-serializable value
```

适配器负责：

```text
读取 canonical 凭据
HTTP 认证
App 状态检查
注册工具
取得 ws_token
建立 WebSocket
hello / ping / pong
断线重连
local_tool_call 分发
参数解析
结果序列化
错误结果封装
优雅停止
```

业务 handler 不应直接处理：

```text
Authorization
WebSocket
ws_token
msg_id / invocation_id / correlation_id
重连
```

---

## 3. 工具 manifest

公开的机器可读 manifest 样例在 `public/app-adapter/v1/`；本节的文字与 fixture
是内部实现说明。工具的 HTTP/WS 线上形状仍以 `contracts/app-ws.schema.json`
和 `docs/api/openapi.yaml` 为准。

每个工具注册到 Levier 的形状：

```json
{
  "name": "counter_add",
  "description": "Add an integer to the counter.",
  "schema": {
    "type": "object",
    "properties": {"delta": {"type": "integer"}},
    "required": ["delta"],
    "additionalProperties": false
  },
  "timeout_ms": 10000
}
```

约束：

```text
name 在单个 App 内唯一
schema 必须是 JSON Schema 对象
timeout_ms 必须为正整数
注册时提交完整工具列表，空列表也允许
```

`timeout_ms` 由服务端用于调用超时；当前同步 Python handler 不能安全强制中断，
因此 handler 应尽快返回，服务端超时仍是最终权威。后续如果引入异步 handler，
再由适配器执行本地 timeout。

---

## 4. 本地工具调用

服务端发送：

```json
{
  "type": "local_tool_call",
  "msg_id": "01...",
  "invocation_id": "01...",
  "run_id": "01...",
  "tool_name": "counter_add",
  "arguments_json": "{\"delta\": 2}",
  "timeout_ms": 10000,
  "deadline_ms": 1790000000000,
  "idempotent": true
}
```

`run_id`、`deadline_ms`、`idempotent` 是 v2 增量字段；v1 客户端可以忽略。
服务端只会把一次 invocation 投递给一个执行连接，不向同一 App 的所有连接
广播。

适配器要求：

```text
arguments_json 解析后必须是 JSON object
未知工具返回 TOOL_NOT_FOUND
参数解析失败返回 TOOL_EXEC_ERROR
handler 异常返回 TOOL_EXEC_ERROR
handler 主动抛 ToolExecutionError 时保留 code/retryable
correlation_id 必须等于收到的 msg_id
invocation_id 必须原样返回
v2 结果带 idempotent: true
```

handler 可以通过 `app.progress(value)` 发送进度：

```json
{
  "type": "local_tool_progress",
  "correlation_id": "<msg_id>",
  "invocation_id": "<invocation_id>",
  "run_id": "<run_id>",
  "progress_json": "{\"percent\": 50}"
}
```

服务端在取消一个 Run 的本地工具时发送：

```json
{"type":"cancel","invocation_id":"<invocation_id>","reason":"run cancelled"}
```

成功结果：

```json
{
  "type": "local_tool_result",
  "correlation_id": "<msg_id>",
  "invocation_id": "<invocation_id>",
  "run_id": "<run_id>",
  "ok": true,
  "result_json": "{\"value\": 2}",
  "idempotent": true
}
```

失败结果：

```json
{
  "type": "local_tool_result",
  "correlation_id": "<msg_id>",
  "invocation_id": "<invocation_id>",
  "ok": false,
  "error": {
    "code": "TOOL_EXEC_ERROR",
    "message": "delta must be an integer",
    "retryable": false
  }
}
```

适配器不把 handler 的异常文本放进日志中的敏感上下文；只记录工具名、错误码和
request/invocation 标识。

---

## 5. 生命周期

```text
读取凭据
检查 App 为 active
注册工具
取得 ws_token
连接 WebSocket
发送 hello
接收消息
连接断开 -> 重新取得 token 并重连
停止信号 -> 关闭 WebSocket，退出
```

重连使用退避，默认从 0.5 秒开始，最大 30 秒。认证失败不重试，直接退出，
并报告凭据或 App 状态问题。

---

## 6. 版本与兼容

v1 内允许增量变化：

```text
新增可选工具字段
新增可选事件字段
新增错误码
新增能力标记
```

不允许：

```text
改变 local_tool_call / local_tool_result 的关联语义
改变 correlation_id 规则
删除已有字段
改变已有字段含义
```

适配器忽略未知事件和未知可选字段；破坏性变化必须进入新的 HTTP/WS/适配器版本。

---

## 7. Fixtures

C0 的协议 fixture 位于：

```text
sdk/python/tests/fixtures/hello.json
sdk/python/tests/fixtures/local_tool_call.json
sdk/python/tests/fixtures/local_tool_result_success.json
sdk/python/tests/fixtures/local_tool_result_error.json
```

fixture 用于 SDK 单元测试和后续 conformance，不用于服务端实现。
