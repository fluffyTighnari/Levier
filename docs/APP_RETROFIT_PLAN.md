<!-- Middleware — App retrofit debug plan. -->

# Counter App 改造调试计划

| 项目 | 内容 |
|---|---|
| 状态 | 计划，待执行 |
| 基线 | `examples/counter_agent/`（0 依赖 Python Agent） |
| 目标产物 | 零依赖 Node App + 真实 Harness + Middleware 全链路 |
| 原则 | 一次只验证一条逻辑边，失败即停 |

## 1. 目标

把“已经能跑的 Counter Agent”改造成 Middleware App，并逐边验证：

1. 计数器逻辑与工具分发正确。
2. App 注册信息真实落盘。
3. 注册信息能进入 `TaskAssign`。
4. App WS 能接收并返回 local tool 调用。
5. Harness 能用 DeepSeek 跑 ReAct。
6. 最终行为与 0 依赖基线一致。
7. HTTP/WS 均能由非 Go 客户端完成。

## 2. 非目标

- 不在本计划中做压测、长时间稳定性、多实例。
- 不实现生产级模型凭据代理；实验期使用 Harness 环境变量。
- 不重写 Middleware 核心架构。
- 不把 fakedaemon 作为主 E2E。

## 3. 组件与边

用单个字母表示组件，每一步只测一条边：

| 代号 | 组件 |
|---|---|
| L | Counter 纯逻辑（add/get/reset） |
| T | Tool dispatcher（name + arguments → result/error） |
| M | Tool manifest（工具名/描述/JSON Schema） |
| H | App HTTP client（注册/提交/查询/取消） |
| W | App WebSocket client（连接/帧/工具结果） |
| API | Middleware HTTP API |
| REG | Register + PostgreSQL（apps/tool_defs） |
| CTL | Control/TaskAssign 组装 |
| GW | Gateway/Invoker/AppHub |
| HAR | Harness daemon |
| LLM | DeepSeek provider |
| EV | run_events / WS replay |
| OBS | metrics/logs/trace |

示例：

- `L→L`：纯函数自测。
- `T→L`：dispatcher 调 counter。
- `H→API`：HTTP 注册。
- `API→REG`：注册落库。
- `REG→CTL`：工具进入 TaskAssign。
- `HAR→GW→W→GW→HAR`：local tool 完整闭环。

## 4. 执行纪律

1. **一步一条边**。不要把注册、WS、LLM 放在同一步验证。
2. **失败即停**。当前步骤断言失败，不进入下一步。
3. **每步独立证据**。输出到：

```text
docs/reports/artifacts/retrofit/<step-id>/
  request.json
  response.json
  db.txt
  logs.txt
  summary.txt
```

4. **真实 Harness 为主**。fakedaemon 只用于观察 `TaskAssign` 或注入故障。
5. **每步可重跑**。使用唯一 App 名/Run ID；不依赖上次内存状态。
6. **每步先写断言，再执行**。
7. **每步记录时间、commit、trace_id**。

## 5. 环境与命名

```text
App 名：retrofit-counter-<ulid>
工具名：counter_add / counter_get / counter_reset
Run：由 API 返回
DB：PostgreSQL :5433
MW：:8080 / :9090
Harness：daemon-local-1
```

模型（实验期）：

```text
DEEPSEEK_API_KEY  ← 仓库 .env
DEEPSEEK_BASE_URL ← https://api.deepseek.com
DEEPSEEK_MODEL    ← deepseek-flash
```

> 实验期由 Harness 读取环境变量；不写入 `runs.model_json`，不进入日志。
> 生产级 MW 凭据注入作为后续硬化项，不在本计划内。

## 6. 阶段总览

| 阶段 | 内容 | 退出条件 |
|---|---|---|
| P0 | 基线冻结与环境 | 基线输出可复现 |
| P1 | Counter 本地逻辑 | L/T/M 单测通过 |
| P2 | App 壳与协议适配 | 本地 mock 通过 |
| P3 | 注册与落库 | API + DB 双向确认 |
| P4 | 注册 → TaskAssign | Harness 看到工具 |
| P5 | WS 传输 | token/帧/事件闭环 |
| P6 | local tool 闭环 | counter 结果与基线一致 |
| P7 | 持久化与观测 | DB/日志/指标可核对 |
| P8 | 语言无关 | Python HTTP + Node WS |
| P9 | 负向与故障 | 指定错误码/恢复 |
| P10 | 回归与收口 | 一键脚本可重复 |

