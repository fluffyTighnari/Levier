<!-- Middleware — retrofit P6D negative and fault scenarios. -->

# Retrofit P6D：负向与故障（P6.12–P6.17）

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-11 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P6D |
| 状态 | 完成 |
| 代码提交 | 无（新增故障探针与证据） |
| 基准提交 | `6658a0e` |

## 1. 方法

正常宿主不会产生这些路径，所以用一个可切换故障模式的探针：
`docs/reports/artifacts/retrofit/P6D/fault_host.py`。

它用真实凭据、真实 WS、真实工具执行，只在回包环节故意做错：

```text
duplicate   同一结果发两次
mismatch    correlation_id 用错的，等服务端报错后再发正确的
unknown     先发一个不存在的 invocation_id，再发正确的
foreign     通过另一个 App 的连接发送本 App 的 invocation 结果
disconnect  收到调用后直接断开，不回包
slow        睡过 tool_ms 再回包
```

每个场景都是一次真实 Run（DeepSeek + Harness + MW + WS），
证据包含宿主日志、`run_events`、`tool_invocations`。

## 2. P6.12 重复 local_tool_result

```text
run_id = 01M74CJKFCRYQ6HR6YXJ1WMDHC
mode   = duplicate
```

宿主：

```text
sent             {"value": 1} correlation=01M74CJQN4DABMTV466J4CC7VN
sent_duplicate   invocation=01M74CJQMSA9E7JCY9X2858XVM
```

服务端结果：

```text
TOOL_CALL_BEGIN  01M74CJQMSA9E7JCY9X2858XVM
TOOL_CALL_END    ok=true {"value": 1} duration_ms=15
tool_invocations 1 行 completed
run              completed
```

**结论**：重复结果被静默丢弃，没有第二次投递、没有第二条事件、没有覆盖结果。

## 3. P6.13 correlation_id 错误

```text
run_id = 01M74CK1RPG4RQGS9V14CZNQ8P
mode   = mismatch
```

宿主：

```text
sent_bad_correlation  bogus-01M74CK2B6A8XG1JP8JQWJ8WRW
server_error          LOCAL_TOOL_CORRELATION_MISMATCH
sent_correct          correlation=01M74CK2B6A8XG1JP8JQWJ8WRW
```

服务端：

```text
TOOL_CALL_END ok=true {"value": 2}
invocation completed
run completed
```

**结论**：错误 correlation 被拒绝，且 **pending 保留**——原 `msg_id` 重发即可完成。

## 4. P6.14 未知 invocation

```text
run_id = 01M74CKJVG3852Q9QQQG5KS4M9
mode   = unknown
```

宿主：

```text
sent_unknown_invocation  unknown-01M74CKKMHEKWMPSFRTZX4Q9NS
server_error             LOCAL_TOOL_UNKNOWN
sent_correct             01M74CKKMHEKWMPSFRTZX4Q9NS
```

服务端：

```text
TOOL_CALL_END ok=true {"value": 3}
invocation completed
```

**结论**：不存在的 invocation 被拒绝；真实调用不受影响。

## 5. P6.15 其他 App 伪造

```text
run_id          = 01M74CM3SEQR2NNX2TMT9NC3EA
owner app       = app_01M74CJ5QZ2JR3ZF033BC3P9CH
foreign app     = app_01M74CJ5YPPJ3Y72ABRXNA7V3P
mode            = foreign
```

宿主用 foreign App 的连接发送 owner 的 invocation：

```text
sent_via_foreign_app   app_01M74CJ5YPPJ3Y72ABRXNA7V3P
forged_invocation_id   01M74CM4S7Y7QRXPPTDM1BW8CA
server_error           LOCAL_TOOL_FORBIDDEN
```

随后 owner 自己回包：

```text
TOOL_CALL_END ok=true {"value": 4}
invocation completed（owner app）
```

**结论**：App 隔离在结果回传侧同样生效，外App无法代答。

## 6. P6.16 App 断线

```text
run_id = 01M74CMNBX5F38GDYY4FGWWR72
mode   = disconnect
```

宿主收到调用后立即关闭连接，不回包。

```text
TOOL_CALL_BEGIN  01M74CMP8V5MB84D8V72AMRE39
TOOL_CALL_END    ok=false APP_UNREACHABLE duration_ms=10 retryable=true
tool_invocations failed
run              completed（模型解释了失败）
```

**结论**：断线立即判 `APP_UNREACHABLE`（10 ms），不是等超时。

## 7. P6.17 工具超时

```text
run_id   = 01M74CN8NA0CGH5BSDDGQSWXVW
mode     = slow
tool_ms  = 1200
delay    = 3 s
```

服务端：

```text
TOOL_CALL_BEGIN
TOOL_CALL_END ok=false TOOL_TIMEOUT duration_ms=1201 retryable=true
tool_invocations failed
```

宿主晚到的结果：

```text
sent_late
server_error LOCAL_TOOL_UNKNOWN
```

**结论**：超时按 `tool_ms` 截断；迟到结果因 pending 已清理被判未知，不会补写结果。

## 8. 汇总

| ID | 场景 | 触发 | 观察 | 结果 |
|---|---|---|---|---|
| P6.12 | 重复结果 | 同结果发两次 | 只投递一次，1 行 invocation | 通过 |
| P6.13 | correlation 错误 | 错 msg_id | `LOCAL_TOOL_CORRELATION_MISMATCH`，pending 保留 | 通过 |
| P6.14 | 未知 invocation | 伪造 id | `LOCAL_TOOL_UNKNOWN` | 通过 |
| P6.15 | 跨 App 伪造 | 另一连接代答 | `LOCAL_TOOL_FORBIDDEN` | 通过 |
| P6.16 | App 断线 | 收到即断开 | `APP_UNREACHABLE`（10 ms） | 通过 |
| P6.17 | 工具超时 | 睡过 tool_ms | `TOOL_TIMEOUT`（1201 ms）+ 迟到结果 UNKNOWN | 通过 |

六次 Run 全部到达 `completed`：错误被记录并反馈给模型，没有卡死。

## 9. 局限

- 故障探针是单线程顺序执行，未覆盖并发重复/并发超时。
- P6.15 用两个 App 连接模拟，未覆盖同 App 多连接竞争。
- 未覆盖 `Run` 取消与工具超时同时发生。

## 10. 证据

```text
docs/reports/artifacts/retrofit/P6D/
  fault_host.py            故障探针
  bootstrap_a.json         owner app
  bootstrap_b.json         foreign app
  register_tools.json
  P6.12/  fault_host.log  db.txt  submit_run.json  summary.txt
  P6.13/  fault_host.log  db.txt  submit_run.json  summary.txt
  P6.14/  fault_host.log  db.txt  submit_run.json  summary.txt
  P6.15/  fault_host.log  db.txt  submit_run.json  summary.txt
  P6.16/  fault_host.log  db.txt  submit_run.json  summary.txt
  P6.17/  fault_host.log  db.txt  submit_run.json  summary.txt
  middleware.log
  harness.log
  migrate.log
```

## 11. 下一步

P7：持久化与观测。

```text
P7.1 run_events 与 tool_invocations 可核对
P7.2 指标可读（工具失败率、WS 连接数、事件丢弃）
P7.3 日志不含密钥
```
