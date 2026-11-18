<!-- Levier — process and data lifecycle plan (development document). -->

# 10. 生命周期（进程与数据）

> 状态：10B 进程优雅停止已实现；10C 数据维护、10D 备份/轮换待开始。
> 本文位于 `docs/`，属于内部开发文档，不进入镜像、release、HTTP 响应或对外分发。
> 计划新增的运行状态或维护命令契约放在 `public/service-lifecycle/v1/`（如最终需要对外暴露）。

---

## 1. 目标

让 Levier 的“开始、运行、停止、重启、保留、清理、备份、恢复”都有明确契约，而不是依赖使用者记忆和手工操作。

本阶段覆盖两条生命周期：

```text
进程生命周期
  start → ready → running → stopping → stopped
  crash / restart / graceful shutdown / child process cleanup

数据生命周期
  create → active → terminal → retained → archived / deleted
  migration / retention / prune / backup / restore / rotate
```

---

## 2. 本阶段不做什么

```text
不做进程守护器 / systemd 替代品
不做应用内自动升级
不做多实例 HA / leader election
不做自动数据归档到对象存储
不做自动 secret 轮换服务
不做 GUI 数据管理
不删除 active / queued / running 数据
不提供公共 HTTP 删除或清理接口
```

Docker / systemd / Kubernetes 仍然负责外部进程守护；Levier 负责自身进程的启动、就绪、优雅停止和恢复语义。

---

## 3. 当前状态盘点

### 3.1 进程

```text
levier serve
  → 读取配置
  → 连接 PostgreSQL
  → 检查 pending migration（有 pending 直接退出）
  → 监听 gRPC / HTTP
  → 启动 workflow workers、reaper、recovery
  → 处理 SIGINT / SIGTERM
  → 停止 workers / controller / hub
  → HTTP graceful shutdown
  → gRPC graceful stop
  → 退出
```

已有：

- `migrate up -> serve` entrypoint
- Compose `restart: unless-stopped`
- HTTP / gRPC shutdown timeout
- Harness、MCP Runtime 的 SIGINT/SIGTERM 处理
- App WebSocket、工具 pending 调用的关闭逻辑

仍缺少：

- 明确的 lifecycle 状态和日志阶段
- worker 停止的总超时
- 子进程 / stdio MCP 进程的退出检查
- Docker `stop_grace_period` 约定
- 统一进程退出码说明
- 进程重启时的数据一致性验收

### 3.2 数据

当前 PostgreSQL 表：

```text
apps
daemons
runs
run_events
run_tasks
tool_defs
mcp_server_defs
sessions
tool_invocations
app_mcp_refs
```

当前状态：

```text
runs / run_events 没有保留期
run_tasks 的 DONE / CANCELLED / DEAD 没有清理
tool_invocations 没有保留期
sessions 有 expires_at 索引，但没有 GC
dead-letter 没有清理命令
temp App / Run 会持续累积
Docker volume 有 pgdata / agent_data / mcp_socket
backup / restore 没有脚本级说明
secret / API key 轮换有局部支持，但没有统一流程
```

---

## 4. 进程生命周期契约

### 4.1 状态

建议定义内部状态：

```text
starting
ready
degraded
stopping
stopped
```

`/status` 的 `health` 已经反映 ready/degraded；本阶段可增加：

```json
{
  "lifecycle": {
    "state": "ready",
    "started_at": "...",
    "uptime_seconds": 123,
    "last_shutdown": "clean|forced|unknown"
  }
}
```

如果对外暴露，必须加入 `status_version` 的兼容规则；第一版可以只在日志和 `selfcheck` 中使用，不急于扩展 `/status`。

### 4.2 启动顺序

```text
1. parse config
2. open database
3. verify migrations
4. start gRPC listener
5. start HTTP listener
6. start workflow workers / reaper
7. recover queued / active runs
8. ready
```

失败规则：

```text
config fail       → exit 1
DB fail           → exit 1
migration pending → exit 1
HTTP/gRPC bind fail → exit 1
worker start fail → exit 1
```

启动完成前不能报告 ready。

### 4.3 停止顺序

建议固定：

