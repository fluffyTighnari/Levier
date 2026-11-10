<!-- Middleware — zero-dependency counter agent example. -->

# Counter Agent（0 依赖基线）

这是一个“已经存在的简单 Agent 应用”，用于模拟改造为 Middleware App 之前的状态。

## 特征

- 单个 `main.py`。
- 只用 Python 标准库：`json`、`os`、`sys`、`urllib`。
- 没有 Middleware 注册、HTTP 服务、SDK、生成代码或第三方包。
- 硬编码计数器工具：
  - `counter_add(delta)`
  - `counter_get()`
  - `counter_reset()`
- 自带最小 ReAct 循环，直接调用 DeepSeek OpenAI-compatible API。
- 系统提示强制模型必须调用工具，不能自己心算计数器。

## 运行

根目录 `.env`：

```dotenv
DEEPSEEK_API_KEY=...
```

可选：

```dotenv
DEEPSEEK_MODEL=deepseek-flash
DEEPSEEK_BASE_URL=https://api.deepseek.com
```

执行：

```sh
python3 examples/counter_agent/main.py
```

自定义任务：

```sh
python3 examples/counter_agent/main.py \
  "Reset the counter, add 7, subtract 2 by adding -2, then read the value."
```

预期输出类似：

```text
[model] deepseek-flash
[user] Add 2, then add 3, then read the counter and tell me the final value...
[tool] counter_add({"delta": 2}) -> {"value": 2}
[tool] counter_add({"delta": 3}) -> {"value": 5}
[tool] counter_get({}) -> {"value": 5}
[assistant] The final counter value is 5.
[final] counter=5 steps=4
```

## 与 Middleware 架构的关系

当前这个基线把 **ReAct 循环 + 工具实现 + 模型调用** 都放在一个进程里。

Middleware 的正式模型是：

| 职责 | 改造后归属 |
|---|---|
| App 注册、Run 提交、状态/事件 | App → HTTP API |
| 计数器函数 | App 本地工具，通过 WS `local_tool_call/local_tool_result` 执行 |
| ReAct 循环、LLM 调用 | Harness `react` strategy |
| 工具路由、幂等、ACL | Middleware Gateway |

因此后续改造不是“把整个 main 搬进 Middleware”，而是拆分：

1. 保留计数器函数，增加 App WebSocket 客户端，响应 `local_tool_call`。
2. 删除本地 ReAct 循环，改为 `POST /api/v1/runs`。
3. Harness 使用 DeepSeek provider 执行同一套工具调用。
4. 通过 HTTP events / WS 验证与基线相同的最终计数器结果。

这样可以比较改造前后的行为差异，而不是只验证“新写了一个 demo”。

## 注册元数据（P1.1）

`tools.py` 中的 `TOOL_DEFINITIONS` 就是 Middleware 的注册格式：

```json
{
  "tools": [
    {
      "name": "counter_add",
      "description": "...",
      "schema": {"type": "object", "...": "..."},
      "timeout_ms": 10000
    }
  ]
}
```

详细合同与实测：

- `docs/reports/REGISTRATION-CONTRACT.md`

## 改造计划

逐边调试步骤见：

- `docs/APP_RETROFIT_PLAN.md`

## 设计约束

- 不读取或打印 API key。
- 只调用 `chat/completions`，不依赖厂商 SDK。
- 工具调用和结果都打印为可核对的行。
- 失败时返回非零退出码。
