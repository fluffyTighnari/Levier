---
title: 环境与密钥
status: published
description: 环境变量优先级、App key、模型 key 与 managed secret。
updated: 2026-11-21
---

## 环境变量与优先级

### 1. 优先级

```text
命令行 flag
  > 环境变量
  > 项目 .levier.json
  > 用户 profile
  > 内置默认值
```

`LEVIER_HOME` 是例外：它直接决定 profile 和数据目录。

### 2. 用户与项目

```text
LEVIER_HOME          覆盖 profile / 数据目录
LEVIER_SERVER        默认 HTTP 地址
LEVIER_ADMIN_TOKEN   管理操作 token
LEVIER_CREDENTIALS   App 凭据文件
LEVIER_APP_CREDENTIALS  同 LEVIER_CREDENTIALS
LEVIER_APP_NAME      默认 App 名
LEVIER_APP_MCP_REFS  逗号分隔的 MCP 引用
LEVIER_RUNTIME_NAME  默认 runtime_name
```

### 3. CLI

```text
LEVIER_PYTHON        levier dev / runtime start 使用的 Python
LEVIER_STACK_FILE    levier up/down/logs/ps 的 Compose 文件
LEVIER_CONFIG        levier serve 的控制面 YAML
LEVIER_GRPC          daemon check / selfcheck 的 gRPC 地址
LEVIER_DAEMON_ID     daemon check / selfcheck 的 daemon id
LEVIER_DAEMON_TOKEN  daemon check / selfcheck 的 daemon token
```

### 4. App

```text
LEVIER_CREDENTIALS       App 凭据文件
LEVIER_APP_MCP_REFS      逗号分隔的 MCP 引用
LEVIER_MODEL_API_KEY     model connect 的 key 替代
```

App SDK 也会读取项目 `.levier.json` 和用户 profile，所以通常不需要在
App 进程里设置这些变量。

### 5. Runtime

```text
LEVIER_RUNTIME_CONFIG           Runtime YAML
LEVIER_MIDDLEWARE_ADDRESS       控制面 gRPC 地址
LEVIER_RUNTIME_NAME             路由名
LEVIER_RUNTIME_INSTANCE_ID      实例 ID
LEVIER_RUNTIME_TOKEN            runtime 认证
LEVIER_RUNTIME_TOKEN_FILE       runtime token 文件
LEVIER_RUNTIME_TYPE             normal / 其他
LEVIER_RUNTIME_MAX_CONCURRENT_RUNS  并发容量
LEVIER_RUNTIME_LABELS          标签
LEVIER_RUNTIME_LOG_LEVEL        日志级别
LEVIER_MCP_RUNTIME_SOCKET       MCP Runtime 地址
LEVIER_MCP_SOCKET               MCP socket
LEVIER_MCP_ADDR                 MCP TCP 地址
LEVIER_MCP_SECRETS_FILE         MCP secret JSON
```

`levier runtime start` 会根据 profile 注入地址、名称和 token；只有远程部署
才需要显式设置。

### 6. 控制面与部署

```text
LEVIER_DATABASE_URL / _FILE
LEVIER_HTTP_ADMIN_TOKEN / _FILE
LEVIER_GRPC_DAEMON_TOKENS_FILE
LEVIER_ENVIRONMENT
LEVIER_LOG_LEVEL / LEVIER_LOG_FORMAT
LEVIER_RETENTION_ENABLED
LEVIER_RETENTION_RUN_DAYS
LEVIER_RETENTION_SESSION_DAYS
LEVIER_RETENTION_BATCH_SIZE
```

这些是控制面进程配置，不是 App / Runtime 的日常接入参数。

### 7. secret 变量

```text
LEVIER_SECRET_<REF>        单个模型 secret
LEVIER_SECRETS_FILE        控制面只读 secret JSON
LEVIER_SECRETS_WRITE_FILE  控制面 managed secret JSON
```

不要把 secret 写进长期运行的 App 环境；使用 `model connect` 或 0600 文件。

## 凭据与密钥

### 1. 两类密钥

| 密钥 | 用途 | 存在哪里 |
|---|---|---|
| App API key | App 连接 Levier、提交/查询 Run | 0600 凭据文件 |
| 模型 API key | Runtime 调模型 | 控制面 managed secret |

App key 不等于模型 key。模型 key 不会出现在 App 凭据、Run 事件或 WebSocket 消息里。

### 2. managed secret

`levier model connect` 会把模型 key 写入控制面的 managed secret 文件：

```text
LEVIER_SECRETS_WRITE_FILE
```

控制面读取时可以使用：

```text
LEVIER_SECRET_<REF>
LEVIER_SECRETS_FILE
```

推荐：

```json
{
  "deepseek_api_key": "sk-..."
}
```

文件权限 0600，不提交、不打印、不进日志。

### 3. App 凭据

`levier init` 写入的凭据文件包含：

```text
app_id
app_name
server
api_key
created_at
```

App API key 只在创建 / 轮换时返回一次；数据库只保存 hash。

### 4. 轮换与恢复

轮换：

```sh
levier app rotate-key --credentials ./data/agent/credentials.json --yes
```

旧 key 立即失效，凭据文件原子更新，并保留受保护的 `.bak` 备份。

凭据丢失：

```sh
levier init --rotate
```

或低层命令：

```sh
levier app init --name <app> --credentials-out <path> --rotate --yes
```

不要删除数据库或凭据卷来“重新开始”。

### 5. 权限与日志

- 凭据文件 0600；目录 0700。
- Admin token 只用于创建/轮换 App，不进入运行时。
- 模型 key 不通过 CLI 输出、`/meta`、Run event 或 checkpoint 暴露。
- `--json` 也不会输出 secret 值。

### 6. 远程部署

生产建议：

```sh
levier setup \
  --server https://levier.example.com \
  --admin-token <admin-token>
```

模型 secret 使用 0600 文件或外部 secret 管理；runtime token 通过
`levier setup --runtime-token` 注入一次。详见 [远程控制面](levier-remote.md)。

## 下一步

- 用户与项目配置：[用户与项目配置](pre-profile.md)
- 模型：[模型配置](pre-model.md)
- MCP：[MCP 配置](pre-mcp.md)
