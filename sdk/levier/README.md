# `levier` Python SDK

One import for both sides of Levier.

## App

```python
from levier import App, tool

counter = {"value": 0}


@tool
def counter_add(delta: int) -> dict:
    """Add delta to the counter."""
    counter["value"] += delta
    return {"value": counter["value"]}


app = App(tools=[counter_add])

if __name__ == "__main__":
    app.run()
```

`@tool` infers the JSON schema from type hints and the description from the
first docstring line. `App` loads credentials, the server address, and MCP refs
from the user profile, and owns the WebSocket lifecycle.

One-shot Run:

```python
app.run("Add 2, then read the counter.")
```

## Runtime

```python
from levier import Context, Runtime


async def run(ctx: Context) -> str:
    tools = await ctx.toolbox()
    reply = await ctx.chat(ctx.input, tools=tools)
    await ctx.invoke_all(reply, toolbox=tools)
    await ctx.reply(reply.content or "done")
    return "completed"


if __name__ == "__main__":
    Runtime.start(run)
```

`Runtime.start()` needs no YAML or runtime token for a local `levier up` stack.
`ctx.toolbox()` includes both local App tools and MCP tools; `ctx.chat()` uses
the model already resolved for the Run; `ctx.invoke_all()` dispatches each tool
call to the right source.

## Install from the repository

```sh
python -m pip install -e ./sdk/python -e ./execution/runtime_sdk -e ./sdk/levier
```
