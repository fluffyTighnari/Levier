---
title: 本地栈生命周期
status: published
description: 查看状态、日志并停止本地栈。
updated: 2026-11-21
---

## 状态

### 1. levier status

```sh
levier status
```

> [!OUTPUT]
> ```text
> server      http://127.0.0.1:8080
> status      ok
> version     ...
> uptime      ...
> health      ok
> ready       ok
> database    up
> daemons     connected=2 healthy=2 capacity=8 used=0 available=8
> apps        registered=1 websocket=1
> runs        total=3 active=0 ...
> workflow    total=... pending=... leased=... dead=...
> ```

### 2. 字段

| 字段 | 看什么 |
|---|---|
| `status` | 汇总状态 |
| `health` | 进程存活 |
| `ready` | 依赖（PostgreSQL 等）就绪 |
| `database` | 数据库连接 |
| `daemons` | Runtime 数量、容量、占用 |
| `apps` | 注册 App 与 WebSocket 连接 |
| `runs` | Run 总数与活跃数 |
| `workflow` | 任务队列与死信 |

### 3. JSON

```sh
levier status --json
```

适合监控和脚本。不要解析展示文本。

### 4. 健康判断

```text
health=ok + ready=ok + database=up
```

是继续操作的最低条件。

`daemons connected=0` 时：

- Run 会一直排队，直到有 Runtime 上线；
- 检查 `levier runtime list` 和 `levier logs`。

### 5. 远程状态

```sh
levier status --server https://levier.example.com
```

需要网络可达；认证由 profile / token 提供。

## 停止

### 1. levier down

```sh
levier down
```

停止本地栈。默认保留数据库卷，下次 `levier up` 可以继续使用。

### 2. 删除数据卷

```sh
levier down --volumes
```

会删除命名卷，Run、App、事件等本地数据一并丢失。只在确认不需要时使用。

### 3. 指定 Compose 文件

```sh
levier down --file ./docker-compose.yml
```

### 4. 重启

```sh
levier down
levier up
```

如果只是配置变化，优先：

```sh
levier up --build
```

### 5. 停止后

```sh
levier status
```

应看到连接失败或不可达。`levier up` 就绪后再继续。

## 日志与进程

### 1. levier logs

```sh
levier logs
```

查看本地 Compose 日志。

持续跟随：

```sh
levier logs --follow
```

退出跟随：`Ctrl+C`。

### 2. levier ps

```sh
levier ps
```

查看 Compose 服务状态。确认 `levier`、`postgres`、`mcp-runtime`、
`react-runtime`、`echo-runtime` 是否在运行。

### 3. 指定文件

```sh
levier logs --file ./docker-compose.yml
levier ps --file ./docker-compose.yml
```

### 4. 日志定位

| 现象 | 看哪里 |
|---|---|
| 控制面启动失败 | `levier` 容器日志 |
| 数据库未就绪 | `postgres` 容器日志 |
| Runtime 未连接 | 对应 runtime 容器日志 |
| MCP 工具失败 | `mcp-runtime` 日志 |
| Run 卡住 | `levier` + 目标 runtime 日志 |

需要按 Run 查看事件时：

```sh
levier run show <run_id> --events
```

### 5. 高级

`levier stack logs` / `levier stack ps` 直接包装 Compose，保留给已有脚本。
日常使用 `levier logs` / `levier ps` 即可。

## 下一步

- 安装、连接与启动：[安装、连接与启动](levier-install.md)
- 诊断与维护：[诊断与维护](levier-doctor.md)
- 运维视图：[运维视图](levier-ops.md)
