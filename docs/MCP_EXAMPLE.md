# 第 4 页：MCP 工具怎么接

这一页只做一个真实例子：

> App 不写 MCP client；Levier 注册一个 MCP Server；React Runtime 调用它。

---

## 0. 先看四个角色

不要把四个地方混在一起：

| 你改哪里 | 属于谁 | 作用 |
|---|---|---|
| `examples/mcp_echo/server.py` | MCP Runtime | 真正的 MCP Server |
| `examples/mcp_echo/config.json` | Levier 的 MCP 目录 | 告诉 Levier 怎么启动 Server |
| `data/mcp-demo.json` | App | App 自己的身份凭据 |
| `execution/react_runtime/react.example.yaml` | React Runtime | 告诉 Runtime MCP socket 在哪 |

关系：

```text
App 只声明 mcp_refs
        |
        v
Levier 保存 MCP Server 目录
        |
        v
React Runtime 读取目录并连接 MCP Runtime
        |
        v
MCP Runtime 启动 MCP Server，执行工具
```

App 进程里没有 MCP client。

---

## 1. 例子目录已经放在项目里

项目里有：

```text
examples/mcp_echo/
  server.py
  config.json
```

`server.py` 是一个很小的 MCP Server，提供两个工具：

```text
echo_text(text)       原样返回文本
add_numbers(a, b)     返回两个整数之和
```

`config.json` 的内容是：

```json
{
  "command": "python",
  "args": ["/app/examples/mcp_echo/server.py"]
}
```

这个路径 `/app/examples/mcp_echo/server.py` 是 **MCP Runtime 容器里的路径**，
不是 App 里的路径，也不是 Levier 控制面的路径。

Docker Compose 已经把：

```text
./examples/mcp_echo
```

挂载到了：

```text
/app/examples/mcp_echo
```

如果你修改了 Compose，重新加载 MCP Runtime：

```powershell
docker compose up -d --force-recreate mcp-runtime
```

---

## 2. 注册 MCP Server

执行：

```powershell
levier mcp add `
  --name mcp-echo `
  --transport stdio `
  --config-file .\examples\mcp_echo\config.json `
  --allowed-tools echo_text,add_numbers
```

### 每个参数

| 参数 | 含义 |
|---|---|
| `--name mcp-echo` | MCP Server 的逻辑名字，后面 App 用这个名字引用 |
| `--transport stdio` | MCP Runtime 启动一个本地子进程 |
| `--config-file` | 告诉 Levier 如何启动这个 MCP Server |
| `--allowed-tools` | 只允许模型使用这两个工具 |

检查：

```powershell
levier mcp list
```

应该看到类似：

```text
name       transport    secret_refs
mcp-echo   stdio
```

如果看不到，说明 `mcp add` 没有成功，先不要继续。

---

## 3. 创建 App 并声明引用

创建一个专门用于验证 MCP 的 App：

```powershell
levier app init `
  --name mcp-demo `
  --server http://127.0.0.1:8080 `
  --admin-token admin-change-me `
  --credentials-out .\data\mcp-demo.json `
  --mcp-ref mcp-echo
```

### 参数解释

| 参数 | 含义 |
|---|---|
| `--name mcp-demo` | App 名字 |
| `--credentials-out` | App 凭据写到哪里 |
| `--mcp-ref mcp-echo` | 这个 App 要使用哪个 MCP Server |

这里不需要写 MCP Server 的 command、args 或 env。  
那些属于 MCP 目录，不属于 App。

如果 App 已经存在，不要重新 init。  
启动 App 时使用：

```powershell
$env:LEVIER_APP_MCP_REFS = "mcp-echo"
```

然后重启 App。

---

## 4. 启动一个 App 进程

MCP 例子使用的是已有示例 App。  
先在一个终端启动它：

```powershell
$env:LEVIER_CREDENTIALS = ".\data\mcp-demo.json"
$env:LEVIER_APP_MCP_REFS = "mcp-echo"

# 从源码运行时还需要：
# $env:PYTHONPATH = "sdk\levier\src;sdk\python\src"
python .\examples\counter_agent\app.py
```

这个 App 还会注册自己的 `counter_*` 本地工具。  
这不影响 MCP 例子。

这个终端要保持运行。

---

## 5. 确认 React Runtime 和 provider

MCP 工具由 React Runtime 决定是否调用，所以需要：

```text
React Runtime 在线
模型 provider 配置正确
```

检查：

```powershell
levier runtime list
levier provider check --name deepseek
```

React Runtime 不再需要自己的 MCP socket 配置；它的 `ctx.toolbox()` 从用户
profile 或环境变量取地址。Compose 内置 React Runtime 已注入：

```text
LEVIER_MCP_RUNTIME_SOCKET=/run/levier/mcp.sock
```

宿主机上的 Runtime 使用 `levier up` 写入的 `tcp://127.0.0.1:9091`。

如果 React Runtime 还是旧镜像，先重建：

```powershell
docker compose up -d --build react-runtime
```

---

## 6. 提交一个明确要求调用 MCP 的 prompt

