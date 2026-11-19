# App / Runtime 接入面简化讨论稿

> 状态：0 步骤、Runtime SDK 与 React Runtime 迁移已实施；App SDK 公开表面收敛待做。
>
> 目标不是继续增加抽象，而是把“接入方真正要写什么”压缩到最小。

---

## 0. 删掉策略定义要做的全部事情

这里的“策略”指：

```text
Hello.supported_strategies
TaskAssign.strategy
TaskAssign.strategy_params
runs.strategy
Harness Strategy / StrategyRouter
CLI --strategy
```

不包括“Runtime 名字”本身。Runtime 名字仍然存在，例如：

```text
react1
react100
echo1
```

App 端预先知道这些名字，提交 Run 时指定 `runtime_name`。  
控制面按名字找到对应 Runtime，不再经过 Strategy 分发。

### 0.1 目标协议形状

```text
Hello:
  daemon_id           进程实例 ID
  runtime_name        稳定路由名，例如 react1
  runtime_type        normal / sandbox / longterm（控制面内部使用）
  auth_token
  max_concurrent_runs
  version
  labels
  running_runs
```

```text
TaskAssign:
  run_id
  app_id
  params              Runtime 自己解释的可选参数
  input
  model               可选
  tools               可选
  mcp_bindings        可选
  timeouts
  session_id          可选
  resume_from         可选
  trace_context
```

```text
CreateRun:
  runtime_name: "react1"
  input: [...]
  params: {...}
  model: {...}        可选
  timeouts: {...}     可选
```

`runtime_type` 不再由 App 选择；App 只选择 `runtime_name`。
`normal/sandbox/longterm` 是部署和调度属性，不是业务行为选择器。

### 0.2 协议与契约

- [ ] `proto/middleware/runtime/v1/runtime.proto`
  - 删除 `Hello.supported_strategies`
  - 删除 `TaskAssign.strategy`
  - 删除 `TaskAssign.strategy_params`
  - 新增 `Hello.runtime_name`
  - `TaskAssign.params` 替代 `strategy_params`
- [ ] 重新生成 Go / Python proto 代码
- [ ] `contracts/` 与 `tests/contract/envelopes.json` 更新
- [ ] `docs/DAEMON_CONTRACT.md` 重写为“RuntimeChannel”而不是“Strategy”
- [ ] `docs/AGENT_CORE_CONTRACT.md` 明确 AgentCore 只是可选循环库
- [ ] `docs/api/openapi.yaml`
  - `CreateRunRequest` 删除 `strategy`
  - 新增 `runtime_name`
  - `model` 改为可选
  - `RunView` 删除 `strategy`
  - Admin Run 视图删除 `strategy`
  - Admin daemon 视图删除 `supported_strategies`
- [ ] `public/service-meta/v1` 删除 `builtin_strategies`
- [ ] `public/service-status/v1` 删除或替换 `daemons.strategies`
- [ ] 所有示例、fixture、golden test 更新

> Note: this step intentionally removes proto fields. `buf.yaml` temporarily excepts `FIELD_NO_DELETE`, `FIELD_SAME_NAME`, and `FIELD_SAME_JSON_NAME`; remove those exceptions after main carries the new RuntimeChannel contract.

### 0.3 HTTP / 数据库

- [ ] `internal/httpapi/contracts.go`
  - 删除 `StrategyRequest`
  - `CreateRunRequest.Strategy` 删除
  - 新增 `RuntimeName string`
  - `ModelRequest` 变为可选
- [ ] `internal/httpapi/validate.go`
  - 删除 `strategy.name required`
  - 新增 `runtime_name required`
- [ ] `internal/httpapi/runs.go`
  - 不再写入 `runs.strategy`
  - 写入 `runs.runtime_name`
  - RunView 返回 `runtime_name`
- [ ] `internal/httpapi/admin_ops.go`
  - 删除 `Strategy`
  - 删除 `SupportedStrategies`
  - 增加 `RuntimeName`
- [ ] `internal/runstore/types.go`
  - 删除 `Strategy`
  - 用 `RuntimeName` 替代；或保留 `Runtime` 但语义改为名字
