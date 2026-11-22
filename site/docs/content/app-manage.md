---
title: App 管理
status: published
description: App 自检、查看与轮换 key。
updated: 2026-11-21
---

## 自检

### 1. app check

```sh
levier app check
```

显式凭据：

```sh
levier app check --credentials ./data/agent/credentials.json
```

> [!OUTPUT]
> ```text
> app check: PASS
> ```

失败时输出 `app check: FAIL` 并逐项列出原因。

### 2. 检查项

```text
凭据格式与版本
文件权限
服务端可达性
App key
App 状态是否为 active
WebSocket token
```

### 3. JSON

```sh
levier app check --json
```

适合脚本或 CI。退出码遵循全局约定：认证失败为 3，网络失败为 4。

### 4. 常见失败

| 现象 | 动作 |
|---|---|
| `read credentials` | 路径错误；检查 `LEVIER_CREDENTIALS` |
| 文件权限错误 | `chmod 600 <credentials>` |
| `status is inactive` | 重新 `levier init` 或查看 App 管理 |
| WebSocket token 失败 | 检查 server 与 ws_token TTL |
| 服务不可达 | `levier status` / `levier doctor` |

### 5. 低层凭据

`app check` 使用显式凭据路径；日常项目通常不需要：

```sh
levier app show --credentials ./data/agent/credentials.json
```

## 管理 App

### 1. app list

```sh
levier app list
```

管理员视图，需要 admin token（profile 或 `LEVIER_ADMIN_TOKEN`）。

### 2. app show

```sh
levier app show --credentials ./data/agent/credentials.json
```

> [!OUTPUT]
> ```text
> app_id              app_01...
> name                counter-agent
> status              active
> credentials         ./data/agent/credentials.json
> credential_version  1
> server              http://127.0.0.1:8080
> key_fingerprint     ...
> ```

只显示 key 指纹，不显示 key。

### 3. app rotate-key

```sh
levier app rotate-key \
  --credentials ./data/agent/credentials.json \
  --yes
```

轮换会：

```text
立即让旧 key 失效
原子写回凭据文件
保留受保护的 .bak 备份
```

轮换后重新执行：

```sh
levier app check --credentials ./data/agent/credentials.json
```

### 4. 凭据恢复

如果 key 丢失：

```sh
levier init --rotate
```

或低层：

```sh
levier app init \
  --name counter-agent \
  --credentials-out ./data/agent/credentials.json \
  --rotate --yes
```

不要删除数据库或凭据卷来“重新开始”。

### 5. 低层 app init

只在需要显式控制凭据路径时使用：

```sh
levier app init \
  --name counter-agent \
  --credentials-out ./data/agent/credentials.json \
  --mcp-ref echo
```

参数：

```text
--name
--credentials-out
--mcp-ref
--rotate
--yes
--force
--dry-run
--json
```

### 6. 权限

`app list`、`app rotate-key` 需要 admin token；`app show` / `app check`
使用 App 自己的 key。不要把 admin token 放进长期运行的 App 环境。

## 下一步

- App 核心流程：[App 核心流程](app-core.md)
- MCP 与重连：[MCP 引用与重连](app-mcp.md)
- SDK 总览：[SDK 总览](sdk.md)
