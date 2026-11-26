# I3 执行清单：多副本控制面

> 状态：Complete（2026-11-26）  
> 分支：`feat/i3-multireplica`  
> 目标：控制面、Runtime 连接和 App 连接可多副本，同时保持 PostgreSQL 单一事实来源。

## 1. 目标

I3 解决四个问题：

- API / Control 可以多副本运行；
- Runtime daemon 连接不再绑死在一个进程；
- App WebSocket 连接不再绑死在一个进程；
- 同一个 local tool invocation 在多个 App 副本之间只执行一次。

I3 不做：

- Redis / NATS；
- Sandbox；
- 对象存储；
- 物理租户隔离；
- 多区域；
- 完整计费；
- exactly-once。

> R1（Runtime 接入层收敛）是 I3 后置工作流，不阻塞 I3 退出，详见第 11 节。默认在 I3.6 退出后启动，I4 sandbox 设计/落地前必须完成。

## 2. 已确认决策

| 决策 | 结论 |
|---|---|
| 进程模型 | 一个代码库，按角色启动：control/api、runtime-gateway、app-gateway |
| 本地开发 | 允许单进程同时运行全部角色 |
| 状态源 | PostgreSQL；通知只负责唤醒 |
| 连接归属 | 数据库 lease + epoch |
| Runtime 路由 | 任意副本解析活跃 Runtime Gateway 后定向发送 |
| App 路由 | 任意副本解析活跃 App Gateway 后定向发送 |
| Local tool | 定向投递，禁止广播执行 |
| App 协议 | Bridge v2 增量协商；v1 继续可用 |
| WSToken | 无状态签名，任意副本可验证 |
| 通知丢失 | 轮询 / 对账补偿 |

## 3. 基线

当前状态：

- Control、HTTP API、DaemonHub、AppHub、workflow worker 在同一进程；
- daemon 连接和 App 连接只在进程内；
- WSToken 只在签发进程内有效；
- local tool 通过单进程 AppHub 路由；
- workflow task / run events / tool invocations 已在 PostgreSQL；
- I2 的 checkpoint、waiting/resume、runtime-local tools、LangGraph 已完成。

## 4. 组件

- Gateway Instance Registry
- Runtime Connection Lease
- App Connection Lease
- Runtime Gateway
- App Gateway
- Database Notification
- App Bridge v2
- Idempotency Store
- Connection Reaper

## 5. 实施阶段

### I3.0 契约与数据库

交付：

- gateway instance 表；
- runtime connection lease 表；
- app connection lease 表；
- notification / outbox 基础表；
- 连接 lease 状态机；
- App Bridge v2 消息契约；
- WSToken 无状态签名格式；
- 数据库迁移测试。

退出：

- 单副本行为不变；
- 两个进程可以通过数据库判断连接归属；
- lease 过期、抢占和 epoch 失效规则明确。

### I3.1 角色拆分

交付：

- control/api 角色；
- runtime-gateway 角色；
- app-gateway 角色；
- 单进程兼容模式。

退出：

- 单进程模式通过全部 I2 测试；
- 拆分后的单副本角色可以完成现有 Run。

### I3.2 Runtime Gateway

交付：

- daemon hello 注册到数据库；
- Runtime Gateway 连接 lease；
- control 通过 registry/gateway 发送 TaskAssign / Cancel / Reconcile；
- daemon 重连与 run reconcile；
- gateway 故障恢复。

退出：

- daemon 可以从一个 Runtime Gateway 迁移到另一个；
- 任意 API 副本取消可以到达正确 Runtime；
- 已持久化事件不丢。

### I3.3 App Gateway 与 WSToken

交付：

- App WebSocket 注册与连接 lease；
- 无状态 WSToken；
- 事件跨副本 fanout；
- App 重连与订阅恢复；
- v1/v2 协议协商。

退出：

- App 在 A 连接，事件由 B 产生，A 能收到；
- 一个副本签发的 token 可在另一个副本验证；
- kill App Gateway 后 App 可以重连。

### I3.4 Local Tool 定向路由

交付：

