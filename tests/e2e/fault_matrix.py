#!/usr/bin/env python3
"""Levier — single-node MVP fault matrix.

Runs real Levier, real Harness, and the Counter App, then injects:

  1. Harness kill -9 mid-run
  2. Levier restart mid-run
  3. App WebSocket unavailable, then recover
  4. Cancel
  5. Run timeout

All evidence stays under data/fault-matrix/<timestamp>/.
Secrets are never printed and the credential file is removed on exit.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import json
import os
import shutil
import signal
import subprocess
import sys
import time
import urllib.error
import urllib.request
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[2]
STAMP = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
OUT = Path(os.environ.get("FAULT_MATRIX_OUT", ROOT / "data" / "fault-matrix" / STAMP))


def _env(name: str, legacy: str, default: str = "") -> str:
    return os.environ.get(name) or os.environ.get(legacy) or default


MW_URL = _env("LEVIER_URL", "MW_URL", "http://127.0.0.1:8080")
CONFIG = _env("LEVIER_CONFIG", "MIDDLEWARE_CONFIG", "levier.example.yaml")
HARNESS_CONFIG = os.environ.get("HARNESS_CONFIG", "execution/harness/levier.example.yaml")
HARNESS_PYTHON = ROOT / os.environ.get("HARNESS_PYTHON", "execution/harness/.venv/bin/python")
APP_PYTHON = os.environ.get("APP_PYTHON", sys.executable)
LEVIER_BIN = os.environ.get("LEVIER_BIN", str(ROOT / "bin" / "levier"))
ADMIN_TOKEN = _env("LEVIER_ADMIN_TOKEN", "MW_ADMIN_TOKEN", "admin-change-me")
DAEMON_TOKEN = _env("LEVIER_DAEMON_TOKEN", "MW_DAEMON_TOKEN", "change-me")
DATABASE_URL = _env("LEVIER_DATABASE_URL", "MIDDLEWARE_DATABASE_URL", "")
PSQL = shutil.which("psql") or "/opt/homebrew/opt/postgresql@16/bin/psql"
GO = shutil.which("go") or "/usr/local/go/bin/go"
CREDENTIALS = OUT / "credentials.json"
APP_NAME = f"fault-matrix-{STAMP}"


class FaultError(RuntimeError):
    """One fault scenario failed."""


@dataclass
class Proc:
    name: str
    popen: subprocess.Popen[bytes]
    log: Any


PROCS: dict[str, Proc] = {}
RESULTS: list[dict[str, str]] = []


def load_env() -> None:
    """Load local env files without printing values."""
    for name in (".env.local", ".env"):
        path = ROOT / name
        if not path.exists():
            continue
        for raw in path.read_text(encoding="utf-8").splitlines():
            line = raw.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            key, value = line.split("=", 1)
            key = key.strip()
            value = value.strip().strip("\"'")
            os.environ.setdefault(key, value)


def run(cmd: list[str], *, env: dict[str, str] | None = None, timeout: float = 120) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        cmd,
        cwd=ROOT,
        env={**os.environ, **(env or {})},
        capture_output=True,
        text=True,
        timeout=timeout,
        check=False,
    )


def start(name: str, cmd: list[str], *, env: dict[str, str] | None = None) -> Proc:
    log_path = OUT / f"{name}.log"
    log = log_path.open("wb")
    popen = subprocess.Popen(
        cmd,
        cwd=ROOT,
        env={**os.environ, **(env or {})},
        stdout=log,
        stderr=subprocess.STDOUT,
        start_new_session=True,
    )
    proc = Proc(name=name, popen=popen, log=log)
    PROCS[name] = proc
    return proc


def stop(name: str, *, sig: int = signal.SIGTERM, timeout: float = 10) -> None:
    proc = PROCS.pop(name, None)
    if proc is None:
        return
    if proc.popen.poll() is None:
        try:
            os.killpg(os.getpgid(proc.popen.pid), sig)
        except ProcessLookupError:
            pass
    try:
        proc.popen.wait(timeout=timeout)
    except subprocess.TimeoutExpired:
        proc.popen.kill()
        proc.popen.wait(timeout=timeout)
    proc.log.close()


def stop_all() -> None:
    for name in list(PROCS):
        stop(name)


def http(method: str, path: str, body: dict[str, Any] | None = None, *, token: str = "") -> tuple[int, Any]:
    data = None if body is None else json.dumps(body).encode("utf-8")
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    request = urllib.request.Request(MW_URL + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=15) as response:
            raw = response.read()
            return response.status, (json.loads(raw.decode("utf-8")) if raw else {})
    except urllib.error.HTTPError as exc:
        raw = exc.read()
        try:
            parsed = json.loads(raw.decode("utf-8"))
        except (json.JSONDecodeError, UnicodeDecodeError):
            parsed = {"raw": raw.decode("utf-8", "replace")}
        return exc.code, parsed


def wait_http(path: str, timeout: float) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            status, _ = http("GET", path)
            if status < 500:
                return
        except OSError:
            pass
        time.sleep(0.1)
    raise FaultError(f"{MW_URL}{path} did not become ready")


def wait_metric(needle: str, timeout: float) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(MW_URL + "/metrics", timeout=5) as response:
                if needle in response.read().decode("utf-8", "replace"):
                    return
        except OSError:
            pass
        time.sleep(0.1)
    raise FaultError(f"metric {needle!r} did not appear")


def psql(sql: str) -> str:
    if not DATABASE_URL:
        raise FaultError("LEVIER_DATABASE_URL is required")
    result = run([PSQL, DATABASE_URL, "-At", "-v", "ON_ERROR_STOP=1", "-c", sql], timeout=30)
    if result.returncode != 0:
        raise FaultError(f"psql failed: {result.stderr.strip()}")
    return result.stdout.strip()


def read_credentials() -> dict[str, str]:
    return json.loads(CREDENTIALS.read_text(encoding="utf-8"))


def app_token() -> str:
    return read_credentials()["api_key"]


def get_run(run_id: str) -> dict[str, Any]:
    status, body = http("GET", f"/api/v1/runs/{run_id}", token=app_token())
    if status != 200:
        raise FaultError(f"get run {run_id}: HTTP {status} {body}")
    return body


def get_events(run_id: str) -> list[dict[str, Any]]:
    status, body = http("GET", f"/api/v1/runs/{run_id}/events?limit=1000", token=app_token())
    if status != 200:
        raise FaultError(f"get events {run_id}: HTTP {status} {body}")
    return body.get("events", [])


def cancel(run_id: str, reason: str = "fault-matrix") -> None:
    status, body = http("POST", f"/api/v1/runs/{run_id}/cancel", {"reason": reason}, token=app_token())
    if status != 200:
        raise FaultError(f"cancel {run_id}: HTTP {status} {body}")


def submit(strategy: str, prompt: str, run_ms: int) -> str:
    cmd = [
        LEVIER_BIN,
        "run", "submit",
        "--credentials", str(CREDENTIALS),
        "--server", MW_URL,
        "--strategy", strategy,
        "--prompt", prompt,
        "--run-ms", str(run_ms),
        "--tool-ms", "10000",
        "--request-id", f"fault-{strategy}-{time.time_ns()}",
        "--json",
    ]
    if strategy == "react":
        cmd += ["--provider", "deepseek", "--model", "deepseek-flash", "--max-steps", "8"]
    result = run(cmd, timeout=30)
    if result.returncode != 0:
        raise FaultError(f"submit failed: {result.stderr.strip()}")
    return json.loads(result.stdout.strip().splitlines()[-1])["run_id"]


def wait_status(run_id: str, wanted: set[str], timeout: float) -> dict[str, Any]:
    deadline = time.monotonic() + timeout
    last: dict[str, Any] = {}
    while time.monotonic() < deadline:
        last = get_run(run_id)
        if last.get("status") in wanted:
            return last
        if last.get("status") in {"completed", "failed", "cancelled", "timeout"}:
            raise FaultError(f"run {run_id} ended as {last.get('status')}, want {sorted(wanted)}")
        time.sleep(0.05)
    raise FaultError(f"run {run_id} did not reach {sorted(wanted)}; last={last.get('status')}")


def wait_terminal(run_id: str, timeout: float) -> dict[str, Any]:
    deadline = time.monotonic() + timeout
    last: dict[str, Any] = {}
    while time.monotonic() < deadline:
        last = get_run(run_id)
        if last.get("status") in {"completed", "failed", "cancelled", "timeout"}:
            return last
        time.sleep(0.1)
    raise FaultError(f"run {run_id} did not terminate; last={last.get('status')}")


def daemon_seq_ok(run_id: str) -> None:
    rows = psql(
        "SELECT attempt||'/'||count(*)||'/'||max(seq)||'/'||(max(seq)-min(seq)+1) "
        f"FROM run_events WHERE run_id='{run_id}' AND source='daemon' GROUP BY attempt"
    )
    for row in rows.splitlines():
        attempt, count, maximum, expected = (int(part) for part in row.split("/"))
        if count != expected or maximum != expected:
            raise FaultError(f"daemon event seq gap at attempt {attempt}: {row}")


def require_mw_event(run_id: str, status: str) -> None:
    payload = psql(
        "SELECT payload::text FROM run_events "
        f"WHERE run_id='{run_id}' AND source='mw' AND type='RUN_STATUS' "
        f"AND payload->>'status'='{status}' LIMIT 1"
    )
    if not payload:
        raise FaultError(f"missing source=mw RUN_STATUS {status} for {run_id}")


def require_no_active_tasks(run_id: str) -> None:
    count = int(psql(
        "SELECT count(*) FROM run_tasks "
        f"WHERE run_id='{run_id}' AND status IN ('PENDING','LEASED')"
    ))
    if count != 0:
        raise FaultError(f"{count} active tasks remain for {run_id}")


def start_middleware() -> None:
    start("middleware", [str(ROOT / "bin" / "middleware"), "-config", CONFIG])
    wait_http("/healthz", 10)


def start_harness() -> None:
    env = {
        "PYTHONPATH": "execution/agent_core/src:execution/harness/src/gen:execution/harness/src",
        "HARNESS_LEVIER_ADDRESS": "127.0.0.1:9090",
        "HARNESS_DAEMON_ID": "daemon-local-1",
        "HARNESS_DAEMON_AUTH_TOKEN": DAEMON_TOKEN,
    }
    start("harness", [str(HARNESS_PYTHON), "-m", "levier_harness", "--config", HARNESS_CONFIG], env=env)
    wait_metric('daemon="daemon-local-1"', 10)


def start_app() -> None:
    app_id = read_credentials()["app_id"]
    start("app", [
        APP_PYTHON,
        "examples/counter_agent/app.py",
    ], env={
        "LEVIER_CREDENTIALS": str(CREDENTIALS),
        "PYTHONPATH": "sdk/python/src",
    })
    wait_metric(f'levier_ws_connections{{app="{app_id}"}} 1', 10)


def bootstrap() -> None:
    result = run([
        LEVIER_BIN,
        "app", "init",
        "--name", APP_NAME,
        "--server", MW_URL,
        "--admin-token", ADMIN_TOKEN,
        "--credentials-out", str(CREDENTIALS),
    ], timeout=30)
    if result.returncode != 0:
        raise FaultError(f"app init failed: {result.stderr.strip()}")


def scenario_harness_kill() -> str:
    for _ in range(3):
        run_id = submit("react", "Add 2, then add 3, then read the counter. Use the counter tools.", 120_000)
        try:
            wait_status(run_id, {"running"}, 20)
        except FaultError:
            continue
        stop("harness", sig=signal.SIGKILL)
        wait_status(run_id, {"queued"}, 20)
        start_harness()
        final = wait_terminal(run_id, 180)
        if final.get("status") != "completed":
            raise FaultError(f"run {run_id} after harness kill: {final.get('status')}")
        if int(final.get("attempt", 0)) < 2:
            raise FaultError(f"run {run_id} was not retried: attempt={final.get('attempt')}")
        daemon_seq_ok(run_id)
        return f"run {run_id} requeued and completed at attempt {final.get('attempt')}"
    raise FaultError("could not catch a running react run before completion")


def scenario_mw_restart() -> str:
    run_id = submit("react", "Add 2, then add 3, then read the counter. Use the counter tools.", 120_000)
    wait_status(run_id, {"running"}, 20)
    stop("middleware")
    start_middleware()
    time.sleep(1.0)
    wait_metric('daemon="daemon-local-1"', 15)
    final = wait_terminal(run_id, 180)
    if final.get("status") != "completed":
        raise FaultError(f"run {run_id} after MW restart: {final.get('status')}")
    daemon_seq_ok(run_id)
    return f"run {run_id} completed after MW restart at attempt {final.get('attempt')}"


def scenario_app_disconnect() -> str:
    stop("app")
    run_id = ""
    final: dict[str, Any] = {}
    tool_failures = 0
    for _ in range(3):
        run_id = submit("react", "Add 2, then add 3, then read the counter. Use the counter tools.", 60_000)
        final = wait_terminal(run_id, 120)
        events = get_events(run_id)
        tool_failures = sum(
            1 for event in events
            if event.get("event_type") == "TOOL_CALL_END"
            and not (event.get("data") or {}).get("ok", False)
        )
        if tool_failures > 0:
            break
    if tool_failures == 0:
        raise FaultError("offline run never attempted a tool call")
    start_app()
    second = submit("react", "Add 1, then read the counter. Use the counter tools.", 120_000)
    second_final = wait_terminal(second, 120)
    if second_final.get("status") != "completed":
        raise FaultError(f"run after App recovery: {second_final.get('status')}")
    return (
        f"offline run {run_id} ended as {final.get('status')} with {tool_failures} failed tool calls; "
        f"run {second} completed after App restart"
    )


def scenario_cancel() -> str:
    run_id = submit("react", "Add 2, then add 3, then read the counter. Use the counter tools.", 120_000)
    cancel(run_id)
    final = wait_status(run_id, {"cancelled"}, 30)
    require_mw_event(run_id, "cancelled")
    require_no_active_tasks(run_id)
    return f"run {run_id} cancelled, MW event present, no active tasks"


def scenario_timeout() -> str:
    for run_ms in (500, 100):
        run_id = submit("react", "Add 2, then add 3, then read the counter. Use the counter tools.", run_ms)
        try:
            final = wait_status(run_id, {"timeout"}, 30)
        except FaultError:
            continue
        require_mw_event(run_id, "timeout")
        require_no_active_tasks(run_id)
        return f"run {run_id} timed out after {run_ms}ms, MW event present, no active tasks"
    raise FaultError("could not force a timeout before react completed")


def main() -> int:
    global OUT, CREDENTIALS
    OUT.mkdir(parents=True, exist_ok=True)
    os.chmod(OUT, 0o700)
    CREDENTIALS = OUT / "credentials.json"
    load_env()
    if not DATABASE_URL:
        print("fault-matrix: LEVIER_DATABASE_URL is required", file=sys.stderr)
        return 2
    if not HARNESS_PYTHON.exists():
        print(f"fault-matrix: harness Python not found: {HARNESS_PYTHON}", file=sys.stderr)
        return 2
    if http_ready():
        print(f"fault-matrix: {MW_URL} is already serving", file=sys.stderr)
        return 2

    print(f"evidence: {OUT}")
    build = run([GO, "build", "-o", "bin/levier", "./cmd/levier"], timeout=120)
    if build.returncode != 0:
        print(build.stderr, file=sys.stderr)
        return 1
    migrate = run([GO, "build", "-o", "bin/migrate", "./cmd/migrate"], timeout=120)
    if migrate.returncode != 0:
        print(migrate.stderr, file=sys.stderr)
        return 1
    migrated = run([str(ROOT / "bin" / "migrate"), "-config", CONFIG], timeout=60)
    if migrated.returncode != 0:
        print(migrated.stderr, file=sys.stderr)
        return 1

    try:
        start_middleware()
        start_harness()
        bootstrap()
        start_app()
        scenarios = [
            ("harness_kill", scenario_harness_kill),
            ("mw_restart", scenario_mw_restart),
            ("app_disconnect", scenario_app_disconnect),
            ("cancel", scenario_cancel),
            ("timeout", scenario_timeout),
        ]
        for name, scenario in scenarios:
            started = time.monotonic()
            detail = scenario()
            elapsed = round(time.monotonic() - started, 2)
            RESULTS.append({"scenario": name, "result": "PASS", "seconds": str(elapsed), "detail": detail})
            print(f"PASS {name} ({elapsed}s): {detail}")
    except FaultError as exc:
        RESULTS.append({"scenario": "fault-matrix", "result": "FAIL", "seconds": "", "detail": str(exc)})
        print(f"FAIL: {exc}", file=sys.stderr)
        write_summary()
        return 1
    finally:
        stop_all()
        try:
            CREDENTIALS.unlink()
        except FileNotFoundError:
            pass
    write_summary()
    print(f"fault-matrix: PASS ({len(RESULTS)} scenarios, evidence: {OUT})")
    return 0


def http_ready() -> bool:
    try:
        status, _ = http("GET", "/healthz")
        return status < 500
    except OSError:
        return False


def write_summary() -> None:
    lines = [f"{item['scenario']}: {item['result']} {item['seconds']}s {item['detail']}" for item in RESULTS]
    (OUT / "summary.txt").write_text("\n".join(lines) + "\n", encoding="utf-8")


if __name__ == "__main__":
    raise SystemExit(main())
