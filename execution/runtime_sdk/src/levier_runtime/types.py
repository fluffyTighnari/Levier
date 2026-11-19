"""Plain public data types for the Levier Runtime SDK.

Runtime authors should never need to import protobuf types directly. The SDK
maps wire messages into these small dataclasses and maps them back internally.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Mapping

from middleware.runtime.v1 import runtime_pb2


@dataclass(frozen=True)
class Message:
    """One conversation message supplied in a TaskAssign."""

    role: str
    content: str = ""
    name: str = ""

    @classmethod
    def from_proto(cls, value: runtime_pb2.ChatMessage) -> "Message":
        return cls(role=value.role, content=value.content, name=value.name)


@dataclass(frozen=True)
class ModelConfig:
    """Optional model settings for a runtime that calls an LLM."""

    provider: str
    model: str
    api_key: str = field(default="", repr=False)
    base_url: str = ""
    params: Mapping[str, str] = field(default_factory=dict)

    @classmethod
    def from_proto(cls, value: runtime_pb2.ModelProfile | None) -> "ModelConfig | None":
        if value is None or not value.provider or not value.model:
            return None
        return cls(
            provider=value.provider,
            model=value.model,
            api_key=value.api_key,
            base_url=value.base_url,
            params=dict(value.params),
        )


@dataclass(frozen=True)
class ToolSpec:
    """One tool visible to the runtime, as declared by the control plane."""

    name: str
    description: str = ""
    input_schema_json: str = "{}"
    scope: str = "global"

    @classmethod
    def from_proto(cls, value: runtime_pb2.ToolManifest) -> "ToolSpec":
        return cls(
            name=value.name,
            description=value.description,
            input_schema_json=value.input_schema_json,
            scope=runtime_pb2.ToolScope.Name(value.scope).lower(),
        )


@dataclass(frozen=True)
class McpBinding:
    """Optional MCP connection descriptor supplied by the control plane."""

    server_name: str
    transport: str
    command: str = ""
    args: tuple[str, ...] = ()
    env: Mapping[str, str] = field(default_factory=dict)
    url: str = ""
    headers: Mapping[str, str] = field(default_factory=dict)
    allowed_tools: tuple[str, ...] = ()
    namespace: str = ""

    @classmethod
    def from_proto(cls, value: runtime_pb2.McpBinding) -> "McpBinding":
        return cls(
            server_name=value.server_name,
            transport=value.transport,
            command=value.command,
            args=tuple(value.args),
            env=dict(value.env),
            url=value.url,
            headers=dict(value.headers),
            allowed_tools=tuple(value.allowed_tools),
            namespace=value.namespace,
        )


@dataclass(frozen=True)
class Timeouts:
    """Run and per-tool deadlines in milliseconds."""

    run_ms: int = 0
    tool_ms: int = 0


@dataclass(frozen=True)
class ToolResult:
    """The plain result of a `ctx.call_tool()` request."""

    ok: bool
    result_json: str = ""
    error_code: str = ""
    error_message: str = ""
    retryable: bool = False

    @classmethod
    def from_proto(cls, value: runtime_pb2.ToolCallResult) -> "ToolResult":
        error = value.error
        return cls(
            ok=value.ok,
            result_json=value.result_json,
            error_code=error.code if error is not None else "",
            error_message=error.message if error is not None else "",
            retryable=error.retryable if error is not None else False,
        )


def params_from_proto(value: Any) -> dict[str, Any]:
    """Convert a protobuf Struct into a plain dictionary."""
    if value is None:
        return {}
    # Struct behaves like a mapping of field names to protobuf Values. The
    # JSON form is simpler and matches what App/Runtime authors expect.
    from google.protobuf.json_format import MessageToDict

    try:
        result = MessageToDict(value)
    except Exception:
        return {}
    return result if isinstance(result, dict) else {}
