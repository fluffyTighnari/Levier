# ADR-0002：精简 Python Harness（v1）

> **Historical design record.** Strategy was removed in the App/Runtime simplification step 0. The current protocol is defined by `proto/middleware/runtime/v1/runtime.proto` and `docs/DAEMON_CONTRACT.md`.


> 路径说明：`agent_core/`、`harness/`、`mcp_runtime/` 已迁移到 `execution/`；本文保留历史路径。

| 项目 | 内容 |
|---|---|
| 状态 | 提议（P0-03；MCP 章节按 ADR-0004 修订） |
| 日期 | 2026-11-03 |
| 决策者 | middleware 项目 |
| 关联 | `docs/DESIGN.md`、`docs/adr/0001-workflow-engine.md`、`docs/adr/0004-mcp-ownership.md` |

---

## 1. 背景

middleware 不执行 Agent 的推理循环。它把 Run 分配给一个 Python Harness daemon，由 daemon：

- 接收 `TaskAssign`；
- 运行 ReAct/策略循环；
- 调用 LLM；
- 通过 middleware 调用 local/global 工具；
- 直连 MCP server；
- 把消息、工具调用、usage、状态等事件上报 middleware。

现有 harness 不可直接复用，且 middleware 只需要一条最小主线。因此本项目选择**重写一个阉割适配版 Harness**，而不是兼容通用 Agent 框架。

---

## 2. 决策

在仓库 `harness/` 下实现一个新的 Python daemon：

- Python 3.12，`asyncio` 单事件循环。
- gRPC 使用 `grpc.aio`，一个 daemon 一条 `RuntimeService.Channel` 双向流。
- 配置使用 `pydantic-settings`；日志使用标准 `logging` + JSON formatter；指标用 `prometheus_client`。
- 策略先实现 `react` 和 `echo`；保留 `Strategy` 接口，`plan_execute` 不在 v1。
- LLM 先实现 **OpenAI-compatible Chat Completions**，Anthropic 作为第二适配器；内部统一归一化事件。
- 工具调用：
  - 非 MCP 工具 → 发 `ToolCallRequest` 给 middleware，等待 `ToolCallResult`；
  - MCP 工具 → 由独立 `mcp_runtime` 进程连接 MCP server；Harness 通过 `McpRuntimeClient` 调用；总控只下发 `mcp_bindings`。
- v1 工具调用串行执行；不并行调用多个工具。
- Longterm 使用 checkpoint/resume；normal/sandbox 不使用 checkpoint。
- 不支持多 Agent、插件市场、GUI、向量记忆、复杂 Plan&Execute、跨 runtime 子 Agent。

---

## 3. 目标与非目标

### 3.1 目标

1. 与 middleware 建立单条 gRPC 双向流，完成 Hello/心跳/重连。
2. 接收 TaskAssign，5s 内 TaskAck；接受后异步执行。
3. 实现 `echo` 与最小 `react` 策略。
4. 支持 OpenAI-compatible 流式 LLM 调用，产生 delta/completed/usage 事件。
5. 支持 middleware 工具调用闭环（local/global 由 MW 路由）。
6. 支持 MCP stdio/SSE/streamable_http 客户端，工具名做命名空间隔离。
7. 支持取消、超时、daemon 重连、优雅退出。
8. 支持 longterm checkpoint/resume，恢复时不重复已持久化消息。
9. 事件 seq、attempt、lease_epoch 正确，不污染 Run 状态。
10. 所有密钥不落日志。

### 3.2 非目标

- 不做多 Agent 编排、子 Agent 调度。
- 不做插件系统、工具市场、GUI。
- 不做向量数据库/长期记忆。
- 不做 `plan_execute`（接口保留，v1 不实现）。
- 不做模型路由/多 provider 自动 fallback；provider 由 TaskAssign 指定。
- 不做 MCP server 生命周期托管；MCP 连接由独立 `mcp_runtime` 负责。
- 不做 middleware 的调度/重试决策；只上报错误与可重试性。
- 不保证 exactly-once；工具调用依赖 invocation_id 幂等。

---

## 4. 运行时结构