---

## P0 基线冻结与环境

| ID | 逻辑链 | 操作 | 断言 | 证据 |
|---|---|---|---|---|
| P0.1 | 基线→基线 | `make counter-agent` | 输出 3 次工具调用，final=5 | `counter.log` |
| P0.2 | 环境→健康 | 启动 PostgreSQL、迁移、启动 MW | `/healthz` 200；`/metrics` 可读 | `health.json`、`metrics.txt` |
| P0.3 | Harness→MW | 启动 echo Harness | Hello 成功；daemon online；无 Auth 错误 | `harness.log` |
| P0.4 | 测试隔离 | 记录 commit、时间、DB URL 指纹 | 不包含密钥 | `env.txt` |

**退出**：P0.1 的 final=5 与 tool call 次数成为后续对照基准。

---

## P1 Counter 本地逻辑

| ID | 逻辑链 | 代码/操作 | 断言 | 层级 |
|---|---|---|---|---|
| P1.1 | L→L | 抽出 `counterAdd/Get/Reset` 纯函数 | add(2)=2；add(3)=5；reset=0 | unit |
| P1.2 | T→L | dispatcher `counter_add` | 正确解析 `delta` 并调用 L | unit |
| P1.3 | T→L | dispatcher `counter_get` | 返回当前值，不改变状态 | unit |
| P1.4 | T→L | dispatcher `counter_reset` | 归零 | unit |
| P1.5 | T→错误 | 未知工具 | 返回 `unknown tool`，不 panic | unit |
| P1.6 | T→错误 | 非法 JSON / 缺 delta | 返回结构化错误 | unit |
| P1.7 | M→M | 生成三个工具 manifest | name/description/schema 合法 JSON | unit |
| P1.8 | L→基线 | 用同一提示词再跑 standalone | final=5，仍调用 3 次工具 | integration |

**注意**：P1 不接触 MW、不接触网络。

---

## P2 App 壳与协议适配

目标：先让 App 具备 HTTP/WS 能力，但只对本地 mock 验证。

| ID | 逻辑链 | 代码/操作 | 断言 | 层级 |
|---|---|---|---|---|
| P2.1 | 配置→内存 | 读取 `MW_URL`、`MW_ADMIN_TOKEN`、`DEEPSEEK_*` | 缺失时明确报错；不打印值 | unit |
| P2.2 | H→mock | 构造 `/admin/apps` 请求 | method/header/body 正确 | unit |
| P2.3 | mock→H | 解析 App 响应 | app_id/api_key 正确；key 不进日志 | unit |
| P2.4 | H→mock | 构造工具注册请求 | schema/timeout 正确 | unit |
| P2.5 | H→mock | 构造 `/runs` 请求 | 字段与 `CreateRunRequest` 一致 | unit |
| P2.6 | mock→H | 解析 run/events/error envelope | request_id、code、message 正确 | unit |
| P2.7 | W→mock | WS 握手 URL 与 `app_id/token` | 连接成功 | unit |
| P2.8 | mock→W | 解析 `local_tool_call` | 提取 msg_id/invocation_id/name/args | unit |
| P2.9 | W→mock | 构造 `local_tool_result` | correlation=msg_id；ok/error 分支正确 | unit |
| P2.10 | W→mock | ping/pong | 收到 pong | unit |

**决策**：App 使用 Node 24 内置 `fetch` + `WebSocket`，零 npm 依赖。
理由：同时完成 WS 语言无关验证；若 Node WebSocket 不支持所需能力，回退 Python 标准库最小 WS 客户端。

---

## P3 注册与真实落库

这一阶段对应“写注册信息并测试链路”。

