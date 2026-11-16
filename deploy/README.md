<!-- Middleware — container deployment. -->

# Container Deployment

## 单元卡

| 项 | 内容 |
|---|---|
| 职责 | 镜像、Compose、入口脚本、smoke |
| 入口 | `docker-compose.yml`、`deploy/*.Dockerfile` |
| 输入 / 输出 | 源码与配置；可运行容器栈 |
| 依赖 | 各组件源码、PostgreSQL、Docker |
| 不负责 | 业务逻辑、协议定义、控制面实现 |
| 测试 | `docker compose config`、`docker compose --profile smoke run --rm smoke` |

一套 Compose 启动：

```text
postgres      PostgreSQL 16
levier        Go 控制面
mcp-runtime   runtime 1：MCP 连接/会话/凭据
harness       runtime 2：Agent 循环
agent         counter_agent App
```

## 1. 构建并启动

```sh
docker compose up --build -d
docker compose ps
```

首次启动会：

```text
levier 等 PostgreSQL → 迁移 → 启动
mcp-runtime 创建 /run/levier/mcp.sock
harness 连接 levier，注册 daemon-local-1
agent 等 /readyz → bootstrap App → 注册工具 → 连 WebSocket
```

## 2. 一键 smoke

```sh
docker compose --profile smoke run --rm smoke
```

smoke 检查：

```text
/healthz、/readyz
Harness 已注册
App 已 bootstrap 并建立 WebSocket
MCP runtime socket 可连接，mcp.health = ok
提交一个 echo Run，最终 completed
```

成功输出：

```text
docker-smoke: PASS
```

## 3. 真实模型

echo smoke 不需要模型 key。要跑真实 ReAct：

```sh
DEEPSEEK_API_KEY=... docker compose up --build -d
```

然后在 App 容器里提交：

```sh
docker compose exec agent python3 /app/examples/counter_agent/submit_run.py \
  --credentials /data/credentials.json \
  --mw-url http://levier:8080 \
  --strategy react \
  --provider deepseek \
  --model deepseek-flash \
  --prompt "Add 2, then add 3, then read the counter."
```

## 4. 日志与清理

```sh
docker compose logs -f
docker compose down
docker compose down -v   # 同时删除 PostgreSQL/凭据/socket 卷
```

## 5. 配置项

Compose 默认值足够本地 smoke：

```text
POSTGRES_PASSWORD   change-me
LEVIER_ADMIN_TOKEN      admin-change-me
LEVIER_APP_NAME         counter-agent
MCP_E2E_TOKEN       mcp-e2e-secret
```

daemon token 当前固定为 `change-me`，与 `levier.example.yaml` 的
`daemon-local-1` 一致；这是本地 demo 配置，生产需要改成独立凭据。

## 6. 单独构建镜像

```sh
docker build -f deploy/levier.Dockerfile -t levier:dev .
docker build -f deploy/harness.Dockerfile -t levier-harness:dev .
docker build -f deploy/mcp-runtime.Dockerfile -t levier-mcp-runtime:dev .
docker build -f deploy/agent.Dockerfile -t levier-agent:dev .
```

如果 Docker 内访问 `proxy.golang.org` 失败，可以用模块代理构建：

```sh
GOPROXY=https://goproxy.cn,direct docker compose build levier
# 或
docker build -f deploy/levier.Dockerfile \
  --build-arg GOPROXY=https://goproxy.cn,direct \
  -t levier:dev .
```
