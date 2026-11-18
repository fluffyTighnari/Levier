<!-- Levier — status overview plan (development document). -->

# 8. 状态总览改造计划

> 状态：计划阶段，尚未改代码。
> 本文位于 `docs/`，属于内部开发文档，不进入镜像、release、HTTP 响应或对外分发。
> 对外暴露的状态契约放在 `public/service-status/v1/`。

---

## 1. 目标

让正在运行的 Levier 能通过一个稳定接口告诉外部“现在系统怎么样”：

```sh
curl http://localhost:8080/status
```

或：

```sh
levier status
```

状态总览回答：

- 服务是否存活
- 数据库是否就绪
- 当前版本和运行时长
- 已连接 daemon 数量、容量、使用量
- App WebSocket 连接数
- Run 各状态数量
- workflow 队列深度和 pending 任务年龄
- 各运行时的容量分布

它和 `/meta` 的分工：

```text
/meta     我能做什么（静态契约）
/status   我现在怎么样（动态快照）
/healthz  进程是否存活
/readyz   依赖是否就绪
/metrics  Prometheus 原始指标
```

---

## 2. 本阶段不做什么

```text
不做历史时间序列
不做日志查询
不做单 Run / 单 App / 单 daemon 详情列表
不做取消、重试等操作
不做多实例聚合
不做 GUI
不做动态配置修改
不暴露 daemon_id / app_id / run_id
不暴露数据库 DSN、token、secret
```

需要单对象详情时，使用现有的 Run / App / daemon 接口；需要历史趋势时，使用外部 Prometheus。

---

## 3. 接口设计

### 3.1 路径

```text
GET /status
```

- 根路径，和 `/meta`、`/healthz`、`/readyz` 同级
- 不需要 Admin token 或 App key
- 只返回聚合状态，不返回资源标识
- 响应头：

```text
Content-Type: application/json
Cache-Control: no-store
X-Request-ID: ...
```

> 当前 `/metrics` 已经公开聚合指标；`/status` 不增加新的敏感信息。
> 后续如果需要单 daemon / 单 App 详情，另做受保护的 `/admin/status`，不复用本接口。

### 3.2 响应结构

```json
{
  "status_version": 1,
  "service": "levier",
  "as_of": "2026-11-18T12:34:56Z",
  "uptime_seconds": 3600,
  "build": {
    "version": "0.1.0-rc.5",
    "commit": "2ce014d"
  },
  "health": {
    "liveness": "ok",
    "readiness": "ok",
    "database": "up"
  },
  "daemons": {
    "connected": 1,
    "healthy": 1,
    "capacity": 4,
    "used": 1,
    "available": 3,
    "by_runtime": {
      "normal": {
        "connected": 1,
        "healthy": 1,
        "capacity": 4,
        "used": 1,
        "available": 3
      }
    },
    "strategies": {
      "react": 1,
      "echo": 1
    }
  },
  "apps": {
    "registered": 1,
    "websocket_connections": 1
  },
  "runs": {
    "total": 1,
    "active": 0,
    "by_status": {
      "queued": 0,
      "assigned": 0,
      "running": 0,
      "completed": 1,
      "failed": 0,
      "cancelled": 0,
      "timeout": 0
    }
  },
  "workflow": {
    "total": 1,
    "pending": 0,
    "leased": 0,
    "done": 1,
    "dead": 0,
    "cancelled": 0,
    "oldest_pending_seconds": 0
  }
}
```

### 3.3 字段含义

| 字段 | 含义 |
|---|---|
| `status_version` | 状态契约版本，初始为 `1` |
| `service` | 固定为 `levier` |
| `as_of` | 快照生成时间，UTC RFC3339 |
| `uptime_seconds` | 进程运行秒数 |
| `build.version` / `commit` | 运行版本 |
| `health.liveness` | 进程存活 |
| `health.readiness` | 依赖是否就绪 |
| `health.database` | `up` / `down` |
| `daemons.connected` | 当前连接 daemon 数 |
| `daemons.capacity` | daemon 总容量 |
| `daemons.used` | 已使用容量 |
| `daemons.available` | 可用容量，最小为 0 |
| `daemons.by_runtime` | 按 runtime type 聚合 |
| `daemons.strategies` | 声明支持各策略的 daemon 数 |
| `apps.registered` | 数据库中的 App 数量 |
| `apps.websocket_connections` | 当前 App WebSocket 连接数 |
| `runs.total` | Run 总数 |
| `runs.active` | assigned + running |
| `runs.by_status` | 按 Run 状态计数 |
| `workflow.total` | RunTask 总数 |
| `workflow.pending` | 等待中的任务 |
| `workflow.leased` | 已租用任务 |
| `workflow.done` | 已完成任务 |
| `workflow.dead` | 死信任务 |
| `workflow.cancelled` | 已取消任务 |
| `workflow.oldest_pending_seconds` | 最老 pending 任务年龄 |