- invocation 到 App connection 的定向投递；
- App Bridge v2 并发调用；
- 进度、取消、幂等；
- invocation store 跨副本去重；
- pending local tool 的重连/失败语义。

退出：

- 两个 App 副本不会重复执行同一个 invocation；
- 重复投递返回同一结果；
- App 断线不会让其他副本重复执行。

### I3.5 通知与恢复

交付：

- PostgreSQL notification 唤醒；
- outbox / pending 查询；
- 通知丢失补偿；
- 连接 reaper；
- lease 过期回收。

退出：

- 故意丢弃通知，系统仍能继续；
- 重复通知不导致重复执行；
- 过期 lease 不会永久占住连接。

### I3.6 多副本测试与退出

测试拓扑：

```text
2 x control/api
2 x runtime-gateway
2 x app-gateway
1 x PostgreSQL
1 x Runtime daemon
1 x App
```

必须验证：

- control 副本故障；
- runtime gateway 故障；
- app gateway 故障；
- daemon 重连；
- App 重连；
- local tool 重复投递；
- 跨副本 cancel；
- 跨副本 event fanout；
- notification 丢失；
- worker lease 过期；
- v1 App 兼容。

退出：

- 所有多副本故障场景通过；
- 事件不丢；
- 工具不重复执行；
- 任意副本可取消和路由；
- I3 checklist 更新完成。

## 6. 连接 Lease 规则

Runtime connection：

```text
daemon_id 主键
gateway_instance_id
connection_epoch
status = online / offline
lease_expires_at
last_heartbeat_at
capabilities / labels / runtime_type
running_count / reserved
```

规则：

- 新连接必须先取得 lease；
- 旧 lease 的发送被拒绝；
- 心跳续租；
- lease 过期由 reaper 标记 offline；
- daemon 重连可以换 gateway，epoch 增加。

App connection：

```text
connection_id 主键
app_id
tenant_id
gateway_instance_id
connection_epoch
protocol_version
capabilities
lease_expires_at
last_heartbeat_at
```

规则：

- 一个 App 可以有多个连接，但执行租约只授予一个连接；
- local tool 只投递给执行租约持有者；
- 订阅和事件回放仍以 Event Store 为准；
- 执行连接断开后，pending invocation 按明确错误失败，不广播重试。

## 7. App Bridge v2 最小契约

必须支持：

```text
hello:
  protocol_version
  connection_id
  app_id
  capabilities
  resume_cursor

tool_call:
  invocation_id
  run_id
  tool_name
  arguments_json
  deadline_ms

tool_result:
  invocation_id
  ok
  result_json / error
  idempotent = true

progress:
  invocation_id
  progress_json

cancel:
  invocation_id
  reason
```

兼容：

- v1 hello 继续接受；
- v1 不支持的能力明确降级；
- v2 消息只增量增加，不改变 v1 语义。

## 8. 可观测性

新增指标：

```text
levier_gateway_instances
levier_runtime_connection_lease
levier_app_connection_lease
levier_connection_reaped_total
levier_notification_lag_seconds
levier_cross_replica_routes_total
levier_tool_duplicate_suppressed_total
```

日志必须带：

```text
gateway_instance_id
connection_id / daemon_id
connection_epoch
app_id / tenant_id
run_id / invocation_id
request_id
```

## 9. 风险

1. **PostgreSQL 通知不是消息队列**：必须有 outbox / 对账兜底。
2. **连接 lease 竞争**：必须用 epoch 和 CAS，不能靠“最后写入获胜”。
3. **Local tool 重试放大**：必须有 invocation 幂等和定向租约。
4. **v1/v2 兼容**：不能假设所有 App 同日升级。
5. **控制面路由延迟**：避免每次 dispatch 全表扫描；registry 查询要可控。
6. **过早引入 Redis/NATS**：只有数据库通知成为真实瓶颈才评估。
7. **R1 不能提前混入 I3**：先完成 I3.1–I3.6 和退出评审；R1 只做接入层收敛，不扩大多副本范围。
8. **框架适配器有版本耦合**：LangGraph checkpointer 内部字段等必须隔离在适配器包，不能进入核心。

## 10. 当前进展

