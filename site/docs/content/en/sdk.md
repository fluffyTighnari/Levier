---
title: SDK reference
status: published
description: "Grouped by task: App, Runtime, tools, runs, config and install."
updated: 2026-11-21
---

> [!NOTE]
> This page is organized by "what you want to do", not by package
> hierarchy. Most projects only use `from levier import ...`.

## 1. Choose by task

| What you want to do | Use |
|---|---|
| Connect business functions as tools | `App` + `@tool` |
| Submit/query runs | `App.submit_run` / `wait_run` / `get_events` |
| Write your own execution loop | `Runtime` + `run(ctx)` |
| Call a model or tools | `ctx.chat` / `ctx.toolbox` |
| Control credentials/transport explicitly | `levier_app.LevierApp` |
| Read configuration and secrets | profile / `.levier.json` / environment variables |

## 2. App integration

```python
from levier import App, tool

@tool
def counter_add(delta: int) -> dict:
    """Add delta to the counter."""
    return {"value": delta}

app = App(tools=[counter_add])
app.run()
```

Details: [App core workflow](app-core.md) · [App construction](app-install.md) · [Tools and schema](app-install.md)

## 3. Runtime integration

```python
from levier import Context, Runtime

async def run(ctx: Context) -> str:
    tools = await ctx.toolbox()
    reply = await ctx.chat(ctx.input, tools=tools)
    await ctx.invoke_all(reply, toolbox=tools)
    await ctx.reply(reply.content or "done")
    return "completed"

Runtime.start(run)
```

Details: [Runtime core workflow](runtime-core.md) · [Context](runtime-context.md) · [Toolbox](runtime-toolbox.md)

## 4. Tools and schema

```text
@tool
app.tool
App.add
Tool.invoke
Tool.__call__
schema_from_function
ToolExecutionError
```

You do not need to hand-write JSON Schema; pass `schema=` explicitly when you need to override it.

Details: [Tools and schema](app-install.md)

## 5. Runs and events

```text
App.submit_run
App.wait_run
App.get_run
App.get_events
App.cancel_run
```

Events:

```text
RUN_STATUS / MESSAGE_DELTA / MESSAGE_COMPLETED
TOOL_CALL_BEGIN / TOOL_CALL_END / ERROR / USAGE
```

Details: [Submitting a run from the SDK](app-runs.md) · [Runs and events](app-runs.md)

## 6. Configuration and credentials

```text
User profile
Project .levier.json
Environment variables
managed secret
App credentials file
```

By default the App reads from the profile / `.levier.json`; by default the Runtime reads from the profile / environment variables.

Details: [Configuration and precedence](pre-env.md) · [Credentials and secrets](pre-env.md)

## 7. Installation and versions

```sh
pip install levier
```

From source:

```sh
python -m pip install -e ./sdk/python -e ./execution/runtime_sdk -e ./sdk/levier
```

An App-only install does not load the Runtime gRPC dependencies immediately.

Details: [Install the SDK](app-install.md) · [Runtime installation](runtime-start.md)

## 8. Low-level packages

Use only when you need explicit control:

```text
levier_app.LevierApp       explicit credentials/transport/tool registration
levier_runtime             Runtime low-level API
levier_runtime.react       optional ReAct helper
```

Prefer `from levier import ...` for everyday integration.

## Next steps

- CLI: [CLI reference](cli.md)
- Environment variables: [Environment variables](env.md)
- Terms: [Glossary](glossary.md)
