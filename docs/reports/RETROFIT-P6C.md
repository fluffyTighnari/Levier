<!-- Middleware — retrofit P6C baseline comparison. -->

# Retrofit P6C：与基线对照（P6.9–P6.11）

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-11 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P6C |
| 状态 | 完成 |
| 代码提交 | 无（纯对照） |
| 基准提交 | `5685fd2` |

## 1. 对照什么

P0.1 冻结的基线（commit `75b93bd`）：

```text
prompt: Add 2, then add 3, then read the counter and tell me the final value.
        Use the counter tools; do not calculate it yourself.

counter_add({"delta": 2}) -> {"value": 2}
counter_add({"delta": 3}) -> {"value": 5}
counter_get({})           -> {"value": 5}
final counter = 5
tool calls    = 3
order         = add, add, get
steps         = 4
```

P6C 要回答的是：

```text
P6.9  最终计数器是否仍为 5
P6.10 工具调用次数是否仍为 3
P6.11 调用顺序是否仍是 add, add, get
```

## 2. 同日基线复跑

先排除“模型行为漂移”这个变量，用同一条提示词重跑裸 App：

```text
[model] deepseek-flash
[user] Add 2, then add 3, then read the counter and tell me the final value...
[tool] counter_add({"delta": 2}) -> {"value": 2}
[tool] counter_add({"delta": 3}) -> {"value": 5}
[tool] counter_get({}) -> {"value": 5}
[assistant] The final counter value is **5**.
[final] counter=5 steps=4
```

与 P0.1 记录的 3 次调用、final=5 完全一致。

## 3. 经 Middleware 的真跑

```text
app_id = app_01M74BHTKXBA38XDTHZ14Y14VE
run_id = 01M74BHTYSGSBTVQBZXCZJAJWK
strategy = react / deepseek-flash
```

宿主执行日志：

```text
counter_add {"delta": 2} -> {"value": 2}
counter_add {"delta": 3} -> {"value": 5}
counter_get {}           -> {"value": 5}
closed counter=5 handled=3
```

`run_events` 顺序：

```text
seq 3  MESSAGE_COMPLETED  "I'll add 2, then 3, then read the result."
seq 4  TOOL_CALL_BEGIN    counter_add {"delta": 2}
seq 5  TOOL_CALL_END      {"value": 2}
seq 8  TOOL_CALL_BEGIN    counter_add {"delta": 3}
seq 9  TOOL_CALL_END      {"value": 5}
seq 12 TOOL_CALL_BEGIN    counter_get {}
seq 13 TOOL_CALL_END      {"value": 5}
seq 15 MESSAGE_COMPLETED  "… The final counter value is **5**."
seq 16 RUN_STATUS         completed
```

`tool_invocations`：

```text
counter_add | completed | {"value": 2}
counter_add | completed | {"value": 5}
counter_get | completed | {"value": 5}
```

## 4. 对照结果

| 项 | 基线 P0.1 | 经 Middleware | 判定 |
|---|---|---|---|
| final counter | 5 | 5 | P6.9 通过 |
| tool call 次数 | 3 | 3 | P6.10 通过 |
| 调用顺序 | add, add, get | add, add, get | P6.11 通过 |
| 参数 | 2 / 3 / — | 2 / 3 / — | 一致 |
| 中间结果 | 2 / 5 / 5 | 2 / 5 / 5 | 一致 |
| 模型步数 | 4 | 4（4 条 MESSAGE_COMPLETED） | 一致 |
| 终态 | 进程退出 | `RUN_STATUS completed` | 语义等价 |

## 5. 这说明了什么

1. Middleware 没有改变工具的**语义**：同样的输入产生同样的状态变化。
2. 没有引入**额外调用**：3 次调用，不多不少。
3. 没有改变**调用顺序**：add → add → get。
4. 模型看到的结果与基线一致，所以最终回答都是 5。
5. 差异只在承载方式：进程内直接调用 vs `local_tool_call/local_tool_result`。

## 6. 局限

- 单次样本；模型有随机性，但本轮同日复跑与基线完全同构。
- 只对照了成功路径；失败/重试路径属 P6D。
- 未对照流式输出与多轮会话。

## 7. 证据

```text
docs/reports/artifacts/retrofit/P6C/
  baseline_recheck.log      同日裸 App 基线复跑
  tool_host.log             经 Middleware 的 3 次执行
  db_run.txt                run 状态 + 事件 + invocation 行
  submit_run.json
  bootstrap.json
  register_tools.json
  middleware.log
  harness.log
  migrate.log
  summary.txt
```

基线原件：

```text
docs/reports/artifacts/retrofit/P0.1/counter.log
docs/reports/artifacts/retrofit/P0.1/summary.txt
```

## 8. 下一步

P6D：负向与故障。

```text
P6.12 重复 local_tool_result
P6.13 correlation 错误
P6.14 未知 invocation
P6.15 其他 App 伪造
P6.16 App 断线
P6.17 工具超时
```
