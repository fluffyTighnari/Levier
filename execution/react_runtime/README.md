<!-- Levier - built-in React Runtime. -->

# React Runtime

## 单元卡

| 项 | 内容 |
|---|---|
| 职责 | 单次 Run 的 Agent 循环、模型适配、工具桥 |
| 入口 | `python -m levier_react_runtime --config react.example.yaml` |
| 输入 / 输出 | TaskAssign（gRPC）；事件 / ToolCallRequest（gRPC） |
| 依赖 | `levier_agent_core`、生成的 proto、模型 API、MCP client |
| 不负责 | HTTP API、App 注册、数据库、工具真实实现 |
| 测试 | `make react-runtime-test` |

The React Runtime is a thin built-in runtime on top of `levier-runtime`.
It owns the ReAct loop, LLM adapters, and React checkpointing. The shared MCP
client lives in the Runtime SDK optional module.
RuntimeChannel, lifecycle, event sequencing, and `RunContext` come from
`levier-runtime` (the independent Runtime SDK). The MCP runtime is a separate process under
`execution/mcp_runtime/`; it is not part of this package.

The React runtime provides:

- configuration loading with `LEVIER_REACT_RUNTIME_*` overrides (legacy `HARNESS_*` fallback)
- JSON/text structured logging
- health state snapshot
- ReAct, LLM providers, MCP tools, and React checkpointing

## Environment

The React Runtime owns its own virtual environment. From `execution/react_runtime/`:

```sh
uv sync --locked
```

or with the standard library:

```sh
python3 -m venv .venv
.venv/bin/python -m ensurepip
# levier-agent-core is a local workspace package; pip does not read
# [tool.uv.sources], so install it first when not using uv.
.venv/bin/python -m pip install -e ../agent_core
.venv/bin/python -m pip install -e ../runtime_sdk
.venv/bin/python -m pip install -e .
```

## Run

From `execution/react_runtime/`:

```sh
PYTHONPATH=src:../runtime_sdk/src .venv/bin/python -m levier_react_runtime \
  --config react.example.yaml --check
```

or start the React runtime:

```sh
PYTHONPATH=src:../runtime_sdk/src .venv/bin/python -m levier_react_runtime \
  --config react.example.yaml
```

The React Runtime is now built on top of `levier-runtime`. The
standalone MCP runtime is a separate process; start it with:

```sh
cd ../mcp_runtime
uv sync --locked
PYTHONPATH=src .venv/bin/python -m levier_mcp_runtime --socket /tmp/levier-mcp.sock
```
