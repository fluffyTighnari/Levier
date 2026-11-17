"""Levier — minimal stdlib WebSocket client.

Implements only what the App retrofit needs: HTTP Upgrade, masked client
text frames, unmasked server text frames, ping/pong, and close.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import base64
import hashlib
import json
import os
import socket
import ssl
import struct
from typing import Any
from urllib.parse import urlencode, urlsplit

_GUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"


class WebSocketError(RuntimeError):
    """WebSocket handshake or frame error."""


def websocket_url(base_url: str, ws_path: str, app_id: str, token: str) -> str:
    """Build a ws/wss URL from LEVIER_URL, ws_url, app_id, and token."""
    parts = urlsplit(base_url)
    scheme = "wss" if parts.scheme == "https" else "ws"
    if ws_path.startswith("ws://") or ws_path.startswith("wss://"):
        target = urlsplit(ws_path)
        scheme = target.scheme
        host = target.netloc
        path = target.path or "/"
        query = target.query
    else:
        host = parts.netloc
        path = ws_path or "/"
        query = ""
    separator = "&" if query else ""
    query += separator + urlencode({"app_id": app_id, "token": token})
    return f"{scheme}://{host}{path}?{query}"


def accept_key(client_key: str) -> str:
    """Compute Sec-WebSocket-Accept."""
    digest = hashlib.sha1((client_key + _GUID).encode("ascii")).digest()
    return base64.b64encode(digest).decode("ascii")


class WebSocketClient:
    """A very small RFC 6455 client for JSON text messages."""

    def __init__(self, sock: socket.socket, buffer: bytes = b"") -> None:
        self.sock = sock
        self.buffer = buffer

    @classmethod
    def connect(cls, url: str, timeout: float = 5.0) -> "WebSocketClient":
        parts = urlsplit(url)
        if parts.scheme not in ("ws", "wss"):
            raise WebSocketError(f"unsupported scheme {parts.scheme!r}")
        host = parts.hostname or ""
        port = parts.port or (443 if parts.scheme == "wss" else 80)
        path = parts.path or "/"
        if parts.query:
            path += "?" + parts.query
        sock = socket.create_connection((host, port), timeout=timeout)
        if parts.scheme == "wss":
            context = ssl.create_default_context()
            sock = context.wrap_socket(sock, server_hostname=host)

        key = base64.b64encode(os.urandom(16)).decode("ascii")
        request = (
            f"GET {path} HTTP/1.1\r\n"
            f"Host: {host}:{port}\r\n"
            "Upgrade: websocket\r\n"
            "Connection: Upgrade\r\n"
            f"Sec-WebSocket-Key: {key}\r\n"
            "Sec-WebSocket-Version: 13\r\n"
            "\r\n"
        )
        sock.sendall(request.encode("ascii"))
        header_bytes, remainder = _read_until(sock, b"\r\n\r\n")
        headers = _parse_headers(header_bytes)
        if headers.get("status") != "101":
            sock.close()
            raise WebSocketError(f"handshake failed: {headers.get('status')}")
        if headers.get("sec-websocket-accept") != accept_key(key):
            sock.close()
            raise WebSocketError("handshake accept key mismatch")
        return cls(sock, remainder)

    def send_json(self, payload: dict[str, Any]) -> None:
        self._send_frame(0x1, json.dumps(payload).encode("utf-8"))

    def recv_json(self, timeout: float = 5.0) -> dict[str, Any]:
        self.sock.settimeout(timeout)
        while True:
            opcode, payload = self._recv_frame()
            if opcode == 0x1:
                return json.loads(payload.decode("utf-8"))
            if opcode == 0x8:
                raise WebSocketError("server closed the connection")
            if opcode == 0x9:
                self._send_frame(0xA, payload)
            elif opcode == 0xA:
                continue

    def close(self) -> None:
        try:
            self._send_frame(0x8, b"")
        except OSError:
            pass
        try:
            self.sock.close()
        except OSError:
            pass

    def _send_frame(self, opcode: int, payload: bytes) -> None:
        header = bytearray([0x80 | opcode])
        length = len(payload)
        if length < 126:
            header.append(0x80 | length)
        elif length < (1 << 16):
            header.append(0x80 | 126)
            header.extend(struct.pack("!H", length))
        else:
            header.append(0x80 | 127)
            header.extend(struct.pack("!Q", length))
        mask = os.urandom(4)
        header.extend(mask)
        masked = bytes(byte ^ mask[index % 4] for index, byte in enumerate(payload))
        self.sock.sendall(bytes(header) + masked)

    def _recv_frame(self) -> tuple[int, bytes]:
        header = self._read_exact(2)
        first, second = header[0], header[1]
        opcode = first & 0x0F
        masked = bool(second & 0x80)
        length = second & 0x7F
        if length == 126:
            length = struct.unpack("!H", self._read_exact(2))[0]
        elif length == 127:
            length = struct.unpack("!Q", self._read_exact(8))[0]
        mask = self._read_exact(4) if masked else b""
        payload = self._read_exact(length)
        if masked:
            payload = bytes(byte ^ mask[index % 4] for index, byte in enumerate(payload))
        return opcode, payload

    def _read_exact(self, size: int) -> bytes:
        data = bytearray()
        if self.buffer:
            take = min(size, len(self.buffer))
            data.extend(self.buffer[:take])
            self.buffer = self.buffer[take:]
        while len(data) < size:
            chunk = self.sock.recv(size - len(data))
            if not chunk:
                raise WebSocketError("connection closed")
            data.extend(chunk)
        return bytes(data)


def _read_until(sock: socket.socket, marker: bytes) -> tuple[bytes, bytes]:
    data = bytearray()
    while marker not in data:
        chunk = sock.recv(4096)
        if not chunk:
            raise WebSocketError("connection closed during handshake")
        data.extend(chunk)
    index = data.index(marker) + len(marker)
    return bytes(data[:index]), bytes(data[index:])


def _parse_headers(raw: bytes) -> dict[str, str]:
    lines = raw.decode("iso-8859-1").split("\r\n")
    headers: dict[str, str] = {}
    if not lines:
        return headers
    parts = lines[0].split(" ", 2)
    if len(parts) >= 2:
        headers["status"] = parts[1]
    for line in lines[1:]:
        if not line or ":" not in line:
            continue
        key, value = line.split(":", 1)
        headers[key.strip().lower()] = value.strip()
    return headers