```
┌──────────────────────────────────────────────┐
│                Harness Process               │
│                                              │
│  ChannelClient (grpc.aio)                    │
│    ├─ Hello / Heartbeat / TaskAck            │
│    ├─ inbound: TaskAssign / Cancel / Reconcile│
│    └─ outbound: Event / ToolCallRequest /     │
│                 Checkpoint / ErrorReport      │
│                                              │
│  RunManager                                  │
│    ├─ semaphore(max_concurrent_runs)         │
│    ├─ map[run_id]*RunContext                 │
│    └─ graceful shutdown / cancellation       │
│                                              │
│  RunContext                                  │
│    ├─ Strategy (react/echo)                  │
│    ├─ LLMClient                              │
│    ├─ ToolBridge (middleware)                │
│    ├─ McpRuntimeClient                       │
│    ├─ EventWriter (seq/attempt/epoch)        │
│    └─ CheckpointStore                        │
└──────────────────────────────────────────────┘
```

### 4.1 并发模型

- 一个进程一个 asyncio event loop。
- `RunManager` 用 `asyncio.Semaphore(max_concurrent_runs)` 限制并发；默认 4。
- 每个 Run 一个 `asyncio.Task`；Run 之间不共享可变业务状态。
- 每个 Run 独立 MCP session/工具清单；Runtime 负责隔离，避免跨 Run 污染。
- 工具调用在 Run 内串行；v1 不做并行 tool calls。

### 4.2 包结构

```text
harness/
├── pyproject.toml
├── src/levier_harness/
│   ├── __main__.py
│   ├── config.py
│   ├── logging.py
│   ├── metrics.py
│   ├── channel/
│   │   ├── client.py          # grpc.aio Channel
│   │   ├── envelope.py        # msg_id/correlation/validation
│   │   └── reconnect.py
│   ├── runtime/
│   │   ├── manager.py         # RunManager / semaphore / shutdown
│   │   ├── run.py             # RunContext
│   │   ├── events.py          # EventWriter: seq/attempt/epoch
│   │   └── cancel.py
│   ├── strategies/
│   │   ├── base.py
│   │   ├── echo.py
│   │   └── react.py
│   ├── llm/
│   │   ├── base.py
│   │   ├── openai_compat.py
│   │   └── anthropic.py
│   ├── tools/
│   │   ├── bridge.py          # ToolCallRequest/Result
│   │   ├── manifest.py        # 工具清单合并/命名空间
│   │   └── mcp_client.py      # McpRuntimeClient + tool discovery
│   └── checkpoint.py
└── tests/
    ├── unit/
    ├── integration/
    └── fixtures/
```

---

## 5. 协议生命周期

### 5.1 建立连接

1. 读取配置：middleware 地址、daemon_id、auth_token、runtime_type、max_concurrent_runs、strategies、labels。
2. 建立 gRPC Channel（TLS 可选，v1 同集群内明文/可选 mTLS）。
3. 3s 内发送 `Hello`：
   - daemon_id、auth_token、runtime_type；
   - max_concurrent_runs；
   - supported_strategies（v1: `react`, `echo`）；
   - version；
   - labels；
   - `running_run_ids`：本进程仍在执行的 Run，携带 `(run_id, lease_epoch)`。
4. 启动心跳：每 10s 发送 `Heartbeat{running_count, cpu_load, mem_mb}`。
5. 启动 inbound 循环，处理 TaskAssign/Cancel/Reconcile/ToolCallResult/Error。

### 5.2 重连

- gRPC 断开后指数退避：1s → 2s → 4s → … 上限 30s，带 jitter。
- 重连后重新 Hello；`running_run_ids` 来自内存中的 RunContext。
- 等待 middleware 的 `Reconcile`：
  - `cancel_run_ids` 中的 Run 立即取消；
  - 未列出的 Run 继续执行（若 middleware 认为已死，会重新分配）。
- 进程重启后内存丢失；由 middleware 判定 daemon lost 并重排队；longterm 从 checkpoint 恢复。

### 5.3 TaskAssign

1. 收到 `TaskAssign`，校验：
   - `run_id`、`app_id`、`strategy`、`attempt`、`lease_epoch` 非空；
   - `strategy` 在 supported_strategies 中；
   - 当前并发未超限。
2. 5s 内发送 `TaskAck{accepted=true|false, reason}`，`correlation_id = TaskAssign.msg_id`。
3. 接受后创建 RunContext，启动异步执行；Ack 不等待执行完成。
4. 拒绝原因：容量不足、未知 strategy、配置非法。
5. 重复 TaskAssign（同 run_id + lease_epoch）幂等：若已在运行，返回 accepted=true 并复用；若已完成/已取消，返回 accepted=false。

