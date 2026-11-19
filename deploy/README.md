<!-- Levier — container deployment. -->

# Container Deployment

## 单元卡

| 项 | 内容 |
|---|---|
| 职责 | 镜像、Compose、入口脚本、smoke |
| 入口 | `docker-compose.yml`、`docker-compose.prod.yml`、`deploy/*.Dockerfile`、`deploy/secrets/` |
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
# 先创建 host 目录，避免 Docker 以 root 创建 bind mount 目录
mkdir -p data/agent

# 启动基础设施；agent 会等待 /data/credentials.json
docker compose up --build -d

# 首次运行：用 canonical CLI 创建 App，credentials 写入宿主机 bind mount
LEVIER_ADMIN_TOKEN=admin-change-me bin/levier app init \
  --name counter-agent \
  --server http://127.0.0.1:8080 \
  --credentials-out data/agent/credentials.json

# 或直接使用封装
make docker-up
docker compose ps
```

首次启动会：

```text
levier 等 PostgreSQL → migrate up → serve
  （serve 启动时检查 pending migration）
mcp-runtime 创建 /run/levier/mcp.sock
harness 连接 levier，注册 daemon-local-1
agent 等 /readyz 和 credentials 文件 → 启动 SDK 薄 App → 连 WebSocket
```

生产环境的 Agent 由用户自己部署；这里的 `agent` 容器只是测试镜像，
不持有 Admin token，也不负责创建 App 身份。

## 2. 一键 smoke

```sh
docker compose --profile smoke run --rm smoke
```

smoke 检查：

```text
/healthz、/readyz
/meta
/status
Harness 已注册
App 凭据已创建并建立 WebSocket
MCP runtime socket 可连接，mcp.health = ok
提交一个 echo Run，最终 completed
```

成功输出：

```text
docker-smoke: PASS
```

运行中的容器内逐跳自检：

```sh
docker compose exec levier /app/levier selfcheck \
  --server http://127.0.0.1:8080
```

需要主动 daemon 协议检查时，显式传入 daemon token：

```sh
docker compose exec levier /app/levier selfcheck --deep \
  --admin-token "$LEVIER_ADMIN_TOKEN" \
  --daemon-token "$LEVIER_DAEMON_TOKEN"
```

`selfcheck` 默认只读；`--deep` 会创建临时 App / Run，不删除已有资源。

### 数据维护

维护命令直接连接 PostgreSQL，不经过 HTTP：

```sh
docker compose exec levier /app/levier maintenance status
docker compose exec levier /app/levier maintenance prune --dry-run
```

`retention.enabled` 默认是 `false`，因此 `--yes` 会被拒绝。确认 dry-run
候选数量后，才在配置中启用 retention，并显式执行：

```sh
docker compose exec levier /app/levier maintenance prune --yes
```

第一版只删除超过 `run_days` 的终态 Run；它的 events、tasks 和 tool
invocations 通过外键级联删除。过期 Session 在没有 active Run 引用时才会
被清理。执行前请先备份，详见 `docs/LIFECYCLE_PLAN.md`。

## 3. 真实模型

echo smoke 不需要模型 key。要跑真实 ReAct：

```sh
DEEPSEEK_API_KEY=... docker compose up --build -d
```

然后在宿主机提交（使用同一个 bind mount 中的凭据）：

```sh
bin/levier run submit \
  --credentials data/agent/credentials.json \
  --server http://127.0.0.1:8080 \
  --strategy react \
  --provider deepseek \
  --model deepseek-flash \
  --prompt "Add 2, then add 3, then read the counter."
