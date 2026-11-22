---
title: Run 提交与查看
status: published
description: 提交 Run，并查看或取消。
updated: 2026-11-21
---

## 提交 Run

### 1. levier run

```sh
levier run "hello"
```

默认：

```text
当前项目 App
默认 runtime
默认 provider / model
等待终态
输出最终消息
```

### 2. 参数

| 参数 | 说明 |
|---|---|
| `--runtime` | runtime_name |
| `--provider` / `--model` | 模型覆盖 |
| `--max-steps` | ReAct 最大步数 |
| `--run-ms` / `--tool-ms` | Run / 工具超时 |
| `--no-watch` | 只提交 |
| `--with-app` | 临时启动 `./app.py` |
| `--app` | 指定 App 文件 |
| `--python` | 指定 Python |
| `--json` | 机器可读 |
| `--credentials` / `--server` | 显式覆盖 |

### 3. --with-app

```sh
levier run "Add 2 and 3." --with-app
```

如果已有 App 连接，会复用：

```text
using the already-connected App
```

### 4. --no-watch

```sh
levier run "hello" --no-watch
```

返回 `run_id` 和 `status`。之后：

```sh
levier run watch <run_id>
levier run show <run_id> --events
```

### 5. JSON

```sh
levier run "hello" --json
```

包含 `run_id`、`status` 等字段，适合脚本。

### 6. 低层 submit

```sh
levier run submit \
  --runtime react1 \
  --prompt "hello" \
  --watch
```

低层命令可以显式控制 `--idempotency-key`、`--request-id` 和所有超时。

## 查看与取消

### 1. run show

```sh
levier run show <run_id>
```

> [!OUTPUT]
> ```text
> run_id       01J...
> status       completed
> runtime      react1
> attempt      1
> session_id   ...
> trace_id     ...
> created_at   ...
> updated_at   ...
> ```

### 2. --events

```sh
levier run show <run_id> --events
```

```text
[1] RUN_STATUS            daemon
[2] MESSAGE_DELTA         daemon
[3] TOOL_CALL_BEGIN       daemon
[4] TOOL_CALL_END         app
[5] MESSAGE_COMPLETED     daemon
[6] RUN_STATUS            daemon
```

事件类型见 [Run 与事件](app-runs.md)。

### 3. run cancel

```sh
levier run cancel <run_id> --yes
```

> [!OUTPUT]
> ```text
> run cancelled: 01J...
> status: cancelled
> ```

非交互环境必须传 `--yes`。取消是协作式的；已经终态的 Run 不会回退。

### 4. JSON

```sh
levier run show <run_id> --json
levier run show <run_id> --events --json
```

### 5. 低层参数

```sh
levier run show <run_id> --credentials ./data/agent/credentials.json
levier run cancel <run_id> --credentials ./data/agent/credentials.json --yes
```

### 6. 常见问题

| 现象 | 先检查 |
|---|---|
| `run not found` | run_id 是否属于当前 App |
| `UNAUTHORIZED` | 凭据路径 / App key |
| 一直 `queued` | `levier runtime list` 是否有可用 Runtime |
| `failed` 但没有工具事件 | 看 `ERROR` 事件与 Runtime 日志 |

## 下一步

- App Run：[Run 与事件](app-runs.md)
- Runtime 列表：[Runtime 列表](levier-runtime-list.md)
- 运维视图：[运维视图](levier-ops.md)
