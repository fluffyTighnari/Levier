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
from pathlib import Path

from config import Secret


class CredentialsError(ValueError):
    """Raised when credential storage is invalid or unsafe."""


@dataclass(frozen=True)
class AppCredentials:
    """App runtime credentials; repr never exposes the API key."""

    app_id: str
    app_name: str
    api_key: Secret

    def summary(self) -> dict[str, str]:
        """Return a log-safe summary."""
        return {
            "app_id": self.app_id,
            "app_name": self.app_name,
            "api_key": str(self.api_key),
        }


def _validate(credentials: AppCredentials) -> None:
    if not credentials.app_id:
        raise CredentialsError("app_id is required")
    if not credentials.app_name:
        raise CredentialsError("app_name is required")
    if not credentials.api_key.reveal():
        raise CredentialsError("api_key is required")


def save_credentials(path: str | Path, credentials: AppCredentials) -> None:
    """Atomically write credentials with 0600 permissions."""
    _validate(credentials)
    target = Path(path).expanduser()
    target.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    payload = {
        "app_id": credentials.app_id,
        "app_name": credentials.app_name,
        "api_key": credentials.api_key.reveal(),
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
    if not isinstance(app_id, str) or not isinstance(app_name, str) or not isinstance(api_key, str):
        raise CredentialsError("credentials file requires app_id, app_name, and api_key")
    credentials = AppCredentials(app_id=app_id, app_name=app_name, api_key=Secret(api_key))
    _validate(credentials)
    return credentials
