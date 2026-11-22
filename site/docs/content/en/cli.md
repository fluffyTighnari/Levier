---
title: CLI reference
status: published
description: Main paths, observability, advanced commands, compatibility, --json and exit codes.
updated: 2026-11-21
---

## 1. Main paths

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

| Command | Purpose | Details |
|---|---|---|
| `up` | Start the local stack | [Start](levier-install.md) |
| `init` | Project App identity | [Project initialization](app-dev.md) |
| `model connect` | Model | [Model connection](pre-model.md) |
| `dev` | Start the App | [dev and serve](app-dev.md) |
| `run` | Submit a run | [Submit a run](levier-run.md) |
| `runtime init` | Generate a Runtime | [runtime init](runtime-start.md) |
| `runtime start` | Start a Runtime | [runtime start](runtime-start.md) |
| `status` | Status | [Status](levier-status.md) |
| `logs` | Logs | [Logs and processes](levier-status.md) |
| `down` | Stop | [Stop](levier-status.md) |

## 2. Observability

```text
levier status
levier run show <run_id> --events
levier runtime list
levier logs --follow
```

## 3. Advanced

```text
levier setup
levier serve
levier doctor
levier describe
levier selfcheck [--deep]
levier app ...
levier mcp ...
levier run submit|watch|show|cancel
levier stack ...
levier ops ...
levier migrate ...
levier maintenance ...
levier config ...
```

They are still available, but they do not appear in the main help or the golden path.

## 4. Compatibility

Legacy entry points are kept:

```text
levier provider ...      # consolidated into model
levier app ...           # internal/automation
levier run submit ...    # low-level scripting
levier stack ...         # Compose wrapper
levier admin ...         # operations namespace
```

Prefer the main paths in new scripts; use advanced commands only when you need precise control.

## 5. --json

Main-path commands support `--json`:

```sh
levier status --json
levier model list --json
levier run "hello" --json
levier runtime list --json
```

Do not parse the human-readable text.

## 6. Exit codes

```text
0  success
1  operation failed
2  invalid command / arguments
3  authentication / authorization failure
4  retryable / upstream failure
5  refused or timed out
```

## Next steps

- Environment variables: [Environment variables](env.md)
- Errors: [Errors and troubleshooting](errors.md)
- SDK overview: [SDK reference](sdk.md)
