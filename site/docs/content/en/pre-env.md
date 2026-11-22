---
title: "Environment and secrets"
status: published
description: "Environment precedence, App and model keys, and managed secrets."
updated: 2026-11-21
---

## Environment and precedence

### 1. Precedence

```text
Command-line flags
  > Environment variables
  > Project .levier.json
  > User profile
  > Built-in defaults
```

`LEVIER_HOME` is the exception: it directly determines the profile and data directory.

### 2. User and project

```text
LEVIER_HOME          Overrides the profile / data directory
LEVIER_SERVER        Default HTTP address
LEVIER_ADMIN_TOKEN   Token for administrative operations
LEVIER_CREDENTIALS   App credentials file
LEVIER_APP_CREDENTIALS  Same as LEVIER_CREDENTIALS
LEVIER_APP_NAME      Default App name
LEVIER_APP_MCP_REFS  Comma-separated MCP references
LEVIER_RUNTIME_NAME  Default runtime_name
```

### 3. CLI

```text
LEVIER_PYTHON        Python used by levier dev / runtime start
LEVIER_STACK_FILE    Compose file for levier up/down/logs/ps
LEVIER_CONFIG        Control-plane YAML for levier serve
LEVIER_GRPC          gRPC address for daemon check / selfcheck
LEVIER_DAEMON_ID     daemon id for daemon check / selfcheck
LEVIER_DAEMON_TOKEN  daemon token for daemon check / selfcheck
```

### 4. App

```text
LEVIER_CREDENTIALS       App credentials file
LEVIER_APP_MCP_REFS      Comma-separated MCP references
LEVIER_MODEL_API_KEY     Key alternative for model connect
```

The App SDK also reads the project `.levier.json` and the user profile, so you
usually do not need to set these variables in the App process.

### 5. Runtime

```text
LEVIER_RUNTIME_CONFIG           Runtime YAML
LEVIER_MIDDLEWARE_ADDRESS       Control-plane gRPC address
LEVIER_RUNTIME_NAME             Route name
LEVIER_RUNTIME_INSTANCE_ID      Instance ID
LEVIER_RUNTIME_TOKEN            Runtime authentication
LEVIER_RUNTIME_TOKEN_FILE       Runtime token file
LEVIER_RUNTIME_TYPE             normal / other
LEVIER_RUNTIME_MAX_CONCURRENT_RUNS  Concurrency capacity
LEVIER_RUNTIME_LABELS           Labels
LEVIER_RUNTIME_LOG_LEVEL        Log level
LEVIER_MCP_RUNTIME_SOCKET       MCP Runtime address
LEVIER_MCP_SOCKET               MCP socket
LEVIER_MCP_ADDR                 MCP TCP address
LEVIER_MCP_SECRETS_FILE         MCP secret JSON
```

`levier runtime start` injects addresses, names and tokens from the profile; you
only need to set them explicitly for remote deployments.

### 6. Control plane and deployment

```text
LEVIER_DATABASE_URL / _FILE
LEVIER_HTTP_ADMIN_TOKEN / _FILE
LEVIER_GRPC_DAEMON_TOKENS_FILE
LEVIER_ENVIRONMENT
LEVIER_LOG_LEVEL / LEVIER_LOG_FORMAT
LEVIER_RETENTION_ENABLED
LEVIER_RETENTION_RUN_DAYS
LEVIER_RETENTION_SESSION_DAYS
LEVIER_RETENTION_BATCH_SIZE
```

These are control-plane process settings, not day-to-day App / Runtime connection parameters.

### 7. Secret variables

```text
LEVIER_SECRET_<REF>        Single model secret
LEVIER_SECRETS_FILE        Control-plane read-only secret JSON
LEVIER_SECRETS_WRITE_FILE  Control-plane managed secret JSON
```

Do not put secrets in a long-running App environment; use `model connect` or 0600 files.

## Credentials and secrets

### 1. Two kinds of keys

| Key | Purpose | Stored in |
|---|---|---|
| App API key | The App connects to Levier and submits/queries runs | 0600 credentials file |
| Model API key | The Runtime calls the model | Control-plane managed secret |

An App key is not a model key. Model keys never appear in App credentials, run events or WebSocket messages.

### 2. managed secret

`levier model connect` writes the model key to the control plane's managed secret file:

```text
LEVIER_SECRETS_WRITE_FILE
```

The control plane can read it from:

```text
LEVIER_SECRET_<REF>
LEVIER_SECRETS_FILE
```

Recommended:

```json
{
  "deepseek_api_key": "sk-..."
}
```

File permissions are 0600; do not commit, print or log it.

### 3. App credentials

The credentials file written by `levier init` contains:

```text
app_id
app_name
server
api_key
created_at
```

The App API key is returned only once, at creation or rotation; the database stores only a hash.

### 4. Rotation and recovery

Rotate:

```sh
levier app rotate-key --credentials ./data/agent/credentials.json --yes
```

The old key is invalidated immediately, the credentials file is updated atomically, and a protected `.bak` backup is kept.

Lost credentials:

```sh
levier init --rotate
```

Or the low-level command:

```sh
levier app init --name <app> --credentials-out <path> --rotate --yes
```

Do not delete the database or credentials volume to "start over".

### 5. Permissions and logs

- Credentials file 0600; directory 0700.
- The admin token is only used to create/rotate Apps and never enters the runtime.
- Model keys are not exposed through CLI output, `/meta`, run events or checkpoints.
- `--json` does not output secret values either.

### 6. Remote deployment

For production:

```sh
levier setup \
  --server https://levier.example.com \
  --admin-token <admin-token>
```

Use 0600 files or an external secret manager for model secrets; inject the runtime token
once with `levier setup --runtime-token`. See [Remote control plane](levier-remote.md).

## Next steps

- Profile and project: [Profile and project configuration](pre-profile.md)
- Model: [Model configuration](pre-model.md)
- MCP: [MCP configuration](pre-mcp.md)
