<!-- Middleware — retrofit P4.4 exact TaskAssign schema comparison. -->

# Retrofit P4.4：TaskAssign schema 与 DB 精确一致

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P4.4 |
| 状态 | 完成 |
| 代码提交 | `2155135` |
| 范围 | 真实 Harness TaskAssign 与 `tool_defs` 精确比较 |

## 1. 验证方式

真实 Harness 现在记录完整 schema：

```text
TaskAssign.tools[].input_schema_json
```

证据：

- `harness.log`
- `task_assign.json`（从日志提取的 TaskAssign）
- `db_schema.tsv`（`tool_defs.schema_json`）
- `schema_compare.json`（逐项精确比较）

## 2. 实际 TaskAssign

```json
{
  "run_id": "01M72D3J6SBFK9NVH6PRW4R920",
  "app_id": "app_01M72D3HZVNZN9E22A5KSFBE1B",
  "strategy": "echo",
  "trace_context": "01M72D3J6RACKP0YMC4R72GZ5P",
  "tool_count": 3,
  "tools": [
    {
      "name": "counter_add",
      "scope": "LOCAL",
      "input_schema_json": "{\"type\":\"object\",\"properties\":{\"delta\":{\"type\":\"integer\"}},\"required\":[\"delta\"],\"additionalProperties\":false}"
    },
    {
      "name": "counter_get",
      "scope": "LOCAL",
      "input_schema_json": "{\"type\":\"object\",\"properties\":{},\"additionalProperties\":false}"
    },
    {
      "name": "counter_reset",
      "scope": "LOCAL",
      "input_schema_json": "{\"type\":\"object\",\"properties\":{},\"additionalProperties\":false}"
    }
  ]
}
```

## 3. 精确比较结果

```json
{
  "all_exact": true,
  "tools": [
    {"name": "counter_add", "exact_match": true, "db_len": 109, "assign_len": 109},
    {"name": "counter_get", "exact_match": true, "db_len": 62, "assign_len": 62},
    {"name": "counter_reset", "exact_match": true, "db_len": 62, "assign_len": 62}
  ]
}
```

比较的是字符串完全相等，不是只比较长度或解析后的对象。

## 4. 证明链路

```text
tool_defs.schema_json
  → register.ToolMaterial.SchemaJSON
  → control.toolManifests
  → ToolManifest.InputSchemaJson
  → TaskAssign.tools[].input_schema_json
  → 真实 Harness 日志
```

说明 Middleware 没有重新序列化 schema，而是把注册时的 JSON 字符串原样传给 Harness。

## 5. 运行状态

```text
run_id = 01M72D3J6SBFK9NVH6PRW4R920
status = completed
attempt = 1
trace_id = 01M72D3J6RACKP0YMC4R72GZ5P
```

## 6. 这证明了什么

| 项目 | 结果 |
|---|---|
| TaskAssign 工具来自 tool_defs | 是 |
| input_schema_json 与 DB schema_json 完全一致 | 是 |
| 3 个工具全部一致 | 是 |
| schema 未被重新格式化 | 是 |
| Run 正常完成 | 是 |

## 7. 证据

```text
docs/reports/artifacts/retrofit/P4.4/
  bootstrap.json
  register_tools.json
  submit_run.json
  harness.log
  task_assign.json
  db_schema.tsv
  schema_compare.json
  db_run.txt
  middleware.log
  migrate.log
  summary.txt
```

## 8. 下一步

P4.5：第二个 App 创建 Run，确认 TaskAssign 只包含它自己的工具，不包含第一个 App 的 local tools。
