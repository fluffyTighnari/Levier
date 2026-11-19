<!-- Levier — App onboarding, current runtime contract. -->

> Current App integration guide: see the repository [`README.md`](../README.md) and
> `sdk/python/README.md`. This document is the lower-level HTTP/WebSocket
> contract reference, not the primary onboarding path.


# App 接入 Levier

这份文档描述**当前可用**的接入方式，对应 P6B 已闭环的实现。
它以 Counter Agent 为参考，但内容不绑定语言、SDK 或文件布局。

> 第一次接入请先按 [`ONBOARDING.md`](ONBOARDING.md) 完成启动、App 创建和 `app check`。
> Python App 推荐先用 [`sdk/python/README.md`](../sdk/python/README.md) 和
> [`APP_ADAPTER_CONTRACT.md`](APP_ADAPTER_CONTRACT.md)；本文保留完整的底层协议与运行细节。

> MVP 只支持 `runtime=normal`。`sandbox` 与 `longterm` 尚未闭环，
> 不要在接入中使用；崩溃语义与限制见 `docs/MVP.md`。

---

## 1. 心智模型

接入后职责被拆成三段：

```text
App      ：业务函数 + 工具元数据 + 一条 WebSocket 通道
          （没有模型循环，也不直接决定什么时候调用工具）

Levier：注册表、调度、工具路由、幂等、事件流
          （不执行你的业务代码）

React Runtime  ：ReAct 循环 + 模型调用
          （只通过 Levier 请求工具）
```

一次带工具调用的 Run：

```text
App 提交 Run
  → MW 调度到某台 React Runtime
  → React Runtime 调模型
  → 模型返回 tool_call
  → React Runtime 发 ToolCallRequest
  → MW 路由到该 App 的 WebSocket
  → App 执行，回 local_tool_result
  → MW 落库并回给 React Runtime
  → React Runtime 把结果加入下一轮模型输入
  → Run 终态
```

App 需要实现的只有：

```text
1. 注册自己的身份与工具
2. 保持一条 WebSocket
3. 收到 local_tool_call 时执行并回 local_tool_result
4. 提交 Run（或由调用方提交）
```

---

## 2. 接入前需要什么

```text
LEVIER_URL          Levier HTTP 地址（非秘密）
LEVIER_ADMIN_TOKEN  管理凭据，只用于创建/轮换 App
APP_NAME        App 自己选定的全局唯一名称
```

运行时只需要：

```text
APP_ID
APP_KEY
LEVIER_URL
```

---

## 3. 一次性 bootstrap

```http
POST {LEVIER_URL}/admin/apps
Authorization: Bearer {LEVIER_ADMIN_TOKEN}
Content-Type: application/json
```

```json
{"name": "{APP_NAME}", "multiagent": false}
```

新建（201）：

```json
{
  "app": {"app_id": "app_...", "name": "...", "status": "active"},
  "api_key": "mw_..."
}
```

已存在（200）：

```json
{
  "app": {"app_id": "app_...", "name": "...", "status": "active"},
  "api_key": ""
}
```

`api_key` 只在创建时返回一次。为空说明需要轮换：

```http
POST {LEVIER_URL}/admin/apps/{APP_ID}/rotate-key
Authorization: Bearer {LEVIER_ADMIN_TOKEN}
```

```json
{"api_key": "mw_..."}
```

保存：

```text
credential_version / app_id / app_name / server / 时间戳  非秘密
api_key                                                  秘密
```

保存后不要继续操作 Admin token。第一次接入建议直接使用
[`ONBOARDING.md`](ONBOARDING.md) 中的 `levier app init` 和 `levier app check`。

> Admin token 不要进入 App 的长期运行环境。

---

## 4. 运行时自检

```http
GET {LEVIER_URL}/api/v1/apps/{APP_ID}
Authorization: Bearer {APP_KEY}
```

```text
200 + status=active   → key 有效
401                   → key 错误或已轮换
```

这一步只验证 key 与 App 的归属，不验证工具。

---

## 5. 工具声明与注册

### 5.1 声明

每个工具需要：

```text
name          唯一
description   给模型看的说明
schema        JSON Schema
timeout_ms    超时（请求未指定时的兜底）
```

示例：

```json
{
  "name": "counter_add",
  "description": "Add an integer delta to the counter.",
  "schema": {
    "type": "object",
    "properties": {"delta": {"type": "integer"}},
    "required": ["delta"],
    "additionalProperties": false
  },
  "timeout_ms": 10000
}
```

### 5.2 注册

```http
POST {LEVIER_URL}/api/v1/apps
Authorization: Bearer {APP_KEY}
Content-Type: application/json
```

```json
{
  "name": "{APP_NAME}",
  "multiagent": false,
  "tools": [ ... ]
}
```

规则：