| 项目 | 状态 | 说明 |
|---|---|---|
| I3.0 契约与数据库 | 已完成 | `0018` schema、cluster store、lease 状态机、迁移测试 |
| Gateway Registry | 已完成 | gateway instance 注册、心跳、epoch、离线 |
| Runtime Connection Lease | 已完成 | daemon 注册、连接 epoch、心跳、接管、离线 |
| App Connection Lease | 已完成 | App 连接注册、协议版本、能力、心跳 |
| App Execution Lease | 已完成 | 单一 local-tool 执行连接、抢占和释放 |
| Notification Outbox | 已完成 | 持久化通知、待发送查询、发布/重试标记 |
| WSToken 无状态化 | 已完成 | 共享 secret HMAC token，可跨副本验证 |
| App Bridge v2 契约 | 已完成 | 协议版本、幂等、进度、取消的最小契约 |
| App Bridge v2 实现 | 已完成 | v2 hello/ack、resume cursor、progress、idempotent result、Python SDK |
| 内部角色 RPC | 已完成 | `RuntimeGatewayService`、内部 listener/token、同步 TaskAck 骨架 |
| Gateway presence 接线 | 已完成 | 启动注册 gateway instance，心跳续租，退出标记 offline |
| RoleSet 配置 | 已完成 | `gateway.roles`，支持 all 展开和未知角色校验 |
| RuntimeTransport port | 已完成 | Control 通过 `ports.RuntimeTransport` 发送/释放 Runtime |
| RuntimePlacement port | 已完成 | placement/reserve/disconnect 与 transport 分离 |
| AppConnections port | 已完成 | HTTP/WS 层不再直接依赖具体 AppHub |
| Capacity Lease | 已完成 | `runtime_capacity_leases`，原子预留、释放、过期回收 |
| Runtime 注册 fencing | 已完成 | 活跃 lease 下拒绝其他 gateway takeover |
| Notification Claim | 已完成 | dedupe key、claim owner/lease、发布/重试 |
| Tool invocation owner 字段 | 已完成 | owner gateway/connection、lease epoch、deadline schema |
| Gateway internal address | 已完成 | `gateway_instances.internal_address` 字段 |
| I3.0.1 契约补充 | 已完成 | 内部 RPC proto、App Bridge v2、地址/配置接线 |
| I3.1 角色拆分 | 已完成 | `all`、`control+api`、`runtime-gateway`、独立 `app-gateway` 均可启动；混合角色静默缺失已禁止 |
| I3.2 Runtime Gateway | 已完成 | 跨进程路由、Dispatch/daemon-state fencing、durable outbox、同 daemon 跨 gateway 迁移均通过矩阵 |
| I3.2 跨进程 smoke | 已完成 | split control/api + runtime-gateway + echo runtime 完成一次 Run |
| I3.2 gateway failover smoke | 已完成 | 杀掉 gateway A 后，同 daemon 以 epoch 2 注册到 gateway B，后续 Run 完成 |
| Runtime envelope outbox | 已完成 | Runtime Gateway 先落 `gateway_notifications`，Control claim 后应用并标记 published |
| App Gateway split smoke | 已完成 | app-gateway WS v2 hello/ack；`app_connections` 与 `app_execution_leases` 落库 |
| App Gateway 多副本 | 已完成 | 2 个 app-gateway 的连接迁移、事件 fanout 与执行租约已验收 |
| I3.3 App Gateway | 已完成 | app-gateway 拆分、v2 handshake/订阅、WSToken 跨副本、连接迁移均通过矩阵 |
| I3.4 Local Tool 路由 | 已完成 | 定向投递、重复 invocation 去重、App 断线 pending 失败、不跨连接重试均通过 |
| I3.5 通知与恢复 | 已完成 | outbox/claim TTL 重放、expired lease reaper、跨 Control envelope 消费均通过 |
| I3.6 多副本测试 | 已完成 | `tests/e2e/i3_multireplica.py` 通过 11 个场景，CI job `i3-multireplica` 通过 |
| I3 退出评审 | 已完成 | 2026-11-26 通过本地与 GitHub Actions 多副本验收 |
| R1 Runtime 接入层收敛 | 待开始（I3 后置） | 核心与 Agent 框架适配器分层；不阻塞 I3 退出 |

