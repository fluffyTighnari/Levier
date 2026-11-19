# Echo Runtime

The smallest Runtime SDK example.

```sh
PYTHONPATH=execution/runtime_sdk/src \
  python examples/echo_runtime/app.py \
  --config examples/echo_runtime/runtime.yaml
```

It implements only:

```python
async def run(ctx: RunContext) -> str:
    await ctx.reply("echo")
    return "completed"
```

No strategy, LLM, tool router, or MCP client is involved.
