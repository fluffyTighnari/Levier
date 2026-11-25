# I3 执行清单：多副本控制面

> 状态：In Progress  
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
| I3.0.1 契约补充 | 进行中 | 内部 RPC proto、App Bridge v2 实现、地址/配置接线 |
| I3.1 角色拆分 | 进行中 | presence、RoleSet、ports、capacity 已落地；下一步装配服务 |
| I3.2 Runtime Gateway | 待开始 | daemon 跨副本路由 |
| I3.3 App Gateway | 待开始 | App 连接与事件 fanout |
| I3.4 Local Tool 路由 | 待开始 | 定向投递与幂等 |
| I3.5 通知与恢复 | 待开始 | 补偿与 reaper |
| I3.6 多副本测试 | 待开始 | 故障矩阵 |
| I3 退出评审 | 待开始 | 多副本验收 |
