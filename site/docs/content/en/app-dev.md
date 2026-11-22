---
title: "Initialize and run an App"
status: published
description: "Initialize a project and serve the App with dev or Python."
updated: 2026-11-21
---

## Initialize a project

### 1. levier init

Run in the project directory:

```sh
levier init
```

The App name defaults to the current directory name. You can also set it explicitly:

```sh
levier init counter-agent
levier init --name counter-agent
```

### 2. What it writes

```text
User profile: server, app_name, credentials, runtime, mcp_refs
Project .levier.json: App identity and defaults for the current project
Default credentials file: 0600, contains the App API key
```

The default credentials location is determined by the profile; usually `<app-name>.json`
in the user config directory.

### 3. Arguments

| Argument | Description |
|---|---|
| `[name]` / `--name` | App name; defaults to the current directory |
| `--server` | Control plane address |
| `--admin-token` | Admin token required to create the App |
| `--runtime` | Default runtime_name |
| `--mcp-ref` | Comma-separated MCP references |
| `--rotate` | Rotate when the App exists but the key is lost |
| `--force` | Overwrite an existing credentials file |
| `--json` | Machine-readable output |
| `--timeout` | Timeout for a single request |

### 4. Output

> [!OUTPUT]
> ```text
> ready: counter-agent
> app:     app_01...
> server:  http://127.0.0.1:8080
> runtime: react1
> next:
>   levier dev            # serve this project's tools
>   levier run "hello"    # submit a run without flags
> ```

If usable credentials already exist, it prints `app already initialized` and reuses them.

### 5. Common failures

| Symptom | Action |
|---|---|
| `credentials already exist` | Overwrite with `--force`, or rotate with `--rotate` |
| `admin token is required` | Run `levier up` once, or set `LEVIER_ADMIN_TOKEN` |
| `app key could not be verified` | Check the server and the credentials path, then re-run `init --rotate` |
| Wrong project name | `levier init --name <name>` |

### 6. Multiple projects

Run `levier init` separately in each project directory. The project `.levier.json` overrides the App identity
and runtime defaults from the user profile. See [Project .levier.json](pre-profile.md) for details.

## dev and serve

### 1. levier dev

```sh
levier dev
```

It looks for these by default:

```text
./app.py
./levier_app.py
```

It injects:

```text
LEVIER_CREDENTIALS
LEVIER_SERVER
LEVIER_APP_MCP_REFS
```

> [!OUTPUT]
> ```text
> app:     /path/to/app.py
> server:  http://127.0.0.1:8080
> ```

The process must keep running; tool handlers execute inside the App process.

### 2. Arguments

| Argument | Description |
|---|---|
| `--file` | App file to use |
| `--python` | Python executable to use |
| `--server` | Override the control plane address |
| `--credentials` | Override the credentials path |
| `--mcp-ref` | Override MCP references |

### 3. Running Python directly

```sh
python app.py
```

`App.run()` without a prompt connects and serves tool calls; the effect matches `levier dev`.
This requires the SDK to be installed in the current Python and the project to have been initialized with `levier init`.

### 4. serve lifecycle

```python
app = App(tools=[counter_add])
app.run()          # blocking; serves tools
```

To stop:

```text
Ctrl+C
```

Or call it from code:

```python
app.stop()
```

### 5. One-shot startup

If you do not want to open two terminals by hand:

```sh
levier run "Add 2 and 3." --with-app
```

`--with-app` starts `./app.py` temporarily for the duration of the run; if an App is already connected, it is not started again.

### 6. Common failures

| Symptom | Check first |
|---|---|
| `no app.py found` | Pass `--file`, or create `app.py` |
| `no App is initialized` | Run `levier init` first |
| `ModuleNotFoundError: levier` | The current Python does not have the SDK installed |
| Tools are not called | Whether the App is online; the websocket count in `levier status` |

## Next steps

- App core: [App core workflow](app-core.md)
- Runs: [Runs](app-runs.md)
- Management: [App management](app-manage.md)
