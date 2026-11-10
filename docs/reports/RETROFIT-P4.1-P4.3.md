<!-- Middleware — retrofit P4.1–P4.3 TaskAssign tool visibility. -->

# Retrofit P4.1–P4.3：TaskAssign 工具可见

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P4.1–P4.3 |
| 状态 | 完成 |
| 代码提交 | `e43f83c`、`b130d29` |

## 1. 真实链路

```text
bootstrap_app.py
  → POST /admin/apps
register_tools.py
  → POST /api/v1/apps
  → tool_defs 三行

submit_run.py
  → POST /api/v1/runs

MiddleWare Control
  → buildTaskAssign
  → TaskAssign.tools

真实 Harness
  → 收到 TaskAssign
  → 打印工具列表
```

## 2. 真实运行结果

App：

```text
app_id = app_01M72CD43AXKP68KQ0G5N3FZWT
```

Run：

```text
run_id = 01M72CD4A5D508M14ZVF4D6PTS
status = completed
trace_id = 01M72CD4A47SPPKZGVCBE322WZ
```

Harness 日志：

```json
{
  "message": "task assign received",
  "run_id": "01M72CD4A5D508M14ZVF4D6PTS",
  "attempt": 1,
  "lease_epoch": 1,
  "strategy": "echo",
  "app_id": "app_01M72CD43AXKP68KQ0G5N3FZWT",
  "trace_context": "01M72CD4A47SPPKZGVCBE322WZ",
  "tool_count": 3,
  "tools": [
    {"name": "counter_add", "scope": "LOCAL", "schema_len": 109},
    {"name": "counter_get", "scope": "LOCAL", "schema_len": 62},
    {"name": "counter_reset", "scope": "LOCAL", "schema_len": 62}
  ]
}
```

## 3. 这证明了什么

```text
tool_defs
  → Control.AssembleTaskMaterials
  → toolManifests
  → TaskAssign.tools
  → 真实 Harness
```

逐项：

| 项目 | 结果 |
|---|---|
| TaskAssign 到达真实 Harness | 是 |
| 工具数量 | 3 |
| 工具名 | counter_add/get/reset |
| scope | LOCAL |
| input_schema_json 非空 | 是（schema_len 109/62/62） |
| trace_context 到达 Harness | 是 |
| Run 完成 | 是 |

## 4. 新增 Harness 日志字段

`harness/src/middleware_harness/runtime/manager.py`：

```python
logger.info("task assign received", extra={
    "run_id": ...,
    "app_id": ...,
    "strategy": ...,
    "attempt": ...,
    "lease_epoch": ...,
    "trace_context": ...,
    "tool_count": ...,
    "tools": [{"name", "scope", "schema_len"}],
})
```

`logging.py` 白名单补充：

```text
app_id
trace_context
tool_count
tools
session_id
```

作用：

- 能证明工具真的进入 TaskAssign。
- trace_context 不再“收到即丢”。
- 为后续工具调用定位提供 run_id/trace。

## 5. 设计依据

```text
DESIGN §5.3 工具调用三分支
proto TaskAssign / ToolManifest
internal/control/control.go buildTaskAssign
internal/control/control.go toolManifests
harness/src/middleware_harness/runtime/manager.py
```

## 6. 未完成

| 项目 | 状态 |
|---|---|
| 精确比较 TaskAssign schema 与 DB schema | P4.4 |
| 第二个 App 只看到自己的工具 | P4.5 |
| 工具执行 | P6 |
| WS local_tool_call | P5/P6 |

## 7. 证据

```text
docs/reports/artifacts/retrofit/P4.1/
  bootstrap.json
  register_tools.json
  submit_run.json
  harness.log
  middleware.log
  db_run.txt
  migrate.log
  summary.txt
```

关键行：

```text
harness.log       task assign received + 3 tools + trace_context
db_run.txt        run status=completed
middleware.log    control assigned run
```

## 8. 下一步

P4.4：精确比较：

```text
TaskAssign.tools[].input_schema_json
  == tool_defs.schema_json
```

需要把 Harness 日志中的 schema 摘要升级为可核对内容，或在诊断探针中记录完整 schema。
