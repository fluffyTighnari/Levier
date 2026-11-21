# Counter App

A minimal App built with the `levier` authoring surface. It owns the counter
state and three local tools; the SDK owns schema inference, credentials, HTTP,
WebSocket, reconnects, tool registration, and local-tool framing.

```text
counter_add    delta -> current value
counter_get    no arguments -> current value
counter_reset  no arguments -> 0
```

## Run

```sh
levier up
levier model connect --url https://api.deepseek.com --key sk-... --model deepseek-chat
levier init --name counter-agent
levier dev
```

`levier dev` finds `app.py`, loads the App credentials for this project, and
serves the tools. In another terminal:

```sh
levier run "Add 2, then add 3, then read the counter."
```

## MCP

Add the server once and declare the reference for this App:

```sh
levier mcp add \
  --name e2e-echo \
  --transport stdio \
  --config-file ./mcp-echo.example.json

levier init --name counter-agent --mcp-ref e2e-echo
```

The App process never opens MCP connections; the shared MCP Runtime does that.

## Code

[`app.py`](app.py) contains only business state, three `@tool` functions, and
the `levier_app` CLI shell. [`tools.py`](tools.py) contains the counter state.

The schema is inferred from the function signature and docstring:

```python
@tool
def counter_add(delta: int) -> dict[str, int]:
    """Add an integer delta to the counter."""
    return {"value": counter.add(delta)}
```