| ID | 逻辑链 | 操作 | 断言 | 证据 |
|---|---|---|---|---|
| P3.1 | H→API | Admin 创建 App（无工具） | 201/200；返回 app_id；api_key 仅首次 | `create_app.json` |
| P3.2 | API→REG | 查 `apps` | 行存在；`api_key_hash` 非明文；status=active | `db_apps.txt` |
| P3.3 | H→API | App key `GET /apps/{id}` | 200；字段匹配 | `get_app.json` |
| P3.4 | H→API | 错误 key 查询 | 401 统一错误信封 | `auth_fail.json` |
| P3.5 | H→API | App 更新写入 `counter_add` | 200 | `register_tool.json` |
| P3.6 | API→REG | 查 `tool_defs` | scope=local、app_id、name、schema、timeout、version=1 | `db_tool_1.txt` |
| P3.7 | H→API | 重复写同一工具 | 200；version+1；仍只有一行 | `db_tool_2.txt` |
| P3.8 | API→REG | 写 `counter_get/reset` | 三个工具都存在 | `db_tool_3.txt` |
| P3.9 | H→API | 替换为只保留 `counter_add` | 旧工具被删除 | `db_tool_prune.txt` |
| P3.10 | API→重启 | 重启 MW 后查 DB + GET App | 工具仍存在；App key 仍可用 | `restart.txt` |
| P3.11 | App2→API | 第二个 App 查询/尝试使用 | 看不到第一个 App 的工具 | `cross_app.txt` |

**停止条件**：P3.6 不通过，不进入 P4。

---

## P4 注册 → TaskAssign

| ID | 逻辑链 | 操作 | 断言 | 证据 |
|---|---|---|---|---|
| P4.1 | HAR→MW | 启动真实 Harness echo | Hello/Heartbeat 正常 | `harness_hello.log` |
| P4.2 | HAR log | 在 Harness 增加 TaskAssign 工具日志 | 日志含 run_id、工具名列表 | `task_assign.log` |
| P4.3 | REG→CTL | 创建 echo Run | TaskAssign 到达 Harness；工具列表包含已注册工具 | `task_assign.json` |
| P4.4 | REG→CTL | 校验工具 schema | Harness 日志中的 schema 与 DB 一致 | `schema_compare.txt` |
| P4.5 | App2→CTL | App2 创建 Run | TaskAssign 不含 App1 的工具 | `app2_tools.txt` |
| P4.6 | 旧状态→CTL | 删除工具后新建 Run | TaskAssign 不含已删除工具 | `pruned_tools.txt` |

---

## P5 WS 传输

| ID | 逻辑链 | 操作 | 断言 | 证据 |
|---|---|---|---|---|
| P5.1 | H→API | `POST /ws-token` | 200；expires_in=600 | `ws_token.json` |
| P5.2 | W→API | 建连 | 101/连接成功；`mw_ws_connections` +1 | `ws_connect.log` |
| P5.3 | W→API | `ping` | 收到 `pong` | `ping.log` |
| P5.4 | W→EV | `subscribe` 空 Run | 收到 `caught_up`，seq=0 | `caught_up.log` |
| P5.5 | EV→W | 创建 echo Run | 收到 `RUN_STATUS/MESSAGE_COMPLETED/RUN_STATUS` | `events.log` |
| P5.6 | EV→W | 带 cursor 重连 | 只收到 cursor 之后的事件；无重无漏 | `replay.log` |
| P5.7 | 错误 token | 过期/跨 App token | 连接被拒绝 | `token_reject.log` |

---

## P6 local tool 闭环

### P6A 模型提供方（先独立测）

| ID | 逻辑链 | 操作 | 断言 |
|---|---|---|---|
| P6A.1 | LLM→mock | 新增 OpenAI-compatible provider | 非流式 JSON 解析正确 |
| P6A.2 | LLM→mock | SSE 流解析 | 文本 delta 与 tool_calls 正确 |
| P6A.3 | LLM→错误 | 401/429/5xx/超时 | 映射为 `AUTH_FAILED/RATE_LIMITED/PROVIDER_UNAVAILABLE/TIMEOUT` |
| P6A.4 | LLM→Harness | provider 路由 | `provider=deepseek` 选 DeepSeek；`anthropic` 保持原路径 |
| P6A.5 | 配置→LLM | 读取 `DEEPSEEK_API_KEY` | 缺失或空时明确失败；日志不含 key |

### P6B 工具往返

