"""Middleware — MCP runtime secret resolution.

Bindings travel from the control plane with ${secret:name} placeholders.
Only the runtime resolves them, so TaskAssign and the Harness never see
plaintext credentials.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import json
import os
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Protocol

_PLACEHOLDER = re.compile(r"\$\{secret:([^}]+)\}")
_DEFAULT_PREFIX = "LEVIER_SECRET_"
_LEGACY_PREFIX = "MIDDLEWARE_SECRET_"


class SecretNotFound(RuntimeError):
    """A placeholder referenced a secret that is not configured."""

    def __init__(self, name: str) -> None:
        super().__init__(f"secret {name!r} is not configured")
        self.name = name


class SecretResolver(Protocol):
    """Resolves one secret name to its value."""

    def resolve(self, name: str) -> str:
        ...


@dataclass
class EnvSecretResolver:
    """Reads LEVIER_SECRET_<NAME>, with MIDDLEWARE_SECRET_ as fallback."""

    prefix: str = _DEFAULT_PREFIX

    def resolve(self, name: str) -> str:
        suffix = name.upper().replace("-", "_")
        keys = [self.prefix + suffix]
        if self.prefix == _DEFAULT_PREFIX:
            keys.append(_LEGACY_PREFIX + suffix)
        for key in keys:
            value = os.environ.get(key)
            if value is not None:
                return value
        raise SecretNotFound(name)


@dataclass
class FileSecretResolver:
    """Reads a JSON object of secret name -> value."""

    values: dict[str, str]

    def resolve(self, name: str) -> str:
        value = self.values.get(name)
        if value is None:
            raise SecretNotFound(name)
        return value

    @classmethod
    def load(cls, path: str) -> "FileSecretResolver":
        payload = json.loads(Path(path).read_text(encoding="utf-8"))
        if not isinstance(payload, dict):
            raise ValueError("secret file must contain a JSON object")
        return cls(values={str(key): str(value) for key, value in payload.items()})


class ChainResolver:
    """Tries each resolver in order."""

    def __init__(self, resolvers: list[SecretResolver]) -> None:
        self.resolvers = resolvers

    def resolve(self, name: str) -> str:
        for resolver in self.resolvers:
            try:
                return resolver.resolve(name)
            except SecretNotFound:
                continue
        raise SecretNotFound(name)


def resolve_string(value: str, resolver: SecretResolver) -> str:
    """Replace every ${secret:name} placeholder in one string."""
    return _PLACEHOLDER.sub(lambda match: resolver.resolve(match.group(1)), value)


def resolve_binding(binding: dict[str, Any], resolver: SecretResolver) -> dict[str, Any]:
    """Return a copy of the binding with placeholders resolved."""
    resolved = dict(binding)
    if isinstance(resolved.get("command"), str):
        resolved["command"] = resolve_string(resolved["command"], resolver)
    if isinstance(resolved.get("url"), str):
        resolved["url"] = resolve_string(resolved["url"], resolver)
    if isinstance(resolved.get("args"), list):
        resolved["args"] = [
            resolve_string(str(value), resolver) for value in resolved["args"]
        ]
    if isinstance(resolved.get("env"), dict):
        resolved["env"] = {
            str(key): resolve_string(str(value), resolver)
            for key, value in resolved["env"].items()
        }
    if isinstance(resolved.get("headers"), dict):
        resolved["headers"] = {
            str(key): resolve_string(str(value), resolver)
            for key, value in resolved["headers"].items()
        }
    return resolved