- [ ] `internal/runstore/runs.go`
  - 查询/插入/扫描字段更新
- [ ] DB migration
  - 新增 `runs.runtime_name`
  - 旧数据从 `strategy` 回填，例如 `legacy-<strategy>` 或直接回填原 strategy
  - 后续版本删除 `runs.strategy`
- [ ] `internal/db/migrations` 新迁移
- [ ] 所有 runstore/httpapi 测试更新

### 0.4 调度与控制面

- [ ] `internal/connection/daemonhub.go`
  - `DaemonInfo` 增加 `RuntimeName`
  - 删除 `SupportedStrategies`
- [ ] `internal/connection/placement.go`
  - `PickHealthy` 参数从 `strategy` 改为 `runtimeName`
  - 匹配 `DaemonInfo.RuntimeName`
- [ ] `internal/control/control.go`
  - `DaemonHub.PickHealthy` 签名更新
- [ ] `internal/control/scheduler.go`
  - `PlacementRequest.Strategy` 删除
  - `Candidate.SupportedStrategies` 删除
  - 增加 `RuntimeName`
- [ ] `internal/control/schedule.go`
  - 从 Run 读取 `RuntimeName`
  - `TaskAssign` 不再设置 `strategy`
  - 设置 `params`
- [ ] 调度测试
  - 多实例同名 Runtime 负载均衡
  - 不同名字 Runtime 不互相接收
  - capacity / labels / runtime_type 过滤保留

### 0.5 CLI

- [ ] `levier run submit`
  - 删除 `--strategy`
  - 新增 `--runtime`（对应 `runtime_name`）
  - 不再默认 `react`
  - `model` 只在指定时发送
- [ ] `levier run watch`
  - 显示 `runtime_name`
- [ ] `levier ops runs`
  - 显示 `runtime_name`
- [ ] `levier daemon check`
  - `--strategy` 改为 `--runtime`
- [ ] `levier selfcheck`
  - 深度检查使用 `--runtime`
- [ ] `levier describe`
  - 删除 `strategies` 输出
- [ ] CLI 帮助、JSON、测试更新

### 0.6 Harness / Runtime

- [ ] 删除 `execution/react_runtime/src/levier_react_runtime/strategies/`
  - `base.py`
  - `echo.py`
  - `react.py`
  - `registry.py`
- [ ] `RunManager` 不再依赖 `StrategyRouter`
  - 改为持有单个 `run` 函数
- [ ] `ChannelClient`
  - Hello 发送 `runtime_name`
  - 删除 `supported_strategies`
- [ ] `config.py`
  - 删除 `daemon.strategies`
  - 新增 `runtime.name`
  - 保留 `runtime_type`、capacity、labels
- [ ] `runtime/checkpoint.py`
  - checkpoint 中的 `strategy` 改为 `runtime_name`，或删除该字段
- [ ] `runtime/run.py`
  - `RunContext` 不再暴露 `assign.strategy`
  - 暴露 `ctx.runtime_name` / `ctx.params`
- [ ] 旧 Harness 变成 **React Runtime** 实现
  - 它的 `run(ctx)` 内部使用 `agent_core`
  - 不再负责 echo
- [ ] 新增一个独立 Echo Runtime 示例
  - `run(ctx)` 直接返回
  - 用于 smoke、conformance、最小接入示例
- [ ] Harness 测试
  - 删除 strategy / router 测试
  - 增加 runtime host / run context 测试

### 0.7 E2E / Compose / 文档

- [ ] Compose 增加独立的 echo Runtime 服务或测试进程
- [ ] `mvp-demo` echo 使用 echo Runtime 名
- [ ] `mvp-demo-react` 使用 React Runtime 名
- [ ] fault matrix、MCP E2E 使用 runtime 名
- [ ] App 示例 `run submit --runtime ...`
- [ ] 所有 `strategy` 文档引用清理

---

## 1. 简化完整 App 端行为清单

### 1.1 App 作者只需要理解这些

```text
credentials.json
LevierApp.from_credentials()
@app.tool(name, description, schema)
handler(arguments: dict) -> JSON
app.run()
```

