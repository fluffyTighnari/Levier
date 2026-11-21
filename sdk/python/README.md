# Levier Python App SDK

`levier_app` is the low-level App adapter. The preferred authoring surface is
the `levier` package, which re-exports `App` and `tool` and infers the JSON
schema from a normal Python signature:

```python
from levier import App, tool

counter = {"value": 0}


@tool
def counter_add(delta: int) -> dict:
    """Add delta to the counter."""
    counter["value"] += delta
    return {"value": counter["value"]}


app = App(tools=[counter_add], mcp=["github"])

if __name__ == "__main__":
    app.run()
```

The sections below document the lower-level `levier_app` adapter, which remains
available for existing integrations.

## Minimal App

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

Install locally from the repository:

```sh
pip install -e ./sdk/python
```

Or install a release wheel from any directory:

```sh
pip install levier_app-0.1.0-py3-none-any.whl
```

`LevierApp.run_cli(register_tools)` owns credential loading, argparse, logging,
signals, and reconnect settings. Use `--credentials`, `--server`, `--mcp-ref`,
and the standard runtime options instead of writing a bootstrap script.

Set the default once, then run from any directory:

```sh
levier setup --server http://127.0.0.1:8080 --credentials ~/.config/levier/counter-app.json
python app.py
```

An explicit `LEVIER_CREDENTIALS` still overrides the profile:

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

## Run API

`LevierApp` exposes the Run operations directly. The HTTP client used inside
the SDK is not part of the App authoring surface.

```python
from levier_app import LevierApp

app = LevierApp.from_credentials()

providers = app.list_model_providers()
run = app.submit_run(
    "Add 2, then add 3.",
    runtime="react1",
    provider="deepseek",
    model="deepseek-flash",
)
result = app.wait_run(run["run_id"], timeout=120)
events = app.get_events(run["run_id"])
```

Public methods:

```text
LevierApp.list_model_providers()
LevierApp.submit_run(prompt, runtime=..., provider=..., model=...)
LevierApp.wait_run(run_id, timeout=...)
LevierApp.get_run(run_id)
LevierApp.get_events(run_id)
LevierApp.cancel_run(run_id)
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
