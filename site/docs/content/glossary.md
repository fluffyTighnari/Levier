---
title: 术语表
status: published
description: App、Runtime、profile、managed secret 等术语。
updated: 2026-11-21
---

## 1. 角色

| 术语 | 含义 |
|---|---|
| **App** | 持有业务状态、注册工具、执行 handler 的进程 |
| **Levier** | 控制面：鉴权、调度、Run 状态、事件、模型/MCP 目录 |
| **Runtime** | 执行 `run(ctx)`，决定模型与工具怎么用 |
| **MCP Runtime** | 独立进程，连接 MCP Server，供 Runtime 共享 |
| **Run** | 一次从提交到终态的执行 |

## 2. 配置

| 术语 | 含义 |
|---|---|
| **用户 profile** | 用户级配置，保存 server、凭据路径、runtime 等 locator |
| **`.levier.json`** | 项目级配置，`levier init` 生成 |
| **App key** | App 连接 Levier 的 API key |
| **admin token** | 管理操作 token |
| **runtime token** | Runtime 连接控制面的 token |
| **managed secret** | 控制面管理的模型 key 文件 |
| **`mcp_ref`** | App 声明的 MCP Server 引用 |
| **`allowed_tools`** | MCP Runtime 强制的工具白名单 |

## 3. 运行

| 术语 | 含义 |
|---|---|
| **`run(ctx)`** | Runtime 的唯一入口函数 |
| **Context** | 本次 Run 的输入、模型、工具、超时、取消状态 |
| **Toolbox** | 本地 + MCP 工具的统一视图 |
| **`ctx.chat`** | 用本次模型配置调模型 |
| **`ctx.invoke_all`** | 执行模型请求的所有工具，local/MCP 自动分流 |
| **checkpoint** | Runtime 保存的状态快照 |
| **terminal** | `completed` / `failed` / `cancelled` / `timeout` |

## 4. 兼容词

这些是内部/旧概念，不再是接入契约：

```text
provider                 已收敛到 model
daemon                   对应 Runtime 实例
secret_ref               provider 内部引用
ws_token                 App WebSocket 短期 token
lease_epoch              Runtime 租约版本
event seq / ack          事件序号与确认
local_tool_call/result   App 工具帧
RuntimeChannel           Runtime gRPC 连接
Hello / Heartbeat        Runtime 握手与心跳
```

需要底层排查时再查架构或协议文档。

## 5. 选择

```text
只想接业务函数        App
想控制 Agent 怎么跑     Runtime
只验证链路             echo1
常规模型 + 工具循环    react1
```

## 下一步

- 接入：[完整接入](onboarding.md)
- CLI：[CLI 总览](cli.md)
- SDK：[SDK 总览](sdk.md)
