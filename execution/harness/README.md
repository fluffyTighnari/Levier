<!-- Middleware — stripped-down Python harness. -->

# Middleware Harness

## 单元卡

| 项 | 内容 |
|---|---|
| 职责 | 单次 Run 的 Agent 循环、模型适配、工具桥 |
| 入口 | `python -m levier_harness --config levier.example.yaml` |
| 输入 / 输出 | TaskAssign（gRPC）；事件 / ToolCallRequest（gRPC） |
| 依赖 | `levier_agent_core`、生成的 proto、模型 API、MCP client |
| 不负责 | HTTP API、App 注册、数据库、工具真实实现 |
| 测试 | `make harness-test` |

The harness is the execution-side daemon for the middleware control plane.
It owns the RuntimeChannel client, ReAct loop, LLM adapters, tool bridge,
MCP client, and checkpointing. The MCP runtime is a separate process under
`execution/mcp_runtime/`; it is not part of this package.

P3-23 provides:

- configuration loading with `HARNESS_*` overrides
- JSON/text structured logging
- health state snapshot
- a start/stop process skeleton

## Environment

The harness owns its own virtual environment. From `execution/harness/`:

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
.venv/bin/python -m pip install -e .
```

## Run

From `execution/harness/`:

```sh
PYTHONPATH=src:src/gen .venv/bin/python -m levier_harness \
  --config levier.example.yaml --check
```

or start the skeleton:

```sh
PYTHONPATH=src:src/gen .venv/bin/python -m levier_harness \
  --config levier.example.yaml
```

RuntimeChannel, ReAct, LLM, and tools are added in P3-24 onward. The
standalone MCP runtime is a separate process; start it with:

```sh
cd ../mcp_runtime
uv sync --locked
PYTHONPATH=src .venv/bin/python -m levier_mcp_runtime --socket /tmp/levier-mcp.sock
```