可选：

```text
ToolExecutionError
```

也就是说，App 作者不需要理解：

```text
Admin token
daemon token
MCP secret
数据库
HTTP
WebSocket
ws_token
reconnect
msg_id / invocation_id
local_tool_result
```

### 1.2 App 端公共 API 收敛

- [ ] `sdk/python/__init__.py` 只保留推荐公共接口：
  - `LevierApp`
  - `ToolExecutionError`
  - 可选 `CredentialsError`
- [ ] `LevierClient`、`Credentials`、`Secret`、`WsToken`、`save_credentials`、
  `resolve_server` 不再作为首次接入文档的一部分
- [ ] 如需提交 Run，增加高层方法或明确示例：
  - `app.submit_run(runtime_name, input, ...)`
  - 不让 App 直接拼 HTTP
- [ ] `LevierApp` 构造函数不再出现在首次接入文档
- [ ] `from_credentials()` 是唯一入口

### 1.3 App 配置收敛

首次接入只使用一个变量：

```text
LEVIER_CREDENTIALS
```

- [ ] credentials 文件保存 `server`，运行时不再要求 `LEVIER_SERVER`
- [ ] `LEVIER_APP_URL` / `LEVIER_URL` / `LEVIER_APP_*` 只作为内部兼容，不作为文档入口
- [ ] 删除 App 运行时的 Admin token 相关路径
- [ ] 删除 App 示例中的自定义 HTTP/WS/config 模块（已完成）

### 1.4 App 命令收敛

```text
levier app init
levier app check
levier app rotate-key
```

- [ ] `app init` 输出只有：
  - credentials 路径
  - app_id
  - key 指纹
  - 下一步
- [ ] `app check` 一条命令完成本地凭据 + 服务端 key + App active 检查
- [ ] `app rotate-key` 更新凭据文件，旧 key 失效
- [ ] Admin token 只出现在 Operator 文档

### 1.5 App 文档重新分层

不要把所有概念放在一篇 onboarding 里：

```text
docs/ONBOARDING_OPERATOR.md
  启动 Levier、Admin token、数据库、runtime token

docs/ONBOARDING_APP.md
  3 步接 App：app init → handlers → app.run

docs/ONBOARDING_RUNTIME.md
  runtime name、run(ctx)、可选能力
```

### 1.6 App 与控制面的最小交互

App 只做：

```text
读取凭据
注册工具（SDK 做）
接收 local_tool_call（SDK 做）
执行 handler
返回结果（SDK 做）
提交 Run（可选，高层 API）
```

控制面不应要求 App 理解：

```text
strategy / supported_strategies
runtime_type
daemon_id
event seq
lease_epoch
```

---

## 2. 简化完整 Runtime 端行为清单

### 2.1 Runtime 作者只需要写一个入口

```python
async def run(ctx: RunContext) -> str:
    ...
    return "completed"
```

- [ ] 框架接管 channel、Hello、心跳、重连、Ack、seq、Cancel、Reconcile
- [ ] Runtime 不再实现 `Strategy`
- [ ] Runtime 不再实现 `StrategyRouter`
- [ ] Runtime 不再支持多个策略字符串
- [ ] Runtime 名字就是部署名：`react1`、`react100`、`echo1`

### 2.2 `RunContext` 是唯一接口

最小字段：

```text
ctx.runtime_name
ctx.input
ctx.params
ctx.model          可选
ctx.tools          可选
ctx.mcp_bindings   可选
ctx.timeouts
ctx.session_id     可选
ctx.resume_from    可选
ctx.cancelled
```

最小方法：

```text
await ctx.reply(text)
await ctx.emit(type, data)
await ctx.call_tool(name, args, timeout_ms)
await ctx.checkpoint(state)
```

- [ ] 如果 Runtime 不调用 `call_tool`，就不会处理工具
- [ ] 如果 Runtime 不读取 `mcp_bindings`，就不会处理 MCP
- [ ] 如果 Runtime 不读取 `session_id`，就不支持 longterm
- [ ] 不需要写“空实现”或“占位接口”

### 2.3 Runtime 框架