## 11. R1：Runtime 接入层收敛（I3 后置）

> 状态：待开始
>
> 触发：I3.1–I3.6 退出评审通过；若 I4 设计提前启动，至少先完成边界评审。
>
> 不阻塞 I3 退出，但必须在 I4 sandbox 设计和落地前完成。

### 11.1 目标

- 核心 SDK 只提供通用 Runtime 接入：协议、生命周期、`RunContext` 基础操作、事件、取消、checkpoint 和工具桥。
- LangGraph、CrewAI、React 等 Agent 框架作为可选适配器或示例，不进入基础安装和默认 runtime 镜像。
- 框架内部观测、错误、重试、取消与 Levier Run 语义有明确 ownership，避免双重处理。
- Sandbox 留在 I4，并且与 Agent 框架解耦。

### 11.2 当前事实

- `levier-runtime` 基础包仍包含 `llm`、`tools`、`mcp`、`react` helper；`Toolbox` 在顶层 import 链中加载 MCP client。
- `ctx.chat`、`ctx.toolbox`、`ctx.invoke_all` 是便利层，不应成为框架中立核心。
- `langgraph_runtime` 已经是独立包，使用官方 `langgraph`，但 checkpoint adapter 依赖 `InMemorySaver` 内部字段。
- `react_runtime` + `agent_core` 仍是默认 runtime 镜像和主要 E2E 路径。
- CrewAI 没有官方适配器。
- `sandbox` 仍只是类型占位，没有 Launcher/Job/Workspace。

### 11.3 改造项

1. 基础 `levier-runtime` 不在顶层导入 MCP/React/LLM/Toolbox helper。
2. 增加可选 extras 或独立包：`llm`、`mcp`、`react`、`langgraph`、`crewai-example`。
3. 基础 runtime 镜像只含核心 SDK 和 Echo；React 使用独立 demo 镜像。
4. `ctx.call_tool`、`ctx.emit`、`ctx.checkpoint`、`ctx.wait` 保留为核心；`ctx.chat`、`ctx.invoke_all`、`ctx.toolbox` 降级为 optional helper。
5. LangGraph 保留可选官方适配器，补齐工具桥、错误/取消/事件投影契约和版本契约测试。
6. CrewAI 先提供 `run(ctx)` 示例，明确不保证 checkpoint/resume。
7. 增加框架 adapter 契约：输入映射、事件投影、工具桥、错误归一化、checkpoint codec、取消传播。
8. 默认 Compose 不再把 React 作为唯一主路径；文档把自定义 runtime 作为正常接入方式。

### 11.4 能力分层

```text
L0  wire 契约
    proto + 生成代码

L1  levier-runtime core
    Runtime / RunContext / channel / manager / events / cancellation /
    call_tool / checkpoint

L2  optional adapters
    llm / mcp / react / langgraph / crewai-example

L3  examples and images
    echo / react-demo / user runtime image

L4  sandbox
    I4 Launcher / Workspace / Job，与 Agent 框架无关
```

### 11.5 职责边界

| 层 | 责任 |
|---|---|
| Levier 核心 | Run 生命周期、lease、调度、事件存储、checkpoint 存储、App/global tool bridge、取消、Run 级重试 |
| 框架适配器 | TaskAssign→框架输入、框架事件→Levier 事件投影、工具桥、错误归一化、checkpoint codec、取消传播 |
| Agent 框架 | 自身循环、模型、图/任务、内部工具、内部 trace、受限内部重试 |

### 11.6 必须固定的语义

- **单一事件源**：框架原生 trace 不直接写入 Run Event Store；适配器只做白名单投影。
- **单一重试源**：Run 级重试归 Levier；框架内部重试必须受限，工具调用保持 `invocation_id` 幂等。
- **单一取消源**：Levier 取消 Run；适配器传播取消，框架不支持时明确标注 best-effort。
- **checkpoint 不透明**：Levier 只存 bytes；适配器拥有格式、版本和 session/run 校验。
- **timeout 层级**：Levier run/tool timeout 为最终权威；框架 timeout 是内层预算。
- **工具事件不重复**：经过 `ctx.call_tool` 的工具由 Levier 发事件；框架内部工具如需可见，由适配器显式映射，不重复发同一事件。