### 3.4 降级语义

`/status` 不应因为单个统计源失败而完全不可用。

建议：

```text
数据库可用：
  health.readiness = ok
  health.database = up
  runs / apps 正常统计

数据库不可用：
  HTTP 仍返回 200
  health.readiness = degraded
  health.database = down
  runs / apps 使用可获得的零值或省略计数
  daemons 仍可从内存读取
```

`/readyz` 仍然负责返回 503；`/status` 是一份“可读的运行快照”，不是探针。

---

## 4. 数据来源

| 数据 | 来源 |
|---|---|
| daemon 数量、容量、运行数 | `connection.Hub.List()` / `Count()` |
| daemon 策略 | `DaemonInfo.SupportedStrategies` |
| App WebSocket 连接 | `AppHub` 新增聚合 Stats |
| App 注册数 | `runstore` 新增 `CountApps` |
| Run 状态计数 | `runstore` 新增 `Stats` |
| workflow 队列 | `workflow.Pool.Stats()` |
| 数据库就绪 | `db.Health()` |
| 版本 | `internal/version` |
| 运行时间 | 进程启动时间 |

所有统计使用短超时（建议 2 秒），避免状态接口拖慢。

---

## 5. 代码结构

### 5.1 新增 `internal/status`

```text
internal/status/
  doc.go
  status.go
  status_test.go
```

职责：

- 定义 `Snapshot`、`Health`、`DaemonSummary`、`AppSummary`、`RunSummary`、`WorkflowSummary`
- 注入各个统计源
- 生成一次性快照
- 不持有 HTTP 逻辑

### 5.2 HTTP

```text
internal/httpapi/status.go
```

新增：

```go
type StatusProvider interface {
    Snapshot(ctx context.Context) (status.Snapshot, error)
}
```

`httpapi.Config` 增加：

```go
Status StatusProvider
```

路由：

```go
s.mux.HandleFunc("GET /status", s.handleStatus)
```

### 5.3 启动装配

在 `cmd/levier/main.go` 创建：

```go
startedAt := time.Now().UTC()
statusService := status.New(status.Options{
    Hub:       hub,
    AppHub:    appHub,
    Runs:      runs,
    Queue:     queue,
    DB:        pool,
    Version:   version.Version,
    Commit:    version.Commit,
    StartedAt: startedAt,
})
```

传入 `httpapi.Config`。

### 5.4 CLI

重写 `cmd/levier/status.go`：

- 优先请求 `/status`
- `--json` 输出服务端快照
- 人类输出保持现有易读格式，并增加状态版本、运行时长、workflow 信息
- 如果服务端是旧版本没有 `/status`，可以回退到现有的 `/healthz` + `/readyz` + `/metrics` 读取方式，并提示：

```text
status endpoint unavailable; using legacy metrics
```

兼容窗口结束后删除回退逻辑。

### 5.5 文档

```text
docs/STATUS_OVERVIEW_PLAN.md     本计划
docs/CLI_REFERENCE.md            status 新行为
docs/README.md                   索引
docs/TODO.md                     8 阶段清单
public/service-status/v1/        对外 schema / example
deploy/smoke.py                  status 检查
```

---

## 6. 分阶段实施

### 8A：契约冻结

- 定义 `status_version: 1`
- 建立 `public/service-status/v1/schema.json`
- 建立 `public/service-status/v1/example.json`
- 确认降级语义、公开边界、不暴露标识

验收：

- schema 可解析
- example 不含 secret / ID
- 字段和来源明确

