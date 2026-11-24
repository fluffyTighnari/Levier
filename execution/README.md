# execution

执行侧代码。Runtime SDK 负责协议与生命周期；MCP Runtime 独立运行；Echo 和 React 是内置 Runtime，同时也是接入示例。

```text
execution/
  runtime_sdk/     Runtime SDK
                   channel / lifecycle / RunContext / events
                   optional: react / llm / tools / mcp client

  mcp_runtime/     独立 MCP Runtime
                   多个 Runtime 共享同一个进程和 socket

  echo_runtime/    最小内置 Runtime
                   只写 run(ctx)，作为接入示例

  react_runtime/   薄内置 ReAct Runtime
                   配置 + 组合 ReactAgent，不承载协议实现

  agent_core/      可选 ReAct 循环库
                   可被 Runtime 作者复用，不是 Runtime 核心

  langgraph_runtime/ 可选 LangGraph Driver
                     LangGraph 状态、interrupt 和节点事件，不进入核心 SDK
```

## 依赖方向

```text
echo_runtime ─────┐
react_runtime ────┼──> runtime_sdk
langgraph_runtime ┘

react_runtime ──> agent_core
react_runtime ──> mcp_runtime（可选共享进程）

mcp_runtime 独立运行，不依赖 SDK 或具体 Runtime。
```

`runtime_sdk` 的核心只包含 Runtime、RunContext、channel、生命周期、事件、取消、错误和工具传输。`levier_runtime.react`、`levier_runtime.llm`、`levier_runtime.tools`、`levier_runtime.mcp` 是可选 helper；Echo 不导入它们。

## 运行

```sh
# Echo
PYTHONPATH=execution/runtime_sdk/src:execution/echo_runtime/src \
  python -m levier_echo_runtime \
  --config execution/echo_runtime/runtime.yaml

# React
PYTHONPATH=execution/agent_core/src:execution/runtime_sdk/src:execution/react_runtime/src \
  python -m levier_react_runtime \
  --config execution/react_runtime/react.example.yaml

# LangGraph (optional package)
pip install -e execution/langgraph_runtime
PYTHONPATH=execution/runtime_sdk/src:execution/langgraph_runtime/src \
  python -m unittest discover -s execution/langgraph_runtime/tests -v

# Shared MCP Runtime
PYTHONPATH=execution/mcp_runtime/src \
  python -m levier_mcp_runtime --socket /tmp/levier-mcp.sock
```

所有内置 Runtime 都只依赖 Runtime SDK 的协议契约；其中 React 通过可选的 `ReactAgent` 组合 ReAct 能力。
