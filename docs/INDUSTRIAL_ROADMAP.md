# Levier 工业化路线

> 状态：Accepted  
> 更新：2026-11-22  
> 定位：工业化演进的唯一权威文档。  
> 说明：本文只定义目标、版本、决策、组件边界和验收，不展开函数、文件、数据库字段级实现。后续 checklist 只拆执行项，不重新定义本文决策。

## 1. 目标与边界

目标：

- App 可以多语言接入，但现有 Python App 写法不变；
- Runtime 可以承载复杂执行模型，但简单 `run(ctx)` 写法不变；
- Control Plane 可以从单机扩展到多副本和集群；
- 在扩展过程中保持单一状态源，避免多套状态不一致。

不做：

- 多区域容灾和跨区域调度；
- 完整 exactly-once；
- 自研图执行引擎；
- 第一阶段做完整 SaaS 计费和强租户隔离；
- 在第一阶段同时引入大量新基础设施。

## 2. 当前基线

- `normal` Runtime 已经可用。
- `sandbox` 和 `longterm` 目前是目标语义，不是完整生产能力。
- 控制面支持 `all` 单进程，以及 `control+api` + `runtime-gateway` 拆分；app-gateway 拆分、多副本故障迁移和事件补偿仍未完成。
- PostgreSQL 已经承担 Run、事件、任务和 lease，是可靠基础。
- 当前 Runtime 一个进程只服务一个 Runtime 定义。
- 当前 App SDK 以 Python 为主，协议是 JSON 消息。
- Runtime SDK 核心、Echo 最小示例、React 组合和独立 LangGraph 适配器已存在。
- CrewAI 没有官方适配器；`sandbox` 仍是类型占位，没有 Launcher/Job/Workspace。
- 默认 runtime 镜像仍安装 `agent_core`/React；基础 SDK 也仍包含 LLM/MCP/React helper，这是 R1 要收敛的耦合。

## 3. 已确认决策

| 决策 | 结论 |
|---|---|
| Runtime 进程模型 | 一个进程只承载一个 Runtime 定义；默认单实例，需要时可选多副本 |
| 多定义 Runtime Host | 不进 I1–I4；I5 视低流量场景选做 |
| 事件分发 | I1–I4 只使用 PostgreSQL 通知；不引入 Redis/NATS |
| Sandbox 后端 | 共享 Kubernetes；每个 Run 一个 Job；Longterm 可复用 Workspace/Job |
| 对象存储 | I2 定义接口，I4 落地；小状态继续数据库，大文件和 artifact 转对象存储 |
| Sidecar 传输 | HTTP 为主；stdio 仅本地 CLI/嵌入式可选 |
| 多租户 | I1 软多租户；I4 配额；I5 物理隔离与计费 |
| Runtime 核心边界 | 核心只负责协议、生命周期和基础上下文；Agent 框架适配器独立发布 |
| 框架观测与错误 | 框架原生 trace 不直接写入 Run Event Store；适配器只做白名单投影和终态归一化 |
| 重试归属 | Run 级重试归 Levier；框架内部重试必须受限，工具调用保持 invocation 幂等 |
| Sandbox 归属 | 与 Agent 框架解耦；I4 由 Launcher/Workspace 提供隔离，而不是 SDK 内置 |

## 4. 不变约束

1. **简单路径不变**：现有 Runtime 和 Python App 写法继续可用。
2. **单一状态源**：I1–I4 以 PostgreSQL 为事实来源。
3. **不引入多套状态**：I1–I4 不引入 Redis/NATS。
4. **一进程一定义**：一个 Runtime 进程不混载多个 Runtime 定义。
5. **多副本可选**：同一定义默认单实例；需要时增加同定义副本。
6. **先角色拆分，后副本**：未完成路由和分布式安全契约前，不直接复制控制面。
7. **软多租户前置**：租户归属从 I1 进入核心对象和访问路径。
8. **事件存储优先**：通知只负责唤醒，历史和回放以 Event Store 为准。
9. **协议只增量演进**：不删除 v1 语义，旧客户端按最小能力运行。
10. **选做项不阻塞主线**：没有触发条件就不做。
11. **核心不绑定 Agent 框架**：Runtime SDK 核心只依赖协议、生命周期和基础上下文；框架适配器独立发布。
12. **框架状态不进入核心**：checkpoint 保持不透明 bytes，由适配器编解码；事件只做白名单投影。
13. **默认部署保持框架中立**：R1 后基础 runtime 镜像只含核心 SDK 和最小示例，不默认内置 Agent 框架。