| ID | 逻辑链 | 操作 | 断言 | 证据 |
|---|---|---|---|---|
| P6.1 | HAR→CTL | react Run：提示“调用 counter_get” | TaskAssign 含工具 | `assign_react.txt` |
| P6.2 | HAR→LLM | DeepSeek 返回 tool_call | Harness 发出 ToolCallRequest | `llm_tool_call.log` |
| P6.3 | GW→W | App 收到 `local_tool_call` | invocation_id/name/args 正确 | `local_call.json` |
| P6.4 | W→GW | App 返回 result | correlation=msg_id；Gateway 收到 | `local_result.json` |
| P6.5 | GW→HAR | Harness 收到 ToolCallResult | tool result 进入下一轮模型输入 | `tool_result.log` |
| P6.6 | GW→REG | 查 `tool_invocations` | status=completed；result_json 正确 | `db_invocation.txt` |
| P6.7 | HAR→EV | 工具事件 | 有 TOOL_CALL_BEGIN/END | `tool_events.json` |
| P6.8 | HAR→终态 | 模型最终回答 | Run terminal=completed | `run.json` |

### P6C 与基线对照

| ID | 检查 | 断言 |
|---|---|---|
| P6.9 | final counter | 与 P0.1 相同 |
| P6.10 | tool call 次数 | 与 P0.1 相同 |
| P6.11 | 调用顺序 | add/add/get 一致 |

### P6D 负向

| ID | 场景 | 断言 |
|---|---|---|
| P6.12 | 重复 local_tool_result | 第二次不重复投递 |
| P6.13 | correlation 错误 | `LOCAL_TOOL_CORRELATION_MISMATCH`，pending 保留 |
| P6.14 | 未知 invocation | `LOCAL_TOOL_UNKNOWN` |
| P6.15 | 其他 App 伪造 | `LOCAL_TOOL_FORBIDDEN` |
| P6.16 | App 断线 | pending 失败 `APP_UNREACHABLE` 或超时 |
| P6.17 | 工具超时 | `TOOL_TIMEOUT`，Run 按策略失败/继续 |

---

## P7 持久化与观测

用户要求“查看注册信息是否真实落盘”，这里做最终核对。

| ID | 检查 | 断言 | 证据 |
|---|---|---|---|
| P7.1 | `apps` | app_id/name/status/hash 正确 | `db_apps.txt` |
| P7.2 | `tool_defs` | 三个工具，scope/app_id/version 正确 | `db_tools.txt` |
| P7.3 | `runs` | runtime/strategy/status/trace_id 正确 | `db_runs.txt` |
| P7.4 | `run_events` | seq 连续；TOOL_CALL_BEGIN/END 存在 | `db_events.txt` |
| P7.5 | `run_tasks` | RUN_DISPATCH/RUN_TIMEOUT 为 DONE/CANCELLED | `db_tasks.txt` |
| P7.6 | `tool_invocations` | 每个 invocation 一行；completed | `db_invocations.txt` |
| P7.7 | trace 链 | HTTP `X-Request-ID` = `runs.trace_id` = TaskAssign/log | `trace.txt` |
| P7.8 | 指标 | runs/tool/ws/event 指标与运行结果一致 | `metrics_before.txt`、`metrics_after.txt` |
| P7.9 | 脱敏 | 日志/指标/证据无 key/token/secret | `redaction.txt` |
| P7.10 | key rotate | 旧 key 401；新 key 可用；工具仍在 | `rotate.txt` |
| P7.11 | MW 重启 | 注册信息与 Run 事件仍在；无重复 | `mw_restart.txt` |

---

## P8 语言无关

| ID | 逻辑链 | 操作 | 断言 |
|---|---|---|---|
| P8.1 | Python→API | 运行 `make http-smoke` | 全流程 200/404 预期通过 |
| P8.2 | Node→WS | Node App 连接 MW | 无 npm 依赖；握手成功 |
| P8.3 | Node→WS→API | Node App 处理 local_tool_call | 返回结果，Harness 继续 |
| P8.4 | 代码检查 | Node App 不 import MW SDK/生成代码 | 无 Go/Proto 依赖 |
| P8.5 | 协议检查 | 字段名与 DESIGN 一致 | msg_id/invocation_id/correlation_id 对齐 |

**语言矩阵**：

```text
Middleware: Go
Harness: Python
App baseline: Python stdlib
App retrofit: Node stdlib
HTTP smoke: Python stdlib
```

---

