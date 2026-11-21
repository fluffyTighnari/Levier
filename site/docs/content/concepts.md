---
title: 核心概念
status: published
description: 四个角色、四条边界，以及一次 Run 的生命周期。先读这一页，后面的接入会简单很多。
updated: 2026-11-20
---

> [!NOTE]
> 这一页只回答三个问题：谁负责什么、边界在哪、一次 Run 怎么走。
> 不需要记住内部包名，也不需要先读架构文档。

## 1. 一句话

**Levier 是 Agent Run 的控制面，并随产品提供默认执行侧 Runtime 与默认 MCP 连接能力。**

控制面负责 Run 生命周期、调度、工具路由和事件；默认组件负责模型循环与 MCP 会话。
两者通过版本化契约解耦，都可以替换。

这样拆开的直接好处是：

- 换掉 App，不需要改 Runtime。
- 换掉 Runtime，不需要改 App。
- 升级 Levier，只依赖契约版本。

## 2. 四个角色

| 角色 | 负责 | 不负责 |
|---|---|---|
| **App** | 业务状态、本地工具处理器、一条 WebSocket | 模型循环、调度、MCP 客户端 |
| **Levier** | 注册、调度、租约、工具路由、事件持久化、认证 | 业务工具、MCP Server、业务 UI |
| **Runtime** | 单次 Run 的模型 / 工具循环 | 业务状态、控制面状态 |
| **MCP Runtime** | MCP 连接、会话、凭据占位解析、allowed_tools | 业务工具、控制面调度 |

PostgreSQL 保存目录、Run、任务、事件与调用记录。它是控制面的存储，不是第五个角色。

## 3. 四条边界

| 边界 | 回答的问题 | 当前决定 |
|---|---|---|
| 产品边界 | 用户到底在用什么 | 控制面 + 随附默认组件，不含参考示例 |
| 进程边界 | 哪些东西跑在不同进程 | 控制面、Runtime、MCP Runtime、App 各自独立 |
| 契约边界 | 升级时不能随便变的东西 | AppPort：HTTP / WS v1；DaemonPort：gRPC v1 |
| 部署边界 | 什么算一个部署单元 | MVP 用 Compose；后续单二进制 + 外部 PostgreSQL |

判断边界是否干净，只问三个问题：

```text
1. 换掉 App，Levier 需要改吗？                  不需要
2. 换掉默认 Runtime，App 需要改吗？             不需要
3. 升级 Levier，App / Runtime 只依赖契约吗？     是
```

## 4. 一次 Run 的生命周期

```text
queued → assigned → running → completed
   │         │         ├──→ failed
   │         │         ├──→ cancelled
   │         │         └──→ timeout
   │         ├──→ queued（daemon lost / reconcile）
   │         └──→ cancelled（user）
   └──→ cancelled / timeout
```

终态不可逆。`attempt` 与 `lease_epoch` 用于 fencing：旧执行的事件不会落库，
也不会覆盖新终态。接入时不需要理解它们，排查并发问题时再看
[架构](architecture.md)。

## 5. 三条工具路径

| 路径 | 经过 | 是否写 tool_invocations |
|---|---|---|
| local | Runtime → Gateway → App WebSocket → App handler | 是 |
| global | Runtime → Gateway → HTTP / webhook executor | 是 |
| MCP | Runtime → MCP Runtime → MCP Server | 否 |

App 有多条连接时，local 调用会广播，**App 必须按 `invocation_id` 幂等**。
工具默认超时 60s，最大 4m。

> [!TIP]
> 已有业务函数优先走 local：它就在你的进程里，不需要另外部署 HTTP 服务。
> 第三方能力优先走 MCP；MCP 不经过 Gateway，也不写 `tool_invocations`。

## 6. 术语

- **local tool**：在 App 进程里执行、通过 WebSocket 调用的工具。
- **global tool**：由控制面的 HTTP / webhook executor 执行的工具。
- **MCP tool**：在共享 MCP Runtime 中执行、模型侧名字为 `mcp__<server>__<tool>` 的工具。
- **secret_ref**：模型服务商密钥的引用名，不是密钥本身。
- **ws_token**：短期 WebSocket 凭据，默认 10 分钟，绑定 `app_id`。

## 7. 下一步

- 第一次跑通：[快速开始](quickstart.md)。
- 接已有业务函数：[App SDK](app-sdk.md)。
- 看控制面的包与依赖方向：[架构](architecture.md)。
