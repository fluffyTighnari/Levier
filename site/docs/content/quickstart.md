---
title: 快速开始
status: published
description: 用内置 Echo Runtime 跑通第一次 Run，不需要模型 key。约 5 分钟。
updated: 2026-11-20
---

> [!NOTE]
> 读完之后：你会得到一个 `completed` 的 Run、它的 run_id，以及 3 条事件。
> 全程不需要模型 key，也不会调用 MCP。

## 1. 准备

- 已安装并启动 **Docker Desktop**。
- 终端位于仓库根目录。
- Windows 用 PowerShell 或 cmd；Linux / macOS 用 sh。

> [!TIP]
> 下文命令用 `./bin/levier`；Windows 换成 `.\bin\levier.exe`。

```sh
cd /path/to/middleware
# Windows: cd D:\Workspace\Code\middleware
```

复制本地环境文件：

```powershell
# Windows
Copy-Item .env.example .env
```

```sh
# Linux / macOS
cp .env.example .env
```

> [!TIP]
> `.env` 只用于本地开发，不要提交。里面的 `LEVIER_ADMIN_TOKEN=admin-change-me`
> 也是开发默认值，生产环境必须换掉。

## 2. 启动本地栈

```sh
docker compose up -d --build postgres levier mcp-runtime react-runtime echo-runtime
```

第一次需要 `--build`，之后可以省略。各服务的分工：

| 服务 | 作用 | 这一页是否需要 |
|---|---|---|
| `postgres` | 保存 Run、App、事件 | 是 |
| `levier` | 控制面，接收请求并调度 | 是 |
| `echo-runtime` | 内置 Echo Runtime | 是 |
| `react-runtime` | 内置 ReAct Runtime | 否，下一步再用 |
| `mcp-runtime` | 共享 MCP 进程 | 否 |

## 3. 确认控制面可用

```sh
./bin/levier status
```

Windows：

```powershell
.\bin\levier.exe status
```

> [!OUTPUT]
> `health`、`ready`、`metrics` 三项都是 `ok`：
>
> ```text
> server      http://127.0.0.1:8080
> health      ok
> ready       ok
> metrics     ok
> daemons     ok 2 (capacity 8, used 0)
> apps        ok 0 websocket
> runs        ...
> ```

> [!FAIL]
> 连接被拒绝：先等 10 秒，再用 `docker compose ps` 看容器是否都在运行。
> 如果 `health` 不是 `ok`，看控制面日志：
>
> ```sh
> docker compose logs --tail=100 levier
> ```

## 4. 创建 App 凭据

```sh
./bin/levier app init \
  --name my-first-app \
  --server http://127.0.0.1:8080 \
  --admin-token admin-change-me \
  --credentials-out ./my-first-app.json
```

PowerShell 用反引号换行：

```powershell
.\bin\levier.exe app init `
  --name my-first-app `
  --server http://127.0.0.1:8080 `
  --admin-token admin-change-me `
  --credentials-out .\my-first-app.json
```

> [!OUTPUT]
> ```text
> app created: app_01...
> name: my-first-app
> server: http://127.0.0.1:8080
> credentials: ./my-first-app.json (0600)
> key fingerprint: ...
> ```

两个参数容易混：

| 参数 | 含义 |
|---|---|
| `--credentials-out` | 创建并写出凭据文件 |
| `--credentials` | 读取已有的凭据文件 |

> [!WARNING]
> `my-first-app.json` 里有一把 App API key，只在创建时返回一次。
> 不要提交到 Git，也不要打印到日志或聊天工具。

## 5. 提交第一次 Run

```sh
./bin/levier run submit \
  --credentials ./my-first-app.json \
  --runtime echo1 \
  --prompt "hello" \
  --watch
```

> [!OUTPUT]
> ```text
> run submitted: 01J...
> status: queued
> status: assigned
> status: running
> status: completed
> runtime_name: echo1
> final: hello
> ```

中间状态可能因为轮询间隔被跳过；只要最后是 `status: completed` 与 `final: hello` 就成功。
Echo 不调用模型，所以没有工具调用，也没有 token 用量。

## 6. 查看事件

```sh
./bin/levier run show \
  --credentials ./my-first-app.json \
  --run-id 01J... \
  --events
```

把 `01J...` 换成上一步真实的 run_id。

> [!OUTPUT]
> ```text
> run_id       01J...
> status       completed
> runtime      echo1
> [1] RUN_STATUS            daemon
> [2] MESSAGE_COMPLETED     daemon
> [3] RUN_STATUS            daemon
> ```

到这里，控制面、凭据、调度、事件四条链路都通了。

## 7. 换成真实模型（可选）

Echo 只能证明链路通。要看到模型调用工具，需要三件事：模型 key、一个已连接的 App、以及 `react1`。

先在 `.env` 里填入模型 key：

```sh
LEVIER_SECRET_DEEPSEEK_API_KEY=sk-...
```

重启控制面与 ReAct Runtime 让环境变量生效：

```sh
docker compose up -d levier react-runtime
```

注册 provider：

```sh
./bin/levier provider add \
  --name deepseek \
  --adapter openai_compatible \
  --base-url https://api.deepseek.com \
  --default-model deepseek-flash \
  --secret-ref deepseek_api_key \
  --default
```

启动示例 App（另开一个终端）。它注册 `counter_add`、`counter_get`、`counter_reset`：

```sh
python examples/counter_agent/app.py --credentials ./my-first-app.json
```

> [!FAIL]
> 报 `No module named levier_app`：先按 [App SDK](app-sdk.md) 的「安装」装好 SDK，
> 再用同一个虚拟环境的 Python 启动。

> [!OUTPUT]
> 再执行一次 `./bin/levier status`，`apps` 应该至少有 1 条 websocket。

提交一次真实 Run：

```sh
./bin/levier run submit \
  --credentials ./my-first-app.json \
  --runtime react1 \
  --prompt "Add 2, then add 3, then read the counter." \
  --watch
```

> [!OUTPUT]
> ```text
> run submitted: 01J...
> status: queued
> status: completed
> runtime_name: react1
> final: 5
> ```

要让模型真正调用**你的**函数，继续看 [App SDK](app-sdk.md)。

## 8. 下一步

- 分清 App、Levier、Runtime 与 MCP：[核心概念](concepts.md)。
- 把已有业务函数接成工具：[App SDK](app-sdk.md)。
- 写一个自己的 Runtime：[Runtime SDK](runtime-sdk.md)。
- 需要外部工具：[MCP](mcp.md)。
- 卡住了：[排查](troubleshooting.md)。
