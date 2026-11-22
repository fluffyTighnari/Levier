---
title: 部署与容量
status: published
description: 查看实例与容量，并通过 token 与 TLS 部署远程 Runtime。
updated: 2026-11-21
---

## 实例与容量

### 1. runtime list

```sh
levier runtime list
```

> [!OUTPUT]
> ```text
> runtime    instance         status    capacity  running  available  heartbeat
> my-runtime daemon-local-1   healthy   4         0        4          ...
> ```

字段与调度含义见 [Runtime 列表](levier-runtime-list.md)。

### 2. 多实例

同一个 `runtime_name` 可以启动多个实例：

```sh
levier runtime start --name my-runtime
levier runtime start --name my-runtime --token ...
```

控制面按容量调度；实例 ID 用于区分连接。

### 3. 容量

`RuntimeConfig` 可以设置：

```python
RuntimeConfig(
    runtime_name="my-runtime",
    instance_id="daemon-local-1",
    middleware=MiddlewareConfig("127.0.0.1:9090", auth_token="..."),
    max_concurrent_runs=4,
)
```

或环境变量：

```text
LEVIER_RUNTIME_MAX_CONCURRENT_RUNS
```

### 4. 心跳

长期没有心跳的实例不参与调度。检查：

```sh
levier runtime list
levier logs
```

### 5. 常见问题

| 现象 | 先检查 |
|---|---|
| 新实例没出现 | token / gRPC 地址 |
| 一直 queued | 所有实例容量已满 |
| 实例重复 | instance_id 是否唯一 |
| 心跳过旧 | 进程、网络、控制面 |

## 远程部署与 YAML

### 1. 远程部署

```sh
levier runtime start \
  --grpc levier.example.com:9090 \
  --token <runtime-token> \
  --name my-runtime
```

或环境变量：

```sh
export LEVIER_MIDDLEWARE_ADDRESS=levier.example.com:9090
export LEVIER_RUNTIME_TOKEN=...
export LEVIER_RUNTIME_NAME=my-runtime
levier runtime start
```

### 2. TLS

```sh
export LEVIER_MIDDLEWARE_TLS=true
```

证书与主机名必须匹配。控制面 gRPC 未启用 TLS 时不要打开。

### 3. YAML

不传 YAML 时，`Runtime.start()` 从 profile / 环境推导。需要显式配置时：

```yaml
middleware:
  address: "levier.example.com:9090"
  auth_token: "..."

runtime:
  name: "my-runtime"
  instance_id: "daemon-remote-1"
  type: "normal"
  max_concurrent_runs: 4

logging:
  level: "info"
```

```python
Runtime.from_config("runtime.yaml").run(run)
```

### 4. 环境变量

```text
LEVIER_RUNTIME_CONFIG
LEVIER_MIDDLEWARE_ADDRESS
LEVIER_RUNTIME_NAME
LEVIER_RUNTIME_INSTANCE_ID
LEVIER_RUNTIME_TOKEN / _FILE
LEVIER_RUNTIME_TYPE
LEVIER_RUNTIME_MAX_CONCURRENT_RUNS
LEVIER_RUNTIME_LABELS
LEVIER_RUNTIME_LOG_LEVEL
```

### 5. 迁移

从旧 Runtime 迁到 `levier` 高层 API：

```text
保留 run(ctx) 逻辑
把 daemon / channel / heartbeat 代码删掉
用 ctx.toolbox() 替代自建 ToolRouter
用 ctx.chat() / ctx.invoke_all() 替代自建模型与工具循环
```

YAML 只保留 middleware / runtime / logging；MCP 地址由 profile 或
`LEVIER_MCP_RUNTIME_SOCKET` 提供。

### 6. 排查

| 现象 | 先检查 |
|---|---|
| TLS handshake | 证书、主机名、TLS 开关 |
| `UNAUTHENTICATED` | runtime token |
| 连接超时 | 防火墙、gRPC 端口 |
| 频繁重连 | 网络、token、控制面日志 |

## 下一步

- 配置 Runtime：[配置 Runtime](runtime-start.md)
- Runtime 核心流程：[Runtime 核心流程](runtime-core.md)
- 远程控制面：[远程控制面](levier-remote.md)
