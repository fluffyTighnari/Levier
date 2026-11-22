---
title: "Model configuration"
status: published
description: "Connect a model and verify it: URL, API key, model name, adapter and checks."
updated: 2026-11-21
---

## Connection

> [!NOTE]
> Models are required for the ReAct Runtime; you can skip this if you only run Echo or your own Runtime.
> This page covers the parameters and behavior of `model connect`; verification follows below.

### 1. Minimal configuration

You only need three things:

```text
URL
API key
model name
```

```sh
levier model connect \
  --url https://api.deepseek.com \
  --key sk-... \
  --model deepseek-chat
```

> [!OUTPUT]
> ```text
> model connected: deepseek
> url:              https://api.deepseek.com
> model:            deepseek-chat
> default:          true
> next:             levier init
> ```

### 2. What model connect does

```text
Infer the provider name from the URL
Infer the adapter from the URL
Generate secret_ref
Save provider metadata
Write the key to the control plane's 0600 managed secret file
Set as the default provider
Immediately check whether the secret is available
```

You do not need to set `LEVIER_SECRET_*`, and you do not need to understand `--secret-ref`.

### 3. Parameters

| Parameter | Required | Description |
|---|---|---|
| `--url` | Yes | OpenAI-compatible or Anthropic base URL |
| `--key` | Yes | API key; `LEVIER_MODEL_API_KEY` also works |
| `--model` | Yes | Default model name |
| `--name` | No | Explicit provider name; use it when the URL is ambiguous |
| `--adapter` | No | `auto` (default), `openai_compatible`, `anthropic` |
| `--models` | No | Allowed model list, comma-separated |
| `--default` | No | Make this the default provider; this is already the default behavior |
| `--default=false` | No | Leave the default provider unchanged |
| `--server` | No | Control plane address; reads the profile by default |
| `--admin-token` | No | Admin token; reads profile / environment by default |
| `--json` | No | Machine-readable output |

Supported adapters:

```text
openai_compatible   OpenAI style /chat/completions
anthropic           Anthropic Messages API
```

### 4. Multiple models and defaults

```sh
levier model connect \
  --name deepseek \
  --url https://api.deepseek.com \
  --key sk-... \
  --model deepseek-chat \
  --models deepseek-chat,deepseek-reasoner \
  --default
```

A second provider can omit `--default` to avoid overwriting the default:

```sh
levier model connect \
  --name local \
  --url http://127.0.0.1:8000/v1 \
  --key local \
  --model qwen2.5 \
  --default=false
```

You can select one explicitly when submitting a run:

```sh
levier run "hello" --provider deepseek --model deepseek-reasoner
```

### 5. Removal

```sh
levier model remove --name deepseek --yes
```

This deletes the provider metadata; whether the value in the managed secret
file is cleared depends on the control-plane implementation, so do not treat
this as a way to erase keys. To rotate a key, run `model connect` again.

### 6. Common failures

| Symptom | Check first |
|---|---|
| `401` / `403` | Whether the key is correct and unexpired |
| `model not found` | Whether `--model` is listed in `--models` |
| `connection refused` | Base URL, proxy, and whether the model service is online |
| `SECRET_MISSING` | Whether the managed secret file is writable, and whether the environment overrides it |
| Connected to the wrong provider | Specify `--name` and `--adapter` explicitly |

## Verification

### 1. model list

```sh
levier model list
```

> [!OUTPUT]
> ```text
> name               model                default  base_url
> deepseek           deepseek-chat        true     https://api.deepseek.com
> ```

Shows provider metadata only; it does not show `secret_ref` or the key.

JSON:

```sh
levier model list --json
```

### 2. model check

```sh
levier model check --name deepseek
```

> [!OUTPUT]
> ```text
> provider check: PASS deepseek
> ```

Checks performed:

```text
Whether the provider exists and is enabled
Whether the default model is empty
Whether the secret is configured
Whether base_url resolves
```

On failure it prints `provider check: FAIL` and the error text.

### 3. Common failures

| Output / symptom | Meaning | Action |
|---|---|---|
| `no models connected` | No provider yet | `levier model connect ...` |
| `provider not found` | `--name` is wrong | Confirm the name with `levier model list` |
| `secret_configured false` | The key was not written | Run `model connect` again |
| `401` / `403` | Invalid key | Replace the key and connect again |
| `model not found` | The default model is not in the allowed list | Adjust `--models` or `--model` |
| `connection refused` | Base URL unreachable | Check the URL, proxy and model service |

### 4. Secret status

`model list` does not show secret_ref; to see whether the secret is configured:

```sh
levier model check --name deepseek
```

To inspect provider details on the control-plane side, use an admin command:

```sh
levier ops daemons
```

A provider's secret_ref is not part of the App-visible surface; an App can only
see the model catalog through `App.list_model_providers()`.

## Next steps

- MCP: [MCP configuration](pre-mcp.md)
- Keys: [Environment and secrets](pre-env.md)
- End-to-end: [Full setup](onboarding.md)
