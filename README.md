# Levier

**A run function is all you need.**

Levier is a local control plane for LLM agent runs. It schedules runs, routes tool calls, records state and events, and gives you one CLI.

It is not an agent framework. You keep the runtime loop and the app tools; Levier owns the run around them.

Current release: `0.1.0` · Python App + Python Runtime.

## Why it stays small

- **One runtime function.** `run(ctx)` is all a runtime implements. Decide the model calls, tool order, loop and stop condition.
- **One app entry.** Decorate tools with `@tool`; `App.run()` serves their calls and returns the results.
- **One local control plane.** Scheduling, run state, events and logs live in the CLI.
- **One machine.** Apps, runtimes, models and MCP servers can all run locally.
- **No runtime.yaml, daemon ID or gRPC.** Connect the function; Levier handles the run.

## App: `@tool` and `App.run()`

```python
from levier import App, tool

counter = {"value": 0}

@tool
def counter_add(delta: int) -> dict:
    """Add delta to the counter."""
    counter["value"] += delta
    return {"value": counter["value"]}

@tool
def counter_get() -> dict:
    """Read the counter."""
    return {"value": counter["value"]}

app = App(tools=[counter_add, counter_get])
app.run()
```

<details>
<summary>Before: framework wiring</summary>

```python
from langchain.agents import AgentExecutor, create_openai_tools_agent
from langchain.tools import StructuredTool
from langchain_openai import ChatOpenAI
from langchain_core.prompts import ChatPromptTemplate

# counter_add / counter_get already exist as Python functions.
tools = [
    StructuredTool.from_function(counter_add),
    StructuredTool.from_function(counter_get),
]
llm = ChatOpenAI(model="gpt-4o", temperature=0)
prompt = ChatPromptTemplate.from_messages([
    ("system", "You are a counter assistant."),
    ("user", "{input}"),
    ("agent_scratchpad", "{agent_scratchpad}"),
])
agent = create_openai_tools_agent(llm, tools, prompt)
AgentExecutor(agent=agent, tools=tools).invoke({"input": "add 5, then read"})
```

</details>

## Runtime: one async function

```python
from levier import Context, Runtime

async def run(ctx: Context) -> str:
    tools = await ctx.toolbox()
    reply = await ctx.chat(ctx.input, tools=tools)
    await ctx.invoke_all(reply, toolbox=tools)
    return "completed"

Runtime.start(run)
```

<details>
<summary>Before: hand-written transport, schema, dispatch and loop</summary>

```text
You build and keep:
  the transport
  the tool schema
  local/MCP dispatch
  MCP connections and reconnects
  retries, errors and events
  the model loop and output stream
```

</details>

Neither side imports the other. The Runtime gets tools through `ctx`; the App serves them through `@tool`. The same `run(ctx)` works with the local scheduler or a remote runtime.

## Quick start

Linux / macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/fluffyTighnari/Levier/main/scripts/install.sh | sh
```

Windows: use the [install guide](https://fluffytighnari.github.io/Levier/docs/#/levier-install).

```sh
levier up
levier status
```

Connect a model, save the App sample above as `app.py`, initialize the project, and submit a run:

```sh
levier model connect --url https://api.deepseek.com --key sk-... --model deepseek-chat

levier init
levier run "Add 2, then add 3, then read the counter." --with-app
```

Stop the stack with `levier down`.

## What works today

Python App SDK, Python Runtime SDK and the local CLI control plane.

**Next:** TypeScript, Java and Go SDKs; sandbox and long-running runtimes; multi-agent runs; streaming.

## Documentation

[Five-minute setup and full reference](https://fluffytighnari.github.io/Levier/docs/)
