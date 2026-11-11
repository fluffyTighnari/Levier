<!-- Middleware — retrofit latency measurement. -->

# Retrofit 延迟测量：工具往返与全链路

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-11 |
| 范围 | P6B 链路（App 本地工具 + MW + Harness + DeepSeek） |
| 状态 | 完成 |
| 代码提交 | `f8e24a1`（仅 benchmark） |
| 结论 | 工具往返 p50 ≈ 3 ms；相对 LLM 的 1.5–4.6 s 可忽略 |

## 1. 测什么

被测对象是 P6B 实际跑的那条链：

```text
Harness 发起 ToolCallRequest
  → gRPC
  → Control.HandleToolCall
  → Gateway.Invoker（幂等 + tool_invocations 两次写）
  → AppHub
  → WebSocket
  → App 执行 counter_add
  → local_tool_result 原路返回
```

分四层测量，避免把 LLM 时间混进来：

| 层 | 测法 | 包含 |
|---|---|---|
| App 本地执行 | 20 万次 `dispatch_counter_tool` | 纯工具计算 + 参数解析 |
| WS 传输 | 200 次 `ping→pong` | App↔MW 消息循环，无 DB |
| DB 幂等写 | Go benchmark `Begin+Complete` | PostgreSQL 两条语句 |
| 工具往返 | 真实 Run 的 `TOOL_CALL_END.duration_ms` | gRPC + Gateway + DB + WS + App |
| 全链路 | `runs`/`run_events` 时间戳 | 排队 + LLM + 工具 + 终态 |

## 2. 环境

```text
Apple M1 / macOS
PostgreSQL 16，本地 127.0.0.1:5433
Middleware：单进程，HTTP :8080 + gRPC :9090
Harness：Python，DeepSeek deepseek-flash
传输：明文 HTTP/WS，无 TLS
```

单机、无网络抖动，数字是**下限**，不是生产值。

## 3. 结果

### 3.1 App 本地执行

```text
count    = 200000
p50      = 5.4 µs
p90      = 7.5 µs
p99      = 11.5 µs
mean     = 6.3 µs
```

工具语义本身可以忽略不计。

### 3.2 WebSocket 传输

```text
count = 200
p50   = 0.092 ms
p90   = 0.105 ms
p99   = 0.232 ms
max   = 0.409 ms
握手：token 8.9 ms，WS connect 0.46 ms
```

App↔MW 的消息循环本身约 0.1 ms。

### 3.3 数据库幂等写

`Begin`（INSERT … ON CONFLICT）+ `Complete`（UPDATE）：

```text
300 iterations × 3
215 µs/op
225 µs/op
252 µs/op
```

每次工具调用约 **0.22–0.25 ms** 的 DB 成本。

### 3.4 工具往返（真实链路）

两次 Run，各 10 次 `counter_add`，共 20 个样本：

```text
count = 20
min   = 1 ms
p50   = 3 ms
p90   = 6 ms
p99   = 14 ms
mean  = 3.65 ms
```

样本分布：

```text
1,1,1,1,2,2,2,2,2,3,3,3,4,4,4,5,5,6,8,14
```

第一次调用偏高（14/6 ms），之后稳定在 1–5 ms。

`tool_invocations` 行从 `Begin` 到 `Complete` 的墙钟：

```text
p50 ≈ 1.3 ms
max ≈ 4.7 ms
```

这 1.3 ms 覆盖了「MW 等待 App 回包」的全过程，说明 WS + App + 回包解析都在毫秒级。

### 3.5 一次 Run 内 10 次调用的总耗时

```text
Run 1：首个 TOOL_CALL_BEGIN → 最后一个 TOOL_CALL_END = 46 ms
Run 2：39 ms
```

平均每次调用间隔约 3.9–4.6 ms，与 p50 一致。

### 3.6 全链路

| 阶段 | Run 1 | Run 2 |
|---|---|---|
| 提交 → 被调度（queue） | 198.3 ms | 145.8 ms |
| 分配 → Harness running | 2.9 ms | 0.5 ms |
| running → 第一次模型返回（含 10 个 tool_calls） | 4617 ms | 1506 ms |
| 第一次返回 → 带工具结果的第二次返回 | 817 ms | 817 ms |
| 最后一次返回 → completed | 0 ms | 0 ms |
| **Run 总耗时** | **5635 ms** | **2469 ms** |

## 4. 归因

以 p50 = 3 ms 为例：

```text
App 计算          0.005 ms   ≈ 0.2%
WebSocket 传输    0.09  ms   ≈ 3%
DB Begin+Complete 0.23  ms   ≈ 8%
其余（gRPC、Gateway 路由、事件写入、调度） ≈ 2.7 ms
```

`tool_invocations` 行生命周期 1.3 ms 说明：真正花在“等 App 回包”的时间约 1.3 ms，
另外约 1.7 ms 花在 gRPC 往返与两侧调度上。

按一次典型 Run（10 次工具调用）计：

```text
工具路径总计     约 40 ms
Run 总耗时       约 2500–5600 ms
工具占比         0.7% – 1.6%
LLM 占比         98% 以上
```

## 5. 结论

1. **Middleware 注入的固定延迟约 2–4 ms/次工具调用**，其中 DB 只占 0.25 ms。
2. 相对 DeepSeek 单步 0.8–4.6 s，工具路径可以忽略。
3. 首次调用有 6–14 ms 冷启动（连接/连接池/预热），之后稳定。
4. 排队延迟 146–198 ms 来自 `workflow.poll_interval: 200ms`，是配置值，不是链路成本。
5. WS 与 App 执行都不是瓶颈；瓶颈在 gRPC 往返与两侧事件循环调度。

## 6. 可优化项（按收益排序）

| 项 | 现状 | 可能收益 |
|---|---|---|
| `workflow.poll_interval` | 200 ms | 首步启动延迟降到 ~10 ms（有 DB 轮询压力） |
| gRPC 小消息往返 | 单次约 1–2 ms 残差 | 批量/长连接复用已存在，收益有限 |
| 事件逐条写 DB | 每事件一次 INSERT | 工具密集场景可合并 |
| 首次调用冷启动 | 6–14 ms | 预热连接池/HTTP keepalive |

不建议为了这 3 ms 改架构；LLM 才是数量级更大的项。

## 7. 局限

- 单机、本地 PG、无 TLS，不代表跨机/生产网络。
- 20 个 E2E 样本，`duration_ms` 为毫秒精度，亚毫秒结论来自独立 benchmark。
- ReAct 串行执行工具；并发工具的争用未测。
- 未测 sandbox/longterm runtime、MCP 工具、跨 App 并发。
- DeepSeek 延迟受网络与模型负载影响，两次 Run 差异明显（4.6 s vs 1.5 s 首步）。

## 8. 证据

```text
docs/reports/artifacts/retrofit/latency/
  probe_ws_rtt.py        WS RTT 探针
  ws_rtt.json            200 次 ping/pong
  dispatch_bench.json    App 本地 20 万次
  db_bench.txt           Begin+Complete benchmark
  db_latency.txt         Run 内工具耗时与会话行
  latency_stats.json     汇总统计与阶段耗时
  tool_host_1.log        第一次 Run 的 10 次执行
  tool_host_2.log        第二次 Run
  submit_run_1.json
  submit_run_2.json
  middleware.log
  harness.log
  migrate.log
```

benchmark 代码：

```text
internal/gateway/invocation_store_bench_test.go
```