## P9 故障与恢复

每条故障单独执行，不与其他故障叠加。

| ID | 故障 | 期望 |
|---|---|---|
| P9.1 | App WS 断开 | Run 继续；pending tool 失败；重连补拉 |
| P9.2 | 重复 local_tool_result | 幂等 |
| P9.3 | 错误 correlation | 拒绝，pending 保留 |
| P9.4 | 伪造 App | 拒绝 |
| P9.5 | 工具超时 | `TOOL_TIMEOUT` |
| P9.6 | LLM 429/5xx | Harness 重试或失败，事件可解释 |
| P9.7 | Daemon 断连 | Running Run requeue；epoch fencing |
| P9.8 | MW 重启 | queued/assigned 不丢；running 对账收敛 |
| P9.9 | Queue 不可用 | `POST /runs` 503（如实现）或明确 500 |
| P9.10 | 慢消费者 | 不阻塞 Control；指标/日志可见 |

---

## P10 回归与收口

| ID | 内容 | 退出条件 |
|---|---|---|
| P10.1 | Go 单测/集成 | `MIDDLEWARE_TEST_POSTGRES=1 go test ./...` |
| P10.2 | Harness 测试 | `make harness-test` |
| P10.3 | MCP Runtime 测试 | `make mcp-runtime-test` |
| P10.4 | 一键脚本 | `make retrofit-e2e`（新增） |
| P10.5 | 证据索引 | 每个 P 步骤有目录与 summary |
| P10.6 | 报告 | `docs/reports/RETROFIT-E2E.md` |

`make retrofit-e2e` 必须：

1. 启动 DB/MW/Harness/App。
2. 注册 App/工具。
3. 跑同一提示词。
4. 校验 counter=5、工具次数=3、事件/DB/指标。
5. 清理进程。

---

## 7. 执行顺序建议

严格按 P0 → P10。

其中最容易“跳步”的地方：

- P3 未确认 DB 就去做 WS。
- P4 未确认 TaskAssign 就去做 LLM。
- P6 未单测 provider 就直接跑真实 DeepSeek。
- P7 只看 HTTP 200，不看 DB。

每一步只回答一个问题：

```text
这一步的输入是什么？
经过哪条边？
期望看到什么？
DB/日志/指标各看到什么？
失败时是哪一端的问题？
```

## 8. 模型凭据策略

实验期：

- Harness 从 `DEEPSEEK_API_KEY` 读取。
- 不写入 `runs.model_json`。
- 不写日志。
- 只用于本地调试。

后续生产硬化（不在本计划内）：

- MW 增加 model provider 配置。
- `AssembleTaskMaterials` 注入凭据。
- TaskAssign 只在受控通道传递。
- 审计与轮换。

## 9. 风险与回退

| 风险 | 影响 | 回退 |
|---|---|---|
| Node WebSocket 能力不足 | P2/P8 阻塞 | 改用 Python 标准库最小 WS 客户端 |
| DeepSeek provider 兼容性 | P6 阻塞 | 先用 fake transport 单测；再切真实 API |
| App 多连接广播 | 工具重复执行 | 单连接运行；App 侧幂等 |
| 注册接口无工具读取 API | 只能查 DB/TaskAssign | 明确以 DB + Harness 日志为准 |
| real Harness 不稳定 | E2E 抖动 | 每步保留 fake 观察探针，但不替代主链路 |
| 模型 key 泄露 | 安全事故 | 只读 env；日志/证据脱敏；rotate |
| 测试数据污染 | 断言不稳 | 唯一 App 名；必要时专用 DB/schema |

## 10. 完成定义

- Counter App 在 Middleware 上完成一次 ReAct Run。
- 最终 counter 与 0 依赖基线一致。
- 工具调用次数与顺序一致。
- `apps/tool_defs/runs/run_events/run_tasks/tool_invocations` 均可核对。
- trace/request_id 贯通。
- Python HTTP 客户端与 Node WS App 均通过。
- 每一步有独立证据与失败定位。
- 无密钥进入日志、指标、证据或数据库明文。

---

## 11. 附录：固定请求与 SQL

### 11.1 Admin 创建 App

```http
POST /admin/apps
Authorization: Bearer <admin_token>
Content-Type: application/json
```

