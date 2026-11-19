<!-- Levier — backup, restore, volume, and secret rotation runbook. -->

# 生命周期运维：备份、恢复与轮换

> 状态：10D 已实现；脚本为显式操作，不自动备份，也不自动轮换。
> 本文属于内部开发/运维文档，不进入镜像、release 或公开 HTTP 接口。
> 相关：`docs/LIFECYCLE_PLAN.md`、`deploy/README.md`、`docs/CONFIG.md`。

---

## 1. 边界

```text
数据库备份使用 PostgreSQL 原生 pg_dump / pg_restore
不实现应用内备份服务
不自动执行 restore
不自动轮换 secret
不把真实 secret 放进 Git、镜像、备份目录或日志
```

生产环境应把备份文件放到受限、加密、可审计的存储中；本仓库的
`data/backups/` 只是本地开发默认目录，已被 `.gitignore` 忽略。

---

## 2. 备份

### 2.1 备份什么

| 内容 | 是否由脚本处理 | 说明 |
|---|---|---|
| PostgreSQL 数据库 | 是 | `pg_dump --format=custom`，包含 schema 与数据 |
| `agent_data` volume | 是 | Agent 的 `credentials.json` 等 |
| `deploy/secrets/*` | 否 | 必须单独加密保存，不能混入普通备份 |
| `mcp_socket` volume | 否 | 只是运行时 socket，不需要长期备份 |
| `.env` / 账号凭据 | 否 | 由部署系统或密码管理器管理 |

PostgreSQL 使用逻辑备份，而不是直接复制 `pgdata` 目录。逻辑备份可读、
可校验、可跨容器版本恢复，不会把 PostgreSQL 内部文件状态复制到目标环境。

### 2.2 使用脚本

开发栈：

```sh
sh scripts/backup.sh
```

指定目录：

```sh
sh scripts/backup.sh --output data/backups/levier-20261119
```

只备份数据库，不复制 Agent 凭据：

```sh
sh scripts/backup.sh --db-only --output data/backups/levier-db-only
```

生产式 Compose：

```sh
COMPOSE_FILE=docker-compose.yml:docker-compose.prod.yml \
  sh scripts/backup.sh --output data/backups/levier-prod
```

脚本要求 `postgres` 服务正在运行。默认还要求 `agent` 服务正在运行；
如果只需要数据库备份，使用 `--db-only`。`agent_data` 是运行中在线复制，
不要在 App key 轮换或凭据写入的同时执行；需要严格一致时先停止 Agent。

### 2.3 输出结构

```text
data/backups/levier-<UTC time>/
  postgres.dump       PostgreSQL custom-format dump
  agent_data.tar      agent_data volume（可选）
  manifest.txt       格式、时间、migration version、文件清单
  SHA256SUMS         文件校验和
```

脚本会：

```text
设置 umask 077
目录权限 0700
文件权限 0600
先写临时目录，成功后原子 rename
校验 dump 非空
生成 SHA256SUMS
```

`manifest.txt` 不包含 token、密码或 DSN。

### 2.4 备份后的检查

```sh
ls -l data/backups/levier-20261119
cat data/backups/levier-20261119/manifest.txt
(cd data/backups/levier-20261119 && sha256sum -c SHA256SUMS)
```

在开发环境可以额外确认 dump 可读：

```sh
docker compose exec -T postgres pg_restore -l \
  < data/backups/levier-20261119/postgres.dump | head
```

备份前记录 migration version。恢复后必须确认版本与备份时一致或已经安全
迁移；不要用旧代码直接连接已经迁移到新版本的数据库。

### 2.5 秘密的处理

`deploy/secrets/` 中的文件不会被 `backup.sh` 包含。需要单独处理：

```sh
# 示例：放到受控介质，路径和加密方式由生产环境决定
umask 077
tar -C deploy -czf /secure/levier-secrets.tar.gz secrets
```

真实 secret 不应上传 Git、对象存储公共桶、聊天工具或工单附件。

---

## 3. 恢复

### 3.1 先演练

```sh
sh scripts/restore.sh \
  --backup data/backups/levier-20261119 \
  --dry-run
```

`--dry-run` 会校验 manifest 和 SHA256，不会停止服务，也不会修改数据库。

### 3.2 执行恢复

