---
title: "Runs"
status: published
description: "Submit runs from the CLI or SDK, then inspect state and events."
updated: 2026-11-21
---

## From the CLI

### 1. levier run

```sh
levier run "Add 2, then add 3, then read the counter."
```

Default behaviour:

```text
Use the current project's App
Use the default runtime
Use the default provider / model
Keep watching until a terminal state
Print the final assistant message
```

> [!OUTPUT]
> ```text
> run submitted: 01J...
> status: queued
> status: completed
> runtime_name: react1
> final: 5
> ```

### 2. Arguments

| Argument | Description |
|---|---|
| `--runtime` | runtime_name to use |
| `--provider` / `--model` | Override the default model |
| `--max-steps` | Maximum ReAct steps |
| `--run-ms` | Run timeout |
| `--tool-ms` | Single-tool timeout |
| `--no-watch` | Submit only, do not wait |
| `--with-app` | Start `./app.py` temporarily for the run |
| `--app` | App file to use; implies `--with-app` |
| `--python` | Python used by `--with-app` |
| `--json` | Machine-readable output |
| `--credentials` / `--server` | Explicitly override configuration |

Flags may appear before or after the prompt.

### 3. --with-app

```sh
levier run "Add 2 and 3." --with-app
```

If an App is already connected:

```text
using the already-connected App
```

It is not started twice, so the same tool is not executed twice.

### 4. --no-watch

```sh
levier run "hello" --no-watch
```

It only returns:

```text
run submitted: 01J...
status: queued
```

Afterwards use:

```sh
levier run watch <run_id>
levier run show <run_id> --events
```

### 5. Low-level commands

Still available, and suitable for scripts and existing integrations:

```sh
levier run submit --runtime react1 --prompt "hello" --watch
levier run watch <run_id>
levier run show <run_id> --events
levier run cancel <run_id> --yes
```

The low-level `submit` can pass `--credentials`, `--provider`, `--model`,
`--idempotency-key` and others explicitly.

See [Inspect and cancel runs](levier-run.md) for details.

### 6. Exit codes

```text
0  completed
1  run failed
2  argument/configuration error
3  authentication failure
4  network/upstream retryable failure
5  rejected or timed out
```

## From the SDK

### 1. submit_run

```python
from levier import App

app = App(tools=[])

run = app.submit_run(
    "Add 2, then add 3.",
    runtime="react1",
)
run_id = run["run_id"]
```

Optional arguments:

```text
provider / model
max_steps
run_ms
tool_ms
idempotency_key
```

When `runtime` is not passed, the project / profile default is used.

### 2. wait_run

```python
result = app.wait_run(run_id, timeout=120)
print(result["status"])
```

Polls until a terminal state. Terminal states:

```text
completed / failed / cancelled / timeout
```

### 3. get_events

```python
events = app.get_events(run_id)
for event in events:
    print(event["event_type"], event.get("data"))
```

Common events:

```text
RUN_STATUS
MESSAGE_DELTA
MESSAGE_COMPLETED
TOOL_CALL_BEGIN
TOOL_CALL_END
ERROR
```

### 4. cancel_run

```python
app.cancel_run(run_id)
```

Cancellation is cooperative: the Runtime decides when to stop after receiving the cancel. A run that has already reached a terminal state does not roll back.

### 5. One-shot app.run

```python
app.run("Add 2, then add 3.")
```

It opens a temporary connection, submits the run, waits for a terminal state and prints the final message.

```python
app.run("hello", runtime="echo1", print_output=False)
```

### 6. Events and results

`wait_run` returns run details, `get_events` returns the event list. When you need stable parsing, prefer:

```text
event["event_type"]
event["data"]
```

Do not rely on display text.

## Run state and events

### 1. Get a run

SDK:

```python
run = app.get_run(run_id)
print(run["status"], run["runtime_name"])
```

CLI:

```sh
levier run show <run_id>
```

### 2. Wait for a terminal state

```python
result = app.wait_run(run_id, timeout=120)
```

Terminal states:

```text
completed / failed / cancelled / timeout
```

### 3. Event list

```python
events = app.get_events(run_id)
```

CLI:

```sh
levier run show <run_id> --events
```

Common events:

| Event | Meaning |
|---|---|
| `RUN_STATUS` | Status change |
| `MESSAGE_DELTA` | Streamed text |
| `MESSAGE_COMPLETED` | One complete assistant message |
| `TOOL_CALL_BEGIN` | A tool call starts |
| `TOOL_CALL_END` | Tool result or error |
| `ERROR` | Explicit Runtime error |
| `USAGE` | Token usage (if the model provides it) |

### 4. Cancel a run

```python
app.cancel_run(run_id)
```

CLI:

```sh
levier run cancel <run_id> --yes
```

Cancellation is cooperative; a run that has already reached a terminal state does not roll back.

### 5. Low-level CLI

```sh
levier run watch <run_id>
levier run show <run_id> --events
levier run cancel <run_id> --yes
```

Prefer `--json` in scripts.

### 6. Event cursor

The HTTP events API supports:

```text
after_attempt
after_seq
limit
```

The SDK's `get_events` fetches all available events by default; use the low-level HTTP API or later arguments when you need to backfill.

## Next steps

- App core: [App core workflow](app-core.md)
- Management: [App management](app-manage.md)
- Levier runs: [Runs](levier-run.md)
