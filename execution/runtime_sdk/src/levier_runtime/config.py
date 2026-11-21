"""Runtime configuration loading for the Levier Runtime SDK.

The public config is intentionally small: middleware connection, one runtime
identity, capacity, labels, and logging. Optional capabilities are configured
by the runtime implementation, not by the SDK.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Mapping

import yaml


class ConfigError(ValueError):
    """Raised when a runtime configuration file is invalid."""


@dataclass(frozen=True)
class MiddlewareConfig:
    """Levier RuntimeChannel connection settings."""

    address: str
    auth_token: str = field(default="", repr=False)
    auth_token_file: str = ""
    tls: bool = False


@dataclass(frozen=True)
class RuntimeConfig:
    """Identity and capacity for one runtime process."""

    runtime_name: str
    instance_id: str
    middleware: MiddlewareConfig
    runtime_type: str = "normal"
    max_concurrent_runs: int = 4
    labels: Mapping[str, str] = field(default_factory=dict)
    log_level: str = "info"


def load_runtime_config(
    path: str | os.PathLike[str] | None = None,
    *,
    environ: Mapping[str, str] | None = None,
) -> RuntimeConfig:
    """Load `runtime.yaml`, then apply `LEVIER_RUNTIME_*` overrides.

    A runtime can start with no YAML at all. In that case the SDK derives the
    server address, runtime name, and local runtime credential from the user
    profile written by ``levier up``/``levier init``.
    """
    env = os.environ if environ is None else environ
    from .profile import default_runtime_config

    configured = path or env.get("LEVIER_RUNTIME_CONFIG", "") or default_runtime_config()
    data: dict[str, Any] = {}
    label = "defaults"
    use_profile_defaults = True
    if configured and str(configured) != ".":
        config_path = Path(configured)
        label = str(config_path)
        try:
            raw = yaml.safe_load(config_path.read_text(encoding="utf-8"))
        except OSError as exc:
            raise ConfigError(f"read config {config_path}: {exc}") from exc
        except yaml.YAMLError as exc:
            raise ConfigError(f"parse config {config_path}: {exc}") from exc
        if raw is None:
            raw = {}
        if not isinstance(raw, dict):
            raise ConfigError(f"config {config_path} must be a mapping")
        data = _deep_copy(raw)
        # An empty placeholder file is the no-config case. A non-empty file is
        # explicit and keeps strict field validation.
        use_profile_defaults = not bool(data)

    _apply_env(data, env)
    if use_profile_defaults:
        _apply_profile_defaults(data, env)
    try:
        return _parse_config(data, env, allow_local_defaults=use_profile_defaults)
    except ConfigError as exc:
        raise ConfigError(f"validate config {label}: {exc}") from exc


def _apply_profile_defaults(data: dict[str, Any], env: Mapping[str, str]) -> None:
    from .profile import (
        default_middleware_address,
        default_runtime_name,
        default_runtime_token,
    )

    middleware = data.get("middleware")
    if not isinstance(middleware, dict):
        middleware = {}
        data["middleware"] = middleware
    if not middleware.get("address") and not env.get("LEVIER_MIDDLEWARE_ADDRESS"):
        address = default_middleware_address() or "127.0.0.1:9090"
        middleware["address"] = address
    if not middleware.get("auth_token") and not middleware.get("auth_token_file"):
        token = env.get("LEVIER_RUNTIME_TOKEN") or default_runtime_token()
        if token:
            middleware["auth_token"] = token

    runtime = data.get("runtime")
    if not isinstance(runtime, dict):
        runtime = {}
        data["runtime"] = runtime
    if not runtime.get("name"):
        runtime["name"] = env.get("LEVIER_RUNTIME_NAME") or default_runtime_name() or "local"
    if not runtime.get("instance_id"):
        import socket

        runtime["instance_id"] = f"{runtime['name']}-{socket.gethostname()}-{os.getpid()}"
    if not runtime.get("type"):
        runtime["type"] = "normal"
    if not runtime.get("max_concurrent_runs"):
        runtime["max_concurrent_runs"] = 4
    if not data.get("logging"):
        data["logging"] = {"level": "info"}


def _parse_config(
    data: dict[str, Any],
    env: Mapping[str, str],
    *,
    allow_local_defaults: bool = False,
) -> RuntimeConfig:
    middleware_data = _section(data, "middleware", required=True)
    runtime_data = _section(data, "runtime", required=True)
    address = _string(middleware_data, "middleware.address", "127.0.0.1:9090")
    auth_token = _string(middleware_data, "middleware.auth_token", "")
    auth_token_file = _string(middleware_data, "middleware.auth_token_file", "")
    if not auth_token and auth_token_file:
        auth_token = _read_secret_file(auth_token_file)
    if not auth_token and allow_local_defaults and _is_local_address(address):
        # The local Compose stack ships with this development credential.
        auth_token = env.get("LEVIER_RUNTIME_TOKEN") or "change-me"
    if not auth_token:
        raise ConfigError(
            "middleware.auth_token or middleware.auth_token_file is required "
            "(or run `levier up`, which records the local runtime credential)"
        )

    runtime_name = _string(runtime_data, "runtime.name", "", required=True)
    instance_id = _string(runtime_data, "runtime.instance_id", "", required=True)
    runtime_type = _string(runtime_data, "runtime.type", "normal")
    if runtime_type not in {"normal", "sandbox", "longterm"}:
        raise ConfigError("runtime.type must be normal, sandbox, or longterm")
    max_concurrent_runs = _positive_int(runtime_data, "runtime.max_concurrent_runs", 4)
    labels = _string_map(runtime_data, "runtime.labels")
    logging_data = _section(data, "logging", required=False)
    log_level = _string(logging_data, "logging.level", "info")

    tls = _boolean(middleware_data, "middleware.tls", False)
    middleware = MiddlewareConfig(
        address=address,
        auth_token=auth_token,
        auth_token_file=auth_token_file,
        tls=tls,
    )
    return RuntimeConfig(
        runtime_name=runtime_name,
        instance_id=instance_id,
        middleware=middleware,
        runtime_type=runtime_type,
        max_concurrent_runs=max_concurrent_runs,
        labels=labels,
        log_level=log_level,
    )


def _apply_env(data: dict[str, Any], env: Mapping[str, str]) -> None:
    _set(data, "middleware", "address", env.get("LEVIER_MIDDLEWARE_ADDRESS"))
    _set(data, "middleware", "tls", env.get("LEVIER_MIDDLEWARE_TLS"))
    _set(data, "middleware", "auth_token", env.get("LEVIER_RUNTIME_TOKEN"))
    _set(data, "middleware", "auth_token_file", env.get("LEVIER_RUNTIME_TOKEN_FILE"))
    _set(data, "runtime", "name", env.get("LEVIER_RUNTIME_NAME"))
    _set(data, "runtime", "instance_id", env.get("LEVIER_RUNTIME_INSTANCE_ID"))
    _set(data, "runtime", "type", env.get("LEVIER_RUNTIME_TYPE"))
    _set(data, "runtime", "max_concurrent_runs", env.get("LEVIER_RUNTIME_MAX_CONCURRENT_RUNS"))
    labels = env.get("LEVIER_RUNTIME_LABELS")
    if labels is not None:
        _section(data, "runtime", required=True)["labels"] = _parse_labels(labels)
    _set(data, "logging", "level", env.get("LEVIER_RUNTIME_LOG_LEVEL"))


def _section(data: dict[str, Any], name: str, *, required: bool) -> dict[str, Any]:
    value = data.get(name)
    if value is None:
        if required:
            raise ConfigError(f"{name} is required")
        return {}
    if not isinstance(value, dict):
        raise ConfigError(f"{name} must be a mapping")
    return value


def _string(
    section: dict[str, Any],
    name: str,
    default: str,
    *,
    required: bool = False,
) -> str:
    value = section.get(name.rsplit(".", 1)[-1], default)
    if not isinstance(value, str):
        raise ConfigError(f"{name} must be a string")
    if required and not value:
        raise ConfigError(f"{name} is required")
    return value


def _positive_int(section: dict[str, Any], name: str, default: int) -> int:
    value = section.get(name.rsplit(".", 1)[-1], default)
    if isinstance(value, bool) or not isinstance(value, int) or value <= 0:
        raise ConfigError(f"{name} must be a positive integer")
    return value


def _boolean(section: dict[str, Any], name: str, default: bool) -> bool:
    value = section.get(name.rsplit(".", 1)[-1], default)
    if isinstance(value, bool):
        return value
    if isinstance(value, str):
        lowered = value.lower()
        if lowered in {"true", "1", "yes"}:
            return True
        if lowered in {"false", "0", "no"}:
            return False
    raise ConfigError(f"{name} must be a boolean")


def _string_map(section: dict[str, Any], name: str) -> dict[str, str]:
    value = section.get(name.rsplit(".", 1)[-1], {})
    if not isinstance(value, dict):
        raise ConfigError(f"{name} must be a mapping")
    result: dict[str, str] = {}
    for key, item in value.items():
        if not isinstance(key, str) or not isinstance(item, str):
            raise ConfigError(f"{name} keys and values must be strings")
        result[key] = item
    return result


def _is_local_address(address: str) -> bool:
    host = address.strip()
    if host.startswith("["):
        host = host[1:].split("]", 1)[0]
    elif ":" in host:
        host = host.rsplit(":", 1)[0]
    return host in {"127.0.0.1", "localhost", "::1", "0.0.0.0", ""}


def _read_secret_file(path: str) -> str:
    try:
        return Path(path).read_text(encoding="utf-8").strip()
    except OSError as exc:
        raise ConfigError(f"read secret file {path}: {exc}") from exc


def _parse_labels(value: str) -> dict[str, str]:
    labels: dict[str, str] = {}
    for item in value.split(","):
        key, separator, label_value = item.partition("=")
        if not separator or not key.strip():
            raise ConfigError("LEVIER_RUNTIME_LABELS must use key=value,key=value")
        labels[key.strip()] = label_value.strip()
    return labels


def _set(data: dict[str, Any], section: str, key: str, value: str | None) -> None:
    if value is None:
        return
    _section(data, section, required=True)[key] = value


def _deep_copy(value: Any) -> Any:
    if isinstance(value, dict):
        return {key: _deep_copy(item) for key, item in value.items()}
    if isinstance(value, list):
        return [_deep_copy(item) for item in value]
    return value
