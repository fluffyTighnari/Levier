# Runtime onboarding（详细版）

> 黄金路径见 [`QUICKSTART.md`](QUICKSTART.md)。最小写法是
> `Runtime.start(run)`，不需要 YAML 或显式 runtime token。
>
> A runtime is one process with one `runtime_name` and one `run(ctx)` function.
>
> The Runtime SDK owns RuntimeChannel, Hello, heartbeat, reconnect, Ack,
> cancellation, event sequencing, tool transport, and graceful shutdown.
> Runtime authors write only the loop and the business decisions.

---

## 1. Minimal runtime

```python
from levier_runtime import RunContext, Runtime


async def run(ctx: RunContext) -> str:
    await ctx.reply("echo")
    return "completed"


if __name__ == "__main__":
    Runtime.start(run)
```

`Runtime.start()` needs no YAML; it uses the user profile written by `levier up`
and the environment. `Runtime.from_config()` remains available when you want an
explicit YAML file.

## 2. Configuration

```yaml
middleware:
  address: "127.0.0.1:9090"
  tls: false
  auth_token_file: "/run/secrets/runtime_token"

runtime:
  name: "react1"
  instance_id: "react-local-1"
  type: "normal"
  max_concurrent_runs: 4
  labels:
    pool: "local"

logging:
  level: "info"
```

- `runtime.name` is the dispatch target used by apps.
- `runtime.instance_id` is this process's identity.
- `runtime.type` is `normal`, `sandbox`, or `longterm`.
- Capacity and labels are runtime-owned.
- Configuration has no `strategy` field.

## 3. `RunContext`

### Core

```text
ctx.runtime_name
ctx.run_id
ctx.input
ctx.params
ctx.timeouts
ctx.cancelled
```

### Optional inputs

```text
ctx.model
ctx.tools
ctx.mcp_bindings
ctx.session_id
ctx.resume_from
```

### Operations

```text
await ctx.reply(text)
await ctx.call_tool(name, arguments, timeout_ms=...)
await ctx.error(code, message, retryable=False)
```

`ctx.call_tool` emits `TOOL_CALL_BEGIN` / `TOOL_CALL_END` automatically. It owns
the invocation id, duration, result, and tool failure fields. A runtime that
already emits these events itself can pass `emit_events=False`.

The SDK maps an uncaught `run(ctx)` exception to `ERROR` plus a failed terminal
run. `ctx.error` is the explicit error event entry point.

`ctx.logger` automatically carries run, app, runtime, attempt, and trace fields.

### Optional operations

```text
await ctx.emit(event_type, data, ephemeral=False)
await ctx.checkpoint(state)
```

`ctx.reply` generates message IDs and event sequence numbers in the SDK.
`ctx.call_tool` returns a plain `ToolResult`, not protobuf.

## 4. Optional inputs

- **Model**: read `ctx.model`; the control plane resolves the registered provider,
  base URL, and key into this run. Runtime code does not need a model key in its
  process environment when using the managed provider catalog.
- **Tools**: read `ctx.tools`; call them through `ctx.call_tool`.
- **MCP**: read `ctx.mcp_bindings`; open sessions via `levier_runtime.mcp` or
  your own client.
- **Longterm**: use `ctx.session_id` / `ctx.resume_from`, and save state with
  `ctx.checkpoint`.
- **Streaming**: use `ctx.emit("MESSAGE_DELTA", ...)`.

## 5. Custom ReAct loop

A custom loop can be written without React, an LLM framework, or the SDK's
optional helpers:

```python
async def run(ctx: RunContext) -> str:
    while not ctx.cancelled:
        answer = await call_your_model(ctx)
        if answer.tool_name:
            result = await ctx.call_tool(
                answer.tool_name,
                answer.arguments,
            )
            if not result.ok:
                await ctx.error(result.error_code, result.error_message)
                return "failed"
            continue
        await ctx.reply(answer.text)
        return "completed"
    return "cancelled"
```

The SDK guarantees the transport and observability baseline; the loop remains
yours.

## 6. Optional authoring helpers

The core does not import these:

```text
levier_runtime.react    ReactAgent and React checkpoint codec
levier_runtime.llm      provider protocol and HTTP providers
levier_runtime.tools    low-level ToolProvider interface
levier_runtime.mcp      shared MCP Runtime client used by ctx.toolbox()
```

The built-in React Runtime is the composition example:

```python
from levier_runtime.llm import ProviderRouter
from levier_runtime.react import ReactAgent
from levier_runtime.runtime import Runtime

runtime = Runtime.from_config("react.example.yaml")
runtime.run(ReactAgent(ProviderRouter()))
```

`ReactAgent` uses `ctx.toolbox()` for MCP and `ctx.call_tool` for local tools. It
does not need an MCP socket, YAML section, or separate tool router.

## 7. Verify a custom runtime

The SDK conformance test in
`execution/runtime_sdk/tests/test_custom_runtime.py` demonstrates a runtime
with no React import. It verifies:

- automatic tool-call events
- event ordering
- terminal status
- manager-level error fallback

A custom runtime should not need to reimplement any of those protocol
mechanics to conform.

## 8. Non-goals

The Runtime SDK core does not include:

```text
LLM providers
ReAct loop
MCP client
Strategy / StrategyRouter
Runtime Admin API
```

`agent_core` remains an optional loop library, not the Runtime core.
