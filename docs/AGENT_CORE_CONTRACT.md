<!-- Middleware — Agent Core contract v1. -->

# Agent Core 契约（v1）

本文件定义 MW 侧 Agent Core 的边界、数据结构和事件顺序。
Python 是第一个实现；未来 TS daemon 按同一契约实现。

## 1. 边界

```text
core
  循环、消息状态、工具编排、事件、取消

adapter
  TaskAssign → RunSpec
  CoreEvent → MW Event
  ToolCall → MW bridge / MCP runtime
  Cancel → abort
  checkpoint 持久化

MW
  调度、租约、重试、事件存储（不变）
```

禁止：

```text
core import MW / harness / mcp_runtime / protobuf
core 持久化
core 调度
core 重试
core 持有 MCP 连接或凭据
```

## 2. 数据结构

### RunSpec

| 字段 | 类型 | 说明 |
|---|---|---|
| `run_id` | str | 事件关联用 |
| `start_step` | int | 恢复时从哪一步开始（默认 1） |
| `messages` | `Message[]` | 初始输入或 checkpoint 恢复的消息 |
| `tools` | `ToolSpec[]` | 模型可用工具 |
| `model` | `ModelRef` | provider/model + 非秘密参数 |
| `max_steps` | int | 循环上限 |
| `step_timeout_ms` | int | 单次模型调用超时 |
| `tool_timeout_ms` | int | 单次工具超时 |
| `session_id` | str | provider 缓存会话（可空） |
| `metadata` | map | 透传标签（可空） |

### Message

| 字段 | 说明 |
|---|---|
| `role` | `system` / `user` / `assistant` / `tool` |
| `content` | 文本 |
| `name` | 工具名（tool 消息） |
| `tool_call_id` | assistant tool_calls 的 call_id |
| `tool_calls` | assistant 消息携带的调用 |

### ToolSpec / ToolCall / ToolResult

```text
ToolSpec   name / description / input_schema_json
ToolCall   call_id / name / arguments_json / invocation_id
ToolResult ok / result_json / error_code / error_message / retryable / duration_ms
```

约定：

```text
arguments_json、result_json 是字符串，不是对象
invocation_id 由 adapter 生成并保证幂等
core 不解释 arguments_json 的内容
```

### Usage

```text
input_tokens
output_tokens
total_tokens
cache_read_tokens
cache_write_tokens
```

adapter 负责把 provider 的 `prompt_tokens/completion_tokens` 等归一化到这里。

## 3. 事件

| type | 必需字段 |
|---|---|
| `run_started` | `run_id` |
| `step_started` | `run_id`, `step` |
| `message_delta` | `run_id`, `step`, `message_id`, `delta` |
| `message_completed` | `run_id`, `step`, `message_id`, `content` |
| `usage` | `run_id`, `step`, `usage` |
| `tool_started` | `run_id`, `step`, `invocation_id`, `tool_name` |
| `tool_finished` | `run_id`, `step`, `invocation_id`, `tool_name`, `ok` |
| `run_finished` | `run_id`, `status` |
| `error` | `run_id`, `code`, `message` |

`status`：

```text
completed / failed / cancelled / timeout
```

`error.code`（core 层）：

```text
STEP_TIMEOUT
TOOL_TIMEOUT
MAX_STEPS
ABORTED
PROVIDER_ERROR
BAD_RESPONSE
```

adapter 负责映射到 MW 的错误码体系。

## 4. 事件顺序

### 正常工具调用

```text
run_started
step_started(step=1)
message_delta / message_completed(tool_calls)
usage
tool_started
tool_finished
step_started(step=2)
message_completed(content)
usage
run_finished(completed)
```

### 工具失败

```text
tool_finished(ok=false, error_code=...)
→ 结果作为 tool 消息回填
→ 模型决定继续或结束
```

### 超时

```text
step 超时  → error(STEP_TIMEOUT) → run_finished(timeout)
tool 超时  → tool_finished(ok=false, TOOL_TIMEOUT)
```

### 取消

```text
adapter 设置 abort
→ core 在下一个检查点停止
→ run_finished(cancelled)
```

## 5. 循环语义

```text
1. 调 provider.complete(messages, tools, model)
2. 发出 message_completed 与 usage
3. 无 tool_calls → run_finished(completed)
4. 有 tool_calls → 逐个 tool_provider.call
5. 结果作为 role=tool 消息追加
6. step += 1，回到 1
7. 超过 max_steps → error(MAX_STEPS) + run_finished(failed)
```

工具执行默认 **串行**。
并行工具是后续可选能力，需 adapter 声明支持。

## 6. 取消与超时

```text
core 只接受 AbortSignal
不在 core 里创建新的事件循环/信号处理器
provider 调用与工具调用都必须可被 abort
```

超时由 adapter 或 core 计时均可，但语义必须一致：
超时产生稳定错误码，不抛裸异常。

adapter 可以额外提供两个钩子：

```text
on_step_end(messages, step)   每步工具完成后（checkpoint）
on_cancel(messages, step)     task 被取消、即将退出时（保存 checkpoint）
```

## 7. Checkpoint / 恢复

```text
core 不持久化
adapter 在 run 结束时取 messages + steps 存 checkpoint
恢复时把 messages 放进 RunSpec，从 step = len 继续
```

不允许 core 持有跨 attempt 的状态。

## 8. 与 MW 事件的映射

| CoreEvent | MW Event |
|---|---|
| `run_started` | `RUN_STATUS running`（由 adapter/manager 发） |
| `step_started` | 不直接发；用于步数 |
| `message_delta` | `MESSAGE_DELTA`（ephemeral） |
| `message_completed` | `MESSAGE_COMPLETED` |
| `usage` | `USAGE` |
| `tool_started` | `TOOL_CALL_BEGIN` |
| `tool_finished` | `TOOL_CALL_END` |
| `run_finished` | `RUN_STATUS <status>` |
| `error` | `ERROR` |

## 9. Conformance fixtures

语言无关，JSON 格式，P8C.4 实现：

```text
1. 单工具调用 → 完成
2. 多工具 → 完成
3. 工具失败 → 完成
4. 工具超时
5. max_steps
6. abort
7. checkpoint 恢复
```

每个 fixture 定义：

```text
run_spec
provider 脚本（每次 complete 的返回）
tool 脚本（每次 call 的返回）
期望 CoreEvent 序列
期望 RunResult
```

Python 与未来 TS 实现都必须跑通同一组 fixture。

## 10. 版本

```text
contract: 1
python package: middleware-agent-core 0.1.0
```

变更规则：

```text
新增可选字段 → 兼容
修改字段含义 / 删除字段 → 升版本
事件 type 不允许重命名
```

## 11. 非目标

```text
MCP 连接
工具路由（local/global/MCP）
凭据解析
transport / protobuf
调度 / 租约 / 重试
插件系统 / DI
```
