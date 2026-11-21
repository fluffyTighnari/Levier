---
title: 排查
status: published
description: 从 status、run show 与日志三个入口定位问题；按症状查，不按内部模块查。
updated: 2026-11-20
---

> [!NOTE]
> 先收集三样东西，再往下查：
>
> 1. `levier status`
> 2. `levier run show --run-id <id> --events`
> 3. `docker compose logs --tail=100 levier`（以及对应 runtime）
>
> 大多数问题在这一步就能定位。

## 1. 控制面起不来

| 现象 | 可能原因 | 动作 |
|---|---|---|
| `connection refused` | 容器还没起完 | 等 10 秒，`docker compose ps` |
| `health fail` | 数据库未就绪 / 配置错误 | 看 `docker compose logs levier` |
| `ready fail` | 迁移或依赖未完成 | `levier admin migrate status` |
| `daemons ok 0` | Runtime 没连上 | 看 runtime 日志与 `auth_token` |
| 端口被占用 | 8080 / 9090 已被使用 | 改 compose 端口或停掉冲突进程 |

```sh
docker compose ps
docker compose logs --tail=100 levier
```

## 2. 凭据与鉴权

| 错误 code | 含义 | 动作 |
|---|---|---|
| `UNAUTHORIZED` | App key 或 Admin token 错误 | 重新 `app init`；确认 `LEVIER_ADMIN_TOKEN` |
| `app exists` | 同名 App 已存在 | 换名字，或显式 `--rotate` |
| 凭据文件不存在 | 路径写错 | 确认 `--credentials-out` 与 `--credentials` 指向同一文件 |

> [!WARNING]
> App key 只在创建 / 轮换时返回一次。丢了就轮换，不要试图从数据库里找回明文。

## 3. App 连不上或工具找不到

| 现象 | 先检查 |
|---|---|
| `apps ok 0 websocket` | App 进程是否启动、`--credentials` 是否正确 |
| `TOOL_NOT_FOUND` | 工具名是否与 `@app.tool("...")` 完全一致 |
| `APP_UNREACHABLE` | App 是否在线；断线期间的 pending 调用会失败 |
| 工具重复执行 | App 是否按 `invocation_id` 幂等 |
| 工具一直超时 | handler 是否阻塞；`timeout_ms` 是否太小 |

```sh
./bin/levier app check --credentials ./my-first-app.json
```

## 4. 模型没有回复

| 现象 | 先检查 |
|---|---|
| `MODEL_PROVIDER_NOT_FOUND` | 是否执行过 `provider add` |
| `SECRET_MISSING` | `LEVIER_SECRET_<UPPER_REF>` 或 secret 文件是否有值 |
| 401 / 403 | 模型 key 是否有效、base URL 是否正确 |
| `final` 为空 | 看 `run show --events` 是否有 `ERROR` |
| 一直 `queued` | 目标 runtime 是否在线、名字是否正确 |

> [!TIP]
> 先用 `--runtime echo1` 跑一次。如果 Echo 能完成，问题在模型或 provider；
> 如果 Echo 也不能完成，问题在控制面或凭据。

## 5. MCP

| 现象 | 先检查 |
|---|---|
| App 初始化失败 | `mcp_ref` 是否已注册 |
| 模型看不到 MCP 工具 | `allowed_tools` 是否包含该工具 |
| `SECRET_MISSING` | `${secret:name}` 与 secret 变量是否对应 |
| MCP 工具没有调用记录 | 正常：MCP 不经过 Gateway，不写 `tool_invocations` |

## 6. 文档页打不开

文档页通过 `fetch` 读取本地 Markdown，浏览器在 `file://` 下会拦截。
在仓库根目录启动一个静态服务器：

```sh
python -m http.server 4173 --directory site
```

然后打开 `http://127.0.0.1:4173/docs/`。

## 7. 仍然卡住

提交 issue 时附上：

- `levier status` 输出
- `run show --events` 输出（隐去 key 与业务数据）
- 相关容器最近 100 行日志
- 你执行的确切命令

GitHub：<https://github.com/fluffyTighnari/levier/issues>

## 8. 下一步

- 回到 [快速开始](quickstart.md) 重走一遍。
- 检查契约与已知缺口：[架构](architecture.md)。