```text
tools 是全量替换
新增工具必须提交完整列表
"tools": [] 清空
不传 tools 不修改
schema 为空时补 {}
重名会提升 version
```

### 5.3 确认落库（运维）

```sql
SELECT name, scope, schema_json, timeout_ms, version
FROM tool_defs WHERE app_id = '{APP_ID}' ORDER BY name;
```

期望 `scope = local`，`schema_json` 与注册内容一致。

---

## 6. 建立 App 通道（WebSocket）

### 6.1 换 token

```http
POST {LEVIER_URL}/api/v1/ws-token
Authorization: Bearer {APP_KEY}
```

```json
{"ws_url": "/api/v1/ws", "ws_token": "...", "expires_in": 60}
```

token 是短期的，只放内存。

### 6.2 连接

```text
GET {LEVIER_URL}{ws_url}?app_id={APP_ID}&token={ws_token}
 Upgrade: websocket
```

服务端会校验 token 与 app_id 是否匹配，不匹配直接断开。

### 6.3 可选：hello / ping / subscribe

```json
{"type": "hello", "protocol": 1}
{"type": "ping"}
{"type": "subscribe", "run_id": "...", "last_attempt": 0, "last_seq": 0}
```

- `ping` → 服务端回 `pong`。
- `subscribe` → 先补历史事件，再回 `caught_up`，之后实时推送 `event`。
- 订阅只影响**事件流**，不是接收工具调用的前提。

---

## 7. 服务 local_tool_call

### 7.1 收到的消息

```json
{
  "type": "local_tool_call",
  "msg_id": "01...",
  "invocation_id": "01...",
  "tool_name": "counter_add",
  "arguments_json": "{\"delta\": 2}",
  "timeout_ms": 10000
}
```

要点：

```text
arguments_json 是字符串，不是对象
invocation_id 是幂等键
msg_id 是本次投递的相关性键
timeout_ms 是本次调用的截止时间
```

### 7.2 执行

```text
parse arguments_json as JSON object
verify tool_name is one you registered
run your business function
serialize the result as JSON text
```

不要在这里调用模型，也不要依赖自己的循环决定顺序。

### 7.3 回包

成功：

```json
{
  "type": "local_tool_result",
  "correlation_id": "<原样回传 msg_id>",
  "invocation_id": "<原样回传>",
  "ok": true,
  "result_json": "{\"value\": 2}"
}
```

失败：

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

校验规则（服务端强制）：

```text
type 必须是 local_tool_result
correlation_id 必填，且等于 msg_id
invocation_id 必填
ok=true 时不能带 error
ok=false 时必须带 error
result_json 必须是字符串
```

### 7.4 服务端如何判定

```text
找不到 invocation_id            → LOCAL_TOOL_UNKNOWN
invocation_id 属于别的 App      → LOCAL_TOOL_FORBIDDEN
correlation_id 不匹配           → LOCAL_TOOL_CORRELATION_MISMATCH（原调用继续等待）
同一结果重复发送                → 静默忽略
```

`LOCAL_TOOL_CORRELATION_MISMATCH` 时**不要换一个 id 重发**，应修正为原 `msg_id`。

### 7.5 生命周期

```text
收到调用 → 执行 → 回包 → 继续等下一条
连接断开 → 该 App 的 pending 调用失败（APP_UNREACHABLE）
超过 timeout_ms 未回 → TOOL_TIMEOUT
```

一个连接可以连续处理多次调用；不要在第一次回包后退出。

---

## 8. 提交 Run

```http
POST {LEVIER_URL}/api/v1/runs
Authorization: Bearer {APP_KEY}
Content-Type: application/json
Idempotency-Key: {可选}
```

```json
{
  "runtime_name": "react1",
  "params": {"max_steps": 8},
  "input": [{"role": "user", "content": "Add 2, then add 3, then read the counter."}],
  "model": {"provider": "deepseek", "model": "deepseek-flash"},
  "timeouts": {"run_ms": 60000, "tool_ms": 10000}
}
```

响应：

```http
202 Accepted
```

```json
{"run_id": "01...", "status": "queued", "ws_url": "/api/v1/ws", "ws_token": "..."}
```

说明：

- 工具清单不需要在 Run 里传，MW 按 `app_id` 从注册表组装。
- `tool_ms` 控制本地工具单次调用的上限。
- 相同 `Idempotency-Key` 重试不会重复建 Run（内容不同则 409）。

查询：

```http
GET {LEVIER_URL}/api/v1/runs/{run_id}
GET {LEVIER_URL}/api/v1/runs/{run_id}/events?attempt=&seq=
```

---

## 9. 事件流（可选）

订阅后收到：

```json
{
  "type": "event",
  "run_id": "01...",
  "attempt": 1,
  "seq": 5,
  "source": "daemon",
  "event_type": "TOOL_CALL_END",
  "data": {"tool_name": "counter_add", "ok": true, "result_json": "{\"value\": 2}"},
  "ts_ms": 1794368388919
}
```

