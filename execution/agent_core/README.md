<!-- Middleware — language-neutral agent core. -->

# Agent Core

## 单元卡

| 项 | 内容 |
|---|---|
| 职责 | 语言无关的 Agent 循环、工具编排、事件、取消 |
| 入口 | `levier_agent_core` |
| 输入 / 输出 | 模型响应 / 工具结果；循环事件与终态 |
| 依赖 | 无 Middleware / Harness / MCP / protobuf 依赖 |
| 不负责 | 传输、持久化、模型供应商适配、工具真实实现 |
| 测试 | `make agent-core-test` |

Loop, tool orchestration, events, and cancellation shared by daemons.

- Contract: `docs/AGENT_CORE_CONTRACT.md`
- No Middleware / Harness / MCP / protobuf imports.
- Adapters own transport, MW event mapping, tool bridges, and persistence.

## Conformance

`conformance/fixtures/*.json` 是语言无关的 core 行为用例。
Python core 和未来的 TS core 必须跑通同一组 fixture。

## Test

```sh
make agent-core-test
# or
PYTHONPATH=execution/agent_core/src python3 -m unittest discover -s execution/agent_core/tests -v
```

## Install into the harness venv

```sh
cd execution/harness && uv sync --locked
```

`execution/harness/pyproject.toml` depends on `levier-agent-core` via a local path.
