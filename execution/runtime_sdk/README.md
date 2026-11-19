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
- operations: `reply`, `call_tool`; optional `emit`, `checkpoint`

The SDK intentionally does not contain an LLM, ReAct loop, MCP client, or tool
router. A runtime chooses those capabilities itself.
