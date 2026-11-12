<!-- Middleware — pi-agent-core evaluation. -->

# pi-agent-core 评估

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-12 |
| 包版本 | `@earendil-works/pi-agent-core` 1.5.2 |
| 依赖 | `@earendil-works/pi-ai` 1.5.2 |
| 运行环境 | Node v24.21.0 |
| 结论 | 不替换当前 Python Harness；保留为可选第二 runner |

## 1. 做了什么

不是只看文档，跑了两个真实 spike：

### Spike 1：工具调用 + 事件流

`artifacts/pi-core/spike.mjs`：

```text
自定义 streamFn（不调真实模型）
第一次返回 toolcall: add{a:2,b:3}
执行 AgentTool
第二次返回文本 "5"
记录所有 AgentEvent
```

结果：

```json
{
  "prompt_ms": 3,
  "stream_calls": 2,
  "tool_results": [{"id": "call-1", "params": {"a": 2, "b": 3}}],
  "event_counts": {
    "agent_start": 1,
    "turn_start": 2,
    "message_start": 4,
    "message_end": 4,
    "tool_execution_start": 1,
    "tool_execution_end": 1,
    "turn_end": 2,
    "message_update:text_delta": 1,
    "message_update:toolcall_end": 1,
    "agent_end": 1
  }
}
```

### Spike 2：取消

`artifacts/pi-core/abort.mjs`：

```text
stream 挂起 1s
50ms 后 agent.abort()
```

结果：

```json
{"abort_ms": 55, "events": ["agent_start","turn_start","message_start","message_end",
 "message_start","message_end","turn_end","agent_end:aborted"]}
```

取消能在 55ms 内收敛，`stopReason=aborted`。

## 2. 它提供什么

| 能力 | 评价 |
|---|---|
| Agent 循环 | 成熟：turn 管理、并行/串行工具、终止条件 |
| 事件流 | 清晰：agent/turn/message/tool 事件，带 delta |
| 工具接口 | `AgentTool` + JSON Schema + hooks（before/after） |
| 取消 | AbortSignal，行为可预测 |
| Provider | pi-ai 统一多 provider，含 DeepSeek |
| 用量/成本 | `Usage` 已归一化（input/output/cache/cost） |
| 上下文处理 | `transformContext` / `prepareRequest` / compaction 钩子 |

## 3. 它不提供什么

```text
MCP 客户端          没有；仍需 mcp_runtime
会话持久化          没有；MW checkpoint 仍是唯一状态源
调度/租约/重试      没有；MW 继续负责
沙箱/执行隔离       没有
```

这点很关键：**换 pi 不会省掉 MCP 和持久化的工作。**

## 4. 适配层估算

如果做 TS runner：

| 模块 | 估算 |
|---|---:|
| TaskAssign / 消息 ↔ pi | 80–120 行 |
| 工具桥（local/global → AgentTool） | 120–180 行 |
| MCP TS 客户端（复用 runtime JSON-RPC） | ~100 行 |
| pi 事件 → MW 事件 | ~100 行 |
| Cancel / abort / 生命周期 | ~20 行 |
| gRPC + proto | 生成，不手写 |
| **合计** | **约 500–700 行 + 生成的 TS 客户端** |

另外还有 Node 打包、进程管理、日志、部署成本。

## 5. 冲突点

```text
1. 重试
   pi-ai 可能自带 provider retry；MW 也有 run 重试。
   必须配置成只由 MW 重试，否则会重复调用模型。

2. 工具参数
   pi 给的是对象；MW App 协议要 arguments_json 字符串。

3. 工具结果
   pi 返回 content blocks；MW 是 result_json 字符串。

4. 并行工具
   pi 默认 parallel；App 参考宿主是串行处理。
   初期应设 sequential。

5. 状态
   pi 不持久化，正好让 MW 做唯一状态源；
   但不能让 pi 的 session/queue 与 MW checkpoint 并存。

6. 事件丢失
   pi 有 thinking/partial 等内部事件，不应全部映射进 MW 契约。
```

## 6. 评估矩阵

| 维度 | 分数（1–5） | 说明 |
|---|---:|---|
| 循环成熟度 | 5 | 明显强于当前 harness |
| Provider 覆盖 | 5 | pi-ai 统一多 provider |
| 事件模型 | 5 | 结构清晰，易映射 |
| 工具执行 | 4 | hooks/并行完善；需适配 MW 协议 |
| MCP | 0 | 不提供 |
| 持久化 | 0 | 不提供 |
| 调度 | 0 | 不提供 |
| 语言/部署契合 | 2 | 引入 Node/TS 第二运行时 |
| 适配成本 | 3 | 约 500–700 行 + 双栈运维 |

## 7. 结论

**不把 pi-agent-core 作为当前 Harness 核心替换。**

理由：

```text
1. 它解决的是循环层，不是 MCP/持久化/执行。
2. 引入 TS 第二运行时，收益与运维成本不匹配。
3. 当前 Python harness 已有协议、checkpoint、MCP、82 个测试。
4. MW 才是调度与状态的所有者，换 runner 不减少这部分复杂度。
```

推荐路径：

```text
短期：保留 Python harness，吸收 pi 的事件/工具/hook 设计。
中期：如确需第二 runner，按 TaskAssign 契约做 TS pi-runner，
      用 parity 用例（P6C/P6D/P7）作为切换门槛。
长期：pi-coding-agent 不做服务端 runner（session/工具模型冲突）。
```

## 8. 顺带发现的问题

评估 provider usage 时发现：

```text
control.observeUsage 读取 input_tokens / output_tokens
OpenAI/DeepSeek provider 返回 prompt_tokens / completion_tokens
→ DeepSeek Run 的 token 指标可能为 0
```

这不是 pi 的问题，而是现有 harness/provider 的归一化缺口。
建议单独修：在 harness 或 control 统一 usage key。

## 9. 证据

```text
docs/reports/artifacts/pi-core/
  spike.mjs
  spike-output.json
  abort.mjs
  abort-output.json
  versions.txt
```

## 10. 建议下一步

```text
1. 修 usage key 归一化（独立小步）
2. 把 pi 的事件/工具接口设计写进 AgentRuntime 契约
3. 暂不实现 TS runner
```
