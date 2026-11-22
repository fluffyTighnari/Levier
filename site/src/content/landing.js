export const INSTALL_CMD =
  "curl -fsSL https://raw.githubusercontent.com/fluffyTighnari/levier/main/scripts/install.sh | sh";

export const HERO_CODE = `async def run(ctx: Context) -> str:
    tools = await ctx.toolbox()
    reply = await ctx.chat(ctx.input, tools=tools)
    await ctx.invoke_all(reply, toolbox=tools)
    return "completed"`;

export const FEATURE_CODE = `async def run(ctx: Context) -> str:
    # Local tools + MCP tools, resolved together
    tools = await ctx.toolbox()

    # Model URL and key are resolved by the control plane
    reply = await ctx.chat(ctx.input, tools=tools)

    # Local and MCP calls are routed automatically
    await ctx.invoke_all(reply, toolbox=tools)

    # You decide how many loops run and when to stop
    return "completed"`;

export const RUNTIME_SAMPLE = `from levier import Context, Runtime


async def run(ctx: Context) -> str:
    tools = await ctx.toolbox()
    reply = await ctx.chat(ctx.input, tools=tools)
    await ctx.invoke_all(reply, toolbox=tools)
    await ctx.reply(reply.content or "done")
    return "completed"


Runtime.start(run)`;

export const APP_SAMPLE = `from levier import App, tool

counter = {"value": 0}


@tool
def counter_add(delta: int) -> dict:
    """Add delta to the counter."""
    counter["value"] += delta
    return {"value": counter["value"]}


app = App(tools=[counter_add])
app.run()`;

export const CLI_STEPS = [
  {
    id: "1",
    number: "01",
    title: "Start the local stack",
    hint: "levier up",
    lines: [
      { type: "cmd", segments: [{ text: "$ ", tone: "prompt" }, { text: "levier up" }] },
      { type: "out", segments: [{ text: "waiting for http://127.0.0.1:8080 ..." }] },
      { type: "ok", segments: [{ text: "levier is ready" }] },
      { type: "dim", segments: [{ text: "next:" }] },
      { type: "dim", segments: [{ text: "  levier model connect ..." }] },
      { type: "dim", segments: [{ text: "  levier init" }] },
      { type: "cmd", segments: [{ text: "$ ", tone: "prompt" }, { text: "levier status" }] },
      { type: "ok", segments: [{ text: "status      ok" }] },
      { type: "ok", segments: [{ text: "health      ok" }] },
      { type: "ok", segments: [{ text: "ready       ok" }] },
      { type: "out", segments: [{ text: "database    up" }] },
      { type: "out", segments: [{ text: "daemons     connected=2 healthy=2 capacity=8 used=0" }] },
      { type: "out", segments: [{ text: "apps        registered=0 websocket=0" }] },
      { type: "out", segments: [{ text: "runs        total=0 active=0" }] }
    ]
  },
  {
    id: "2",
    number: "02",
    title: "Connect a model",
    hint: "levier model connect",
    lines: [
      { type: "cmd", segments: [{ text: "$ ", tone: "prompt" }, { text: "levier model connect \\" }] },
      { type: "dim", segments: [{ text: "    " }, { text: "--url", tone: "flag" }, { text: " https://api.deepseek.com \\" }] },
      { type: "dim", segments: [{ text: "    " }, { text: "--key", tone: "flag" }, { text: " sk-... \\" }] },
      { type: "dim", segments: [{ text: "    " }, { text: "--model", tone: "flag" }, { text: " deepseek-chat" }] },
      { type: "ok", segments: [{ text: "model connected: deepseek" }] },
      { type: "out", segments: [{ text: "url:              https://api.deepseek.com" }] },
      { type: "out", segments: [{ text: "model:            deepseek-chat" }] },
      { type: "out", segments: [{ text: "default:          true" }] },
      { type: "dim", segments: [{ text: "next:             levier init" }] },
      { type: "cmd", segments: [{ text: "$ ", tone: "prompt" }, { text: "levier model check " }, { text: "--name", tone: "flag" }, { text: " deepseek" }] },
      { type: "ok", segments: [{ text: "provider check: PASS deepseek" }] }
    ]
  },
  {
    id: "3",
    number: "03",
    title: "Initialize the app",
    hint: "levier init / dev",
    lines: [
      { type: "cmd", segments: [{ text: "$ ", tone: "prompt" }, { text: "levier init" }] },
      { type: "ok", segments: [{ text: "ready: counter-agent" }] },
      { type: "out", segments: [{ text: "app:     app_01..." }] },
      { type: "out", segments: [{ text: "server:  http://127.0.0.1:8080" }] },
      { type: "out", segments: [{ text: "runtime: react1" }] },
      { type: "dim", segments: [{ text: "next:" }] },
      { type: "dim", segments: [{ text: "  levier dev            # serve this project's tools" }] },
      { type: "dim", segments: [{ text: "  levier run \"hello\"    # submit a run without flags" }] },
      { type: "cmd", segments: [{ text: "$ ", tone: "prompt" }, { text: "levier dev" }] },
      { type: "out", segments: [{ text: "app:     /path/to/app.py" }] },
      { type: "out", segments: [{ text: "server:  http://127.0.0.1:8080" }] }
    ]
  },
  {
    id: "4",
    number: "04",
    title: "Submit and schedule",
    hint: "levier run",
    lines: [
      { type: "cmd", segments: [{ text: "$ ", tone: "prompt" }, { text: "levier run \"Mark order A-1042 as shipped\"" }] },
      { type: "out", segments: [{ text: "run submitted: 01J..." }] },
      { type: "out", segments: [{ text: "status: queued" }] },
      { type: "out", segments: [{ text: "status: assigned" }] },
      { type: "out", segments: [{ text: "status: running" }] },
      { type: "ok", segments: [{ text: "tool -> orders_mark_shipped" }] },
      { type: "ok", segments: [{ text: "tool <- orders_mark_shipped ok" }] },
      { type: "ok", segments: [{ text: "status: completed" }] },
      { type: "out", segments: [{ text: "runtime_name: react1" }] },
      { type: "ok", segments: [{ text: "final: shipped via SF" }] }
    ]
  },
  {
    id: "5",
    number: "05",
    title: "Observe and stop",
    hint: "runtime list / logs / down",
    lines: [
      { type: "cmd", segments: [{ text: "$ ", tone: "prompt" }, { text: "levier runtime list" }] },
      { type: "dim", segments: [{ text: "runtime   instance         status    capacity  running  available" }] },
      { type: "out", segments: [{ text: "react1    daemon-local-1   healthy   4         0        4" }] },
      { type: "out", segments: [{ text: "echo1     daemon-local-2   healthy   4         0        4" }] },
      { type: "cmd", segments: [{ text: "$ ", tone: "prompt" }, { text: "levier logs " }, { text: "--follow", tone: "flag" }] },
      { type: "out", segments: [{ text: "levier-1         | run 01J... assigned to react1" }] },
      { type: "out", segments: [{ text: "react-runtime-1  | run 01J... completed" }] },
      { type: "cmd", segments: [{ text: "$ ", tone: "prompt" }, { text: "levier down" }] }
    ]
  }
];

