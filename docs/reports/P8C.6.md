<!-- Middleware — P8C.6 regression. -->

# P8C.6：Agent Core 全量回归

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-12 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P8C.6 |
| 状态 | 完成 |
| 代码基线 | `b7cb376`（本步不改代码） |

## 1. 范围

P8C.2 抽循环、P8C.4 加 conformance 后，确认 Middleware 原有链路没有回归：

```text
Go 单元/集成      含 PostgreSQL
clean checkout    b7cb376 重新 clone
Python core       18 tests（含 9 fixtures）
harness           82 tests
mcp_runtime       27 tests
retrofit-e2e      App + 真实模型 + 工具
mcp-e2e           MCP runtime + 凭据解析
echo / m1         legacy smoke
daemon-check      gRPC 任务、取消、事件
lint / build / deps / size / contract / proto
```

## 2. 结果

| 检查 | 结果 | 证据 |
|---|---|---|
| go test（含 DB，`-count=1`） | pass | `go_test_with_db.log` |
| clean checkout `b7cb376` | pass | `clean_checkout.log` |
| agent_core | 18 tests | `core_test.log` |
| harness | 82 tests | `harness_test.log` |
| mcp_runtime | 27 tests | `mcp_runtime_test.log` |
| retrofit-e2e | PASS | `retrofit_e2e.log` |
| mcp-e2e | PASS | `mcp_e2e.log` |
| e2e-echo | PASS | `e2e_echo.log` |
| m1-e2e | PASS | `m1_e2e.log` |
| daemon-check | PASS | `daemon_check.log` |
| build / lint / deps / size | pass | `build.log`、`lint.log`、`deps_check.log`、`size_report.log` |
| contract / proto | pass | `contract_check.log`、`proto_check.log` |

## 3. 关键运行

```text
retrofit-e2e
  run 01M76ZN5Z9BEB6H009KTMMS09R
  status completed
  tool begin=3 end=3 invocations=3
  final {"value": 5}
  seq 14/14/14 连续
  RUN_DISPATCH=DONE, RUN_TIMEOUT=CANCELLED

mcp-e2e
  status completed
  mcp__e2e_echo__echo_token ok
  result 含 secret
  gateway_invocations=0
  error_events=0
  task_assign_binding=1

m1-e2e
  run 01M76ZPGY68BT1GHJ6TJP5KJZN completed
  events seq 1/2/3 valid
  workflow DONE

daemon-check
  hello / ack / task_assign / events / completed / cancel 全部 ok
```

## 4. 事件数差异说明

`retrofit-e2e` 这次是 14 个事件，P8C.2 是 16 个。不是回归：

```text
这次模型第 1 步一次返回 2 个 counter_add
P8C.2 每次只返回 1 个工具
```

工具仍然串行执行，`begin/end/invocations` 都是 3，`seq` 连续，指标一致。
脚本校验的是顺序与计数，不依赖固定事件总数。

## 5. e2e-echo 的配置

`e2e-echo` 是旧版 bearer smoke，使用：

```text
allow_legacy_bearer: true
bearer_token: change-me
```

仅用于本地兼容性验证；`config.example.yaml` 的生产默认仍是 `false`。
临时配置放在 `/tmp`，没有提交。

## 6. 规模

```text
Go 非测试文件        93
Go 行数             13150
Python 源文件        40
Python 行数          5089
超 450 行文件         0
长函数                8 个，按评审保留
```

`react.py` 目前 430 行，仍在 450 行阈值内；后续再长就按 adapter 职责拆文件。

## 7. 结论

```text
P8C.2 / P8C.3 / P8C.4 的行为与 Middleware 现有链路一致
事件、工具、checkpoint、MCP、调度、取消均未回归
```

## 8. 证据

```text
docs/reports/artifacts/P8C.6/
  go_test_with_db.log
  clean_checkout.log
  core_test.log
  harness_test.log
  mcp_runtime_test.log
  retrofit_e2e.log + retrofit-e2e/
  mcp_e2e.log + mcp-e2e/
  e2e_echo.log
  m1_e2e.log
  daemon_check.log + daemon_check_middleware.log
  build.log
  lint.log
  deps_check.log
  size_report.log
  contract_check.log
  proto_check.log
```

## 9. 下一步

P8C 主干完成。TS core（P8C.5）只在出现真实 daemon 需求时实现，
直接用 `agent_core/conformance/fixtures` 对齐。
