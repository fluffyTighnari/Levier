# Python SDK Counter App

这是推荐的新 App 接入方式：业务代码只定义 Counter 状态和三个工具 handler；
协议客户端全部由 `levier_app` 提供。

## 准备

```sh
python -m venv .venv
.venv/Scripts/python -m pip install -e ../../sdk/python
```

Linux/macOS 使用 `.venv/bin/python`。

先按 [`docs/ONBOARDING.md`](../../docs/ONBOARDING.md) 创建 App：

```sh
levier app init --name sdk-counter
```

## 运行

```sh
# Linux/macOS
export LEVIER_CREDENTIALS=~/.config/levier/sdk-counter.json
.venv/bin/python app.py

# Windows PowerShell
$env:LEVIER_CREDENTIALS = "$HOME/.config/levier/sdk-counter.json"
.venv/Scripts/python app.py
```

然后提交一个 Run：

```sh
levier run submit \
  --credentials "$LEVIER_CREDENTIALS" \
  --strategy react \
  --prompt "Add 2, then add 3, then read the counter." \
  --watch
```

实际业务接入只需要：

```text
Counter 状态
三个 handler
三个 JSON Schema
```

不需要实现 HTTP 认证、WebSocket、token、重连或 local_tool_call 结果封装。
