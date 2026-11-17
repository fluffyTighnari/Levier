#!/usr/bin/env node
// Levier — Counter App in Node.js (P8).
//
// Standard library only: no npm packages, no Levier SDK, no generated code.
// It verifies the App key, registers tools, opens the App WebSocket, and serves
// local_tool_call messages using the documented JSON contract.
//
// Usage:
//   node serve.mjs --credentials /tmp/app.json --mw-url http://127.0.0.1:8080 \
//     --calls 3 --idle-seconds 30

import http from "node:http";
import https from "node:https";
import { readFileSync } from "node:fs";

const TOOLS = [
  {
    name: "counter_add",
    description: "Add an integer delta to the counter.",
    schema: {
      type: "object",
      properties: { delta: { type: "integer" } },
      required: ["delta"],
      additionalProperties: false,
    },
    timeout_ms: 10000,
  },
  {
    name: "counter_get",
    description: "Read the current counter value.",
    schema: { type: "object", properties: {}, additionalProperties: false },
    timeout_ms: 10000,
  },
  {
    name: "counter_reset",
    description: "Reset the counter to zero.",
    schema: { type: "object", properties: {}, additionalProperties: false },
    timeout_ms: 10000,
  },
];

function parseArgs(argv) {
  const args = {
    credentials: "",
    mwUrl: process.env.LEVIER_URL || process.env.MW_URL || "http://127.0.0.1:8080",
    calls: 0,
    idleSeconds: 30,
  };
  for (let index = 0; index < argv.length; index += 1) {
    const flag = argv[index];
    const value = argv[index + 1];
    if (flag === "--credentials") {
      args.credentials = value;
      index += 1;
    } else if (flag === "--mw-url") {
      args.mwUrl = value;
      index += 1;
    } else if (flag === "--calls") {
      args.calls = Number(value);
      index += 1;
    } else if (flag === "--idle-seconds") {
      args.idleSeconds = Number(value);
      index += 1;
    }
  }
  if (!args.credentials) {
    throw new Error("--credentials is required");
  }
  return args;
}

function log(payload) {
  process.stdout.write(JSON.stringify(payload) + "\n");
}

function request(method, url, { token, body } = {}) {
  return new Promise((resolve, reject) => {
    const parsed = new URL(url);
    const transport = parsed.protocol === "https:" ? https : http;
    const headers = { Accept: "application/json" };
    let payload = null;
    if (body !== undefined) {
      payload = Buffer.from(JSON.stringify(body));
      headers["Content-Type"] = "application/json";
      headers["Content-Length"] = payload.length;
    }
    if (token) {
      headers.Authorization = "Bearer " + token;
    }
    const req = transport.request(parsed, { method, headers }, (res) => {
      const chunks = [];
      res.on("data", (chunk) => chunks.push(chunk));
      res.on("end", () => {
        const text = Buffer.concat(chunks).toString("utf8");
        let json = {};
        try {
          json = text ? JSON.parse(text) : {};
        } catch {
          json = { raw: text };
        }
        resolve({ status: res.statusCode, json });
      });
    });
    req.on("error", reject);
    if (payload) {
      req.write(payload);
    }
    req.end();
  });
}

