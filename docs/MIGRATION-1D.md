<!-- Levier — 1D stateful cutover (Compose project, service, database, metrics). -->

# 1D 迁移：有状态标识符

| 项目 | 内容 |
|---|---|
| 状态 | 已执行（2026-11-15，fresh DB 方案） |
| 影响 | Compose 项目名、服务名、数据库用户/库名、socket 路径、指标前缀 |
| 风险 | 会创建新容器与新数据卷；旧 stack 必须停止才能释放 8080/9090 |
| 原则 | 先备份，再切换；旧卷保留，便于回滚 |

---

## 1. 变更对照

```text
Compose 项目     middleware-mvp            → levier-mvp
Compose 服务     middleware                → levier
PostgreSQL 用户  middleware                → levier
PostgreSQL 库    middleware                → levier
测试库           middleware_test           → levier_test
MCP socket       /run/middleware/mcp.sock  → /run/levier/mcp.sock
本地 MCP socket  /tmp/middleware-mcp.sock  → /tmp/levier-mcp.sock
指标前缀         mw_                       → levier_
```

不变：

```text
宿主机端口 8080 / 9090
PostgreSQL 数据格式与迁移版本
App / Harness / MCP 协议
```

---

## 2. 先备份

旧 stack 还在运行时执行：

```sh
# 全库备份（custom format）
docker exec middleware-mvp-postgres-1 \
  pg_dump -U middleware -d middleware -Fc \
  > levier-before-1d.dump

# 可选：逻辑备份，便于人工检查
docker exec middleware-mvp-postgres-1 \
  pg_dump -U middleware -d middleware \
  > levier-before-1d.sql

# 保留旧卷（不要 down -v）
docker compose -p middleware-mvp down
```

确认卷还在：

```sh
docker volume ls | grep middleware-mvp
```

预期：

```text
middleware-mvp_pgdata
middleware-mvp_mcp_socket
middleware-mvp_agent_data
```

---

## 3. 方案 A：本地演示，接受空库

适合本地 smoke、没有需要保留的 Run/App。

```sh
docker compose -p levier-mvp up --build -d
docker compose -p levier-mvp --profile smoke run --rm smoke
```

新项目会创建新卷：

```text
levier-mvp_pgdata
levier-mvp_mcp_socket
levier-mvp_agent_data
```

agent 会重新 bootstrap 一个新 App。旧卷保留，回滚见第 5 节。

---

## 4. 方案 B：保留数据

适合需要保留 Run、事件、App、工具注册的场景。

### 4.1 启动新的 PostgreSQL（只启动 postgres）

```sh
docker compose -p levier-mvp up -d postgres
```

等健康：

```sh
docker exec levier-mvp-postgres-1 pg_isready -U levier -d levier
```

### 4.2 恢复数据

```sh
docker exec -i levier-mvp-postgres-1 \
  pg_restore -U levier -d levier --no-owner --no-privileges \
  < levier-before-1d.dump
```

如果 `pg_restore` 报 role 不存在，说明备份里有 `middleware` 角色引用。  
本地 MVP 建议用 `--no-owner --no-privileges`；生产环境应先创建角色并做 ownership 映射。

### 4.3 启动新 stack

```sh
docker compose -p levier-mvp up --build -d
```

### 4.4 App 凭据

旧的 `agent_data` 卷里是旧 App 的 `credentials.json`。  
新 project 使用新卷，agent 会重新 bootstrap。  
如果必须复用旧 App：

```sh
docker run --rm \
  -v middleware-mvp_agent_data:/from \
  -v levier-mvp_agent_data:/to \
  alpine sh -c 'cp -a /from/. /to/'
```

然后重启 agent：

```sh
docker compose -p levier-mvp restart agent
```

前提：数据库已经恢复，旧 App 的 key hash 还在。

### 4.5 smoke

```sh
docker compose -p levier-mvp --profile smoke run --rm smoke
```

---

## 5. 回滚

旧容器已经移除，但旧卷和旧镜像仍然保留：

```text
卷：middleware-mvp_pgdata / _mcp_socket / _agent_data
镜像：middleware-mvp-*:latest
备份：data/migration-1d/levier-before-1d.dump
```

因为 `docker-compose.yml` 已经改成 `levier-mvp` / `levier`，不能直接用当前文件执行 `docker compose -p middleware-mvp up` 回滚。  
如果需要回滚代码：

```sh
# 1. 停新 stack
docker compose -p levier-mvp down

# 2. 恢复 1D 之前的 compose 文件与代码（git checkout / 旧分支）
# 3. 用旧镜像启动
docker compose -p middleware-mvp up -d
```

如果只是需要旧数据，优先按第 4 节把备份恢复到新 stack，而不是整体回滚。

---

## 6. 指标与看板

指标前缀从 `mw_` 改为 `levier_`：

```text
mw_runs_total              → levier_runs_total
mw_events_total            → levier_events_total
mw_tool_invocations_total  → levier_tool_invocations_total
mw_daemon_capacity         → levier_daemon_capacity
mw_ws_connections          → levier_ws_connections
```

需要同步更新：

```text
Grafana dashboard
Prometheus recording rules
告警表达式
本地 curl / grep 脚本
```

没有历史指标迁移；新旧前缀之间不兼容。

---

## 7. 服务发现

Compose 内部服务名从 `middleware` 改为 `levier`：

```text
http://middleware:8080  → http://levier:8080
middleware:9090         → levier:9090
```

宿主机访问不变：

```text
http://127.0.0.1:8080
127.0.0.1:9090
```

---

## 8. 验收清单

```text
[ ] 旧库备份文件存在且可读
[ ] docker compose -p levier-mvp config 通过
[ ] postgres 健康检查使用 levier/levier
[ ] middleware / harness / agent / smoke 的 URL 指向 levier
[ ] MCP socket 指向 /run/levier/mcp.sock
[ ] /metrics 只出现 levier_ 前缀
[ ] smoke PASS
[ ] 旧卷 / 旧镜像 / 备份仍存在
```

---

## 9. 执行记录

2026-11-15 实际执行结果：

```text
方案          fresh DB（未恢复旧数据）
备份          data/migration-1d/levier-before-1d.dump（31 KB）
旧 stack      middleware-mvp 已停止并移除容器
旧卷          middleware-mvp_pgdata / _mcp_socket / _agent_data 保留
新 stack      levier-mvp 已启动
新卷          levier-mvp_pgdata / _mcp_socket / _agent_data
smoke         docker-smoke: PASS
指标          /metrics 中 levier_ 前缀 210 行，mw_ 前缀 0 行
App           agent 重新 bootstrap 了新 App
构建          Docker 内 proxy.golang.org 不可达，使用
              GOPROXY=https://goproxy.cn,direct 构建 levier 镜像
```

旧数据仍在备份和旧卷中，但**当前新库是空的（除新 bootstrap 的 App）**。  
如果需要恢复旧数据，按第 4 节执行；这会替换当前新库，并需要把旧 `middleware-mvp_agent_data` 复制到新卷，或重新 bootstrap。