确认备份和演练结果后：

```sh
sh scripts/restore.sh \
  --backup data/backups/levier-20261119 \
  --yes
```

脚本按顺序：

```text
校验备份格式和 SHA256
确保 postgres 正在运行
停止 levier / harness / agent，停止写入
清理该数据库的残留连接
pg_restore --clean --if-exists --no-owner --no-privileges
可选恢复 agent_data
重新启动 levier / harness / agent
读取 migrate status
```

如果 PostgreSQL 或 `agent_data` 恢复失败，脚本会让相关服务保持停止状态，
不会在失败后继续启动并接受流量。保留备份目录和输出日志，先排查再重试。

### 3.3 只恢复数据库

```sh
sh scripts/restore.sh \
  --backup data/backups/levier-db-only \
  --skip-agent-data \
  --yes
```

### 3.4 恢复后验证

```sh
docker compose ps
docker compose exec levier /app/levier migrate status
docker compose --profile smoke run --rm smoke
```

再检查：

```text
migration current 与 manifest 兼容
pending = 0
/readyz 为 ready
App 可连接 WebSocket
Harness daemon 已注册
MCP runtime socket 正常
一个 echo Run 可以完成
```

如果数据库恢复了旧数据，而 `agent_data` 仍包含当前 App key，需要确认 App
key hash 与数据库一致；不一致时重新 bootstrap 或恢复对应的凭据卷。

### 3.5 失败与回滚

```text
不要执行 docker compose down -v
不要删除现有 pgdata / agent_data volume
不要覆盖原始备份
先停止流量，再从最近一次已验证备份重试
```

恢复脚本不会自动删除卷；如果恢复失败，旧卷和旧备份仍是回退依据。生产
恢复前应先保存当前数据库的额外快照，避免“恢复旧备份后无法回到刚才状态”。

---

## 4. Docker volume 指南

项目名为 `levier-mvp` 时，默认卷名：

```text
levier-mvp_pgdata
levier-mvp_agent_data
levier-mvp_mcp_socket
```

含义：

| 卷 | 内容 | 备份策略 |
|---|---|---|
| `pgdata` | PostgreSQL 数据目录 | 用 `pg_dump` 逻辑备份；不要热复制目录 |
| `agent_data` | Agent credentials / 本地凭据 | 由 `backup.sh` 复制，或停机后复制 |
| `mcp_socket` | MCP 运行时 Unix socket | 不备份，重建容器即可 |

查看：

```sh
docker volume ls | grep levier-mvp
docker volume inspect levier-mvp_pgdata
```

`docker compose down` 保留 volume；`docker compose down -v` 会删除 volume，
包括数据库和凭据。后者只应在明确确认“数据不要了”或已经从备份恢复成功后
使用。

不要把 `pgdata` 目录直接复制到另一个 PostgreSQL 版本或另一个平台作为
常规备份；需要迁移时使用 `pg_dump`/`pg_restore`。

---

## 5. Secret / API key 轮换

所有轮换都必须先备份，并记录：

```text
轮换对象
影响范围
新值存放位置
验证命令
回滚方式
```

任何 token、密码、App key 都不要出现在 shell history、聊天记录或日志中。
生产环境使用 `openssl rand -hex 32` 或密码管理器生成随机值。

### 5.1 Admin token

影响：

```text
Levier HTTP Admin API
Agent bootstrap 使用的 LEVIER_APP_ADMIN_TOKEN
运维脚本里的 LEVIER_ADMIN_TOKEN
```

步骤：

```sh
# 1. 生成新 token，不要直接打印到共享日志
openssl rand -hex 32 > /secure/new-admin-token

# 2. 原子替换生产 secret，保持 0600
install -m 0600 /secure/new-admin-token deploy/secrets/levier_admin_token

# 3. 让 levier 和 agent 重新读取挂载文件
docker compose \
  -f docker-compose.yml \
  -f docker-compose.prod.yml \
  up -d --force-recreate levier agent

# 4. 在受控环境中把新 token 写入 LEVIER_ADMIN_TOKEN，再验证 Admin API
#    不要把 token 值直接写进命令历史
curl -fsS -H "Authorization: Bearer $LEVIER_ADMIN_TOKEN" \
  http://127.0.0.1:8080/admin/mcp >/dev/null

# 5. 用旧 token 单独请求一次 /admin/mcp，确认返回认证失败
```

