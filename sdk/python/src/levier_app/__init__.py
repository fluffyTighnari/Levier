"""Python adapter for Levier Apps.

A minimal App:

    from levier_app import LevierApp

    app = LevierApp.from_credentials()

    @app.tool("counter_add", schema={...})
    def counter_add(arguments):
        return {"value": arguments["delta"]}

    app.run()

The adapter owns authentication, tool registration, WebSocket lifecycle,
reconnects, and the local_tool_call/local_tool_result contract. Run submission,
provider discovery, waiting, events, and cancel are methods on LevierApp.

Copyright 2026 The Levier Authors.
"""

from .app import LevierApp, PROTOCOL_VERSION, SDK_VERSION, ToolSpec
from .cli import run_app
from .credentials import (
    Credentials,
    CredentialsError,
    Secret,
    credential_fingerprint,
    default_credentials_path,
    load_credentials,
    resolve_server,
    save_credentials,
)
from .errors import (
    AppInactiveError,
    HTTPError,
    LevierError,
    ProtocolError,
    ToolExecutionError,
    TransportError,
)
from .http import WsToken

__version__ = SDK_VERSION

__all__ = [
    "AppInactiveError",
    "Credentials",
    "CredentialsError",
    "HTTPError",
    "LevierApp",
    "LevierError",
    "PROTOCOL_VERSION",
    "run_app",
    "ProtocolError",
    "SDK_VERSION",
    "Secret",
    "ToolExecutionError",
    "ToolSpec",
    "TransportError",
    "WsToken",
    "__version__",
    "credential_fingerprint",
    "default_credentials_path",
    "load_credentials",
    "resolve_server",
    "save_credentials",
]
