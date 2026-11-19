# execution

执行侧分为一个 SDK、一个共享 MCP Runtime，以及两个薄的 Runtime 实现。

```text
execution/
  runtime_sdk/     Runtime SDK
                   channel / lifecycle / RunContext / events / optional MCP client

  mcp_runtime/     独立 MCP 进程
                   所有需要 MCP 的 Runtime 共用一个 socket

  echo_runtime/    内置最小 Runtime
                   只依赖 runtime_sdk，也是 Runtime 接入示例

  react_runtime/   内置 ReAct Runtime
                   基于 runtime_sdk + agent_core + LLM/tool helpers

  agent_core/      可选 ReAct loop helper
                   不是 Runtime 框架
```

## 依赖方向

```text
echo_runtime ─────┐
                  ├──> runtime_sdk
react_runtime ────┘
       │
       ├──> agent_core
       └──> mcp_runtime (optional shared process)

mcp_runtime 独立运行，不依赖 runtime_sdk 或具体 Runtime。
```

## 入口

```sh
# Echo
PYTHONPATH=execution/runtime_sdk/src:execution/echo_runtime/src \
  python -m levier_echo_runtime \
  --config execution/echo_runtime/runtime.yaml

# React
PYTHONPATH=execution/agent_core/src:execution/runtime_sdk/src:execution/react_runtime/src \
  python -m levier_react_runtime \
  --config execution/react_runtime/react.example.yaml

# Shared MCP Runtime
PYTHONPATH=execution/mcp_runtime/src \
  python -m levier_mcp_runtime --socket /tmp/levier-mcp.sock
```

两个 built-in Runtime 同时就是 Runtime SDK 的接入示例。
