---
title: 安装、连接与启动
status: published
description: 安装 CLI、连接 profile 并启动本地栈。
updated: 2026-11-21
---

## 安装与升级

### 1. 发布版

Linux / macOS：

```sh
curl -fsSL https://raw.githubusercontent.com/fluffyTighnari/levier/main/scripts/install.sh | sh
```

固定版本：

```sh
sh scripts/install.sh --version v0.1.0
```

Windows：

```powershell
.\scripts\install.ps1
```

固定版本与目录：

```powershell
.\scripts\install.ps1 -Version v0.1.0 -Prefix "$env:USERPROFILE\.local\bin"
```

安装器会：

```text
把 levier 放进 PATH
下载 Compose 源码包
安装 Python SDK（from levier import App, tool, Runtime）
```

### 2. 源码

```sh
make install
levier setup --server http://127.0.0.1:8080
```

`setup` 写入用户 profile，不是项目配置。

### 3. 升级

重复执行同一条安装命令即可。生产环境建议固定版本：

```sh
sh scripts/install.sh --version v0.1.0
```

升级后确认：

```sh
levier version
```

### 4. 验证

```sh
levier version
```

> [!OUTPUT]
> ```text
> levier v0.1.0
> commit: ...
> platform: ...
> ```

如果 `levier` 不在 PATH，重新打开终端或检查安装目录。

### 5. 卸载与清理

删除安装目录中的 `levier` 可执行文件即可。用户 profile 与数据目录不会自动删除：

```text
~/.config/levier/
%APPDATA%\levier\
```

需要彻底清理时再手动删除；不要误删仍在使用中的凭据。

## 启动

### 1. levier up

```sh
levier up
```

它会：

```text
启动 PostgreSQL
启动 Levier 控制面
启动 MCP Runtime
启动内置 React Runtime
启动内置 Echo Runtime
等待 /readyz
写入用户 profile
```

默认不启动示例 agent 容器。

> [!OUTPUT]
> ```text
> waiting for http://127.0.0.1:8080 ...
> levier is ready
> next:
>   levier model connect ...
>   levier init
> ```

### 2. 参数

| 参数 | 说明 |
|---|---|
| `--file` | 指定 Compose 文件 |
| `--server` | 覆盖等待的 HTTP 地址 |
| `--profile` | 只启动指定服务 profile |
| `--build` | 构建镜像 |
| `--all` | 启动全部服务，包括示例 agent |
| `--wait` | 等待就绪 |
| `--dry-run` | 只打印将要执行的命令 |
| `--timeout` | 等待超时 |

### 3. 等待就绪

`levier up` 会轮询 `/readyz`。看到 `levier is ready` 再继续。

如果超时：

```sh
levier status
levier logs --tail=100
```

### 4. profile

`up` 会写入：

```text
server
runtime
runtime_token
middleware_address
mcp_socket
stack_file
```

之后 `levier init`、`model connect`、`run` 都可以不再传地址和 token。

### 5. 直接启动控制面

已有部署或需要自定义 YAML 时：

```sh
levier serve --config ./levier.yaml
```

这是高级入口；日常本地栈使用 `levier up`。

## setup 与 profile

### 1. levier setup

```sh
levier setup --server http://127.0.0.1:8080
```

写入用户 profile。之后 `init`、`model connect`、`run` 自动读取。

连接已有控制面：

```sh
levier setup \
  --server https://levier.example.com \
  --admin-token <admin-token>
```

### 2. 参数

| 参数 | 说明 |
|---|---|
| `--server` | 控制面 HTTP 地址 |
| `--admin-token` | 写入 profile 的 admin token |
| `--credentials` | 默认 App 凭据路径 |
| `--runtime` | 默认 runtime_name |
| `--grpc` | Runtime gRPC 地址 |
| `--runtime-token` | 写入 profile 的 runtime token |
| `--runtime-config` | 默认 Runtime YAML |
| `--mcp-socket` | MCP Runtime 地址 |
| `--control-config` | 控制面 YAML |
| `--stack-file` | Compose 文件 |
| `--reset` | 清空后写入 |
| `--show` | 只显示，不写入 |
| `--json` | 机器可读 |

### 3. show

```sh
levier setup --show
```

> [!OUTPUT]
> ```text
> profile      /home/me/.config/levier/config.yaml
> server       http://127.0.0.1:8080
> runtime      react1
> grpc         127.0.0.1:9090
> credentials  /home/me/.config/levier/counter-agent.json
> mcp_socket   tcp://127.0.0.1:9091
> ```

### 4. reset

```sh
levier setup --reset
```

清空 profile 值后重新写入。不会删除 App 凭据、数据库或数据卷。

### 5. 安全

profile 是 0600，但只应保存 locator。Admin / runtime token 只有显式传入
`--admin-token` / `--runtime-token` 时才写入；否则继续使用环境变量。

## 下一步

- 本地栈生命周期：[本地栈生命周期](levier-status.md)
- Run：[Run 提交与查看](levier-run.md)
- 远程控制面：[远程控制面](levier-remote.md)
