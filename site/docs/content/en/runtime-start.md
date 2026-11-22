---
title: "Set up a runtime"
status: published
description: "Install the Runtime SDK, initialize a template and start the runtime."
updated: 2026-11-21
---

## Install the SDK

### 1. PyPI

```sh
pip install levier
```

App-only usage does not load the Runtime gRPC dependencies immediately; they are
loaded only when `Context` / `Runtime` is imported.

### 2. From source

```sh
cd /path/to/middleware
python -m pip install -e ./sdk/python -e ./execution/runtime_sdk -e ./sdk/levier
```

To also run the built-in Runtimes:

```sh
python -m pip install -e ./execution/echo_runtime
python -m pip install -e ./execution/react_runtime
python -m pip install -e ./execution/agent_core
```

### 3. Dependencies

The Runtime SDK requires:

```text
Python 3.11+
gRPC
protobuf
```

MCP tools are used through `ctx.toolbox()`; there is no need to install an MCP
client in the Runtime.

### 4. Verify

```python
from levier import Context, Runtime
print(Context, Runtime)
```

If this raises `No module named levier_runtime`, only the App-side `levier`
package is installed.

## runtime init

### 1. runtime init

```sh
levier runtime init
```

Writes `./runtime.py` by default.

> [!OUTPUT]
> ```text
> created runtime.py
> start it with: levier runtime start
> ```

### 2. Specify a file

```sh
levier runtime init --file ./my-runtime/runtime.py
```

If the file already exists:

```sh
levier runtime init --force
```

`--force` overwrites the existing file; do not use it on a Runtime you are actively maintaining.

### 3. Directory conventions

Recommended:

```text
my-runtime/
  runtime.py
  requirements.txt
  README.md
```

`levier runtime start` looks for these by default:

```text
./runtime.py
./levier_runtime.py
./my_runtime.py
```

So you can start directly from the Runtime directory.

### 4. Template contents

The generated file contains:

```text
Context / Runtime imports
async def run(ctx)
ctx.toolbox()
ctx.chat()
ctx.invoke_all()
ctx.reply()
Runtime.start(run)
```

Keep only the parts you need; do not keep unused code just to be "complete".

### 5. Next steps

- Start: [runtime start](runtime-start.md)
- Context API: [Context](runtime-context.md)
- Tools: [Toolbox](runtime-toolbox.md)

## runtime start

### 1. runtime start

```sh
levier runtime start
```

Defaults:

```text
Look for ./runtime.py
Read runtime name, gRPC address and token from the profile
Start the Python process
Keep the connection open
```

> [!OUTPUT]
> ```text
> runtime: my-runtime
> file:    /path/to/runtime.py
> server:  127.0.0.1:9090
> ```

### 2. Options

| Option | Description |
|---|---|
| `--file` | Path to the runtime file |
| `--name` | runtime_name; defaults to the directory name |
| `--token` | runtime token |
| `--grpc` | control plane gRPC address |
| `--python` | Python executable |
| `--config` | optional Runtime YAML |
| `--log-level` | log level |

### 3. Profile identity

`levier up` / `levier setup` write:

```text
LEVIER_RUNTIME_NAME
LEVIER_RUNTIME_TOKEN
LEVIER_MIDDLEWARE_ADDRESS
LEVIER_MCP_RUNTIME_SOCKET
```

`runtime start` injects these values automatically.

### 4. Log level

```sh
levier runtime start --log-level debug
```

Also supported:

```sh
LEVIER_RUNTIME_LOG_LEVEL=debug levier runtime start
```

### 5. Confirm the connection

```sh
levier runtime list
```

You should see the new runtime / instance. If it does not appear:

```sh
levier logs
levier doctor
```

### 6. Remote

Remote deployments require an explicit token:

```sh
levier runtime start \
  --grpc levier.example.com:9090 \
  --token <runtime-token>
```

For TLS and certificates, see [Remote deployment and YAML](runtime-remote.md).

## Next steps

- Runtime core: [Runtime core workflow](runtime-core.md)
- Built-in runtimes: [Built-in runtimes](runtime-builtin.md)
- Context: [Context](runtime-context.md)