### 5.4 Cancel / Reconcile

- `Cancel{reason}`：
  - 设置 `cancel_event`；
  - 关闭当前 LLM stream；
  - 取消挂起的 ToolCallRequest（向 middleware 返回取消错误或直接忽略结果）；
  - 在安全点保存 checkpoint（longterm）；
  - 发送 `RUN_STATUS cancelled`，然后清理。
- `Reconcile{cancel_run_ids}`：对每个 id 执行上述取消；其余 Run 继续。
- 取消是幂等的；重复 Cancel 不报错。

### 5.5 优雅退出

1. 停止接受新的 TaskAssign（可选：先发送一个 draining 心跳/状态）。
2. 对每个 Run 触发 Cancel 或 checkpoint（longterm）。
3. 发送终态事件后关闭 gRPC。
4. 关闭 MCP session、HTTP 客户端、DB/文件句柄。

> v1 没有单独的 draining 消息；可通过部署侧先停止分配（Control 标记 daemon draining）再发 SIGTERM。

---

## 6. 事件模型

### 6.1 Envelope 字段

每个出站 `Envelope` 带：

- `msg_id`：ULID；
- `correlation_id`：响应类消息回填请求 msg_id；
- `run_id`；
- `ts_ms`；
- `attempt`：Run 的执行 attempt（TaskAssign 下发）；
- `lease_epoch`：Run 的归属代次（TaskAssign 下发）。

> `attempt` 与 `lease_epoch` 必须进入 Event/Envelope，否则 middleware 无法对重试事件分区、也无法丢弃旧 daemon 的事件。

### 6.2 seq 规则

**只有持久化事件消耗 `seq`。** ephemeral 事件不占用 seq：

| 事件 | ephemeral | seq |
|---|---|---|
| `RUN_STATUS` | 否 | 递增 |
| `MESSAGE_COMPLETED` | 否 | 递增 |
| `TOOL_CALL_BEGIN/END` | 否 | 递增 |
| `USAGE` | 否 | 递增 |
| `ERROR` | 否 | 递增 |
| `CHECKPOINT_SAVED` | 否 | 递增 |
| `MESSAGE_DELTA` | 是 | `0`（或 payload 内 `delta_index`） |

原因：如果 ephemeral 也消耗 seq，持久化事件会出现“空洞”，middleware 的乱序/补拉逻辑会误判 `SEQ_GAP`。MESSAGE_DELTA 的实时性由连接保证；断线后由最终 `MESSAGE_COMPLETED` 兜底。

### 6.3 事件 payload（v1 约定）

| EventType | data 关键字段 |
|---|---|
| `RUN_STATUS` | `status`（running/completed/failed/cancelled/timeout）、`reason` |
| `MESSAGE_DELTA` | `message_id`、`role`、`delta`、`delta_index` |
| `MESSAGE_COMPLETED` | `message_id`、`role`、`content`、`tool_calls[]` |
| `TOOL_CALL_BEGIN` | `invocation_id`、`tool_name`、`arguments_json`、`source`（mw/mcp） |
| `TOOL_CALL_END` | `invocation_id`、`ok`、`result_json`/`error` |
| `USAGE` | `provider`、`model`、`input_tokens`、`output_tokens` |
| `ERROR` | `code`、`message`、`retryable` |
| `CHECKPOINT_SAVED` | `session_id`、`attempt`、`seq` |

约束：

- 事件 payload 不放密钥、Authorization header、MCP env。
- `MESSAGE_COMPLETED` 的 `content` 使用统一结构，避免 provider 差异泄漏到 middleware。
- 工具参数/结果按字符串 JSON 传递；大结果由 middleware 限制大小。

---

## 7. 策略接口与 ReAct

### 7.1 Strategy 接口

```python
class Strategy(Protocol):
    name: str

    async def run(self, ctx: RunContext, assign: TaskAssign) -> None: ...
```

`RunContext` 提供：

- `emit(event)`：分配 seq、写 Envelope；
- `call_tool(name, arguments, timeout_ms)`：走 middleware 或 MCP；
- `llm`：归一化 LLM 客户端；
- `checkpoint`：longterm 保存；
- `cancelled`：取消检查；
- `step`：当前步数。

### 7.2 React 循环（v1）

