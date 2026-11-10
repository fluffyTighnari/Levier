<!-- Middleware — registration contract verification. -->

# Middleware 注册格式验证（REG-01）

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 状态 | 已验证 |
| 范围 | App 注册、local tool 注册、DB 落库、TaskAssign |
| 证据 | `docs/reports/artifacts/retrofit/REG-01/` |

## 1. 结论先说

Middleware **不接收 App 的代码**，只接收两类元数据：

1. App 身份：`app_id` + API key hash。
2. App 的 local tool 定义：名称、描述、JSON Schema、超时。

你的项目仍然是独立进程，通过：

```text
HTTP  → 注册、提交 Run、查状态/事件
WS    → 接收 local_tool_call、返回 local_tool_result
```

接入不是“上传代码”，而是：

```text
保留函数
+ 写工具元数据
+ HTTP 注册
+ WS 工具宿主
- 删除本地 ReAct/模型循环（交给 Harness）
```

## 2. 注册流程

### 2.1 创建 App

```http
POST /admin/apps
Authorization: Bearer <admin_token>
```

```json
{"name": "counter-app", "multiagent": false}
```

返回 `app_id` 和一次性 `api_key`。

落库到 `apps`：

```text
app_id
name
multiagent
status
api_key_hash   ← SHA-256，不是明文
```

### 2.2 注册 local tools

```http
POST /api/v1/apps
Authorization: Bearer <app_api_key>
```

```json
{
  "name": "counter-app",
  "multiagent": false,
  "tools": [
    {
      "name": "counter_add",
      "description": "Add an integer delta to the counter.",
      "schema": {
        "type": "object",
        "properties": {"delta": {"type": "integer"}},
        "required": ["delta"],
        "additionalProperties": false
      },
      "timeout_ms": 10000
    },
    {
      "name": "counter_get",
      "description": "Read the current counter value.",
      "schema": {
        "type": "object",
        "properties": {},
        "additionalProperties": false
      },
      "timeout_ms": 10000
    }
  ]
}
```

规则：

- `tools` 是**全量替换**；新增工具必须一起提交现有工具。
- `"tools": []` 清空。
- 不传 `tools` 不改工具。
- 同名工具重写 `version++`。
- `timeout_ms` 默认 60000。
- `schema` 为空时 HTTP 层会补 `{}`。
- 当前 `GET /apps/{id}` **不返回工具列表**，验证工具只能查 DB 或看 TaskAssign。

落库到 `tool_defs`：

```text
tool_id
scope = 'local'
app_id
name
description
schema_json       ← JSON 字符串
timeout_ms
version
```

### 2.3 Harness 收到的工具形态

TaskAssign：

```json
{
  "tools": [
    {
      "name": "counter_add",
      "description": "Add an integer delta to the counter.",
      "input_schema_json": "{\"type\":\"object\",...}",
      "scope": "LOCAL"
    }
  ]
}
```

注意字段名变化：

```text
注册：schema            → TaskAssign：input_schema_json
注册：tools[]           → TaskAssign：tools[]
注册：timeout_ms        → TaskAssign 不直接携带 per-tool timeout
```

### 2.4 App 收到的工具调用

WS：

```text
GET /api/v1/ws?app_id=<app_id>&token=<ws_token>
```

```json
{
  "type": "local_tool_call",
  "msg_id": "01...",
  "invocation_id": "01...",
  "tool_name": "counter_add",
  "arguments_json": "{\"delta\":2}",
  "timeout_ms": 10000
}
```

App 返回：

```json
{
  "type": "local_tool_result",
  "correlation_id": "01...",
  "invocation_id": "01...",
  "ok": true,
  "result_json": "{\"value\":2}"
}
```

错误：

```json
{
  "type": "local_tool_result",
  "correlation_id": "01...",
  "invocation_id": "01...",
  "ok": false,
  "error": {
    "code": "TOOL_EXEC_ERROR",
    "message": "delta must be an integer",
    "retryable": false
  }
}
```