## 5. 版本总览

| 版本 | 主题 | 核心目标 | 退出条件 |
|---|---|---|---|
| I1 | 单机稳定版 | 稳定当前单机和契约，建立软多租户边界 | 当前功能稳定；无新增基础设施；租户边界可追踪 |
| I2 | Runtime 扩展版 | 让复杂 Runtime 可写，但不引入 sandbox/对象存储 | 简单 Runtime 不变；Driver、自定义事件、checkpoint/resume 可用 |
| I3 | 多副本控制面版 | 控制面和 App 连接可多副本 | 两个 Control / App 副本通过故障切换和重复执行测试 |
| R1 | Runtime 接入层收敛（I3 后置） | 核心 SDK 只保留通用接入；官方框架适配器可选 | 基础安装/镜像无 Agent 框架；LangGraph/CrewAI 接入可运行 |
| I4 | 隔离与长期版 | 真实 sandbox 隔离和长期会话 | Job 隔离、Workspace、长期会话、对象存储、配额可运营 |
| I5 | SaaS 规模版 | 只做真实瓶颈驱动的扩展 | 通过目标规模、隔离等级和成本验收 |

版本关系：

```text
I1 稳定单机
  -> I2 扩展 Runtime
  -> I3 扩展控制面
  -> R1 收敛 Runtime 接入层（后置，不阻塞 I3 退出）
  -> I4 扩展隔离和长期状态
  -> I5 扩展规模和商业化
```

I2 和 I3 可以在契约稳定后并行，但 I3 不能跳过 I2 的 Runtime 能力；App Bridge v2 在 I3 落地。R1 默认在 I3.6 退出后启动，最迟必须在 I4 sandbox 设计和落地前完成，避免把内置 Agent 框架带进隔离层。

## 6. 主题与版本索引

| 主题 | I1 | I2 | I3 | I4 | I5 |
|---|---|---|---|---|---|
| Runtime 简单入口 | 保持 | 保持 | 保持 | 保持 | 保持 |
| Runtime Driver | 不做 | 引入 | 保持 | 复用 | 复用 |
| 自定义事件 | 不做 | 引入 | 复用 | 复用 | 复用 |
| Runtime-local 工具 | 不做 | 引入 | 复用 | 复用 | 复用 |
| Checkpoint / Resume | 不做 | PostgreSQL 内实现 | 复用 | 生产化 | 复用 |
| LangGraph | 不做 | 原型 | 复用 | 可选官方适配器 | 复用 |
| Agent 框架内置 | 不做 | React 示例 | 示例/可选 | R1 后不进入默认 | 复用 |
| App Bridge v2 | 冻结语义 | 不做 | 实现 | 复用 | 扩展语言 |
| App 多副本 | 不做 | 不做 | 支持 | 复用 | 复用 |
| 软多租户字段 | 引入 | 保持 | 保持 | 保持 | 保持 |
| 配额 | 接口 | 接口 | 接口 | 执行 | 细化 |
| Runtime Gateway | 不做 | 不做 | 引入 | 复用 | 复用 |
| App Gateway | 不做 | 不做 | 引入 | 复用 | 复用 |
| PostgreSQL 通知 | 可用 | 可用 | 跨实例使用 | 复用 | 可选替换 |
| Redis/NATS | 不做 | 不做 | 不做 | 不做 | 按需 |
| Sandbox | 不做 | 不做 | 不做 | Job 落地 | 预热池可选 |
| 对象存储 | 不做 | 接口 | 不做 | 落地 | 复用 |
| Sidecar | 不做 | 不做 | 不做 | 不做 | HTTP 可选 |
| 物理租户隔离 | 不做 | 不做 | 不做 | 不做 | 按需 |

R1 是横切工作流，不在上表逐年展开。它调整 Runtime 核心边界、可选框架适配器和默认镜像，但不修改 Runtime wire 协议。

