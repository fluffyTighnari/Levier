# 五分钟接入 Levier

这一页只写用户真正要做的事。凭据路径、secret 引用、adapter、runtime token、
CLI 参数和错误封装都由 SDK 处理。

---

## 1. 安装

发布版：

```sh
curl -fsSL https://raw.githubusercontent.com/fluffyTighnari/levier/main/scripts/install.sh | sh
```

安装器会把 `levier` 放进 PATH，并安装 Python SDK。Windows 使用
[`scripts/install.ps1`](../scripts/install.ps1)。

从源码开发：

```sh
make install
levier setup --server http://127.0.0.1:8080
```

`setup` 写入的是用户级配置，不是业务项目配置。之后在任意目录执行 `levier`。

---

## 2. 启动本地 Levier

```sh
levier up
```

它会启动 PostgreSQL、控制面、MCP Runtime、内置 React Runtime 和 Echo Runtime，
等待 `/readyz`，并把 server、runtime 默认值和本地 runtime token 写进用户配置。

查看：

```sh
levier status
```

停止：

```sh
levier down
```

---

## 3. 配置模型

你只需要知道三件事：URL、API key、model。

```sh
levier model connect \
  --url https://api.deepseek.com \
  --key sk-... \
  --model deepseek-chat
```

`levier model connect` 会：

```text
从 URL 推断 provider 名称
从 URL 推断 adapter
生成 secret_ref
保存 provider 元数据
把 key 写入控制面的 0600 managed secret 文件
把它设为默认 provider
立即检查 secret 是否可用
```

不需要设置 `LEVIER_SECRET_*`，也不需要理解 `--secret-ref`。

本地栈使用 HTTP；连接远程控制面时请使用 HTTPS，避免 key 在传输中暴露。

---

## 4. 接入你的 Python 项目

安装 SDK：

```sh
pip install levier
```

从仓库开发时：

```sh
python -m pip install -e ./sdk/python -e ./execution/runtime_sdk -e ./sdk/levier
```

在项目里新增 `app.py`：

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


app = App(
    tools=[counter_add, counter_get],
    mcp=[],  # 需要哪些 MCP Server 就写在这里，例如 ["github"]
)


if __name__ == "__main__":
    app.run()
```

只需要改这些：

```text
导入 levier 包
把普通 Python 函数加上 @tool
把 tools 交给 App
声明要用的 MCP
```

schema 由函数签名和 docstring 自动生成。不需要手写 JSON Schema。

设置这个项目的 App 身份：

```sh
levier init
```

然后启动 App：

```sh
levier dev
```

`levier dev` 默认找 `app.py`，自动注入凭据、server 和 MCP 引用。
你也可以直接运行 `python app.py`，效果一样。

---

## 5. 提交并观测 Run

另开一个终端：

```sh
levier run "Add 2, then add 3, then read the counter."
```

如果不想手动开两个终端，也可以让命令临时启动 `app.py`：

```sh
levier run "Add 2, then add 3, then read the counter." --with-app
```

默认使用当前项目的 App、默认 runtime、默认 provider，并持续观测到终态。
不再需要：

```text
--credentials
--credentials-out
--admin-token
--runtime
--provider
--model
--watch
```

需要高级控制时才用：

```sh
levier run "..." --runtime echo1 --no-watch
levier run show <run_id> --events
levier run cancel <run_id>
```

---

## 6. 接入自己的 Runtime

```sh
levier runtime init
```

会生成 `runtime.py`。把逻辑写成：

```python
from levier import Context, Runtime


async def run(ctx: Context) -> str:
    # 一行取出所有本地工具和 MCP 工具
    tools = await ctx.toolbox()

    # 一行取出所有 MCP 工具（如果只想自己拼接）
    mcp_tools = tools.mcp()

    # 一行调用模型；模型地址和 key 已经在 ctx.model 里
    reply = await ctx.chat(ctx.input, tools=tools)

    # 一行解析并执行模型请求的所有工具（local 和 MCP 自动分流）
    results = await ctx.invoke_all(reply, toolbox=tools)

    # 工具错误已经在 ToolResult 里；未捕获异常会自动变成 ERROR + failed Run
    await ctx.reply(reply.content or "done")
    return "completed"


if __name__ == "__main__":
    Runtime.start(run)
```

上面只演示一行式 API；是否把 `results` 再拼回下一轮 prompt、循环几次，由你的业务逻辑决定。

启动：

```sh
levier runtime start
```

不需要：

```text
runtime.yaml
runtime_token
daemon id
middleware address
gRPC / protobuf
event seq / lease_epoch / ack
```

`levier runtime start` 从用户配置里取地址、runtime 名称和本地 runtime token。
远程部署时，operator 只需在 `levier setup --runtime-token ...` 时提供一次。

---

## 7. 用户不需要知道的内部词

```text
credential_version
secret_ref
LEVIER_SECRET_<REF>
LEVIER_SECRETS_FILE
daemon token
ws_token
lease_epoch
seq
ack
local_tool_call / local_tool_result
RuntimeChannel
Hello / Heartbeat
```

这些仍然是实现的一部分，但不是接入契约。

---

## 8. 仍然是可脚本化的

所有高层命令都支持 `--json`：

```sh
levier status --json
levier init --json
levier model connect ... --json
levier run "..." --json
```

内部/运维入口保留，但不再作为用户命令：

```text
levier app init|show|check|rotate-key   # 显式凭据路径/轮换
levier run submit|watch|show|cancel     # 低层 Run 控制
levier mcp add|list|remove
levier admin ...
```

`provider` 命令已收敛为 `model`。

它们不再出现在主帮助和黄金路径里。
