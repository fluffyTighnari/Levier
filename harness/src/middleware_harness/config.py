"""Middleware — harness configuration.

Loads YAML configuration, applies HARNESS_* overrides, and validates the
result with the Python standard library.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Mapping

import yaml

RUNTIME_TYPES = {"normal", "sandbox", "longterm"}
LOG_LEVELS = {"debug", "info", "warning", "error"}
LOG_FORMATS = {"json", "text"}


class ConfigError(RuntimeError):
    """Raised when configuration cannot be loaded or validated."""


@dataclass(frozen=True)
class SecretToken:
    """A token whose repr never exposes its value."""

    _value: str

    def get_secret_value(self) -> str:
        """Return the raw secret for connection setup only."""
        return self._value

    def __repr__(self) -> str:
        return "SecretToken('***')"

    def __str__(self) -> str:
        return "***"


@dataclass
class MiddlewareSettings:
    """Middleware connection settings."""

    address: str = "127.0.0.1:9090"
    tls: bool = False


@dataclass
class DaemonSettings:
    """Daemon identity and capacity settings."""

    id: str
    auth_token: SecretToken
    runtime_type: str = "normal"
    max_concurrent_runs: int = 4
    strategies: list[str] = field(default_factory=lambda: ["react", "echo"])
    labels: dict[str, str] = field(default_factory=dict)


@dataclass
class LoggingSettings:
    """Structured logging settings."""

    level: str = "info"
    format: str = "json"


@dataclass
class Settings:
    """Complete harness configuration."""

    middleware: MiddlewareSettings = field(default_factory=MiddlewareSettings)
    daemon: DaemonSettings = field(default_factory=lambda: DaemonSettings(
        id="", auth_token=SecretToken("")
    ))
    logging: LoggingSettings = field(default_factory=LoggingSettings)

    def summary(self) -> dict[str, Any]:
        """Return a log-safe summary without the daemon token."""
        return {
            "middleware_address": self.middleware.address,
            "middleware_tls": self.middleware.tls,
            "daemon_id": self.daemon.id,
            "runtime_type": self.daemon.runtime_type,
            "max_concurrent_runs": self.daemon.max_concurrent_runs,
            "strategies": list(self.daemon.strategies),
            "labels": dict(self.daemon.labels),
            "log_level": self.logging.level,
            "log_format": self.logging.format,
        }


def load_settings(path: str | Path, environ: Mapping[str, str] | None = None) -> Settings:
    """Load YAML, apply environment overrides, and validate settings."""
    config_path = Path(path)
    env = os.environ if environ is None else environ

    try:
        raw_text = config_path.read_text(encoding="utf-8")
    except OSError as exc:
        raise ConfigError(f"read config {config_path}: {exc}") from exc

    try:
        raw = yaml.safe_load(raw_text) or {}
    except yaml.YAMLError as exc:
        raise ConfigError(f"parse config {config_path}: {exc}") from exc
    if not isinstance(raw, dict):
        raise ConfigError(f"config {config_path} must be a mapping")

    data: dict[str, Any] = _deep_copy(raw)
    _apply_env(data, env)

    try:
        return _parse_settings(data)
    except ConfigError as exc:
        raise ConfigError(f"validate config {config_path}: {exc}") from exc


def _parse_settings(data: dict[str, Any]) -> Settings:
    middleware_data = _section(data, "middleware")
    middleware = MiddlewareSettings(
        address=_string(middleware_data, "middleware.address", "127.0.0.1:9090", required=True),
        tls=_boolean(middleware_data, "middleware.tls", False),
    )

    daemon_data = _section(data, "daemon", required=True)
    daemon = DaemonSettings(
        id=_string(daemon_data, "daemon.id", "", required=True),
        auth_token=SecretToken(_string(daemon_data, "daemon.auth_token", "", required=True)),
        runtime_type=_runtime_type(daemon_data),
        max_concurrent_runs=_positive_int(daemon_data, "daemon.max_concurrent_runs", 4),
        strategies=_string_list(daemon_data, "daemon.strategies", ["react", "echo"]),
        labels=_string_map(daemon_data, "daemon.labels"),
    )

    logging_data = _section(data, "logging")
    level = _string(logging_data, "logging.level", "info", required=False)
    if level not in LOG_LEVELS:
        raise ConfigError(f"logging.level must be one of {sorted(LOG_LEVELS)}")
    log_format = _string(logging_data, "logging.format", "json", required=False)
    if log_format not in LOG_FORMATS:
        raise ConfigError(f"logging.format must be one of {sorted(LOG_FORMATS)}")

    return Settings(
        middleware=middleware,
        daemon=daemon,
        logging=LoggingSettings(level=level, format=log_format),
    )


def _section(data: dict[str, Any], name: str, required: bool = False) -> dict[str, Any]:
    value = data.get(name, {})
    if value is None:
        value = {}
    if not isinstance(value, dict):
        raise ConfigError(f"config section {name!r} must be a mapping")
    if required and not value:
        raise ConfigError(f"config section {name!r} is required")
    return value


def _string(
    section: dict[str, Any],
    name: str,
    default: str,
    required: bool,
) -> str:
    value = section.get(name.rsplit(".", 1)[-1], default)
    if not isinstance(value, str):
        raise ConfigError(f"{name} must be a string")
    if required and not value:
        raise ConfigError(f"{name} is required")
    return value


def _boolean(section: dict[str, Any], name: str, default: bool) -> bool:
    value = section.get(name.rsplit(".", 1)[-1], default)
    if not isinstance(value, bool):
        raise ConfigError(f"{name} must be a boolean")
    return value


def _positive_int(section: dict[str, Any], name: str, default: int) -> int:
    value = section.get(name.rsplit(".", 1)[-1], default)
    if isinstance(value, bool) or not isinstance(value, int):
        raise ConfigError(f"{name} must be an integer")
    if value <= 0:
        raise ConfigError(f"{name} must be positive")
    return value


def _runtime_type(section: dict[str, Any]) -> str:
    value = section.get("runtime_type", "normal")
    if not isinstance(value, str) or value not in RUNTIME_TYPES:
        raise ConfigError(f"daemon.runtime_type must be one of {sorted(RUNTIME_TYPES)}")
    return value


def _string_list(section: dict[str, Any], name: str, default: list[str]) -> list[str]:
    value = section.get(name.rsplit(".", 1)[-1], default)
    if not isinstance(value, list) or not value:
        raise ConfigError(f"{name} must be a non-empty list")
    result: list[str] = []
    for item in value:
        if not isinstance(item, str) or not item:
            raise ConfigError(f"{name} must contain non-empty strings")
        result.append(item)
    return result


def _string_map(section: dict[str, Any], name: str) -> dict[str, str]:
    value = section.get(name.rsplit(".", 1)[-1], {})
    if not isinstance(value, dict):
        raise ConfigError(f"{name} must be a mapping")
    result: dict[str, str] = {}
    for key, item in value.items():
        if not isinstance(key, str) or not key:
            raise ConfigError(f"{name} keys must be non-empty strings")
        if not isinstance(item, str):
            raise ConfigError(f"{name}.{key} must be a string")
        result[key] = item
    return result


def _deep_copy(value: dict[str, Any]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, item in value.items():
        if isinstance(item, dict):
            result[key] = _deep_copy(item)
        else:
            result[key] = item
    return result


def _apply_env(data: dict[str, Any], env: Mapping[str, str]) -> None:
    _set(data, "middleware", "address", env.get("HARNESS_MIDDLEWARE_ADDRESS"))
    _set_bool(data, "middleware", "tls", env.get("HARNESS_MIDDLEWARE_TLS"))

    _set(data, "daemon", "id", env.get("HARNESS_DAEMON_ID"))
    _set(data, "daemon", "auth_token", env.get("HARNESS_DAEMON_AUTH_TOKEN"))
    _set(data, "daemon", "runtime_type", env.get("HARNESS_DAEMON_RUNTIME_TYPE"))
    _set_int(
        data,
        "daemon",
        "max_concurrent_runs",
        env.get("HARNESS_DAEMON_MAX_CONCURRENT_RUNS"),
    )
    _set_list(data, "daemon", "strategies", env.get("HARNESS_DAEMON_STRATEGIES"))
    _set_map(data, "daemon", "labels", env.get("HARNESS_DAEMON_LABELS"))

    _set(data, "logging", "level", env.get("HARNESS_LOGGING_LEVEL"))
    _set(data, "logging", "format", env.get("HARNESS_LOGGING_FORMAT"))


def _set(data: dict[str, Any], section: str, key: str, value: str | None) -> None:
    if value is not None:
        _section(data, section)[key] = value


def _set_bool(data: dict[str, Any], section: str, key: str, value: str | None) -> None:
    if value is None:
        return
    normalized = value.strip().lower()
    if normalized not in {"1", "0", "true", "false", "yes", "no"}:
        raise ConfigError(f"{section}.{key} must be a boolean, got {value!r}")
    _section(data, section)[key] = normalized in {"1", "true", "yes"}


def _set_int(data: dict[str, Any], section: str, key: str, value: str | None) -> None:
    if value is None:
        return
    try:
        parsed = int(value)
    except ValueError as exc:
        raise ConfigError(f"{section}.{key} must be an integer, got {value!r}") from exc
    _section(data, section)[key] = parsed


def _set_list(data: dict[str, Any], section: str, key: str, value: str | None) -> None:
    if value is None:
        return
    items = [item.strip() for item in value.split(",") if item.strip()]
    _section(data, section)[key] = items


def _set_map(data: dict[str, Any], section: str, key: str, value: str | None) -> None:
    if value is None:
        return
    result: dict[str, str] = {}
    for pair in value.split(","):
        if not pair.strip():
            continue
        if "=" not in pair:
            raise ConfigError(f"{section}.{key} entries must be key=value, got {pair!r}")
        map_key, map_value = pair.split("=", 1)
        result[map_key.strip()] = map_value.strip()
    _section(data, section)[key] = result
