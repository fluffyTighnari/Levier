<!-- Middleware — retrofit P4.6 tool pruning. -->

# Retrofit P4.6：删除工具后 TaskAssign 更新

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P4.6 |
| 状态 | 完成，P4 收口 |
| 代码提交 | 无（仅验证；使用已有 `--tools-file`） |
| 基准提交 | `61807bb` |

## 1. 测试流程

```text
App 注册 3 个工具
  → 提交 Run1
  → TaskAssign 应有 3 个

App 重新注册，只保留 counter_add
  → tool_defs 删除 get/reset
  → 提交 Run2
  → TaskAssign 只应有 counter_add
```

## 2. 注册变化

第一次：

```json
["counter_add", "counter_get", "counter_reset"]
```

第二次：

```json
["counter_add"]
```

`ReplaceLocalTools` 语义：

```text
DELETE 不在列表中的 local tools
UPSERT 列表中的工具
version++
```

## 3. TaskAssign 结果

```json
{
  "before_run": {
    "run_id": "01M72DH7Z53XYRFQZEV5EJZKZP",
    "tools": ["counter_add", "counter_get", "counter_reset"]
  },
  "after_run": {
    "run_id": "01M72DH85GTG7WWW8PFTV8QS30",
    "tools": ["counter_add"]
  },
  "before_ok": true,
  "after_ok": true,
  "pruned": true
}
```

## 4. DB 结果

```text
name          scope  version  timeout_ms
counter_add   local  2        10000
```

- `counter_get`、`counter_reset` 已删除。
- `counter_add` 仍在，`version` 从 1 变为 2。
- 没有残留行。

## 5. 两个 Run

```text
01M72DH7Z53XYRFQZEV5EJZKZP | App1 | echo | completed
01M72DH85GTG7WWW8PFTV8QS30 | App1 | echo | completed
```

## 6. 这证明了什么

| 项目 | 结果 |
|---|---|
| `tools` 全量替换 | 是 |
| 删除的工具从 DB 移除 | 是 |
| 新 Run 的 TaskAssign 不含已删除工具 | 是 |
| 保留工具 version 增加 | 是（2） |
| 旧 Run 不受影响 | 是 |

## 7. P4 收口

| 步骤 | 状态 |
|---|---|
| P4.1 TaskAssign 到达真实 Harness | 完成 |
| P4.2 TaskAssign 工具日志 | 完成 |
| P4.3 TaskAssign 包含注册工具 | 完成 |
| P4.4 schema 与 DB 精确一致 | 完成 |
| P4.5 App 间工具隔离 | 完成 |
| P4.6 删除后 TaskAssign 更新 | 完成 |

P4 结论：

```text
App 注册工具
  → tool_defs
  → Control 按 app_id 组装
  → TaskAssign
  → 真实 Harness
```

已逐节点验证。

## 8. 证据

```text
docs/reports/artifacts/retrofit/P4.6/
  bootstrap.json
  register_three.json
  counter_add_only.json
  register_one.json
  submit_before.json
  submit_after.json
  harness.log
  prune.json
  db_tools.txt
  db_runs.txt
  middleware.log
  migrate.log
  summary.txt
```

## 9. 下一步

P5.1：App key 换 ws_token。

```text
POST /api/v1/ws-token
Authorization: Bearer <app_api_key>
→ 200
→ ws_token / expires_in
```

这是进入真实 WS 工具宿主前的第一个节点。
