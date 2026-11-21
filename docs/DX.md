# 操作面设计：少做事，而不是多封装

> 状态：黄金路径第一版已实现。目标是让 App 作者和 Runtime 作者只写业务逻辑，
> 协议、凭据、CLI 包装和错误收敛都由 SDK 负责。

---

## 1. 判断标准

任何一个接入动作，先问三个问题：

```text
用户是不是必须理解这个概念，才能写业务逻辑？
这个值是不是可以由项目/用户配置推导？
这个命令能不能只表达一个意图？
```

如果答案是否定的，它就不应该出现在黄金路径里。

因此：

```text
默认值 > 必填参数
项目配置 > 用户每次输入
一个命令 > 一组参数
SDK 包装 > 用户复制样板代码
```

防御性检查仍然存在，但它们放在实现内部和错误信息里，不放在接入清单里。

---

## 2. 黄金路径

```sh
# 安装（二选一）
curl -fsSL .../install.sh | sh
make install && levier setup --server http://127.0.0.1:8080

# 启动
levier up

# 模型
levier model connect --url https://api.deepseek.com --key sk-... --model deepseek-chat

# 项目接入
pip install levier
levier init
levier dev
levier run "hello"
levier run "hello" --with-app     # 或者一条命令临时启动 app.py

# 自己的 Runtime
levier runtime init
levier runtime start
```

对应的 Python 面：

```python
# App
from levier import App, tool

@tool
def counter_add(delta: int) -> dict:
    """Add delta to the counter."""
    return {"value": delta}

app = App(tools=[counter_add], mcp=[])
app.run()

# Runtime
from levier import Context, Runtime

async def run(ctx: Context) -> str:
    tools = await ctx.toolbox()
    reply = await ctx.chat(ctx.input, tools=tools)
    await ctx.invoke_all(reply, toolbox=tools)
    await ctx.reply(reply.content or "done")
    return "completed"

Runtime.start(run)
```

---

## 3. 隐藏了什么

以下概念仍然存在，但不再要求用户逐项配置：

| 内部概念 | 用户看到的东西 |
|---|---|
| `--credentials-out` / `--credentials` | `levier init` + 项目配置 |
| Admin token | `levier up` 的本地默认、`setup --admin-token` 或环境变量 |
| Provider adapter | 从 URL 推断 |
| `secret_ref` | 从 provider 名称生成 |
| `LEVIER_SECRET_*` / secret file | `levier model connect` 直接写 managed 文件 |
| runtime token / daemon id | `levier up` 写入本地凭据，runtime 自动读取 |
| Runtime YAML | `Runtime.start()` 从 profile 推导 |
| gRPC / protobuf | SDK 内部 |
| event seq / lease_epoch / ack | SDK 内部 |
| tool JSON Schema | `@tool` 从函数签名和 docstring 生成 |
| tool / MCP 分流 | `ctx.toolbox()` + `ctx.invoke_all()` |
| 未捕获异常 | SDK 转 `ERROR` + failed Run |
| Run 轮询、终态、退出码 | `levier run` 包装 |

---

## 4. 兼容面

旧命令没有删除，但不再出现在主帮助里：

```text
levier app ...
levier provider ...
levier mcp ...
levier run submit|watch|show|cancel
levier stack ...
levier admin ...
```

它们用于：

```text
已有部署脚本
精确控制凭据路径
生产 secret 文件
诊断与自检
```

新增能力也不应该反向污染黄金路径。每增加一个参数，先判断它是不是应该
由 profile、项目配置或 SDK 默认值承担。

---

## 5. 有意的取舍

### 5.1 本地默认 Admin token

`levier up` 之后 profile 记录本地 server。`levier init`/`model connect` 在
loopback server 上使用开发默认 `admin-change-me`。这不是生产建议；生产应设置
`LEVIER_ADMIN_TOKEN` 或 `levier setup --admin-token`。

### 5.2 Managed model secrets

`levier model connect` 需要把 key 放到控制面。控制面提供
`LEVIER_SECRETS_WRITE_FILE`（0600 JSON）并通过 Admin API 更新它。生产仍然可以
只用环境变量或只读 secret 文件。后续可以替换成 KMS/Vault，不改变 CLI。

### 5.3 本地 runtime token

`levier up` 把本地 runtime token 写进 0600 profile，control plane 的开发配置
接受一个共享 development token（`daemon_tokens["*"]`）。共享/生产部署应移除
通配 token，改为显式 daemon token。

### 5.4 App 进程仍需保持在线

工具 handler 在 App 进程里执行，所以 `levier dev` 必须保持运行。
`levier run --with-app` 为一次性场景临时启动 `app.py`；如果已有 App 连接，
不会重复启动，避免同一工具被执行两次。

---

## 6. 下一步

```text
在真实 Compose 上跑通 levier up → model connect → init → run --with-app
managed secret 的加密实现
runtime 自动注册/令牌撤销
多项目 profile 的显式切换
Node SDK 的同等操作面
```