## 7. I1：单机稳定版

**目标**

把当前单机能力做稳，先把契约、租户边界和一致性原则固定下来，不引入新基础设施。

**逻辑变更**

- 一个进程只承载一个 Runtime 定义；
- 默认单实例，需要时可启动同定义多副本；
- 不引入多定义 Runtime Host；
- 所有状态以 PostgreSQL 为准；
- 不引入 Redis/NATS；
- 软多租户字段、认证、日志和指标先落地；
- 不把 sandbox/longterm 做成半成品。

**引入组件**

- Tenant Scope
- Scoped Auth
- Quota Hooks
- Tenant Metrics
- 统一的能力和事件契约

**不做**

- Sandbox 生产化
- Longterm 生产化
- 多定义 Runtime Host
- Redis/NATS
- 控制面多副本

**退出条件**

- 当前 normal Runtime 功能稳定；
- 没有新增基础设施；
- Run、App、Session、日志和指标可按租户追踪；
- 契约文档和决策记录一致。

## 8. I2：Runtime 扩展版

**目标**

解决“复杂 Runtime 写不出来”，同时保持简单 Runtime 不变。

**逻辑变更**

- 增加 Runtime Driver 和 Capability Manifest；
- 增加自定义事件；
- 增加 Runtime-local 工具；
- 增加模型 Provider 扩展点；
- checkpoint 和 resume 先落 PostgreSQL；
- LangGraph 作为第一个复杂 Driver 原型；
- 仍然一进程一定义；
- 不引入 sandbox、对象存储、Redis/NATS。

**引入组件**

- Runtime Driver
- Capability Manifest
- Custom Event
- Tool Registry
- Provider Registry
- Session/Checkpoint Store（PostgreSQL）
- LangGraph Adapter（原型）

**不做**

- Sandbox
- 对象存储
- 多定义 Runtime Host
- Redis/NATS
- 物理租户隔离

**退出条件**

- 现有简单 Runtime 不需要修改；
- LangGraph 可以中断和恢复；
- checkpoint 可持久化、查询和过期；
- 自定义 Runtime 可以注册工具和发自定义事件；
- 不依赖外部消息系统。

## 9. I3：多副本控制面版

**目标**

解决控制面不能多副本，以及 App 长连接和工具调用不能跨实例的问题。

**逻辑变更**

- 拆出 Runtime Gateway 和 App Gateway；
- 控制面只负责调度和状态；
- 使用 PostgreSQL 通知做跨实例唤醒；
- WebSocket token 无状态化；
- App 连接使用执行租约；
- 本地工具定向投递，不再广播；
- App Bridge v2 支持并发、进度、取消和幂等；
- 仍然只有 PostgreSQL，不引入 Redis/NATS。

**引入组件**

- Runtime Gateway
- App Gateway
- Runtime Registry
- App Registry
- Connection Lease
- Capacity Lease
- Database Event Notification
- Idempotency Store
- App Bridge v2

**不做**

- Redis/NATS
- 物理租户隔离
- Sandbox 生产化
- 多定义 Runtime Host

**退出条件**

- 两个 Control 副本不会重复分配同一个 Run；
- 杀掉一个 Control，Run 能继续或正确重新入队；
- 从任意 API 副本取消 Run 都能到达正确 Runtime；
- App 连接在 A，事件由 B 产生，A 仍能收到；
- 两个 App 副本不会重复执行同一个工具调用；
- 单个 Gateway 故障不会丢失已持久化事件。

## 9.5 R1：Runtime 接入层收敛（I3 后置）

**定位**

R1 不阻塞 I3 退出，但必须在 I4 sandbox 设计和落地前完成。目标是让 Levier 核心只提供通用 Runtime 接入，而不是内置 Agent 框架。

**目标**

- 核心 SDK 只保留协议、生命周期、`RunContext` 基础操作、事件、取消、checkpoint 和工具桥。
- LangGraph/CrewAI/React 等作为可选适配器或示例，不进入基础安装和默认镜像。
- 框架内部观测、错误、重试、取消与 Levier Run 语义有明确 ownership，避免双重处理。
- Sandbox 留在 I4，并且与 Agent 框架解耦。

**逻辑变更**