```text
messages = assign.input
step = 0

while step < max_steps and not cancelled:
    resp = await llm.chat(messages, tools, stream=True)
    emit MESSAGE_COMPLETED(resp.message)
    emit USAGE(resp.usage)

    if no tool_calls:
        emit RUN_STATUS completed
        return

    for call in resp.tool_calls:
        emit TOOL_CALL_BEGIN
        result = await ctx.call_tool(call.name, call.arguments, timeout)
        emit TOOL_CALL_END
        messages.append(tool_result_message(call.id, result))

    step += 1

emit RUN_STATUS failed(reason=MAX_STEPS)
```

- `max_steps` 来自 TaskAssign.strategy_params，默认 32。
- 每个 LLM 调用前检查取消。
- 每次工具调用后检查取消。
- 工具错误作为 tool result 回给模型；是否终止由模型/策略决定；连续错误可触发策略上限。
- v1 串行调用工具；不并行。

### 7.3 echo 策略

- 不调用 LLM。
- 读取 TaskAssign.input 最后一条用户消息，返回固定/回显内容。
- 发 `RUN_STATUS running` → `MESSAGE_COMPLETED` → `RUN_STATUS completed`。
- 用于 M1 E2E 和契约验证。

---

## 8. LLM 适配

### 8.1 Provider 抽象

```python
class LLMClient(Protocol):
    async def chat(
        self,
        messages: list[Message],
        tools: list[ToolSpec],
        *,
        stream: bool,
        cancel: asyncio.Event,
    ) -> LLMResponse: ...
```

`LLMResponse` 归一化：

- `message`：role/content/tool_calls；
- `usage`：input/output tokens；
- `finish_reason`；
- `raw`（仅调试，不持久化）。

### 8.2 v1 Provider

- 首选：**OpenAI-compatible Chat Completions**（覆盖 OpenAI 及多数兼容网关）。
- 第二：Anthropic Messages API。
- 若首个部署明确是 Anthropic，可交换实现顺序，接口不变。
- 不做自动 provider fallback；provider/model 由 TaskAssign 指定。

### 8.3 流式与错误

- 流式 delta 归一化为 `MESSAGE_DELTA`（ephemeral）。
- 流结束发送完整 `MESSAGE_COMPLETED`（持久化）。
- 429/5xx/网络超时：可重试；策略层指数退避，最多 N 次（默认 2）。
- 401/403/400：不可重试，发 `ERROR` + `RUN_STATUS failed`。
- 取消：关闭 stream，丢弃后续 delta，发送终态。
- usage 缺失时允许 0，但记录 provider/model。

---

## 9. 工具路由

### 9.1 middleware 工具（local/global）

- TaskAssign.tools 是 middleware 已合并的清单（local 遮蔽同名 global）。
- Harness 只看到工具名/schema，不关心 scope；调用统一发：

```json
{
  "type": "tool_call_request",
  "msg_id": "01...",
  "run_id": "...",
  "attempt": 1,
  "lease_epoch": 3,
  "invocation_id": "01...",
  "tool_name": "...",
  "arguments_json": "...",
  "timeout_ms": 60000
}
```

- 等待 `ToolCallResult`，按 `invocation_id` / `correlation_id` 关联。
- `ToolCallResult.error.retryable=true`：策略可重试；**重试必须复用同一个 invocation_id**，让 middleware 缓存/single-flight 生效。
- 默认每个工具最多重试 2 次；仍失败则把错误作为 tool result 回给模型。
- v1 不并行调用多个工具。

### 9.2 MCP 归属：声明在总控，连接在独立 McpRuntime

> 本节按 ADR-0004 修订：MCP Runtime 是独立进程，不是 Harness 内部类，也不在 `harness/` 目录内。

MCP 的职责必须拆成两半：

| 能力 | 归属 | 说明 |
|---|---|---|
| MCP 目录、App 引用、ACL、凭据引用 | Register / Control | 总控只存“声明”和“授权结果” |
| `mcp_bindings` 组装 | Control | 下发连接描述，不下发活连接 |
| MCP 连接、stdio 进程、`tools/list`、`tools/call` | `mcp_runtime` 独立进程 | 与 Harness 同层的执行侧组件 |
| 工具循环 / 结果回填 LLM | Harness | 通过 `McpRuntimeClient` 调用 Runtime |
| 中央审计/限流/共享连接 | 未来独立 MCP Hub（v2） | 不在 middleware 主进程内 |