```powershell
levier run submit `
  --credentials .\data\mcp-demo.json `
  --runtime react1 `
  --provider deepseek `
  --model deepseek-flash `
  --prompt "Call the tool named mcp__mcp_echo__echo_text with text `"hello mcp`", then report exactly what it returns." `
  --watch
```

MCP 工具在模型侧的名字是：

```text
mcp__mcp_echo__echo_text
```

它由三部分组成：

```text
mcp__ + server 名字（- 变 _） + 工具名
```

成功时应该看到类似：

```text
status: completed
final: hello mcp
```

---

## 7. 查看真实调用事件

```powershell
levier run show `
  --credentials .\data\mcp-demo.json `
  --run-id <上一步的 run id> `
  --events
```

你应该看到：

```text
RUN_STATUS
TOOL_CALL_BEGIN
TOOL_CALL_END
MESSAGE_COMPLETED
RUN_STATUS
```

其中 `TOOL_CALL_END` 的 `tool_name` 应该是：

```text
mcp__mcp_echo__echo_text
```

看 MCP Runtime 日志：

```powershell
docker compose logs -f mcp-runtime
```

如果 MCP 调用成功，日志里会出现连接和工具调用记录。

App 终端不会出现 `local_tool_call`，因为这不是 App 的本地工具。

---

## 8. MCP 配置、App 配置、Runtime 配置的边界

这是最容易混的地方。

### MCP Server 配置

文件：

```text
examples/mcp_echo/config.json
```

管的是：

```text
怎么启动 MCP Server
启动命令是什么
需要什么环境变量
```

### App 配置

文件：

```text
data/mcp-demo.json
```

管的是：

```text
App 的 app_id 和 API key
App 使用哪个控制面
```

另外：

```text
LEVIER_APP_MCP_REFS
```

只负责：

```text
这个 App 引用哪些 MCP Server
```

它不包含 MCP Server 的 command 或 env。

### Runtime 配置

文件：

```text
execution/react_runtime/react.example.yaml
```

管的是：

```text
Runtime 连接哪个控制面
Runtime 的 runtime_name
MCP Runtime socket 在哪里
```

### MCP secret

如果 MCP Server 需要 secret，配置里写：

```json
{
  "TOKEN": "${secret:my_token}"
}
```

真正的值由 MCP Runtime 读取：

```text
LEVIER_MCP_SECRETS_FILE
```

或 MCP Runtime 的：

```text
LEVIER_SECRET_MY_TOKEN
```

App 不需要知道这个 token。

---

## 9. 这一页常见错误

| 现象 | 真正原因 | 先查 |
|---|---|---|
| `MCP_RUNTIME_UNAVAILABLE` | MCP Runtime 没启动或 socket 不对 | `docker compose ps mcp-runtime` |
| `MCP_CONNECT_FAILED` | Server 命令/路径错误 | `mcp-runtime` 日志 |
| `SECRET_MISSING` | MCP 配置引用了不存在的 secret | `LEVIER_MCP_SECRETS_FILE` |
| `TOOL_DENIED` | 工具不在 `allowed_tools` | `mcp list` |
| `MCP_NOT_FOUND` | MCP Server 没注册 | `mcp list` |
| App 注册时报 unknown ref | App 的 ref 名字和 `--name` 不一致 | `--name` 和 `--mcp-ref` |

---

## 10. 一句话总结

```text
MCP Server 配置放在 MCP 目录里
App 只写 mcp_refs
Runtime 只写 mcp socket
MCP secret 由 MCP Runtime 解析
```

## 11. 真实运行结果

按上面的例子实际运行时，`run submit --watch` 应该看到类似：

```text
run submitted: 01...
status: queued
status: running
status: completed
runtime_name: react1
final: The tool returned exactly: `hello mcp`
```

`run show --events` 应该看到：

```text
RUN_STATUS
USAGE
MESSAGE_COMPLETED
TOOL_CALL_BEGIN
TOOL_CALL_END
USAGE
MESSAGE_COMPLETED
RUN_STATUS
```

MCP Runtime 日志应该出现：

```text
mcp runtime listening
mcp session opened
mcp session closed
```

如果 `mcp list` 成功，但运行时报工具不存在，先看 MCP Runtime 日志。  
例如：

```text
python: can't open file '/app/examples/mcp_echo/server.py': No such file or directory
```

这说明 MCP Runtime 容器没有挂载示例目录。重新加载：

```powershell
docker compose up -d --force-recreate mcp-runtime
```

如果看到 session 打开和关闭，但 Runtime 仍然没有工具，检查
`LEVIER_MCP_RUNTIME_SOCKET` / profile 里的 `mcp_socket` 是否和 MCP Runtime
使用同一个地址。

真实回归测试仍然可以运行：

```powershell
bash tests/e2e/mcp_e2e.sh
```

这个脚本会启动控制面、MCP Runtime、React Runtime 和一个 App，真实调用一次
MCP 工具并检查事件和结果。Windows 上需要 Git Bash/WSL、PostgreSQL 和 Python
环境；如果本机不方便，按上面第 2～7 步手动执行一次即可。