```json
{
  "name": "retrofit-counter",
  "multiagent": false
}
```

返回后立即记录 `app_id`；`api_key` 只返回一次。

### 11.2 App 注册工具

```http
POST /api/v1/apps
Authorization: Bearer <app_api_key>
Content-Type: application/json
```

```json
{
  "name": "retrofit-counter",
  "multiagent": false,
  "tools": [
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
    },
    {
      "name": "counter_get",
      "description": "Read the current counter value.",
      "schema": {
        "type": "object",
        "properties": {},
        "additionalProperties": false
      },
      "timeout_ms": 10000
    },
    {
      "name": "counter_reset",
      "description": "Reset the counter to zero.",
      "schema": {
        "type": "object",
        "properties": {},
        "additionalProperties": false
      },
      "timeout_ms": 10000
    }
  ]
}
```

### 11.3 创建 React Run

```http
POST /api/v1/runs
Authorization: Bearer <app_api_key>
Content-Type: application/json
```

```json
{
  "runtime": "normal",
  "strategy": {
    "name": "react",
    "params": {"max_steps": 8}
  },
  "input": [
    {
      "role": "user",
      "content": "Add 2, then add 3, then read the counter and tell me the final value. Use the counter tools; do not calculate it yourself."
    }
  ],
  "model": {
    "provider": "deepseek",
    "model": "deepseek-flash"
  },
  "timeouts": {
    "run_ms": 60000,
    "tool_ms": 10000
  }
}
```

### 11.4 WS 消息

连接：

```text
GET /api/v1/ws?app_id=<app_id>&token=<ws_token>
```

服务端 local tool call：

```json
{
  "type": "local_tool_call",
  "msg_id": "01...",
  "invocation_id": "01...",
  "tool_name": "counter_add",
  "arguments_json": "{\"delta\":2}",
  "timeout_ms": 10000
}
```

客户端结果：

```json
{
  "type": "local_tool_result",
  "correlation_id": "01...",
  "invocation_id": "01...",
  "ok": true,
  "result_json": "{\"value\":2}"
}
```

### 11.5 SQL 核对

```sql
-- App 注册
SELECT app_id, name, multiagent, status, api_key_hash
FROM apps
WHERE name = 'retrofit-counter';

-- 工具注册
SELECT tool_id, scope, app_id, name, description, schema_json,
       timeout_ms, version, updated_at
FROM tool_defs
WHERE app_id = '<app_id>'
ORDER BY name;

-- Run
SELECT run_id, app_id, runtime, strategy, status, attempt,
       trace_id, last_event_attempt, last_event_seq
FROM runs
WHERE run_id = '<run_id>';

-- 事件
SELECT attempt, seq, source, type, payload, ts_ms
FROM run_events
WHERE run_id = '<run_id>'
ORDER BY attempt, seq;

-- 工具调用
SELECT invocation_id, tool_name, status, result_json, error_json,
       created_at, completed_at
FROM tool_invocations
WHERE run_id = '<run_id>'
ORDER BY created_at;

-- 任务
SELECT task_id, task_type, status, attempt, max_attempts,
       lease_owner, lease_expires_at, updated_at
FROM run_tasks
WHERE run_id = '<run_id>'
ORDER BY created_at;
```

### 11.6 指标核对

```sh
curl -fsS http://127.0.0.1:8080/metrics | grep -E \
'mw_runs_total|mw_tool_invocations_total|mw_tool_duration_seconds|mw_ws_connections|mw_events_total'
```

期望至少出现：

```text
mw_runs_total{runtime="normal",status="completed"} 1
mw_tool_invocations_total{scope="local",status="ok",tool="counter_add"} 2
mw_tool_invocations_total{scope="local",status="ok",tool="counter_get"} 1
```

具体次数以 P0 基线为准。

### 11.7 证据模板

```text
docs/reports/artifacts/retrofit/P6.5/
  assign.json
  local_tool_call.json
  local_tool_result.json
  db_tool_invocations.txt
  run_events.json
  metrics.txt
  summary.txt
```

`summary.txt`：

```text
step: P6.5
commit: <sha>
time: <UTC>
app_id: <id>
run_id: <id>
trace_id: <id>
expected: tool result reaches Harness
observed: ok
evidence: local_tool_result.json, tool_result.log
```