**结论：MCP 连接归属执行层的独立 `mcp_runtime` 进程，不进入 Middleware，也不是 Harness 进程内的一个类。** App 只在总控声明 `mcp_refs`；middleware 只维护目录/ACL/bindings；Harness 通过本地 RPC 调用 Runtime。

`TaskAssign.mcp_bindings` 的字段（proto 只传描述，不传连接对象）：

```text
server_name
transport            # stdio | sse | streamable_http
command / args / env # stdio
url / headers        # sse / streamable_http
allowed_tools[]      # 空 = 全部；非空 = 白名单
namespace            # 默认 mcp__<server_name>__
```

### 9.3 McpRuntime 契约

`mcp_runtime` 通过本地 JSON-RPC（v1 默认 Unix domain socket）暴露：

```text
mcp.health() -> {status, version}
mcp.open(binding) -> {session_id, tools[]}
mcp.call(session_id, tool_name, arguments_json, timeout_ms) -> {ok, result_json, error}
mcp.close(session_id) -> {}
```

- Harness 侧只实现 `McpRuntimeClient`，不导入 MCP SDK，不直接启动 MCP server。
- v1 提供独立 Python `mcp_runtime` 进程；单客户端调试可用 stdio，生产默认 Unix socket。
- 同一 host/pod 的多个 Harness 可以共享一个 Runtime。
- 每个 Run 使用独立 session；不跨 Run 复用，避免状态/凭据串扰。
- 每个 session 打开时连接 `mcp_bindings`，调用 `tools/list`，按 `allowed_tools` 过滤。
- **命名空间**：`mcp__<server_name>__<tool_name>`；server_name 做字符清洗。
- LLM 调用 `mcp__...` 时，Harness 通过 `McpRuntimeClient` 执行，不经过 middleware。
- MCP 工具也发 `TOOL_CALL_BEGIN/END` 事件，`source=mcp`，便于观测。
- MCP 连接失败/初始化失败：
  - 记录 `ERROR`；
  - 不阻塞整个 Run 的其他工具；
  - 对应工具不出现在工具清单中，或调用时返回工具错误。
- MCP 凭据只在 Runtime 内存中使用；不写日志、不进 checkpoint。

### 9.4 多 Harness 与未来 MCP Hub

- **v1（Python）**：独立 `mcp_runtime` 进程，Harness 只持客户端，零 MCP SDK 依赖。
- **第二种语言的 Harness**：连接同一 `mcp_runtime` 契约，不重复实现 MCP。
- **部署**：Runtime 与 Harness 同 pod/同 host，共享 Unix socket；不是新的控制面层。
- **未来中央 MCP Hub**：只有满足以下条件才引入：
  - 多语言 Harness 不愿各自维护 Runtime；
  - MCP server 昂贵，需要跨 App/Run 共享连接；
  - 需要总控级审计、限流、配额、工具白名单强制执行；
  - sandbox 不允许直接访问 MCP，必须走受控出口。
  - 引入时是独立数据面服务（显式 ADR），Harness 侧只替换 `McpRuntimeClient` 的远程实现，三层结构不变。

**明确拒绝：** 把 MCP 连接放进 middleware 的 Tool Gateway。Tool Gateway 只路由 local/global 工具；把 MCP 塞进去会让 middleware 变成数据面，并破坏 sandbox/stdio 的放置模型。

### 9.5 工具清单合并

1. TaskAssign.tools（MW 已合并 local/global）。
2. MCP 工具（`mcp_runtime` 发现，命名空间化）。
3. 若 MCP 工具名与 MW 工具名冲突，MCP 前缀保证不冲突。
4. 工具 schema 转成 provider 原生格式；MCP 的 inputSchema 直接使用。

---

## 10. Checkpoint 与恢复

### 10.1 触发

Longterm Run：

- 每 20 条持久化事件，或每 60s；
- 收到 Cancel 前；
- 正常完成前（可选，终态后可不再恢复）。

Checkpoint 只在安全点生成：

- 一次 LLM 流结束后；
- 一次工具调用完成后；
- 不在一半 delta、不在一半工具调用中。

### 10.2 内容

```json
{
  "version": 1,
  "session_id": "...",
  "run_id": "...",
  "attempt": 2,
  "strategy": "react",
  "step": 7,
  "messages": [],
  "last_event_attempt": 2,
  "last_event_seq": 42,
  "created_at": "..."
}
```

对 middleware 不透明；Harness 自己负责版本兼容。

### 10.3 恢复

