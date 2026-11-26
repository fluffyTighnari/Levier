#!/usr/bin/env python3
"""I3 multi-replica fault matrix.

Topology:
    2 x control/api
    2 x runtime-gateway
    2 x app-gateway
    1 x PostgreSQL
    1 x Runtime daemon (tests/e2e/i3daemon)
    1 x App (tests/e2e/i3app.py)

The script starts real processes with isolated configs and exercises replica
failure, reconnect, cross-replica cancel, local tool idempotency, event fanout,
durable envelope replay, and a v1 App connection.

Usage:
    python tests/e2e/i3_multireplica.py

It uses a temporary Docker PostgreSQL by default. Set LEVIER_I3_DATABASE_URL
to use an existing database, and LEVIER_I3_PSQL to point at psql.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import json
import os
import select
import shutil
import signal
import socket
import socketserver
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Callable

ROOT = Path(__file__).resolve().parents[2]
STAMP = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
OUT = Path(os.environ.get("LEVIER_I3_OUT", ROOT / "data" / "i3-matrix" / STAMP))

ADMIN_TOKEN = "i3-admin-token"
DAEMON_TOKEN = "i3-daemon-token"
INTERNAL_TOKEN = "i3-internal-token"
WS_SECRET = "i3-ws-secret"
APP_NAME = f"i3-matrix-{STAMP}"
RUNTIME_NAME = "i3daemon"
DAEMON_ID = "i3-daemon"
SUBSCRIBE_FILE = OUT / "subscribes.txt"

DATABASE_URL = os.environ.get("LEVIER_I3_DATABASE_URL", "")
PG_CONTAINER = f"levier-i3-pg-{os.getpid()}"
PG_PORT = 0
PG_DB = f"levier_i3_{os.getpid()}"
PSQL = os.environ.get("LEVIER_I3_PSQL", shutil.which("psql") or "")

GO = shutil.which("go") or "go"
PYTHON = os.environ.get("LEVIER_I3_PYTHON") or sys.executable
LEVIER_BIN = ROOT / ("bin/levier.exe" if os.name == "nt" else "bin/levier")
DAEMON_BIN = ROOT / ("bin/i3-daemon.exe" if os.name == "nt" else "bin/i3-daemon")


class MatrixError(RuntimeError):
    """One matrix assertion failed."""


@dataclass
class Proc:
    name: str
    popen: subprocess.Popen[bytes]
    log: Any


PROCS: dict[str, Proc] = {}
RESULTS: list[dict[str, str]] = []
PORTS: dict[str, int] = {}


def run(
    cmd: list[str],
    *,
    env: dict[str, str] | None = None,
    timeout: float = 120,
    check: bool = False,
) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(
        cmd,
        cwd=ROOT,
        env={**os.environ, **(env or {})},
        capture_output=True,
        text=True,
        timeout=timeout,
        check=False,
    )
    if check and result.returncode != 0:
        raise MatrixError(
            f"command failed ({result.returncode}): {' '.join(cmd)}\n"
            f"stdout: {result.stdout[-2000:]}\nstderr: {result.stderr[-2000:]}"
        )
    return result


def start(name: str, cmd: list[str], *, env: dict[str, str] | None = None) -> Proc:
    log_path = OUT / f"{name}.log"
    log = log_path.open("ab")
    kwargs: dict[str, Any] = {
        "cwd": ROOT,
        "env": {**os.environ, **(env or {})},
        "stdout": log,
        "stderr": subprocess.STDOUT,
    }
    if os.name == "nt":
        kwargs["creationflags"] = subprocess.CREATE_NEW_PROCESS_GROUP
    else:
        kwargs["start_new_session"] = True
    popen = subprocess.Popen(cmd, **kwargs)
    proc = Proc(name=name, popen=popen, log=log)
    PROCS[name] = proc
    return proc


def stop(name: str, *, force: bool = False) -> None:
    proc = PROCS.pop(name, None)
    if proc is None:
        return
    if proc.popen.poll() is None:
        if os.name == "nt":
            subprocess.run(
                ["taskkill", "/F" if force else "/T", "/PID", str(proc.popen.pid)],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                check=False,
            )
        else:
            sig = signal.SIGKILL if force else signal.SIGTERM
            try:
                os.killpg(os.getpgid(proc.popen.pid), sig)
            except ProcessLookupError:
                pass
    try:
        proc.popen.wait(timeout=10)
    except subprocess.TimeoutExpired:
        proc.popen.kill()
        proc.popen.wait(timeout=10)
    proc.log.close()


def stop_all() -> None:
    for name in list(PROCS):
        stop(name, force=True)


def free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def wait_until(predicate: Callable[[], bool], timeout: float, description: str) -> None:
    deadline = time.monotonic() + timeout
    last_error: Exception | None = None
    while time.monotonic() < deadline:
        try:
            if predicate():
                return
        except Exception as exc:  # polling deliberately absorbs transient errors
            last_error = exc
        time.sleep(0.1)
    suffix = f" (last error: {last_error})" if last_error else ""
    raise MatrixError(f"timeout waiting for {description}{suffix}")


def http(
    method: str,
    base: str,
    path: str,
    body: dict[str, Any] | None = None,
    *,
    token: str = "",
    timeout: float = 15,
) -> tuple[int, Any]:
    data = None if body is None else json.dumps(body).encode("utf-8")
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    request = urllib.request.Request(base + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            raw = response.read()
            return response.status, json.loads(raw.decode("utf-8")) if raw else {}
    except urllib.error.HTTPError as exc:
        raw = exc.read()
        try:
            parsed = json.loads(raw.decode("utf-8"))
        except (json.JSONDecodeError, UnicodeDecodeError):
            parsed = {"raw": raw.decode("utf-8", "replace")}
        return exc.code, parsed


def wait_http(base: str, path: str = "/healthz", timeout: float = 20) -> None:
    def ready() -> bool:
        try:
            status, _ = http("GET", base, path)
            return status < 500
        except OSError:
            return False

    wait_until(ready, timeout, f"{base}{path}")


def sql(query: str, *, database: str | None = None) -> str:
    if PSQL:
        result = run(
            [PSQL, DATABASE_URL, "-At", "-v", "ON_ERROR_STOP=1", "-c", query],
            timeout=30,
        )
    else:
        if not PG_CONTAINER:
            raise MatrixError("psql is unavailable and no Docker PostgreSQL manager is active")
        result = run(
            [
                "docker", "exec", PG_CONTAINER,
                "psql", "-U", "levier", "-d", database or PG_DB,
                "-At", "-v", "ON_ERROR_STOP=1", "-c", query,
            ],
            timeout=30,
        )
    if result.returncode != 0:
        raise MatrixError(f"sql failed: {result.stderr.strip()}\nquery: {query}")
    return result.stdout.strip()


def sql_int(query: str) -> int:
    value = sql(query)
    return int(value) if value else 0


def sql_quote(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def start_postgres() -> None:
    global DATABASE_URL, PG_PORT, PG_CONTAINER
    if DATABASE_URL:
        PG_CONTAINER = ""
        wait_until(lambda: _database_ready(DATABASE_URL), 30, "external PostgreSQL")
        return
    if not shutil.which("docker"):
        raise MatrixError("Docker is required unless LEVIER_I3_DATABASE_URL is set")
    run(["docker", "rm", "-f", PG_CONTAINER], timeout=30)
    PG_PORT = free_port()
    result = run(
        [
            "docker", "run", "-d", "--name", PG_CONTAINER,
            "-e", "POSTGRES_USER=levier",
            "-e", "POSTGRES_PASSWORD=change-me",
            "-e", f"POSTGRES_DB={PG_DB}",
            "-p", f"127.0.0.1:{PG_PORT}:5432",
            "postgres:16-alpine",
        ],
        timeout=120,
    )
    if result.returncode != 0:
        raise MatrixError(f"start postgres: {result.stderr.strip()}")
    wait_until(lambda: _docker_pg_ready(), 60, "temporary PostgreSQL")
    DATABASE_URL = (
        f"postgres://levier:change-me@127.0.0.1:{PG_PORT}/{PG_DB}?sslmode=disable"
    )


def _docker_pg_ready() -> bool:
    result = run(
        ["docker", "exec", PG_CONTAINER, "pg_isready", "-U", "levier", "-d", PG_DB],
        timeout=10,
    )
    return result.returncode == 0


def _database_ready(dsn: str) -> bool:
    if not PSQL:
        return True
    result = run([PSQL, dsn, "-At", "-c", "select 1"], timeout=10)
    return result.returncode == 0


def cleanup_postgres() -> None:
    if PG_CONTAINER:
        run(["docker", "rm", "-f", PG_CONTAINER], timeout=30)


def write_configs() -> dict[str, dict[str, Any]]:
    global PORTS
    names = [
        "controlA", "controlB",
        "rtgwA", "rtgwB",
        "appgwA", "appgwB",
        "proxy",
    ]
    for name in names:
        PORTS[name] = free_port()
    config: dict[str, dict[str, Any]] = {}

    def base(instance: str, roles: list[str], http_port: int, grpc_port: int, internal_port: int) -> str:
        return f"""\
