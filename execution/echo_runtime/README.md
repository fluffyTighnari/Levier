# Echo Runtime

Built-in minimal Runtime SDK example.

```sh
PYTHONPATH=execution/runtime_sdk/src \
  python -m levier_echo_runtime \
  --config execution/echo_runtime/runtime.yaml
```

It implements only:

```python
async def run(ctx: RunContext) -> str:
    await ctx.reply("echo")
    return "completed"
```

The package is also a Runtime SDK integration example. It has no LLM, strategy,
tool router, or MCP client.