- [ ] 新建 `levier_runtime` 包（名称暂定）
  - channel
  - runtime host
  - run context
  - event seq
  - cancellation
  - reconnection
- [ ] `levier_runtime` 不包含 LLM、ReAct、MCP、Tool 路由
- [ ] `agent_core` 改为可选循环库
  - React Runtime 可以使用它
  - Echo Runtime 不使用它
  - 不再把它称为 Runtime Core

### 2.4 Runtime 配置

```yaml
runtime:
  name: react1
  type: normal
  max_concurrent_runs: 4
  labels:
    pool: local
```

- [ ] 删除 `daemon.strategies`
- [ ] 删除 `supported_strategies`
- [ ] 保留 `runtime_type` 作为控制面/部署属性
- [ ] `runtime_name` 作为 App 路由键
- [ ] 同名 Runtime 可以有多个 daemon 实例，按 capacity 负载均衡

### 2.5 最小 Runtime 清单

Echo：

```python
async def run(ctx):
    await ctx.reply("echo")
    return "completed"
```

React：

```python
async def run(ctx):
    # 自己的 Agent 循环
    # 可以使用 ctx.model、ctx.call_tool
    return "completed"
```

Longterm：

```python
async def run(ctx):
    if ctx.resume_from:
        ...
    await ctx.checkpoint(...)
    return "completed"
```

不需要为了 Echo 创建 Strategy，也不需要为了 React 创建 Router。

### 2.6 Runtime 接入流程

```text
1. 选择/设置 runtime_name
2. 写 run(ctx)
3. 配置 runtime token
4. 启动 Runtime
5. 用最小 Run 验证
6. 再按需实现 tool / mcp / checkpoint
```

Runtime 作者不需要先理解：

```text
Strategy
StrategyRouter
AgentCoreRunSpec
ProviderAdapter
ToolAdapter
```

### 2.7 Runtime 文档重新组织

```text
docs/RUNTIME_ONBOARDING.md
  5 分钟写一个 echo runtime

docs/RUNTIME_CONTEXT.md
  RunContext 字段与方法

docs/RUNTIME_CAPABILITIES.md
  tools / MCP / checkpoint 可选能力

docs/RUNTIME_CONFORMANCE.md
  如何验证一个 Runtime
```

---

## 3. 推荐实施顺序

```text
第一步：冻结 runtime_name / run(ctx) / 删除 strategy 的目标契约
第二步：先写独立 Echo Runtime 验证 run(ctx)
第三步：把现有 React Harness 改成单个 React Runtime
第四步：控制面切换 runtime_name 调度，保留短期兼容
第五步：收敛 App SDK 公共表面
第六步：清理文档、E2E、Compose、metrics、status
```

不建议：

```text
先改一堆控制面字段
先做 TS SDK
先做 runtime 运维面
先做 Sidecar
```

---

## 4. 一句话判断标准

一个完全没看过仓库的新人，应该能回答：

### App

```text
我只需要 credential 文件、几个 handler、app.run()。
```

### Runtime

```text
我只需要 runtime_name、run(ctx) 和可选能力。
```

如果他仍然需要理解：

```text
strategy
supported_strategies
strategy_params
AgentCore
ProviderAdapter
ToolAdapter
ws_token
```

说明这层还没有简化完成。

---

## 5. 需要先确认的四个决定

1. **路由字段名**
   - 推荐：`runtime_name`
   - 不复用当前含义为 `normal/sandbox/longterm` 的 `runtime` 字段
   - App 只选择 runtime name；`runtime_type` 留在控制面内部

2. **Echo 是否独立 Runtime**
   - 推荐：独立 `echo` Runtime，用于 smoke 和最小接入示例
   - 不再让同一个 React Harness 通过 Strategy 切换 echo

3. **`agent_core` 的定位**
   - 推荐：可选 loop helper，不叫 Runtime Core
   - Runtime 框架只规定 `run(ctx)`

4. **兼容策略**
   - 推荐：内部先删除 Strategy 类；协议字段先加 `runtime_name`，
     `strategy` 保留一个版本的兼容别名，之后再删除
   - 不建议一次性同时改所有外部字段和内部实现
