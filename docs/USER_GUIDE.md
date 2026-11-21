# 从 0 到 1 使用 Levier

这份文档按“人真正会遇到的顺序”编排，而不是按命令树编排。

它回答的是：

```text
我现在要做什么？
下一步是什么？
做完了怎么确认？
出问题时先看哪里？
```

逐命令的完整清单见 [`OPERATIONS_SURFACE.md`](OPERATIONS_SURFACE.md)。那份文档用于查询；本文用于照着做。

---

## 0. 先选你的角色

大多数情况下你只需要其中一条路径：

```text
A. 我是 App 开发者：写工具，让模型调用我的业务函数。
B. 我是 Runtime 开发者：写 run(ctx)，决定 Agent 怎么跑。
C. 我是平台/运维：启动 Levier、配置模型和 MCP、排查 Run。
```

三条路径共享第 1、2 节。第 3 节是 App 路径，第 4 节是 Runtime 路径，第 5 节之后是所有人都会用到的运行和排错。

---

## 1. 第一次启动

### 1.1 安装 CLI

发布版：

```sh
curl -fsSL https://raw.githubusercontent.com/fluffyTighnari/levier/main/scripts/install.sh | sh
```

从源码开发：

```sh
make install
levier setup --server http://127.0.0.1:8080
```

安装器负责：

```text
levier 二进制进入 PATH
下载 Compose 源码包
安装 Python SDK
```

确认命令可用：

```sh
levier version
```

### 1.2 启动本地 Levier

如果你还没有自己的部署：

```sh
levier up
```

它会启动：

```text
PostgreSQL
Levier 控制面
MCP Runtime
内置 React Runtime
内置 Echo Runtime
```

并等待控制面 `/readyz`，然后写入用户级 profile。

确认：

```sh
levier status
```

你应该看到 health、ready、database 和 daemon 状态正常。

停止：

```sh
levier down
```

### 1.3 如果你连接的是已有服务

不需要 `up`，只需要写入全局配置：

```sh
levier setup \
  --server https://your-levier.example.com \
  --admin-token <admin-token>
```

`setup` 写入的是用户级 profile，不是业务项目配置。之后的命令会自动读取。

此时先验证：

```sh
levier status
```

---

## 2. 配置 Levier 的能力

这一节是“Levier 端能提供什么”。只有模型是必需的；MCP 按需接入。

### 2.1 接入模型

你只需要知道三件事：

```text
URL
API key
model 名称
```

执行：

```sh
levier model connect \
  --url https://api.deepseek.com \
  --key sk-... \
  --model deepseek-chat
```

`model connect` 会自动：

```text
推断 provider 名称
推断 adapter
生成 secret_ref
保存 provider 元数据
把 key 写入控制面的 managed secret 文件
设置为默认 provider
检查 secret 是否可用
```

可选参数只在你需要时使用：

```text
--name NAME                 显式 provider 名称
--adapter auto|openai_compatible|anthropic
--models model-a,model-b    允许的模型列表
--default=false             不设为默认
```

确认：

```sh
levier model list
levier model check --name deepseek
```

如果没有模型，App 仍然可以接入，但 React Runtime 无法真正调用模型。

### 2.2 接入 MCP Server

如果你的 App 需要 MCP 工具，先注册 MCP Server：

```sh
levier mcp add \
  --name echo \
  --transport stdio \
  --config-file ./examples/mcp_echo/config.json
```

如果只允许部分工具：

```sh
levier mcp add \
  --name github \
  --transport stdio \
  --config-file ./github-mcp.json \
  --allowed-tools search_issues,get_issue
```

确认：

```sh
levier mcp list
```

App 不直接连接 MCP Server；App 只声明引用，Runtime 通过 MCP Runtime 使用它。

### 2.3 这一步结束时应确认

```text
levier status              正常
levier model list          至少有一个模型
levier mcp list            已注册需要的 MCP Server（如果使用 MCP）
```

---

## 3. 接入 App

这一节是 App 开发者路径。

### 3.1 安装 Python SDK

发布到 PyPI 后：

```sh
pip install levier
```

当前测试期如果 PyPI 还没有完整的 `levier-runtime`，改用 GitHub Release 的
wheel bundle 或 TestPyPI；正式发布后不需要这一步。

从源码开发：

```sh
python -m pip install -e ./sdk/python -e ./execution/runtime_sdk -e ./sdk/levier
```

### 3.2 写工具

新建 `app.py`：

```python
from levier import App, tool

counter = {"value": 0}


@tool
def counter_add(delta: int) -> dict:
    """Add an integer delta to the counter."""
    counter["value"] += delta
    return {"value": counter["value"]}


@tool
def counter_get() -> dict:
    """Read the current counter value."""
    return {"value": counter["value"]}


app = App(tools=[counter_add, counter_get])


if __name__ == "__main__":
    app.run()
```

