"""Read the user-level Levier CLI profile for MCP defaults.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import os
import sys
from pathlib import Path


def profile_path() -> Path:
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


def default_mcp_socket() -> str:
    return profile_values().get("mcp_socket", "")