function dispatch(name, argumentsJson, state) {
  if (!TOOLS.some((tool) => tool.name === name)) {
    return { ok: false, error: { code: "TOOL_NOT_FOUND", message: `unknown tool ${name}`, retryable: false } };
  }
  let args = {};
  try {
    args = argumentsJson ? JSON.parse(argumentsJson) : {};
  } catch (error) {
    return { ok: false, error: { code: "TOOL_EXEC_ERROR", message: "invalid arguments_json", retryable: false } };
  }
  if (name === "counter_get") {
    return { ok: true, result_json: JSON.stringify({ value: state.value }) };
  }
  if (name === "counter_reset") {
    state.value = 0;
    return { ok: true, result_json: JSON.stringify({ value: state.value }) };
  }
  const delta = args.delta;
  if (!Number.isInteger(delta)) {
    return { ok: false, error: { code: "TOOL_EXEC_ERROR", message: "counter_add.delta must be an integer", retryable: false } };
  }
  state.value += delta;
  return { ok: true, result_json: JSON.stringify({ value: state.value }) };
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const credentials = JSON.parse(readFileSync(args.credentials, "utf8"));
  const base = args.mwUrl.replace(/\/$/, "");

  const app = await request("GET", `${base}/api/v1/apps/${credentials.app_id}`, { token: credentials.api_key });
  if (app.status !== 200 || app.json.status !== "active") {
    throw new Error(`app verification failed: ${app.status}`);
  }

  const registered = await request("POST", `${base}/api/v1/apps`, {
    token: credentials.api_key,
    body: { name: credentials.app_name, multiagent: false, tools: TOOLS },
  });
  if (registered.status !== 200) {
    throw new Error(`tool registration failed: ${registered.status}`);
  }

  const token = await request("POST", `${base}/api/v1/ws-token`, { token: credentials.api_key });
  if (token.status !== 200) {
    throw new Error(`ws-token failed: ${token.status}`);
  }
  const wsBase = base.replace(/^http/, "ws");
  const url = `${wsBase}${token.json.ws_url}?app_id=${encodeURIComponent(credentials.app_id)}&token=${encodeURIComponent(token.json.ws_token)}`;

  const state = { value: 0 };
  let handled = 0;
  let idleTimer = null;
  const ws = new WebSocket(url);

  const finish = (code, reason) => {
    if (idleTimer) {
      clearTimeout(idleTimer);
      idleTimer = null;
    }
    log({ stopped: true, reason, handled, counter: state.value });
    try {
      ws.close();
    } catch {
      // already closed
    }
    setTimeout(() => process.exit(code), 20);
  };

  const resetIdle = () => {
    if (idleTimer) {
      clearTimeout(idleTimer);
    }
    idleTimer = setTimeout(() => finish(handled === 0 ? 1 : 0, "idle"), args.idleSeconds * 1000);
  };

  ws.addEventListener("open", () => {
    log({
      serving: true,
      app_id: credentials.app_id,
      registered_tools: TOOLS.map((tool) => tool.name),
      counter: state.value,
    });
    ws.send(JSON.stringify({ type: "ping" }));
    resetIdle();
  });

  ws.addEventListener("error", (event) => {
    log({ socket_error: String(event.message || "websocket error") });
  });

  ws.addEventListener("close", () => {
    if (idleTimer) {
      clearTimeout(idleTimer);
      idleTimer = null;
    }
  });

  ws.addEventListener("message", (event) => {
    resetIdle();
    let message = null;
    try {
      message = JSON.parse(String(event.data));
    } catch {
      return;
    }
    if (message.type === "pong") {
      log({ pong: true });
      return;
    }
    if (message.type === "error") {
      log({ server_error: message });
      return;
    }
    if (message.type !== "local_tool_call") {
      return;
    }
    const outcome = dispatch(message.tool_name, message.arguments_json, state);
    const result = {
      type: "local_tool_result",
      correlation_id: message.msg_id,
      invocation_id: message.invocation_id,
    };
    if (outcome.ok) {
      result.ok = true;
      result.result_json = outcome.result_json;
    } else {
      result.ok = false;
      result.error = outcome.error;
    }
    ws.send(JSON.stringify(result));
    handled += 1;
    log({
      local_tool_call: {
        msg_id: message.msg_id,
        invocation_id: message.invocation_id,
        tool_name: message.tool_name,
        arguments_json: message.arguments_json,
      },
      local_tool_result: result,
      counter: state.value,
    });
    if (args.calls > 0 && handled >= args.calls) {
      finish(0, "count");
    }
  });
}

main().catch((error) => {
  process.stderr.write(`serve failed: ${error.message}\n`);
  process.exit(1);
});
