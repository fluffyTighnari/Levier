---
title: 诊断与维护
status: published
description: 运行 doctor / describe / selfcheck，并校验、迁移或维护控制面。
updated: 2026-11-21
---

## doctor / describe / selfcheck

### 1. doctor

```sh
levier doctor
```

检查：

```text
控制面 YAML
HTTP 可达性
数据库
迁移
daemon
可选 App 凭据
```

输出逐项 `PASS` / `FAIL`，失败项给出 `fix`。

```sh
levier doctor --server http://127.0.0.1:8080
levier doctor --config ./levier.yaml
levier doctor --json
```

### 2. describe

```sh
levier describe
```

读取 `/meta`，显示：

```text
service / version / commit
api / ws protocol
runtime types
limits
endpoints
```

用于确认控制面能力和限制。

### 3. selfcheck

```sh
levier selfcheck
```

逐跳检查：

```text
config
http /healthz
database
migrations
http /readyz
service /meta
service /status
daemon registered
```

> [!OUTPUT]
> ```text
> selfcheck: PASS
> ```

深度检查：

```sh
levier selfcheck --deep
```

会做主动 daemon / WebSocket / tool / MCP 检查。

### 4. 结果解读

```text
PASS    这一跳正常
FAIL    这一跳失败，后面可能连带失败
fix     给出的修复命令或检查点
```

从第一个 `FAIL` 开始修，不要同时改多处。

### 5. 常见失败

| 跳 | 先检查 |
|---|---|
| config | YAML 路径与语法 |
| http | 控制面是否启动、端口 |
| database | PostgreSQL、DSN |
| migrations | `levier migrate status` |
| daemon | `levier runtime list` |
| App | 凭据路径与 `app check` |

## config / migrate / maintenance

### 1. config check

```sh
levier config check --config ./levier.yaml
```

生产模式：

```sh
levier config check --config ./levier.yaml --production
```

会拒绝示例默认 token / 密码。

### 2. config print

```sh
levier config print --config ./levier.yaml
```

打印脱敏后的配置。secret 不会输出。

### 3. migrate

```sh
levier migrate status --config ./levier.yaml
levier migrate up --config ./levier.yaml
```

`status` 显示当前版本、已应用、pending 列表。

### 4. maintenance

```sh
levier maintenance status --config ./levier.yaml
levier maintenance prune --config ./levier.yaml
```

`prune` 默认 dry-run；确认后才真正删除：

```sh
levier maintenance prune --config ./levier.yaml --yes
```

清理单位是终态 Run：

```text
只删除 completed / failed / cancelled / timeout
且超过 retention.run_days 的 Run
不会删除 queued / assigned / running
```

### 5. retention 配置

```yaml
retention:
  enabled: false
  run_days: 30
  session_days: 7
  batch_size: 1000
```

对应环境变量：

```text
LEVIER_RETENTION_ENABLED
LEVIER_RETENTION_RUN_DAYS
LEVIER_RETENTION_SESSION_DAYS
LEVIER_RETENTION_BATCH_SIZE
```

### 6. 顺序

```text
config check
  → migrate up
  → maintenance status
  → maintenance prune --yes（确认后）
```

## 下一步

- 本地栈生命周期：[本地栈生命周期](levier-status.md)
- 运维视图：[运维视图](levier-ops.md)
- 远程控制面：[远程控制面](levier-remote.md)
