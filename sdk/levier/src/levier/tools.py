"""Declare ordinary Python functions as Levier tools.

The decorator removes the hand-written JSON schema from the App onboarding
path. Types and defaults are inferred from the function signature; the first
docstring line becomes the model-facing description.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import enum
import inspect
import types
import typing
from dataclasses import dataclass
from typing import Any, Callable, Mapping, Sequence, Union, get_args, get_origin, get_type_hints


@dataclass(frozen=True)
class Tool:
    """One callable capability exposed to a model."""

    name: str
    handler: Callable[..., Any]
    description: str
    schema: Mapping[str, Any]
    timeout_ms: int = 10_000

    def invoke(self, arguments: Mapping[str, Any] | None = None) -> Any:
        """Call the handler with validated keyword arguments."""
        values = dict(arguments or {})
        return self.handler(**values)

    def __call__(self, **arguments: Any) -> Any:
        """Call the handler directly, for example ``counter_add(delta=2)``."""
        return self.handler(**arguments)


def tool(
    function: Callable[..., Any] | str | None = None,
    *,
    name: str | None = None,
    description: str | None = None,
    schema: Mapping[str, Any] | None = None,
    timeout_ms: int = 10_000,
) -> Tool | Callable[[Callable[..., Any]], Tool]:
    """Turn a function into a ``Tool``.

    All of these are equivalent::

        @tool
        def counter_add(delta: int) -> dict: ...

        @tool()
        def counter_add(delta: int) -> dict: ...

        @tool("counter_add")
        def add(delta: int) -> dict: ...
    """

    def decorate(function: Callable[..., Any], explicit_name: str | None = None) -> Tool:
        tool_name = (explicit_name or name or function.__name__).strip()
        if not tool_name:
            raise ValueError("tool name is required")
        if timeout_ms <= 0:
            raise ValueError("tool timeout_ms must be positive")
        return Tool(
            name=tool_name,
            handler=function,
            description=(description or _description(function)).strip(),
            schema=dict(schema) if schema is not None else schema_from_function(function),
            timeout_ms=timeout_ms,
        )

    if callable(function):
        return decorate(function)
    if isinstance(function, str):
        return lambda fn: decorate(fn, function)
    if function is not None:
        raise TypeError("tool() expects a function, a name, or keyword options")
    return decorate


def schema_from_function(function: Callable[..., Any]) -> dict[str, Any]:
    """Build a small JSON Schema from one function signature."""
    signature = inspect.signature(function)
    try:
        hints = get_type_hints(function)
    except (NameError, TypeError):
        hints = {}

    properties: dict[str, Any] = {}
    required: list[str] = []
    for parameter in signature.parameters.values():
        if parameter.kind in (parameter.VAR_POSITIONAL, parameter.VAR_KEYWORD):
            continue
        if parameter.name in {"self", "cls"}:
            continue
        annotation = hints.get(parameter.name, parameter.annotation)
        field = _json_schema(annotation)
        if parameter.default is inspect.Parameter.empty:
            required.append(parameter.name)
        elif _json_value(parameter.default):
            field["default"] = parameter.default
        properties[parameter.name] = field

    schema: dict[str, Any] = {
        "type": "object",
        "properties": properties,
        "additionalProperties": False,
    }
    if required:
        schema["required"] = required
    return schema


def _description(function: Callable[..., Any]) -> str:
    doc = inspect.getdoc(function) or ""
    return doc.splitlines()[0].strip() if doc.splitlines() else ""


def _json_schema(annotation: Any) -> dict[str, Any]:
    if annotation is inspect.Parameter.empty or annotation is Any:
        return {}
    origin = get_origin(annotation)
    if origin is Union or origin is types.UnionType:
        options = [item for item in get_args(annotation) if item is not type(None)]
        if len(options) == 1:
            return _json_schema(options[0])
        return {"anyOf": [_json_schema(item) for item in options]}
    if origin is typing.Annotated:
        return _json_schema(get_args(annotation)[0])
    if origin is typing.Literal:
        values = list(get_args(annotation))
        return {"enum": values}
    if origin in (list, Sequence, typing.Sequence, tuple, set, frozenset):
        args = get_args(annotation)
        result: dict[str, Any] = {"type": "array"}
        if args:
            result["items"] = _json_schema(args[0])
        return result
    if origin in (dict, Mapping, typing.Mapping):
        result = {"type": "object"}
        args = get_args(annotation)
        if len(args) == 2 and args[1] is not Any:
            result["additionalProperties"] = _json_schema(args[1])
        return result

    if isinstance(annotation, type) and issubclass(annotation, enum.Enum):
        return {"type": "string", "enum": [item.value for item in annotation]}
    mapping = {
        str: {"type": "string"},
        int: {"type": "integer"},
        float: {"type": "number"},
        bool: {"type": "boolean"},
        dict: {"type": "object"},
        list: {"type": "array"},
    }
    return dict(mapping.get(annotation, {}))


def _json_value(value: Any) -> bool:
    return isinstance(value, (str, int, float, bool)) or value is None


__all__ = ["Tool", "schema_from_function", "tool"]
