<!-- Levier — documentation index. -->

# 文档入口

先看：

```text
QUICKSTART.md   五分钟接入（黄金路径）
SCOPE.md        产品边界与责任
MAP.md          仓库地图
```

## 设计与边界

| 文档 | 内容 |
|---|---|
| `SCOPE.md` | 产品范围、四个边界、责任矩阵、非目标 |
| `NAMING.md` | Levier 命名、标识符地图、改名记录 |
| `ARCHITECTURE.md` | 一页架构与依赖方向 |
| `DESIGN.md` | 控制面核心设计 |
| `CONTRACT.md` | 事件、错误码、契约索引 |
| `CONFIG.md` | 配置盘点、环境变量矩阵与外部化契约 |
| `CODE_STYLE.md` | 代码组织与评审约定 |

## 接入与运行

| 文档 | 内容 |
|---|---|
| `QUICKSTART.md` | **黄金路径：安装、启动、模型、App、Run、自己的 Runtime** |
| `OPERATIONS_SURFACE.md` | **当前 CLI、SDK、HTTP/WS、环境变量逐项清单** |
| `DX.md` | 操作面设计：默认值、隐藏项、取舍与后续 |
| `ONBOARDING.md` | 从启动 Levier 到第一个 Run 的详细引导 |
| `APP_ONBOARDING.md` | App 接入流程与底层协议 |
| `RUNTIME_ONBOARDING.md` | Runtime 作者最小接入：runtime_name + run(ctx) + 可选能力 |
| `APP_ADAPTER_CONTRACT.md` | Python App adapter C0/C1 契约（handler、工具、重连、结果） |
| `DAEMON_CONTRACT.md` | React Runtime / Daemon 协议 |
| `AGENT_CORE_CONTRACT.md` | Agent Core v1 契约 |
| `MCP.md` | MCP 接入与运行 |
| `CLI_REFERENCE.md` | 当前 CLI 命令、参数、环境变量、JSON、退出码 |
| `INSTALL.md` | CLI 安装、升级、卸载、checksum、镜像/离线 |
| `MVP.md` | 单机 MVP 启动、验收、限制 |
| `LIFECYCLE.md` | 备份、恢复、Docker volume 与 secret 轮换 |
| `RUNTIME_OPS.md` | 运维运行列表、Run 取消、daemon 和任务视图 |
| `MIGRATION-1D.md` | Compose / 数据库 / 指标改名迁移 |

## 路线与待办

| 文档 | 内容 |
|---|---|
| `SIMPLIFICATION.md` | App / Runtime 接入面简化讨论稿（runtime_name、run(ctx)、凭证分层） |
| `ADAPTER_ROADMAP.md` | Adapter / Sidecar 优先级、收益、代价 |
| `CLI_DISTRIBUTION_PLAN.md` | CLI 操作面与分发安装计划 |
| `CLI_ACTIONS.md` | 动作型 CLI：app / run / migrate / daemon 设计 |
| `RELEASE.md` | release 构建、checksums、tag 与发布流程 |
| `TODO.md` | 公开前最终检查清单 + 6 配置外部化 / 7 服务自描述 / 8 状态总览 / 9 逐跳自检 / 10 生命周期 / 11 引导与凭据 / 12 运行操作面 / 13 Python App adapter |
| `SELF_DESCRIPTION_PLAN.md` | 7 服务自描述改造计划（内部开发文档） |
| `STATUS_OVERVIEW_PLAN.md` | 8 状态总览改造计划（内部开发文档） |
| `HOP_CHECK_PLAN.md` | 9 逐跳自检改造计划（内部开发文档） |
| `LIFECYCLE_PLAN.md` | 10 生命周期（进程与数据）改造计划（内部开发文档） |
| `ONBOARDING_CREDENTIALS_PLAN.md` | 11 引导与凭据管理设计与实现记录（内部开发文档） |
| `adr/` | 历史架构决策记录 |
| `api/openapi.yaml` | HTTP API 契约 |

## 阅读顺序

```text
SCOPE → MAP → 你的任务对应文档
第一次接入    → QUICKSTART.md
接 Python App → examples/counter_agent + sdk/python/README.md + APP_ADAPTER_CONTRACT.md
底层 App 协议 → APP_ONBOARDING.md
引导计划      → ONBOARDING_CREDENTIALS_PLAN.md
用 CLI        → CLI_REFERENCE.md
安装 CLI      → INSTALL.md
发布          → RELEASE.md
待办 / 配置   → TODO.md
改控制面      → DESIGN.md + ARCHITECTURE.md
改配置        → CONFIG.md
改 React Runtime    → AGENT_CORE_CONTRACT.md + execution/react_runtime/README.md
改 MCP        → MCP.md
部署          → deploy/README.md
运行操作      → RUNTIME_OPS.md
备份 / 轮换   → LIFECYCLE.md
迁移          → MIGRATION-1D.md
```

- [`PLAN_EXECUTE.md`](PLAN_EXECUTE.md) - Counter App plan/execute walkthrough.
