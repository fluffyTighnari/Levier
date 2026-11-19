# Levier CLI reference

This page describes the supported operational surface. The main onboarding
walkthrough lives in the repository [`README.md`](../README.md).

## Public surface

```text
levier version
levier serve
levier status
levier doctor
levier app init|show|check|list|rotate-key
levier provider list|show|add|update|remove|check
levier mcp list|add|remove
levier run submit|watch|show|cancel
levier runtime list
```

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

## Common flags

```text
--server URL        Levier HTTP address
--admin-token TOKEN Admin token (or LEVIER_ADMIN_TOKEN)
--credentials PATH  App credentials file (or LEVIER_CREDENTIALS)
--json              Machine-readable output
--timeout DURATION  Per-request timeout
```

The admin token is used to manage Apps, providers, MCP servers, and fleet
resources. An App API key is used to submit and inspect that App's Runs.

## App

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
once; rotating it invalidates the previous key immediately.

## Providers

Provider records contain metadata and a secret reference, never a plaintext
key. The key comes from `LEVIER_SECRET_<REF>` or `LEVIER_SECRETS_FILE`.

```sh
levier provider add \
  --name deepseek \
  --adapter openai_compatible \
  --base-url https://api.deepseek.com \
  --default-model deepseek-flash \
  --models deepseek-flash,deepseek-reasoner \
  --secret-ref deepseek_api_key \
  --default

levier provider list
levier provider show --name deepseek
levier provider check --name deepseek
levier provider update --name deepseek ...
levier provider remove --name deepseek --yes
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
  --mcp-socket PATH
```

## Environment

```text
LEVIER_SERVER                 default HTTP address
LEVIER_ADMIN_TOKEN            admin operations
LEVIER_CREDENTIALS            App credentials file
LEVIER_SECRET_<REF>           model provider secret
LEVIER_SECRETS_FILE           control-plane secret JSON file
LEVIER_MCP_SECRETS_FILE       MCP Runtime secret JSON file
LEVIER_RUNTIME_CONFIG         runtime YAML
LEVIER_MIDDLEWARE_ADDRESS     runtime gRPC address
LEVIER_RUNTIME_NAME           runtime routing name
LEVIER_RUNTIME_INSTANCE_ID    runtime instance id
LEVIER_APP_MCP_REFS           comma-separated App MCP refs
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