你只需要写：

```text
业务状态
普通 Python 函数
@tool 装饰器
工具列表
MCP 引用
```

schema 由函数签名和 docstring 自动生成。`App` 默认从项目 `.levier.json` 和
用户 profile 读取 MCP 引用；需要 MCP 时用 `levier init --mcp-ref` 写入，
不需要在 Python 里重复写一遍。

### 3.3 创建项目身份

在项目目录执行：

```sh
levier init
```

它会：

```text
创建或复用 App
写入默认凭据位置
写入用户 profile
写入当前项目的 .levier.json
记录默认 runtime
记录 MCP 引用
```

如果你需要 MCP：

```sh
levier init --mcp-ref echo
```

### 3.4 启动 App

```sh
levier dev
```

`levier dev` 会：

```text
找到 ./app.py
注入凭据和 server
注入 MCP 引用
启动 WebSocket
保持工具在线
```

这个终端要一直保持运行。

也可以直接运行：

```sh
python app.py
```

效果相同。

### 3.5 提交第一次 Run

另开一个终端：

```sh
levier run "Add 2, then add 3, then read the counter."
```

默认行为：

```text
使用当前项目 App
使用默认 runtime
使用默认 provider
等待 Run 到终态
输出最终 assistant 消息
```

如果不想手动开两个终端：

```sh
levier run "Add 2, then add 3, then read the counter." --with-app
```

`--with-app` 会在 Run 期间临时启动 `./app.py`；如果已经有 App 连接，不会重复启动。

### 3.6 在 Python 里提交 Run

CLI 适合终端操作；SDK 适合脚本和测试。

```python
from levier import App

app = App(tools=[], credentials=None)

run = app.submit_run(
    "Add 2, then add 3.",
    runtime="react1",
)
result = app.wait_run(run["run_id"])
events = app.get_events(run["run_id"])
```

或者一次性：

```python
app.run("Add 2, then add 3.")
```

`app.run()` 不带 prompt 时是“服务工具”；带 prompt 时是“临时连接、提交、等待、打印”。

### 3.7 查看 Run

```sh
levier run show <run_id> --events
```

继续观察已有 Run：

```sh
levier run watch <run_id>
```

取消：

```sh
levier run cancel <run_id> --yes
```

### 3.8 App 侧错误处理

你的 handler 可以直接抛异常：

```python
from levier import ToolExecutionError

@tool
def my_tool(value: str) -> dict:
    if not value:
        raise ToolExecutionError("BAD_INPUT", "value is required")
    return {"ok": True}
```

SDK 会把它变成结构化工具结果，不会让 WebSocket 崩溃。

工具失败时先看：

```sh
levier run show <run_id> --events
```

重点事件：

```text
TOOL_CALL_BEGIN
TOOL_CALL_END
ERROR
```

### 3.9 检查 App 接入

```sh
levier app check
```

如果你使用的是显式凭据路径：

```sh
levier app check --credentials ./data/agent/credentials.json
```

检查内容：

```text
本地凭据格式
文件权限
服务端可达性
App key
App active 状态
WebSocket token
```

---

## 4. 接入 Runtime

这一节是 Runtime 开发者路径。

### 4.1 先用内置 Runtime

如果你只是接 App，不需要写 Runtime。

本地栈默认已经有：

```text
react1    内置 ReAct Runtime
echo1     最小 Echo Runtime
```

用 `levier run --runtime echo1` 可以在没有模型的情况下验证整条链路。

### 4.2 写自己的 Runtime

```sh
levier runtime init
```

编辑生成的 `runtime.py`：

```python
from levier import Context, Runtime


async def run(ctx: Context) -> str:
    tools = await ctx.toolbox()

    # 自己决定 prompt 怎么拼
    reply = await ctx.chat(ctx.input, tools=tools)

    # 一行执行模型请求的所有工具，local/MCP 自动分流
    await ctx.invoke_all(reply, toolbox=tools)

    await ctx.reply(reply.content or "done")
    return "completed"


if __name__ == "__main__":
    Runtime.start(run)
```

`ctx` 已经包含：

```text
本次输入
本次模型配置
本地/global 工具
MCP bindings
超时
取消状态
日志上下文
```

`ctx.toolbox()` 负责：

```text
聚合本地工具和 MCP 工具
按 Run 打开 MCP session
统一 call 接口
Run 结束后关闭 session
```

`ctx.chat()` 负责：

```text
使用 ctx.model 的 provider/model/base_url/key
把 Toolbox 转成模型工具描述
返回 content + tool_calls
```

`ctx.invoke_all()` 负责：

```text
解析 tool call
本地工具走 ctx.call_tool
MCP 工具走 toolbox
自动发 TOOL_CALL_BEGIN/END
```

Runtime 作者不需要：

