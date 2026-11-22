---
title: Full setup
status: published
description: The shortest complete path from install to your first run, with links into each management surface.
updated: 2026-11-21
---

> [!NOTE]
> This page walks a single complete path. Command flags, error handling and further capabilities live in the links at the end of each section.

## 1. Install the CLI

Release build:

```sh
curl -fsSL https://raw.githubusercontent.com/fluffyTighnari/levier/main/scripts/install.sh | sh
```

Develop from source:

```sh
make install
levier setup --server http://127.0.0.1:8080
```

Confirm the command is available:

```sh
levier version
```

> [!OUTPUT]
> ```text
> levier v0.1.0
> ```

Details: [Install and upgrade](levier-install.md)

## 2. Start

```sh
levier up
```

> [!OUTPUT]
> ```text
> waiting for http://127.0.0.1:8080 ...
> levier is ready
> ```

```sh
levier status
```

> [!OUTPUT]
> `health`, `ready`, `database` and `daemons` are all healthy:
>
> ```text
> status      ok
> health      ok
> ready       ok
> database    up
> daemons     connected=2 healthy=2 capacity=8 used=0
> ```

Stop:

```sh
levier down
```

Details: [Start the stack](levier-install.md) · [Status](levier-status.md) · [Stop the stack](levier-status.md)

## Optional setup: models

You only need three things:

```text
URL
API key
model name
```

Run:

```sh
levier model connect \
  --url https://api.deepseek.com \
  --key sk-... \
  --model deepseek-chat
```

> [!OUTPUT]
> ```text
> model connected: deepseek
> default:          true
> ```

Verify:

```sh
levier model list
levier model check --name deepseek
```

> [!OUTPUT]
> ```text
> provider check: PASS deepseek
> ```

Details: [Model connection](pre-model.md) · [Verify a model](pre-model.md)

## Optional setup: MCP

```sh
levier mcp add \
  --name echo \
  --transport stdio \
  --config-file ./examples/mcp_echo/config.json
```

Verify:

```sh
levier mcp list
```

> [!OUTPUT]
> ```text
> name                 transport          secret_refs
> echo                 stdio
> ```

Details: [MCP servers](pre-mcp.md) · [MCP config and allowed_tools](pre-mcp.md)

## 3. Connect an App

Install the SDK:

```sh
pip install levier
```

Add `app.py` to the project:

```python
from levier import App, tool

counter = {"value": 0}


@tool
def counter_add(delta: int) -> dict:
    """Add delta to the counter."""
    counter["value"] += delta
    return {"value": counter["value"]}


@tool
def counter_get() -> dict:
    """Read the counter."""
    return {"value": counter["value"]}


app = App(tools=[counter_add, counter_get])


if __name__ == "__main__":
    app.run()
```

Run this in the project directory:

```sh
levier init
```

> [!OUTPUT]
> ```text
> ready: counter-agent
> app:     app_01...
> server:  http://127.0.0.1:8080
> runtime: react1
> ```

Start the App:

```sh
levier dev
```

Submit a run from another terminal:

```sh
levier run "Add 2, then add 3, then read the counter."
```

> [!OUTPUT]
> ```text
> run submitted: 01J...
> status: queued
> status: completed
> runtime_name: react1
> final: 5
> ```

You can also let the CLI start the App temporarily:

```sh
levier run "Add 2, then add 3, then read the counter." --with-app
```

One-off submit from the SDK:

```python
app.run("Add 2, then add 3, then read the counter.")
```

Details: [App core workflow](app-core.md) · [@tool and automatic schema](app-install.md) · [Initialize a project](app-dev.md) · [dev and serve](app-dev.md) · [Submit a run from the CLI](app-runs.md) · [Submit a run from the SDK](app-runs.md) · [Errors and reconnects](app-mcp.md)

More App capabilities: [App self-check](app-manage.md) · [Manage apps](app-manage.md) · [Runs and events](app-runs.md) · [Reference MCP servers](app-mcp.md)

## 4. Connect a Runtime

The local stack already includes:

```text
react1    built-in ReAct Runtime
echo1     minimal Echo Runtime
```

Generate a template:

```sh
levier runtime init
```

> [!OUTPUT]
> ```text
> created runtime.py
> start it with: levier runtime start
> ```

Edit the generated `runtime.py`:

```python
from levier import Context, Runtime


async def run(ctx: Context) -> str:
    tools = await ctx.toolbox()

    # assemble the prompt however you want
    reply = await ctx.chat(ctx.input, tools=tools)

    # run every tool requested by the model; local/MCP is routed automatically
    await ctx.invoke_all(reply, toolbox=tools)

    await ctx.reply(reply.content or "done")
    return "completed"


if __name__ == "__main__":
    Runtime.start(run)
```

Start it:

```sh
levier runtime start
```

> [!OUTPUT]
> ```text
> runtime: counter-runtime
> file:    /path/to/runtime.py
> server:  127.0.0.1:9090
> ```

Uncaught exceptions in the Runtime automatically become:

```text
ERROR event
Run failed
```

When you need an explicit error:

```python
await ctx.error("MY_ERROR", "something failed", retryable=False)
```

Tool errors are already in `ToolResult`:

```python
result = await ctx.call_tool("search", {"query": "levier"})
if not result.ok:
    await ctx.error(result.error_code, result.error_message)
```

Details: [Runtime core workflow](runtime-core.md) · [runtime init](runtime-start.md) · [runtime start](runtime-start.md) · [Errors and cancellation](runtime-errors.md)

More Runtime capabilities: [Built-in runtimes](runtime-builtin.md) · [Context](runtime-context.md) · [Toolbox](runtime-toolbox.md) · [chat and invoke](runtime-chat.md) · [Checkpoint](runtime-errors.md) · [Instances and capacity](runtime-remote.md) · [Remote deployment and YAML](runtime-remote.md)

## 5. Observability

```sh
levier status
```

```sh
levier run show <run_id> --events
```

```sh
levier runtime list
```

```sh
levier logs --follow
```

Details: [Status](levier-status.md) · [Inspect and cancel runs](levier-run.md) · [Runtime list](levier-runtime-list.md) · [Logs and processes](levier-status.md)

## 6. More

App management:

```text
app list / app show / app rotate-key
get run / wait / get events / cancel run
```

Details: [Manage apps](app-manage.md) · [Runs and events](app-runs.md)

Levier operations:

```text
doctor / describe / selfcheck
config / migrate / maintenance
ops runs / ops daemons / ops tasks
```

Details: [doctor / describe / selfcheck](levier-doctor.md) · [config / migrate / maintenance](levier-doctor.md) · [Operations view](levier-ops.md) · [Remote control plane](levier-remote.md)

## 7. Stop

```sh
levier down
```

Details: [Stop the stack](levier-status.md)
