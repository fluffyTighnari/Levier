# Plan & Execute with the Counter App

This walkthrough uses the existing Counter App and React Runtime. It shows the
complete public surface: provider registration, App credentials, tools, a Run,
and the expected plan/tool sequence.

## 1. Start Levier

```sh
cp .env.example .env
docker compose up --build -d postgres levier mcp-runtime react-runtime echo-runtime
bin/levier status
```

For local development, start the control plane and runtimes separately; see the
repository `README.md`.

## 2. Register a model provider

Keep the provider key out of App and Runtime code:

```sh
export LEVIER_ADMIN_TOKEN=admin-change-me
export LEVIER_SECRET_DEEPSEEK_API_KEY=sk-...

levier provider add \
  --name deepseek \
  --adapter openai_compatible \
  --base-url https://api.deepseek.com \
  --default-model deepseek-flash \
  --models deepseek-flash,deepseek-reasoner \
  --secret-ref deepseek_api_key \
  --default
```

Check it:

```sh
levier provider check --name deepseek
```

## 3. Create the App

```sh
levier app init \
  --name counter-agent \
  --server http://127.0.0.1:8080 \
  --credentials-out ./data/agent/credentials.json
```

This creates a 0600 credentials file. The App API key is not a model key.

## 4. Start the App

The example exposes three local tools:

```text
counter_add
counter_get
counter_reset
```

```sh
LEVIER_CREDENTIALS=./data/agent/credentials.json \
  python3 examples/counter_agent/app.py
```

The App connects to Levier and registers the tool manifest. It does not call a
model and does not contain a ReAct loop.

## 5. Submit a plan-and-execute prompt

```sh
levier run submit \
  --credentials ./data/agent/credentials.json \
  --runtime react1 \
  --provider deepseek \
  --prompt "Add 2, then add 3, then read the counter." \
  --watch
```

The intended model behavior is:

```text
1. plan: add 2
2. tool: counter_add({"delta": 2})
3. plan: add 3
4. tool: counter_add({"delta": 3})
5. plan: read the counter
6. tool: counter_get({})
7. final answer: 5
```

You can see the tool events without waiting for a live model:

```sh
levier run show --run-id <run_id> --events \
  --credentials ./data/agent/credentials.json
```

Expected event types include:

```text
RUN_STATUS
MESSAGE_DELTA
MESSAGE_COMPLETED
TOOL_CALL_BEGIN
TOOL_CALL_END
RUN_STATUS
```

## 6. Submit from Python

```python
from levier_app import LevierApp

app = LevierApp.from_credentials()

providers = app.list_model_providers()
run = app.submit_run(
    "Add 2, then add 3, then read the counter.",
    runtime="react1",
    provider="deepseek",
    model="deepseek-flash",
)
result = app.wait_run(run["run_id"], timeout=120)
print(result["status"])
```

## 7. MCP variant

If the plan needs an MCP tool:

```sh
levier mcp add \
  --name e2e-echo \
  --transport stdio \
  --config-file ./examples/counter_agent/mcp-echo.example.json

levier app init \
  --name mcp-counter-agent \
  --mcp-ref e2e-echo \
  --credentials-out ./data/agent/mcp-credentials.json
```

Run the MCP Runtime with the same socket used by the React Runtime:

```sh
PYTHONPATH=execution/mcp_runtime/src \
  python -m levier_mcp_runtime \
  --socket /tmp/levier-mcp.sock
```

The model sees the MCP tool as:

```text
mcp__e2e_echo__echo_token
```

The App still only owns its local Counter tools. MCP tool execution stays in
`mcp_runtime`.
