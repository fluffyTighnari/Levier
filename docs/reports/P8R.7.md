<!-- Middleware — P8R.7 architecture doc and function review. -->

# P8R.7：架构文档与长函数评审

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-12 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P8R.7 |
| 状态 | 完成 |
| 代码提交 | 见下 |

## 1. 架构文档

新增 `docs/ARCHITECTURE.md`，一页回答：

```text
角色
包分层与依赖方向
一次 Run 的路径
local / global / MCP 三条工具路径
新东西加在哪
明确不做的事
长函数评审结果
```

不重复 DESIGN、APP_ONBOARDING、DAEMON_CONTRACT、MCP 的细节，只做索引与地图。

## 2. 长函数评审

按要求**不为行数拆函数**。评审结果：

| 函数 | 行数 | 保留原因 |
|---|---:|---|
| `main` | 261 | 启动与清理顺序必须在一处可见 |
| `handleEvent` | 167 | 事件处理顺序敏感，拆开隐藏时序 |
| `Handle` | 146 | 分配重试循环是一个完整状态机 |
| `Stats` | 121 | SQL 聚合字段与扫描顺序强耦合 |
| `handleWS` | 109 | 一条连接的消息 switch |
| `handleCreateRun` | 99 | 校验、落库、入队是一段事务性流程 |
| `Validate` | 82 | 配置校验按字段顺序阅读更直接 |

原则写进架构文档：

```text
只在重复逻辑或深层嵌套时抽取
提取后必须有清晰名字和独立契约
不为满足行数指标拆
```

## 3. 规模报告改为建议

`scripts/size_report.sh` 开头明确：

```text
size report is advisory: split by responsibility, not by line count
```

文件 > 450 行仍然是拆分候选；
函数 > 80 行只提示，不要求处理。

## 4. 验证

```text
go test ./...          全部通过
make lint              OK
make build             OK
dependency check       OK (9 packages with rules, 17 packages scanned)
size report            建议性输出
```

## 5. 证据

```text
docs/reports/artifacts/P8R.7/
  go_test.log
  lint.log
  build.log
  deps.txt
  size.txt
```

## 6. 下一步

P8R.8：回归与评估。

```text
全量测试
clean checkout 测试
retrofit-e2e
mcp-e2e
daemon-check
依赖与规模报告
```
