---
title: Errors and troubleshooting
status: published
description: Exit codes, common failures and what to check first.
updated: 2026-11-21
---

## 1. Exit codes

```text
0  success
1  operation failed
2  invalid command / arguments
3  authentication / authorization failure
4  retryable / upstream failure
5  refused or timed out
```

Check the exit code first, then the error text.

## 2. Startup

```sh
levier status
levier logs
levier doctor
```

| Symptom | Check first |
|---|---|
| connection refused | process / port |
| health fail | control plane logs |
| ready fail | database / migrations |
| daemons connected=0 | whether the Runtime is started |
| port in use | 8080 / 9090 |

## 3. Models

```sh
levier model list
levier model check --name <name>
```

| code / symptom | Action |
|---|---|
| `MODEL_PROVIDER_NOT_FOUND` | `levier model connect` |
| `SECRET_MISSING` | reconnect / check the secret file |
| 401 / 403 | change the key |
| model not found | check `--models` / `--model` |

## 4. App

```sh
levier app check
levier run show <run_id> --events
```

| code | Action |
|---|---|
| `UNAUTHORIZED` | re-run init / check credentials |
| `TOOL_NOT_FOUND` | tool name, whether the App is online |
| `TOOL_TIMEOUT` | handler logs, timeout |
| `TOOL_EXEC_ERROR` | App logs |
| `APP_UNREACHABLE` | restart the App, then submit again |

## 5. MCP

| code | Action |
|---|---|
| `MCP_RUNTIME_UNAVAILABLE` | start the MCP Runtime |
| `MCP_CONNECT_FAILED` | MCP server configuration/startup |
| `SECRET_MISSING` | whether `${secret:...}` has a value |
| `TOOL_DENIED` | `allowed_tools` |
| `TOOL_TIMEOUT` | MCP server / network |
| `MCP_RUNTIME_ERROR` | MCP Runtime logs |

## 6. Runtime

```sh
levier runtime list
levier logs
```

| Symptom | Check first |
|---|---|
| Runtime never appears | token / gRPC / process |
| stuck in queued | capacity / heartbeat |
| failed + ERROR | `run show --events` |
| cancellation has no effect | whether the Runtime checks `ctx.cancelled` |

## 7. Filing an issue

Attach:

```text
levier version
levier status
levier run show <run_id> --events
levier doctor
Last 100 log lines from the relevant container
The full command that was run
```

Redact keys, tokens and business data.

## Next steps

- CLI: [CLI reference](cli.md)
- Status: [Status](levier-status.md)
- Logs: [Logs and processes](levier-status.md)
