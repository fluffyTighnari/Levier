---
title: 环境变量
status: published
description: 用户级、项目级、CLI、SDK 与部署环境变量。
updated: 2026-11-21
---

## 1. 优先级

```text
命令行 flag
  > 环境变量
  > 项目 .levier.json
  > 用户 profile
  > 内置默认值
```

`LEVIER_HOME` 直接决定 profile / 数据目录。

## 2. 用户与项目

| 变量 | 用途 |
|---|---|
| `LEVIER_HOME` | profile / 数据目录 |
| `LEVIER_SERVER` | 默认 HTTP 地址 |
| `LEVIER_ADMIN_TOKEN` | 管理操作 |
| `LEVIER_CREDENTIALS` | App 凭据文件 |
| `LEVIER_APP_CREDENTIALS` | 同上 |
| `LEVIER_APP_NAME` | 默认 App 名 |
| `LEVIER_APP_MCP_REFS` | MCP 引用 |
| `LEVIER_MODEL_API_KEY` | model connect 的 key 替代 |
| `LEVIER_RUNTIME_NAME` | 默认 runtime_name |

## 3. CLI

| 变量 | 用途 |
|---|---|
| `LEVIER_PYTHON` | dev / runtime start 的 Python |
| `LEVIER_STACK_FILE` | Compose 文件 |
| `LEVIER_CONFIG` | 控制面 YAML |
| `LEVIER_GRPC` | daemon check 的 gRPC |
| `LEVIER_DAEMON_ID` | daemon check |
| `LEVIER_DAEMON_TOKEN` | daemon check |

## 4. App SDK

```text
LEVIER_CREDENTIALS
LEVIER_APP_MCP_REFS
LEVIER_MODEL_API_KEY
```

通常由 `levier init` / `levier dev` 自动注入。

## 5. Runtime

| 变量 | 用途 |
|---|---|
| `LEVIER_RUNTIME_CONFIG` | Runtime YAML |
| `LEVIER_MIDDLEWARE_ADDRESS` | 控制面 gRPC |
| `LEVIER_RUNTIME_NAME` | 路由名 |
| `LEVIER_RUNTIME_INSTANCE_ID` | 实例 ID |
| `LEVIER_RUNTIME_TOKEN` | runtime token |
| `LEVIER_RUNTIME_TOKEN_FILE` | token 文件 |
| `LEVIER_RUNTIME_TYPE` | Runtime 类型 |
| `LEVIER_RUNTIME_MAX_CONCURRENT_RUNS` | 并发容量 |
| `LEVIER_RUNTIME_LABELS` | 标签 |
| `LEVIER_RUNTIME_LOG_LEVEL` | 日志级别 |
| `LEVIER_MCP_RUNTIME_SOCKET` | MCP 地址 |
| `LEVIER_MCP_SOCKET` | MCP socket |
| `LEVIER_MCP_ADDR` | MCP TCP |
| `LEVIER_MCP_SECRETS_FILE` | MCP secret 文件 |

## 6. 控制面与部署

```text
LEVIER_DATABASE_URL / LEVIER_DATABASE_URL_FILE
LEVIER_HTTP_ADMIN_TOKEN / LEVIER_HTTP_ADMIN_TOKEN_FILE
LEVIER_GRPC_DAEMON_TOKENS_FILE
LEVIER_ENVIRONMENT
LEVIER_LOG_LEVEL / LEVIER_LOG_FORMAT
LEVIER_RETENTION_ENABLED
LEVIER_RETENTION_RUN_DAYS
LEVIER_RETENTION_SESSION_DAYS
LEVIER_RETENTION_BATCH_SIZE
```

## 7. secret

```text
LEVIER_SECRET_<REF>
LEVIER_SECRETS_FILE
LEVIER_SECRETS_WRITE_FILE
LEVIER_MCP_SECRETS_FILE
```

secret 值只应来自 0600 文件或环境变量，不要写进 Compose 明文或命令行历史。

## 下一步

- 配置层级：[配置与优先级](pre-env.md)
- 凭据：[凭据与密钥](pre-env.md)
- 远程部署：[远程控制面](levier-remote.md)
