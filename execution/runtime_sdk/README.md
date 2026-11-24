# Levier Runtime SDK

The Runtime SDK is the small execution-side counterpart to the App SDK.

A runtime only needs:

```python
from levier_runtime import Context, Runtime

async def run(ctx: Context) -> str:
    await ctx.reply("hello")
    return "completed"

Runtime.start(run)
```

`Runtime.start()` needs no YAML and no explicit runtime token for a local
`levier up` stack. The higher-level `levier` package exposes the same objects
as `from levier import Context, Runtime`.

The SDK owns the RuntimeChannel, Hello, heartbeat, reconnect, Ack, event
sequence numbers, cancellation, tool-call transport, and graceful shutdown.

`RunContext` is plain Python:

- core: `runtime_name`, `run_id`, `input`, `params`, `timeouts`, `cancelled`
- optional: `model`, `tools`, `mcp_bindings`, `session_id`, `resume_from`
- operations: `reply`, `call_tool`, `error`; optional `emit`, `checkpoint`
- one-liners: `await ctx.toolbox()`, `await ctx.chat(...)`,
  `await ctx.invoke_all(reply, toolbox=...)`

`ctx.toolbox()` merges App, global, MCP, and Runtime-local tools into one
callable sequence. `ctx.chat()` uses the model already resolved for the Run.
`ctx.invoke_all()` dispatches model tool calls to the right source and emits
the same tool observability events.
- `ctx.logger` automatically carries run/app/runtime/attempt/trace fields
- `ctx.call_tool` automatically emits `TOOL_CALL_BEGIN` / `TOOL_CALL_END`
- uncaught `run(ctx)` exceptions become `ERROR` + a failed terminal run

## Runtime-local tools

A Runtime can register tools that live and execute inside its own process.
They join the same model-facing tool list, but never cross the App Gateway:

```python
from levier_runtime import Context, Runtime

async def run(ctx: Context) -> str:
    @ctx.tool(
        "lookup_order",
        description="Look up one order.",
        schema={
            "type": "object",
            "properties": {"order_id": {"type": "string"}},
            "required": ["order_id"],
        },
    )
    async def lookup_order(arguments):
        return {"order_id": arguments["order_id"], "status": "paid"}

    result = await ctx.call_tool("lookup_order", {"order_id": "o-1"})
    await ctx.reply(result.result_json)
    return "completed"

Runtime.start(run)
```

Register Runtime-local tools before opening `ctx.toolbox()`, `ctx.chat()`, or
`ctx.invoke()`. Names must not collide with control-plane tools. A Runtime that
advertises `RuntimeCapabilities(runtime_tools=True)` makes the capability
visible in the daemon handshake.

## Runtime Driver

The simple `run(ctx)` entry point remains the default. A Runtime Driver is an
optional composition layer for runtimes that need capability declarations or
startup/shutdown hooks:

```python
from levier_runtime import Runtime, RuntimeCapabilities


class MyDriver:
    capabilities = RuntimeCapabilities(checkpoint=True, custom_events=True)

    async def on_start(self, runtime):
        ...

    async def on_shutdown(self):
        ...

    async def run(self, ctx):
        ...
        return "completed"


Runtime.start_driver(MyDriver())
```

The driver does not replace the transport or lifecycle layer. The SDK still
owns the RuntimeChannel, heartbeat, reconnect, cancellation, events, and tool
transport. Capabilities are declarative metadata used by the daemon handshake
and admin views. Model providers are resolved by the control-plane catalog; a
Runtime that implements its own model integration can advertise
`RuntimeCapabilities(providers=True)` and use its own provider client.

## Tests

```sh
make runtime-sdk-test
```

The suite includes the optional React/LLM/tool helpers as well as the
transport-free custom-runtime conformance test.

## Optional helpers

The SDK core does not depend on an LLM or a ReAct loop. Tool routing is built
into `ctx.toolbox()`.
Optional subpackages are included for runtimes that want them:

```text
levier_runtime.llm      provider protocol and HTTP providers
levier_runtime.tools    low-level ToolProvider interface
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
