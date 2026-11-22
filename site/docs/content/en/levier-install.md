---
title: "Install, connect and start"
status: published
description: "Install the CLI, connect a profile and start the local stack."
updated: 2026-11-21
---

## Install and upgrade

### 1. Release build

Linux / macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/fluffyTighnari/levier/main/scripts/install.sh | sh
```

Pin the version:

```sh
sh scripts/install.sh --version v0.1.0
```

Windows:

```powershell
.\scripts\install.ps1
```

Pin the version and directory:

```powershell
.\scripts\install.ps1 -Version v0.1.0 -Prefix "$env:USERPROFILE\.local\bin"
```

The installer will:

```text
Put levier on PATH
Download the Compose source bundle
Install the Python SDK (from levier import App, tool, Runtime)
```

### 2. Source

```sh
make install
levier setup --server http://127.0.0.1:8080
```

`setup` writes the user profile, not the project config.

### 3. Upgrade

Re-running the same install command is enough. For production, pin the version:

```sh
sh scripts/install.sh --version v0.1.0
```

After upgrading, confirm:

```sh
levier version
```

### 4. Verify

```sh
levier version
```

> [!OUTPUT]
> ```text
> levier v0.1.0
> commit: ...
> platform: ...
> ```

If `levier` is not on PATH, reopen the terminal or check the install directory.

### 5. Uninstall and cleanup

Deleting the `levier` executable from the install directory is enough. The user profile and data directory are not removed automatically:

```text
~/.config/levier/
%APPDATA%\levier\
```

Delete them manually only when you need a complete cleanup; do not remove credentials that are still in use.

## Start the stack

### 1. levier up

```sh
levier up
```

It does the following:

```text
Start PostgreSQL
Start the Levier control plane
Start the MCP Runtime
Start the built-in React Runtime
Start the built-in Echo Runtime
Wait for /readyz
Write the user profile
```

The example agent container is not started by default.

> [!OUTPUT]
> ```text
> waiting for http://127.0.0.1:8080 ...
> levier is ready
> next:
>   levier model connect ...
>   levier init
> ```

### 2. Options

| Option | Description |
|---|---|
| `--file` | Specify the Compose file |
| `--server` | Override the HTTP address to wait on |
| `--profile` | Start only the specified service profile |
| `--build` | Build images |
| `--all` | Start all services, including the example agent |
| `--wait` | Wait for readiness |
| `--dry-run` | Only print the commands that would run |
| `--timeout` | Wait timeout |

### 3. Waiting for readiness

`levier up` polls `/readyz`. Continue once you see `levier is ready`.

If it times out:

```sh
levier status
levier logs --tail=100
```

### 4. profile

`up` writes:

```text
server
runtime
runtime_token
middleware_address
mcp_socket
stack_file
```

After that, `levier init`, `model connect` and `run` no longer need the address and token passed in.

### 5. Start the control plane directly

For existing deployments or when you need a custom YAML:

```sh
levier serve --config ./levier.yaml
```

This is an advanced entry point; for everyday local stacks, use `levier up`.

## setup and profile

### 1. levier setup

```sh
levier setup --server http://127.0.0.1:8080
```

Writes the user profile. After that, `init`, `model connect` and `run` read it automatically.

Connecting to an existing control plane:

```sh
levier setup \
  --server https://levier.example.com \
  --admin-token <admin-token>
```

### 2. Options

| Option | Description |
|---|---|
| `--server` | Control plane HTTP address |
| `--admin-token` | Admin token written to the profile |
| `--credentials` | Default App credentials path |
| `--runtime` | Default runtime_name |
| `--grpc` | Runtime gRPC address |
| `--runtime-token` | Runtime token written to the profile |
| `--runtime-config` | Default Runtime YAML |
| `--mcp-socket` | MCP Runtime address |
| `--control-config` | Control plane YAML |
| `--stack-file` | Compose file |
| `--reset` | Clear, then write |
| `--show` | Display only, do not write |
| `--json` | Machine-readable |

### 3. show

```sh
levier setup --show
```

> [!OUTPUT]
> ```text
> profile      /home/me/.config/levier/config.yaml
> server       http://127.0.0.1:8080
> runtime      react1
> grpc         127.0.0.1:9090
> credentials  /home/me/.config/levier/counter-agent.json
> mcp_socket   tcp://127.0.0.1:9091
> ```

### 4. reset

```sh
levier setup --reset
```

Clears the profile values and writes them again. It does not delete App credentials, the database or data volumes.

### 5. Security

The profile is 0600, but it should only hold locators. Admin / runtime tokens are written only when
`--admin-token` / `--runtime-token` is passed explicitly; otherwise the environment variables continue to be used.

## Next steps

- Stack lifecycle: [Stack lifecycle](levier-status.md)
- Runs: [Runs](levier-run.md)
- Remote control plane: [Remote control plane](levier-remote.md)
