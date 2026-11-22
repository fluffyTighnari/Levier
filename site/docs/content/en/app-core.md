---
title: App core workflow
status: published
description: Install, tools, init, dev, run and error handling in one loop.
updated: 2026-11-21
---

> [!NOTE]
> This is the App-side trunk. Parameter details, SDK methods and App management commands are covered on the following pages.

## 1. Install the SDK

```sh
pip install levier
```

From source:

```sh
python -m pip install -e ./sdk/python -e ./execution/runtime_sdk -e ./sdk/levier
```

## 2. Writing tools

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

`@tool` generates JSON Schema from the type annotations and takes the description from the first line of the docstring.
You do not need to write schemas by hand.

## 3. Project initialization

```sh
levier init
```

> [!OUTPUT]
> ```text
> ready: counter-agent
> app:     app_01...
> server:  http://127.0.0.1:8080
> runtime: react1
> next:
>   levier dev            # serve this project's tools
>   levier run "hello"    # submit a run without flags
> ```

`init` writes the user profile, the project `.levier.json` and a default 0600 credentials file.

## 4. Start the App

```sh
levier dev
```

It looks for `./app.py` / `./levier_app.py` and injects credentials, the server and MCP references.
The process must stay online; tool handlers execute inside the App process.

You can also run:

```sh
python app.py
```

The effect is the same.

## 5. Submit a run

In another terminal:

```sh
levier run "Add 2, then add 3, then read the counter."
```

Start the App temporarily with a single command:

```sh
levier run "Add 2, then add 3, then read the counter." --with-app
```

One-shot submission from the SDK:

```python
app.run("Add 2, then add 3, then read the counter.")
```

> [!OUTPUT]
> ```text
> run submitted: 01J...
> status: queued
> status: completed
> runtime_name: react1
> final: 5
> ```

## 6. Error handling

A handler can raise a structured error directly:

```python
from levier import ToolExecutionError


@tool
def my_tool(value: str) -> dict:
    if not value:
        raise ToolExecutionError("BAD_INPUT", "value is required")
    return {"ok": True}
```

The SDK turns it into a tool result and does not let the WebSocket crash.

For tool failures, check first:

```sh
levier run show <run_id> --events
```

Key events:

```text
TOOL_CALL_BEGIN
TOOL_CALL_END
ERROR
```

## Next steps

- Installation and versions: [Install the App SDK](app-install.md)
- Tools and schema: [@tool and automatic schema](app-install.md)
- Constructor arguments: [App construction](app-install.md)
- Initialization: [Initialize a project](app-dev.md)
- Starting: [dev and serve](app-dev.md)
- Submit a run: [Submit a run from the CLI](app-runs.md) · [Submit a run from the SDK](app-runs.md)
- Errors and reconnects: [Errors and reconnects](app-mcp.md)
