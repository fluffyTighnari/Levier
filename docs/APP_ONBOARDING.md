# App onboarding

This page is the App-side onboarding guide. The full two-sided walkthrough is
in the repository [`README.md`](../README.md); the SDK reference is
[`sdk/python/README.md`](../sdk/python/README.md).

## 1. What an App owns

An App owns:

- business state
- local tool handlers
- tool JSON schemas
- its own UI/API entry point

The SDK owns:

- reading the App credentials file
- HTTP authentication
- WebSocket lifecycle and reconnect
- tool registration
- `local_tool_call` / `local_tool_result`
- tool error mapping

## 2. Register App credentials

```sh
export LEVIER_ADMIN_TOKEN=admin-change-me

levier app init \
  --name counter-agent \
  --server http://127.0.0.1:8080 \
  --credentials-out ./data/agent/credentials.json
```

For MCP tools, register the server first and add a reference:

```sh
levier mcp add \
  --name e2e-echo \
  --transport stdio \
  --config-file ./mcp-echo.json

levier app init \
  --name counter-agent \
  --mcp-ref e2e-echo \
  --credentials-out ./data/agent/credentials.json
```

The credentials file is 0600. The App API key is separate from any model
provider key.

## 3. Register local tools

```python
from levier_app import LevierApp

counter = {"value": 0}


def register_tools(app):
    @app.tool(
        "counter_add",
        description="Add an integer to the counter.",
        schema={
            "type": "object",
            "properties": {"delta": {"type": "integer"}},
            "required": ["delta"],
            "additionalProperties": False,
        },
    )
    def counter_add(arguments):
        counter["value"] += arguments["delta"]
        return {"value": counter["value"]}

    @app.tool("counter_get", schema={"type": "object", "properties": {}})
    def counter_get(_arguments):
        return {"value": counter["value"]}


if __name__ == "__main__":
    raise SystemExit(LevierApp.run_cli(register_tools))
```

MCP references can also be declared in code or via environment:

```python
app = LevierApp.from_credentials(mcp_refs=["e2e-echo"])
```

```sh
export LEVIER_APP_MCP_REFS=e2e-echo
```

## 4. Submit a Run

Use `LevierClient` with the same credentials file:

```python
from levier_app import LevierClient

client = LevierClient.from_credentials()

providers = client.list_model_providers()
run = client.submit_run(
    "Add 2, then add 3, then read the counter.",
    runtime="react1",
    provider="deepseek",
    model="deepseek-flash",
)
result = client.wait_run(run["run_id"], timeout=120)
events = client.get_events(run["run_id"])
```

Or use the CLI:

```sh
levier run submit \
  --credentials ./data/agent/credentials.json \
  --runtime react1 \
  --provider deepseek \
  --model deepseek-flash \
  --prompt "Add 2, then add 3, then read the counter." \
  --watch
```

## 5. Public API key vs model key

| Key | Used by | Managed by |
|---|---|---|
| App API key `mw_...` | App HTTP/WS | `levier app init` |
| Model provider key | control plane -> Runtime | `levier provider` + secret file/env |
| Daemon token | Runtime -> control plane | deployment config |

App code never receives the model provider key.

## 6. Useful commands

```sh
levier app show --credentials ./data/agent/credentials.json
levier app check --credentials ./data/agent/credentials.json
levier app rotate-key --credentials ./data/agent/credentials.json --yes
levier run show --run-id <id> --events --credentials ./data/agent/credentials.json
```