config_version: 1
environment: development
http:
  addr: "127.0.0.1:{http_port}"
  read_timeout: "15s"
  write_timeout: "30s"
  shutdown_timeout: "5s"
  admin_token: "{ADMIN_TOKEN}"
  ws_token_ttl: "10m"
  ws_token_secret: "{WS_SECRET}"
  public_ws_url: "ws://127.0.0.1:{PORTS['proxy']}/api/v1/ws"
gateway:
  instance_id: "{instance}"
  roles: {json.dumps(roles)}
  heartbeat_interval: "1s"
  lease_ttl: "6s"
grpc:
  addr: "127.0.0.1:{grpc_port}"
  internal_addr: "127.0.0.1:{internal_port}"
  internal_token: "{INTERNAL_TOKEN}"
  max_recv_msg_bytes: 16777216
  max_send_msg_bytes: 16777216
  shutdown_timeout: "5s"
  daemon_tokens:
    "*": "{DAEMON_TOKEN}"
database:
  url: "{DATABASE_URL}"
  max_conns: 10
  min_conns: 1
  connect_timeout: "5s"
  health_timeout: "2s"
workflow:
  workers: 2
  lease_ttl: "3s"
  poll_interval: "100ms"
  max_attempts: 5
  reap_interval: "1s"
  reap_limit: 100
  recovery_delay: "2s"
  event_buffer: 1024