- `levier-runtime` 基础包不顶层导入 LLM/MCP/React/Toolbox helper。
- 增加可选 extras 或独立包：`llm`、`mcp`、`react`、`langgraph`、`crewai-example`。
- 基础 runtime 镜像只含核心 SDK 和 Echo；React demo 使用独立镜像。
- LangGraph 保留可选官方适配器，补齐工具桥、错误/取消/事件投影契约。
- CrewAI 先提供 `run(ctx)` 示例，不承诺完整 checkpoint/resume。
- `ctx.chat`、`ctx.invoke_all`、`ctx.toolbox` 降级为 optional helper；`ctx.call_tool` 保留为核心工具桥。

**职责边界**

| 层 | 责任 |
|---|---|
| Levier 核心 | Run 生命周期、lease、调度、事件存储、checkpoint 存储、App/global tool bridge、取消、Run 级重试 |
| 适配器 | TaskAssign→框架输入、框架事件→Levier 事件投影、工具桥、错误归一化、checkpoint codec、取消传播 |
| Agent 框架 | 自身循环、模型、图/任务、内部工具、内部 trace、受限内部重试 |

**必须固定的语义**

- 单一事件源：框架原生 trace 不直接写入 Run Event Store；适配器只做白名单投影。
- 单一 Run 重试源：Run 级重试归 Levier；框架内部重试必须受限，工具调用保持 `invocation_id` 幂等。
- 单一取消源：Levier 取消 Run；适配器传播取消，框架不支持时明确标注 best-effort。
- checkpoint 不透明：Levier 只存 bytes；适配器拥有格式、版本以及 session/run 校验。
- timeout 层级：Levier run/tool timeout 是最终权威；框架 timeout 是内层预算。

**退出条件**

- 基础安装和基础镜像不包含 Agent 框架依赖；`import levier_runtime` 不加载 MCP/React/LLM helper。
- Echo 和手写 `run(ctx)` 通过 conforming runtime 测试。
- 官方 LangGraph 可选适配器可以运行并完成 checkpoint/resume。
- CrewAI 示例可以运行，并明确其不保证 resume。
- 默认部署不再把 React 当作唯一主路径。
- Runtime wire 协议不变，v1/v2 兼容不变。

**不做**

- 不抽象统一所有 Agent 框架状态。
- 不因 R1 修改 Runtime wire 协议。
- 不在 R1 做 sandbox（属于 I4）。
- 不承诺 CrewAI 完整 checkpoint/resume。

**触发条件**

I3.1–I3.6 退出评审通过后启动。若 I4 设计提前启动，至少先完成 R1 的边界评审。执行项见 `docs/I3_CHECKLIST.md` 第 11 节。

## 10. I4：隔离与长期版

**目标**

让 sandbox 和 longterm 从“类型/占位”变成可运营能力。

**逻辑变更**

- 共享 Kubernetes 集群；
- 每个 Run 一个 Job；
- Longterm 会话可复用同一 Workspace/Job；
- Runtime 只拿 Workspace，不管理 Kubernetes；
- Session/Checkpoint Store 生产化；
- 大 checkpoint 和 artifact 转对象存储；
- 软多租户开始执行配额和隔离策略。

**引入组件**

- Runtime Launcher
- Sandbox Provider
- Workspace Service
- Session Manager
- Checkpoint Store
- Object Store
- Quota Manager
- Artifact Store

**不做**

- Redis/NATS
- 多定义 Runtime Host
- 多区域
- 完整计费
- 物理租户隔离

**退出条件**

- Sandbox Runtime 可以在独立 Job 中执行；
- Runtime 不直接管理 Kubernetes；
- 长期会话可以保存、恢复和过期；
- 大状态和 artifact 不进入数据库主表；
- 租户配额可以生效。

## 11. I5：SaaS 规模版

**目标**

只做真实瓶颈驱动的扩展，不为“可能用到”提前引入复杂度。

**可选内容**

- Runtime Host 多定义；
- Redis；
- NATS；
- Pod 预热池；
- HTTP Sidecar；
- 更多语言 SDK；
- 物理租户隔离；
- 多集群、多区域；
- 计费和更细配额。

**引入原则**

