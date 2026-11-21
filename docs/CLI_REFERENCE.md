# Levier CLI reference

This page describes the supported operational surface. The main onboarding
walkthrough lives in the repository [`README.md`](../README.md).

## Public surface

The golden path is intentionally small:

```text
levier up
levier init [name]
levier model connect --url URL --key KEY --model MODEL
levier dev
levier run "prompt"
levier runtime init
levier runtime start
levier status
levier logs
levier down
```

Advanced/compatibility commands remain available but are not part of the main
help:

```text
levier setup
levier doctor
levier serve
levier app init|show|check|list|rotate-key   # internal/automation
levier mcp list|add|remove
levier run submit|watch|show|cancel
levier runtime list
levier stack up|down|ps|logs
```

`levier setup` writes a user-level profile (locators only, never an App key or
admin token). Client commands read it before falling back to built-in defaults.
The profile is stored at `%APPDATA%\levier\config.yaml` on Windows and
`~/.config/levier/config.yaml` on Linux/macOS; `LEVIER_HOME` overrides it.

Admin/operator commands are grouped under `levier admin`:

```text
levier admin ops runs|daemons|tasks
levier admin migrate status|up
levier admin maintenance status|prune
levier admin config check|print
levier admin daemon check
levier admin describe
levier admin selfcheck [--deep]
```

Old top-level aliases remain for deployment compatibility, but new scripts
should use `levier admin ...`.

## Golden path

```sh
levier up
levier model connect --url https://api.deepseek.com --key sk-... --model deepseek-chat
levier init                    # App name defaults to the current directory
levier dev                     # serves ./app.py with profile defaults
levier run "hello"             # uses the profile App, runtime, and provider
levier run "hello" --with-app  # starts ./app.py for the duration of the Run
levier runtime init            # writes ./runtime.py
levier runtime start           # serves the runtime with profile defaults
```

`levier init` writes the App identity to the default 0600 credentials location
and records `server`, `runtime`, `app_name`, and MCP refs in the user profile.
The paths and tokens are not part of the onboarding surface.

`levier model connect` infers the provider name and adapter from the URL,
generates the secret reference, stores the key through the Admin API, and makes
the provider the default. `--name` and `--adapter` remain available if the URL
is ambiguous.

## Common flags

```text
--server URL        Levier HTTP address (or profile / LEVIER_SERVER)
--admin-token TOKEN Admin token (or LEVIER_ADMIN_TOKEN)
--credentials PATH  App credentials file (or profile / LEVIER_CREDENTIALS)
--json              Machine-readable output
--timeout DURATION  Per-request timeout
```

`levier init` and `levier model connect` resolve the admin token from
`--admin-token`, `LEVIER_ADMIN_TOKEN`, the 0600 profile (if saved), or the
development default when the profile server is loopback. An App API key is used
to submit and inspect that App's Runs.

## Setup

```sh
levier setup --server http://127.0.0.1:8080
levier setup --show
levier setup --credentials ~/.config/levier/my-app.json
levier setup --stack-file /path/to/docker-compose.yml
```

The profile contains server, default credentials, runtime, runtime config, MCP
socket, and Compose paths. It does not contain the App API key. Admin and
runtime tokens are written only when explicitly passed to `--admin-token` /
`--runtime-token`; omit them to keep using environment variables.

## App（内部/自动化）

日常使用 `levier init`、`levier dev` 和 `levier run`。以下低层命令只用于显式
控制凭据路径、轮换和诊断。

```sh
levier app init \
  --name counter-agent \
  --credentials-out ./data/agent/credentials.json \
  [--mcp-ref github,e2e-echo]

levier app show --credentials ./data/agent/credentials.json
levier app check --credentials ./data/agent/credentials.json
levier app list
levier app rotate-key --credentials ./data/agent/credentials.json --yes
```

`app init` writes a 0600 credentials file. The App API key is returned only
once; rotating it invalidates the previous key immediately. If
`--credentials-out` is omitted, the file is placed next to the user profile
and that path becomes the default for later commands.

## Models

用户只使用 `model` 命令；`provider` 是控制面内部术语，不再暴露为公开命令。

```sh
levier model connect \
  --url https://api.deepseek.com \
  --key sk-... \
  --model deepseek-flash \
  --models deepseek-flash,deepseek-reasoner \
  --default

levier model list
levier model check --name deepseek
levier model remove --name deepseek --yes
```

Supported adapters:

```text
openai_compatible   OpenAI-style /chat/completions (DeepSeek, local vLLM, ...)
anthropic           Anthropic Messages API
```

## MCP

MCP server definitions are managed centrally. The Runtime connects through the
shared MCP Runtime socket.

```sh
levier mcp add \
  --name e2e-echo \
  --transport stdio \
  --config-file ./mcp-echo.json \
  --allowed-tools echo_token

levier mcp list
levier mcp remove --name e2e-echo --yes [--force]
```

`config-file` contains the MCP server command/args/env or URL/headers. Secret
placeholders use `${secret:name}` and are resolved by the MCP Runtime, not the
control plane.

## Runs

```sh
levier run submit \
  --credentials ./data/agent/credentials.json \
  --runtime react1 \
  --prompt "Add 2, then add 3." \
  [--provider deepseek] \
  [--model deepseek-flash] \
  --watch

levier run watch --run-id <id> --credentials ./data/agent/credentials.json
levier run show --run-id <id> --events --credentials ./data/agent/credentials.json
levier run cancel --run-id <id> --yes --credentials ./data/agent/credentials.json
```

If `--provider` is omitted, the control plane uses the default provider. If
`--model` is omitted, the provider default model is used.

## Runtime

```sh
levier runtime list [--json]
```

This shows connected runtime names, instances, health, capacity, and heartbeat.

Built-in Runtime CLIs:

```text
levier-echo-runtime
  --config PATH
  --check
  --print-config
  --version
  --log-level LEVEL

levier-react-runtime
  --config PATH
  --check
  --print-config
  --version
  --log-level LEVEL
```

## Environment

```text
LEVIER_HOME                   alternate user profile/data directory
LEVIER_SERVER                 default HTTP address
LEVIER_ADMIN_TOKEN            admin operations
LEVIER_CREDENTIALS            App credentials file
LEVIER_MODEL_API_KEY          alternative to --key for model connect
LEVIER_SECRET_<REF>           model provider secret
LEVIER_SECRETS_FILE           control-plane read-only secret JSON file
LEVIER_SECRETS_WRITE_FILE     control-plane managed secret JSON file
LEVIER_MCP_SECRETS_FILE       MCP Runtime secret JSON file
LEVIER_MCP_ADDR               MCP Runtime TCP listen address
LEVIER_RUNTIME_CONFIG         runtime YAML
LEVIER_PYTHON                 Python executable for levier dev/runtime start
LEVIER_MIDDLEWARE_ADDRESS     runtime gRPC address
LEVIER_RUNTIME_NAME           runtime routing name
LEVIER_RUNTIME_INSTANCE_ID    runtime instance id
LEVIER_APP_MCP_REFS           comma-separated App MCP refs
LEVIER_STACK_FILE             default compose file for levier stack
LEVIER_MCP_SOCKET             MCP Runtime socket
```

Secret values must be 0600 files or environment variables. They are never
printed by `status`, `doctor`, provider commands, or Runtime `--print-config`.

## Exit codes

```text
0  success
1  operation failed
2  invalid command or arguments
3  authentication / authorization failure
4  retryable/upstream failure
5  refused or timed out
```
