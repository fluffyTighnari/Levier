# Counter App

A minimal App built with `levier_app`. It owns the counter state and three
local tools; the SDK owns credentials, HTTP, WebSocket, reconnects, tool
registration, and local-tool framing.

```text
counter_add    delta -> current value
counter_get    no arguments -> current value
counter_reset  no arguments -> 0
```

## 1. Install

```sh
python3 -m venv .venv
.venv/Scripts/python -m pip install -e ./sdk/python
```

Use `.venv/bin/python` on Linux/macOS.

## 2. Register a model provider

The control plane resolves provider keys; the App and Runtime only pass a
provider name.

```sh
export LEVIER_ADMIN_TOKEN=admin-change-me
export LEVIER_SECRET_DEEPSEEK_API_KEY=sk-...

levier provider add \
  --name deepseek \
  --adapter openai_compatible \
  --base-url https://api.deepseek.com \
  --default-model deepseek-flash \
  --secret-ref deepseek_api_key \
  --default
```

## 3. Register the App

Start Levier first (Compose or local), then:

```sh
export LEVIER_ADMIN_TOKEN=admin-change-me

levier app init \
  --name counter-agent \
  --server http://127.0.0.1:8080 \
  --credentials-out ./data/agent/credentials.json
```

For MCP tools, add the server references at creation time:

```sh
levier mcp add \
  --name e2e-echo \
  --transport stdio \
  --config-file ./mcp-echo.example.json

levier app init \
  --name counter-agent \
  --mcp-ref e2e-echo \
  --credentials-out ./data/agent/credentials.json
```

The App can also declare MCP references through the SDK:

```python
app = LevierApp.from_credentials(mcp_refs=["e2e-echo"])
```

or via `LEVIER_APP_MCP_REFS=e2e-echo`.

## 4. Run the App

```sh
LEVIER_CREDENTIALS=./data/agent/credentials.json \
  python3 examples/counter_agent/app.py
```

The App connects to Levier, registers its tools and MCP refs, and waits for
`local_tool_call` messages.

## 5. Submit a Run

Using the CLI:

```sh
levier run submit \
  --credentials ./data/agent/credentials.json \
  --runtime react1 \
  --prompt "Add 2, then add 3, then read the counter." \
  --watch
```

If the control plane has a default provider, `--provider` and `--model` can be
omitted. Otherwise:

```sh
levier run submit \
  --credentials ./data/agent/credentials.json \
  --runtime react1 \
  --provider deepseek \
  --model deepseek-flash \
  --prompt "Add 2, then add 3, then read the counter." \
  --watch
```

## 6. Files

```text
app.py       LevierApp, tool schemas, handlers, process entry point
tools.py     Counter business state (no transport/protocol code)
tests/       Counter behavior and SDK dispatch tests
```

## 7. Tests

```sh
make counter-agent-test
```
