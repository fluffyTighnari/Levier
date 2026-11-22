---
title: Environment variables
status: published
description: User, project, CLI, SDK and deployment environment variables.
updated: 2026-11-21
---

## 1. Precedence

```text
Command-line flag
  > Environment variables
  > Project .levier.json
  > User profile
  > Built-in defaults
```

`LEVIER_HOME` directly determines the profile / data directory.

## 2. User and project

| Variable | Purpose |
|---|---|
| `LEVIER_HOME` | profile / data directory |
| `LEVIER_SERVER` | default HTTP address |
| `LEVIER_ADMIN_TOKEN` | administrative operations |
| `LEVIER_CREDENTIALS` | App credentials file |
| `LEVIER_APP_CREDENTIALS` | same as above |
| `LEVIER_APP_NAME` | default App name |
| `LEVIER_APP_MCP_REFS` | MCP references |
| `LEVIER_MODEL_API_KEY` | key override for model connect |
| `LEVIER_RUNTIME_NAME` | default runtime_name |

## 3. CLI

| Variable | Purpose |
|---|---|
| `LEVIER_PYTHON` | Python for dev / runtime start |
| `LEVIER_STACK_FILE` | Compose file |
| `LEVIER_CONFIG` | control plane YAML |
| `LEVIER_GRPC` | gRPC for daemon check |
| `LEVIER_DAEMON_ID` | daemon check |
| `LEVIER_DAEMON_TOKEN` | daemon check |

## 4. App SDK

```text
LEVIER_CREDENTIALS
LEVIER_APP_MCP_REFS
LEVIER_MODEL_API_KEY
```

Usually injected automatically by `levier init` / `levier dev`.

## 5. Runtime

| Variable | Purpose |
|---|---|
| `LEVIER_RUNTIME_CONFIG` | Runtime YAML |
| `LEVIER_MIDDLEWARE_ADDRESS` | control plane gRPC |
| `LEVIER_RUNTIME_NAME` | routing name |
| `LEVIER_RUNTIME_INSTANCE_ID` | instance ID |
| `LEVIER_RUNTIME_TOKEN` | runtime token |
| `LEVIER_RUNTIME_TOKEN_FILE` | token file |
| `LEVIER_RUNTIME_TYPE` | Runtime type |
| `LEVIER_RUNTIME_MAX_CONCURRENT_RUNS` | concurrency capacity |
| `LEVIER_RUNTIME_LABELS` | labels |
| `LEVIER_RUNTIME_LOG_LEVEL` | log level |
| `LEVIER_MCP_RUNTIME_SOCKET` | MCP address |
| `LEVIER_MCP_SOCKET` | MCP socket |
| `LEVIER_MCP_ADDR` | MCP TCP |
| `LEVIER_MCP_SECRETS_FILE` | MCP secret file |

## 6. Control plane and deployment

```text
LEVIER_DATABASE_URL / LEVIER_DATABASE_URL_FILE
LEVIER_HTTP_ADMIN_TOKEN / LEVIER_HTTP_ADMIN_TOKEN_FILE
LEVIER_GRPC_DAEMON_TOKENS_FILE
LEVIER_ENVIRONMENT
LEVIER_LOG_LEVEL / LEVIER_LOG_FORMAT
LEVIER_RETENTION_ENABLED
LEVIER_RETENTION_RUN_DAYS
LEVIER_RETENTION_SESSION_DAYS
LEVIER_RETENTION_BATCH_SIZE
```

## 7. Secrets

```text
LEVIER_SECRET_<REF>
LEVIER_SECRETS_FILE
LEVIER_SECRETS_WRITE_FILE
LEVIER_MCP_SECRETS_FILE
```

Secret values should only come from 0600 files or environment variables; never write them into Compose plaintext or shell history.

## Next steps

- Configuration layers: [Configuration and precedence](pre-env.md)
- Credentials: [Credentials and secrets](pre-env.md)
- Remote deployment: [Remote control plane](levier-remote.md)
