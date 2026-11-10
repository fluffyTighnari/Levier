<!-- Middleware — retrofit P3.5 tool_defs persistence. -->

# Retrofit P3.5：确认工具真实落库

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P3.5 |
| 状态 | 完成 |
| 代码提交 | 无（仅查询验证） |
| 基准提交 | `6cbbcf4` |

## 1. 验证对象

P3.4 真实注册：

```text
app_id = app_01M72BNV0VX8A1BHP7W6CN7KE6
name   = retrofit-p3.4-1794301946
tools  = counter_add / counter_get / counter_reset
```

P3.5 直接查 PostgreSQL `tool_defs`。

## 2. SQL

```sql
SELECT tool_id, scope, app_id, name, description,
       schema_json, timeout_ms, version
FROM tool_defs
WHERE app_id = 'app_01M72BNV0VX8A1BHP7W6CN7KE6'
ORDER BY name;
```

## 3. 实际结果

```text
counter_add   | local | app_01M72BNV... | timeout_ms=10000 | version=1
counter_get   | local | app_01M72BNV... | timeout_ms=10000 | version=1
counter_reset | local | app_01M72BNV... | timeout_ms=10000 | version=1
```

schema_json：

```json
counter_add:
{"type":"object","properties":{"delta":{"type":"integer"}},"required":["delta"],"additionalProperties":false}

counter_get:
{"type":"object","properties":{},"additionalProperties":false}

counter_reset:
{"type":"object","properties":{},"additionalProperties":false}
```

## 4. 与注册 payload 对比

`schema_compare.json`：

```json
{
  "all_match": true,
  "tools": ["counter_add", "counter_get", "counter_reset"]
}
```

逐项校验：

| 字段 | 结果 |
|---|---|
| 名称 | 匹配 |
| `schema_json` 解析后与注册 `schema` 相等 | 匹配 |
| `timeout_ms` | 10000 |
| `version` | 1 |
| `scope` | local |
| `app_id` | 当前 App |
| 行数 | 3 |

## 5. 这证明了什么

```text
POST /api/v1/apps
  → handleUpdateApp
  → applyAppCatalog
  → ReplaceLocalTools
  → INSERT/UPDATE tool_defs
  → PostgreSQL 3 行
```

同时证明：

- 注册 payload 的 `schema` 被原样保存为 `schema_json`。
- `timeout_ms` 被保存。
- `version=1`。
- 工具归属当前 `app_id`。
- 不是 global 工具。

## 6. 没有证明什么

| 项目 | 步骤 |
|---|---|
| Control 组装 TaskAssign | P4 |
| Harness 看到工具 | P4 |
| WS local_tool_call | P5/P6 |
| 工具执行结果 | P6 |

## 7. 证据

```text
docs/reports/artifacts/retrofit/P3.5/
  query.sql
  db_tool_defs.txt
  db_tool_defs.tsv
  schema_compare.json
  summary.txt
```

## 8. 下一步

P4.1：启动真实 Harness，确认 TaskAssign 中的工具列表来自这些 `tool_defs` 行。

需要观察：

```text
TaskAssign.tools[].name
TaskAssign.tools[].input_schema_json
TaskAssign.tools[].scope = LOCAL
```
