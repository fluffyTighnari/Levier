<!-- Middleware — retrofit P7 persistence and observability. -->

# Retrofit P7：持久化与观测（P7.1–P7.11）

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-11 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P7 |
| 状态 | 完成 |
| 代码提交 | `daf21f8` |
| 样本 Run | `01M74CYEXPRG5DM9FWGBDCDNGA` |
| 样本 App | `app_01M74CWXT44CWACYF02146JXYF` |

## 1. 样本

正常接入路径跑一次基线同款任务：

```text
App: serve.py（验证 → 注册 → WS → 服务）
Run: react / deepseek-flash，add 2 → add 3 → get
HTTP: X-Request-ID = p7-trace-1794370386
```

## 2. P7.1 apps

```text
app_id = app_01M74CWXT44CWACYF02146JXYF
name   = p7-app-1794370336
status = active
hash_len = 64
hash looks plaintext = false
```

key 只保存 hash，长度 64，非 `mw_` 明文。

## 3. P7.2 tool_defs

```text
counter_add   | local | timeout 10000 | version 3
counter_get   | local | timeout 10000 | version 3
counter_reset | local | timeout 10000 | version 3
```

version=3 是因为 bootstrap 后 `register_tools.py` 与 `serve.py` 各自全量注册过一次，
再叠加前一次尝试；重复注册提升 version 是既定行为，不是重复行。

## 4. P7.3 runs

```text
runtime  = normal
strategy = react
status   = completed
attempt  = 1
trace_id = p7-trace-1794370386
```

## 5. P7.4 run_events

```text
events = 16
min_seq = 1
max_seq = 16
expected = 16
distinct_seq = 16
```

序列完整：

```text
1  RUN_STATUS
2  USAGE
3  MESSAGE_COMPLETED
4  TOOL_CALL_BEGIN  counter_add
5  TOOL_CALL_END    counter_add
6  USAGE
7  MESSAGE_COMPLETED
8  TOOL_CALL_BEGIN  counter_add
9  TOOL_CALL_END    counter_add
10 USAGE
11 MESSAGE_COMPLETED
12 TOOL_CALL_BEGIN  counter_get
13 TOOL_CALL_END    counter_get
14 USAGE
15 MESSAGE_COMPLETED
16 RUN_STATUS
```

## 6. P7.5 run_tasks

```text
RUN_DISPATCH | DONE      | attempt 1
RUN_TIMEOUT  | CANCELLED | attempt 0
```

## 7. P7.6 tool_invocations

```text
01M74CYG22CBYGE43HYBF5YP2P | counter_add | completed | {"value": 2}
01M74CYH3STB0VEHVGB80BYNTH | counter_add | completed | {"value": 5}
01M74CYHR9ZZ1CXHNDENSF4J1A | counter_get | completed | {"value": 5}
```

## 8. P7.7 trace 链

```text
submit X-Request-ID : p7-trace-1794370386
runs.trace_id       : p7-trace-1794370386
TaskAssign          : trace_context = p7-trace-1794370386
```

同一 ID 贯穿 HTTP → DB → 调度 → Harness。

## 9. P7.8 指标

用运行前后两次抓取计算增量：

```text
mw_runs_total{status="completed"}                     +1
mw_run_queue_duration_seconds_count                   +1
mw_tool_invocations_total{tool="counter_add", ok}     +2
mw_tool_invocations_total{tool="counter_get", ok}     +1
mw_events_total{type="TOOL_CALL_BEGIN"}               +3
mw_events_total{type="TOOL_CALL_END"}                 +3
mw_events_total{type="MESSAGE_COMPLETED"}             +4
mw_events_total{type="USAGE"}                         +4
mw_events_total{type="RUN_STATUS"}                    +2
mw_ws_connections{app="..."}                          0（断开后归零）
```

与 Run 的 3 次工具调用、16 条事件、4 次模型消息完全对应。
指标没有多算，也没有漏算。

## 10. P7.9 脱敏

```text
evidence/logs 出现 api_key：0 个文件
git 跟踪文件出现 api_key：0 个文件
middleware/harness 日志出现 "Bearer "：0
middleware/harness 日志出现 "ws_token="：0
apps.api_key_hash == 明文：false
```

## 11. P7.10 key rotate

```text
旧 key GET /api/v1/apps/{app_id} → 401
新 key GET /api/v1/apps/{app_id} → 200
tool_defs before = 3
tool_defs after  = 3
```

轮换只换凭据，不动工具归属。

## 12. P7.11 MW 重启

```text
              before   after
apps          42       42
tool_defs      3        3
run_events    16/16    16/16
invocations    3        3
```

```text
GET /api/v1/runs/{id} → 200, status=completed
runs row              → completed / lease_epoch=1
```

重启后注册信息、事件、调用记录都在；没有重复写入；终态没有被改写。

## 13. 汇总

| ID | 检查 | 结果 |
|---|---|---|
| P7.1 | apps | 通过 |
| P7.2 | tool_defs | 通过 |
| P7.3 | runs | 通过 |
| P7.4 | run_events 连续 | 通过 |
| P7.5 | run_tasks | 通过 |
| P7.6 | tool_invocations | 通过 |
| P7.7 | trace 链 | 通过 |
| P7.8 | 指标增量 | 通过 |
| P7.9 | 脱敏 | 通过 |
| P7.10 | key rotate | 通过 |
| P7.11 | 重启持久化 | 通过 |

## 14. 局限

- 指标为单次运行增量，未做持续压测下的基数核对。
- 重启验证只覆盖单实例、且 Harness 已下线；未覆盖重启期间仍有活跃 Run 的情况。
- 脱敏检查是精确匹配 key/token 形态，不排除其他敏感字段被记录。

## 15. 证据

```text
docs/reports/artifacts/retrofit/P7/
  bootstrap.json
  register_tools.json
  submit_run.json
  serve.log
  db_apps.txt
  db_tools.txt
  db_runs.txt
  db_events.txt
  db_tasks.txt
  db_invocations.txt
  trace.txt
  metrics_before2.txt
  metrics_after2.txt
  metrics_delta.txt
  redaction.txt
  rotate.txt
  mw_restart.txt
  middleware.log
  middleware_after_restart.log
  harness.log
  migrate.log
  summary.txt
```

## 16. 下一步

P8：语言无关验证。

```text
P8.1 Python → HTTP
P8.2 Node → WS 握手
P8.3 Node 处理 local_tool_call
P8.4 无 SDK / 生成代码依赖
P8.5 字段与 DESIGN 对齐
```
