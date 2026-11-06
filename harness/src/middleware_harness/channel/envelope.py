"""Middleware — RuntimeChannel envelope helpers.

Builds envelopes with ULID message IDs and response correlation.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import time
from typing import Any

from middleware.runtime.v1 import runtime_pb2
from middleware_harness.ids import new_ulid


def new_envelope(
    payload: Any,
    field: str,
    *,
    msg_id: str | None = None,
    ts_ms: int | None = None,
    run_id: str = "",
    attempt: int = 0,
    lease_epoch: int = 0,
    correlation_id: str = "",
) -> runtime_pb2.Envelope:
    """Build an envelope and copy payload into its oneof field."""
    envelope = runtime_pb2.Envelope(
        msg_id=msg_id or new_ulid(),
        correlation_id=correlation_id,
        run_id=run_id,
        ts_ms=int(time.time() * 1000) if ts_ms is None else ts_ms,
        attempt=attempt,
        lease_epoch=lease_epoch,
    )
    target = getattr(envelope, field)
    target.CopyFrom(payload)
    return envelope


def response_envelope(
    request: runtime_pb2.Envelope,
    payload: Any,
    field: str,
    **kwargs: Any,
) -> runtime_pb2.Envelope:
    """Build a response correlated to request.msg_id."""
    return new_envelope(
        payload,
        field,
        run_id=request.run_id,
        attempt=request.attempt,
        lease_epoch=request.lease_epoch,
        correlation_id=request.msg_id,
        **kwargs,
    )
