<!-- Levier — daemon protocol contract. -->

# Daemon 接入契约

面向任何要接入 Levier 的执行侧 daemon（Harness 或自定义 runtime）。

## 1. 传输与身份

```text
gRPC  RuntimeService.Channel（双向流）
认证  daemon_id + auth_token，由 Levier 配置预分配
```

连接后第一条消息必须是 `Hello`。
`daemon_id` 不能自助注册；运维先在配置中创建。

## 2. 生命周期

```text
daemon                          middleware
  | ---- Hello ------------------> |
  | <--- TaskAssign -------------- |
  | ---- TaskAck(accepted) ------> |
  | ---- RUN_STATUS running -----> |
  | ---- 业务事件 ---------------> |
  | ---- RUN_STATUS terminal ----> |
  | <--- Cancel ------------------ |   （用户/超时）
  | <--- Reconcile --------------- |   （重连对账）
  | ---- Heartbeat --------------> |   每 10s
```

## 3. Hello

```text
daemon_id             预分配 ID
auth_token            预分配 token
runtime_type          normal / sandbox / longterm
max_concurrent_runs   容量
runtime_name          稳定路由名，例如 react1
version               daemon 版本
labels                pool / region 等调度标签
running_runs          重连时仍在执行的 Run
```

`running_runs[]` 包含 `run_id` 与 `lease_epoch`。
Levier 会用它做对账，并下发 `Reconcile`。

## 4. TaskAssign

```text
run_id / app_id / params
input                 ChatMessage[]
tools                 ToolManifest[]
mcp_bindings          McpBinding[]
timeouts              run_ms / tool_ms
session_id            longterm 会话（可空）
resume_from           checkpoint（可空）
trace_context         traceparent
```

Envelope 上还有：

```text
attempt / lease_epoch
msg_id
```

### Ack

```text
task_ack.accepted = true
correlation_id = TaskAssign.msg_id
```

拒绝时：

```text
accepted = false
reason = "capacity" / "runtime mismatch" / ...
```

Levier 会换 daemon 或重试。

## 5. 事件

```text
event.seq        每个 (run_id, attempt) 从 1 递增
event.source     daemon
event.type       RUN_STATUS / MESSAGE_* / TOOL_CALL_* / ERROR
event.data       JSON object
envelope.attempt / lease_epoch 必须与分配一致
```

终态：

```text
RUN_STATUS data.status ∈ completed / failed / cancelled / timeout
```

规则：

```text
seq 必须单调，不能跳号后回退
旧 attempt / lease_epoch 的事件会被丢弃
RUN_STATUS terminal 之后不要再发持久事件
```

## 6. 工具调用

daemon 主动发起：

```text
ToolCallRequest
  invocation_id   幂等键
  tool_name
  arguments_json  字符串
  timeout_ms
```

Levier 返回：

```text
ToolCallResult
  invocation_id
  ok
  result_json / error{code,message,retryable}
```

`correlation_id` 由 Levier 回填请求的 `msg_id`。
超时或 App 不可达会返回 `TOOL_TIMEOUT` / `APP_UNREACHABLE`。

## 7. Cancel

```text
Cancel.reason = user / timeout / reconcile / daemon_lost
```

daemon 应：

```text
尽快停止该 Run
释放资源
不要再发非终态事件
```

不需要 ack。

## 8. Reconcile

```text
Reconcile.cancel_run_ids[]
```

daemon 必须取消这些 Run。通常发生在重连后，
Levier 认为某些 Run 已经不属于它。

## 9. Heartbeat

```text
每 10s 发送一次
running_count
cpu_load / mem_mb（可选）
```

长时间无心跳会被判定为不健康，影响调度。

## 10. 容量与策略

```text
max_concurrent_runs  超过时 Levier 不会继续分配
runtime_name 不匹配的 Runtime 不会分配
labels               参与 placement
```

## 11. 错误处理

```text
Ack 拒绝            → 换 daemon / 重试
事件 epoch 过期      → 丢弃
工具调用失败         → 以 ToolCallResult.error 返回
流断开              → Levier 按 daemon 下线处理
```

## 12. 参考实现与自检

参考客户端：

```text
internal/testutil/fakedaemon
```

自检命令：

```sh
make daemon-check
# 或
levier daemon check --grpc 127.0.0.1:9090 --server http://127.0.0.1:8080
```

它会：

```text
Hello
接受 TaskAssign + Ack
发 running/completed 事件
创建第二个 Run 并验证 Cancel
输出 PASS/FAIL（--json 为 JSON）
```

前提：

```text
daemon_id 已在 Levier 配置中预分配
该 daemon_id 当前空闲；真实 Harness 占用的 id 会返回 AlreadyExists
```

## 13. 常见错误

```text
1. Hello 后不 ack，导致 MW 反复重派
2. 事件 seq 从 0 开始或回退
3. 用旧 lease_epoch 发事件
4. terminal 状态后又发 RUN_STATUS
5. Cancel 后继续发事件
6. 不上报 running_runs，重连后无法对账
7. Hello.runtime_name 不匹配，永远拿不到任务
```