```text
1. stop accepting new HTTP/gRPC requests
2. stop workflow workers（有总超时）
3. close controller / hub / app connections
4. shutdown HTTP（有总超时）
5. graceful stop gRPC（有总超时）
6. close MCP / stdio child processes
7. close database pool
8. exit 0
```

超时后允许强制退出，但必须记录：

```text
forced shutdown
active runs
pending handlers
```

### 4.4 信号

```text
SIGINT   → graceful stop
SIGTERM  → graceful stop
SIGHUP   → 不做配置热加载；当前忽略或记录
SIGKILL  → 无法捕获；由 restart policy 和 DB 恢复处理
```

### 4.5 Compose

增加/确认：

```yaml
stop_grace_period: 30s
restart: unless-stopped
```

文档明确：

```text
docker compose down      保留 volume
docker compose down -v   删除数据库/凭据/socket 数据
```

---

## 5. 数据生命周期契约

### 5.1 数据分类

| 数据 | 生命周期 | 默认 |
|---|---|---|
| App / API key | 长期，显式轮换 | 保留 |
| Run / Run events | 运行期 + 保留期 | 保留 |
| RunTask | 运行期 + 短期保留 | 保留 |
| Tool invocation | 运行期 + 保留期 | 保留 |
| Session | expires_at + GC | 保留 |
| Dead-letter task | 诊断期 | 保留 |
| Logs | 外部日志系统 | 不自动删除 |
| Secrets | 显式轮换 | 不自动删除 |
| Docker volumes | 显式备份 / 删除 | 保留 |

**第一版默认不自动删除任何用户数据。**

### 5.2 保留配置（建议）

```yaml
retention:
  enabled: false
  run_days: 30
  event_days: 30
  tool_invocation_days: 30
  workflow_days: 7
  session_days: 7
  dead_letter_days: 30
  batch_size: 1000
```

规则：

```text
enabled=false      不删除任何数据
只删除终态数据
不删除 queued / assigned / running 的 Run
外键级联删除前必须确认影响范围
按 batch 分批删除
每次删除记录 count 和耗时
dry-run 必须显示候选数量
```

### 5.3 维护命令（建议）

```sh
levier maintenance status
levier maintenance prune --dry-run
levier maintenance prune --yes
```

`status` 输出：

```text
runs.total / terminal / active
events.count
tool_invocations.count
run_tasks.pending / leased / done / dead
sessions.active / expired
oldest_terminal_age
```

`prune` 默认 dry-run；只有显式 `--yes` 才删除。

### 5.4 Session 过期

虽然 `sessions` 有 `expires_at` 和索引，但当前没有 GC。建议：

```text
先查询 expires_at < now() - grace
标记为 expired
保留 session / checkpoint 一段时间后按 retention 清理
绝不删除仍被 active run 引用的 session
```

### 5.5 App / 凭据生命周期

当前 App 删除能力不完整。建议分两步：

```text
1. app archive（软删除，禁止新 key / 新 run）
2. 后续版本再提供硬删除
```

规则：

```text
archive 不立即删除 Run / App 历史
archive 后 App key 不能再创建 Run
rotate key 必须显式确认
selfcheck / daemon-check 创建的临时 App 需要可识别名称和清理策略
```

### 5.6 备份与恢复

不把备份做成隐式行为。建议：

```sh
pg_dump ... > backup.sql
pg_restore ... < backup.sql
```

提供：

```text
docs/LIFECYCLE.md 中的备份/恢复步骤
scripts/backup.sh / restore.sh（可选）
备份前检查 migration version
恢复后运行 migrate status / selfcheck
```

Docker volume：

```text
pgdata        → PostgreSQL 数据
agent_data    → App credentials
mcp_socket    → 仅运行时 socket，不需要长期备份
```

### 5.7 Secret / API key 轮换

```text
Admin token      修改 secret 后重启 levier + agent
daemon token     修改 JSON + harness secret 后重启对应服务
App key          app rotate-key，更新 Agent credentials
数据库密码       修改 postgres secret + DSN，按 PostgreSQL 实际密码流程
```

所有轮换都必须有步骤、影响范围、回滚说明。

---

## 6. 对外暴露边界

