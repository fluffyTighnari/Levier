# Counter Agent（Python SDK 快速接入）

这是一个**最小可运行**的 Levier App。业务代码只有：

```text
Counter 状态
counter_add / counter_get / counter_reset
三份 JSON Schema
```

认证、HTTP、WebSocket、工具注册、重连、`local_tool_call` 分发和
`local_tool_result` 封装全部由 [`levier_app`](../../sdk/python/README.md) 处理。

## 1. 安装 SDK

在仓库根目录：

```sh
python3 -m venv .venv
.venv/Scripts/python -m pip install -e ./sdk/python
```

Linux/macOS 使用 `.venv/bin/python`。

## 2. 创建 App

```sh
export LEVIER_ADMIN_TOKEN=admin-change-me

levier app init \
  --name counter-agent \
  --server http://127.0.0.1:8080 \
  --credentials-out ./data/agent/credentials.json
```

`app init` 会创建 App、写入 0600 凭据文件并验证 key。之后 App 进程不再需要
Admin token。

## 3. 启动 App

```sh
LEVIER_CREDENTIALS=./data/agent/credentials.json \
  python3 examples/counter_agent/app.py
```

启动后 SDK 会自动：

```text
读取凭据
检查 App 状态
注册三个工具
获取 ws_token
连接 WebSocket
发送 hello
接收 local_tool_call
调用 Python handler
返回 local_tool_result
断线重连
```

## 4. 提交 Run

另开终端：

```sh
levier run submit \
  --credentials ./data/agent/credentials.json \
  --runtime react1 \
  --prompt "Add 2, then add 3, then read the counter." \
  --watch
```

不需要 `bootstrap_app.py`、`serve.py`、`submit_run.py`、自定义 WebSocket
客户端或自定义 HTTP 客户端。

## 5. 文件职责

```text
app.py      LevierApp、工具 schema、工具 handler、启动入口
tools.py    纯业务状态 Counter
tests/      Counter 和 SDK 分发测试
```

## 6. 测试

```sh
make counter-agent-test
```