旧 token 应返回认证失败。App key 不受 Admin token 轮换影响。

### 5.2 Daemon token

影响：

```text
Levier gRPC daemon 认证
Harness 的 daemon auth token
其他 daemon 客户端
```

步骤：

```sh
# 1. 生成新 token
openssl rand -hex 32 > /secure/new-daemon-token

# 2. 更新 Levier 的 token JSON（只显示 key，不显示 value）
#    deploy/secrets/levier_daemon_tokens.json
#    例如：{"daemon-local-1":"<新 token>"}

# 3. 更新 Harness 使用的同一个 token
install -m 0600 /secure/new-daemon-token \
  deploy/secrets/levier_harness_daemon_token

# 4. 重新创建 Levier 和 Harness
docker compose \
  -f docker-compose.yml \
  -f docker-compose.prod.yml \
  up -d --force-recreate levier harness

# 5. 检查注册和主动链路
docker compose exec levier /app/levier selfcheck --deep \
  --admin-token "$(cat deploy/secrets/levier_admin_token)" \
  --daemon-token "$(cat deploy/secrets/levier_harness_daemon_token)"
```

JSON 中每个 daemon 的 token 必须和对应客户端一致。只轮换一个 daemon 时，
其他 daemon 的 token 保持不变；对应客户端需要重启以重新读取新值。

### 5.3 App key

影响：

```text
App 的 API key
Agent / App 的 credentials 文件
使用该 App key 的所有客户端
```

使用已有 CLI：

```sh
levier app rotate-key \
  --credentials /path/to/app.json \
  --admin-token "$LEVIER_ADMIN_TOKEN" \
  --yes
```

行为：

```text
新 key 只返回一次
旧 key 立即失效
credentials 文件生成 .bak
消费方必须更新凭据并重连
未完成请求可能返回认证失败，需要重试
```

轮换后检查 App 状态、WebSocket 和一次 echo Run。`.bak` 文件同样包含 secret，
应限制权限并按组织策略清理。

### 5.4 PostgreSQL 密码

影响：

```text
PostgreSQL 角色密码
Levier 的 database.url / database.url_file
任何直接连接数据库的运维工具
```

推荐步骤：

```sh
# 1. 先备份数据库
sh scripts/backup.sh --output data/backups/before-db-password-rotation

# 2. 交互式修改角色密码，避免进入 shell history
docker compose exec postgres psql -U levier -d levier
# 在 psql 中执行：
#   \password levier
#   \q

# 3. 更新密码 secret 和 DSN secret
#    deploy/secrets/levier_postgres_password
#    deploy/secrets/levier_database_url
#    确保 DSN 中的密码与新密码一致；优先使用 URL 安全字符

# 4. 让 Levier 使用新 DSN
docker compose \
  -f docker-compose.yml \
  -f docker-compose.prod.yml \
  up -d --force-recreate levier

# 5. 验证
docker compose exec levier /app/levier migrate status
docker compose --profile smoke run --rm smoke
```

注意：已有 PostgreSQL volume 不会因为修改 Compose 环境变量而自动修改
角色密码。必须实际更新数据库角色密码，再更新 DSN。不要用 `down -v`
“重新初始化”来轮换密码，那会删除数据。

回滚时把角色密码改回旧值，恢复旧 secret 和 DSN，再重建 Levier；旧值要在
验证成功前保留在受控位置。

### 5.5 轮换记录建议

```text
时间
操作者
对象（admin / daemon id / app / postgres）
新值存放位置
验证结果
回滚窗口
关联备份目录
```

---

## 6. 验收清单

```text
[ ] 备份目录存在，manifest 可读
[ ] SHA256SUMS 校验通过
[ ] 备份包含当前 migration version
[ ] 已单独保存并限制 deploy/secrets 权限
[ ] restore --dry-run 通过
[ ] 恢复后 postgres / levier / harness / agent 正常
[ ] migrate status pending = 0
[ ] App WebSocket 和 echo Run 正常
[ ] 轮换后的新凭据可用，旧凭据已失效
[ ] 旧卷没有被 down -v 删除
[ ] 回滚方式和旧 secret 仍可用
```
