<!-- Levier — product scope and boundary freeze. -->

# 产品范围与边界（SCOPE）

| 项目 | 内容 |
|---|---|
| 状态 | 第 0 跳已封板 |
| 产品名 | Levier（已确定） |
| 最近更新 | 2026-11-15 |
| 目的 | 冻结产品边界，防止实现细节倒灌 |
| 本文不覆盖 | 架构设计、协议细节、部署步骤、路线图 |

> 本文只回答四件事：**产品是什么、不包含什么、谁负责什么、什么算越界。**
> 变更边界必须修改本文；跨边界的实现变更需要 ADR。

---

## 1. 一句话定义

**Levier 是 Agent Run 的控制面，并随产品提供默认执行侧 runtime 与默认 MCP 连接能力。**

控制面负责 Run 生命周期、调度、工具路由和事件；默认组件负责模型循环与 MCP 会话；两者通过版本化契约解耦，均可替换。

Levier 不实现业务工具、不实现 MCP Server，也不提供业务界面。

---

## 2. 产品边界

### 2.1 产品负责

```text
App / 工具 / MCP 引用的注册与隔离
Run 的创建、排队、调度、租约与状态机
local / global 工具的路由与调用记录
global 工具执行（http / webhook）
事件的持久化、回放与 WebSocket 推送
App 认证、短期 ws_token、Admin bootstrap
PostgreSQL 中的目录、Run、任务、事件、调用记录
运行时可操作面：健康、就绪、指标、自描述、状态、自检
默认执行侧 runtime（normal 策略、模型循环、工具桥）
默认 MCP runtime（连接、会话、凭据占位解析、allowed_tools）
```

### 2.2 产品不负责

```text
业务工具的实现与业务数据
MCP Server 的实现
模型供应商的扩展适配（默认 runtime 只承诺现有适配）
业务前端、业务 UI、业务权限模型
GUI 控制台（以后考虑）
多租户的业务隔离策略（后续再考虑）
部署平台、集群编排、服务网格
```

> 默认 runtime 内的模型调用与模型凭据由随附组件负责，不属于控制面。

---

## 3. 四个边界

| 边界 | 内容 | 当前决定 |
|---|---|---|
| 产品边界 | 用户到底在用什么 | 控制面 + 随附默认组件，不包含参考示例 |
| 进程边界 | 哪些东西跑在不同进程 | 控制面进程、`harness`、`mcp_runtime`、App 各自独立 |
| 契约边界 | 升级时不能随便变的东西 | AppPort：HTTP / WS v1；DaemonPort：gRPC v1 |
| 部署边界 | 什么算一个部署单元 | MVP 用 Compose；后续单二进制 + 外部 PostgreSQL |

---

## 4. 责任矩阵

| 能力 | Levier | App | Harness | 出问题时先看 |
|---|---|---|---|---|
| Run 生命周期 | 负责（控制面） | — | — | Run 状态、事件、任务表 |
| 调度与租约 | 负责（控制面） | — | 上报容量 | daemon 注册、任务队列 |
| 工具路由 | 负责（控制面） | 执行 local 工具 | 发起工具调用 | 调用记录、App 连接 |
| global 工具执行 | 负责（控制面） | — | 发起工具调用 | executor 日志、调用记录 |
| 业务工具 | 不负责 | 负责 | — | App 日志、工具返回 |
| 模型循环 | 负责（默认 runtime） | — | 使用 | Harness 日志、ERROR 事件 |
| 模型凭据 | 负责（默认 runtime 环境） | — | 使用 | Harness 配置 |
| MCP 连接 | 负责（默认 MCP runtime） | — | 使用 | `mcp_runtime` 日志 |
| MCP Server | 不负责 | — | — | 第三方 server 日志 |
| App 认证 | 负责（控制面） | 持有 App key | — | 401、request_id |
| 事件持久化 | 负责（控制面） | — | 上报事件 | 事件游标、存储 |
| 运行时可操作面 | 负责（控制面） | — | — | health / ready / metrics |

---

## 5. 默认组件、参考实现与契约

产品核心是控制面；以下组件随产品交付，属于**默认能力**：

```text
execution/agent_core/       语言无关的循环核心
execution/harness/          Python 执行侧默认 runtime
execution/mcp_runtime/      MCP 默认连接进程
```

默认组件可以被同契约实现替换，但产品承诺它们开箱可用、有默认验收。  
MCP 能力是产品的一部分：Levier 默认提供 MCP 连接、会话、凭据占位解析和 `allowed_tools` 过滤；MCP Server 本身仍由外部提供。

以下只作为参考或示例，不构成产品承诺：

```text
examples/counter_agent      参考 App
```

对外契约只来自：

```text
contracts/                   HTTP / WS 契约
proto/                       gRPC 契约
docs/api/openapi.yaml        HTTP 契约源
```

这条边界很重要：**默认组件可以替换，契约不能随便改。**

> `execution/agent_core/`、`execution/harness/`、`execution/mcp_runtime/` 已按执行侧归位；这不改变本文的责任划分。

---

## 6. 非目标

### 6.1 产品永久不负责

```text
业务工具与业务数据
MCP Server 实现
模型供应商的扩展适配
业务前端与业务权限
部署平台与集群编排
```

### 6.2 本迭代暂不做

```text
接入新的 App
接入新的 Harness
Adapter / Sidecar SDK（已立项，见 docs/ADAPTER_ROADMAP.md）
多 Daemon / 多 Levier / HA
sandbox runtime
longterm checkpoint 续跑
性能与压测优化
MCP 深度加固
多租户业务模型（后续再考虑）
GUI / TUI（以后考虑）
```

“暂不做”不是“永远不做”，只是当前不占用注意力。

---

## 7. 兼容性承诺

```text
AppPort 与 DaemonPort 都有版本号
破坏性变更必须提升协议版本
默认组件可以跟着变，契约必须先变
错误码、事件字段、WebSocket 消息类型都属于契约
```

---

## 8. 边界是否健康：三个测试

```text
1. 换掉 App，Levier 控制面需要改吗？                    不需要
2. 换掉默认 Harness，App 需要改吗？                      不需要
3. 升级 Levier 控制面，App / Harness 只依赖版本化契约吗？  是
```

三个答案都是“是”，边界才是干净的。

---

## 9. 变更流程

```text
改产品边界        → 修改本文并评审
改契约            → 更新 contracts/ 或 proto/ + 契约测试
跨边界实现变更    → 先写 ADR，再改代码
只改默认组件/示例  → 不影响本文与契约
```

---

## 10. 待确认

```text
无。
```

---

## 11. 决策记录

| 日期 | 决策 | 结果 |
|---|---|---|
| 2026-11-15 | 产品名 | Levier |
| 2026-11-15 | MCP 能力 | 产品默认能力；`mcp_runtime` 为默认实现，可替换 |
| 2026-11-15 | GUI | 以后考虑，本迭代不做 |
| 2026-11-15 | global executor | 产品能力 |
| 2026-11-15 | Adapter / Sidecar | 延后实现，见 `docs/ADAPTER_ROADMAP.md` |
| 2026-11-15 | 多租户 | 后续再考虑，本迭代不做 |

第 0 跳封板。命名与标识落地见 `docs/NAMING.md`。