- 每项必须有明确触发条件；
- 每项必须评估运维成本和一致性影响；
- 未达到触发条件时不做；
- 不因为“以后可能用”而提前引入。

**退出条件**

- 通过目标规模、隔离等级、延迟和成本验收；
- 新增组件有明确监控、故障和回滚方案；
- 不破坏 I1–I4 的单一状态源原则。

## 12. 选做池

| 选做项 | 触发条件 | 代价 | 结论 |
|---|---|---|---|
| Runtime Host 多定义 | 大量低流量 Runtime，进程内存成为主要成本 | 隔离、版本、容量核算复杂 | I5 再评估 |
| Redis | 容量、presence 或幂等查询出现数据库热点 | 多一个高可用组件和短期状态一致性 | I5 再评估 |
| NATS | Gateway 数量、连接数或事件 fanout 超过数据库通知能力 | 新消息系统、重复投递和运维复杂度 | I5 再评估 |
| Pod 预热池 | Sandbox Job 冷启动成为用户可见瓶颈 | 预热资源、清理、隔离复杂度 | I5 再评估 |
| 对象存储 | checkpoint 超过约定阈值，或 artifact 量明显增长 | 生命周期、权限和网络依赖 | I4 评估 |
| HTTP Sidecar | 第二语言、遗留系统或客户内网接入 | 额外进程和部署复杂度 | I5 再评估 |
| stdio Sidecar | 仅本地 CLI 或嵌入式场景 | 不能跨网络 | 可选，不作为默认 |
| 物理租户隔离 | 企业客户、合规或强噪声邻居要求 | 多集群/多节点池成本 | I5 再评估 |
| 多区域 | 真实跨地域延迟或容灾要求 | 数据复制和一致性复杂度 | 暂缓 |
| exactly-once | 业务副作用无法通过幂等解决 | 分布式事务成本极高 | 不做；at-least-once + 幂等 |

## 13. 风险与依赖

1. **LangGraph 依赖 checkpoint 闭环**：没有持久化和恢复，interrupt/resume 没有意义。
2. **控制面多副本依赖 App 定向路由**：继续广播会造成工具重复执行。
3. **多语言 SDK 依赖 App Bridge v2**：先写语言 SDK 会复制同步、幂等和重连问题。
4. **Sandbox 依赖 Launcher/Workspace**：仅增加 Runtime 类型不会产生隔离。
5. **兼容性风险**：协议只加字段，升级需要 canary 和 drain。
6. **一致性风险**：通知不是状态源，事件回放必须以 Event Store 为准。
7. **过早引入 Redis/NATS 会把问题从业务转移到基础设施运维**，必须由真实瓶颈驱动。
8. **框架适配器会成为版本耦合点**：LangGraph checkpointer 等内部状态可能存在版本差异，必须隔离在适配器包并用契约测试锁定。
9. **双重重试/观测/取消是最严重的语义耦合**：Run 级语义必须唯一归属于 Levier，框架内部只能做受限处理。
10. **R1 不能反向污染 I3**：R1 是后置工作流，先完成多副本退出评审，避免同时扩大两个阶段的范围。

## 14. 文档与后续

本文是唯一权威文档。

- `docs/adr/0001`–`0004` 是历史设计记录，继续保留。
- 本次新增的 Runtime、App、Control Plane 决策已合并到本文。
- I1 执行清单：`docs/I1_CHECKLIST.md`，代码基础已完成，等待退出评审。
- I2 执行计划：`docs/I2_CHECKLIST.md`，当前详细计划。
- I3 执行计划：`docs/I3_CHECKLIST.md`，多副本控制面阶段。
- R1 执行项：见 `docs/I3_CHECKLIST.md` 第 11 节，I3 退出后启动。
- 后续 checklist 不重新定义本文决策；范围变化先改本文。

## 15. 变更记录

| 日期 | 变更 |
|---|---|
| 2026-11-22 | 建立工业化路线，确认 I1–I5 版本划分和七项基础决策 |
| 2026-11-23 | 增加 I2 详细执行计划，关联 I1 完成状态 |
| 2026-11-26 | 增加 R1 Runtime 接入层收敛：核心与 Agent 框架适配器分层，I3 后置、I4 前完成 |
