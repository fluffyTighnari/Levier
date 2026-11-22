---
title: 配置 Runtime
status: published
description: 安装 Runtime SDK、初始化模板并启动 Runtime。
updated: 2026-11-21
---

## 安装 SDK

### 1. PyPI

```sh
pip install levier
```

App-only 使用不会立即加载 Runtime 的 gRPC 依赖；导入 `Context` / `Runtime`
时才加载。

### 2. 源码

```sh
cd /path/to/middleware
python -m pip install -e ./sdk/python -e ./execution/runtime_sdk -e ./sdk/levier
```

如果还要运行内置 Runtime：

```sh
python -m pip install -e ./execution/echo_runtime
python -m pip install -e ./execution/react_runtime
python -m pip install -e ./execution/agent_core
```

### 3. 依赖

Runtime SDK 需要：

```text
Python 3.11+
gRPC
protobuf
```

MCP 工具通过 `ctx.toolbox()` 使用，不需要在 Runtime 里安装 MCP client。

### 4. 验证

```python
from levier import Context, Runtime
print(Context, Runtime)
```

如果这里报 `No module named levier_runtime`，说明只装了 App 侧的 `levier`。

## runtime init

### 1. runtime init

```sh
levier runtime init
```

默认写入 `./runtime.py`。

> [!OUTPUT]
> ```text
> created runtime.py
> start it with: levier runtime start
> ```

### 2. 指定文件

```sh
levier runtime init --file ./my-runtime/runtime.py
```

如果文件已存在：

```sh
levier runtime init --force
```

`--force` 会覆盖原文件；不要对正在维护的 Runtime 使用。

### 3. 目录约定

推荐：

```text
my-runtime/
  runtime.py
  requirements.txt
  README.md
```

`levier runtime start` 默认查找：

```text
./runtime.py
./levier_runtime.py
./my_runtime.py
```

所以直接在 Runtime 目录启动即可。

### 4. 模板内容

生成的文件包含：

```text
Context / Runtime 导入
async def run(ctx)
ctx.toolbox()
ctx.chat()
ctx.invoke_all()
ctx.reply()
Runtime.start(run)
```

只保留你需要的那部分，不要为了“完整”保留无用代码。

### 5. 下一步

- 启动：[runtime start](runtime-start.md)
- 上下文 API：[Context](runtime-context.md)
- 工具：[Toolbox](runtime-toolbox.md)

## runtime start

### 1. runtime start

```sh
levier runtime start
```

默认：

```text
查找 ./runtime.py
从 profile 读取 runtime name、gRPC 地址、token
启动 Python 进程
保持连接
```

> [!OUTPUT]
> ```text
> runtime: my-runtime
> file:    /path/to/runtime.py
> server:  127.0.0.1:9090
> ```

### 2. 参数

| 参数 | 说明 |
|---|---|
| `--file` | 指定 runtime 文件 |
| `--name` | runtime_name；默认目录名 |
| `--token` | runtime token |
| `--grpc` | 控制面 gRPC 地址 |
| `--python` | Python 可执行文件 |
| `--config` | 可选 Runtime YAML |
| `--log-level` | 日志级别 |

### 3. profile 身份

`levier up` / `levier setup` 会写入：

```text
LEVIER_RUNTIME_NAME
LEVIER_RUNTIME_TOKEN
LEVIER_MIDDLEWARE_ADDRESS
LEVIER_MCP_RUNTIME_SOCKET
```

`runtime start` 自动注入这些值。

### 4. 日志级别

```sh
levier runtime start --log-level debug
```

也支持：

```sh
LEVIER_RUNTIME_LOG_LEVEL=debug levier runtime start
```

### 5. 确认连接

```sh
levier runtime list
```

应看到新的 runtime / instance。没有出现时：

```sh
levier logs
levier doctor
```

### 6. 远程

远程部署需要显式 token：

```sh
levier runtime start \
  --grpc levier.example.com:9090 \
  --token <runtime-token>
```

TLS 与证书见 [远程部署与 YAML](runtime-remote.md)。

## 下一步

- Runtime 核心流程：[Runtime 核心流程](runtime-core.md)
- 内置 Runtime：[内置 Runtime](runtime-builtin.md)
- Context：[Context](runtime-context.md)
