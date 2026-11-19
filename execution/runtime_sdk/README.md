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

The SDK intentionally does not contain an LLM, ReAct loop, or tool router.
The MCP runtime is an independent shared process; its optional client lives at
`levier_runtime.mcp` so every runtime can reuse the same protocol implementation.
