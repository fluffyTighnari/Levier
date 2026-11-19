#!/usr/bin/env python3
"""Levier structure guard.

Checks top-level layout, unit cards, Python boundaries, generated code,
and stale repository paths. Keep rules coarse and actionable.
"""

from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]

REQUIRED_DIRS = [
    "cmd",
    "internal",
    "contracts",
    "proto",
    "gen",
    "execution/agent_core",
    "execution/runtime_sdk",
    "execution/echo_runtime",
    "execution/react_runtime",
    "execution/mcp_runtime",
    "sdk/python",
    "examples",
    "deploy",
    "tests",
    "docs",
]

FORBIDDEN_DIRS = [
    "agent_core",
    "harness",
    "mcp_runtime",
]

UNIT_CARDS = [
    "cmd/README.md",
    "internal/README.md",
    "contracts/README.md",
    "proto/README.md",
    "gen/README.md",
    "execution/README.md",
    "execution/agent_core/README.md",
    "execution/runtime_sdk/README.md",
    "execution/echo_runtime/README.md",
    "execution/react_runtime/README.md",
    "execution/mcp_runtime/README.md",
    "sdk/python/README.md",
    "examples/README.md",
    "deploy/README.md",
    "tests/README.md",
    "docs/README.md",
    ".github/README.md",
]

AGENT_CORE_FORBIDDEN = [
    "levier_react_runtime",
    "levier_mcp_runtime",
    "middleware_harness",
    "mcp_runtime",
    "middleware.runtime",
    "google.protobuf",
    "internal/",
]

OLD_PATH_PATTERNS = [
    "agent_core/src",
    "agent_core/tests",
    "harness/src",
    "harness/tests",
    "harness/.venv",
    "mcp_runtime/src",
    "mcp_runtime/tests",
    "mcp_runtime/.venv",
    "./agent_core",
    "./harness",
    "./mcp_runtime",
    "cd harness",
    "cd mcp_runtime",
]

SKIP_FILES = {
    "docs/NAMING.md",
    "tests/checks/check_structure.py",
    "tests/checks/check_structure.sh",
}

TEXT_SUFFIXES = {
    ".md",
    ".go",
    ".py",
    ".sh",
    ".yaml",
    ".yml",
    ".toml",
    ".json",
    ".Dockerfile",
    ".proto",
}

failures: list[tuple[str, str, str]] = []


def fail(rule: str, message: str, fix: str = "") -> None:
    failures.append((rule, message, fix))


def read_text(path: Path) -> str:
    try:
        return path.read_text(encoding="utf-8")
    except UnicodeDecodeError:
        return ""


def check_layout() -> None:
    for name in REQUIRED_DIRS:
        if not (ROOT / name).is_dir():
            fail("required-dir", f"missing directory: {name}", f"create {name}/")
    for name in FORBIDDEN_DIRS:
        if (ROOT / name).exists():
            fail(
                "forbidden-dir",
                f"forbidden top-level directory: {name}/",
                f"move it under execution/",
            )


def check_unit_cards() -> None:
    for name in UNIT_CARDS:
        if not (ROOT / name).is_file():
            fail("unit-card", f"missing unit card: {name}", f"add {name}")


def is_virtualenv_path(path: Path) -> bool:
    return any(part == ".venv" or part.startswith(".venv") for part in path.parts)


def check_examples_isolation() -> None:
    patterns = [
        'levier/examples',
        '"github.com/fluffyTighnari/levier/examples',
        'from examples',
        'import examples',
        'examples.',
    ]
    for base in ("internal", "cmd", "execution"):
        for path in (ROOT / base).rglob("*"):
            if is_virtualenv_path(path):
                continue
            if not path.is_file() or path.suffix not in {".go", ".py"}:
                continue
            text = read_text(path)
            for pattern in patterns:
                if pattern in text:
                    rel = path.relative_to(ROOT).as_posix()
                    fail("examples-isolation", f"{rel} references examples/", "move shared code out of examples/")
                    break


def check_agent_core_boundary() -> None:
    base = ROOT / "execution/agent_core/src/levier_agent_core"
    for path in base.rglob("*.py"):
        if is_virtualenv_path(path):
            continue
        text = read_text(path)
        for token in AGENT_CORE_FORBIDDEN:
            if token in text:
                rel = path.relative_to(ROOT).as_posix()
                fail("agent-core-boundary", f"{rel} contains {token}", "keep agent_core transport-free")
                break