- TaskAssign 携带 `resume_from`（checkpoint blob）；新 `attempt` 来自 `Envelope.attempt`。
- Harness 加载 messages/step，继续策略循环。
- **事件 watermark 必须带 attempt**：
  - `Envelope.attempt + Checkpoint.event_seq_watermark`；
  - middleware 的 `sessions` 存 `(event_attempt, event_seq)`；
  - 恢复时只重放 `(attempt > wm_attempt) OR (attempt = wm_attempt AND seq > wm_seq)`。
- 新 attempt 的事件 seq 从 1 开始；主键是 `(run_id, attempt, seq)`。
- 若 checkpoint 缺失/损坏：从头开始（normal）或返回 `ERROR`（longterm，由 Control 决定是否重跑）。

> DESIGN 只有单个 `event_watermark` 字段；本 ADR 明确需要复合 watermark，P5-37/P9-65 要增加 `event_attempt`。

---

## 11. 取消、超时、故障

### 11.1 取消

- Cancel 可发生在任意阶段。
- LLM 流：关闭 HTTP stream；丢弃后续 delta。
- 工具等待：取消 future；向 middleware 发送取消/忽略结果；工具调用由 MW 幂等处理。
- MCP：取消 in-flight request；关闭连接。
- 发送 `RUN_STATUS cancelled`；longterm 保存 checkpoint。

### 11.2 超时

- run_timeout 由 middleware 的 `RUN_TIMEOUT` 任务强制；Harness 不负责最终终态。
- 单次 LLM 调用有 provider timeout（默认 120s）。
- 单次工具调用使用 TaskAssign/ToolDef 的 timeout_ms。
- 策略 step 可配置超时；超时后作为工具/LLM 错误处理，或终止 Run。

### 11.3 故障

| 故障 | 行为 |
|---|---|
| gRPC 断开 | 重连退避；Hello 带 running_run_ids；等待 Reconcile |
| LLM 429/5xx | 退避重试；超过次数发 ERROR/failed |
| LLM 401/400 | 终止 Run，发 ERROR/failed |
| ToolCallResult 超时 | 发 TOOL_CALL_END(error)；策略决定重试/继续 |
| App 不可达 | MW 返回 `APP_UNREACHABLE(retryable)`；Harness 可重试 |
| MCP 连接失败 | 该工具不可用；不阻塞其他工具 |
| 进程崩溃 | middleware 判定 daemon lost，重排队；longterm 从 checkpoint 恢复 |
| 取消与完成竞态 | 先到者生效；后续事件带同一 attempt/epoch，由 MW 去重 |

---

## 12. 配置与安全

### 12.1 配置

```yaml
middleware:
  address: "mw:9443"
  tls: false
daemon:
  id: "daemon-01"
  auth_token: "${DAEMON_TOKEN}"
  runtime_type: "normal"
  max_concurrent_runs: 4
  strategies: ["react", "echo"]
  labels: { pool: "normal", region: "cn1" }
logging:
  level: "info"
  format: "json"
```

- 环境变量覆盖 YAML；密钥只从 env/secret 文件读取。
- 配置校验失败立即退出。

### 12.2 安全

- `auth_token`、LLM api_key、MCP env/header 不落日志、不进事件、不进 checkpoint。
- 日志脱敏：Authorization、api_key、token、cookie。
- 可选 mTLS（v1 同集群可明文；见 DESIGN §13.2 风险）。
- sandbox 模式由部署层限制 egress，Harness 不做网络策略。

---

## 13. 可观测性

### 13.1 日志

- JSON 字段：`daemon_id`、`run_id`、`attempt`、`lease_epoch`、`step`、`strategy`、`invocation_id`。
- 不记录完整 prompt/工具结果（避免 PII）；需要时用 debug 且显式开关。

### 13.2 指标

| 指标 | 类型 |
|---|---|
| `harness_runs_active` | gauge |
| `harness_runs_total` | counter（status） |
| `harness_llm_duration_seconds` | histogram（provider/model） |
| `harness_tool_duration_seconds` | histogram（source=mw/mcp） |
| `harness_tokens_total` | counter（provider/model/type） |
| `harness_reconnects_total` | counter |
| `harness_cancellations_total` | counter |

### 13.3 Trace

- 从 TaskAssign.trace_context 提取 W3C traceparent。
- Span：`harness.run`、`harness.llm.chat`、`harness.tool.call`、`harness.checkpoint`。
- 事件不携带 trace 上下文，日志关联 run_id/attempt 即可。

