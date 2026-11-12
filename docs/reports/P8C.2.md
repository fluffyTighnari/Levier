<!-- Middleware — P8C.2 extract the loop. -->

# P8C.2：抽取 ReAct 循环

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-12 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P8C.2 |
| 状态 | 完成 |
| 代码提交 | 见下 |

## 1. 核心变化

```text
harness/strategies/react.py
  之前：循环、模型调用、工具执行、事件、checkpoint 全在一起
  现在：只做 TaskAssign ↔ RunSpec、CoreEvent ↔ MW Event、工具桥
```

`agent_core/loop.py` 负责：

```text
step 循环
provider 调用与 step 超时
message / usage 事件
工具调用、invocation_id、tool 超时
tool 消息回填
max_steps
abort
on_step_end / on_cancel
```

## 2. 适配层

| 方向 | 处理 |
|---|---|
| TaskAssign → RunSpec | proto 消息/tools/model → core 类型 |
| CoreEvent → MW Event | message/usage/tool/error 映射 |
| ToolCall → ToolRouter | local/global/MCP 由现有 router 执行 |
| ctx.cancelled → AbortSignal | `_CtxAbort` |
| checkpoint | `on_step_end` / `on_cancel` / finally |

转换函数集中在 `react.py`：

```text
_to_core_message / _to_harness_message
_to_core_tool / _to_harness_tool
_to_core_response
_to_core_usage
```

## 3. Usage 归一化

core 使用：

```text
input_tokens / output_tokens / total / cache_read / cache_write
```

适配层兼容：

```text
input_tokens / output_tokens
prompt_tokens / completion_tokens
prompt_cache_hit_tokens → cache_read_tokens
```

这同时修掉了 DeepSeek 事件里的 token key 与 control 读取不一致的问题。

## 4. 兼容性

保留的行为：

```text
事件顺序
MESSAGE_COMPLETED 字段
TOOL_CALL_BEGIN/END 字段
MAX_STEPS_EXCEEDED
STEP_TIMEOUT
LLM_ERROR
checkpoint 保存时机
```

新增：

```text
core 事件 run_started / step_started / run_finished（adapter 忽略）
agent_core ProviderFailure
```

## 5. 验证

```text
agent_core 17 tests            OK
harness    82 tests            OK
retrofit-e2e                  PASS
  run 01M76ZDWFWYTGP3HQZ7FVK2FZP
  3 次工具调用，final=5
mcp-e2e                       PASS
  run 01M773...（见日志）
  tool ok / secret / 0 invocation
```

## 6. 依赖调整

```text
harness/pyproject.toml
  middleware-agent-core = { path = "../agent_core", editable = true }
harness/uv.lock 更新

Makefile / e2e 脚本 PYTHONPATH 增加 agent_core/src
```

这样即使没有执行 `uv sync`，测试和脚本也能找到 core。

## 7. 证据

```text
docs/reports/artifacts/P8C.2/
  retrofit_e2e.log
  mcp_e2e.log
  retrofit-e2e/
  mcp-e2e/
```

## 8. 下一步

P8C.3（工具适配层）已随本步完成：
core 只依赖 `ToolProvider`，local/global/MCP 路由仍在 adapter。

P8C.4：conformance fixtures。
