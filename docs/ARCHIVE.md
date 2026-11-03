# Archive 记录

## workflow_engine（DAG/Effect/Saga 引擎）

| 项目 | 内容 |
|---|---|
| 归档日期 | 2026-11-03 |
| 原路径 | `workflow_engine/` |
| 原 module | `github.com/fluffyTighnari/workflow-engine` |
| 归档 commit | `b999ac0` |
| 归档 branch | `archive/dag-effect-v0.1` |
| 归档 tag | `archive/dag-effect-v0.1` |
| 基线 Go | `go1.26.0 darwin/arm64` |
| 基线测试 | `go test ./...` 全部通过 |

**原因：** middleware v1 只需要 PostgreSQL `RunTaskQueue`（dispatch/retry/timeout/reconcile）；该引擎的 DAG 执行、Effect Journal、Saga 补偿、Async 装配、独立 HTTP server 均超出范围。新实现位于主模块 `internal/workflow`，不得 import 本模块。

**保留方式：** 目录暂时保留为只读归档；新根模块创建后不将其纳入构建路径。后续如需物理删除，可从 tag `archive/dag-effect-v0.1` 恢复。

**未纳入归档 commit 的工作区噪音：** `.DS_Store`、`.idea/*`、`.stfolder/*`（与代码无关，未提交）。
