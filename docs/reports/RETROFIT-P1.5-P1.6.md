<!-- Middleware — retrofit P1.5/P1.6 dispatcher errors. -->

# Retrofit P1.5–P1.6：dispatcher 异常处理

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P1.5–P1.6 |
| 状态 | 完成 |
| 代码提交 | `92bd589` |
| 范围 | 未知工具、非法参数；未接 main/HTTP/WS |

## 1. 错误映射

| 情况 | code | retryable |
|---|---|---|
| 未知工具 | `TOOL_NOT_FOUND` | false |
| JSON 语法错误 | `TOOL_EXEC_ERROR` | false |
| 参数不是 JSON 对象 | `TOOL_EXEC_ERROR` | false |
| 缺少 `delta` | `TOOL_EXEC_ERROR` | false |
| `delta` 不是整数 | `TOOL_EXEC_ERROR` | false |
| `arguments_json` 不是字符串 | `TOOL_EXEC_ERROR` | false |

依据：

```text
docs/DESIGN.md §4.5 工具错误模型
proto/.../runtime.proto ErrorInfo
```

## 2. 代码调整

`dispatch.py`：

- 非字符串 `arguments_json` 明确返回 `TOOL_EXEC_ERROR`，不再抛 `AttributeError`。
- 缺少 `delta` 使用明确消息：

```text
counter_add.delta is required
```

- 错误结构统一：

```json
{
  "ok": false,
  "error": {
    "code": "TOOL_EXEC_ERROR",
    "message": "...",
    "retryable": false
  }
}
```

## 3. P1.5 验证

```sh
python3 -m unittest -v test_dispatch.DispatcherTest.test_p1_5_unknown_tool
```

```text
test_p1_5_unknown_tool ... ok
Ran 1 test
OK
```

断言：

```text
tool = counter_missing
ok = false
error.code = TOOL_NOT_FOUND
error.retryable = false
counter 不变
```

## 4. P1.6 验证

```sh
python3 -m unittest -v test_dispatch.DispatcherTest.test_p1_6_invalid_arguments
```

```text
test_p1_6_invalid_arguments ... ok
Ran 1 test
OK
```

子用例：

| 子用例 | 输入 |
|---|---|
| malformed json | `{` |
| array arguments | `[]` |
| missing delta | `{}` |
| string delta | `{"delta": "2"}` |
| bool delta | `{"delta": true}` |
| non-string arguments | `123` |

每个子用例都断言：

```text
ok = false
code = TOOL_EXEC_ERROR
retryable = false
counter 不变
```

## 5. 全量本地测试

```sh
make counter-agent-test
```

```text
tests 8
OK
```

## 6. 证据

```text
docs/reports/artifacts/retrofit/P1.5/test.log
docs/reports/artifacts/retrofit/P1.5/summary.txt
docs/reports/artifacts/retrofit/P1.6/test.log
docs/reports/artifacts/retrofit/P1.6/summary.txt
```

## 7. P1 当前进度

| 步骤 | 状态 |
|---|---|
| P1.1 Counter + 注册元数据 | 完成 |
| P1.2 dispatcher add | 完成 |
| P1.3 dispatcher get | 完成 |
| P1.4 dispatcher reset | 完成 |
| P1.5 unknown tool | 完成 |
| P1.6 invalid args | 完成 |
| P1.7 manifest 双形态 | 待做 |
| P1.8 main 接 dispatcher + 基线 | 待做 |

## 8. 下一步

P1.7：只验证 manifest 双形态：

```text
POST /api/v1/apps 用 schema 对象
TaskAssign 用 input_schema_json 字符串
```