第一版不新增公共 `/lifecycle` 或 `/maintenance` HTTP 接口。

已有安全面：

```text
/healthz
/readyz
/meta
/status
/metrics
```

维护命令只作为 CLI：默认只读，破坏性操作需要 `--yes`。

如果后续需要远程 maintenance：

```text
POST /admin/maintenance/prune
```

必须 Admin token、dry-run 默认、审计日志、禁止删除 active 数据。

---

## 7. 代码结构

### 7.1 进程

```text
cmd/levier/main.go
  启动/停止顺序
  lifecycle state
  shutdown 总超时
  exit code

internal/observe/
  lifecycle 日志字段或事件

internal/workflow/
  Pool.StopContext(ctx)
  in-flight handler 超时策略
```

### 7.2 数据

```text
internal/maintenance/
  doc.go
  status.go
  prune.go
  retention.go
  maintenance_test.go

internal/runstore/
  Stats
  CountTerminalBefore
  DeleteTerminalBefore

internal/workflow/
  Stats / cleanup terminal tasks

internal/db/
  migration / backup helper（不自动执行）
```

### 7.3 CLI

```text
cmd/levier/maintenance.go
  levier maintenance status
  levier maintenance prune --dry-run
  levier maintenance prune --yes
```

可选：`levier lifecycle status` 或在 `levier status` 中显示 lifecycle。

---

## 8. 分阶段实施

### 10A：生命周期契约

- 冻结进程状态、启动/停止顺序、退出码
- 冻结数据保留规则和默认值
- 写出备份/恢复/轮换流程
- 不删除任何数据

### 10B：进程优雅停止

- lifecycle 状态与日志
- worker stop timeout
- HTTP/gRPC 停止顺序和强制超时
- Compose `stop_grace_period`
- 子进程清理测试

### 10C：数据维护

- `internal/maintenance`
- `maintenance status`
- `maintenance prune --dry-run`
- 终态 Run / events / tool invocations / run_tasks 的批量清理
- session GC
- 默认关闭 retention

### 10D：备份、恢复、轮换

- 备份/恢复脚本或文档
- Docker volume 指南
- Admin / daemon / App key / DB password 轮换步骤
- 不自动轮换

### 10E：测试与验收

```text
go test ./...
make lint
make contract-check
make harness-test
make mcp-runtime-test
make counter-agent-test
docker compose config --quiet
docker compose up -d --no-build
docker compose --profile smoke run --rm smoke
selfcheck
```

---

## 9. 测试矩阵

| 测试 | 覆盖 |
|---|---|
| 启动失败 | config / DB / migration / port |
| 优雅停止 | SIGTERM、in-flight、worker、HTTP/gRPC |
| 强制停止 | 超时、exit code、日志 |
| 重启恢复 | queued / running / assigned Run |
| retention dry-run | 只读、候选数量、不删除 |
| retention prune | 只删终态、batch、事务、audit |
| session GC | expired / grace / active run 保护 |
| backup / restore | migration 版本、数据数量、selfcheck |
| secret rotation | admin / daemon / app key / DB password |
| 输出安全 | 不出现 token / DSN / secret |

---

## 10. 完成标准

```text
[x] 启动顺序和失败退出码有文档
[x] SIGTERM/SIGINT 优雅停止有测试
[x] stop 超时后强制退出并记录
[x] Compose stop_grace_period 配置
[ ] retention 默认关闭
[ ] maintenance status 可用
[ ] maintenance prune 默认 dry-run
[ ] 只删除终态数据
[ ] session GC 不删除 active 数据
[ ] 备份/恢复文档可执行
[ ] secret/API key 轮换步骤明确
[ ] Docker volume 删除风险明确
[ ] smoke / selfcheck / CI 通过
```

---

## 11. 需要确认的决策

1. 第一版 retention 默认关闭，避免自动删数据
2. 维护操作只做 CLI，不做公共 HTTP 接口
3. 先软删除 App；硬删除留到后续
4. 只清理终态 Run / events / tool invocations / terminal tasks
5. 备份使用 PostgreSQL 原生备份工具，不做应用内备份服务
6. 不实现进程守护器；Docker / systemd 仍负责外部重启