约束：

- `correlation_id == msg_id`。
- `invocation_id` 是幂等键。
- `arguments_json` / `result_json` 是字符串内的 JSON。
- 错误码使用 DESIGN §4.5 集合；App 不伪造 `TOOL_TIMEOUT/TOOL_DENIED/APP_UNREACHABLE`。

## 3. 本次实测结果

使用一次性诊断探针：

```text
docs/reports/artifacts/retrofit/REG-01/observe_task_assign.py
```

探针只：

- 创建 App。
- 注册两个工具。
- 以 daemon-local-1 连接 RuntimeChannel。
- 创建 echo Run。
- 打印 TaskAssign 后退出。

它不是业务代码，不是 fakedaemon，也不替代真实 Harness E2E。

### 3.1 HTTP 响应

```text
app_id = app_01M71NKR6DPKZVWSAGF1N455RX
app_name = retrofit-reg-8aa2db039d
GET /apps/{id} = 200，status=active
```

### 3.2 DB

`apps`：

```text
status=active
hash_len=64
hash_prefix=0b730ac8
```

`tool_defs`：

```text
counter_add | local | schema_json=... | timeout_ms=10000 | version=1
counter_get | local | schema_json=... | timeout_ms=10000 | version=1
```

### 3.3 TaskAssign

```json
{
  "app_id": "app_01M71NKR6DPKZVWSAGF1N455RX",
  "strategy": "echo",
  "tool_count": 2,
  "tools": [
    {
      "name": "counter_add",
      "input_schema_json": "{\"type\": \"object\", ...}",
      "scope": "LOCAL"
    },
    {
      "name": "counter_get",
      "input_schema_json": "{\"type\": \"object\", ...}",
      "scope": "LOCAL"
    }
  ]
}
```

这说明：

```text
App HTTP 注册
  → apps/tool_defs 落库
  → Control 组装 TaskAssign
  → Harness 看到工具
```

## 4. 已完成的验证边界

| 验证 | 状态 |
|---|---|
| App 创建 | ✅ |
| API key hash 落库 | ✅ |
| App key 读取自身 | ✅ |
| local tool 注册 | ✅ |
| `tool_defs` 落库 | ✅ |
| TaskAssign 包含工具 | ✅ |
| 工具 schema 字段转换 | ✅ |
| App WS 工具调用 | 未验证（P5/P6） |
| 真实 Harness ReAct | 未验证（P6） |
| 工具执行结果 | 未验证（P6） |
| 工具幂等 | 未验证（P6） |

## 5. 对原项目的改造映射

以 `examples/counter_agent/` 为例：

| 原项目 | 改造 |
|---|---|
| `counter_add/get/reset` 函数 | 保留 |
| 函数参数 | 写进 `schema.properties` |
| 函数返回 | 包成 `{"value": n}` → `result_json` |
| 本地函数调用 | 改成 WS `local_tool_call` 分发 |
| 本地 ReAct 循环 | 删除，交给 Harness |
| 本地 DeepSeek 调用 | 删除；Harness 调 DeepSeek |
| `main.py` | App 进程：HTTP 注册 + WS 工具宿主 |
| 最终输出 | 通过 Run events / WS 观察 |

## 6. 证据

```text
docs/reports/artifacts/retrofit/REG-01/
  registration_payload.json
  task_assign.json
  db_apps.txt
  db_tool_defs.txt
  middleware.log
  migrate.log
  observer.err
  summary.txt
  observe_task_assign.py
```

没有保存 API key、ws_token、DB 密码。

## 7. 下一步

现在注册格式已经明确。建议下一步只做一个小节：

**P1.1（Python 版）：在原 `examples/counter_agent/main.py` 中抽出计数器函数和工具描述，不联网。**

然后再分步：

- 加 HTTP 注册。
- 查 DB。
- 加 WS 工具宿主。
- 对真实 Harness。