断线重连时用 `last_attempt` / `last_seq` 续传；服务端会先补历史再发 `caught_up`。

不订阅也能正常执行工具；订阅只影响可观测性。

---

## 10. 错误与重试矩阵

| 现象 | 服务端行为 | App 应该做什么 |
|---|---|---|
| 未连接 | `APP_UNREACHABLE`，可重试 | 重连后继续服务 |
| 超时未回 | `TOOL_TIMEOUT`，可重试 | 检查是否卡死 |
| 工具不存在 | `TOOL_NOT_FOUND` | 核对注册名 |
| 跨 App 调用 | `TOOL_DENIED` / `LOCAL_TOOL_FORBIDDEN` | 不应出现，检查 app_id |
| App 返回 ok=false | 错误码透传给模型 | 用稳定、可读的 code |
| correlation 错误 | `LOCAL_TOOL_CORRELATION_MISMATCH` | 用原 msg_id 重发 |
| 重复回包 | 忽略 | 不要主动重发成功结果 |
| 非法消息 | `BAD_MESSAGE`（连接保留） | 修正 JSON |

---

## 11. 安全红线

```text
App key       不进源码、日志、证据、git
Admin token   只用于 bootstrap/rotate
ws_token      只放内存，不入库
日志          不打印 Authorization、token、完整凭据文件
错误信息      不回显 key
```

凭据文件建议 0600。

---

## 12. 验收清单

| 检查 | 期望 | 已实测 |
|---|---|---|
| 创建 App | 201/200，key 只返回一次 | P3.1 |
| `apps` 落库 | 只存 hash | P3.2 |
| App key 认证 | 200；错误 key 401 | P3.3 |
| 工具注册 | 200 | P3.4 |
| `tool_defs` | local + app_id + schema + timeout | P3.5 |
| TaskAssign | 含注册工具，schema 一致 | P4.1–P4.2 |
| App 隔离 | 只能看到自己的工具 | P4.3 |
| WS token | 可签发、过期/跨 App 拒绝 | P5.1–P5.7 |
| `local_tool_call` | invocation_id / args / timeout 正确 | P6.3 |
| `local_tool_result` | correlation=msg_id，网关接受 | P6.4 |
| 结果进入下一轮 | 模型引用工具结果 | P6.5 |
| `tool_invocations` | completed + result_json | P6.6 |
| 事件成对 | BEGIN/END 一一对应 | P6.7 |
| 终态 | completed，且不被离线 requeue | P6.8 |

---

## 13. 参考实现

`examples/counter_agent/` 是一个可运行的参考，**不是规范**：

| 文件 | 职责 |
|---|---|
| `app.py` | `LevierApp`、工具 schema、业务 handler、启动入口 |
| `tools.py` | 纯业务 Counter 状态 |
| `tests/` | Counter 与 SDK 分发测试 |

Python App 推荐直接使用 `sdk/python`；其他语言只需满足：

```text
能发 HTTP/WS
能解析与生成 JSON
能安全保存 APP_ID/APP_KEY
把 arguments_json / result_json 当字符串处理
```

不需要照搬目录结构。

---

## 14. 常见坑

1. **把 `arguments_json` 当对象解析两次。** 它已经是字符串。
2. **回包后断开连接。** 后续调用会变成 `APP_UNREACHABLE`。
3. **用新的 msg_id 回包。** 必须是收到的那个。
4. **以为要订阅 Run 才能收到工具调用。** 不需要。
5. **在 Run 里传工具。** 工具来自注册表。
6. **多实例 App 同时连同一 app_id。** 工具调用会广播到所有连接，只有一个能正确回包；
   需要多实例时先做连接归属设计。
7. **把 key 写进日志便于排查。** 排查用 app_id / invocation_id / run_id。

---

## 15. 自检

协议自检：

```sh
python3 tests/app/app_check.py --credentials /path/to/app.json
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
可选：subscribe → caught_up
非法 ws_token 被拒绝
```

真实工具自检（App 必须已经在运行）：

```sh
python3 tests/app/app_check.py   --credentials /path/to/app.json   --run-tool   --tool-name counter_add   --arguments '{"delta":2}'   --expect-substring 'value'
```

退出码：

```text
0  通过
1  失败（stderr 给出具体检查项）
```

`--json` 输出适合 CI 解析。

机器可读契约：

```text
docs/api/openapi.yaml            HTTP
contracts/app-ws.schema.json     WebSocket v1
internal/errs/errors.go          错误码
```

---

## 16. 相关文档

```text
docs/DESIGN.md                  核心设计
docs/MVP.md                     单机 MVP 边界与运行
docs/MCP.md                     MCP 接入与运行
contracts/README.md             机器可读契约索引
tests/app/app_check.py          App 接入自检
```
