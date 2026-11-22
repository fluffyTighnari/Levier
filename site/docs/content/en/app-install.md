---
title: "App SDK, tools and construction"
status: published
description: "Install the App SDK, define tools and construct an App."
updated: 2026-11-21
---

## Install the SDK

### 1. PyPI

```sh
pip install levier
```

An App-only install does not import the Runtime SDK's gRPC dependencies up front; `Context` / `Runtime`
are loaded lazily.

### 2. From source

```sh
cd /path/to/middleware
python -m pip install -e ./sdk/python -e ./execution/runtime_sdk -e ./sdk/levier
```

If you only work on Apps and do not write Runtimes, you can install just the App side:

```sh
python -m pip install -e ./sdk/python -e ./sdk/levier
```

### 3. Version

```sh
python -c "import levier; print(levier.__version__)"
```

Pin the version in your project:

```text
levier==0.1.0
```

### 4. Verify

```python
from levier import App, tool

@tool
def ping() -> dict:
    """Return pong."""
    return {"pong": True}

print(App(tools=[ping]).tools_manifest())
```

If this raises `No module named levier`, the current Python is not the one where the SDK is installed.

## @tool and schema

### 1. @tool

```python
from levier import tool


@tool
def counter_add(delta: int) -> dict:
    """Add delta to the counter."""
    return {"value": delta}
```

`@tool` turns a plain function into a `Tool`:

```text
name         function name; can be overridden by arguments
description  first line of the docstring
schema       generated from the function signature
timeout_ms   defaults to 10000
```

### 2. Type hints

Supported primitive types:

```text
str / int / float / bool
list / dict
Optional[...]
```

For complex objects, prefer `dict` or `list[dict]`; do not rely on automatic serialization of custom classes.

### 3. docstring

The first line is the tool description the model sees:

```python
@tool
def counter_add(delta: int) -> dict:
    """Add an integer delta to the counter."""
```

Keep it to one sentence, start with a verb, and state clearly what the parameters mean.

### 4. Automatic schema

The generated result looks like:

```json
{
  "type": "object",
  "properties": {
    "delta": {"type": "integer"}
  },
  "required": ["delta"],
  "additionalProperties": false
}
```

You can also override it explicitly:

```python
@tool(
    name="counter_add",
    description="Add delta to the counter.",
    schema={
        "type": "object",
        "properties": {"delta": {"type": "integer"}},
        "required": ["delta"],
        "additionalProperties": False,
    },
    timeout_ms=10000,
)
def counter_add(delta: int) -> dict:
    ...
```

### 5. timeout

```python
@tool(timeout_ms=30000)
def slow_tool(...):
    ...
```

Timeouts are mapped to `TOOL_TIMEOUT` by the control plane/gateway. Do not wait indefinitely.

### 6. Tool names

Prefer letters, digits and underscores only:

```text
counter_add
orders_mark_shipped
```

Most model providers do not accept dots, spaces or slashes. Use underscores for namespaces, not dots.

### 7. Testing

```python
tool = counter_add
assert tool.invoke({"delta": 2}) == {"value": 2}
assert tool(delta=2) == {"value": 2}
```

`schema_from_function` lets you inspect the generated result on its own.

## App construction

### 1. Constructor arguments

```python
App(
    tools=(),
    *,
    mcp=None,
    runtime=None,
    credentials=None,
    server=None,
    logger=None,
    on_event=None,
)
```

| Parameter | Description |
|---|---|
| `tools` | List of `Tool` objects or plain functions |
| `mcp` | String or list of strings, MCP references |
| `runtime` | Default runtime_name |
| `credentials` | Explicit credentials path |
| `server` | Explicit control plane address |
| `logger` | Custom logger |
| `on_event` | Event callback |

When `credentials` / `server` are not passed, they are read from the project `.levier.json` and the user profile.

### 2. Defaults

```python
app = App(tools=[counter_add])
```

is equivalent to:

```text
tools        counter_add
mcp          mcp_refs from profile / .levier.json
runtime      runtime from profile / .levier.json
credentials  credentials from profile / .levier.json
server       server from profile / .levier.json
```

### 3. App.run

```python
# serve tool calls, blocking
app.run()

# one-shot submit and print the final message
app.run("Add 2 and 3.")

# specify a runtime
app.run("hello", runtime="echo1")

# do not print the final message
app.run("hello", print_output=False)
```

Signature:

```text
App.run(prompt=None, *, runtime=None, provider="", model="",
        max_steps=8, run_ms=60000, tool_ms=10000,
        timeout=300.0, print_output=True)
```

### 4. App.serve

```python
app.serve()
```

Connects to Levier and blocks while serving tool calls. `app.run()` without a prompt is equivalent to `serve()`.

To stop:

```python
app.stop()
```

### 5. Dynamic registration

```python
app = App()

@app.tool
def ping() -> dict:
    """Return pong."""
    return {"pong": True}

app.run()
```

Also supported:

```python
@app.tool("ping")
def ping_tool() -> dict:
    ...

@app.tool(name="ping", timeout_ms=5000)
def ping_tool() -> dict:
    ...
```

### 6. Queries

```text
app.tools_manifest()
app.list_model_providers()
app.submit_run(...)
app.wait_run(...)
app.get_run(...)
app.get_events(...)
app.cancel_run(...)
```

See [Submit a run from the SDK](app-runs.md) for details.

## Next steps

- App core: [App core workflow](app-core.md)
- Initialize and run: [Initialize and run an App](app-dev.md)
- Runs: [Runs](app-runs.md)
