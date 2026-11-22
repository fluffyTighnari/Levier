---
title: MCP 配置
status: published
description: 注册 MCP Server，配置 transport、secret 与 allowed_tools。
updated: 2026-11-21
---

## MCP Server

> [!NOTE]
> App 不实现 MCP client，只声明引用。真正的连接由共享的 MCP Runtime 完成。

### 1. 谁负责什么

```text
App           只声明 mcp_refs
Levier        保存 MCP Server 与 allowed_tools
MCP Runtime   连接 MCP Server、解析 secret 占位
Runtime       通过 ctx.toolbox() 使用解析后的工具
```

### 2. mcp add

```sh
levier mcp add \
  --name echo \
  --transport stdio \
  --config-file ./examples/mcp_echo/config.json
```

> [!OUTPUT]
> ```text
> MCP server saved: echo (stdio)
> ```

参数：

| 参数 | 必填 | 说明 |
|---|---|---|
| `--name` | 是 | App 引用的名字 |
| `--config-file` | 是 | MCP Server 自身的 JSON 配置 |
| `--transport` | 否 | `stdio`（默认）、`sse`、`streamable_http` |
| `--allowed-tools` | 否 | 逗号分隔的工具白名单 |
| `--server` | 否 | 控制面地址 |
| `--admin-token` | 否 | 管理 token |
| `--json` | 否 | 机器可读输出 |

`mcp add` 是管理操作；本地栈由 `levier up` 写入 profile 后可以直接执行。

### 3. mcp list

```sh
levier mcp list
```

> [!OUTPUT]
> ```text
> name                 transport          secret_refs
> echo                 stdio
> ```

只显示 server 元数据，不显示 secret 值。

### 4. mcp remove

```sh
levier mcp remove --name echo --yes
```

如果已有 App 仍引用它：

```sh
levier mcp remove --name echo --yes --force
```

`--force` 会同时清理 App 引用。删除后 App 下次连接或初始化会失败，
直到引用被移除。

### 5. 下一步

- 写 config、secret 占位与 allowed_tools：见下文。
- App 声明引用：[MCP 引用](app-mcp.md)
- Runtime 使用工具：[Toolbox](runtime-toolbox.md)
- 失败码：[错误与排查](errors.md)

## 配置与工具

### 1. stdio config

```json
{
  "command": "npx",
  "args": ["-y", "@modelcontextprotocol/server-github"],
  "env": {
    "GITHUB_PERSONAL_ACCESS_TOKEN": "${secret:github_token}"
  }
}
```

```sh
levier mcp add \
  --name github \
  --transport stdio \
  --config-file ./github.mcp.json \
  --allowed-tools search_repositories,get_file_contents
```

### 2. remote config

```json
{
  "url": "https://mcp.example.com/mcp",
  "headers": {
    "Authorization": "Bearer ${secret:remote_token}"
  }
}
```

```sh
levier mcp add \
  --name remote \
  --transport streamable_http \
  --config-file ./remote.mcp.json
```

`transport` 可选：

```text
stdio             子进程 + stdin/stdout
sse               Server-Sent Events
streamable_http   HTTP 流式传输
```

### 3. secret 占位

`${secret:name}` 由 MCP Runtime 在连接前解析，不会以明文进入 Runtime 或事件。

来源：

```text
LEVIER_SECRET_<NAME>
LEVIER_MCP_SECRETS_FILE 指向的 0600 JSON 文件
```

示例：

```json
{
  "github_token": "ghp_...",
  "remote_token": "..."
}
```

### 4. allowed_tools

`allowed_tools` 是 MCP Runtime 强制的白名单：

```sh
levier mcp add \
  --name github \
  --transport stdio \
  --config-file ./github.mcp.json \
  --allowed-tools search_repositories,get_file_contents
```

白名单之外的工具返回 `TOOL_DENIED`。不传 `allowed_tools` 时由 MCP Runtime
按 server 暴露的工具处理。

### 5. 工具命名

模型侧名字固定为：

```text
mcp__<server>__<tool>
```

例如：

```text
mcp__github__search_repositories
```

App 和 Runtime 都不需要自己拼接这个名字；`ctx.toolbox()` 已经处理。

### 6. 失败码

| code | 含义 |
|---|---|
| `MCP_RUNTIME_UNAVAILABLE` | 共享 MCP Runtime 没启动 |
| `MCP_CONNECT_FAILED` | MCP Server 启动/连接失败 |
| `SECRET_MISSING` | `${secret:...}` 没有对应值 |
| `TOOL_DENIED` | 工具不在 `allowed_tools` |
| `TOOL_TIMEOUT` | MCP 工具调用超时 |
| `MCP_RUNTIME_ERROR` | MCP Runtime 返回错误 |

一个 MCP Server 失败不需要重启控制面或 Runtime；它们是独立失败域。

## 下一步

- 模型：[模型配置](pre-model.md)
- 密钥：[环境与密钥](pre-env.md)
- App 侧引用：[MCP 引用](app-mcp.md)
