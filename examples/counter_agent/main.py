"""Levier — zero-dependency counter agent baseline.

A standalone ReAct loop with hardcoded counter tools and an
OpenAI-compatible chat-completions client. No Levier imports, no HTTP
server, and no third-party Python packages.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import json
import os
import sys
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

from dispatch import dispatch_counter_tool
from tools import TOOL_DEFINITIONS, Counter

DEFAULT_BASE_URL = "https://api.deepseek.com"
DEFAULT_MODEL = "deepseek-flash"
MAX_STEPS = 8
REQUEST_TIMEOUT_SECONDS = 120


def _openai_tools() -> list[dict[str, Any]]:
    """Build OpenAI/DeepSeek tool specs from the same registration definitions."""
    return [
        {
            "type": "function",
            "function": {
                "name": tool["name"],
                "description": tool["description"],
                "parameters": tool["schema"],
            },
        }
        for tool in TOOL_DEFINITIONS
    ]


TOOLS = _openai_tools()

SYSTEM_PROMPT = """You are a counter agent.
You MUST use the provided tools for every counter read or write.
Never guess the counter value and never do the arithmetic yourself.
For "add 2 then 3", call counter_add twice, then call counter_get.
Only after the tools return may you report the final value."""


def load_dotenv() -> None:
    """Load the repository .env without python-dotenv, preserving real env vars."""
    root = Path(__file__).resolve().parents[2]
    path = root / ".env"
    if not path.exists():
        return
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        key = key.strip()
        value = value.strip().strip("\"'")
        if key and key not in os.environ:
            os.environ[key] = value


def post_json(url: str, api_key: str, payload: dict[str, Any]) -> dict[str, Any]:
    request = urllib.request.Request(
        url,
        data=json.dumps(payload).encode("utf-8"),
        headers={
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
            "Accept": "application/json",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=REQUEST_TIMEOUT_SECONDS) as response:
            return json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"LLM HTTP {exc.code}: {body[:800]}") from exc
    except urllib.error.URLError as exc:
        raise RuntimeError(f"LLM connection failed: {exc.reason}") from exc


def chat(
    base_url: str,
    api_key: str,
    model: str,
    messages: list[dict[str, Any]],
) -> dict[str, Any]:
    payload = {
        "model": model,
        "messages": messages,
        "tools": TOOLS,
        "tool_choice": "auto",
        "temperature": 0,
    }
    response = post_json(base_url.rstrip("/") + "/chat/completions", api_key, payload)
    choices = response.get("choices")
    if not choices:
        raise RuntimeError(f"LLM response has no choices: {json.dumps(response)[:800]}")
    return choices[0]["message"]


def execute_tool(counter: Counter, name: str, arguments_json: str) -> dict[str, Any]:
    """Adapt the local tool dispatcher result for the model message."""
    result = dispatch_counter_tool(counter, name, arguments_json)
    if result["ok"]:
        return json.loads(result["result_json"])
    error = result["error"]
    return {
        "error": error["message"],
        "code": error["code"],
        "retryable": error["retryable"],
    }


def run(prompt: str) -> int:
    load_dotenv()
    api_key = os.environ.get("DEEPSEEK_API_KEY", "").strip()
    if not api_key:
        print("DEEPSEEK_API_KEY is required (put it in the repository .env)", file=sys.stderr)
        return 2
    base_url = os.environ.get("DEEPSEEK_BASE_URL", DEFAULT_BASE_URL)
    model = os.environ.get("DEEPSEEK_MODEL", DEFAULT_MODEL)

    counter = Counter()
    messages: list[dict[str, Any]] = [
        {"role": "system", "content": SYSTEM_PROMPT},
        {"role": "user", "content": prompt},
    ]
    print(f"[model] {model}")
    print(f"[user] {prompt}")

    for step in range(1, MAX_STEPS + 1):
        message = chat(base_url, api_key, model, messages)
        tool_calls = message.get("tool_calls") or []
        assistant_message: dict[str, Any] = {
            "role": "assistant",
            "content": message.get("content") or "",
        }
        if message.get("reasoning_content"):
            assistant_message["reasoning_content"] = message["reasoning_content"]
        if tool_calls:
            assistant_message["tool_calls"] = tool_calls
        messages.append(assistant_message)

        if not tool_calls:
            print(f"[assistant] {message.get('content') or ''}")
            print(f"[final] counter={counter.value} steps={step}")
            return 0

        for call in tool_calls:
            function = call.get("function") or {}
            name = function.get("name", "")
            arguments = function.get("arguments", "{}")
            result = execute_tool(counter, name, arguments)
            print(f"[tool] {name}({arguments}) -> {json.dumps(result, ensure_ascii=False)}")
            messages.append(
                {
                    "role": "tool",
                    "tool_call_id": call.get("id", ""),
                    "name": name,
                    "content": json.dumps(result, ensure_ascii=False),
                }
            )

    print(f"agent exceeded {MAX_STEPS} steps", file=sys.stderr)
    return 1


def main() -> int:
    prompt = " ".join(sys.argv[1:]).strip()
    if not prompt:
        prompt = (
            "Add 2, then add 3, then read the counter and tell me the final value. "
            "Use the counter tools; do not calculate it yourself."
        )
    return run(prompt)


if __name__ == "__main__":
    raise SystemExit(main())
