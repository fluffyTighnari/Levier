# Levier Python App SDK

`levier_app` is the App-side SDK. It owns credentials, HTTP, WebSocket,
reconnects, tool registration, and the `local_tool_call` /
`local_tool_result` contract. An App owns its own state and tool handlers.

## Minimal App

```python
from levier_app import LevierApp

app = LevierApp.from_credentials()


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
    return {"value": arguments["delta"]}


if __name__ == "__main__":
    raise SystemExit(app.run())
```

Install locally:

```sh
pip install -e ./sdk/python
```

Run with the credentials created by `levier app init`:

```sh
LEVIER_CREDENTIALS=~/.config/levier/counter-app.json python app.py
```

## MCP references

MCP tools are not implemented in the App process. The App declares which
registered MCP servers it wants:

```python
app = LevierApp.from_credentials(mcp_refs=["github", "e2e-echo"])
```

or:

```sh
export LEVIER_APP_MCP_REFS=github,e2e-echo
```

The same references can be set when the App is created:

```sh
levier app init --name counter-agent --mcp-ref github,e2e-echo
```

The Runtime resolves the referenced MCP servers into its per-run MCP bindings.
The App does not need any MCP client code.

## Client API

`LevierClient` uses the same credentials file as `LevierApp`:

```python
from levier_app import LevierClient

client = LevierClient.from_credentials()

providers = client.list_model_providers()
run = client.submit_run(
    "Add 2, then add 3.",
    runtime="react1",
    provider="deepseek",
    model="deepseek-flash",
)
result = client.wait_run(run["run_id"], timeout=120)
events = client.get_events(run["run_id"])
```

Public methods:

```text
LevierClient.from_credentials()
LevierClient.list_model_providers()
LevierClient.submit_run(prompt, runtime=..., provider=..., model=...)
LevierClient.wait_run(run_id, timeout=...)
LevierClient.get_run(run_id)
LevierClient.get_events(run_id)
LevierClient.cancel_run(run_id)
```

`provider` and `model` are optional when the control plane has a default
provider. App code never receives or stores the provider API key.

## Credentials

`levier app init` writes a 0600 JSON file containing:

```json
{
  "credential_version": 1,
  "app_id": "...",
  "app_name": "counter-agent",
  "api_key": "...",
  "server": "http://127.0.0.1:8080"
}
```

`LEVIER_CREDENTIALS` selects the file. The App API key is distinct from model
provider keys; provider keys live in the control plane secret configuration.

## Tests

```sh
PYTHONPATH=sdk/python/src python3 -m unittest discover -s sdk/python/tests -v
```
