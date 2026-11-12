<!-- Middleware — P8C.1 agent core contract and types. -->

# P8C.1：Agent Core 契约与类型

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-12 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P8C.1 |
| 状态 | 完成 |
| 代码提交 | 见下 |

## 1. 新增包

```text
agent_core/
  pyproject.toml
  src/agent_core/
    __init__.py
    types.py
    protocols.py
    events.py
  tests/test_types.py
```

独立 Python 包，没有第三方依赖，不 import MW / harness / MCP。

## 2. 契约

`docs/AGENT_CORE_CONTRACT.md` 定义：

```text
边界与禁止依赖
RunSpec / Message / ToolSpec / ToolCall / ToolResult / Usage
CoreEvent 词汇与必需字段
事件顺序（工具调用 / 失败 / 超时 / 取消）
循环语义
checkpoint 归属
CoreEvent → MW Event 映射
conformance fixture 格式
版本规则
```

## 3. 类型设计

- `RunSpec`：messages / tools / model / limits / session。
- `ModelRef`：只有 provider/model/params，凭据留在 adapter。
- `Usage`：归一化 `input/output/total/cache_*`，修掉 provider key 不一致的坑。
- `ToolCall/ToolResult`：JSON 仍是字符串，core 不解析业务参数。
- `CoreEvent`：9 个 dataclass 的联合，每个带稳定 `type` 和 `to_dict()`。

事件：

```text
run_started / step_started / message_delta / message_completed
usage / tool_started / tool_finished / run_finished / error
```

## 4. 边界测试

`ImportBoundaryTest` 扫描 `agent_core/src`，禁止出现：

```text
middleware_harness
mcp_runtime
middleware.runtime
google.protobuf
```

这条测试会一直守住 core 的独立性。

## 5. 验证

```sh
make agent-core-test
```

```text
Ran 10 tests
OK
```

覆盖：

```text
Usage total 推导
事件 type 固定与 JSON 序列化
嵌套 usage
RunSpec 默认值
ToolResult 错误语义
core 无 MW/MCP import
```

## 6. 决策

```text
core 位置        顶层 agent_core/ 独立包
首个实现         Python
TS               等出现 daemon 需求再实现
工具执行          默认串行
重试              core 不做，adapter/MW 负责
checkpoint        MW/adapter 唯一持有
```

## 7. 下一步

P8C.2：把 `strategies/react.py` 的循环搬进 core。

```text
core loop + fake provider/tool
harness 改为调用 core
现有 82 个测试保持通过
```
