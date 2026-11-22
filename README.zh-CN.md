# Levier

[English](README.md) · **中文**

**一个 `run` 函数就够了。**

Levier 是 LLM Agent Run 的本地控制面。它负责调度 Run、路由工具调用、记录状态与事件，并提供一个 CLI。

它不是 Agent 框架。Runtime 循环和 App 工具仍由你掌握，Levier 只负责它们外面的 Run。

当前版本：`0.1.0` · Python App + Python Runtime。

## 为什么它保持轻量

- **一个 Runtime 函数。** `run(ctx)` 就是 Runtime 需要实现的全部。模型调用、工具顺序、循环和停止条件都由你决定。
- **一个 App 入口。** 用 `@tool` 注册工具；`App.run()` 接收调用并返回结果。
- **一个本地控制面。** 调度、Run 状态、事件和日志都在 CLI 中。
- **一台机器。** App、Runtime、模型和 MCP Server 都可以在本地运行。
- **不需要 `runtime.yaml`、daemon ID 或 gRPC。** 接入函数，Run 交给 Levier。

## App：`@tool` 与 `App.run()`

```python
from levier import App, tool

counter = {"value": 0}

@tool
def counter_add(delta: int) -> dict:
    """把 delta 加到计数器。"""
    counter["value"] += delta
    return {"value": counter["value"]}

@tool
def counter_get() -> dict:
    """读取计数器。"""
    return {"value": counter["value"]}

app = App(tools=[counter_add, counter_get])
app.run()
```

<details>
<summary>之前：框架接线</summary>

```python
from langchain.agents import AgentExecutor, create_openai_tools_agent
from langchain.tools import StructuredTool
from langchain_openai import ChatOpenAI
from langchain_core.prompts import ChatPromptTemplate

# counter_add / counter_get 已经是普通 Python 函数。
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

## Runtime：一个 async 函数

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
<summary>之前：手写传输、Schema、分发和循环</summary>

```text
你需要自己实现并维护：
  传输层
  工具 Schema
  本地 / MCP 分发
  MCP 连接与重连
  重试、错误与事件
  模型循环与输出流
```

</details>

两侧互不导入。Runtime 通过 `ctx` 获取工具，App 通过 `@tool` 提供工具。同一个 `run(ctx)` 可以运行在本地调度器或远程 Runtime 上。

## 快速开始

Linux / macOS：

```sh
curl -fsSL https://raw.githubusercontent.com/fluffyTighnari/Levier/main/scripts/install.sh | sh
```

Windows：使用[安装指南](https://fluffytighnari.github.io/Levier/docs/#/levier-install)。

```sh
levier up
levier status
```

连接模型，将上面的 App 示例保存为 `app.py`，初始化项目并提交 Run：

```sh
levier model connect --url https://api.deepseek.com --key sk-... --model deepseek-chat

levier init
levier run "Add 2, then add 3, then read the counter." --with-app
```

用 `levier down` 停止本地栈。

## 目前可用

Python App SDK、Python Runtime SDK 和本地 CLI 控制面。

**下一步：** TypeScript、Java 和 Go SDK；沙箱与长运行 Runtime；多 Agent Run；流式输出。

## 文档

[五分钟上手与完整参考](https://fluffytighnari.github.io/Levier/docs/)
