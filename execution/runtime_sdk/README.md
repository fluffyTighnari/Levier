# Levier Runtime SDK

The Runtime SDK is the small execution-side counterpart to the App SDK.

A runtime only needs:

```python
from levier_runtime import Runtime, RunContext

async def run(ctx: RunContext) -> str:
    await ctx.reply("hello")
    return "completed"

Runtime.from_config("runtime.yaml").run(run)
```

The SDK owns the RuntimeChannel, Hello, heartbeat, reconnect, Ack, event
sequence numbers, cancellation, tool-call transport, and graceful shutdown.

`RunContext` is plain Python:

- core: `runtime_name`, `run_id`, `input`, `params`, `timeouts`, `cancelled`
- optional: `model`, `tools`, `mcp_bindings`, `session_id`, `resume_from`
- operations: `reply`, `call_tool`, `error`; optional `emit`, `checkpoint`
- `ctx.logger` automatically carries run/app/runtime/attempt/trace fields
- `ctx.call_tool` automatically emits `TOOL_CALL_BEGIN` / `TOOL_CALL_END`
- uncaught `run(ctx)` exceptions become `ERROR` + a failed terminal run

## Tests

```sh
make runtime-sdk-test
```

The suite includes the optional React/LLM/tool helpers as well as the
transport-free custom-runtime conformance test.

## Optional helpers

The SDK core does not depend on an LLM, a ReAct loop, or a tool router.
Optional subpackages are included for runtimes that want them:

```text
levier_runtime.llm      provider protocol and HTTP providers
levier_runtime.tools    ToolProvider / ToolRouter
levier_runtime.react    ReactAgent and React checkpoint codec
levier_runtime.mcp      shared MCP Runtime client
```

`levier_runtime.react` requires `levier-agent-core`. A runtime that does not
use it does not install or import that dependency.

The MCP runtime is an independent shared process; its optional client lives at
`levier_runtime.mcp` so every runtime can reuse the same protocol implementation.

## Custom runtime example

```python
from levier_runtime import Runtime, RunContext


async def run(ctx: RunContext) -> str:
    await ctx.reply("working")
    result = await ctx.call_tool("search", {"query": "levier"})
    if not result.ok:
        await ctx.error(result.error_code, result.error_message, retryable=True)
        return "failed"
    await ctx.reply(result.result_json)
    return "completed"


Runtime.from_config("runtime.yaml").run(run)
```

The loop, model calls, and tool decisions belong to the runtime author. The
SDK only supplies the protocol, observability, cancellation, and lifecycle
baseline.
