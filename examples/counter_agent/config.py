"""Levier — Counter App HTTP configuration (P2.1).

Reads only environment values. No network calls and no secret printing.

Design references:
- docs/DESIGN.md §4.2: App HTTP prefix and Bearer authentication.
- docs/DESIGN.md §13.1: Admin token is separate from App API keys.
- docs/APP_RETROFIT_PLAN.md P2.1.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path
from typing import Mapping
from urllib.parse import urlsplit

DEFAULT_MW_URL = "http://127.0.0.1:8080"
DEFAULT_APP_NAME = "retrofit-counter"


class ConfigError(ValueError):
    """Raised when App configuration is missing or invalid."""


class Secret:
    """A string whose repr/str never exposes its value."""

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
class AppConfig:
    """Configuration needed to register/submit from the App process."""

    mw_url: str
    admin_token: Secret
    app_name: str

    def summary(self) -> dict[str, str]:
        """Return a log-safe summary."""
        return {
            "mw_url": self.mw_url,
            "admin_token": str(self.admin_token),
            "app_name": self.app_name,
        }


def _required(environ: Mapping[str, str], *keys: str) -> str:
    for key in keys:
        value = environ.get(key, "").strip()
        if value:
            return value
    raise ConfigError(f"{keys[0]} is required")


def _env_value(
    environ: Mapping[str, str],
    *keys: str,
    default: str = "",
) -> str:
    for key in keys:
        value = environ.get(key, "").strip()
        if value:
            return value
    return default


def _validate_url(value: str) -> str:
    parts = urlsplit(value)
    if parts.scheme not in ("http", "https") or not parts.netloc:
        raise ConfigError("LEVIER_URL must be an absolute http(s) URL")
    return value.rstrip("/")


def load_dotenv(path: Path | None = None) -> None:
    """Load the repository .env without python-dotenv or secret printing."""
    target = path or Path(__file__).resolve().parents[2] / ".env"
    if not target.exists():
        return
    for raw in target.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        key = key.strip()
        value = value.strip().strip("\"'")
        if key and key not in os.environ:
            os.environ[key] = value


def from_env(environ: Mapping[str, str] | None = None) -> AppConfig:
    """Load and validate App configuration from environment variables."""
    env = os.environ if environ is None else environ
    mw_url = _validate_url(
        _env_value(
            env,
            "LEVIER_APP_URL",
            "LEVIER_URL",
            "MW_URL",
            default=DEFAULT_MW_URL,
        )
    )
    admin_token = _required(
        env,
        "LEVIER_APP_ADMIN_TOKEN",
        "LEVIER_ADMIN_TOKEN",
        "MW_ADMIN_TOKEN",
    )
    app_name = _env_value(
        env,
        "LEVIER_APP_NAME",
        "MW_APP_NAME",
        default=DEFAULT_APP_NAME,
    ) or DEFAULT_APP_NAME
    return AppConfig(
        mw_url=mw_url,
        admin_token=Secret(admin_token),
        app_name=app_name,
    )
