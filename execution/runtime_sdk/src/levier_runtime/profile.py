"""Read the user-level Levier CLI profile.

The CLI owns the profile file; runtimes only use it as a fallback when no
explicit ``--config`` or environment variable is provided. Runtime authors do
not need to parse this file themselves.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import os
import sys
from pathlib import Path


def profile_path() -> Path:
    """Return the same profile path used by the Go CLI."""
    value = os.environ.get("LEVIER_HOME")
    if value:
        return Path(value).expanduser() / "config.yaml"
    if os.name == "nt":
        base = os.environ.get("APPDATA")
        if base:
            return Path(base) / "levier" / "config.yaml"
    elif sys.platform == "darwin":
        return Path.home() / "Library" / "Application Support" / "levier" / "config.yaml"
    else:
        base = os.environ.get("XDG_CONFIG_HOME")
        if base:
            return Path(base) / "levier" / "config.yaml"
    return Path.home() / ".config" / "levier" / "config.yaml"


def profile_values() -> dict[str, str]:
    """Read the flat profile without adding another YAML dependency."""
    try:
        lines = profile_path().read_text(encoding="utf-8").splitlines()
    except OSError:
        return {}
    values: dict[str, str] = {}
    for raw in lines:
        line = raw.strip()
        if not line or line.startswith("#") or ":" not in line:
            continue
        key, value = line.split(":", 1)
        key = key.strip()
        value = value.strip()
        if not key:
            continue
        if len(value) >= 2 and value[0] == value[-1] and value[0] in {"'", '"'}:
            value = value[1:-1]
        values[key] = value
    return values


def default_runtime_config() -> str:
    """Return the configured Runtime YAML path, or an empty string."""
    return profile_values().get("runtime_config", "")


def default_mcp_socket() -> str:
    """Return the configured MCP Runtime socket, or an empty string."""
    return profile_values().get("mcp_socket", "")


def default_runtime_name() -> str:
    """Return the process name used when no runtime YAML is supplied."""
    return profile_values().get("runtime_name", "")


def default_runtime_token() -> str:
    """Return the local runtime credential, if one is in the user profile."""
    return profile_values().get("runtime_token", "")


def default_middleware_address() -> str:
    """Return the gRPC address derived by ``levier up``/``levier setup``."""
    value = profile_values().get("middleware_address", "")
    if value:
        return value
    server = profile_values().get("server", "")
    if not server:
        return ""
    from urllib.parse import urlparse

    parsed = urlparse(server if "://" in server else f"http://{server}")
    host = parsed.hostname or ""
    if not host:
        return ""
    return f"{host}:9090"
