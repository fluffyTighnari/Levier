<!-- Middleware — brownfield app baseline experiment. -->

# APP-BASELINE：0 依赖 Agent 基线

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-09 |
| 状态 | 基线完成 |
| 关联 | `examples/counter_agent/`、`examples/http_client/` |
| 目的 | 在接入 Middleware 前，先建立一个可运行、可对照的存量 Agent 应用 |

## 1. 结论

新增两个 0 依赖示例：

1. `examples/counter_agent/`：单文件 Python 标准库 Agent。
   - 硬编码计数器工具。
   - 自带最小 ReAct 循环。
   - 直接调用 DeepSeek `deepseek-flash`。
   - 不注册、不起 HTTP 服务、不依赖 Middleware SDK。
2. `examples/http_client/`：单文件 Python 标准库 HTTP 客户端。
   - 不导入 Middleware/生成代码。
   - 走 Admin App bootstrap、App key、Run/Events/ws-token/cancel/session。
   - 用于证明 HTTP 层语言无关。

## 2. Counter Agent

运行：

```sh
make counter-agent
```

实测：

```text
[model] deepseek-flash
[user] Add 2, then add 3, then read the counter...
[tool] counter_add({"delta": 2}) -> {"value": 2}
[tool] counter_add({"delta": 3}) -> {"value": 5}
[tool] counter_get({}) -> {"value": 5}
[assistant] The counter tool reports a final value of **5**.
[final] counter=5 steps=4
```

特点：

- 只用 `urllib` 调 OpenAI-compatible `chat/completions`。
- 工具调用循环最多 8 步。
- 系统提示强制“必须调用工具，不能心算”。
- 自动读取仓库根 `.env` 的 `DEEPSEEK_API_KEY`。
- 不打印 key。

## 3. 与正式架构的差异

当前基线把：

```text
App 入口 + ReAct 循环 + LLM 调用 + 工具执行
```

放在同一进程。

正式 Middleware 拆分：

| 职责 | 正式归属 |
|---|---|
| Run 提交、状态、事件 | App → HTTP API |
| 计数器函数 | App local tools → WS |
| ReAct 循环 / LLM | Harness `react` |
| 工具路由 / 幂等 / ACL | Gateway |

后续改造步骤：

1. 保留计数器函数。
2. 增加 App WS 客户端，处理 `local_tool_call` 并回 `local_tool_result`。
3. 删除本地 ReAct 循环，改为 `POST /api/v1/runs`。
4. Harness 用 DeepSeek provider 跑同一提示词。
5. 对比改造前后的最终计数器、工具调用次数、事件序列。

## 4. HTTP 语言无关验证

运行：

```sh
make http-smoke
```

实测：

```text
health: ok request_id=01...
app: app_01...
ws_token: url=/api/v1/ws expires_in=600
run: 01... status=queued request_id=...
run_status: queued attempt=0
events: count=0 caught_up=True
error_envelope: ok
cancel: ok
```

证明点：

- 客户端不共享 Go 类型或生成代码。
- 认证、JSON、错误信封、请求 ID 全部由 Python 自行构造/解析。
- 同一进程完成 Admin bootstrap、App key、Run、Events、ws-token、cancel、session 404。

限制：

- 只证明 HTTP，不证明 WebSocket。
- 没有 Harness 时 Run 停在 queued；脚本只要求契约响应，不要求完成。

## 5. WebSocket 语言无关验证

WS 是下一项需要单独证明的协议边界：

- 握手：HTTP Upgrade + `app_id/token` query。
- 文本帧 JSON。
- `hello` / `subscribe` / `ping`。
- `event` / `caught_up` / `pong`。
- `local_tool_call` / `local_tool_result`。

最简单方案：

1. Node 内置 `WebSocket`（无 npm 依赖）。
2. 或 Python 标准库手写最小 WS 握手/帧编解码。
3. 或 `websocat` 手工 smoke。

推荐 Node 或 Python 标准库实现，确保不导入 Middleware 代码。

## 6. fakedaemon 评估

`internal/testutil/fakedaemon` 不是 App，而是 daemon 侧 gRPC 测试替身。

保留价值：

- 拒绝 TaskAck、断连、心跳、容量、慢消费者等确定性故障注入。
- 单元/集成测试速度快，不依赖 Python Harness 和 LLM。

不适用场景：

- 本次“存量 App 改造”验证。
- 真实 Harness/LLM 行为。
- 需要真实事件、工具循环、checkpoint 的 E2E。

结论：**不删除 fakedaemon，但不再作为主 E2E；主链路改用真实 Harness，fakedaemon 只服务故障矩阵和快速测试。**

## 7. 下一步

详细逐边计划见 `docs/APP_RETROFIT_PLAN.md`。

1. 改造 counter_agent：拆出 App WS 工具宿主。
2. Harness 增加 DeepSeek provider 配置，跑同一提示词。
3. 用 `examples/http_client` 提交 Run，并对比基线。
4. 补一个语言无关 WS 客户端。
5. 再进入全链路调试计划。
