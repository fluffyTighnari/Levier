"""Levier — Counter App runtime credentials.

The bootstrap tool saves the one-time App API key to a 0600 file. The runtime
loads it without needing the Admin token.

Design references:
- docs/DESIGN.md §4.2: App API key authentication.
- docs/DESIGN.md §13.1/§13.2: App keys and secret handling.
- docs/APP_RETROFIT_PLAN.md P3.3.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import json
import os
import stat
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path

from config import Secret

CREDENTIAL_VERSION = 1
DEFAULT_SERVER = "http://127.0.0.1:8080"


class CredentialsError(ValueError):
    """Raised when credential storage is invalid or unsafe."""


@dataclass(frozen=True)
class AppCredentials:
    """App runtime credentials; repr never exposes the API key."""

    app_id: str
    app_name: str
    api_key: Secret
    server: str = ""
    created_at: str = ""
    updated_at: str = ""
    credential_version: int = CREDENTIAL_VERSION

    def summary(self) -> dict[str, str]:
        """Return a log-safe summary."""
        return {
            "app_id": self.app_id,
            "app_name": self.app_name,
            "server": self.server,
            "api_key": str(self.api_key),
        }


def default_credentials_path(app_name: str = "counter-app") -> Path:
    """Return the canonical credentials path, with a legacy read fallback."""
    value = (
        os.environ.get("LEVIER_APP_CREDENTIALS")
        or os.environ.get("LEVIER_CREDENTIALS")
        or os.environ.get("COUNTER_APP_CREDENTIALS")
    )
    if value:
        return Path(value).expanduser()
    current = Path.home() / ".config" / "levier" / f"{app_name}.json"
    legacy = Path.home() / ".config" / "middleware" / f"{app_name}.json"
    if not current.exists() and legacy.exists():
        return legacy
    return current


def resolve_server(value: str, credentials: AppCredentials) -> str:
    """Resolve an explicit server, the stored server, then environment default."""
    selected = (
        value
        or credentials.server
        or os.environ.get("LEVIER_APP_URL")
        or os.environ.get("LEVIER_URL")
        or os.environ.get("MW_URL")
        or DEFAULT_SERVER
    )
    return selected.rstrip("/")


def _validate(credentials: AppCredentials) -> None:
    if not credentials.app_id:
        raise CredentialsError("app_id is required")
    if not credentials.app_name:
        raise CredentialsError("app_name is required")
    if not credentials.api_key.reveal():
        raise CredentialsError("api_key is required")
    if credentials.credential_version != CREDENTIAL_VERSION:
        raise CredentialsError(
            f"unsupported credential_version {credentials.credential_version}; "
            f"expected {CREDENTIAL_VERSION}"
        )


def save_credentials(path: str | Path, credentials: AppCredentials) -> None:
    """Atomically write canonical credentials with 0600 permissions."""
    _validate(credentials)
    target = Path(path).expanduser()
    target.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    payload = {
        "credential_version": CREDENTIAL_VERSION,
        "app_id": credentials.app_id,
        "app_name": credentials.app_name,
        "api_key": credentials.api_key.reveal(),
        "server": credentials.server,
        "created_at": credentials.created_at or now,
        "updated_at": now,
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


def load_credentials(path: str | Path) -> AppCredentials:
    """Load credentials and reject group/world-readable files on POSIX."""
    target = Path(path).expanduser()
    try:
        info = target.stat()
    except FileNotFoundError as exc:
        raise CredentialsError(f"credentials file not found: {target}") from exc
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
    version = payload.get("credential_version", CREDENTIAL_VERSION)
    if (
        not isinstance(app_id, str)
        or not isinstance(app_name, str)
        or not isinstance(api_key, str)
        or not isinstance(server, str)
        or not isinstance(created_at, str)
        or not isinstance(updated_at, str)
        or not isinstance(version, int)
    ):
        raise CredentialsError("credentials file has invalid field types")
    credentials = AppCredentials(
        app_id=app_id,
        app_name=app_name,
        api_key=Secret(api_key),
        server=server.rstrip("/"),
        created_at=created_at,
        updated_at=updated_at,
        credential_version=version,
    )
    _validate(credentials)
    return credentials
