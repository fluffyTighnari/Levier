"""Deprecated compatibility shim for levier_mcp_runtime.

Use ``levier_mcp_runtime`` in new code. This shim is removed after one release.
"""

from __future__ import annotations

import sys

import levier_mcp_runtime as _impl

sys.modules[__name__] = _impl
