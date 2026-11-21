"""Canonical Levier App credentials for the Python adapter.

The SDK reads the same credential file as the Levier CLI. It never prints or
serializes the API key, and it rejects group/world-readable files on POSIX.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import json
import os
import stat
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any

DEFAULT_CREDENTIAL_VERSION = 1
DEFAULT_SERVER = "http://127.0.0.1:8080"


class CredentialsError(ValueError):
    """Raised when a credential file is missing, unsafe, or invalid."""


class Secret:
    """A string whose repr and str never expose the value."""

    __slots__ = ("_value",)

    def __init__(self, value: str) -> None:
        self._value = value

    def reveal(self) -> str:
        """Return the raw value for request construction only."""
        return self._value

    def __repr__(self) -> str:
        return "Secret('***')"

    def __str__(self) -> str:
        return "***"


@dataclass(frozen=True)
class Credentials:
    """One App identity loaded from the canonical credential file."""

    app_id: str
    app_name: str
    api_key: Secret
    server: str = ""
    created_at: str = ""
    updated_at: str = ""
    credential_version: int = DEFAULT_CREDENTIAL_VERSION

    def summary(self) -> dict[str, str]:
        """Return a log-safe summary; the key is always redacted."""
        return {
            "app_id": self.app_id,
            "app_name": self.app_name,
            "server": self.server,
            "api_key": "***",
        }


def profile_path() -> Path:
    """Return the same user profile path used by the Go CLI."""
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
    """Read the small flat YAML profile without adding a PyYAML dependency."""
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


def project_values() -> dict[str, Any]:
    """Read the per-project .levier.json written by ``levier init``."""
    path = Path.cwd() / ".levier.json"
    try:
        payload = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return {}
    return payload if isinstance(payload, dict) else {}


def default_mcp_refs() -> list[str]:
    """Return MCP refs from the environment, project config, or profile."""
    raw = os.environ.get("LEVIER_APP_MCP_REFS", "")
    if not raw.strip():
        value = project_values().get("mcp_refs", "")
        raw = value if isinstance(value, str) else ""
    if not raw.strip():
        raw = profile_values().get("mcp_refs", "")
    return [item.strip() for item in raw.split(",") if item.strip()]


def default_credentials_path(app_name: str = "app") -> Path:
    """Resolve credentials from the environment, project, profile, or config dir."""
    value = os.environ.get("LEVIER_APP_CREDENTIALS") or os.environ.get("LEVIER_CREDENTIALS")
    if value:
        return Path(value).expanduser()
    configured = project_values().get("credentials", "")
    if isinstance(configured, str) and configured:
        return Path(configured).expanduser()
    configured = profile_values().get("credentials", "")
    if configured:
        return Path(configured).expanduser()
    return profile_path().parent / f"{app_name}.json"


def load_credentials(path: str | Path | None = None) -> Credentials:
    """Load and validate one canonical credential file."""
    target = Path(path).expanduser() if path else default_credentials_path()
    try:
        info = target.stat()
    except FileNotFoundError as exc:
        raise CredentialsError(f"credentials file not found: {target}") from exc
    if info.st_mode and stat.S_ISDIR(info.st_mode):
        raise CredentialsError(f"credentials path is a directory: {target}")
    if os.name != "nt" and stat.S_IMODE(info.st_mode) & 0o077:
        raise CredentialsError(f"credentials file must be 0600: {target}")

    try:
        payload = json.loads(target.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise CredentialsError(f"read credentials: {exc}") from exc
    if not isinstance(payload, dict):
        raise CredentialsError("credentials file must contain a JSON object")

    app_id = payload.get("app_id")
    app_name = payload.get("app_name")
    api_key = payload.get("api_key")
    server = payload.get("server", "")
    created_at = payload.get("created_at", "")
    updated_at = payload.get("updated_at", "")
    version = payload.get("credential_version", DEFAULT_CREDENTIAL_VERSION)
    if not isinstance(app_id, str) or not app_id:
        raise CredentialsError("credentials require app_id")
    if not isinstance(app_name, str) or not app_name:
        raise CredentialsError("credentials require app_name")
    if not isinstance(api_key, str) or not api_key:
        raise CredentialsError("credentials require api_key")
    if not all(isinstance(value, str) for value in (server, created_at, updated_at)):
        raise CredentialsError("credentials metadata must be strings")
    if not isinstance(version, int) or version != DEFAULT_CREDENTIAL_VERSION:
        raise CredentialsError(f"unsupported credential_version {version!r}")
    return Credentials(
        app_id=app_id,
        app_name=app_name,
        api_key=Secret(api_key),
        server=server.rstrip("/"),
        created_at=created_at,
        updated_at=updated_at,
        credential_version=version,
    )


def resolve_server(credentials: Credentials, override: str | None = None) -> str:
    """Resolve explicit server, CLI/example env vars, stored server, localhost."""
    project_server = project_values().get("server", "")
    value = (
        override
        or os.environ.get("LEVIER_SERVER")
        or os.environ.get("LEVIER_APP_URL")
        or os.environ.get("LEVIER_URL")
        or (project_server if isinstance(project_server, str) else "")
        or profile_values().get("server", "")
        or credentials.server
        or DEFAULT_SERVER
    )
    return value.rstrip("/")


def credential_fingerprint(credentials: Credentials) -> str:
    """Return a short, non-reversible identifier for local diagnostics."""
    import hashlib

    digest = hashlib.sha256(credentials.api_key.reveal().encode("utf-8")).hexdigest()
    return digest[:12]


def save_credentials(path: str | Path, credentials: Credentials) -> None:
    """Atomically write canonical credentials; useful for tests and tools."""
    target = Path(path).expanduser()
    target.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    payload: dict[str, Any] = {
        "credential_version": DEFAULT_CREDENTIAL_VERSION,
        "app_id": credentials.app_id,
        "app_name": credentials.app_name,
        "api_key": credentials.api_key.reveal(),
        "server": credentials.server.rstrip("/"),
        "created_at": credentials.created_at,
        "updated_at": credentials.updated_at,
    }
    temp = target.with_name(target.name + ".tmp")
    descriptor = os.open(temp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
            json.dump(payload, handle, separators=(",", ":"))
            handle.write("\n")
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temp, target)
        os.chmod(target, 0o600)
    finally:
        if temp.exists():
            temp.unlink()
