---
title: 远程控制面
status: published
description: HTTPS、admin token、runtime token 与网络边界。
updated: 2026-11-21
---

## 1. 远程 setup

```sh
levier setup \
  --server https://levier.example.com \
  --admin-token <admin-token>
```

之后：

```sh
levier status
levier model connect ...
levier init
```

## 2. HTTPS

远程连接必须使用 HTTPS，避免 App key 与模型 key 在传输中暴露。

本地栈使用 HTTP 仅用于回环地址。

如果使用反向代理，确认：

```text
WebSocket upgrade 已转发
/healthz /readyz /status 可访问
超时足够长
```

## 3. admin token

用于：

```text
model connect / remove
mcp add / remove
app list / rotate-key
ops / migrate / maintenance
```

来源：

```text
--admin-token
LEVIER_ADMIN_TOKEN
profile 中显式保存的值
```

不要放进长期运行的 App 环境。

## 4. runtime token

远程 Runtime 需要与控制面一致的 token：

```sh
levier setup \
  --server https://levier.example.com \
  --grpc levier.example.com:9090 \
  --runtime-token <runtime-token>
```

或：

```sh
export LEVIER_RUNTIME_TOKEN=...
export LEVIER_MIDDLEWARE_ADDRESS=levier.example.com:9090
```

生产环境使用显式 daemon token，不要使用开发通配 token。

## 5. TLS

控制面 gRPC 启用 TLS 时：

```sh
export LEVIER_MIDDLEWARE_TLS=true
```

Runtime SDK 会使用 TLS 连接。证书与主机名必须匹配。

## 6. 网络排查

| 现象 | 先检查 |
|---|---|
| `connection refused` | 地址、端口、防火墙 |
| TLS 错误 | 证书、主机名、TLS 开关 |
| WebSocket 失败 | 反向代理 upgrade |
| 认证失败 | admin / runtime token |
| Run 一直 queued | Runtime 是否能连上 gRPC |

## 7. secret

远程部署的模型 key 使用 0600 文件或 secret manager：

```text
LEVIER_SECRETS_FILE
LEVIER_SECRETS_WRITE_FILE
LEVIER_SECRET_<REF>
```

不要把 secret 写进 Compose 明文或命令行历史。

## 下一步

- setup 字段：[setup 与 profile](levier-install.md)
- 凭据：[凭据与密钥](pre-env.md)
- 环境变量：[环境变量](env.md)
