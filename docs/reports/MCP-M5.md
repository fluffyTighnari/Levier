<!-- Middleware — MCP M5 real server. -->

# MCP M5：真实第三方 MCP server

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-11 |
| 状态 | 完成 |
| Server | `@modelcontextprotocol/server-filesystem` 2.3.0 |
| 运行环境 | Node v24.21.0，stdio |
| 样本 Run | `01M75CDS14H00WJXC5228AKJ09` |

## 1. 选择

用官方参考实现，而不是自写 fixture：

```text
包    @modelcontextprotocol/server-filesystem
版本  2.3.0
入口  /tmp/m5-fs/node_modules/.../dist/index.js
参数  /tmp/m5-mcp
```

被测文件：

```text
/tmp/m5-mcp/hello.txt  →  m5-content-from-filesystem
```

## 2. 目录与 allowlist

```text
name        m5fs
transport   stdio
command     node
args        [index.js, /tmp/m5-mcp]
allowed     read_text_file, list_directory
```

Server 本身暴露 16 个工具：

```text
read_file / read_text_file / read_media_file / read_multiple_files
write_file / append_file / edit_file
create_directory / list_directory / list_directory_with_sizes / directory_tree
move_file / copy_file / search_files / get_file_info / list_allowed_directories
```

allowlist 后 Harness 只看到：

```json
{"visible_tools": ["mcp__m5fs__read_text_file", "mcp__m5fs__list_directory"]}
```

## 3. 端到端

```text
App mcp_refs = [m5fs]
Run react / deepseek-flash
prompt: 用 mcp__m5fs__read_text_file 读 /tmp/m5-mcp/hello.txt
```

事件：

```text
seq 4  TOOL_CALL_BEGIN  mcp__m5fs__read_text_file
seq 5  TOOL_CALL_END    ok=true
       {"content":[{"type":"text","text":"m5-content-from-filesystem\n"}],
        "structured_content":{"content":"m5-content-from-filesystem\n"},
        "is_error":false}
seq 7  MESSAGE_COMPLETED  "... Exact text payload ..."
seq 8  RUN_STATUS completed
```

其他：

```text
tool_invocations              0
runtime session opened/closed 1 次
run 结束后 node server 进程    0
```

## 4. 这证明了什么

| 项目 | 结果 |
|---|---|
| 第三方 MCP server 可注册 | 是 |
| 真实 tools/list（16 个） | 是 |
| allowlist 在真实 server 上生效 | 是（2 个） |
| 模型可调用真实工具 | 是 |
| 结果进入下一轮模型输入 | 是 |
| 不写 Gateway invocation | 是 |
| 进程回收 | 是 |

## 5. 局限

- 依赖 npm 安装（本机已缓存/可访问 registry），生产需固定版本与离线镜像。
- 只测 stdio；SSE / streamable_http 的真实 server 还未覆盖。
- 文件系统 server 的 allowed-directories 只给了 `/tmp/m5-mcp`。
- 未做写操作与权限边界（只读工具）。

## 6. 证据

```text
docs/reports/artifacts/mcp/M5/
  versions.txt         server 2.3.0 / node v24.21.0
  probe.json           全量工具列表 + 读取结果
  probe_allow.json     allowlist 后仅 2 个工具
  catalog.json / catalog.status
  app.json / app.status / bootstrap.json
  db.txt               事件、invocation=0、最终回答
  runtime.log
  harness.log
  middleware.log
  migrate.log
  submit_run.json
  summary.txt
```

## 7. 下一步

M6：收口。

```text
make mcp-e2e      目录 → App → Run → MCP → 断言 → 清理
docs/MCP.md       运行边界、注册方式、失败语义
```
