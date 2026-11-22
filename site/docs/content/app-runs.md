---
title: Run 与事件
status: published
description: 通过 CLI 或 SDK 提交 Run，并查看状态与事件。
updated: 2026-11-21
---

## 通过 CLI

### 1. levier run

```sh
levier run "Add 2, then add 3, then read the counter."
```

默认行为：

```text
使用当前项目的 App
使用默认 runtime
使用默认 provider / model
持续观测到终态
输出最终 assistant 消息
```

> [!OUTPUT]
> ```text
> run submitted: 01J...
> status: queued
> status: completed
> runtime_name: react1
> final: 5
> ```

### 2. 参数

| 参数 | 说明 |
|---|---|
| `--runtime` | 指定 runtime_name |
| `--provider` / `--model` | 覆盖默认模型 |
| `--max-steps` | ReAct 最大步数 |
| `--run-ms` | Run 超时 |
| `--tool-ms` | 单工具超时 |
| `--no-watch` | 只提交，不等待 |
| `--with-app` | Run 期间临时启动 `./app.py` |
| `--app` | 指定 App 文件，隐含 `--with-app` |
| `--python` | `--with-app` 使用的 Python |
| `--json` | 机器可读输出 |
| `--credentials` / `--server` | 显式覆盖配置 |

prompt 前后都可以放 flag。

### 3. --with-app

```sh
levier run "Add 2 and 3." --with-app
```

如果已有 App 连接：

```text
using the already-connected App
```

不会重复启动，避免同一工具被执行两次。

### 4. --no-watch

```sh
levier run "hello" --no-watch
```

只返回：

```text
run submitted: 01J...
status: queued
```

之后用：

```sh
levier run watch <run_id>
levier run show <run_id> --events
```

### 5. 低层命令

仍然可用，适合脚本和已有集成：

```sh
levier run submit --runtime react1 --prompt "hello" --watch
levier run watch <run_id>
levier run show <run_id> --events
levier run cancel <run_id> --yes
```

低层 `submit` 可以显式传 `--credentials`、`--provider`、`--model`、
`--idempotency-key` 等。

详见 [查看与取消 Run](levier-run.md)。

### 6. 退出码

```text
0  completed
1  运行失败
2  参数/配置错误
3  认证失败
4  网络/上游可重试失败
5  拒绝或超时
```

## 通过 SDK

### 1. submit_run

```python
from levier import App

app = App(tools=[])

run = app.submit_run(
    "Add 2, then add 3.",
    runtime="react1",
)
run_id = run["run_id"]
```

可选参数：

```text
provider / model
max_steps
run_ms
tool_ms
idempotency_key
```

不传 `runtime` 时使用项目 / profile 默认值。

### 2. wait_run

```python
result = app.wait_run(run_id, timeout=120)
print(result["status"])
```

轮询到终态。终态：

```text
completed / failed / cancelled / timeout
```

### 3. get_events

```python
events = app.get_events(run_id)
for event in events:
    print(event["event_type"], event.get("data"))
```

常用事件：

```text
RUN_STATUS
MESSAGE_DELTA
MESSAGE_COMPLETED
TOOL_CALL_BEGIN
TOOL_CALL_END
ERROR
```

### 4. cancel_run

```python
app.cancel_run(run_id)
```

取消是协作式的：Runtime 收到取消后决定何时停止。已经进入终态的 Run 不会回退。

### 5. 一次性 app.run

```python
app.run("Add 2, then add 3.")
```

它会临时建立连接、提交 Run、等待终态并打印最终消息。

```python
app.run("hello", runtime="echo1", print_output=False)
```

### 6. 事件与结果

`wait_run` 返回 Run 详情，`get_events` 返回事件列表。需要稳定解析时优先看：

```text
event["event_type"]
event["data"]
```

不要依赖展示文本。

## Run 状态与事件

### 1. 获取 Run

SDK：

```python
run = app.get_run(run_id)
print(run["status"], run["runtime_name"])
```

CLI：

```sh
levier run show <run_id>
```

### 2. 等待终态

```python
result = app.wait_run(run_id, timeout=120)
```

终态：

```text
completed / failed / cancelled / timeout
```

### 3. 事件列表

```python
events = app.get_events(run_id)
```

CLI：

```sh
levier run show <run_id> --events
```

常用事件：

| 事件 | 含义 |
|---|---|
| `RUN_STATUS` | 状态变化 |
| `MESSAGE_DELTA` | 流式文本 |
| `MESSAGE_COMPLETED` | 一条完整 assistant 消息 |
| `TOOL_CALL_BEGIN` | 开始工具调用 |
| `TOOL_CALL_END` | 工具结果或错误 |
| `ERROR` | Runtime 显式错误 |
| `USAGE` | token 用量（如果模型提供） |

### 4. 取消 Run

```python
app.cancel_run(run_id)
```

CLI：

```sh
levier run cancel <run_id> --yes
```

取消是协作式的；已经终态的 Run 不会回退。

### 5. 低层 CLI

```sh
levier run watch <run_id>
levier run show <run_id> --events
levier run cancel <run_id> --yes
```

脚本中优先使用 `--json`。

### 6. 事件游标

HTTP 事件接口支持：

```text
after_attempt
after_seq
limit
```

SDK 的 `get_events` 默认拉取全部可用事件；需要补拉时使用低层 HTTP 或后续参数。

## 下一步

- App 核心流程：[App 核心流程](app-core.md)
- App 管理：[App 管理](app-manage.md)
- Levier Run：[Run 提交与查看](levier-run.md)
