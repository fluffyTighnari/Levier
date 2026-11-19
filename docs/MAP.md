<!-- Levier — repository map. -->

# 仓库地图

> 只做导航，不重复设计。最后更新：2026-11-17。

## 1. 五层

```text
控制面（产品）
  cmd/ + internal/

契约
  contracts/ + proto/ + gen/ + docs/api/

执行侧（默认组件）
  execution/agent_core/ + execution/harness/ + execution/mcp_runtime/

参考示例
  examples/

部署与验证
  deploy/ + tests/ + .github/

文档
  docs/
```

## 2. 顶层目录

`execution/` 是执行侧的父目录，下面三个单元各自独立。

| 目录 | 是什么 | 入口 | 测试 |
|---|---|---|---|
| `.github/` | CI 工作流 | `.github/workflows/ci.yml` | GitHub Actions |
| `cmd/` | Go 可执行入口 | `cmd/levier`、`cmd/migrate`、`cmd/mw-daemon-check` | `make build` |
| `internal/` | 控制面实现 | 由 `cmd/levier` 装配 | `go test ./...` |
| `contracts/` | App HTTP / WS 契约 | `contracts/app-ws.schema.json`、`docs/api/openapi.yaml` | `make contract-check` |
| `proto/` | gRPC 契约 | `proto/middleware/runtime/v1/runtime.proto` | `buf lint`、`make proto-lint` |
| `gen/` | proto 生成的 Go 代码 | `gen/middleware/runtime/v1` | 不手改，`make proto` |
| `execution/agent_core/` | 语言无关循环核心 | `levier_agent_core` | `make agent-core-test` |
| `execution/harness/` | 默认执行侧 runtime | `python -m levier_harness` | `make harness-test` |
| `execution/mcp_runtime/` | 默认 MCP 连接进程 | `python -m levier_mcp_runtime` | `make mcp-runtime-test` |
| `examples/` | 参考 App / 客户端 | `counter_agent`、`http_client`、`node_counter_agent` | `make counter-agent-test` |
| `deploy/` | 镜像、Compose、smoke | `docker-compose.yml`、`deploy/*.Dockerfile` | `docker compose config`、smoke |
| `scripts/` | 发布、安装与生命周期脚本 | `scripts/build-release.sh`、`scripts/install.sh`、`scripts/backup.sh`、`scripts/restore.sh` | `make release-dist`、`make lint`、备份/恢复演练 |
| `tests/` | 跨单元测试与检查 | `tests/contract`、`tests/e2e`、`tests/app`、`tests/checks` | 对应 make target |
| `docs/` | 设计、范围、迁移、接入 | `docs/SCOPE.md`、`docs/MAP.md` | 链接检查 |
| `bin/` | 本地构建产物 | `bin/levier` | 不提交 |
| `data/` | 本地运行输出 | 日志、证据、迁移备份 | 不提交 |

## 3. 我想改 X，去哪里

| 需求 | 位置 |
|---|---|
| HTTP 路由 / 请求校验 | `internal/httpapi/` + `docs/api/openapi.yaml` |
| Run 状态机 / 调度 / 租约 | `internal/control/` |
| 工具路由 / 幂等 / 调用记录 | `internal/gateway/` |
| 持久化 / 查询 / 迁移 | `internal/runstore/` + `internal/db/migrations/` |
| 事件模型 / 错误码 | `internal/contract/`、`internal/errs/` |
| 首次启动 / App 接入 | `docs/ONBOARDING.md` |
| App 协议细节 | `docs/APP_ONBOARDING.md` |
| CLI 使用 / 参数 / 退出码 | `docs/CLI_REFERENCE.md` |
| Release 构建 / checksums | `scripts/build-release.sh` + `docs/RELEASE.md` |
| CLI 安装 / 校验 / 镜像 | `scripts/install.sh`、`scripts/install.ps1` + `docs/INSTALL.md` |
| 公开前检查 / 下一阶段 | `docs/TODO.md` |
| 配置盘点 / 环境变量 | `docs/CONFIG.md` |
| 备份 / 恢复 / 卷 / secret 轮换 | `docs/LIFECYCLE.md` + `scripts/backup.sh`、`scripts/restore.sh` |
| 引导 / App 凭据管理 | `docs/ONBOARDING.md`、`docs/ONBOARDING_CREDENTIALS_PLAN.md` |
| Harness / Agent 循环 | `execution/harness/` |
| Agent Core 契约 | `execution/agent_core/` + `docs/AGENT_CORE_CONTRACT.md` |
| MCP 连接 / 会话 | `execution/mcp_runtime/` + `docs/MCP.md` |
| Daemon 协议自检 | `internal/daemoncheck/` + `cmd/levier/daemon.go` |
| 镜像 / Compose / smoke | `deploy/` + `docker-compose.yml` |
| CI | `.github/workflows/ci.yml` |
| 单元 / 契约 / E2E 测试 | `tests/` + 各组件 `tests/` |
| 命名 / 范围 / 迁移 | `docs/NAMING.md`、`docs/SCOPE.md`、`docs/MIGRATION-1D.md` |

## 4. 依赖方向

```text
httpapi   → register / runstore / connection / workflow
control   → register / runstore / workflow / connection / protocol
gateway   → register / connection
register  → runstore
runstore  → 无 internal 依赖
contract  → 无 internal 依赖

agent_core → 不依赖 harness / mcp_runtime / protobuf
harness    → agent_core / generated proto / mcp client
mcp_runtime → 不直连控制面
```

护栏：`make deps-check`、`make structure-check`。

## 5. 阅读顺序

```text
1. docs/SCOPE.md           产品边界
2. docs/MAP.md             仓库导航
3. docs/ONBOARDING.md      启动与首次接入
4. docs/APP_ONBOARDING.md  接 App
5. docs/DESIGN.md          控制面设计
6. docs/ARCHITECTURE.md    依赖与分层
7. deploy/README.md        部署
8. docs/LIFECYCLE.md       备份/恢复/轮换
9. docs/MIGRATION-1D.md    命名/有状态迁移
```

## 6. 结构护栏

```sh
make structure-check
```

检查：

```text
顶层目录必须存在，旧顶层目录不能复活
每个顶层单元必须有 README.md
internal / cmd / execution 不能引用 examples/
agent_core 不能 import harness / MCP / protobuf
cmd/* 只放 package main
gen/*.go 必须标记为生成代码
operational 文件不能引用搬迁前的旧路径
```

Go 包依赖方向由 `make deps-check` 负责；两者都在 CI 中执行。

## 7. 维护规则

```text
目录职责变化时更新本页
本页只放导航和入口，不展开实现
每节优先链接到唯一事实来源
新增顶层目录必须同时更新本页
```
