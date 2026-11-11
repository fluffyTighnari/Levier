<!-- Middleware — retrofit regression and one-click closure. -->

# Retrofit P10：回归与收口

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-11 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P10 |
| 状态 | 完成 |
| 一键脚本 | `scripts/retrofit_e2e.sh` / `make retrofit-e2e` |
| 本次运行 | `01M74KKBS34CC0Y9DW06HF596H` |

## 1. 一键脚本做什么

```text
preflight   检查环境变量、psql/curl/python3、Harness venv、迁移
start       Middleware → Harness → App（serve.py）
run         bootstrap App + 注册工具 + 提交 P0.1 提示词
wait        轮询 runs.status 到终态（最长 120s）
validate    DB：3 次调用、counter=5、事件连续、trace 一致
            metrics：1 completed run、add=2、get=1、BEGIN=3
evidence    写入 P10/run-<UTC>/summary.txt 与原始日志
cleanup     杀掉 App/Harness/MW，删除凭据文件
```

退出码：

```text
0  PASS
1  任一断言失败或依赖缺失
```

## 2. 运行方式

```sh
make retrofit-e2e

# 或指定证据目录
RETROFIT_E2E_OUT=/tmp/retrofit-e2e ./scripts/retrofit_e2e.sh
```

前置条件：

```text
PostgreSQL 可连接（MIDDLEWARE_DATABASE_URL）
DEEPSEEK_API_KEY 可用（.env）
bin/middleware 可执行（缺失时自动构建）
harness/.venv 存在
8080/9090 未被占用
```

脚本会读取 `.env.local` / `.env`，但不会打印其中的值。

## 3. P10.1 Go 全量

```sh
MIDDLEWARE_TEST_POSTGRES=1 MIDDLEWARE_TEST_DATABASE_URL=... go test -count=1 ./...
```

```text
全部包 ok
```

日志：`docs/reports/artifacts/retrofit/P10/go_test.log`

## 4. P10.2 Harness

```text
make harness-test
Ran 82 tests
OK
```

## 5. P10.3 MCP Runtime

```text
make mcp-runtime-test
Ran 17 tests
OK
```

## 6. P10.4 一键脚本实跑

```text
$ make retrofit-e2e
== preflight ==
== start Middleware ==
== start Harness ==
== bootstrap App ==
== start App ==
== submit run ==
run_id=01M74KKBS34CC0Y9DW06HF596H trace=retrofit-e2e-20261111T060920Z
== wait for terminal state ==
== validate database ==
== validate metrics ==
== write evidence ==

run_id: 01M74KKBS34CC0Y9DW06HF596H
trace_id: retrofit-e2e-20261111T060920Z
status: completed
tool_calls: begin=3 end=3 invocations=3
final_value: {"value": 5}
events: 16/16/16 (count/max_seq/expected)
tasks: RUN_DISPATCH=DONE,RUN_TIMEOUT=CANCELLED
metrics: runs_completed=1 counter_add_ok=2 counter_get_ok=1 tool_begin=3

retrofit-e2e: PASS
```

## 7. 校验项

| 检查 | 期望 | 实际 |
|---|---|---|
| Run 终态 | completed | completed |
| TOOL_CALL_BEGIN | 3 | 3 |
| TOOL_CALL_END | 3 | 3 |
| completed invocations | 3 | 3 |
| `counter_get` 结果 | `{"value": 5}` | `{"value": 5}` |
| 事件序号 | 连续 | 16/16/16 |
| `trace_id` | 提交时的 request-id | 一致 |
| 任务 | DISPATCH DONE / TIMEOUT CANCELLED | 一致 |
| 指标 | 1 run / 2 add / 1 get / 3 BEGIN | 一致 |

## 8. P10.5 证据索引

`docs/reports/artifacts/retrofit/README.md` 已补全 P0–P10 的全部目录与合并说明：

```text
P4.1/     覆盖 P4.1–P4.3（共享一次运行）
P6C/      覆盖 P6.9–P6.11
P6D/      覆盖 P6.12–P6.17
P10/run-* 一键脚本的运行证据
```

规则仍为：一目录一运行、`summary.txt` 固定字段、不存密钥。

## 9. 清理验证

```text
credentials.json  已删除
8080/9090         已释放
Residual processes  无
```

## 10. 局限

- 脚本**不启动** PostgreSQL，只做连接检查；环境准备仍由 `dev-up` 或本机 PG 负责。
- 依赖真实 DeepSeek，一次运行需要数秒且受网络影响；失败时会保留日志。
- 端口固定 8080/9090，并行运行会冲突。
- 只验证成功路径；负向路径由 P6D 覆盖。
- 证据目录随运行累积，需要在收工时定期归档。

## 11. 收口结论

| 阶段 | 状态 |
|---|---|
| P0 基线 | 完成 |
| P1 本地工具 | 完成 |
| P2 App 壳 | 完成 |
| P3 注册落库 | 完成 |
| P4 TaskAssign | 完成 |
| P5 WS 传输 | 完成 |
| P6（含 P6A/P6B/P6C/P6D） | 完成 |
| P7 持久化与观测 | 完成 |
| P8 语言无关 | 完成 |
| P10 回归收口 | 完成 |

一条命令可复现的最小闭环：

```sh
make retrofit-e2e
```

## 12. P9 修复后复跑

P9 的恢复修复（queued sweep、App 自动重连、503 映射）合入后再次执行：

```text
run_id: 01M74MKJJ03N02PHEB3VJZSN62
trace_id: retrofit-e2e-20261111T062656Z
status: completed
tool_calls: begin=3 end=3 invocations=3
final_value: {"value": 5}
events: 16/16/16
retrofit-e2e: PASS
```

日志：`docs/reports/artifacts/retrofit/P10/retrofit_e2e_after_p9.log`

## 13. 证据

```text
docs/reports/artifacts/retrofit/P10/
  go_test.log
  harness_test.log
  mcp_runtime_test.log
  retrofit_e2e.log
  run-20261111T060920Z/
    migrate.log
    middleware.log
    harness.log
    serve.log
    serve.err
    submit_run.json
    submit_run.err
    metrics_before.txt
    metrics_after.txt
    bootstrap.json
    bootstrap.err
    summary.txt
```
