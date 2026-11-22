---
title: 错误与排查
status: published
description: 退出码、常见错误与先看什么。
updated: 2026-11-21
---

## 1. 退出码

```text
0  success
1  operation failed
2  invalid command / arguments
3  authentication / authorization failure
4  retryable / upstream failure
5  refused or timed out
```

先看退出码，再看错误文本。

## 2. 启动

```sh
levier status
levier logs
levier doctor
```

| 现象 | 先检查 |
|---|---|
| connection refused | 进程/端口 |
| health fail | 控制面日志 |
| ready fail | 数据库 / 迁移 |
| daemons connected=0 | Runtime 是否启动 |
| 端口占用 | 8080 / 9090 |

## 3. 模型

```sh
levier model list
levier model check --name <name>
```

| code / 现象 | 动作 |
|---|---|
| `MODEL_PROVIDER_NOT_FOUND` | `levier model connect` |
| `SECRET_MISSING` | 重新 connect / 检查 secret 文件 |
| 401 / 403 | 换 key |
| model not found | 检查 `--models` / `--model` |

## 4. App

```sh
levier app check
levier run show <run_id> --events
```

| code | 动作 |
|---|---|
| `UNAUTHORIZED` | 重新 init / 检查凭据 |
| `TOOL_NOT_FOUND` | 工具名、App 是否在线 |
| `TOOL_TIMEOUT` | handler 日志、timeout |
| `TOOL_EXEC_ERROR` | App 日志 |
| `APP_UNREACHABLE` | 重启 App，重新提交 |

## 5. MCP

| code | 动作 |
|---|---|
| `MCP_RUNTIME_UNAVAILABLE` | 启动 MCP Runtime |
| `MCP_CONNECT_FAILED` | MCP Server 配置/启动 |
| `SECRET_MISSING` | `${secret:...}` 是否有值 |
| `TOOL_DENIED` | `allowed_tools` |
| `TOOL_TIMEOUT` | MCP Server / 网络 |
| `MCP_RUNTIME_ERROR` | MCP Runtime 日志 |

## 6. Runtime

```sh
levier runtime list
levier logs
```

| 现象 | 先检查 |
|---|---|
| Runtime 未出现 | token / gRPC / 进程 |
| 一直 queued | 容量 / 心跳 |
| failed + ERROR | `run show --events` |
| 取消不生效 | Runtime 是否检查 `ctx.cancelled` |

## 7. 提交 issue

附上：

```text
levier version
levier status
levier run show <run_id> --events
levier doctor
相关容器最近 100 行日志
执行的完整命令
```

隐去 key、token、业务数据。

## 下一步

- CLI：[CLI 总览](cli.md)
- 状态：[状态](levier-status.md)
- 日志：[日志与进程](levier-status.md)
