<!-- Middleware — retrofit P4.5 cross-app TaskAssign isolation. -->

# Retrofit P4.5：App 间工具隔离

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P4.5 |
| 状态 | 完成 |
| 代码提交 | `4521393` |
| 范围 | 两个真实 App，各自注册不同工具，真实 Harness 对比 TaskAssign |

## 1. 测试设置

### App1

```text
app_id = app_01M72D6KTZQDDY3T1G4E120NE1
tools  = counter_add / counter_get / counter_reset
```

### App2

```text
app_id = app_01M72D6M1JF0X60QGYV05JH84S
tools  = other_ping
```

App2 通过：

```sh
register_tools.py --tools-file app2_tools.json
```

注册自定义工具列表。

## 2. 真实 TaskAssign

Harness 日志按 app_id 聚合：

```json
{
  "app1": {
    "app_id": "app_01M72D6KTZQDDY3T1G4E120NE1",
    "tools": ["counter_add", "counter_get", "counter_reset"]
  },
  "app2": {
    "app_id": "app_01M72D6M1JF0X60QGYV05JH84S",
    "tools": ["other_ping"]
  },
  "app1_ok": true,
  "app2_ok": true,
  "app2_has_app1_tool": false,
  "isolated": true
}
```

## 3. DB `tool_defs`

```text
App1:
  counter_add   local
  counter_get   local
  counter_reset local

App2:
  other_ping    local
```

共 4 行，没有交叉。

## 4. 两个 Run

```text
01M72D6M82MJQ942JQFD9DXDJW | App1 | echo | completed
01M72D6MBCHH48E2Y1F65YV0WX | App2 | echo | completed
```

## 5. 证明链路

```text
Control.AssembleTaskMaterials(app_id)
  → tool_defs WHERE app_id = 当前 App
  → toolManifests
  → TaskAssign.tools
  → 真实 Harness
```

结果：

- App1 只看到自己的 3 个工具。
- App2 只看到自己的 1 个工具。
- App2 的 TaskAssign 不含任何 `counter_*`。
- 两个 Run 都完成。

## 6. 这证明了什么

| 项目 | 结果 |
|---|---|
| `AssembleTaskMaterials` 按 app_id 过滤 | 是 |
| 一个 App 的 local tools 不泄露给另一个 App | 是 |
| 同名/不同名工具都按 App 隔离 | 是 |
| 自定义工具列表可注册 | 是（`--tools-file`） |

## 7. 证据

```text
docs/reports/artifacts/retrofit/P4.5/
  bootstrap_app1.json
  register_app1.json
  bootstrap_app2.json
  register_app2.json
  app2_tools.json
  submit_app1.json
  submit_app2.json
  harness.log
  isolation.json
  db_tools.txt
  db_runs.txt
  middleware.log
  migrate.log
  summary.txt
```

## 8. 下一步

P4.6：删除工具后重新提交 Run，确认 TaskAssign 不再包含已删除的工具。

```text
App1 注册 counter_add/get/reset
→ 再注册只保留 counter_add
→ 新 Run 的 TaskAssign 只含 counter_add
```