### 11.7 退出条件

- 基础安装和基础镜像不包含 Agent 框架依赖；`import levier_runtime` 不加载 MCP/React/LLM helper。
- Echo 和手写 `run(ctx)` 通过 conforming runtime 测试。
- 官方 LangGraph 可选适配器可以运行并完成 checkpoint/resume。
- CrewAI 示例可以运行，并明确不保证 resume。
- 默认部署不再把 React 当作唯一主路径。
- Runtime wire 协议不变，v1/v2 兼容不变。

### 11.8 不做

- 不抽象统一所有 Agent 框架状态。
- 不因 R1 修改 Runtime wire 协议。
- 不在 R1 做 sandbox（属于 I4）。
- 不承诺 CrewAI 完整 checkpoint/resume。

### 11.9 与现有实现的关系

- **LangGraph**：保留为可选官方适配器，因为 interrupt/checkpoint/resume 有真实集成价值；但必须隔离版本和内部字段依赖。
- **React / agent_core**：移出默认镜像，保留示例和兼容 shim。
- **CrewAI**：先示例，不做深度内置；其同步/线程模型和缺少标准 checkpoint 不适合作为核心依赖。
- **Sandbox**：I4 独立基础设施，不属于 R1，也不属于 Agent 框架。

## 12. 多副本验收记录（2026-11-26）

拓扑为 2×control/api、2×runtime-gateway、2×app-gateway、1×PostgreSQL、1×Runtime daemon、1×App。自动化和真实进程入口：

```sh
make i3-multireplica
```

`tests/e2e/i3_multireplica.py` 实际启动独立角色进程，并通过 PostgreSQL 断言状态和事件，覆盖：

| 场景 | 验收点 |
|---|---|
| baseline | Run 完成；事件持久化；local tool 仅执行一次；v2 App 收到 WebSocket 事件；`app_connections` 记录 v2 协议与能力 |
| notification_replay | 故意延迟通知 claim 后仍能重放，且不重复写入事件 |
| cross_replica_cancel | 从 control A 提交，从 control B 取消，Runtime 收到取消且无活动任务 |
| control_failure | control A 被杀后，control B 继续消费 envelope 并完成 Run |
| app_disconnect_pending | App 断线后 pending local tool 直接失败，不自动换连接重试 |
| worker_lease_expiry | 伪造过期 worker lease 后由 reaper 回到 PENDING 并完成 Run |
| runtime_gateway_failure | 杀掉 runtime-gateway A 后 daemon 迁移到 B，Run 完成 |
| active_run_recovery | daemon 与 gateway 同时消失、offline 通知丢失时，lease 过期后的周期性 active-run 对账仍能恢复 Run |
| app_gateway_failure | 杀掉 app-gateway A 后 App 重连 B，执行租约迁移，local tool 与事件继续 |
| local_tool_idempotency | 同一 invocation 投递两次只产生一次 tool_invocations 记录和一次 App 执行 |
| v1_app_compat | 不发 hello 的 v1 App 可连接并收到 Run 事件 |

GitHub Actions 同时增加了：

- `go-postgres`：真实 PostgreSQL 下运行 `go test ./...` 与 `go vet ./...`；
- `i3-multireplica`：执行上述多副本矩阵；
- 本地 `make test-postgres` 作为同一集成测试入口。

### 12.1 internal 目录归纳

I3 完成后已将平铺的 `internal/` 按层收敛为：

```text
internal/
  contract/       contract, protocol, roles
  platform/       clock, config, db, errs, id, observe, version
  state/          cluster, register, runstore, workflow
  controlplane/   control, controlremote, controlservice
  runtime/        connection, daemoncheck, gateway, internalapi, ports, runtimegateway
  app/            appgateway, httpapi
  ops/            maintenance, selfcheck, selfdesc, status
  testing/        testutil
```

`make deps-check` 的层依赖规则和所有导入路径已同步更新；该结构不改变 I3 的 wire 契约和部署角色边界。


