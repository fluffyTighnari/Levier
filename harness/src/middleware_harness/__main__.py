"""Middleware — harness command entry point.

The real daemon starts in P3. This skeleton only proves packaging and
importability.

Copyright 2026 The Middleware Authors.
"""

from middleware_harness import __version__


def main() -> int:
    print(f"middleware-harness {__version__} (skeleton)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