log:
  level: "info"
  format: "json"
"""

    config["controlA"] = {
        "path": OUT / "controlA.yaml",
        "url": f"http://127.0.0.1:{PORTS['controlA']}",
        "instance": "control-a",
    }
    config["controlB"] = {
        "path": OUT / "controlB.yaml",
        "url": f"http://127.0.0.1:{PORTS['controlB']}",
        "instance": "control-b",
    }
    config["rtgwA"] = {
        "path": OUT / "rtgwA.yaml",
        "addr": f"127.0.0.1:{PORTS['rtgwA']}",
        "instance": "runtime-gateway-a",
    }
    config["rtgwB"] = {
        "path": OUT / "rtgwB.yaml",
        "addr": f"127.0.0.1:{PORTS['rtgwB']}",
        "instance": "runtime-gateway-b",
    }
    config["appgwA"] = {
        "path": OUT / "appgwA.yaml",
        "url": f"http://127.0.0.1:{PORTS['appgwA']}",
        "instance": "app-gateway-a",
    }
    config["appgwB"] = {
        "path": OUT / "appgwB.yaml",
        "url": f"http://127.0.0.1:{PORTS['appgwB']}",
        "instance": "app-gateway-b",
    }

    config["controlA"]["path"].write_text(
        base("control-a", ["control", "api"], PORTS["controlA"], free_port(), free_port()),
        encoding="utf-8",
    )
    config["controlB"]["path"].write_text(
        base("control-b", ["control", "api"], PORTS["controlB"], free_port(), free_port()),
        encoding="utf-8",
    )
    config["rtgwA"]["path"].write_text(
        base("runtime-gateway-a", ["runtime-gateway"], free_port(), PORTS["rtgwA"], free_port()),
        encoding="utf-8",
    )
    config["rtgwB"]["path"].write_text(
        base("runtime-gateway-b", ["runtime-gateway"], free_port(), PORTS["rtgwB"], free_port()),
        encoding="utf-8",
    )
    config["appgwA"]["path"].write_text(
        base("app-gateway-a", ["app-gateway"], PORTS["appgwA"], free_port(), free_port()),
        encoding="utf-8",
    )
    config["appgwB"]["path"].write_text(
        base("app-gateway-b", ["app-gateway"], PORTS["appgwB"], free_port(), free_port()),
        encoding="utf-8",
    )
    if os.environ.get("LEVIER_I3_DIRECT_WS") == "1":
        direct = f"ws://127.0.0.1:{PORTS['appgwA']}/api/v1/ws"
        for key in ("controlA", "controlB"):
            path = config[key]["path"]
            text = path.read_text(encoding="utf-8")
            path.write_text(text.replace(
                f"ws://127.0.0.1:{PORTS['proxy']}/api/v1/ws", direct
            ), encoding="utf-8")
    return config


def start_role(name: str, config_key: str, config: dict[str, dict[str, Any]]) -> None:
    cfg = config[config_key]
    start(name, [str(LEVIER_BIN), "-config", str(cfg["path"])])
    if "url" in cfg:
        wait_http(cfg["url"])


def wait_gateway(instance: str, timeout: float = 20) -> None:
    def online() -> bool:
        return sql_int(
            "SELECT count(*) FROM gateway_instances "
            f"WHERE gateway_id={sql_quote(instance)} "
            "AND status='online' AND lease_expires_at > now()"
        ) == 1

    wait_until(online, timeout, f"gateway {instance} online")


def wait_gateway_offline(instance: str, timeout: float = 20) -> None:
    def offline() -> bool:
        return sql_int(
            "SELECT count(*) FROM gateway_instances "
            f"WHERE gateway_id={sql_quote(instance)} "
            "AND status='online' AND lease_expires_at > now()"
        ) == 0

    wait_until(offline, timeout, f"gateway {instance} offline")


def wait_runtime(gateway: str, timeout: float = 20) -> None:
    def online() -> bool:
        return sql_int(
            "SELECT count(*) FROM runtime_connections "
            f"WHERE daemon_id={sql_quote(DAEMON_ID)} "
            f"AND gateway_id={sql_quote(gateway)} "
            "AND status='online' AND lease_expires_at > now()"
        ) == 1

    wait_until(online, timeout, f"runtime {DAEMON_ID} on {gateway}")


def wait_runtime_gone(timeout: float = 20) -> None:
    def offline() -> bool:
        return sql_int(
            "SELECT count(*) FROM runtime_connections "
            f"WHERE daemon_id={sql_quote(DAEMON_ID)} "
            "AND status='online' AND lease_expires_at > now()"
        ) == 0

    wait_until(offline, timeout, f"runtime {DAEMON_ID} lease gone")


def start_daemon(config: dict[str, dict[str, Any]], gateway_key: str, *, tool: str = "counter_add", delay: str = "0s", repeat: int = 1) -> None:
    stop("daemon", force=True)
    cfg = config[gateway_key]
    start(
        "daemon",
        [
            str(DAEMON_BIN),
            "-gateway", cfg["addr"],
            "-token", DAEMON_TOKEN,
            "-daemon-id", DAEMON_ID,
            "-runtime", RUNTIME_NAME,
            "-tool", tool,
            "-args", '{"delta":1}',
            "-delay", delay,
            "-repeat-invocation", str(repeat),
        ],
    )
    wait_runtime(cfg["instance"])


def start_app(server: str, credentials: Path, log_path: Path) -> None:
    stop("app", force=True)
    before_hello = hello_ack_count(log_path)
    env = {
        "PYTHONPATH": os.pathsep.join([
            str(ROOT / "sdk/levier/src"),
            str(ROOT / "sdk/python/src"),
        ]),
    }
    start(
        "app",
        [
            PYTHON, str(ROOT / "tests/e2e/i3app.py"),
            "--credentials", str(credentials),
            "--server", server,
            "--log", str(log_path),
            "--subscribe-file", str(SUBSCRIBE_FILE),
        ],
        env=env,
    )
    wait_app_connection(
        "app-gateway-a", log_path, min_hello_acks=before_hello + 1
    )


def hello_ack_count(log_path: Path) -> int:
    if not log_path.exists():
        return 0
    return sum(
        1
        for line in log_path.read_text(encoding="utf-8").splitlines()
        if '"kind":"hello_ack"' in line
    )


def wait_app_connection(
    gateway: str, log_path: Path, timeout: float = 25, *, min_hello_acks: int = 1
) -> None:
    def connected() -> bool:
        if hello_ack_count(log_path) < min_hello_acks:
            return False
        return sql_int(
            "SELECT count(*) FROM app_connections "
            "WHERE status='online' AND lease_expires_at > now() "
            f"AND gateway_id={sql_quote(gateway)}"
        ) >= 1

    wait_until(connected, timeout, f"app connected to {gateway}")


def read_credentials(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def api_key(credentials: Path) -> str:
    return str(read_credentials(credentials)["api_key"])


def submit(control_url: str, credentials: Path, *, run_ms: int = 120000) -> str:
    body = {
        "runtime_name": RUNTIME_NAME,
        "runtime": "normal",
        "input": [{"role": "user", "content": "run the I3 matrix"}],
        "timeouts": {"run_ms": run_ms, "tool_ms": 30000},
    }
    status, response = http(
        "POST", control_url, "/api/v1/runs", body, token=api_key(credentials)
    )
    if status not in (200, 201, 202):
        raise MatrixError(f"submit run: HTTP {status} {response}")
    run_id = str(response.get("run_id") or "")
    if not run_id:
        raise MatrixError(f"submit run returned no run_id: {response}")
    with SUBSCRIBE_FILE.open("a", encoding="utf-8") as handle:
        handle.write(run_id + "\n")
    return run_id


def get_run(control_url: str, credentials: Path, run_id: str) -> dict[str, Any]:
    status, response = http(
        "GET", control_url, f"/api/v1/runs/{run_id}", token=api_key(credentials)
    )
    if status != 200:
        raise MatrixError(f"get run {run_id}: HTTP {status} {response}")
    return response


def wait_run(
    control_url: str,
    credentials: Path,
    run_id: str,
    wanted: set[str],
    timeout: float = 60,
) -> dict[str, Any]:
    final: dict[str, Any] = {}

    def reached() -> bool:
        nonlocal final
        final = get_run(control_url, credentials, run_id)
        if final.get("status") in wanted:
            return True
        if final.get("status") in {"failed", "cancelled", "timeout", "completed"}:
            raise MatrixError(
                f"run {run_id} ended as {final.get('status')}, want {sorted(wanted)}"
            )
        return False

    wait_until(reached, timeout, f"run {run_id} in {sorted(wanted)}")
    return final


def cancel(control_url: str, credentials: Path, run_id: str) -> None:
    status, response = http(
        "POST",
        control_url,
        f"/api/v1/runs/{run_id}/cancel",
        {"reason": "i3-matrix"},
        token=api_key(credentials),
    )
    if status != 200:
        raise MatrixError(f"cancel {run_id}: HTTP {status} {response}")


def wait_terminal(
    control_url: str, credentials: Path, run_id: str, timeout: float = 90
) -> dict[str, Any]:
    return wait_run(
        control_url, credentials, run_id,
        {"completed", "failed", "cancelled", "timeout"}, timeout,
    )


def assert_run_clean(run_id: str) -> None:
    active = sql_int(
        "SELECT count(*) FROM run_tasks "
        f"WHERE run_id={sql_quote(run_id)} AND status IN ('PENDING','LEASED')"
    )
    if active != 0:
        raise MatrixError(f"{run_id} still has {active} active workflow tasks")
    rows = sql(
        "SELECT attempt, count(*), min(seq), max(seq) FROM run_events "
        f"WHERE run_id={sql_quote(run_id)} AND source='daemon' GROUP BY attempt"
    )
    for row in rows.splitlines():
        if not row:
            continue
        attempt, count, minimum, maximum = (int(part) for part in row.split("|"))
        if count != maximum - minimum + 1:
            raise MatrixError(f"{run_id} attempt {attempt} daemon event seq gap: {row}")


def assert_event(run_id: str, source: str, event_type: str) -> None:
    found = sql_int(
        "SELECT count(*) FROM run_events "
        f"WHERE run_id={sql_quote(run_id)} AND source={sql_quote(source)} "
        f"AND type={sql_quote(event_type)}"
    )
    if found < 1:
        raise MatrixError(f"{run_id} missing {source}/{event_type} event")


def app_has(path: Path, predicate: Callable[[dict[str, Any]], bool]) -> bool:
    if not path.exists():
        return False
    for line in path.read_text(encoding="utf-8").splitlines():
        try:
            record = json.loads(line)
        except json.JSONDecodeError:
            continue
        if predicate(record):
            return True
    return False


def wait_app_ws(path: Path, event_type: str, timeout: float = 30) -> None:
    def seen() -> bool:
        return app_has(
            path,
            lambda record: record.get("kind") == "ws"
            and (record.get("message") or {}).get("event_type") == event_type,
        )

    wait_until(seen, timeout, f"App WebSocket {event_type}")


def wait_app_completed(path: Path, timeout: float = 30) -> None:
    wait_until(
        lambda: app_has(
            path,
            lambda record: record.get("kind") == "ws"
            and (record.get("message") or {}).get("type") == "run_completed",
        ),
        timeout,
        "App WebSocket run_completed",
    )


def app_tool_count(path: Path, tool: str) -> int:
    if not path.exists():
        return 0
    count = 0
    for line in path.read_text(encoding="utf-8").splitlines():
        try:
            record = json.loads(line)
        except json.JSONDecodeError:
            continue
        if record.get("kind") == "tool" and record.get("tool") == tool:
            count += 1
    return count


class WSProxy(socketserver.ThreadingTCPServer):
    allow_reuse_address = True
    daemon_threads = True

    def __init__(self, address: tuple[str, int]) -> None:
        super().__init__(address, WSProxyHandler)
        self.target: tuple[str, int] = ("127.0.0.1", 1)
        self.target_lock = threading.Lock()

    def set_target(self, host: str, port: int) -> None:
        with self.target_lock:
            self.target = (host, port)

    def get_target(self) -> tuple[str, int]:
        with self.target_lock:
            return self.target


class WSProxyHandler(socketserver.BaseRequestHandler):
    def handle(self) -> None:
        target = self.server.get_target()
        try:
            upstream = socket.create_connection(target, timeout=5)
        except OSError:
            self.request.close()
            return
        self.request.settimeout(None)
        upstream.settimeout(None)
        peers = {self.request: upstream, upstream: self.request}
        try:
            while True:
                readable, _, _ = select.select([self.request, upstream], [], [], 30)
                if not readable:
                    continue
                for source in readable:
                    data = source.recv(65536)
                    if not data:
                        return
                    peers[source].sendall(data)
        except OSError:
            return
        finally:
            try:
                upstream.close()
            except OSError:
                pass
            try:
                self.request.close()
            except OSError:
                pass


def start_proxy(config: dict[str, dict[str, Any]], target_key: str) -> WSProxy:
    proxy = WSProxy(("127.0.0.1", PORTS["proxy"]))
    proxy.set_target("127.0.0.1", int(config[target_key]["url"].rsplit(":", 1)[1]))
    threading.Thread(target=proxy.serve_forever, daemon=True).start()
    return proxy


def build() -> None:
    LEVIER_BIN.parent.mkdir(parents=True, exist_ok=True)
    run([GO, "build", "-o", str(LEVIER_BIN), "./cmd/levier"], timeout=180, check=True)
    run([GO, "build", "-o", str(DAEMON_BIN), "./tests/e2e/i3daemon"], timeout=180, check=True)


def migrate(config: dict[str, dict[str, Any]]) -> None:
    run(
        [str(LEVIER_BIN), "migrate", "up", "--config", str(config["controlA"]["path"])],
        timeout=120,
        check=True,
    )


def bootstrap_app(config: dict[str, dict[str, Any]], credentials: Path) -> None:
    result = run(
        [
            str(LEVIER_BIN), "app", "init",
            "--name", APP_NAME,
            "--server", str(config["controlA"]["url"]),
            "--admin-token", ADMIN_TOKEN,
            "--credentials-out", str(credentials),
            "--json",
        ],
        timeout=30,
        check=True,
    )
    # The CLI intentionally never prints the key, but the 0600 file contains it.
    if not credentials.exists():
        raise MatrixError(f"app init did not write {credentials}: {result.stdout[-1000:]}")


def scenario_baseline(config, credentials, app_log) -> str:
    start_daemon(config, "rtgwA", delay="200ms")
    run_id = submit(config["controlA"]["url"], credentials)
    final = wait_terminal(config["controlA"]["url"], credentials, run_id)
    if final.get("status") != "completed":
        raise MatrixError(f"baseline run: {final}")
    assert_run_clean(run_id)
    assert_event(run_id, "daemon", "RUN_STATUS")
    if sql_int(
        "SELECT count(*) FROM tool_invocations "
        f"WHERE run_id={sql_quote(run_id)} AND status='completed'"
    ) != 1:
        raise MatrixError("baseline tool invocation was not persisted exactly once")
    wait_app_ws(app_log, "MESSAGE_COMPLETED")
    wait_app_completed(app_log)
    if app_tool_count(app_log, "counter_add") < 1:
        raise MatrixError("baseline App did not execute counter_add")
    return f"run {run_id} completed, event and local tool observed"


def scenario_cross_replica_cancel(config, credentials, app_log) -> str:
    start_daemon(config, "rtgwA", delay="8s")
    run_id = submit(config["controlA"]["url"], credentials)
    wait_run(config["controlA"]["url"], credentials, run_id, {"running"}, 20)
    cancel(config["controlB"]["url"], credentials, run_id)
    final = wait_run(config["controlB"]["url"], credentials, run_id, {"cancelled"}, 20)
    if final.get("status") != "cancelled":
        raise MatrixError(f"cancel run ended as {final.get('status')}")
    assert_run_clean(run_id)
    if sql_int(
        "SELECT count(*) FROM run_events "
        f"WHERE run_id={sql_quote(run_id)} AND source='mw' "
        "AND type='RUN_STATUS' AND payload->>'status'='cancelled'"
    ) < 1:
        raise MatrixError("cancel did not persist the MW cancelled event")
    return f"run {run_id} cancelled through control B while dispatched from control A"


def scenario_control_failure(config, credentials, app_log) -> str:
    start_daemon(config, "rtgwA", delay="3s")
    run_id = submit(config["controlA"]["url"], credentials)
    wait_run(config["controlA"]["url"], credentials, run_id, {"running"}, 20)
    stop("controlA", force=True)
    final = wait_terminal(config["controlB"]["url"], credentials, run_id, 60)
    if final.get("status") != "completed":
        raise MatrixError(f"run {run_id} after control A failure: {final}")
    wait_app_ws(app_log, "MESSAGE_COMPLETED")
    assert_run_clean(run_id)
    if sql_int(
        "SELECT count(*) FROM gateway_notifications "
        "WHERE channel='control.envelope' AND published_at IS NULL"
    ) != 0:
        raise MatrixError("control A failure left undelivered Runtime envelopes")
    return f"run {run_id} survived control A failure and completed on control B"


def scenario_app_disconnect_pending(config, credentials, app_log) -> str:
    start_daemon(config, "rtgwB", delay="3s")
    run_id = submit(config["controlB"]["url"], credentials)
    wait_run(config["controlB"]["url"], credentials, run_id, {"running"}, 20)
    stop("app", force=True)
    final = wait_run(
        config["controlB"]["url"], credentials, run_id,
        {"failed", "cancelled", "timeout", "completed"}, 30,
    )
    if final.get("status") != "failed":
        raise MatrixError(f"run {run_id} after App disconnect: {final}")
    assert_run_clean(run_id)
    start_app(config["controlB"]["url"], credentials, app_log)
    return f"run {run_id} failed its pending local tool without cross-connection retry"


def scenario_notification_replay(config, credentials, app_log) -> str:
    payload = sql(
        "SELECT payload_json::text FROM gateway_notifications "
        "WHERE channel='control.envelope' ORDER BY notification_id DESC LIMIT 1"
    )
    if not payload:
        raise MatrixError("no control envelope is available for replay")
    before = sql_int("SELECT count(*) FROM run_events")
    notification_id = int(sql(
        "INSERT INTO gateway_notifications "
        "(channel, dedupe_key, payload_json, attempts, next_attempt_at, claimed_by, claimed_until) "
        f"VALUES ('control.envelope', 'i3-replay-{time.time_ns()}', "
        f"{sql_quote(payload)}::jsonb, 1, now(), 'i3-dead-claim', now() + interval '2 seconds') "
        "RETURNING notification_id"
    ).splitlines()[0])
    wait_until(
        lambda: sql_int(
            "SELECT count(*) FROM gateway_notifications "
            f"WHERE notification_id={int(notification_id)} AND published_at IS NOT NULL"
        ) == 1,
        10,
        "claimed notification replay",
    )
    after = sql_int("SELECT count(*) FROM run_events")
    if after != before:
        raise MatrixError(
            f"duplicate notification changed event count: {before} -> {after}"
        )
    return f"notification {notification_id} survived a dead claim and replayed idempotently"


def scenario_runtime_gateway_failure(config, credentials, app_log) -> str:
    stop("daemon", force=True)
    stop("rtgwA", force=True)
    wait_runtime_gone(20)
    start_daemon(config, "rtgwB", delay="1s")
    run_id = submit(config["controlB"]["url"], credentials)
    final = wait_terminal(config["controlB"]["url"], credentials, run_id, 60)
    if final.get("status") != "completed":
        raise MatrixError(f"run {run_id} after Runtime Gateway A failure: {final}")
    assert_run_clean(run_id)
    gateway = sql(
        "SELECT gateway_id FROM runtime_connections "
        f"WHERE daemon_id={sql_quote(DAEMON_ID)}"
    )
    if gateway != "runtime-gateway-b":
        raise MatrixError(f"daemon migrated to {gateway!r}, want runtime-gateway-b")
    return f"run {run_id} completed after daemon migrated to Runtime Gateway B"


def scenario_app_gateway_failure(config, credentials, app_log, proxy: WSProxy) -> str:
    before_hello = hello_ack_count(app_log)
    stop("appgwA", force=True)
    wait_gateway_offline("app-gateway-a", 20)
    proxy.set_target("127.0.0.1", int(config["appgwB"]["url"].rsplit(":", 1)[1]))
    wait_app_connection(
        "app-gateway-b", app_log, 30, min_hello_acks=before_hello + 1
    )
    app_id = str(read_credentials(credentials)["app_id"])
    wait_until(
        lambda: sql_int(
            "SELECT count(*) FROM app_execution_leases "
            f"WHERE app_id={sql_quote(app_id)} AND gateway_id='app-gateway-b' "
            "AND expires_at > now()"
        ) >= 1,
        20,
        "execution lease on app-gateway B",
    )
    start_daemon(config, "rtgwB", delay="200ms")
    before = app_tool_count(app_log, "counter_add")
    run_id = submit(config["controlB"]["url"], credentials)
    final = wait_terminal(config["controlB"]["url"], credentials, run_id, 60)
    if final.get("status") != "completed":
        raise MatrixError(f"run {run_id} after App Gateway A failure: {final}")
    wait_until(
        lambda: app_tool_count(app_log, "counter_add") > before,
        20,
        "local tool after App Gateway failover",
    )
    wait_app_ws(app_log, "MESSAGE_COMPLETED")
    wait_app_completed(app_log)
    assert_run_clean(run_id)
    return f"App reconnected to app-gateway B and run {run_id} completed"


def scenario_local_tool_idempotency(config, credentials, app_log) -> str:
    start_daemon(config, "rtgwB", delay="0s", repeat=2)
    before = app_tool_count(app_log, "counter_add")
    run_id = submit(config["controlB"]["url"], credentials)
    final = wait_terminal(config["controlB"]["url"], credentials, run_id, 60)
    if final.get("status") != "completed":
        raise MatrixError(f"duplicate invocation run: {final}")
    wait_until(
        lambda: app_tool_count(app_log, "counter_add") == before + 1,
        20,
        "exactly one App execution for duplicate invocation",
    )
    count = sql_int(
        "SELECT count(*) FROM tool_invocations "
        f"WHERE run_id={sql_quote(run_id)}"
    )
    if count != 1:
        raise MatrixError(f"duplicate invocation produced {count} tool_invocations rows")
    assert_run_clean(run_id)
    return f"run {run_id} delivered the same invocation twice but executed it once"


def scenario_v1_compat(config, credentials, app_log) -> str:
    start_daemon(config, "rtgwB", delay="1s")
    run_id = submit(config["controlB"]["url"], credentials)
    v1_log = OUT / "app-v1-events.jsonl"
    v1_log.unlink(missing_ok=True)
    env = {
        "PYTHONPATH": os.pathsep.join([
            str(ROOT / "sdk/levier/src"),
            str(ROOT / "sdk/python/src"),
        ]),
    }
    start(
        "app-v1",
        [
            PYTHON, str(ROOT / "tests/e2e/i3v1app.py"),
            "--credentials", str(credentials),
            "--server", str(config["controlB"]["url"]),
            "--run-id", run_id,
            "--log", str(v1_log),
        ],
        env=env,
    )
    try:
        wait_until(
            lambda: app_has(
                v1_log,
                lambda record: record.get("kind") == "ws"
                and (record.get("message") or {}).get("type") == "event"
                and (record.get("message") or {}).get("event_type") == "MESSAGE_COMPLETED",
            ),
            30,
            "v1 App WebSocket event",
        )
    finally:
        stop("app-v1", force=True)
    final = wait_terminal(config["controlB"]["url"], credentials, run_id, 60)
    if final.get("status") != "completed":
        raise MatrixError(f"v1 compatibility run: {final}")
    assert_run_clean(run_id)
    return f"v1 App connected without hello and received run {run_id} events"


def write_summary() -> None:
    lines = [
        f"{item['scenario']}: {item['result']} {item['seconds']}s {item['detail']}"
        for item in RESULTS
    ]
    (OUT / "summary.txt").write_text("\n".join(lines) + "\n", encoding="utf-8")


def main() -> int:
    OUT.mkdir(parents=True, exist_ok=True)
    os.chmod(OUT, 0o700)
    credentials = OUT / "credentials.json"
    app_log = OUT / "app-events.jsonl"
    app_log.unlink(missing_ok=True)
    SUBSCRIBE_FILE.unlink(missing_ok=True)
    cleanup_postgres()
    proxy: WSProxy | None = None
    try:
        print(f"evidence: {OUT}")
        start_postgres()
        build()
        config = write_configs()
        migrate(config)
        start_role("controlA", "controlA", config)
        start_role("controlB", "controlB", config)
        start_role("rtgwA", "rtgwA", config)
        start_role("rtgwB", "rtgwB", config)
        start_role("appgwA", "appgwA", config)
        start_role("appgwB", "appgwB", config)
        for instance in (
            "control-a", "control-b",
            "runtime-gateway-a", "runtime-gateway-b",
            "app-gateway-a", "app-gateway-b",
        ):
            wait_gateway(instance)
        proxy = start_proxy(config, "appgwA")
        bootstrap_app(config, credentials)
        start_app(config["controlB"]["url"], credentials, app_log)

        scenarios: list[tuple[str, Callable[[], str]]] = [
            ("baseline", lambda: scenario_baseline(config, credentials, app_log)),
            ("notification_replay", lambda: scenario_notification_replay(config, credentials, app_log)),
            ("cross_replica_cancel", lambda: scenario_cross_replica_cancel(config, credentials, app_log)),
            ("control_failure", lambda: scenario_control_failure(config, credentials, app_log)),
            ("app_disconnect_pending", lambda: scenario_app_disconnect_pending(config, credentials, app_log)),
            ("runtime_gateway_failure", lambda: scenario_runtime_gateway_failure(config, credentials, app_log)),
            ("app_gateway_failure", lambda: scenario_app_gateway_failure(config, credentials, app_log, proxy)),
            ("local_tool_idempotency", lambda: scenario_local_tool_idempotency(config, credentials, app_log)),
            ("v1_app_compat", lambda: scenario_v1_compat(config, credentials, app_log)),
        ]
        for name, scenario in scenarios:
            started = time.monotonic()
            detail = scenario()
            elapsed = round(time.monotonic() - started, 2)
            RESULTS.append({
                "scenario": name, "result": "PASS",
                "seconds": str(elapsed), "detail": detail,
            })
            print(f"PASS {name} ({elapsed}s): {detail}")
    except (MatrixError, subprocess.TimeoutExpired, OSError) as exc:
        RESULTS.append({
            "scenario": "i3-multireplica", "result": "FAIL",
            "seconds": "", "detail": str(exc),
        })
        print(f"FAIL: {exc}", file=sys.stderr)
        for name, proc in PROCS.items():
            print(f"process {name}: returncode={proc.popen.poll()}", file=sys.stderr)
        try:
            print("app connections: " + sql(
                "SELECT app_id, gateway_id, status, protocol_version, "
                "lease_expires_at > now() FROM app_connections ORDER BY connected_at DESC LIMIT 5"
            ), file=sys.stderr)
        except Exception as diag:
            print(f"diagnostics failed: {diag}", file=sys.stderr)
        write_summary()
        return 1
    finally:
        stop_all()
        if proxy is not None:
            proxy.shutdown()
            proxy.server_close()
        cleanup_postgres()
        credentials.unlink(missing_ok=True)
    write_summary()
    print(f"i3-multireplica: PASS ({len(RESULTS)} scenarios, evidence: {OUT})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