```

## 4. 日志与清理

```sh
docker compose logs -f
docker compose down
docker compose down -v   # 删除 PostgreSQL/socket 卷；data/agent 需手动删除
```

### 优雅停止

Compose 配置了：

```text
levier / postgres   stop_grace_period: 30s
harness / mcp / agent stop_grace_period: 20s
```

停止 Levier 并查看生命周期日志：

```sh
docker compose stop levier
docker compose logs --tail=40 levier
docker compose start levier
```

日志应包含：

```text
levier ready
levier stopping
workflow workers stopped
control stopped
daemon hub stopped
levier stopped
```

如果超过停止宽限期，Docker 会发送 SIGKILL；这只应出现在异常进程或未响应的子进程。
`mcp-runtime`、`harness`、`agent` 使用 `init: true`，由容器 init 回收子进程。

## 5. 开发默认值

`docker compose up` 是本地开发配置，默认值足够跑 smoke：

```text
POSTGRES_PASSWORD       change-me
LEVIER_ADMIN_TOKEN      admin-change-me（仅 host 初始化 / 控制面）
LEVIER_APP_NAME         counter-agent（仅测试 App 初始化）
LEVIER_HARNESS_DAEMON_AUTH_TOKEN  change-me
```

开发配置样例是根目录 `levier.example.yaml`。
daemon token 与 `levier.example.yaml` 的 `daemon-local-1` 一致。
这些值只用于本地 demo，不要用于生产。

## 6. 生产式部署：secret 文件

生产式配置使用：

```text
docker-compose.prod.yml
deploy/config/levier.prod.yaml
deploy/secrets/*
```

准备 secret 文件：

```sh
for f in deploy/secrets/*.example; do
  cp "$f" "${f%.example}"
done
chmod 600 deploy/secrets/levier_*
```

然后编辑这些文件：

```text
deploy/secrets/levier_postgres_password
deploy/secrets/levier_admin_token
deploy/secrets/levier_daemon_tokens.json
deploy/secrets/levier_database_url
deploy/secrets/levier_harness_daemon_token
deploy/secrets/levier_mcp_secrets.json
```

`levier_admin_token` 同时供控制面与 Agent 使用，避免同一 Admin token 出现两份来源。
Harness 的 daemon token 必须与 `levier_daemon_tokens.json` 中对应 daemon 的值一致。

启动：

```sh
docker compose \
  -f docker-compose.yml \
  -f docker-compose.prod.yml \
  up -d
```

生产式 overlay 的行为：

```text
levier 使用 LEVIER_ENV=production
admin token 从 /run/secrets/levier_admin_token 读取
daemon tokens 从 /run/secrets/levier_daemon_tokens.json 读取
数据库 DSN 从 /run/secrets/levier_database_url 读取
harness daemon token 从 secret 文件读取
MCP secrets 从 secret 文件读取
agent Admin token 复用同一个 admin secret
PostgreSQL 密码从 secret 文件读取
```

`docker-compose.prod.yml` 不会把真实 secret 写进 Compose 文件或镜像。
`.dockerignore` 排除了 `deploy/secrets`，避免 secret 进入 Docker build context。
secret 文件必须是宿主机上的 `0600` 文件；生产目标为 Linux。Docker Desktop
在 Windows 上可能不保留文件权限位，因此不适合作为生产式 overlay 的最终验证环境。

从本地开发栈切换到生产式 overlay 时，如果 PostgreSQL 卷已存在，需要让
`levier_database_url` 与卷内已有密码匹配，或使用新卷重新初始化。

## 7. 备份、恢复与轮换

备份和恢复使用显式脚本，不自动执行：

```sh
sh scripts/backup.sh --output data/backups/levier-prod
sh scripts/restore.sh --backup data/backups/levier-prod --dry-run
sh scripts/restore.sh --backup data/backups/levier-prod --yes
```

生产式 overlay：

```sh
COMPOSE_FILE=docker-compose.yml:docker-compose.prod.yml \
  sh scripts/backup.sh --output data/backups/levier-prod
```

备份包含 PostgreSQL custom dump、可选的 `agent_data.tar`、manifest 和
SHA256 校验；不包含 `deploy/secrets/`。备份文件和 secret 都要限制权限，
不要提交到 Git。

默认卷名：

```text
levier-mvp_pgdata        PostgreSQL 数据
data/agent               Agent credentials（host bind mount）
levier-mvp_mcp_socket    运行时 socket，不需要长期备份
```

`docker compose down` 保留 volume；`docker compose down -v` 删除数据卷，只在
明确确认数据不再需要或已恢复成功后使用。

Admin token、daemon token、App key 和 PostgreSQL 密码的轮换步骤见
[`docs/LIFECYCLE.md`](../docs/LIFECYCLE.md)。所有轮换都需要先备份，并保留
回滚用的旧 secret；不要自动轮换。

## 8. 单独构建镜像

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