export const CONTRACT_STEPS = [
  {
    label: "Runtime",
    title: "run(ctx)",
    body: "Chooses the model, tools, loop and return value."
  },
  {
    label: "Levier",
    title: "Control plane",
    body: "Schedules the run, routes tool calls, records state and events."
  },
  {
    label: "App",
    title: "App.run()",
    body: "Serves @tool functions, keeps business state and returns results."
  }
];

export const TODAY = [
  "Python App SDK",
  "Python Runtime SDK",
  "Local CLI control plane"
];

export const NEXT = [
  "TypeScript, Java and Go SDKs",
  "Sandbox runtimes",
  "Long-running runtimes",
  "Multi-agent runs",
  "Streaming"
];

export const DOC_MAP = [
  {
    index: "01",
    title: "Start here",
    description: "Install, start, models and MCP, App, Runtime and observability.",
    href: "./docs/index.html#/onboarding",
    cta: "Full setup"
  },
  {
    index: "02",
    title: "Configure capabilities",
    description: "Model URL, key and name, MCP servers, config precedence and secrets.",
    href: "./docs/index.html#/pre-model",
    cta: "Model connection"
  },
  {
    index: "03",
    title: "App.run and tools",
    description: "@tool, App.run, events, credentials and app-side management.",
    href: "./docs/index.html#/app-core",
    cta: "App core"
  },
  {
    index: "04",
    title: "CLI control plane",
    description: "Local stack, status, runs, runtime list and operations.",
    href: "./docs/index.html#/levier-install",
    cta: "CLI control plane"
  },
  {
    index: "05",
    title: "Write your run(ctx)",
    description: "Context, Toolbox, instance capacity, remote deployment and YAML compatibility.",
    href: "./docs/index.html#/runtime-core",
    cta: "Runtime core"
  },
  {
    index: "06",
    title: "Commands and interfaces",
    description: "CLI, SDK, environment variables, error codes and glossary.",
    href: "./docs/index.html#/cli",
    cta: "CLI reference"
  }
];
