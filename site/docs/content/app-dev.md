---
title: 初始化与运行 App
status: published
description: 初始化项目，并通过 dev 或 Python 运行 App。
updated: 2026-11-21
---

## 项目初始化

### 1. levier init

在项目目录执行：

```sh
levier init
```

App 名默认取当前目录名。也可以显式指定：

```sh
levier init counter-agent
levier init --name counter-agent
```

### 2. 写入内容

```text
用户 profile：server、app_name、credentials、runtime、mcp_refs
项目 .levier.json：当前项目的 App 身份与默认值
默认凭据文件：0600，包含 App API key
```

默认凭据位置由 profile 决定；通常是用户配置目录下的
`<app-name>.json`。

### 3. 参数

| 参数 | 说明 |
|---|---|
| `[name]` / `--name` | App 名；默认当前目录 |
| `--server` | 控制面地址 |
| `--admin-token` | 创建 App 所需管理 token |
| `--runtime` | 默认 runtime_name |
| `--mcp-ref` | 逗号分隔的 MCP 引用 |
| `--rotate` | 已有 App 但 key 丢失时轮换 |
| `--force` | 覆盖已存在的凭据文件 |
| `--json` | 机器可读输出 |
| `--timeout` | 单次请求超时 |

### 4. 输出

> [!OUTPUT]
> ```text
> ready: counter-agent
> app:     app_01...
> server:  http://127.0.0.1:8080
> runtime: react1
> next:
>   levier dev            # serve this project's tools
>   levier run "hello"    # submit a run without flags
> ```

如果已有可用凭据，会显示 `app already initialized` 并复用。

### 5. 常见失败

| 现象 | 动作 |
|---|---|
| `credentials already exist` | 用 `--force` 覆盖，或 `--rotate` 轮换 |
| `admin token is required` | `levier up` 一次，或设置 `LEVIER_ADMIN_TOKEN` |
| `app key could not be verified` | 检查 server 与凭据路径，重新 `init --rotate` |
| 项目名不对 | `levier init --name <name>` |

### 6. 多项目

每个项目目录单独执行 `levier init`。项目 `.levier.json` 会覆盖用户 profile
中的 App 身份和 runtime 默认值。详见 [项目 .levier.json](pre-profile.md)。

## dev 与 serve

### 1. levier dev

```sh
levier dev
```

默认查找：

```text
./app.py
./levier_app.py
```

它会注入：

```text
LEVIER_CREDENTIALS
LEVIER_SERVER
LEVIER_APP_MCP_REFS
```

> [!OUTPUT]
> ```text
> app:     /path/to/app.py
> server:  http://127.0.0.1:8080
> ```

进程必须保持运行；工具 handler 在 App 进程里执行。

### 2. 参数

| 参数 | 说明 |
|---|---|
| `--file` | 指定 App 文件 |
| `--python` | 指定 Python 可执行文件 |
| `--server` | 覆盖控制面地址 |
| `--credentials` | 覆盖凭据路径 |
| `--mcp-ref` | 覆盖 MCP 引用 |

### 3. 直接 python

```sh
python app.py
```

`App.run()` 不带 prompt 时会连接并服务工具调用；效果与 `levier dev` 相同。
前提是当前 Python 已安装 SDK，并且项目已 `levier init`。

### 4. serve 生命周期

```python
app = App(tools=[counter_add])
app.run()          # 阻塞，服务工具
```

停止：

```text
Ctrl+C
```

或代码中调用：

```python
app.stop()
```

### 5. 一次性启动

不想手动开两个终端：

```sh
levier run "Add 2 and 3." --with-app
```

`--with-app` 会在 Run 期间临时启动 `./app.py`；如果已有 App 连接，不会重复启动。

### 6. 常见失败

| 现象 | 先检查 |
|---|---|
| `no app.py found` | 用 `--file` 指定，或创建 `app.py` |
| `no App is initialized` | 先 `levier init` |
| `ModuleNotFoundError: levier` | 当前 Python 没装 SDK |
| 工具没被调用 | App 是否在线，`levier status` 的 websocket 数 |

## 下一步

- App 核心流程：[App 核心流程](app-core.md)
- Run：[Run 与事件](app-runs.md)
- App 管理：[App 管理](app-manage.md)
