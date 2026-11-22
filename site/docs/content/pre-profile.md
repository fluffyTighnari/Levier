---
title: 用户与项目配置
status: published
description: 用户 profile、项目 .levier.json 与配置路径。
updated: 2026-11-21
---

## 用户 profile

> [!NOTE]
> profile 保存的是 locator（地址、路径、runtime 名），不是 App key。
> Admin / runtime token 只有显式传入时才会写入。

### 1. 路径

```text
Windows        %APPDATA%\levier\config.yaml
Linux / macOS  ~/.config/levier/config.yaml
```

`LEVIER_HOME` 可以覆盖整个目录：

```sh
LEVIER_HOME=/tmp/levier-home levier setup --show
```

profile 以 0600 写入。

### 2. 字段

```yaml
server: http://127.0.0.1:8080
credentials: /home/me/.config/levier/counter-agent.json
app_name: counter-agent
mcp_refs: echo
runtime: react1
runtime_token: change-me
middleware_address: 127.0.0.1:9090
runtime_config: /path/to/runtime.yaml
mcp_socket: tcp://127.0.0.1:9091
control_config: /path/to/levier.yaml
stack_file: /path/to/docker-compose.yml
```

| 字段 | 用途 |
|---|---|
| `server` | 默认 HTTP 地址 |
| `credentials` | 默认 App 凭据文件 |
| `app_name` | 默认 App 名 |
| `mcp_refs` | 默认 MCP 引用 |
| `runtime` | 默认 runtime_name |
| `runtime_token` | 本地 runtime 认证 |
| `middleware_address` | Runtime gRPC 地址 |
| `runtime_config` | 可选 Runtime YAML |
| `mcp_socket` | MCP Runtime 地址 |
| `control_config` | 控制面 YAML |
| `stack_file` | Compose 文件 |

### 3. setup 与 show

```sh
levier setup --server http://127.0.0.1:8080
levier setup --show
```

`--show` 只打印，不写入。输出包括：

```text
profile / server / runtime / grpc / credentials
runtime config / mcp_socket / control / stack
```

连接已有控制面：

```sh
levier setup \
  --server https://levier.example.com \
  --admin-token <admin-token>
```

只需要 App 功能时，也可以只写：

```sh
levier setup --server https://levier.example.com
```

### 4. reset

```sh
levier setup --reset
```

清空已有 profile 值后重新写入。不会删除 App 凭据、数据库或 Compose 数据卷。

### 5. 优先级

命令行 flag > 环境变量 > 项目 `.levier.json` > 用户 profile > 内置默认值。
详见 [环境变量与优先级](pre-env.md)。

## 项目 .levier.json

### 1. 生成方式

在项目目录执行：

```sh
levier init
```

它会创建或复用 App，并写入：

```text
用户 profile
当前项目的 .levier.json
默认 0600 凭据文件
```

### 2. 字段

```json
{
  "app_name": "counter-agent",
  "credentials": "/home/me/.config/levier/counter-agent.json",
  "server": "http://127.0.0.1:8080",
  "runtime": "react1",
  "mcp_refs": "echo"
}
```

| 字段 | 用途 |
|---|---|
| `app_name` | 当前项目的 App 身份 |
| `credentials` | App 凭据文件 |
| `server` | 控制面地址 |
| `runtime` | 默认 runtime_name |
| `mcp_refs` | 逗号分隔的 MCP 引用 |

`.levier.json` 只保存 locator，不保存 API key。

### 3. 优先级

读取顺序：

```text
命令行 flag
  > 环境变量
  > 项目 .levier.json
  > 用户 profile
  > 内置默认值
```

因此同一个用户可以在不同项目里使用不同 App、runtime 和 MCP 引用。

### 4. 多项目

每个业务目录执行一次：

```sh
cd project-a && levier init
cd project-b && levier init
```

每个目录保留自己的 `.levier.json`。`levier dev` / `levier run` 在当前目录读取它。

如果项目要使用不同的凭据文件：

```sh
levier init --name project-b-app --force
```

或显式指定：

```sh
levier dev --credentials ./project-b.json
```

### 5. 排查

| 现象 | 先检查 |
|---|---|
| `no App is initialized` | 当前目录是否有 `.levier.json`，或先 `levier init` |
| 用错 App | `.levier.json` 的 `app_name` / `credentials` |
| 用错 runtime | `.levier.json` 的 `runtime` |
| MCP 引用失败 | `.levier.json` 的 `mcp_refs` 是否已注册 |

## 下一步

- 环境与密钥：[环境与密钥](pre-env.md)
- 控制面 setup：[setup 与 profile](levier-install.md)
- 完整接入：[完整接入](onboarding.md)
