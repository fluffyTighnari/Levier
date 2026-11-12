#!/usr/bin/env python3
"""Middleware — scan tracked files for accidental secrets.

Only git-tracked files are scanned, so local .env files and untracked evidence
cannot be committed by this check. Generated code and recorded artifacts are
excluded because they are not source.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import re
import subprocess
import sys

EXCLUDE_PREFIXES = (
    "docs/reports/",
    "gen/",
    "harness/src/gen/",
    "workflow_engine/",
)

PATTERNS = {
    "openai-style key": re.compile(r"sk-[A-Za-z0-9]{20,}"),
    "bearer token": re.compile(r"Bearer\s+[A-Za-z0-9._~+/=-]{20,}"),
    "postgres password": re.compile(r"postgres(?:ql)?://[^\s\"']+:[^\s\"']+@"),
    "deepseek key": re.compile(r"DEEPSEEK_API_KEY=[A-Za-z0-9_./+~-]{16,}"),
    "inline api key": re.compile(
        r"(?i)(?:api[_-]?key|secret|token)\s*[:=]\s*(?:[\"'][^\"']{20,}[\"']|[A-Z][A-Z0-9_]{2,}\s*=\s*[A-Za-z0-9_./+~-]{32,})"
    ),
}

PLACEHOLDER = re.compile(r"change-me|\$\{|<[^>]+>|\[REDACTED\]|example")


def tracked_files() -> list[str]:
    output = subprocess.check_output(["git", "ls-files", "-z"], cwd=".")
    return [
        item.decode("utf-8")
        for item in output.split(b"\0")
        if item and not item.decode("utf-8").startswith(EXCLUDE_PREFIXES)
    ]


SELF_TEST_SAMPLES = {
    "openai-style key": "sk-" + "a" * 32,
    "bearer token": "Bearer " + "a" * 32,
    "postgres password": "postgres://user:real-password@db.example/middleware",
    "deepseek key": "DEEPSEEK_API_KEY=" + "a" * 32,
    "inline api key": 'api_key = "' + "a" * 32 + '"',
}


def self_test() -> int:
    for name, sample in SELF_TEST_SAMPLES.items():
        if not PATTERNS[name].search(sample):
            print(f"secret scan self-test: FAIL ({name})", file=sys.stderr)
            return 1
    print("secret scan self-test: OK")
    return 0


def main() -> int:
    if "--self-test" in sys.argv:
        return self_test()
    findings: list[str] = []
    for path in tracked_files():
        try:
            text = open(path, "r", encoding="utf-8").read()
        except (OSError, UnicodeDecodeError):
            continue
        for line_no, line in enumerate(text.splitlines(), 1):
            if PLACEHOLDER.search(line):
                continue
            for name, pattern in PATTERNS.items():
                if pattern.search(line):
                    findings.append(f"{path}:{line_no}: {name}")
    if findings:
        print("secret scan: FAIL", file=sys.stderr)
        for finding in findings:
            print(f"  {finding}", file=sys.stderr)
        print("Remove the secret or add a narrow allowlist entry with review.", file=sys.stderr)
        return 1
    print("secret scan: OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