### 8B：状态聚合核心

- 新增 `internal/status`
- 新增 `runstore.Stats` / `CountApps`
- 新增 `AppHub.Stats`
- 汇总 daemon、App、Run、workflow
- 单元测试

验收：

- 各统计源可独立测试
- 数据库失败时能生成降级快照
- 不包含资源标识

### 8C：HTTP `/status`

- 添加 `GET /status`
- 接入 `StatusProvider`
- 无认证访问
- `no-store`
- 路由 / OpenAPI / HTTP 测试
- smoke 增加 `status: ok`

验收：

```sh
curl -s http://localhost:8080/status
```

返回完整聚合状态。

### 8D：CLI `levier status`

- 优先读取 `/status`
- 保留旧 metrics 回退
- 更新人类输出和 JSON
- 参数、超时、服务不可达测试
- 更新 CLI_REFERENCE

验收：

```sh
levier status
levier status --json
```

输出与服务端 `/status` 一致。

### 8E：文档、smoke、验收

- 更新 README / deploy README
- 更新 TODO
- 公开 schema / example 校验
- 完整回归
- CI 和 Compose smoke

验收：

```sh
go test ./...
make lint
make contract-check
make harness-test
make mcp-runtime-test
make counter-agent-test
docker compose config --quiet
docker compose up -d --no-build
docker compose --profile smoke run --rm smoke
```

---

## 7. 测试矩阵

| 测试 | 覆盖 |
|---|---|
| status 聚合单元测试 | 各来源合并、容量计算 |
| 降级测试 | 数据库不可用时返回 200 + degraded |
| secret / ID 泄露测试 | 不出现 token、DSN、daemon_id、app_id、run_id |
| HTTP `/status` 测试 | 200、JSON、无认证、no-store |
| 方法测试 | `POST /status` 返回 405 |
| OpenAPI / 路由测试 | `/status` 已登记 |
| CLI status 测试 | 人类输出、JSON、超时、旧服务回退 |
| workflow 测试 | pending / leased / dead 映射 |
| smoke | `status: ok` |
| 回归测试 | `/meta`、`/healthz`、`/readyz`、`/metrics` 不变 |

---

## 8. 公开契约

建议新增：

```text
public/service-status/v1/schema.json
public/service-status/v1/example.json
```

它们和 `public/service-meta/v1/` 同级，属于对外暴露内容。

`docs/STATUS_OVERVIEW_PLAN.md` 只解释设计，不作为客户端契约。

---

## 9. 风险与控制

| 风险 | 控制 |
|---|---|
| `/status` 暴露资源标识 | 只返回聚合计数，不返回 IDs |
| 慢查询拖垮状态接口 | 2 秒超时 + 索引；失败降级 |
| 数据库故障导致接口不可用 | 返回 200 + degraded；`/readyz` 仍 503 |
| 状态与 metrics 数值不一致 | 统一从组件/数据库读取，不解析 Prometheus 文本 |
| 旧 CLI 无法连接新服务 | 先 `/status`，404 回退 metrics |
| 统计口径漂移 | schema + 单元测试固定字段含义 |

---

## 10. 完成标准

```text
[ ] GET /status 无需认证即可访问
[ ] 返回 daemon / app / run / workflow / health 聚合状态
[ ] 不包含 daemon_id / app_id / run_id / secret / DSN
[ ] 数据库不可用时返回 200 + degraded
[ ] levier status 使用 /status
[ ] levier status --json 输出快照
[ ] 旧服务端 metrics 回退可用（迁移窗口）
[ ] public/service-status/v1/schema.json 存在
[ ] smoke 显示 status: ok
[ ] go test ./... 通过
[ ] make lint 通过
[ ] GitHub Actions CI 绿
```

---

## 11. 需要确认的决策

1. `/status` 默认公开聚合状态，不要求 Admin token
2. 不在 `/status` 中返回 daemon_id / app_id / run_id
3. 数据库不可用时 `/status` 返回 200 + `degraded`，而不是 503
4. `levier status` 在 `/status` 不存在时回退到旧 metrics
5. 对外契约使用 `public/service-status/v1/`
6. 第一阶段不实现历史趋势和单资源详情
