<!-- Middleware — P8C.4 conformance fixtures. -->

# P8C.4：语言无关 conformance fixtures

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-12 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P8C.4 |
| 状态 | 完成 |
| 代码提交 | 见下 |

## 1. 目的

把“core 应该怎么表现”从 Python 实现里拿出来，变成一组可执行的 JSON 用例。

```text
provider 脚本 → core → 期望事件序列 + 终态
```

Python core 现在跑这组用例；未来 TS core 必须跑同一组用例，才算契约一致。

## 2. Fixture 格式

```text
agent_core/conformance/fixtures/<name>.json
```

关键字段：

```text
provider_script   每一步模型返回：content / tool_calls / usage / deltas / error / sleep_ms
tool_script       每次工具返回：ok / result_json / error / sleep_ms
abort_after_steps 第 N 步工具完成后置 abort
expected          终态、步数、调用次数、事件序列
```

匹配规则：

```text
事件按顺序逐条比对
expected 是子集匹配，实际事件可带额外字段（ts_ms、duration_ms、message_id）
```

## 3. 覆盖的 9 个场景

| fixture | 验证点 |
|---|---|
| `single_tool` | 一次工具调用后继续，最终回答 |
| `multi_tool` | 单步内多个工具，串行执行且顺序稳定 |
| `tool_failure` | 工具失败回填给模型，模型恢复 |
| `tool_timeout` | 工具超时 → `TOOL_TIMEOUT`，不中断循环 |
| `step_timeout` | 模型超时 → `STEP_TIMEOUT`，run failed |
| `max_steps` | 达到上限 → `MAX_STEPS` + `run_finished failed` |
| `abort` | 外部 abort 在下一步开始前生效 |
| `checkpoint_resume` | 从 `start_step` 续跑，历史消息不重复执行 |
| `streaming_delta` | delta 事件在 `message_completed` 之前 |

固定事件顺序（以 `single_tool` 为例）：

```text
run_started
step_started 1
usage
message_completed
tool_started
tool_finished
step_started 2
usage
message_completed
run_finished completed
```

## 4. 本步修正

运行 `abort` fixture 时发现：轮询在下一步开始前检测到 abort，但 `run_finished.steps`
用的是循环变量（已前进到下一步），导致少跑一步却多记一步。

修正：

```text
last_step 只在 step_started 成功后更新
abort 在 step_started 之前 → steps 保持上一步
run_finished / RunResult 统一使用 last_step
```

这同时让 `on_cancel` 拿到真实的已完成步数。

## 5. 运行方式

```sh
make agent-core-test
# 或
PYTHONPATH=agent_core/src python3 -m unittest discover -s agent_core/tests -v
```

单独跑 conformance：

```sh
PYTHONPATH=agent_core/src python3 -m unittest discover -s agent_core/tests -p 'test_conformance.py' -v
```

## 6. 验证

```text
conformance fixtures   9/9 OK
agent_core tests      18 OK
harness tests         82 OK
```

## 7. 文件

```text
agent_core/conformance/README.md
agent_core/conformance/fixtures/*.json
agent_core/tests/test_conformance.py
agent_core/src/agent_core/loop.py   （last_step 修正）
```

## 8. 下一步

P8C.6 全量回归；TS core（P8C.5）等出现真实需求再实现。