---

## 14. 测试策略

| 层级 | 内容 | 方式 |
|---|---|---|
| 单元 | envelope、seq、工具命名空间、checkpoint 编解码 | pytest |
| 策略 | echo、react（假 LLM） | pytest + fake LLM |
| 协议 | Hello/TaskAck/Cancel/Reconcile | fake gRPC server |
| 工具 | MW ToolCall 闭环、MCP stdio/HTTP | fake MW + fake MCP server |
| 重连 | 断线、Hello running_run_ids、Reconcile | 集成测试 |
| 恢复 | checkpoint 后重启、attempt/watermark | 集成测试 |
| E2E | 真实 middleware + 真实 LLM（可选） | docker-compose |

**纪律：** 所有 LLM/MCP 调用可注入 fake；测试不依赖公网。

---

## 15. 备选方案

| 方案 | 结论 | 原因 |
|---|---|---|
| 复用通用 Agent 框架 | 拒绝 | 依赖重、抽象多、难以裁剪；middleware 只需要一条主线 |
| 在 Go 中实现 Agent 循环 | 拒绝 | LLM/MCP 生态在 Python；与 DESIGN 不符 |
| 多 Agent 框架 | v1 拒绝 | 非目标；事件模型未设计 |
| 并行工具调用 | v1 拒绝 | 增加竞态与观测复杂度；串行足够 |
| 自动 provider fallback | v1 拒绝 | 计费/行为不可控；由 TaskAssign 明确 provider |
| MCP 经过 middleware / Tool Gateway | 拒绝 | 会把控制面变成数据面；stdio 进程必须在 Harness/sandbox 本地 |

---

## 16. 验收标准

- [ ] 与 middleware 建连、Hello 鉴权、心跳、断线重连。
- [ ] TaskAssign 5s 内 Ack；容量不足/未知策略可拒绝。
- [ ] echo 策略完成一次 Run，事件 seq 连续（仅持久化事件）。
- [ ] react 策略在假 LLM 下完成多步工具调用。
- [ ] local/global 工具经 middleware 闭环；重试复用 invocation_id。
- [ ] MCP 连接只在独立 `mcp_runtime` 内建立；middleware 不持有连接。
- [ ] MCP 工具命名空间正确，stdio/HTTP 至少各通一条。
- [ ] `mcp_bindings` 的 `allowed_tools` 生效，未授权工具不可见/不可调用。
- [ ] Cancel 能中断 LLM 流和工具等待，并发出 cancelled 终态。
- [ ] longterm checkpoint 恢复不重复已持久化消息；复合 watermark 正确。
- [ ] 旧 lease_epoch 的事件不会从 Harness 发出（TaskAssign 校验）。
- [ ] 日志/事件/checkpoint 无密钥。
- [ ] 进程崩溃后 middleware 能重排队；longterm 从 checkpoint 恢复。

---

## 17. 实施映射

| ADR 内容 | 计划步骤 |
|---|---|
| 包结构/配置/日志 | P3-23 |
| Channel/Hello/心跳/重连 | P3-24 |
| TaskAssign/TaskAck/Cancel/Reconcile | P3-25 |
| Strategy 接口 + echo | P3-26 |
| ReAct 循环 | P3-27 |
| LLM 适配 | P3-28 |
| 工具 + MCP | P3-29 |
| Checkpoint/打包 | P3-30 |
| `Envelope.attempt` / `Envelope.lease_epoch` | P1-08 |
| 复合 watermark 存储 | P5-37/P9-65 |

---

## 18. 对 DESIGN 的修改（已于 P0-04 并入 v0.2）

| DESIGN 位置 | 修改 |
|---|---|
| §3.1 `McpServerDef` | 增加 `allowed_tools_json`；连接仍不存 |
| §4.1 `TaskAssign.mcp_bindings` | 改为 `repeated McpBinding mcp_bindings`，含 `allowed_tools`、`namespace` |
| §5.3 MCP 行 | 改为“独立 `mcp_runtime` 直连，配置来自 `mcp_bindings`” |
| §9.2 `AssembleTaskMaterials` | 返回 `mcp_bindings[]`，含 ACL 过滤后的 `allowed_tools` |
| §13.2 凭据 | 明确只到 Harness；总控不持有连接 |
| §17 Q2 | 明确为“独立数据面服务”，不是 middleware 主进程 |
