"""Deprecated compatibility shim for levier_harness.

Use ``levier_harness`` in new code. This shim is removed after one release.
"""

from __future__ import annotations

import sys

import levier_harness as _impl

sys.modules[__name__] = _impl
