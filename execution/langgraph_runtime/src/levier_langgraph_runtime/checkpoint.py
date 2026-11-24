"""Persist LangGraph checkpoint state through a Levier Run checkpoint.

The adapter stores an in-memory LangGraph checkpointer snapshot in the opaque
Levier checkpoint blob. It deliberately avoids pickle: checkpoint bytes may
come from a remote daemon, so every persisted value is JSON plus base64.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import base64
import json
from collections import defaultdict
from typing import Any, Sequence

from langgraph.checkpoint.memory import InMemorySaver

CHECKPOINT_VERSION = 1
_PACK_TAG = "__levier_type__"


class LangGraphCheckpointError(ValueError):
    """Raised when a LangGraph checkpoint cannot be encoded or restored."""


class LevierCheckpointSaver(InMemorySaver):
    """In-memory LangGraph saver that mirrors its state to RunContext."""

    def __init__(self, ctx: Any, *, thread_id: str) -> None:
        super().__init__()
        self._ctx = ctx
        self._thread_id = thread_id

    async def aput(self, config, checkpoint, metadata, new_versions):
        result = await super().aput(config, checkpoint, metadata, new_versions)
        await self._persist()
        return result

    async def aput_writes(
        self,
        config,
        writes: Sequence[tuple[str, Any]],
        task_id: str,
        task_path: str = "",
    ) -> None:
        # Writes are persisted as part of the next checkpoint. Mirroring every
        # intermediate write to the control plane would spam checkpoint events
        # without improving crash recovery of a completed graph step.
        await super().aput_writes(config, writes, task_id, task_path)

    async def _persist(self) -> None:
        if not getattr(self._ctx, "session_id", None):
            return
        await self._ctx.checkpoint(self.dump())

    def dump(self) -> bytes:
        """Encode the full in-memory checkpointer state."""
        payload = {
            "version": CHECKPOINT_VERSION,
            "runtime_name": getattr(self._ctx, "runtime_name", ""),
            "session_id": getattr(self._ctx, "session_id", "") or "",
            "thread_id": self._thread_id,
            "storage": _pack(self.storage),
            "writes": _pack(self.writes),
            "blobs": _pack(self.blobs),
        }
        return json.dumps(
            payload,
            ensure_ascii=False,
            separators=(",", ":"),
        ).encode("utf-8")

    def load(self, data: bytes) -> None:
        """Restore the full in-memory checkpointer state from checkpoint bytes."""
        try:
            payload = json.loads(data.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as exc:
            raise LangGraphCheckpointError("LangGraph checkpoint is not valid JSON") from exc
        if not isinstance(payload, dict):
            raise LangGraphCheckpointError("LangGraph checkpoint must be an object")
        if payload.get("version") != CHECKPOINT_VERSION:
            raise LangGraphCheckpointError(
                f"unsupported LangGraph checkpoint version {payload.get('version')!r}"
            )
        expected_runtime = getattr(self._ctx, "runtime_name", "")
        if expected_runtime and payload.get("runtime_name") != expected_runtime:
            raise LangGraphCheckpointError(
                "LangGraph checkpoint belongs to another runtime"
            )
        expected_session = getattr(self._ctx, "session_id", "") or ""
        if expected_session and payload.get("session_id") != expected_session:
            raise LangGraphCheckpointError(
                "LangGraph checkpoint belongs to another session"
            )
        if payload.get("thread_id") != self._thread_id:
            raise LangGraphCheckpointError(
                "LangGraph checkpoint belongs to another thread"
            )

        storage = _unpack(payload.get("storage"))
        writes = _unpack(payload.get("writes"))
        blobs = _unpack(payload.get("blobs"))
        if not isinstance(storage, dict) or not isinstance(writes, dict) or not isinstance(blobs, dict):
            raise LangGraphCheckpointError("LangGraph checkpoint state is invalid")

        self.storage = defaultdict(lambda: defaultdict(dict))
        for thread_id, namespaces in storage.items():
            if not isinstance(thread_id, str) or not isinstance(namespaces, dict):
                raise LangGraphCheckpointError("LangGraph storage is invalid")
            for checkpoint_ns, checkpoints in namespaces.items():
                if not isinstance(checkpoints, dict):
                    raise LangGraphCheckpointError("LangGraph namespace is invalid")
                for checkpoint_id, entry in checkpoints.items():
                    self.storage[thread_id][checkpoint_ns][checkpoint_id] = entry
        self.writes = defaultdict(dict)
        self.writes.update(writes)
        self.blobs = defaultdict(dict)
        self.blobs.update(blobs)


def _pack(value: Any) -> Any:
    if isinstance(value, bytes):
        return {_PACK_TAG: "bytes", "value": base64.b64encode(value).decode("ascii")}
    if isinstance(value, tuple):
        return {_PACK_TAG: "tuple", "value": [_pack(item) for item in value]}
    if isinstance(value, list):
        return {_PACK_TAG: "list", "value": [_pack(item) for item in value]}
    if isinstance(value, dict):
        return {
            _PACK_TAG: "dict",
            "value": [[_pack(key), _pack(item)] for key, item in value.items()],
        }
    if value is None or isinstance(value, (str, int, float, bool)):
        return value
    raise LangGraphCheckpointError(
        f"LangGraph checkpoint contains unsupported value {type(value).__name__}"
    )


def _unpack(value: Any) -> Any:
    if not isinstance(value, dict) or _PACK_TAG not in value:
        return value
    kind = value.get(_PACK_TAG)
    payload = value.get("value")
    if kind == "bytes":
        try:
            return base64.b64decode(str(payload).encode("ascii"), validate=True)
        except (ValueError, TypeError) as exc:
            raise LangGraphCheckpointError("invalid base64 in LangGraph checkpoint") from exc
    if kind == "tuple":
        if not isinstance(payload, list):
            raise LangGraphCheckpointError("invalid tuple in LangGraph checkpoint")
        return tuple(_unpack(item) for item in payload)
    if kind == "list":
        if not isinstance(payload, list):
            raise LangGraphCheckpointError("invalid list in LangGraph checkpoint")
        return [_unpack(item) for item in payload]
    if kind == "dict":
        if not isinstance(payload, list):
            raise LangGraphCheckpointError("invalid dict in LangGraph checkpoint")
        result = {}
        for pair in payload:
            if not isinstance(pair, list) or len(pair) != 2:
                raise LangGraphCheckpointError("invalid dict pair in LangGraph checkpoint")
            key = _unpack(pair[0])
            if not isinstance(key, (str, int, float, bool, tuple, bytes)):
                raise LangGraphCheckpointError("invalid dict key in LangGraph checkpoint")
            result[key] = _unpack(pair[1])
        return result
    raise LangGraphCheckpointError(f"unknown LangGraph checkpoint value {kind!r}")
