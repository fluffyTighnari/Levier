---
title: MCP
status: published
description: 注册 MCP Server、声明 mcp_refs、理解工具命名与凭据占位。
updated: 2026-11-20
---

> [!NOTE]
> 读完之后：你能注册一个 MCP Server，让 App 声明引用，并在 Runtime 侧看到
> `mcp__<server>__<tool>` 形式的工具。

## 1. 谁负责什么

MCP Server 由独立进程 `mcp_runtime` 连接。App 只声明引用；Runtime 收到解析后的 bindings。

```text
App           只声明 mcp_refs
Levier        注册 MCP Server，保存引用与配置
MCP Runtime   连接、会话、凭据占位解析、allowed_tools
Runtime       使用解析后的工具绑定
```

App 代码不实现 MCP client。

## 2. 注册 MCP Server

```sh
levier mcp add \
  --name e2e-echo \
  --transport stdio \
  --config-file ./mcp-echo.json \
  --allowed-tools echo_token
```

`mcp-echo.json` 是 MCP Server 自身的配置：

```json
{
  "command": "python",
  "args": ["path/to/your_mcp_server.py"],
  "env": {
    "TOKEN": "${secret:e2e_token}"
  }
}
```

> [!NOTE]
> `mcp add` 属于管理操作，需要 Admin token（`--admin-token` 或 `LEVIER_ADMIN_TOKEN`）。

## 3. App 声明引用

```sh
levier app init \
  --name counter-agent \
  --mcp-ref e2e-echo \
  --credentials-out ./data/agent/credentials.json
```

或：

```sh
export LEVIER_APP_MCP_REFS=github,e2e-echo
```

`mcp_ref` 必须已经注册，否则 App 初始化或连接会失败。

## 4. 模型侧工具名

```text
mcp__<server>__<tool>
```

例如：

```text
mcp__e2e_echo__echo_token
```

## 5. 凭据

配置中的 `${secret:name}` 由 MCP Runtime 解析，占位符不会展开成明文进入 Runtime 或事件。

```json
{
  "env": {
    "TOKEN": "${secret:e2e_token}"
  }
}
```

> [!FAIL]
> 错误码是 `SECRET_MISSING`：说明 `${secret:...}` 引用的名字没有对应值。
> 检查 `LEVIER_SECRET_<UPPER_REF>` 或 `LEVIER_SECRETS_FILE`。

## 6. 执行路径与记录

```text
Runtime → mcp_runtime → MCP Server
```

MCP 调用**不经过** Levier Gateway，也不写 `tool_invocations`。`allowed_tools` 由 MCP Runtime 强制。

## 7. 排查

| 现象 | 先检查 |
|---|---|
| App 启动即失败 | `mcp_ref` 是否已注册 |
| 模型看不到工具 | `allowed_tools` 是否包含该工具 |
| 工具执行报错 | MCP Server 自身日志与 `mcp_runtime` 日志 |
| 凭据未生效 | `${secret:name}` 与 `LEVIER_SECRET_*` 是否一致 |

## 8. 下一步

- 跑通 MCP 变体：仓库 `docs/PLAN_EXECUTE.md` 第 7 节。
- App 侧工具：[App SDK](app-sdk.md)。
- Runtime 侧调用：[Runtime SDK](runtime-sdk.md)。