```text
runtime.yaml
runtime_token
daemon id
gRPC / protobuf
event seq / lease_epoch / ack
MCP client
ToolRouter
```

### 4.3 启动 Runtime

```sh
levier runtime start
```

默认会：

```text
查找 runtime.py
从 profile 读取 runtime name、gRPC 地址、token
启动 Python 进程
```

查看已连接 Runtime：

```sh
levier runtime list
```

### 4.4 Runtime 错误处理

Runtime 里未捕获的异常会自动变成：

```text
ERROR 事件
Run failed
```

需要显式错误时：

```python
await ctx.error("MY_ERROR", "something failed", retryable=False)
```

工具错误已经在 `ToolResult` 里：

```python
result = await ctx.call_tool("search", {"query": "levier"})
if not result.ok:
    await ctx.error(result.error_code, result.error_message)
```

---

## 5. 运行、观测与停止

### 5.1 日常状态

```sh
levier status
```

看：

```text
health
ready
database
daemons
apps
runs
```

更细的 Runtime 视图：

```sh
levier runtime list
```

### 5.2 日志

```sh
levier logs
levier logs --follow
```

只看某个阶段时，用：

```sh
levier run show <run_id> --events
```

### 5.3 常用 Run 生命周期

```sh
levier run "prompt"                 # 提交并观察
levier run "prompt" --no-watch      # 只提交
levier run watch <run_id>           # 观察已有 Run
levier run show <run_id> --events   # 查看详情和事件
levier run cancel <run_id> --yes    # 取消
```

### 5.4 停止本地栈

```sh
levier down
```

需要删除数据库卷时：

```sh
levier down --volumes
```

---

## 6. 出问题时先看什么

### 6.1 启动失败

```sh
levier status
levier logs
```

检查：

```text
status 是否 ok
readyz 是否 ok
database 是否 up
daemon 是否 connected
```

### 6.2 模型不可用

```sh
levier model list
levier model check --name <name>
```

检查：

```text
base_url
model 名称
secret 是否 configured
provider 是否 enabled/default
```

### 6.3 App 工具没有被调用

```sh
levier app check
levier run show <run_id> --events
```

重点看：

```text
App 是否在线
工具是否在 manifest 里
是否有 TOOL_CALL_BEGIN
TOOL_CALL_END 的 error_code
```

### 6.4 MCP 工具没有被调用

```sh
levier mcp list
levier run show <run_id> --events
```

检查：

```text
MCP Server 是否注册
App 是否声明了对应的 mcp-ref
allowed_tools 是否过滤掉了工具
```

### 6.5 需要更深的诊断

```sh
levier doctor
levier selfcheck
levier selfcheck --deep
```

这些是诊断命令，不是日常接入步骤。

---

## 7. 运维与管理

日常用户不需要这些；出现运维需求时再查。

### 7.1 本地栈

```sh
levier up
levier ps
levier logs --follow
levier down
```

### 7.2 Run / daemon / task

```sh
levier ops runs list
levier ops runs show --run-id <id>
levier ops runs cancel --run-id <id> --reason "..." --yes
levier ops daemons
levier ops tasks
```

推荐使用统一入口：

```sh
levier admin ops ...
```

### 7.3 配置与数据库

```sh
levier config check
levier config print
levier migrate status
levier migrate up
levier maintenance status
levier maintenance prune --dry-run
```

### 7.4 自检

```sh
levier doctor
levier describe
levier selfcheck
levier selfcheck --deep
```

---

## 8. 这条路径之外，可以忽略什么

下面这些不是新用户接入需要关心的：

```text
levier app ...            显式凭据路径、轮换、底层诊断
levier stack ...          已由 up/down/logs/ps 覆盖
levier run submit ...     已由 run "prompt" 覆盖
levier provider ...       已合并到 levier model
levier serve              运维直接启动控制面
levier admin ...          运维入口
```

其中 `app`、`stack`、`run submit` 目前作为隐藏兼容入口保留，主要用于 e2e 和显式凭据自动化；它们不应出现在新用户的第一条路径里。

---

## 9. 一页命令地图

```text
安装与启动
  levier setup
  levier up
  levier status
  levier down

Levier 配置
  levier model connect
  levier model list
  levier model check
  levier mcp add
  levier mcp list

App
  pip install levier
  levier init
  levier dev
  levier run "prompt"
  levier run "prompt" --with-app
  levier app check

Runtime
  levier runtime init
  levier runtime start
  levier runtime list

观测与排错
  levier run watch
  levier run show --events
  levier run cancel
  levier logs
  levier doctor
  levier selfcheck

运维
  levier admin ops
  levier admin migrate
  levier admin maintenance
  levier admin config
```

完整参数和低层命令见 [`OPERATIONS_SURFACE.md`](OPERATIONS_SURFACE.md)。
