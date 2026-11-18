<!-- Levier — documentation index. -->

# 文档入口

先看：

```text
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
| `APP_ONBOARDING.md` | App 接入流程与协议 |
| `DAEMON_CONTRACT.md` | Harness / Daemon 协议 |
| `AGENT_CORE_CONTRACT.md` | Agent Core v1 契约 |
| `MCP.md` | MCP 接入与运行 |
| `CLI_REFERENCE.md` | 当前 CLI 命令、参数、环境变量、JSON、退出码 |
| `INSTALL.md` | CLI 安装、升级、卸载、checksum、镜像/离线 |
| `MVP.md` | 单机 MVP 启动、验收、限制 |
| `MIGRATION-1D.md` | Compose / 数据库 / 指标改名迁移 |

## 路线与待办

| 文档 | 内容 |
|---|---|
| `ADAPTER_ROADMAP.md` | Adapter / Sidecar 优先级、收益、代价 |
| `CLI_DISTRIBUTION_PLAN.md` | CLI 操作面与分发安装计划 |
| `CLI_ACTIONS.md` | 动作型 CLI：app / run / migrate / daemon 设计 |
| `RELEASE.md` | release 构建、checksums、tag 与发布流程 |
| `TODO.md` | 公开前最终检查清单 + 6 配置外部化计划 |
| `adr/` | 历史架构决策记录 |
| `api/openapi.yaml` | HTTP API 契约 |

## 阅读顺序

```text
SCOPE → MAP → 你的任务对应文档
接 App        → APP_ONBOARDING.md
用 CLI        → CLI_REFERENCE.md
安装 CLI      → INSTALL.md
发布          → RELEASE.md
待办 / 配置   → TODO.md
改控制面      → DESIGN.md + ARCHITECTURE.md
改配置        → CONFIG.md
改 Harness    → AGENT_CORE_CONTRACT.md + execution/harness/README.md
改 MCP        → MCP.md
部署          → deploy/README.md
迁移          → MIGRATION-1D.md
```
