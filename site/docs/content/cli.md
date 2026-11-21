---
title: CLI 参考
status: published
description: 日常操作面——status、app、provider、mcp、run 与 runtime。按任务分组，不只列参数。
updated: 2026-11-20
---

> [!NOTE]
> 以下命令写作 `levier`。在仓库内可以直接用 `./bin/levier`；
> Windows 用 `.\bin\levier.exe`。完整参数、JSON 输出与退出码以仓库
> `docs/CLI_REFERENCE.md` 为准。

## 1. 日常操作面

```text
levier version
levier serve
levier status
levier doctor
levier app init|show|check|list|rotate-key
levier provider list|show|add|update|remove|check
levier mcp list|add|remove
levier run submit|watch|show|cancel
levier runtime list
```

## 2. 最短成功路径

### 看控制面

```sh
levier status
```

> [!OUTPUT]
> ```text
> server      http://127.0.0.1:8080
> health      ok
> ready       ok
> metrics     ok
> daemons     ok 2 (capacity 8, used 0)
> ```

### 创建 App

```sh
levier app init \
  --name counter-agent \
  --server http://127.0.0.1:8080 \
  --credentials-out ./data/agent/credentials.json
```

> [!OUTPUT]
> ```text
> app created: app_01...
> name: counter-agent
> credentials: ./data/agent/credentials.json (0600)
> key fingerprint: ...
> ```

### 注册 provider

```sh
levier provider add \
  --name deepseek \
  --adapter openai_compatible \
  --base-url https://api.deepseek.com \
  --default-model deepseek-flash \
  --models deepseek-flash,deepseek-reasoner \
  --secret-ref deepseek_api_key \
  --default
```

先确保 `LEVIER_SECRET_DEEPSEEK_API_KEY` 或 secret 文件里已经有值。
控制面只保存 `secret_ref`，不保存 key 本身。

### 提交 Run

```sh
levier run submit \
  --credentials ./data/agent/credentials.json \
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

### 看事件

```sh
levier run show --credentials ./data/agent/credentials.json \
  --run-id <id> --events
```

## 3. 管理 / 内部命令

```text
levier admin ops runs|daemons|tasks
levier admin migrate status|up
levier admin maintenance status|prune
levier admin config check|print
levier admin daemon check
levier admin describe
levier admin selfcheck [--deep]
```

这些命令保留兼容入口，但不出现在主帮助里。

## 4. 配置与密钥

```text
LEVIER_ADMIN_TOKEN                  管理员 token
LEVIER_CREDENTIALS                  App 凭据文件
LEVIER_SECRET_<UPPER_REF>           单密钥环境变量
LEVIER_SECRETS_FILE                 0600 JSON secret 文件
```

生产环境优先使用 secret 文件；模型 key 不会通过 CLI 输出、`/meta`、
Run event 或 checkpoint 暴露。

## 5. 排查顺序

按这个顺序缩小范围，不要一上来读全部日志：

```text
1. levier status 是否 ok
2. app check 是否 PASS
3. run show 的 status 是什么
4. levier 和对应 runtime 的日志
```

```sh
docker compose logs --tail=100 levier
docker compose logs --tail=100 echo-runtime
```

> [!TIP]
> 错误优先看结构化 `code`，不要匹配中文错误文本。
> 常见 code 与处理方式见 [排查](troubleshooting.md)。

## 6. 下一步

- 第一次跑通：[快速开始](quickstart.md)。
- 权限与凭据：[核心概念](concepts.md)。
- 卡住了：[排查](troubleshooting.md)。