def check_cmd_packages() -> None:
    cmd = ROOT / "cmd"
    if not cmd.is_dir():
        return
    for directory in sorted(p for p in cmd.iterdir() if p.is_dir()):
        files = sorted(directory.glob("*.go"))
        if not files:
            fail("cmd-main", f"{directory.name}/ has no Go files", "add a main package")
            continue
        packages = set()
        for path in files:
            for line in read_text(path).splitlines():
                if line.startswith("package "):
                    packages.add(line.split(maxsplit=1)[1].strip())
                    break
        if packages != {"main"}:
            rel = directory.relative_to(ROOT).as_posix()
            found = ", ".join(sorted(packages)) or "unknown"
            fail("cmd-main", f"{rel} is package {found}, want main", "make cmd entrypoints package main")


def check_generated() -> None:
    for path in (ROOT / "gen").rglob("*.go"):
        text = read_text(path)
        if "Code generated by" not in text or "DO NOT EDIT" not in text:
            rel = path.relative_to(ROOT).as_posix()
            fail("generated-code", f"{rel} is not marked as generated", "regenerate with make proto")
            break


def iter_text_files() -> list[Path]:
    files: list[Path] = []

    for name in (
        "README.md",
        "Makefile",
        "buf.gen.yaml",
        "docker-compose.yml",
        "levier.example.yaml",
        "execution/react_runtime/react.example.yaml",
        ".env.example",
    ):
        path = ROOT / name
        if path.is_file():
            files.append(path)

    for base in (".github", "deploy", "tests"):
        for path in (ROOT / base).rglob("*"):
            if path.is_file():
                files.append(path)

    for path in (ROOT / "docs").rglob("*"):
        if not path.is_file():
            continue
        rel = path.relative_to(ROOT).as_posix()
        if rel.startswith("docs/reports/") or rel.startswith("docs/adr/"):
            continue
        if rel == "docs/NAMING.md":
            continue
        files.append(path)

    for path in (ROOT / "execution").glob("*/README.md"):
        if path.is_file():
            files.append(path)

    return [
        path
        for path in files
        if path.relative_to(ROOT).as_posix() not in SKIP_FILES
        and (path.suffix in TEXT_SUFFIXES or path.name in {"Makefile", "Dockerfile"})
    ]


def check_old_paths() -> None:
    for path in iter_text_files():
        text = read_text(path)
        rel = path.relative_to(ROOT).as_posix()
        # Strip new and container paths so valid references do not look stale.
        stripped = text
        for new in (
            "execution/agent_core/src",
            "execution/agent_core/tests",
            "execution/agent_core/README.md",
            "execution/agent_core/.venv",
            "execution/runtime_sdk/src",
            "execution/runtime_sdk/tests",
            "execution/runtime_sdk/README.md",
            "execution/runtime_sdk/src",
            "execution/runtime_sdk/tests",
            "execution/runtime_sdk/README.md",
            "execution/echo_runtime/src",
            "execution/echo_runtime/tests",
            "execution/echo_runtime/README.md",
            "execution/echo_runtime/runtime.yaml",
            "execution/react_runtime/src",
            "execution/react_runtime/tests",
            "execution/react_runtime/README.md",
            "execution/react_runtime/.venv",
            "execution/react_runtime/react.example.yaml",
            "execution/mcp_runtime/src",
            "execution/mcp_runtime/tests",
            "execution/mcp_runtime/README.md",
            "execution/mcp_runtime/.venv",
            "/app/agent_core/src",
            "/app/harness/src",
            "/app/mcp_runtime/src",
        ):
            stripped = stripped.replace(new, "")
        stripped = stripped.replace("../agent_core", "../EXEC_AGENT_CORE")
        stripped = stripped.replace("../harness", "../EXEC_HARNESS")
        stripped = stripped.replace("../mcp_runtime", "../EXEC_MCP_RUNTIME")
        for pattern in OLD_PATH_PATTERNS:
            if pattern in stripped:
                fail("old-path", f"{rel} references old path: {pattern}", "update to execution/...")
                break


def main() -> int:
    failures.clear()

    check_layout()
    check_unit_cards()
    check_examples_isolation()
    check_agent_core_boundary()
    check_cmd_packages()
    check_generated()
    check_old_paths()

    if failures:
        print("structure check: FAIL")
        for rule, message, fix in failures:
            print(f"  rule: {rule}")
            print(f"  {message}")
            if fix:
                print(f"  fix: {fix}")
        return 1

    print(f"structure check: OK ({len(REQUIRED_DIRS)} units, 7 rules)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
