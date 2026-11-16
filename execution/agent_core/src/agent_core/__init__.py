"""Deprecated compatibility shim for levier_agent_core.

Use ``levier_agent_core`` in new code. This shim is removed after one release.
"""

from __future__ import annotations

import sys

import levier_agent_core as _impl

sys.modules[__name__] = _impl
