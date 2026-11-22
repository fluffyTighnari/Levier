---
title: "Profile and project configuration"
status: published
description: "User profile, project .levier.json and where each setting lives."
updated: 2026-11-21
---

## User profile

> [!NOTE]
> The profile stores locators (addresses, paths, runtime names), not App keys.
> Admin and runtime tokens are only written when passed explicitly.

### 1. Paths

```text
Windows        %APPDATA%\levier\config.yaml
Linux / macOS  ~/.config/levier/config.yaml
```

`LEVIER_HOME` can override the entire directory:

```sh
LEVIER_HOME=/tmp/levier-home levier setup --show
```

The profile is written with 0600 permissions.

### 2. Fields

```yaml
server: http://127.0.0.1:8080
credentials: /home/me/.config/levier/counter-agent.json
app_name: counter-agent
mcp_refs: echo
runtime: react1
runtime_token: change-me
middleware_address: 127.0.0.1:9090
runtime_config: /path/to/runtime.yaml
mcp_socket: tcp://127.0.0.1:9091
control_config: /path/to/levier.yaml
stack_file: /path/to/docker-compose.yml
```

| Field | Purpose |
|---|---|
| `server` | Default HTTP address |
| `credentials` | Default App credentials file |
| `app_name` | Default App name |
| `mcp_refs` | Default MCP references |
| `runtime` | Default runtime_name |
| `runtime_token` | Local runtime authentication |
| `middleware_address` | Runtime gRPC address |
| `runtime_config` | Optional Runtime YAML |
| `mcp_socket` | MCP Runtime address |
| `control_config` | Control-plane YAML |
| `stack_file` | Compose file |

### 3. setup and show

```sh
levier setup --server http://127.0.0.1:8080
levier setup --show
```

`--show` only prints; it writes nothing. The output includes:

```text
profile / server / runtime / grpc / credentials
runtime config / mcp_socket / control / stack
```

Connect to an existing control plane:

```sh
levier setup \
  --server https://levier.example.com \
  --admin-token <admin-token>
```

If you only need App features, you can write just:

```sh
levier setup --server https://levier.example.com
```

### 4. reset

```sh
levier setup --reset
```

Clears existing profile values and writes them again. It does not delete App credentials, the database or Compose data volumes.

### 5. Precedence

Command-line flags > environment variables > project `.levier.json` > user profile > built-in defaults.
See [Environment and precedence](pre-env.md).

## Project .levier.json

### 1. How it is created

Run this in the project directory:

```sh
levier init
```

It creates or reuses an App and writes:

```text
User profile
The current project's .levier.json
Default 0600 credentials file
```

### 2. Fields

```json
{
  "app_name": "counter-agent",
  "credentials": "/home/me/.config/levier/counter-agent.json",
  "server": "http://127.0.0.1:8080",
  "runtime": "react1",
  "mcp_refs": "echo"
}
```

| Field | Purpose |
|---|---|
| `app_name` | App identity for the current project |
| `credentials` | App credentials file |
| `server` | Control plane address |
| `runtime` | Default runtime_name |
| `mcp_refs` | Comma-separated MCP references |

`.levier.json` stores locators only, never API keys.

### 3. Precedence

Read order:

```text
Command-line flags
  > Environment variables
  > Project .levier.json
  > User profile
  > Built-in defaults
```

This lets one user use different Apps, runtimes and MCP references across projects.

### 4. Multiple projects

Run once in each project directory:

```sh
cd project-a && levier init
cd project-b && levier init
```

Each directory keeps its own `.levier.json`. `levier dev` / `levier run` read it from the current directory.

If a project needs a different credentials file:

```sh
levier init --name project-b-app --force
```

Or specify it explicitly:

```sh
levier dev --credentials ./project-b.json
```

### 5. Troubleshooting

| Symptom | Check first |
|---|---|
| `no App is initialized` | Whether the current directory has `.levier.json`, or run `levier init` first |
| Wrong App | `app_name` / `credentials` in `.levier.json` |
| Wrong runtime | `runtime` in `.levier.json` |
| MCP reference fails | Whether `mcp_refs` in `.levier.json` is registered |

## Next steps

- Environment and keys: [Environment and secrets](pre-env.md)
- Control-plane setup: [setup and profile](levier-install.md)
- End-to-end: [Full setup](onboarding.md)
